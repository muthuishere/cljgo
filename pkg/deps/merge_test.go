package deps

import (
	"strings"
	"testing"
)

func TestMergeGoRequiresFlatten(t *testing.T) {
	sets := [][]GoReq{
		{{Path: "github.com/google/uuid", Version: "v1.6.0"}},
		{{Path: "github.com/google/go-cmp", Version: "v0.6.0"}},
		{{Path: "github.com/google/uuid", Version: "v1.6.0"}}, // dup, same version: fine
	}
	out, err := MergeGoRequires(sets, nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(out) != 2 {
		t.Fatalf("expected 2 merged requires, got %d: %+v", len(out), out)
	}
	// path-sorted
	if out[0].Path != "github.com/google/go-cmp" || out[1].Path != "github.com/google/uuid" {
		t.Fatalf("not path-sorted: %+v", out)
	}
}

func TestMergeGoRequiresConflict(t *testing.T) {
	sets := [][]GoReq{
		{{Path: "github.com/google/go-cmp", Version: "v0.6.0"}},
		{{Path: "github.com/google/go-cmp", Version: "v0.7.0"}},
	}
	_, err := MergeGoRequires(sets, nil)
	if err == nil {
		t.Fatal("expected a conflict error, got nil")
	}
	msg := err.Error()
	for _, want := range []string{"go-cmp", "v0.6.0", "v0.7.0"} {
		if !strings.Contains(msg, want) {
			t.Fatalf("conflict error must name %q; got: %s", want, msg)
		}
	}
	if strings.Contains(msg, "MVS") == false {
		// not required, but the message should make clear no silent pick happens
		t.Logf("conflict message: %s", msg)
	}
}

func TestMergeGoRequiresAcceptResolves(t *testing.T) {
	sets := [][]GoReq{
		{{Path: "github.com/google/go-cmp", Version: "v0.6.0"}},
		{{Path: "github.com/google/go-cmp", Version: "v0.7.0"}},
	}
	out, err := MergeGoRequires(sets, map[string]string{"github.com/google/go-cmp": "v0.7.0"})
	if err != nil {
		t.Fatalf("accept override should resolve the conflict: %v", err)
	}
	if len(out) != 1 || out[0].Version != "v0.7.0" {
		t.Fatalf("expected accepted v0.7.0, got %+v", out)
	}
}

func TestMergeProvenanceNamesRequirers(t *testing.T) {
	in := []provGoReq{
		{GoReq: GoReq{Path: "m", Version: "v1"}, From: "depA"},
		{GoReq: GoReq{Path: "m", Version: "v2"}, From: "depB"},
	}
	_, err := mergeGoReqProv(in, nil)
	if err == nil {
		t.Fatal("expected conflict")
	}
	for _, want := range []string{"depA", "depB", "v1", "v2"} {
		if !strings.Contains(err.Error(), want) {
			t.Fatalf("provenance error must name %q: %s", want, err.Error())
		}
	}
}
