package build

import (
	"strings"
	"testing"
)

// ADR 0052 decision 4: a consumer's own go-requires must be conflict-checked
// even with no (dep …) declarations, so a self-conflict (same module pinned at
// two versions in one build.cljgo) hard-errors here rather than being silently
// MVS-collapsed by `go mod tidy`. This covers the no-deps branch of
// buildArtifact via its helper.
func TestMergeOwnGoRequiresSelfConflictErrors(t *testing.T) {
	reqs := []GoRequire{
		{Path: "github.com/google/go-cmp", Version: "v0.6.0"},
		{Path: "github.com/google/go-cmp", Version: "v0.7.0"},
	}
	if _, err := mergeOwnGoRequires(reqs, nil); err == nil {
		t.Fatal("expected a conflict error for the same module at two versions, got nil")
	} else if !strings.Contains(err.Error(), "go-cmp") ||
		!strings.Contains(err.Error(), "v0.6.0") || !strings.Contains(err.Error(), "v0.7.0") {
		t.Fatalf("conflict error should name the module and both versions, got: %v", err)
	}
}

func TestMergeOwnGoRequiresAcceptOverrideResolves(t *testing.T) {
	reqs := []GoRequire{
		{Path: "github.com/google/go-cmp", Version: "v0.6.0"},
		{Path: "github.com/google/go-cmp", Version: "v0.7.0"},
	}
	accept := map[string]string{"github.com/google/go-cmp": "v0.7.0"}
	out, err := mergeOwnGoRequires(reqs, accept)
	if err != nil {
		t.Fatalf("accept override should resolve the conflict, got: %v", err)
	}
	if len(out) != 1 || out[0].Version != "v0.7.0" {
		t.Fatalf("want single go-cmp@v0.7.0, got %+v", out)
	}
}

func TestMergeOwnGoRequiresNoConflictPassesThrough(t *testing.T) {
	reqs := []GoRequire{
		{Path: "github.com/a/one", Version: "v1.0.0"},
		{Path: "github.com/b/two", Version: "v2.0.0"},
	}
	out, err := mergeOwnGoRequires(reqs, nil)
	if err != nil {
		t.Fatalf("no conflict expected, got: %v", err)
	}
	if len(out) != 2 {
		t.Fatalf("want both requires, got %+v", out)
	}
}
