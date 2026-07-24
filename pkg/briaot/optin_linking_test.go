package briaot_test

import (
	"os/exec"
	"strings"
	"testing"
)

// depsLinkAny reports whether `go list -deps mod/pkg` reaches any package
// whose import path starts with one of the given prefixes — the package-graph
// half of an opt-in namespace's zero-cost guarantee (the binary-size / symbol
// half is measured on real `cljgo build` output in the s45 spike + parity
// harness).
func depsLinkAny(t *testing.T, pkg string, prefixes ...string) bool {
	t.Helper()
	const mod = "github.com/muthuishere/cljgo/"
	out, err := exec.Command("go", "list", "-deps", mod+pkg).CombinedOutput()
	if err != nil {
		t.Fatalf("go list -deps %s: %v\n%s", pkg, err, out)
	}
	for _, dep := range strings.Split(strings.TrimSpace(string(out)), "\n") {
		for _, p := range prefixes {
			if strings.HasPrefix(dep, p) {
				return true
			}
		}
	}
	return false
}

// alwaysLinked are the pkg/briaot sub-packages every bri binary carries: the
// umbrella, the shared shims, and bri.http (what any bri web app links). No
// opt-in namespace's heavy dependency may appear in their closure.
var alwaysLinked = []string{"pkg/briaot", "pkg/bri", "pkg/briaot/brihttp"}

// TestOtelIsOptIn is ADR 0074's zero-cost proof: the OpenTelemetry SDK must
// NOT link into a bri binary that does not require tracing. The always-linked
// packages (+ the OTHER opt-in sub-package, bridb) must have ZERO
// "go.opentelemetry.io" packages in their closure; only pkg/briaot/briotel
// carries the SDK.
func TestOtelIsOptIn(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping go list -deps in -short mode")
	}
	const otel = "go.opentelemetry.io/"
	for _, pkg := range append(append([]string{}, alwaysLinked...), "pkg/briaot/bridb") {
		if depsLinkAny(t, pkg, otel) {
			t.Errorf("%s links the OpenTelemetry SDK — bri.otel is no longer zero-cost (ADR 0074); a non-tracing bri binary now carries the SDK", pkg)
		}
	}
	if !depsLinkAny(t, "pkg/briaot/briotel", otel) {
		t.Error("pkg/briaot/briotel does NOT link the OpenTelemetry SDK — the opt-in namespace cannot trace")
	}
}

// TestDbIsOptIn is ADR 0076's zero-cost proof: the SQLite + pgx drivers must
// NOT link into a bri binary that never touches a database. The always-linked
// packages (+ the OTHER opt-in sub-package, briotel) must have ZERO
// modernc.org/sqlite or jackc/pgx packages in their closure; only
// pkg/briaot/bridb carries the drivers. This closes the ADR 0072 tradeoff
// where every bri binary linked SQLite whether or not it used the database.
func TestDbIsOptIn(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping go list -deps in -short mode")
	}
	sqlite, pgx := "modernc.org/sqlite", "github.com/jackc/pgx"
	for _, pkg := range append(append([]string{}, alwaysLinked...), "pkg/briaot/briotel") {
		if depsLinkAny(t, pkg, sqlite, pgx) {
			t.Errorf("%s links the SQLite/pgx drivers — bri.db is no longer zero-cost (ADR 0076); a db-less bri binary now carries ~7 MB of drivers", pkg)
		}
	}
	if !depsLinkAny(t, "pkg/briaot/bridb", sqlite) {
		t.Error("pkg/briaot/bridb does NOT link modernc.org/sqlite — the opt-in namespace cannot reach a database")
	}
}

// TestSecretsIsOptIn is ADR 0086's zero-cost proof: the OS-keychain client
// (zalando/go-keyring + its godbus transport) must NOT link into a bri binary
// that never touches a secret store. The always-linked packages (+ the other
// opt-in sub-packages) must have ZERO go-keyring/godbus packages in their
// closure; only pkg/briaot/brisecrets carries the keychain client.
func TestSecretsIsOptIn(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping go list -deps in -short mode")
	}
	keyring, dbus := "github.com/zalando/go-keyring", "github.com/godbus/dbus"
	for _, pkg := range append(append([]string{}, alwaysLinked...), "pkg/briaot/bridb", "pkg/briaot/briotel") {
		if depsLinkAny(t, pkg, keyring, dbus) {
			t.Errorf("%s links go-keyring — bri.core.secrets is no longer zero-cost (ADR 0086); a secrets-less bri binary now carries the keychain client", pkg)
		}
	}
	if !depsLinkAny(t, "pkg/briaot/brisecrets", keyring) {
		t.Error("pkg/briaot/brisecrets does NOT link go-keyring — the opt-in namespace cannot reach a secret store")
	}
}
