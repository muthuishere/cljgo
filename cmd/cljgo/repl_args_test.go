package main

import "testing"

// parseReplArgs is the whole surface of `cljgo repl`'s argument handling
// (ADR 0070): resume by id, resume by index, a bare id, list-on-no-id, and
// the malformed shapes that must be rejected rather than silently starting a
// fresh session.
func TestParseReplArgs(t *testing.T) {
	cases := []struct {
		name     string
		args     []string
		wantAct  replAction
		wantExit int
	}{
		{"no args → plain repl", nil, replAction{}, -1},
		{":resume <id>", []string{":resume", "abc123"}, replAction{resumeID: "abc123"}, -1},
		{":resume <#>", []string{":resume", "2"}, replAction{resumeID: "2"}, -1},
		{":resume alone lists", []string{":resume"}, replAction{list: true}, -1},
		{":sessions lists", []string{":sessions"}, replAction{list: true}, -1},
		{"bare id resumes", []string{"20260724-000437-5497"}, replAction{resumeID: "20260724-000437-5497"}, -1},
		{"two positional args is an error", []string{"a", "b"}, replAction{}, 2},
		{":resume with two ids is an error", []string{":resume", "a", "b"}, replAction{}, 2},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			act, exit := parseReplArgs(c.args)
			if act != c.wantAct || exit != c.wantExit {
				t.Errorf("parseReplArgs(%q) = (%+v, %d), want (%+v, %d)",
					c.args, act, exit, c.wantAct, c.wantExit)
			}
		})
	}
}
