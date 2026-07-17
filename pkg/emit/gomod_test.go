package emit

// ADR 0028 coverage: SynthGoMod's require-vs-replace decision (offline,
// every branch of the precedence chain) plus the end-to-end release-pin
// build against the public Go module proxy — the S12 replay.

import (
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/muthuishere/cljgo/pkg/corelib"
	"github.com/muthuishere/cljgo/pkg/lang"
	"github.com/muthuishere/cljgo/pkg/version"
)

// setVersion swaps pkg/version.Version for one test, simulating the release
// ldflags stamp ("0.1.0") or the in-source dev default.
func setVersion(t *testing.T, v string) {
	t.Helper()
	old := version.Version
	version.Version = v
	t.Cleanup(func() { version.Version = old })
}

func readGoMod(t *testing.T, dir string) string {
	t.Helper()
	b, err := os.ReadFile(filepath.Join(dir, "go.mod"))
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

func repoRoot(t *testing.T) string {
	t.Helper()
	abs, err := filepath.Abs(filepath.Join("..", ".."))
	if err != nil {
		t.Fatal(err)
	}
	return abs
}

// TestSynthGoModReleasePin: a release binary with no override writes the
// bare require pinned to its own version — no replace (ADR 0028 case 1).
func TestSynthGoModReleasePin(t *testing.T) {
	setVersion(t, "0.1.0")
	t.Setenv("CLJGO_SRC", "") // empty == unset for the precedence check
	dir := t.TempDir()
	if err := SynthGoMod(dir, "", "", nil); err != nil {
		t.Fatalf("SynthGoMod: %v", err)
	}
	got := readGoMod(t, dir)
	if !strings.Contains(got, runtimeModule+" v0.1.0") {
		t.Fatalf("go.mod missing release pin:\n%s", got)
	}
	if strings.Contains(got, "replace") {
		t.Fatalf("release-pinned go.mod must not carry a replace:\n%s", got)
	}
}

// TestSynthGoModDevWalkUp: a dev binary keeps today's walk-up replace —
// this is the conformance harness's path and must stay offline.
func TestSynthGoModDevWalkUp(t *testing.T) {
	setVersion(t, "0.1.0-dev")
	t.Setenv("CLJGO_SRC", "")
	dir := t.TempDir()
	if err := SynthGoMod(dir, "", "", nil); err != nil {
		t.Fatalf("SynthGoMod: %v", err)
	}
	got := readGoMod(t, dir)
	if !strings.Contains(got, "replace "+runtimeModule+" => "+repoRoot(t)) {
		t.Fatalf("dev go.mod missing walk-up replace:\n%s", got)
	}
}

// TestSynthGoModExplicitRuntimeBeatsRelease: the -runtime flag forces a
// replace even in a release binary (precedence: flag first).
func TestSynthGoModExplicitRuntimeBeatsRelease(t *testing.T) {
	setVersion(t, "0.1.0")
	t.Setenv("CLJGO_SRC", "")
	dir := t.TempDir()
	if err := SynthGoMod(dir, "", repoRoot(t), nil); err != nil {
		t.Fatalf("SynthGoMod: %v", err)
	}
	got := readGoMod(t, dir)
	if !strings.Contains(got, "replace "+runtimeModule+" => "+repoRoot(t)) {
		t.Fatalf("-runtime dir must force a replace:\n%s", got)
	}
}

// TestSynthGoModCljgoSrcBeatsRelease: CLJGO_SRC forces a replace even in a
// release binary (precedence: env second).
func TestSynthGoModCljgoSrcBeatsRelease(t *testing.T) {
	setVersion(t, "0.1.0")
	t.Setenv("CLJGO_SRC", repoRoot(t))
	dir := t.TempDir()
	if err := SynthGoMod(dir, "", "", nil); err != nil {
		t.Fatalf("SynthGoMod: %v", err)
	}
	got := readGoMod(t, dir)
	if !strings.Contains(got, "replace "+runtimeModule+" => "+repoRoot(t)) {
		t.Fatalf("CLJGO_SRC must force a replace:\n%s", got)
	}
}

// TestSynthGoModDevErrorIsHelpful: a dev binary with no locatable runtime
// tree says what it is and how to fix it.
func TestSynthGoModDevErrorIsHelpful(t *testing.T) {
	setVersion(t, "0.1.0-dev")
	t.Setenv("CLJGO_SRC", t.TempDir()) // set but not a runtime tree
	err := SynthGoMod(t.TempDir(), "", "", nil)
	if err == nil {
		t.Fatal("want an error for a bad CLJGO_SRC")
	}
	if !strings.Contains(err.Error(), "CLJGO_SRC") {
		t.Fatalf("error should mention CLJGO_SRC: %v", err)
	}
}

// TestBuildFromReleasePin is the S12 replay as a test: with the version
// stamped to the published 0.1.0 tag and no CLJGO_SRC, a module generated
// in a clean temp dir gets the bare require, `go build` fetches the runtime
// from the Go module proxy (GoBuild's ensureGoSum runs `go mod tidy`), and
// the binary runs. Requires network; skipped in -short and when offline.
func TestBuildFromReleasePin(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping proxy-backed go-build in -short mode")
	}
	setVersion(t, "0.1.0") // v0.1.0 is published; the proxy serves it (S12)
	t.Setenv("CLJGO_SRC", "")

	lang.RemoveNamespace(lang.NewSymbol("user"))
	oldOut := corelib.Out
	corelib.Out = io.Discard
	forms, err := CompileReader(strings.NewReader(`(reduce + (range 11))`), "test.clj")
	corelib.Out = oldOut
	if err != nil {
		t.Fatalf("compile: %v", err)
	}

	dir := t.TempDir()
	if err := WriteModule(dir, forms, Options{PrintLastValue: true}); err != nil {
		t.Fatalf("write module: %v", err)
	}
	got := readGoMod(t, dir)
	if !strings.Contains(got, runtimeModule+" v0.1.0") || strings.Contains(got, "replace") {
		t.Fatalf("expected a bare release pin:\n%s", got)
	}

	bin := filepath.Join(dir, "prog"+ExeSuffix)
	if err := GoBuild(dir, bin); err != nil {
		if isNetworkErr(err) {
			t.Skipf("network unavailable for the module-proxy fetch — the go.mod "+
				"shape is asserted above and by TestSynthGoModReleasePin; full "+
				"build skipped: %v", err)
		}
		if isUnpublishedRuntimePkgErr(err) {
			t.Skipf("release-pin build needs a PUBLISHED runtime that already "+
				"contains the packages emitted code imports; %v", err)
		}
		t.Fatalf("go build: %v", err)
	}
	out, err := exec.Command(bin).Output()
	if err != nil {
		t.Fatalf("run: %v", err)
	}
	if string(out) != "55\n" {
		t.Fatalf("output = %q, want %q", out, "55\n")
	}
}

// isNetworkErr heuristically detects a proxy fetch failure caused by no
// network (offline CI), mirroring pkg/build's helper of the same name.
func isNetworkErr(err error) bool {
	s := err.Error()
	for _, m := range []string{
		"dial tcp", "no such host", "network is unreachable",
		"timeout", "connection refused", "i/o timeout",
		"TLS handshake", "lookup ", "unrecognized import path",
	} {
		if strings.Contains(s, m) {
			return true
		}
	}
	return false
}

// isUnpublishedRuntimePkgErr detects the one failure a release-pin build
// hits legitimately in the window between adding a package to the runtime
// module and publishing the release that contains it: the proxy serves the
// pinned tag, and that tag has no such package.
//
// ADR 0046 put pkg/coreaot (the AOT-compiled core) into every emitted
// binary's import surface, so a build pinned to v0.1.0/v0.2.0 — tags that
// predate it — cannot resolve it. The pin SHAPE is still asserted above and
// by TestSynthGoModReleasePin; what cannot be asserted before the release
// exists is the proxy fetch. Once a release containing pkg/coreaot is
// published, bump setVersion here and in TestBuildStdlibInteropOutsideRepo
// and this skip goes away on its own.
func isUnpublishedRuntimePkgErr(err error) bool {
	s := err.Error()
	return strings.Contains(s, "does not contain package") &&
		strings.Contains(s, runtimeModule)
}
