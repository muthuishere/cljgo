package diag

import (
	"testing"

	"github.com/muthuishere/cljgo/pkg/lang"
)

// TestFromErrorRuntimeCarriers pins the rendered line for the runtime error
// values enriched in ADR 0048 batch 1: compiled arity errors (lang.ArityError)
// and raise-site-coded runtime errors (lang.CodedError / the DiagCode seam).
// RenderError == Render(FromError(err)), the exact path every non-REPL
// context uses, so these assert what a user/agent actually reads.
func TestFromErrorRuntimeCarriers(t *testing.T) {
	cases := []struct {
		name string
		err  error
		want string
	}{
		{
			name: "compiled arity error names the fn and shows expected arities",
			err:  lang.NewArityError(3, "user/f", "1: [x]"),
			want: "wrong number of args (3) passed to: user/f (expects 1: [x])\n" +
				"help: run `cljgo explain A2004`",
		},
		{
			name: "compiled multi-arity error lists every arity",
			err:  lang.NewArityError(4, "user/g", "1: [x] or 2: [x y]"),
			want: "wrong number of args (4) passed to: user/g (expects 1: [x] or 2: [x y])\n" +
				"help: run `cljgo explain A2004`",
		},
		{
			name: "un-named FnFuncN arity error stays byte-stable but gains the code",
			err:  lang.NewArityError(3, "", "1"),
			want: "wrong number of arguments: expected 1, got 3\n" +
				"help: run `cljgo explain A2004`",
		},
		{
			name: "not-a-number carries G5001",
			err:  lang.NewCodedError("G5001", "cannot convert string to Ops"),
			want: "cannot convert string to Ops\n" +
				"help: run `cljgo explain G5001`",
		},
		{
			name: "not-a-function carries G5002",
			err:  lang.NewCodedError("G5002", "cannot apply non-function int64"),
			want: "cannot apply non-function int64\n" +
				"help: run `cljgo explain G5002`",
		},
		{
			name: "not-seqable carries G5003",
			err:  lang.NewCodedError("G5003", "can't convert int64 to ISeq"),
			want: "can't convert int64 to ISeq\n" +
				"help: run `cljgo explain G5003`",
		},
		{
			name: "index out of bounds carries G5004 via the typed error",
			err:  lang.NewIndexOutOfBoundsError(),
			want: "index out of bounds\n" +
				"help: run `cljgo explain G5004`",
		},
		{
			name: "not-a-collection carries G5005",
			err:  lang.NewCodedError("G5005", "conj: cannot conj onto 5"),
			want: "conj: cannot conj onto 5\n" +
				"help: run `cljgo explain G5005`",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := RenderError(tc.err); got != tc.want {
				t.Fatalf("RenderError mismatch:\n got: %q\nwant: %q", got, tc.want)
			}
		})
	}
}
