// cli_test.go — the behavior suite for bri.cli + bri.cli.validate (ADR
// 0078). Like the rest of bri these behaviors have NO JVM oracle (bri.cli
// does not exist in Clojure 1.12.5), so they run against the real
// interpreter here rather than in conformance/tests. The dual-mode
// (interpreted vs AOT-compiled) parity is covered end-to-end by a
// cmd/cljgo build test; here we exercise the deterministic core — the
// unified parameter model: parse → coerce → trim → validate → dispatch,
// plus help/version/did-you-mean.
package bri_test

import (
	"strings"
	"testing"

	"github.com/muthuishere/cljgo/pkg/repl"
)

// evalErr evaluates code expecting an error, returning its message.
func evalErr(t *testing.T, d *repl.Driver, code string) string {
	t.Helper()
	_, err := d.EvalString(code, "cli_test")
	if err == nil {
		t.Fatalf("eval %q: expected an error, got none", code)
	}
	return err.Error()
}

// cliPrelude defines a representative app: an `add` command with a trimmed
// string, a defaulted+range-validated int, an enum, and a bool; plus a `ls`.
const cliPrelude = `
(require '[bri.cli :as cli] '[bri.cli.validate :as v])
(cli/defcommand add "Add an item"
  [text     {:type :string :about "item text" :validate [(v/non-empty) (v/min-len 2)]}
   priority {:type :int    :about "1-5" :default 3 :validate [(v/min 1) (v/max 5)]}
   channel  {:type :enum   :one-of [:stable :beta] :default :stable}
   done     {:type :bool   :about "mark done"}]
  {:text text :priority priority :channel channel :done done})
(cli/defcommand ls "List items" [all {:type :bool}] {:all all})
(cli/defcommands app {:name "todo" :version "1.0" :about "a tiny todo"} add ls)
`

// TestCLIParseResolvesAndTrims: flags, positionals, defaults, and the
// default string trim all flow from one declaration.
func TestCLIParseResolvesAndTrims(t *testing.T) {
	d := newDriver(t)
	eval(t, d, cliPrelude)

	// a flag value, trimmed by default
	if got := evalString(t, d, `(:text (cli/parse app ["add" "--text" "  hello  "]))`); got != "hello" {
		t.Errorf("trimmed flag = %q, want %q", got, "hello")
	}
	// a bare positional binds to the first param
	if got := evalString(t, d, `(:text (cli/parse app ["add" "buy-milk"]))`); got != "buy-milk" {
		t.Errorf("positional = %q, want buy-milk", got)
	}
	// an unset param falls to its :default
	if got := eval(t, d, `(:priority (cli/parse app ["add" "ok"]))`); got != int64(3) {
		t.Errorf("default priority = %v, want 3", got)
	}
	// :trim false preserves whitespace verbatim
	eval(t, d, `(cli/defcommand raw "" [s {:type :string :trim false}] {:s s})
                (cli/defcommands app2 {:name "x"} raw)`)
	if got := evalString(t, d, `(:s (cli/parse app2 ["raw" "--s" "  keep  "]))`); got != "  keep  " {
		t.Errorf(":trim false = %q, want '  keep  '", got)
	}
}

// TestCLICoercion: :type drives parsing uniformly.
func TestCLICoercion(t *testing.T) {
	d := newDriver(t)
	eval(t, d, cliPrelude)
	if got := eval(t, d, `(:priority (cli/parse app ["add" "ok" "--priority" "4"]))`); got != int64(4) {
		t.Errorf("int coercion = %v, want 4", got)
	}
	if got := eval(t, d, `(:done (cli/parse app ["add" "ok" "--done"]))`); got != true {
		t.Errorf("bool flag present = %v, want true", got)
	}
	if got := eval(t, d, `(:done (cli/parse app ["add" "ok"]))`); got != false {
		t.Errorf("bool flag absent = %v, want false", got)
	}
	if got := eval(t, d, `(:channel (cli/parse app ["add" "ok" "--channel" "beta"]))`); got != eval(t, d, `:beta`) {
		t.Errorf("enum coercion = %v, want :beta", got)
	}
	// an out-of-set enum is rejected with the allowed values named
	if msg := evalErr(t, d, `(cli/parse app ["add" "ok" "--channel" "gamma"])`); !strings.Contains(msg, "one of") {
		t.Errorf("bad enum error = %q, want it to name the allowed values", msg)
	}
	// a non-numeric int is a named error, not a crash
	if msg := evalErr(t, d, `(cli/parse app ["add" "ok" "--priority" "high"])`); !strings.Contains(msg, "int") {
		t.Errorf("bad int error = %q", msg)
	}
}

// TestCLIValidatorsGuardBoth(the ADR 0078 §3 win): validators run in the
// resolution pipeline, so a bad value is rejected with the validator's own
// message — proving custom fns, vector-AND, and composed built-ins all fire.
func TestCLIValidators(t *testing.T) {
	d := newDriver(t)
	eval(t, d, cliPrelude)

	// vector = AND, first failure wins (min then max)
	if msg := evalErr(t, d, `(cli/parse app ["add" "ok" "--priority" "0"])`); !strings.Contains(msg, ">= 1") {
		t.Errorf("min validator = %q", msg)
	}
	if msg := evalErr(t, d, `(cli/parse app ["add" "ok" "--priority" "9"])`); !strings.Contains(msg, "<= 5") {
		t.Errorf("max validator = %q", msg)
	}
	// non-empty + min-len on the text param
	if msg := evalErr(t, d, `(cli/parse app ["add" "a"])`); !strings.Contains(msg, "at least 2") {
		t.Errorf("min-len validator = %q", msg)
	}
	// a valid value passes clean
	if got := eval(t, d, `(:priority (cli/parse app ["add" "ok" "--priority" "5"]))`); got != int64(5) {
		t.Errorf("valid value = %v, want 5", got)
	}

	// custom validator = any fn value->nil|message
	eval(t, d, `(cli/defcommand serve "" [port {:type :int :validate (fn [p] (when (< p 1024) "use a non-privileged port"))}] {:port port})
                (cli/defcommands capp {:name "c"} serve)`)
	if msg := evalErr(t, d, `(cli/parse capp ["serve" "--port" "80"])`); !strings.Contains(msg, "non-privileged") {
		t.Errorf("custom validator = %q", msg)
	}

	// composed + built-in validators: v/all, v/matches, v/email, v/one-of
	eval(t, d, `(def slug (v/all (v/non-empty) (v/max-len 5) (v/matches #"^[a-z]+$")))
                (cli/defcommand mk "" [name {:type :string :validate slug}
                                       addr {:type :string :validate (v/email)}]
                   {:name name :addr addr})
                (cli/defcommands mapp {:name "m"} mk)`)
	if msg := evalErr(t, d, `(cli/parse mapp ["mk" "--name" "Toolong" "--addr" "a@b.co"])`); !strings.Contains(msg, "at most 5") {
		t.Errorf("composed v/all (max-len) = %q", msg)
	}
	if msg := evalErr(t, d, `(cli/parse mapp ["mk" "--name" "ok" "--addr" "nope"])`); !strings.Contains(msg, "match") {
		t.Errorf("v/email = %q", msg)
	}
	if got := evalString(t, d, `(:name (cli/parse mapp ["mk" "--name" "ok" "--addr" "a@b.co"]))`); got != "ok" {
		t.Errorf("all-valid composed = %q", got)
	}
}

// TestCLIRequiredAndUnknown: a missing required arg and an unknown command
// are named errors from parse (never a hang or a crash).
func TestCLIRequiredAndUnknown(t *testing.T) {
	d := newDriver(t)
	eval(t, d, `(require '[bri.cli :as cli])
              (cli/defcommand need "" [x {:type :string :required true :about "the x"}] {:x x})
              (cli/defcommands app {:name "t"} need)`)
	if msg := evalErr(t, d, `(cli/parse app ["need"])`); !strings.Contains(msg, "missing required") || !strings.Contains(msg, "the x") {
		t.Errorf("missing-required error = %q, want it to name the arg + its about", msg)
	}
	if msg := evalErr(t, d, `(cli/parse app ["nope"])`); !strings.Contains(msg, "unknown command") {
		t.Errorf("unknown-command error = %q", msg)
	}
}

// TestCLIRunDispatchesAndRenders: the full run path — dispatch to the
// handler, and the generated help / version / did-you-mean output — all
// captured via with-out-str (run prints to *out*/*err*).
func TestCLIRunDispatchesAndRenders(t *testing.T) {
	d := newDriver(t)
	eval(t, d, `(require '[bri.cli :as cli])
              (def hits (atom nil))
              (cli/defcommand add "Add an item" [text {:type :string}] (reset! hits text))
              (cli/defcommand ls "List items" [] (reset! hits :listed))
              (cli/defcommands app {:name "todo" :version "1.0"} add ls)`)

	// dispatch: run invokes the matched command's handler with resolved args
	eval(t, d, `(cli/run app ["add" "hello"])`)
	if got := evalString(t, d, `@hits`); got != "hello" {
		t.Errorf("run did not dispatch: hits=%q", got)
	}

	// generated help lists the commands
	help := evalString(t, d, `(with-out-str (cli/run app []))`)
	for _, want := range []string{"todo 1.0", "commands:", "add", "Add an item", "ls"} {
		if !strings.Contains(help, want) {
			t.Errorf("help missing %q:\n%s", want, help)
		}
	}
	// --version
	if v := evalString(t, d, `(with-out-str (cli/run app ["--version"]))`); !strings.Contains(v, "todo 1.0") {
		t.Errorf("--version = %q", v)
	}
	// per-command help
	if ch := evalString(t, d, `(with-out-str (cli/run app ["add" "--help"]))`); !strings.Contains(ch, "--text") {
		t.Errorf("command help missing the param: %q", ch)
	}
	// unknown command → did-you-mean, on *err*
	dym := evalString(t, d, `(with-out-str (binding [*err* *out*] (cli/run app ["addd"])))`)
	if !strings.Contains(dym, "did you mean") || !strings.Contains(dym, "add") {
		t.Errorf("did-you-mean missing: %q", dym)
	}
	// a failed validation inside run is caught and reported, not thrown
	eval(t, d, `(cli/defcommand v1 "" [n {:type :int :validate (fn [x] (when (> x 5) "too big"))}] (reset! hits n))
              (cli/defcommands vapp {:name "v"} v1)
              (reset! hits :untouched)`)
	verr := evalString(t, d, `(with-out-str (binding [*err* *out*] (cli/run vapp ["v1" "--n" "9"])))`)
	if !strings.Contains(verr, "too big") {
		t.Errorf("run should report the validation error: %q", verr)
	}
	if got := eval(t, d, `@hits`); got != eval(t, d, `:untouched`) {
		t.Errorf("a failed validation must not dispatch the handler; hits=%v", got)
	}
}
