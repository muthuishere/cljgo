package ast

import "testing"

func TestOpString(t *testing.T) {
	// Every v0 op has a name (keeps the enum and opNames in sync).
	for op := OpConst; op <= OpQuote; op++ {
		if _, ok := opNames[op]; !ok {
			t.Errorf("op %d has no name", op)
		}
	}
	if OpIf.String() != "if" {
		t.Errorf("OpIf.String() = %q", OpIf.String())
	}
	if Op(0).String() != "Op(0)" {
		t.Errorf("unknown op string = %q", Op(0).String())
	}
}

func TestBindKindString(t *testing.T) {
	for k, want := range map[BindKind]string{BindLet: "let", BindArg: "arg", BindFn: "fn"} {
		if k.String() != want {
			t.Errorf("%d.String() = %q, want %q", k, k.String(), want)
		}
	}
	if BindKind(0).String() != "BindKind(0)" {
		t.Errorf("unknown kind string = %q", BindKind(0).String())
	}
}
