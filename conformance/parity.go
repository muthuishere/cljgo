// parity.go — the ADR 0049 decision 4 dual-mode host-resolution parity
// comparator, extending the ADR 0007 dual harness (conformance_test.go +
// compiled_test.go). The two-outcome rule the plain dual harness enforces
// ("identical output OR the file is eval-only") is too strict for a
// declared host-capability gap: the correct fix for the unlinked
// third-party require-go divergence makes `cljgo run` hard-error while the
// AOT binary succeeds with the real value (S36). This comparator therefore
// accepts THREE outcomes and forbids only the silent-divergence quadrant.
package conformance

import "strings"

// ParityLeg is one execution leg's result: its captured stdout and the
// error it raised (nil = success).
type ParityLeg struct {
	Output string
	Err    error
}

// capabilityMarkers are the substrings that identify an HONEST interpreter
// capability-gap error (ADR 0049): the interpreter genuinely cannot satisfy
// a reference the AOT binary can. Only such an error may pair with an AOT
// success under accepted outcome 3 — any other interpreter error paired
// with AOT success is a real divergence.
var capabilityMarkers = []string{
	"not linked into the interpreter",            // unlinked third-party require-go (dec 2)
	"was not compiled into this binary",          // uncompiled require in a binary (dec 3)
	"is not available in an AOT-compiled binary", // analyzer-only op in a binary (ADR 0046)
}

// IsCapabilityError reports whether err names an unavailable host capability
// (the only interpreter error permitted to pair with an AOT success).
func IsCapabilityError(err error) bool {
	if err == nil {
		return false
	}
	msg := err.Error()
	for _, m := range capabilityMarkers {
		if strings.Contains(msg, m) {
			return true
		}
	}
	return false
}

// ClassifyParity applies the ADR 0049 decision 4 three-outcome rule to a
// parity case run under the interpreter leg and the AOT leg. It returns
// ok=true for exactly one of the three ACCEPTED outcomes:
//
//  1. identical output — both legs succeed with byte-identical stdout;
//  2. identical error — both legs refuse (each raised an error);
//  3. honest capability divergence — the interpreter hard-errors naming an
//     unavailable host capability AND the AOT leg succeeds.
//
// It returns ok=false with a reason for the FORBIDDEN quadrant: different
// non-error values, or one leg silently yielding nil/""/false/a no-op while
// the other produces a real value (which manifests as differing output with
// no error), or an interpreter error that is NOT a declared capability gap
// paired with an AOT success.
func ClassifyParity(interp, aot ParityLeg) (ok bool, reason string) {
	switch {
	case interp.Err == nil && aot.Err == nil:
		if interp.Output == aot.Output {
			return true, "identical output"
		}
		return false, "different non-error values (silent divergence): " +
			"interpreter=" + quote(interp.Output) + " aot=" + quote(aot.Output)

	case interp.Err != nil && aot.Err != nil:
		// Both legs refuse — an honest, symmetric failure.
		return true, "identical error (both legs refuse)"

	case interp.Err != nil && aot.Err == nil:
		// The interpreter refused while the AOT leg produced a value. This is
		// accepted ONLY when the interpreter error names an unavailable host
		// capability (outcome 3); any other interpreter error here is a real
		// divergence.
		if IsCapabilityError(interp.Err) {
			return true, "honest capability divergence (interpreter capability-error, AOT success)"
		}
		return false, "interpreter errored (non-capability) but AOT succeeded: " + interp.Err.Error()

	default: // interp.Err == nil && aot.Err != nil
		// The interpreter silently succeeded while the AOT leg failed — the
		// binary cannot reproduce the interpreted value. Forbidden.
		return false, "AOT errored but interpreter silently succeeded: " + aot.Err.Error()
	}
}

// quote renders a captured output for a failure message, trimming a
// trailing newline so short outputs read cleanly.
func quote(s string) string {
	return "\"" + strings.TrimRight(s, "\n") + "\""
}
