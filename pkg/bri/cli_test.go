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

// --- increment 2: the interactive half of the unified parameter model -------

// TestCLIEnvResolution: a missing value falls to :env before its :default,
// and a flag still wins over :env — the ADR 0078 precedence
// (flag → positional → env → prompt → default).
func TestCLIEnvResolution(t *testing.T) {
	d := newDriver(t)
	t.Setenv("TODO_PRIORITY", "4")
	eval(t, d, `(require '[bri.cli :as cli] '[bri.cli.validate :as v])
	  (cli/defcommand add "" [priority {:type :int :env "TODO_PRIORITY" :default 3
	                                     :validate [(v/min 1) (v/max 5)]}]
	    {:priority priority})
	  (cli/defcommands app {:name "todo"} add)`)
	if got := eval(t, d, `(:priority (cli/parse app ["add"]))`); got != int64(4) {
		t.Errorf("env resolution = %v, want 4 (env beats :default)", got)
	}
	if got := eval(t, d, `(:priority (cli/parse app ["add" "--priority" "2"]))`); got != int64(2) {
		t.Errorf("flag should override env = %v, want 2", got)
	}
}

// TestCLIEnvValidated: an :env value is coerced + validated through the SAME
// pipeline as a flag — a bad env value is a named error, not a silent pass.
func TestCLIEnvValidated(t *testing.T) {
	d := newDriver(t)
	t.Setenv("TODO_PRIORITY", "9") // out of the 1..5 range
	eval(t, d, `(require '[bri.cli :as cli] '[bri.cli.validate :as v])
	  (cli/defcommand add "" [priority {:type :int :env "TODO_PRIORITY"
	                                     :validate [(v/min 1) (v/max 5)]}]
	    {:priority priority})
	  (cli/defcommands app {:name "todo"} add)`)
	if msg := evalErr(t, d, `(cli/parse app ["add"])`); !strings.Contains(msg, "<= 5") {
		t.Errorf("an env value must be validated like a flag: %q", msg)
	}
}

// TestCLIPromptResolvesMissingValues: with a *prompt* backend bound (the
// test/no-TTY seam), missing values are prompted in param order; a blank
// answer with a :default takes the default; bools never prompt; a prompted
// value runs the SAME validators (an invalid answer re-prompts); and :secret
// is passed through to the backend (password path).
func TestCLIPromptResolvesMissingValues(t *testing.T) {
	d := newDriver(t)
	eval(t, d, cliPrelude)

	// script answers in param order: text, priority(blank→default), channel(blank→default)
	res := evalString(t, d, `
	  (let [q  (atom ["  hello  " "" ""])
	        pf (fn [_ _] (let [a (first @q)] (swap! q rest) a))]
	    (binding [bri.cli/*prompt* pf]
	      (let [m (cli/parse app ["add"])]
	        (str (:text m) "|" (:priority m) "|" (name (:channel m)) "|" (:done m)))))`)
	if res != "hello|3|stable|false" {
		t.Errorf("prompt resolution = %q, want hello|3|stable|false", res)
	}

	// an invalid prompted value re-prompts (first "a" fails min-len 2, "ok" passes)
	txt := evalString(t, d, `
	  (let [q  (atom ["a" "ok" "" ""])
	        pf (fn [_ _] (let [a (first @q)] (swap! q rest) a))]
	    (binding [bri.cli/*prompt* pf]
	      (:text (cli/parse app ["add"]))))`)
	if txt != "ok" {
		t.Errorf("re-prompt on invalid = %q, want ok", txt)
	}

	// :secret reaches the backend so it can turn echo off
	sec := evalString(t, d, `
	  (do
	    (cli/defcommand login "" [pass {:type :string :secret true :about "password"}] {:pass pass})
	    (cli/defcommands lapp {:name "l"} login)
	    (let [seen (atom nil)
	          pf   (fn [_ secret?] (reset! seen secret?) "hunter2")]
	      (binding [bri.cli/*prompt* pf]
	        (str (:pass (cli/parse lapp ["login"])) "|" @seen))))`)
	if sec != "hunter2|true" {
		t.Errorf("secret prompt = %q, want hunter2|true", sec)
	}
}

// --- increment 2 Part B: the native TUI core (s47 Elm loop, no Charm) --------

// TestCLIDecodeKey: raw key bytes decode to portable key events (the same
// decoder the JVM host will reuse). Arrows are 3-byte CSI; enter/esc/ctrl-c/
// backspace/tab are single bytes; a printable byte is [:rune ch].
func TestCLIDecodeKey(t *testing.T) {
	d := newDriver(t)
	eval(t, d, `(require '[bri.cli :as cli])`)
	// each case: bytes -> the pr-str of the decoded event
	cases := map[string]string{
		`[27 91 65]`: ":up",
		`[27 91 66]`: ":down",
		`[27 91 67]`: ":right",
		`[27 91 68]`: ":left",
		`[13]`:       ":enter",
		`[10]`:       ":enter",
		`[27]`:       ":esc",
		`[3]`:        ":ctrl-c",
		`[9]`:        ":tab",
		`[127]`:      ":backspace",
		`[8]`:        ":backspace",
		`[]`:         ":esc",
		`[97]`:       `[:rune \a]`,
	}
	for bytes, want := range cases {
		if got := evalString(t, d, `(pr-str (cli/decode-key `+bytes+`))`); got != want {
			t.Errorf("decode-key %s = %q, want %q", bytes, got, want)
		}
	}
}

// TestCLIRenderDiff: the renderer repaints only changed lines and clears
// trailing removed ones — the flicker-free line diff.
func TestCLIRenderDiff(t *testing.T) {
	d := newDriver(t)
	eval(t, d, `(require '[bri.cli :as cli] '[clojure.string :as s])`)

	// first paint writes every line
	full := evalString(t, d, `
	  (let [sink (atom "") w (fn [x] (swap! sink str x))]
	    (cli/render-diff [] ["alpha" "bravo"] w) @sink)`)
	if !strings.Contains(full, "alpha") || !strings.Contains(full, "bravo") {
		t.Errorf("full paint missing lines: %q", full)
	}
	// a diff paints only the changed line (unchanged "alpha" is skipped)
	diff := evalString(t, d, `
	  (let [sink (atom "") w (fn [x] (swap! sink str x))]
	    (cli/render-diff ["alpha" "bravo"] ["alpha" "charlie"] w) @sink)`)
	if !strings.Contains(diff, "charlie") || strings.Contains(diff, "alpha") {
		t.Errorf("diff should write only 'charlie', got: %q", diff)
	}
}

// TestCLISelectWidget: the whole Elm loop (drive + select-widget + renderer)
// runs against an in-memory key queue and string sink — no PTY. Cursor moves
// clamp at the ends; enter returns the index; esc cancels to -1.
func TestCLISelectWidget(t *testing.T) {
	d := newDriver(t)
	eval(t, d, `(require '[bri.cli :as cli])
	  (defn drive-keys [opts ks]
	    (let [q (atom ks)
	          rk (fn [] (let [k (first @q)] (swap! q rest) k))
	          w  (fn [_] nil)]
	      (cli/drive (cli/select-widget "Pick" opts) rk w)))`)

	if got := eval(t, d, `(drive-keys ["lib" "cli" "web"] [:down :down :enter])`); got != int64(2) {
		t.Errorf("down,down,enter = %v, want index 2", got)
	}
	if got := eval(t, d, `(drive-keys ["lib" "cli" "web"] [:enter])`); got != int64(0) {
		t.Errorf("enter at rest = %v, want index 0", got)
	}
	// up clamps at 0
	if got := eval(t, d, `(drive-keys ["lib" "cli" "web"] [:up :up :enter])`); got != int64(0) {
		t.Errorf("up clamps at 0 = %v, want 0", got)
	}
	// down clamps at the last index
	if got := eval(t, d, `(drive-keys ["lib" "cli" "web"] [:down :down :down :down :enter])`); got != int64(2) {
		t.Errorf("down clamps at last = %v, want 2", got)
	}
	// esc cancels to -1
	if got := eval(t, d, `(drive-keys ["lib" "cli" "web"] [:down :esc])`); got != int64(-1) {
		t.Errorf("esc cancels = %v, want -1", got)
	}
}

// TestCLIConfirmWidget: yes/no toggle + accept/cancel, all through the loop.
func TestCLIConfirmWidget(t *testing.T) {
	d := newDriver(t)
	eval(t, d, `(require '[bri.cli :as cli])
	  (defn confirm-keys [default ks]
	    (let [q (atom ks) rk (fn [] (let [k (first @q)] (swap! q rest) k)) w (fn [_] nil)]
	      (pr-str (cli/drive (cli/confirm-widget "OK?" default) rk w))))`)

	if got := evalString(t, d, `(confirm-keys false [:enter])`); got != "false" {
		t.Errorf("default false, enter = %q, want false", got)
	}
	if got := evalString(t, d, `(confirm-keys true [:enter])`); got != "true" {
		t.Errorf("default true, enter = %q, want true", got)
	}
	if got := evalString(t, d, `(confirm-keys false [:left :enter])`); got != "true" {
		t.Errorf("toggle then enter = %q, want true", got)
	}
	if got := evalString(t, d, `(confirm-keys false [[:rune \y] ])`); got != "true" {
		t.Errorf("y = %q, want true", got)
	}
	if got := evalString(t, d, `(confirm-keys true [[:rune \n]])`); got != "false" {
		t.Errorf("n = %q, want false", got)
	}
	if got := evalString(t, d, `(confirm-keys true [:esc])`); got != "nil" {
		t.Errorf("esc cancels = %q, want nil", got)
	}
}

// TestCLIMultiselectWidget: space toggles rows, enter returns sorted indices,
// esc cancels to nil.
func TestCLIMultiselectWidget(t *testing.T) {
	d := newDriver(t)
	eval(t, d, `(require '[bri.cli :as cli])
	  (defn multi-keys [opts ks]
	    (let [q (atom ks) rk (fn [] (let [k (first @q)] (swap! q rest) k)) w (fn [_] nil)]
	      (pr-str (cli/drive (cli/multiselect-widget "Pick" opts) rk w))))`)

	// toggle row 0 and row 2 (down,down to reach 2), accept
	if got := evalString(t, d, `(multi-keys ["a" "b" "c"] [[:rune \space] :down :down [:rune \space] :enter])`); got != "[0 2]" {
		t.Errorf("toggle 0 and 2 = %q, want [0 2]", got)
	}
	// toggle then untoggle row 0 → empty
	if got := evalString(t, d, `(multi-keys ["a" "b"] [[:rune \space] [:rune \space] :enter])`); got != "[]" {
		t.Errorf("toggle+untoggle = %q, want []", got)
	}
	// esc cancels
	if got := evalString(t, d, `(multi-keys ["a" "b"] [[:rune \space] :esc])`); got != "nil" {
		t.Errorf("esc cancels = %q, want nil", got)
	}
}

// TestCLIMultiParam: a :multi param collects a comma-separated flag (or env)
// into a vector, coercing each element by :of + :one-of and validating the
// whole vector; an unset :multi is [] (or its :default).
func TestCLIMultiParam(t *testing.T) {
	d := newDriver(t)
	eval(t, d, `(require '[bri.cli :as cli] '[bri.cli.validate :as v])
	  (cli/defcommand build "Build"
	    [tags     {:type :string :multi true :about "labels"}
	     nums     {:multi true :of :int :about "ints"}
	     features {:multi true :one-of [:a :b :c] :about "features"}]
	    {:tags tags :nums nums :features features})
	  (cli/defcommands app {:name "b"} build)`)

	// comma-separated → a vector of trimmed strings
	if got := evalString(t, d, `(pr-str (:tags (cli/parse app ["build" "--tags" "x, y ,z"])))`); got != `["x" "y" "z"]` {
		t.Errorf("multi string = %s, want [\"x\" \"y\" \"z\"]", got)
	}
	// :of coerces each element (ints)
	if got := evalString(t, d, `(pr-str (:nums (cli/parse app ["build" "--nums" "1,2,3"])))`); got != `[1 2 3]` {
		t.Errorf("multi int = %s, want [1 2 3]", got)
	}
	// :one-of coerces each to a keyword and validates membership
	if got := evalString(t, d, `(pr-str (:features (cli/parse app ["build" "--features" "a,c"])))`); got != `[:a :c]` {
		t.Errorf("multi one-of = %s, want [:a :c]", got)
	}
	// an out-of-set element is rejected, naming the value
	if msg := evalErr(t, d, `(cli/parse app ["build" "--features" "a,zzz"])`); !strings.Contains(msg, "one of") {
		t.Errorf("multi one-of bad element = %q, want it to name the allowed set", msg)
	}
	// an unset :multi is an empty vector, not nil
	if got := evalString(t, d, `(pr-str (:tags (cli/parse app ["build"])))`); got != `[]` {
		t.Errorf("unset multi = %s, want []", got)
	}
	// a whole-vector validator runs on the collected value
	eval(t, d, `(cli/defcommand pick "" [xs {:multi true :one-of [:a :b :c]
	                                          :validate (fn [v] (when (> (count v) 2) "at most 2"))}] {:xs xs})
	            (cli/defcommands papp {:name "p"} pick)`)
	if msg := evalErr(t, d, `(cli/parse papp ["pick" "--xs" "a,b,c"])`); !strings.Contains(msg, "at most 2") {
		t.Errorf("multi vector validator = %q", msg)
	}
	if got := evalString(t, d, `(pr-str (:xs (cli/parse papp ["pick" "--xs" "a,b"])))`); got != `[:a :b]` {
		t.Errorf("multi vector valid = %s, want [:a :b]", got)
	}
}

// TestCLIEditorWidget: the multi-line buffer — insert, newline, backspace
// (incl. line-join), arrow-nav — all pure index math through the loop.
func TestCLIEditorWidget(t *testing.T) {
	d := newDriver(t)
	eval(t, d, `(require '[bri.cli :as cli])
	  (defn edit-keys [ks]
	    (let [q (atom ks) rk (fn [] (let [k (first @q)] (swap! q rest) k)) w (fn [_] nil)]
	      (cli/drive (cli/editor-widget "Notes") rk w)))
	  (defn runes [s] (mapv (fn [c] [:rune c]) s))`)

	// type "hi", esc → "hi"
	if got := evalString(t, d, `(edit-keys (conj (runes "hi") :esc))`); got != "hi" {
		t.Errorf("type hi = %q, want hi", got)
	}
	// "ab" enter "cd" esc → two lines "ab\ncd"
	if got := evalString(t, d, `(edit-keys (concat (runes "ab") [:enter] (runes "cd") [:esc]))`); got != "ab\ncd" {
		t.Errorf("two lines = %q, want ab\\ncd", got)
	}
	// backspace joins lines: "ab" enter (cursor at col 0 of line 2) backspace → "ab"
	if got := evalString(t, d, `(edit-keys (concat (runes "ab") [:enter :backspace] (runes "c") [:esc]))`); got != "abc" {
		t.Errorf("backspace join = %q, want abc", got)
	}
	// mid-line insert via left-arrow: type "ac", left, insert "b" → "abc"
	if got := evalString(t, d, `(edit-keys (concat (runes "ac") [:left] (runes "b") [:esc]))`); got != "abc" {
		t.Errorf("mid-insert = %q, want abc", got)
	}
	// backspace mid-line: "abc" backspace → "ab"
	if got := evalString(t, d, `(edit-keys (concat (runes "abc") [:backspace :esc]))`); got != "ab" {
		t.Errorf("backspace = %q, want ab", got)
	}
}
