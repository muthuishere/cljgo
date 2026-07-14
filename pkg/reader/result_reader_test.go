package reader

import (
	"testing"

	"github.com/muthuishere/cljgo/pkg/lang"
)

// Reader support for the Result/Option tagged literals (ADR 0014 D4):
// #cljgo/ok, #cljgo/err, #cljgo/just read into the pkg/lang tagged
// values and round-trip through PrintString.

func TestReadResultTaggedLiterals(t *testing.T) {
	cases := []struct {
		src  string
		pred func(any) bool
		want string // pr-str round-trip
	}{
		{"#cljgo/ok 5", lang.IsOk, "#cljgo/ok 5"},
		{"#cljgo/err :bad", lang.IsErr, "#cljgo/err :bad"},
		{"#cljgo/just 9", lang.IsJust, "#cljgo/just 9"},
		{`#cljgo/ok "hi"`, lang.IsOk, `#cljgo/ok "hi"`},
		{"#cljgo/none nil", lang.IsNone, "none"},
	}
	for _, c := range cases {
		v := mustRead(t, c.src)
		if !c.pred(v) {
			t.Errorf("read %q: predicate failed, got %T", c.src, v)
		}
		if got := lang.PrintString(v); got != c.want {
			t.Errorf("read %q: round-trip pr-str = %q, want %q", c.src, got, c.want)
		}
	}
}

func TestReadUnknownTag(t *testing.T) {
	if _, err := readOne(t, "#cljgo/nope 1"); err == nil {
		t.Fatal("unknown #cljgo/... tag should error")
	}
}
