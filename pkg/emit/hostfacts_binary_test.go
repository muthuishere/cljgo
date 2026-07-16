package emit

// ADR 0033 coverage (spike S17): Go-interop host facts resolve from the
// generated module directory, not a checked-out cljgo source tree — the
// gap that broke `cljgo build` on a downloaded binary with no go-require
// in play.

import (
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/muthuishere/cljgo/pkg/eval"
	"github.com/muthuishere/cljgo/pkg/lang"
)

const stdlibInteropSrc = "(require-go '[strings])\n(strings/ToUpper \"hi\")"

// TestHostFactsNoNetworkForStdlib proves directly (S17) that resolving
// stdlib host facts against a freshly created, go.mod-less generated
// module directory touches no network: with GOPROXY=off, `go/packages`
// still resolves `strings.ToUpper` from GOROOT. This isolates the
// fact-loading step from the unrelated, genuinely network-dependent step
// (fetching the release-pinned runtime module itself, covered by
// TestBuildFromReleasePin) so this assertion can't pass by accident.
func TestHostFactsNoNetworkForStdlib(t *testing.T) {
	t.Setenv("GOPROXY", "off")

	lang.RemoveNamespace(lang.NewSymbol("user"))
	oldOut := eval.Out
	eval.Out = io.Discard
	forms, err := CompileReader(strings.NewReader(stdlibInteropSrc), "test.clj")
	eval.Out = oldOut
	if err != nil {
		t.Fatalf("compile: %v", err)
	}
	if len(collectHostPaths(forms)) == 0 {
		t.Fatal("fixture must reference a host path (strings) to exercise fact loading")
	}

	// A fresh, empty dir — no go.mod — exactly the S17 "completely empty
	// dir" case, and what compile.go's Build now always points
	// HostFactsDir at before any go.mod exists.
	dir := t.TempDir()
	if _, _, err := EmitMain(forms, Options{HostFactsDir: dir}); err != nil {
		t.Fatalf("EmitMain with GOPROXY=off should resolve stdlib facts with no network: %v", err)
	}
}

// TestBuildStdlibInteropOutsideRepo is the S17 end-to-end regression: a
// (require-go '[strings])-only program (no go-require, so no third-party
// module) builds via compile.go's Build from a directory outside the
// repo, with CLJGO_SRC unset and the process cwd moved away from the
// repo too — so FindRuntimeDir()'s walk-up (both from cwd and from the
// test binary's own location) cannot locate a cljgo checkout. Before ADR
// 0033, Build never set opts.HostFactsDir, so the host-fact load fell
// through to that same FindRuntimeDir() and failed with "cannot locate
// the ... source tree" (note: running this from inside the repo's own
// test process would NOT have reproduced the bug, since FindRuntimeDir's
// walk-up would accidentally succeed from cwd — hence the Chdir below).
// After the fix, opts.HostFactsDir = genDir makes stdlib resolve
// regardless. The release-pinned runtime module itself still needs the
// Go module proxy to `go build` (a separate, pre-existing dependency —
// ADR 0028/S12) — skipped here exactly as TestBuildFromReleasePin does.
func TestBuildStdlibInteropOutsideRepo(t *testing.T) {
	setVersion(t, "0.1.0") // release shape: no walk-up replace is available
	t.Setenv("CLJGO_SRC", "")

	lang.RemoveNamespace(lang.NewSymbol("user"))

	origWD, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	outsideDir := t.TempDir()
	if err := os.Chdir(outsideDir); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chdir(origWD) })

	srcPath := filepath.Join(outsideDir, "test.clj")
	if err := os.WriteFile(srcPath, []byte(stdlibInteropSrc), 0o644); err != nil {
		t.Fatal(err)
	}
	genDir := filepath.Join(outsideDir, "gen")
	bin := filepath.Join(outsideDir, "prog"+ExeSuffix)

	if _, err := Build(srcPath, bin, genDir, Options{PrintLastValue: true}); err != nil {
		if isNetworkErr(err) {
			t.Skipf("network unavailable for the runtime-module proxy fetch (unrelated to host-fact resolution, proven by TestHostFactsNoNetworkForStdlib): %v", err)
		}
		t.Fatalf("Build: %v", err)
	}

	got, err := os.ReadFile(bin)
	if err != nil || len(got) == 0 {
		t.Fatalf("expected a built binary at %s: %v", bin, err)
	}
}
