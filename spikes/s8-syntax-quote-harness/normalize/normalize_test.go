package normalize

import "testing"

func TestGensyms(t *testing.T) {
	cases := []struct {
		name string
		in   string
		want string
	}{
		{
			name: "no gensyms untouched",
			in:   "(quote user/x)",
			want: "(quote user/x)",
		},
		{
			name: "single gensym",
			in:   "(quote x__153__auto__)",
			want: "(quote x__1__auto__)",
		},
		{
			name: "same id repeated stays same",
			in:   "((quote x__154__auto__) (quote x__154__auto__))",
			want: "((quote x__1__auto__) (quote x__1__auto__))",
		},
		{
			name: "distinct ids stay distinct, numbered by first appearance",
			in:   "((quote x__155__auto__) (quote y__156__auto__))",
			want: "((quote x__1__auto__) (quote y__2__auto__))",
		},
		{
			name: "two separate syntax-quotes, same name, different ids",
			in:   "[(quote x__158__auto__) (quote x__159__auto__)]",
			want: "[(quote x__1__auto__) (quote x__2__auto__)]",
		},
		{
			name: "interleaved reuse",
			in:   "(a__9__auto__ b__12__auto__ a__9__auto__ c__3__auto__)",
			want: "(a__1__auto__ b__2__auto__ a__1__auto__ c__3__auto__)",
		},
		{
			name: "fn-shorthand arg gensyms",
			in:   "(fn* [p1__449# p2__450#] (+ p1__449# p2__450#))",
			want: "(fn* [p1__1# p2__2#] (+ p1__1# p2__2#))",
		},
		{
			name: "mixed auto and hash markers share one numbering",
			in:   "(x__7__auto__ p1__8# x__7__auto__)",
			want: "(x__1__auto__ p1__2# x__1__auto__)",
		},
		{
			name: "plain double underscore without marker untouched",
			in:   "(foo__bar x__12y)",
			want: "(foo__bar x__12y)",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := Gensyms(tc.in)
			if got != tc.want {
				t.Errorf("Gensyms(%q)\n got: %s\nwant: %s", tc.in, got, tc.want)
			}
			// idempotence: normalizing normalized output is a no-op
			if again := Gensyms(got); again != got {
				t.Errorf("not idempotent: %q -> %q", got, again)
			}
		})
	}
}
