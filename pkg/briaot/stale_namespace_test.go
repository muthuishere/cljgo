package briaot_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// ADR 0102 moved bri.core.{cache,jobs,secrets,data} to cljg.* and made its
// own acceptance conditional on "a grep-gate confirms no stale
// bri.core.{cache,jobs,secrets,data} reference survives". This is that gate.
//
// It exists because the rename's real failure mode is not a build error —
// these are Clojure namespace names in source strings, docstrings and docs,
// so a stale one compiles fine and simply tells a reader to require a
// namespace that no longer exists. That is the same
// document-promises-what-code-does-not-do defect ADR 0124 was opened for.
func TestNoStaleBriCoreNamespaceReferences(t *testing.T) {
	// The four namespaces ADR 0102 moved. bri.core.config, bri.core.audit
	// and bri.core.telemetry legitimately remain under bri.core.
	stale := []string{
		"bri.core.cache",
		"bri.core.jobs",
		"bri.core.secrets",
		"bri.core.data",
	}

	repo, err := filepath.Abs("../..")
	if err != nil {
		t.Fatalf("resolve repo root: %v", err)
	}

	// Scanned: live source, ACTIVE openspec changes, and the docs a user is
	// pointed at. Excluded as immutable historical record: docs/adr (ADR 0102
	// must be free to name what it moved), openspec/changes/archive (what was
	// true when it was applied), site/…/releases.md (released notes), plus
	// spikes (frozen), refs/ (vendored clones), and this file.
	skipDir := map[string]bool{
		".git": true, ".claude": true, "refs": true, "spikes": true,
		"node_modules": true, "dist": true, ".build": true,
	}

	var offenders []string
	err = filepath.Walk(repo, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return nil // unreadable paths are not this gate's business
		}
		rel, _ := filepath.Rel(repo, path)
		if info.IsDir() {
			if skipDir[info.Name()] || strings.HasPrefix(rel, "docs/adr") ||
				strings.HasPrefix(rel, filepath.Join("openspec", "changes", "archive")) {
				return filepath.SkipDir
			}
			return nil
		}
		switch filepath.Ext(path) {
		case ".cljg", ".clj", ".cljc", ".go", ".md", ".mdx", ".edn":
		default:
			return nil
		}
		if strings.HasSuffix(rel, "stale_namespace_test.go") ||
			rel == filepath.Join("site", "src", "content", "docs", "reference", "releases.md") {
			return nil
		}
		b, err := os.ReadFile(path)
		if err != nil {
			return nil
		}
		for _, ns := range stale {
			if strings.Contains(string(b), ns) {
				offenders = append(offenders, rel+" mentions "+ns)
			}
		}
		return nil
	})
	if err != nil {
		t.Fatalf("walk: %v", err)
	}

	if len(offenders) > 0 {
		t.Errorf("stale pre-ADR-0102 namespace references (%d) — these namespaces are now cljg.*:\n  %s",
			len(offenders), strings.Join(offenders, "\n  "))
	}
}
