// s51 — Clojars/Maven DEPLOY round-trip spike (ADR 0095 decision 3).
//
// Falsifiable question: does a PURE-GO Maven deploy — generated pom.xml + a
// source-bearing .jar + .sha1/.md5 checksums, in the correct Maven repository
// layout — round-trip? I.e. can we PUBLISH a pure-Clojure library and then
// CONSUME it back (the s50 resolver) and recover byte-identical source?
//
// Kill condition: Clojars' deploy protocol needs JVM-only tooling (gpg signing
// we can't do shell-free, maven-metadata.xml races) that breaks the pure-Go
// constraint, OR a self-deployed jar can't be consumed back.
//
// SAFETY: this spike deploys to a LOCAL FILE-BASED repo dir and consumes it back
// — it does NOT push to public Clojars. The authenticated HTTP PUT to
// clojars.org is implemented (deployHTTP) but GATED behind CLOJARS_DEPLOY=1 +
// credentials in the environment, so it never fires during a normal spike run
// and never bakes a secret. Stdlib only.
package main

import (
	"archive/zip"
	"bytes"
	"crypto/md5"
	"crypto/sha1"
	"encoding/xml"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
)

type coord struct{ group, artifact, version string }

func (c coord) dir() string {
	return filepath.Join(filepath.FromSlash(strings.ReplaceAll(c.group, ".", "/")), c.artifact, c.version)
}
func (c coord) base() string { return c.artifact + "-" + c.version }

// ---- a tiny pure-Clojure "library" to publish ----

var librarySource = map[string]string{
	"greetlib/core.clj": `(ns greetlib.core)

(defn greet
  "Pure Clojure — no Java, consumable on cljgo's Go host."
  [who]
  (str "Hello, " who "!"))

(defn shout [who]
  (clojure.string/upper-case (greet who)))
`,
}

// ---- 1. build the Maven artifact (pom + source jar) ----

func buildPom(c coord, deps []coord) []byte {
	var b strings.Builder
	b.WriteString(`<?xml version="1.0" encoding="UTF-8"?>` + "\n")
	b.WriteString(`<project xmlns="http://maven.apache.org/POM/4.0.0">` + "\n")
	b.WriteString("  <modelVersion>4.0.0</modelVersion>\n")
	fmt.Fprintf(&b, "  <groupId>%s</groupId>\n", c.group)
	fmt.Fprintf(&b, "  <artifactId>%s</artifactId>\n", c.artifact)
	fmt.Fprintf(&b, "  <version>%s</version>\n", c.version)
	b.WriteString("  <packaging>jar</packaging>\n")
	fmt.Fprintf(&b, "  <name>%s</name>\n", c.artifact)
	if len(deps) > 0 {
		b.WriteString("  <dependencies>\n")
		for _, d := range deps {
			b.WriteString("    <dependency>\n")
			fmt.Fprintf(&b, "      <groupId>%s</groupId>\n", d.group)
			fmt.Fprintf(&b, "      <artifactId>%s</artifactId>\n", d.artifact)
			fmt.Fprintf(&b, "      <version>%s</version>\n", d.version)
			b.WriteString("    </dependency>\n")
		}
		b.WriteString("  </dependencies>\n")
	}
	b.WriteString("</project>\n")
	return []byte(b.String())
}

// buildSourceJar zips the pure .clj source tree — the payload a JVM Clojure
// consumer compiles (ADR 0054: cljgo reaches Clojure only as pure source).
func buildSourceJar(src map[string]string) ([]byte, error) {
	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	// deterministic order for a stable artifact
	names := make([]string, 0, len(src))
	for n := range src {
		names = append(names, n)
	}
	sortStrings(names)
	for _, n := range names {
		w, err := zw.Create(n)
		if err != nil {
			return nil, err
		}
		if _, err := w.Write([]byte(src[n])); err != nil {
			return nil, err
		}
	}
	if err := zw.Close(); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

// ---- 2. checksums (Maven requires .sha1 + .md5 next to each artifact) ----

func sha1hex(b []byte) string { s := sha1.Sum(b); return fmt.Sprintf("%x", s) }
func md5hex(b []byte) string  { s := md5.Sum(b); return fmt.Sprintf("%x", s) }

// ---- 3a. deploy to a LOCAL file repo (the safe round-trip) ----

func deployLocal(repoRoot string, c coord, jar, pom []byte) ([]string, error) {
	dir := filepath.Join(repoRoot, c.dir())
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return nil, err
	}
	artifacts := map[string][]byte{
		c.base() + ".jar": jar,
		c.base() + ".pom": pom,
	}
	var written []string
	for name, data := range artifacts {
		for suffix, content := range map[string][]byte{
			"":      data,
			".sha1": []byte(sha1hex(data)),
			".md5":  []byte(md5hex(data)),
		} {
			p := filepath.Join(dir, name+suffix)
			if err := os.WriteFile(p, content, 0o644); err != nil {
				return nil, err
			}
			written = append(written, p)
		}
	}
	return written, nil
}

// ---- 3b. deploy to Clojars over HTTP (GATED — never fires without opt-in) ----

// deployHTTP PUTs the same files to a Maven repo (Clojars). It reads credentials
// from the environment at the point of use and NEVER logs or bakes them. It is
// only reachable when CLOJARS_DEPLOY=1 — the default spike run does the local
// round-trip only, so no public artifact is ever published from a spike.
func deployHTTP(repoBase string, c coord, jar, pom []byte) error {
	user := os.Getenv("CLOJARS_USERNAME")
	pass := os.Getenv("CLOJARS_PASSWORD") // a Clojars DEPLOY TOKEN, not a password
	if user == "" || pass == "" {
		return fmt.Errorf("set CLOJARS_USERNAME and CLOJARS_PASSWORD (deploy token) to deploy; " +
			"mint a token at clojars.org > Deploy Tokens")
	}
	base := strings.TrimRight(repoBase, "/") + "/" + filepath.ToSlash(c.dir()) + "/"
	files := map[string][]byte{
		c.base() + ".jar":      jar,
		c.base() + ".jar.sha1": []byte(sha1hex(jar)),
		c.base() + ".jar.md5":  []byte(md5hex(jar)),
		c.base() + ".pom":      pom,
		c.base() + ".pom.sha1": []byte(sha1hex(pom)),
		c.base() + ".pom.md5":  []byte(md5hex(pom)),
	}
	for name, data := range files {
		req, err := http.NewRequest(http.MethodPut, base+name, bytes.NewReader(data))
		if err != nil {
			return err
		}
		req.SetBasicAuth(user, pass) // value never printed
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			return err
		}
		resp.Body.Close()
		if resp.StatusCode/100 != 2 {
			return fmt.Errorf("PUT %s: HTTP %d", name, resp.StatusCode)
		}
	}
	return nil
}

// ---- 4. consume it back from the local repo (s50-style) ----

type pomModel struct {
	Deps []struct {
		GroupID    string `xml:"groupId"`
		ArtifactID string `xml:"artifactId"`
		Version    string `xml:"version"`
	} `xml:"dependencies>dependency"`
}

func consumeLocal(repoRoot string, c coord) (map[string]string, []coord, error) {
	dir := filepath.Join(repoRoot, c.dir())
	// 4a. read + verify the pom, parse deps
	pomBytes, err := os.ReadFile(filepath.Join(dir, c.base()+".pom"))
	if err != nil {
		return nil, nil, err
	}
	if err := verifyChecksum(dir, c.base()+".pom", pomBytes); err != nil {
		return nil, nil, err
	}
	var pm pomModel
	if err := xml.Unmarshal(pomBytes, &pm); err != nil {
		return nil, nil, err
	}
	var deps []coord
	for _, d := range pm.Deps {
		deps = append(deps, coord{d.GroupID, d.ArtifactID, d.Version})
	}
	// 4b. read + verify the jar, extract source
	jarBytes, err := os.ReadFile(filepath.Join(dir, c.base()+".jar"))
	if err != nil {
		return nil, nil, err
	}
	if err := verifyChecksum(dir, c.base()+".jar", jarBytes); err != nil {
		return nil, nil, err
	}
	zr, err := zip.NewReader(bytes.NewReader(jarBytes), int64(len(jarBytes)))
	if err != nil {
		return nil, nil, err
	}
	out := map[string]string{}
	for _, f := range zr.File {
		if !strings.HasSuffix(f.Name, ".clj") {
			continue
		}
		rc, _ := f.Open()
		b, _ := io.ReadAll(rc)
		rc.Close()
		out[f.Name] = string(b)
	}
	return out, deps, nil
}

func verifyChecksum(dir, name string, data []byte) error {
	want, err := os.ReadFile(filepath.Join(dir, name+".sha1"))
	if err != nil {
		return err
	}
	if got := sha1hex(data); got != string(want) {
		return fmt.Errorf("%s sha1 mismatch: got %s want %s", name, got, want)
	}
	return nil
}

func main() {
	fmt.Println("s51 — Clojars/Maven deploy round-trip (pure-Go stdlib only)")
	c := coord{"io.github.muthuishere", "greetlib", "0.1.0"}
	deps := []coord{{"org.clojure", "tools.cli", "1.1.230"}} // a pure transitive dep

	// 1. build artifact
	pom := buildPom(c, deps)
	jar, err := buildSourceJar(librarySource)
	must(err)
	fmt.Printf("\n1. built artifact: %s\n   pom %d bytes, source jar %d bytes (sha1 %s…)\n",
		c.base(), len(pom), len(jar), sha1hex(jar)[:12])

	// 2. deploy to a local file repo (SAFE — no public push)
	repoRoot, err := os.MkdirTemp("", "s51-repo-")
	must(err)
	defer os.RemoveAll(repoRoot)
	written, err := deployLocal(repoRoot, c, jar, pom)
	must(err)
	fmt.Printf("\n2. deployed to local repo %s\n   %d files in Maven layout:\n", repoRoot, len(written))
	for _, w := range written {
		fmt.Printf("     %s\n", strings.TrimPrefix(w, repoRoot+"/"))
	}

	// 3. consume it back
	source, gotDeps, err := consumeLocal(repoRoot, c)
	must(err)
	fmt.Printf("\n3. consumed back: %d source file(s), %d transitive dep(s), checksums verified\n",
		len(source), len(gotDeps))

	// 4. round-trip assertion: byte-identical source recovered
	ok := true
	for name, orig := range librarySource {
		if source[name] != orig {
			fmt.Printf("   ✗ %s DIFFERS after round-trip\n", name)
			ok = false
		} else {
			fmt.Printf("   ✓ %s byte-identical (%d bytes)\n", name, len(orig))
		}
	}
	depOK := len(gotDeps) == 1 && gotDeps[0] == deps[0]
	fmt.Printf("   %s transitive dep recovered from pom: %v\n", tick(depOK), gotDeps)

	// 5. HTTP deploy: report reachability WITHOUT firing (gated)
	fmt.Printf("\n4. public Clojars deploy path (HTTP PUT + basic-auth deploy token):\n")
	if os.Getenv("CLOJARS_DEPLOY") == "1" {
		fmt.Printf("   CLOJARS_DEPLOY=1 — attempting real deploy to https://repo.clojars.org …\n")
		if err := deployHTTP("https://repo.clojars.org", c, jar, pom); err != nil {
			fmt.Printf("   deploy error: %v\n", err)
		} else {
			fmt.Printf("   deployed.\n")
		}
	} else {
		fmt.Printf("   NOT fired (default). Set CLOJARS_DEPLOY=1 + CLOJARS_USERNAME/CLOJARS_PASSWORD\n")
		fmt.Printf("   to push for real. The PUT is 6 authenticated requests (jar/pom + .sha1/.md5\n")
		fmt.Printf("   each) — mechanically identical to the local write above.\n")
	}

	fmt.Printf("\n═══ VERDICT: %s ═══\n", verdict(ok && depOK))
}

func verdict(ok bool) string {
	if ok {
		return "ROUND-TRIP MET — published pure source recovered byte-identical, pure-Go, no JVM"
	}
	return "FAILED — round-trip did not recover the artifact"
}

func tick(b bool) string {
	if b {
		return "✓"
	}
	return "✗"
}
func must(err error) {
	if err != nil {
		fmt.Fprintln(os.Stderr, "fatal:", err)
		os.Exit(1)
	}
}

// sortStrings — tiny stdlib-free insertion sort (spike avoids importing sort for
// one call; deterministic jar ordering only).
func sortStrings(s []string) {
	for i := 1; i < len(s); i++ {
		for j := i; j > 0 && s[j-1] > s[j]; j-- {
			s[j-1], s[j] = s[j], s[j-1]
		}
	}
}
