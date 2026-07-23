package emit

// guard_seal_test.go — spike s43 / ADR 0066 make-or-break proof.
//
// The sealed-core-guard optimization elides the per-call var deref +
// interface-compare from the arithmetic intrinsics (rt.Add2 …) while no
// sealed core arithmetic var has been redefined (lang.CoreArithDirty
// false). A (with-redefs [+ …]) alter-var-roots the core + var, which trips
// CoreArithDirty, and the intrinsic MUST then fall back to the guarded path
// and see the redefinition. This test freezes that escape hatch AND the
// dual-harness invariant (REPL == compiled, ADR 0002/0007).
//
// It is NOT a JVM-oracle conformance test, and deliberately lives here
// rather than under conformance/tests/*.clj: real Clojure 1.12.5 returns
// [7 7 7] for this program — its `:inline` on + emits Numbers.add(3,4) at
// compile time and NEVER sees the runtime redefinition (verified 2026-07-23:
// `(with-redefs [+ (fn [a b] (* a b))] (+ 3 4))` => 7, and even
// `(alter-var-root #'clojure.core/+ …)` before an inlined `(+ 3 4)` => 7).
// cljgo's intrinsic derefs at runtime, so it is strictly MORE live than the
// JVM here — a pre-existing divergence this spike PRESERVES (see the ADR's
// tradeoff section). The frozen expectation is therefore cljgo's own
// contract, enforced as eval == compiled.

import (
	"bytes"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/muthuishere/cljgo/pkg/corelib"
	"github.com/muthuishere/cljgo/pkg/lang"
	"github.com/muthuishere/cljgo/pkg/repl"
)

// redefPlusProgram: normal (+ 3 4) = 7; inside with-redefs + becomes *,
// so (+ 3 4) = 12; after the form the root is restored, so 7 again.
const redefPlusProgram = `(def normal (+ 3 4))
(def redefd (with-redefs [+ (fn [a b] (* a b))] (+ 3 4)))
(def restored (+ 3 4))
[normal redefd restored]
`

// redefPlusExpected is cljgo's contract: the middle value is 12 because the
// intrinsic falls back through the redefined root while CoreArithDirty is
// tripped. (JVM's :inline would yield [7 7 7]; see the file header.)
const redefPlusExpected = "[7 12 7]\n"

func TestSealedGuardWithRedefsEscapeHatch(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping compile-and-run in -short mode")
	}

	// --- eval (REPL) leg ---
	snap := namespaceSnapshot()
	defer removeNewNamespaces(snap)
	lang.RemoveNamespace(lang.NewSymbol("user"))
	var buf bytes.Buffer
	oldOut := corelib.Out
	corelib.Out = &buf
	d := repl.New(nil, io.Discard, io.Discard)
	last, err := d.EvalReader(strings.NewReader(redefPlusProgram), "redefplus.clj")
	corelib.Out = oldOut
	if err != nil {
		t.Fatalf("eval: %v", err)
	}
	evalOut := buf.String() + lang.PrintString(last) + "\n"
	if evalOut != redefPlusExpected {
		t.Fatalf("eval output = %q, want %q (with-redefs of + must be seen — the ADR 0066 escape hatch)", evalOut, redefPlusExpected)
	}

	// --- compiled leg ---
	lang.RemoveNamespace(lang.NewSymbol("user"))
	corelib.Out = io.Discard
	forms, err := CompileReader(strings.NewReader(redefPlusProgram), "redefplus.clj")
	corelib.Out = oldOut
	if err != nil {
		t.Fatalf("compile: %v", err)
	}
	gen := t.TempDir()
	if err := WriteModule(gen, forms, Options{PrintLastValue: true}); err != nil {
		t.Fatalf("WriteModule: %v", err)
	}
	bin := filepath.Join(gen, "redefplus"+ExeSuffix)
	if err := GoBuild(gen, bin); err != nil {
		t.Fatalf("go build: %v", err)
	}
	out, err := exec.Command(bin).Output()
	if err != nil {
		t.Fatalf("run: %v", err)
	}
	if string(out) != evalOut {
		t.Fatalf("REPL/binary divergence (release blocker, ADR 0002/0007):\n--- eval ---\n%q\n--- compiled ---\n%q", evalOut, out)
	}
	_ = os.Remove(bin)
}
