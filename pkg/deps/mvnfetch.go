package deps

// Fetching and extracting Maven artifacts, pure Go: net/http + archive/zip +
// crypto/sha256 (+ encoding/xml next door). This is exactly what spike s50
// proved sufficient — no JVM, no mvn, no Aether.
//
// Every failure here is a coded diagnostic. A DNS failure, a TLS error, a 500,
// a truncated zip: none of them reaches the user as a wrapped *url.Error.

import (
	"archive/zip"
	"bytes"
	"crypto/sha1"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path"
	"path/filepath"
	"strings"
	"time"
)

// Fetch limits — a hostile or broken jar must not exhaust the machine.
const (
	maxArtifactBytes = 64 << 20 // 64 MiB per downloaded file
	maxZipEntries    = 10000
	maxZipEntrySize  = 32 << 20
)

// httpClientFor returns the HTTP client resolution uses. Tests inject their
// own through ResolveOptions.MvnHTTPClient and point MvnRepos at an
// httptest.Server, so the code path under test IS the production one.
func httpClientFor(opts ResolveOptions) *http.Client {
	if opts.MvnHTTPClient != nil {
		return opts.MvnHTTPClient
	}
	return &http.Client{Timeout: 60 * time.Second}
}

// repoList returns the repositories to shop, in order: project-declared
// (mvn-repo …) entries first, then the defaults.
func repoList(opts ResolveOptions) []string {
	out := append([]string(nil), opts.MvnRepos...)
	if len(out) == 0 {
		out = append(out, DefaultMvnRepos...)
	}
	return out
}

// repoAttempt records one repository's answer, so G5010 can name every URL
// tried with the status it returned.
type repoAttempt struct {
	Repo   string
	Status string
}

// fetchArtifact returns the bytes of one artifact file, from the cache when
// warm and from the first repository that answers 200 otherwise. It returns
// the repository that served it, so the lock can record it and a later resolve
// need not re-shop the list.
//
// When pinnedRepo is non-empty (the lock already recorded which repository
// served this coordinate) only that repository is consulted.
func fetchArtifact(root string, c Coord, ext, pinnedRepo string, opts ResolveOptions) (data []byte, repo string, err error) {
	repos := repoList(opts)
	if pinnedRepo != "" {
		repos = []string{pinnedRepo}
	}

	// Cache first — a warm cache resolves with no network at all.
	for _, r := range repos {
		if b, err := os.ReadFile(mvnBlobPath(root, r, c, ext)); err == nil {
			return b, r, nil
		}
	}
	if opts.Offline {
		return nil, "", codedf("G5014", "offline: maven coordinate %s is not in the cache (%s)",
			c, mvnBlobPath(root, repos[0], c, ext)).
			withFix("build once while online to populate the cache, then --offline works")
	}

	client := httpClientFor(opts)
	var attempts []repoAttempt
	for _, r := range repos {
		url := strings.TrimSuffix(r, "/") + "/" + c.artifactPath(ext)
		b, status, err := httpGet(client, url)
		if err != nil {
			attempts = append(attempts, repoAttempt{Repo: r, Status: err.Error()})
			continue
		}
		if status != http.StatusOK {
			attempts = append(attempts, repoAttempt{Repo: r, Status: fmt.Sprintf("%d %s", status, http.StatusText(status))})
			continue
		}
		// The repository's .sha1 sidecar, when it serves one, is a cheap
		// corroboration at download time; build.lock.edn's sha256 is the real
		// integrity story and is checked by the caller.
		if err := verifySHA1(client, url, b, c, ext); err != nil {
			return nil, "", err
		}
		if err := writeBlob(mvnBlobPath(root, r, c, ext), b); err != nil {
			return nil, "", err
		}
		return b, r, nil
	}

	e := codedf("G5010", "maven coordinate %s not found in any repository (%s)", c, strings.TrimPrefix(ext, "."))
	for _, a := range attempts {
		e.note("tried " + a.Repo + " — " + a.Status)
	}
	return nil, "", e.withFix("check the coordinate, or add its repository with (mvn-repo b \"https://…\")")
}

// httpGet performs one GET, capping the body. A transport failure is returned
// as an error whose text is the reason G5010 will print for that repository.
func httpGet(client *http.Client, url string) ([]byte, int, error) {
	resp, err := client.Get(url)
	if err != nil {
		return nil, 0, errors.New(transportReason(err))
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		io.Copy(io.Discard, io.LimitReader(resp.Body, 4096))
		return nil, resp.StatusCode, nil
	}
	b, err := io.ReadAll(io.LimitReader(resp.Body, maxArtifactBytes+1))
	if err != nil {
		return nil, 0, errors.New("truncated response: " + err.Error())
	}
	if len(b) > maxArtifactBytes {
		return nil, 0, fmt.Errorf("artifact exceeds the %d MiB limit", maxArtifactBytes>>20)
	}
	return b, http.StatusOK, nil
}

// transportReason renders a transport failure as a short, non-Go-shaped
// reason ("no such host", "connection refused", "TLS handshake failed").
func transportReason(err error) string {
	s := err.Error()
	switch {
	case strings.Contains(s, "no such host"):
		return "no such host"
	case strings.Contains(s, "connection refused"):
		return "connection refused"
	case strings.Contains(s, "certificate") || strings.Contains(s, "tls:"):
		return "TLS handshake failed"
	case strings.Contains(s, "timeout") || strings.Contains(s, "deadline exceeded"):
		return "timed out"
	}
	if i := strings.LastIndex(s, ": "); i >= 0 {
		return s[i+2:]
	}
	return s
}

// verifySHA1 checks the repository's .sha1 sidecar when it serves one. A
// missing sidecar is not an error (many repositories omit them); a MISMATCH is
// G5012.
func verifySHA1(client *http.Client, url string, body []byte, c Coord, ext string) error {
	b, status, err := httpGet(client, url+".sha1")
	if err != nil || status != http.StatusOK {
		return nil // no sidecar served — the lock's sha256 remains the real check
	}
	want := strings.ToLower(strings.TrimSpace(string(b)))
	if i := strings.IndexAny(want, " \t\r\n"); i >= 0 {
		want = want[:i]
	}
	if len(want) != 40 {
		return nil // not a sha1 sidecar; ignore rather than guess
	}
	sum := sha1.Sum(body)
	got := hex.EncodeToString(sum[:])
	if got != want {
		return codedf("G5012", "maven artifact checksum mismatch for %s (%s)", c, strings.TrimPrefix(ext, ".")).
			withExpectedFound("sha1:"+want+" (repository sidecar)", "sha1:"+got).
			withFix("run `cljgo cache clean` and resolve again")
	}
	return nil
}

// writeBlob stores raw artifact bytes in the cache, atomically.
func writeBlob(dst string, b []byte) error {
	if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
		return err
	}
	tmp, err := os.CreateTemp(filepath.Dir(dst), ".tmp-")
	if err != nil {
		return err
	}
	defer os.Remove(tmp.Name())
	if _, err := tmp.Write(b); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	if err := os.Rename(tmp.Name(), dst); err != nil {
		if _, statErr := os.Stat(dst); statErr == nil {
			return nil // a racer published the same immutable bytes
		}
		return err
	}
	return nil
}

// mvnSourceExts are the entries kept when a jar is extracted. .cljs is kept
// only so a .cljc's sibling does not look missing; it is never a load-path
// candidate. Everything else — .class files, META-INF/, nested jars — is
// dropped. Dropping META-INF/ alone removes a real spike-s50 false positive:
// clj-http ships META-INF/leiningen/…/project.clj, which s50's scanner counted
// as a Java namespace.
var mvnSourceExts = map[string]bool{
	".clj": true, ".cljc": true, ".cljs": true, ".edn": true,
}

// extractJar writes the source subset of a jar into dir.
func extractJar(jar []byte, dir string, c Coord) error {
	zr, err := zip.NewReader(bytes.NewReader(jar), int64(len(jar)))
	if err != nil {
		return codedf("G5012", "cannot read the jar of %s: %v", c, err).
			withFix("run `cljgo cache clean` and resolve again")
	}
	if len(zr.File) > maxZipEntries {
		return codedf("G5012", "the jar of %s has %d entries, over the %d-entry limit", c, len(zr.File), maxZipEntries)
	}
	for _, f := range zr.File {
		name := f.Name
		if f.FileInfo().IsDir() || name == "" {
			continue
		}
		if !mvnSourceExts[strings.ToLower(path.Ext(name))] {
			continue
		}
		if strings.HasPrefix(name, "META-INF/") {
			continue
		}
		// A jar-root build script (Leiningen's project.clj and friends) is
		// not a library namespace. Dropping it at extraction keeps it off the
		// load path AND out of the namespace count — the same reason META-INF/
		// is dropped one line above.
		if isMvnBuildScript(name) {
			continue
		}
		// zip-slip: refuse absolute paths and any ".." segment outright.
		clean := path.Clean("/" + name)
		if clean == "/" {
			continue
		}
		rel := strings.TrimPrefix(clean, "/")
		if strings.HasPrefix(rel, "../") || strings.Contains(rel, "/../") {
			return codedf("G5012", "the jar of %s contains an unsafe path %q", c, name)
		}
		if f.UncompressedSize64 > maxZipEntrySize {
			return codedf("G5012", "the jar of %s has an entry over the %d MiB limit (%s)", c, maxZipEntrySize>>20, name)
		}
		rc, err := f.Open()
		if err != nil {
			return codedf("G5012", "cannot read %s from the jar of %s: %v", name, c, err)
		}
		body, err := io.ReadAll(io.LimitReader(rc, maxZipEntrySize+1))
		rc.Close()
		if err != nil {
			return codedf("G5012", "cannot read %s from the jar of %s: %v", name, c, err)
		}
		if int64(len(body)) > maxZipEntrySize {
			return codedf("G5012", "the jar of %s has an entry over the %d MiB limit (%s)", c, maxZipEntrySize>>20, name)
		}
		out := filepath.Join(dir, filepath.FromSlash(rel))
		if err := os.MkdirAll(filepath.Dir(out), 0o755); err != nil {
			return err
		}
		if err := os.WriteFile(out, body, 0o644); err != nil {
			return err
		}
	}
	return nil
}
