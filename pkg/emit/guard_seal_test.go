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

// redefPlusSealedExpected is the OPT-IN hard seal's contract (ADR 0066
// alternative 1, `cljgo build --seal-core`): with the guard elided the
// site IS the int64 op, so the with-redefs is not observed and the answer
// is [7 7 7] — byte-for-byte what real Clojure 1.12.5 prints for this
// program (its :inline on + compiles the site to Numbers.add; verified
// 2026-07-23, see the file header). The sealed binary is therefore MORE
// JVM-conformant than the default one, at the documented cost of the
// redefinition not being seen at that site.
const redefPlusSealedExpected = "[7 7 7]\n"

// TestSealCoreHardSealElidesGuard freezes BOTH modes of the flag:
//
//   - SealCore false (the default): identical to the guarded emission —
//     the call sites still carry the operator var and the rt.CoreDirty
//     check, and the program still answers [7 12 7].
//   - SealCore true: no var reference, no rt.CoreDirty load, no rt.Add2
//     family at all — and the program answers [7 7 7].
//
// The [7 12 7] → [7 7 7] difference is the WHOLE observable cost of the
// flag, deliberately frozen here so it can never change by accident. Under
// --seal-core the compiled leg intentionally diverges from the REPL (which
// is always fully live); that divergence is the opt-in's entire point and
// moves the binary TOWARD the JVM oracle, not away from it.
func TestSealCoreHardSealElidesGuard(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping compile-and-run in -short mode")
	}
	oldOut := corelib.Out
	snap := namespaceSnapshot()
	defer removeNewNamespaces(snap)

	build := func(opts Options) (src string, output string) {
		t.Helper()
		lang.RemoveNamespace(lang.NewSymbol("user"))
		corelib.Out = io.Discard
		forms, err := CompileReader(strings.NewReader(redefPlusProgram), "redefplus.clj")
		corelib.Out = oldOut
		if err != nil {
			t.Fatalf("compile: %v", err)
		}
		gen := t.TempDir()
		opts.PrintLastValue = true
		if err := WriteModule(gen, forms, opts); err != nil {
			t.Fatalf("WriteModule: %v", err)
		}
		b, err := os.ReadFile(filepath.Join(gen, "main.go"))
		if err != nil {
			t.Fatalf("read generated main.go: %v", err)
		}
		bin := filepath.Join(gen, "redefplus"+ExeSuffix)
		if err := GoBuild(gen, bin); err != nil {
			t.Fatalf("go build: %v", err)
		}
		out, err := exec.Command(bin).Output()
		if err != nil {
			t.Fatalf("run: %v", err)
		}
		_ = os.Remove(bin)
		return string(b), string(out)
	}

	offSrc, offOut := build(Options{})
	if offOut != redefPlusExpected {
		t.Fatalf("--seal-core OFF output = %q, want %q (the default MUST keep today's liveness)", offOut, redefPlusExpected)
	}
	if !strings.Contains(offSrc, "rt.Add2(") {
		t.Fatalf("--seal-core OFF: expected the guarded rt.Add2(v, x, y) emission, got:\n%s", offSrc)
	}
	if strings.Contains(offSrc, "rt.Add2S(") {
		t.Fatalf("--seal-core OFF: sealed helper leaked into the default emission:\n%s", offSrc)
	}

	onSrc, onOut := build(Options{SealCore: true})
	if onOut != redefPlusSealedExpected {
		t.Fatalf("--seal-core ON output = %q, want %q (hard seal must NOT observe the with-redefs)", onOut, redefPlusSealedExpected)
	}
	if !strings.Contains(onSrc, "rt.Add2S(") {
		t.Fatalf("--seal-core ON: expected the sealed rt.Add2S(x, y) emission, got:\n%s", onSrc)
	}
	for _, forbidden := range []string{"rt.Add2(", "rt.CoreDirty()"} {
		if strings.Contains(onSrc, forbidden) {
			t.Fatalf("--seal-core ON: %q still present — the guard was not elided:\n%s", forbidden, onSrc)
		}
	}
}

// TestSealCoreEnvSeam covers the CLJGO_SEAL_CORE=1 environment seam, the
// path project builds / the conformance harness / benchmark scripts use
// when no --seal-core flag is plumbed through.
func TestSealCoreEnvSeam(t *testing.T) {
	if (Options{}).sealCore() {
		t.Fatal("Options{}.sealCore() must default to false")
	}
	t.Setenv("CLJGO_SEAL_CORE", "1")
	if !(Options{}).sealCore() {
		t.Fatal("CLJGO_SEAL_CORE=1 must turn the hard seal on")
	}
	t.Setenv("CLJGO_SEAL_CORE", "0")
	if (Options{}).sealCore() {
		t.Fatal("CLJGO_SEAL_CORE=0 must leave the hard seal off")
	}
	if !(Options{SealCore: true}).sealCore() {
		t.Fatal("explicit SealCore must win over an unset/0 env")
	}
}
