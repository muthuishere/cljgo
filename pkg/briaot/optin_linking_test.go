package briaot_test

import (
	"os/exec"
	"strings"
	"testing"
)

// TestOptInNamespaceIsNotInUmbrella is ADR 0074's zero-cost proof: bri.otel
// is OPT-IN because its OpenTelemetry SDK must NOT link into a bri binary
// that does not require tracing. The mechanism is a separately-linked
// sub-package (pkg/briaot/briotel) that the emitter blank-imports only when
// the app requires bri.otel — so the umbrella pkg/briaot, the shared shims
// pkg/bri, and every always-linked sub-package (here brihttp, what any bri
// web app links) must have ZERO OpenTelemetry packages in their dependency
// closure, while the opt-in sub-package briotel must carry the SDK.
//
// This is the package-graph half of the guarantee; the binary-size / string
// half is measured on real `cljgo build` output (a bri.http app has 0
// "go.opentelemetry.io" strings; a bri.otel app has thousands, ~6 MB more).
func TestOptInNamespaceIsNotInUmbrella(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping go list -deps in -short mode")
	}
	const otel = "go.opentelemetry.io/"
	const mod = "github.com/muthuishere/cljgo/"

	links := func(pkg string) bool {
		out, err := exec.Command("go", "list", "-deps", mod+pkg).CombinedOutput()
		if err != nil {
			t.Fatalf("go list -deps %s: %v\n%s", pkg, err, out)
		}
		for _, dep := range strings.Split(strings.TrimSpace(string(out)), "\n") {
			if strings.HasPrefix(dep, otel) {
				return true
			}
		}
		return false
	}

	// Always-linked packages every bri binary carries: none may reach otel.
	for _, pkg := range []string{"pkg/briaot", "pkg/bri", "pkg/briaot/brihttp", "pkg/briaot/bridb"} {
		if links(pkg) {
			t.Errorf("%s links the OpenTelemetry SDK — bri.otel is no longer zero-cost (ADR 0074); a non-tracing bri binary now carries the SDK", pkg)
		}
	}

	// The opt-in sub-package MUST carry the SDK (else it could not export).
	if !links("pkg/briaot/briotel") {
		t.Error("pkg/briaot/briotel does NOT link the OpenTelemetry SDK — the opt-in namespace cannot trace")
	}
}
