package diag

import (
	"os"
	"testing"
)

// TestRegistryLockMatches keeps docs/diagnostics/registry.lock in step with
// the append-only registry (registry.go's own instructions), and — with
// -run TestRegistryLockMatches -args -write — rewrites it.
func TestRegistryLockMatches(t *testing.T) {
	if os.Getenv("CLJGO_WRITE_REGISTRY_LOCK") != "" {
		if err := os.WriteFile("../../docs/diagnostics/registry.lock", []byte(LockText()), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	got, err := LockFile()
	if err != nil {
		t.Fatal(err)
	}
	if got != LockText() {
		t.Fatalf("docs/diagnostics/registry.lock is stale:\n--- have ---\n%s\n--- want ---\n%s", got, LockText())
	}
}

// TestEveryCodeHasAnExplainPage is the other half of the registry contract:
// a registered code must be explainable (`cljgo explain <CODE>`).
func TestEveryCodeHasAnExplainPage(t *testing.T) {
	for _, c := range Codes() {
		if _, err := Explain(c); err != nil {
			t.Errorf("%s: %v", c, err)
		}
	}
}
