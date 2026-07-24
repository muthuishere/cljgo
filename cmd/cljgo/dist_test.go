// dist_test.go — the gate for `cljgo dist` (ADR 0077). The headline claim
// is that one host cross-compiles a cljgo program to every platform, so the
// build-gated test proves genuine cross-compilation by MAGIC BYTES: a
// linux/amd64 artifact is a real ELF, a windows/amd64 artifact a real PE,
// a darwin artifact a real Mach-O — none of which the macOS/linux CI host
// could produce by accident. Target resolution is unit-tested without a build.
package main

import (
	"bufio"
	"bytes"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestResolveTargets(t *testing.T) {
	// default (no flags) is the mainstream matrix
	def, err := resolveTargets("", false)
	if err != nil || len(def) != len(defaultMatrix) {
		t.Fatalf("default targets = %v, %v; want the %d-target matrix", def, err, len(defaultMatrix))
	}

	// an explicit, valid subset
	got, err := resolveTargets("linux/amd64,windows/amd64", false)
	if err != nil {
		t.Fatalf("valid --target: %v", err)
	}
	if len(got) != 2 || got[0].String() != "linux/amd64" || got[1].String() != "windows/amd64" {
		t.Fatalf("--target parse = %v", got)
	}

	// malformed and unsupported targets are named errors, not build failures
	for _, bad := range []string{"linux", "not-a-target", "linux/nope", "/amd64", "windows/"} {
		if _, err := resolveTargets(bad, false); err == nil {
			t.Errorf("resolveTargets(%q) should error", bad)
		}
	}
}

// magic returns the first n bytes of a file.
func magic(t *testing.T, path string, n int) []byte {
	t.Helper()
	f, err := os.Open(path)
	if err != nil {
		t.Fatalf("open %s: %v", path, err)
	}
	defer f.Close()
	b := make([]byte, n)
	if _, err := f.Read(b); err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	return b
}

// TestDistCrossCompiles is ADR 0077's proof: `cljgo dist` on a single host
// emits real ELF / PE / Mach-O binaries + a checksums file, and the host
// artifact runs.
func TestDistCrossCompiles(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping cross-compile build in -short mode")
	}
	bin := buildCljgo(t)
	app := t.TempDir()
	if err := os.WriteFile(filepath.Join(app, "hello.clj"),
		[]byte(`(println "hello from cljgo dist")`), 0o644); err != nil {
		t.Fatal(err)
	}

	cmd := exec.Command(bin, "dist", "--target", "darwin/arm64,linux/amd64,windows/amd64", "hello.clj")
	cmd.Dir = app
	cmd.Env = append(os.Environ(), "CLJGO_SRC="+repoRoot(t))
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("cljgo dist: %v\n%s", err, out)
	}

	dist := filepath.Join(app, "dist")
	cases := []struct {
		file  string
		magic []byte
	}{
		{"hello_linux-amd64", []byte{0x7f, 'E', 'L', 'F'}},     // ELF
		{"hello_windows-amd64.exe", []byte{'M', 'Z'}},          // PE
		{"hello_darwin-arm64", []byte{0xcf, 0xfa, 0xed, 0xfe}}, // Mach-O 64 LE
	}
	for _, c := range cases {
		p := filepath.Join(dist, c.file)
		if _, err := os.Stat(p); err != nil {
			t.Fatalf("expected artifact %s: %v", c.file, err)
		}
		if got := magic(t, p, len(c.magic)); !bytes.Equal(got, c.magic) {
			t.Errorf("%s magic = % x, want % x — not a genuine cross-compiled binary", c.file, got, c.magic)
		}
	}

	// checksums.txt: one sha256sum -c line per artifact, names matching.
	csb, err := os.ReadFile(filepath.Join(dist, "checksums.txt"))
	if err != nil {
		t.Fatalf("checksums.txt: %v", err)
	}
	seen := map[string]bool{}
	sc := bufio.NewScanner(bytes.NewReader(csb))
	for sc.Scan() {
		fields := strings.Fields(sc.Text())
		if len(fields) != 2 || len(fields[0]) != 64 {
			t.Errorf("bad checksum line: %q", sc.Text())
			continue
		}
		seen[fields[1]] = true
	}
	for _, c := range cases {
		if !seen[c.file] {
			t.Errorf("checksums.txt is missing %s", c.file)
		}
	}

	// the host artifact runs (this test host is darwin/arm64 or linux/amd64).
	for _, c := range cases {
		if strings.Contains(c.file, hostSlug()) {
			out, err := exec.Command(filepath.Join(dist, c.file)).CombinedOutput()
			if err != nil {
				t.Fatalf("running host artifact %s: %v\n%s", c.file, err, out)
			}
			if strings.TrimSpace(string(out)) != "hello from cljgo dist" {
				t.Fatalf("host artifact printed %q", out)
			}
		}
	}
}

// TestDistProjectAndLibraryError covers the project path (build.cljgo install
// artifact) and the library rejection.
func TestDistProjectAndLibraryError(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping cross-compile build in -short mode")
	}
	bin := buildCljgo(t)

	// project (cli template): dist produces a named, cross-compiled binary.
	work := t.TempDir()
	if out, err := runIn(work, bin, "new", "--template", "cli", "mytool"); err != nil {
		t.Fatalf("cljgo new cli: %v\n%s", err, out)
	}
	app := filepath.Join(work, "mytool")
	cmd := exec.Command(bin, "dist", "--target", "linux/arm64")
	cmd.Dir = app
	cmd.Env = append(os.Environ(), "CLJGO_SRC="+repoRoot(t))
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("cljgo dist (project): %v\n%s", err, out)
	}
	p := filepath.Join(app, "dist", "mytool_linux-arm64")
	if got := magic(t, p, 4); !bytes.Equal(got, []byte{0x7f, 'E', 'L', 'F'}) {
		t.Errorf("project artifact is not an ELF: % x", got)
	}

	// library (no --template = lib, no install step): dist errors, pointing
	// the author at `cljgo publish`.
	lib := t.TempDir()
	if out, err := runIn(lib, bin, "new", "demo"); err != nil {
		t.Fatalf("cljgo new lib: %v\n%s", err, out)
	}
	out, err := runIn(filepath.Join(lib, "demo"), bin, "dist")
	if err == nil {
		t.Fatal("cljgo dist on a library should fail")
	}
	if !strings.Contains(out, "publish") {
		t.Errorf("the library dist error should point at `cljgo publish`, got: %q", out)
	}
}

// hostSlug is this test host's target slug (e.g. "darwin-arm64"), so the
// cross-compile test knows which artifact it can actually execute.
func hostSlug() string { return runtime.GOOS + "-" + runtime.GOARCH }
