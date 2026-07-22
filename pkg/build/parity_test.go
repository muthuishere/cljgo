package build

import (
	"bytes"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/muthuishere/cljgo/conformance"
	"github.com/muthuishere/cljgo/pkg/corelib"
	"github.com/muthuishere/cljgo/pkg/emit"
	"github.com/muthuishere/cljgo/pkg/lang"
	"github.com/muthuishere/cljgo/pkg/repl"
)

// TestParityThirdPartyGoRequire is the S31/S32 third-party go-require repro,
// run live through both legs and asserted with the ADR 0049 dec 4 parity
// comparator (conformance.ClassifyParity). The interpreter cannot link a
// third-party Go module (gorilla/websocket), so it MUST hard-error naming it
// ("not linked into the interpreter"); the AOT binary links it and prints the
// real value (close-normal code 1000). That pairing is the honest capability
// divergence — accepted outcome 3 — NOT the silent nil-vs-value quadrant that
// shipped on main (interpreter printed nil, exit 0). The AOT leg needs `go
// get`; it skips on an offline box, but the interpreter leg (the actual fix)
// always runs.
func TestParityThirdPartyGoRequire(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping go-build parity leg in -short mode")
	}
	resetUser()
	main := filepath.Join(examplesDir(t), "build-websocket", "src", "main.cljg")

	// Interpreter leg — MUST refuse with an unlinked-capability error.
	interp := parityEvalLeg(t, main)
	if !conformance.IsCapabilityError(interp.Err) {
		t.Fatalf("interpreter leg: want an unlinked-capability error, got err=%v output=%q",
			interp.Err, interp.Output)
	}

	// AOT leg — build the module in for real and run it.
	resetUser()
	bf := filepath.Join(examplesDir(t), "build-websocket", BuildFileName)
	plan, err := LoadPlan(bf)
	if err != nil {
		t.Fatalf("LoadPlan: %v", err)
	}
	bin := filepath.Join(t.TempDir(), "wsclient"+emit.ExeSuffix)
	var aot conformance.ParityLeg
	if _, berr := plan.buildArtifact(plan.Artifacts[0], bin, emit.Options{}, false); berr != nil {
		if isNetworkErr(berr) {
			t.Skipf("network unavailable for `go get` — the interpreter capability-error "+
				"(the fix) is verified above; AOT link skipped: %v", berr)
		}
		aot = conformance.ParityLeg{Err: berr}
	} else {
		out, rerr := exec.Command(bin).Output()
		aot = conformance.ParityLeg{Output: string(out), Err: rerr}
	}

	ok, reason := conformance.ClassifyParity(interp, aot)
	if !ok {
		t.Fatalf("third-party go-require parity FAILED (%s):\n--- interp ---\n%q err=%v\n--- aot ---\n%q err=%v",
			reason, interp.Output, interp.Err, aot.Output, aot.Err)
	}
	if reason != "honest capability divergence (interpreter capability-error, AOT success)" {
		t.Fatalf("unexpected accepted outcome: %s", reason)
	}
}

// parityEvalLeg runs a source file through the interpreter, capturing printed
// output (corelib.Out, where println writes) and the raised error (nil on
// success) — the interpreter half of a parity case. It does NOT fatal on
// error: an interpreter capability-error is the expected, accepted outcome.
func parityEvalLeg(t *testing.T, path string) conformance.ParityLeg {
	t.Helper()
	var buf bytes.Buffer
	oldOut := corelib.Out
	corelib.Out = &buf
	defer func() { corelib.Out = oldOut }()

	d := repl.New(nil, io.Discard, io.Discard)
	f, err := os.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	last, evErr := d.EvalReader(f, path)
	if evErr != nil {
		return conformance.ParityLeg{Output: buf.String(), Err: evErr}
	}
	return conformance.ParityLeg{Output: buf.String() + lang.PrintString(last) + "\n"}
}
