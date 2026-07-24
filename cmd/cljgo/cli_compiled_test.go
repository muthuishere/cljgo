// cli_compiled_test.go — the dual-mode parity gate for bri.cli (ADR 0078).
// bri.cli's behavior suite (pkg/bri/cli_test.go) runs interpreted; this
// proves the AOT-compiled binary behaves BYTE-IDENTICALLY (the unforgivable
// failure mode is a REPL-vs-binary divergence, CLAUDE.md). It builds a real
// bri.cli app through the cljgo binary and drives it as a user would.
package main

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

const briCliApp = `(require '[bri.cli :as cli] '[bri.cli.validate :as v])
(cli/defcommand add "Add an item"
  [text     {:type :string :about "item text" :validate [(v/non-empty) (v/min-len 2)]}
   priority {:type :int    :about "1-5" :default 3 :validate [(v/min 1) (v/max 5)]}]
  (println "added" text "priority" priority))
(cli/defcommand ls "List items" [all {:type :bool :about "include done"}]
  (println "listing all=" all))
(cli/defcommands app {:name "todo" :version "1.0" :about "a tiny todo"} add ls)
(defn -main [& args] (cli/run app args))
`

func TestBriCliCompiledParity(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping binary build in -short mode")
	}
	bin := buildCljgo(t)
	dir := t.TempDir()
	src := filepath.Join(dir, "todo.clj")
	if err := os.WriteFile(src, []byte(briCliApp), 0o644); err != nil {
		t.Fatal(err)
	}
	app := filepath.Join(dir, "todo")
	build := exec.Command(bin, "build", "-o", app, src)
	build.Env = append(os.Environ(), "CLJGO_SRC="+repoRoot(t), "CGO_ENABLED=0")
	if out, err := build.CombinedOutput(); err != nil {
		t.Fatalf("cljgo build (bri.cli app): %v\n%s", err, out)
	}

	run := func(args ...string) string {
		out, _ := exec.Command(app, args...).CombinedOutput() // errors go to stderr, captured
		return strings.TrimSpace(string(out))
	}

	// Each expectation is exactly what the interpreter produces (pkg/bri
	// cli_test.go), so a mismatch here IS a REPL-vs-binary divergence.
	cases := []struct {
		name string
		args []string
		want []string // substrings that must appear
	}{
		{"help", nil, []string{"todo 1.0", "commands:", "add", "ls"}},
		{"version", []string{"--version"}, []string{"todo 1.0"}},
		{"dispatch+coerce", []string{"add", "--text", "hello", "--priority", "4"}, []string{"added hello priority 4"}},
		{"default+trim", []string{"add", "  buymilk  "}, []string{"added buymilk priority 3"}},
		{"bool", []string{"ls", "--all"}, []string{"listing all= true"}},
		{"validate", []string{"add", "x"}, []string{"must be at least 2"}},
		{"range-validate", []string{"add", "ok", "--priority", "9"}, []string{"must be <= 5"}},
		{"bad-int", []string{"add", "ok", "--priority", "high"}, []string{"expects an int"}},
		{"command-help", []string{"add", "--help"}, []string{"--text", "--priority"}},
		{"did-you-mean", []string{"addd"}, []string{"did you mean", "add"}},
	}
	for _, c := range cases {
		got := run(c.args...)
		for _, w := range c.want {
			if !strings.Contains(got, w) {
				t.Errorf("%s: compiled output missing %q\ngot: %s", c.name, w, got)
			}
		}
	}
}
