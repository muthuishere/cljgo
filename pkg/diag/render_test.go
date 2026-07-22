package diag

import "testing"

func TestRenderDegradesAndEnriches(t *testing.T) {
	cases := []struct {
		name string
		d    Diagnostic
		want string
	}{
		{
			name: "bare message only",
			d:    Diagnostic{Message: "boom"},
			want: "boom",
		},
		{
			name: "expects only, no found (count already in message)",
			d: Diagnostic{
				Message:    "wrong number of args (3) passed to: user/f",
				Expected:   "1: [x]",
				ErrorCode:  "A2004",
				Location:   Location{File: "demo.clj", Line: 4, Column: 1},
				ExplainURL: ExplainURL("A2004"),
			},
			want: "wrong number of args (3) passed to: user/f (expects 1: [x]) at demo.clj:4:1\n" +
				"help: run `cljgo explain A2004`",
		},
		{
			name: "expected and found",
			d: Diagnostic{
				Message:  "type mismatch",
				Expected: "Number",
				Found:    "String",
			},
			want: "type mismatch (expects Number, got String)",
		},
		{
			name: "did-you-mean fix + explain",
			d: Diagnostic{
				Message:   "unable to resolve symbol: pritnln in this context",
				ErrorCode: "A2001",
				Location:  Location{File: "demo.clj", Line: 1, Column: 1},
				Fixes:     []Fix{{Title: "did you mean println?", Replacement: "println"}},
			},
			want: "unable to resolve symbol: pritnln in this context at demo.clj:1:1\n" +
				"help: did you mean println?\n" +
				"help: run `cljgo explain A2001`",
		},
		{
			name: "reader error with related note",
			d: Diagnostic{
				Message:   "EOF while reading",
				ErrorCode: "R1001",
				Location:  Location{File: "demo.clj", Line: 3, Column: 5},
				Related:   []Related{{Message: "form starts here", Location: Location{File: "demo.clj", Line: 1, Column: 1}}},
			},
			want: "EOF while reading at demo.clj:3:5\n" +
				"note: form starts here at demo.clj:1:1\n" +
				"help: run `cljgo explain R1001`",
		},
		{
			name: "unregistered code prints no explain pointer",
			d:    Diagnostic{Message: "mystery", ErrorCode: "Z9999"},
			want: "mystery",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := Render(tc.d); got != tc.want {
				t.Fatalf("Render mismatch:\n got: %q\nwant: %q", got, tc.want)
			}
		})
	}
}
