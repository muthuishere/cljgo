// secretsparity_test.go — the bri.core.secrets dual-mode parity gate (ADR 0086).
// testdata/secretsparity.cljg drives the masked-secret surface over the env
// provider; this runs it BOTH interpreted (`cljgo run`) and AOT-compiled
// (`cljgo build`) and asserts byte-identical output. A REPL↔binary divergence
// is the release blocker (CLAUDE.md). It is also the proof a bri.core.secrets app
// LINKS opt-in and compiles CGO_ENABLED=0 (go-keyring is pure Go — ADR 0086),
// and that the plaintext never leaks into a printed surface.
package main

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/muthuishere/cljgo/pkg/emit"
)

func TestBriSecretsParity(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping binary build in -short mode")
	}
	bin := buildCljgo(t)
	src, err := filepath.Abs(filepath.Join("testdata", "secretsparity.cljg"))
	if err != nil {
		t.Fatal(err)
	}
	withSecret := append(os.Environ(), "BRI_PARITY_SECRET=alpha")

	// Interpreted: `cljgo run`.
	runCmd := exec.Command(bin, "run", src)
	runCmd.Env = withSecret
	interp, err := runCmd.Output()
	if err != nil {
		t.Fatalf("cljgo run: %v", err)
	}

	// Compiled: `cljgo build` → a static binary running the same forms.
	out := filepath.Join(t.TempDir(), "secretsparity"+emit.ExeSuffix)
	build := exec.Command(bin, "build", "-o", out, src)
	build.Env = append(os.Environ(), "CLJGO_SRC="+repoRoot(t))
	if b, err := build.CombinedOutput(); err != nil {
		t.Fatalf("cljgo build (bri.core.secrets app): %v\n%s", err, b)
	}
	runBin := exec.Command(out)
	runBin.Env = withSecret
	compiled, err := runBin.Output()
	if err != nil {
		t.Fatalf("running the compiled bri.core.secrets binary: %v", err)
	}

	if string(interp) != string(compiled) {
		t.Fatalf("bri.core.secrets REPL↔binary divergence (release blocker):\n--- interpreted ---\n%s\n--- compiled ---\n%s",
			interp, compiled)
	}

	want := "masked len=5 ***…ha\nrevealed alpha\nchain alpha\nmiss nil\nsecret? true\n"
	if string(compiled) != want {
		t.Fatalf("bri.core.secrets parity transcript =\n%q\nwant\n%q", compiled, want)
	}
	// the masked line must not carry the plaintext (only the last-2 tail)
	if strings.Contains(strings.SplitN(string(compiled), "\n", 2)[0], "alpha") {
		t.Fatalf("SECRET LEAKED in masked output: %q", compiled)
	}
}
