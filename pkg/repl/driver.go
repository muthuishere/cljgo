// Package repl is the REPL driver of design/03-analyzer-eval.md §7b: a
// loop of Read (pkg/reader) → Analyze+Eval (pkg/eval) → bind *1 *2 *3
// (and *e on error) → print via pr-str, over an injected reader/writer
// pair. The terminal frontend (cmd/cljgo) and the future nREPL server
// are both thin frontends of this one driver.
package repl

import (
	"bufio"
	"errors"
	"fmt"
	"io"
	"strings"

	"github.com/muthuishere/cljgo/pkg/eval"
	"github.com/muthuishere/cljgo/pkg/lang"
	"github.com/muthuishere/cljgo/pkg/reader"
)

// Driver owns one evaluator session. *1 *2 *3 *e are proper dynamic
// vars in clojure.core (design/03 §7b): Run pushes a session frame
// binding them (plus *ns*) and set!s them after each eval; they revert
// to their nil roots when the session ends, as on JVM Clojure.
type Driver struct {
	// Prompts controls whether Run writes a prompt to Out before each
	// line of input (on for a terminal, off for piped input).
	Prompts bool

	ev             *eval.Evaluator
	in             io.Reader
	out            io.Writer // results and prompts
	errOut         io.Writer // error reports
	v1, v2, v3, ve *lang.Var
}

// New returns a driver with a fresh evaluator. in may be nil when only
// EvalReader/EvalString will be used (e.g. `cljgo run`).
func New(in io.Reader, out, errOut io.Writer) *Driver {
	ev := eval.New() // interns the core builtins incl. *1 *2 *3 *e
	d := &Driver{ev: ev, in: in, out: out, errOut: errOut}
	find := func(name string) *lang.Var {
		return lang.NSCore.FindInternedVar(lang.NewSymbol(name))
	}
	d.v1, d.v2, d.v3, d.ve = find("*1"), find("*2"), find("*3"), find("*e")
	return d
}

// Evaluator exposes the session's evaluator (tests, future nREPL ops).
func (d *Driver) Evaluator() *eval.Evaluator { return d.ev }

// Run is the interactive loop. Input is accumulated line by line; when
// the buffer ends mid-form (reader.ErrIncomplete) more input is read
// before anything is evaluated, so a form may span lines and one line
// may hold many forms. Reader syntax errors and eval errors are printed
// (with position when available) and the loop continues; only input
// exhaustion or an I/O error ends it.
func (d *Driver) Run() error {
	// The session frame (design/03 §7b): *ns* and the result/error vars
	// are thread-bound for the session's goroutine; in-ns and the per-eval
	// set!s below mutate the bindings, and everything reverts on exit.
	lang.PushThreadBindings(lang.NewMap(
		lang.VarCurrentNS, d.ev.CurrentNS(),
		d.v1, nil, d.v2, nil, d.v3, nil, d.ve, nil,
	))
	defer lang.PopThreadBindings()

	sc := bufio.NewScanner(d.in)
	sc.Buffer(make([]byte, 0, 64*1024), 1<<20)
	var pending strings.Builder
	for {
		if d.Prompts {
			if pending.Len() == 0 {
				// The prompt names the CURRENT namespace (in-ns moves it).
				fmt.Fprintf(d.out, "%s=> ", d.ev.CurrentNS().Name().Name())
			} else {
				fmt.Fprint(d.out, "  #_=> ")
			}
		}
		if !sc.Scan() {
			break
		}
		pending.WriteString(sc.Text())
		pending.WriteString("\n")
		if d.dispatch(pending.String(), false) {
			pending.Reset()
		}
	}
	if err := sc.Err(); err != nil {
		return err
	}
	// Input ended with an unfinished form: report it as the positioned
	// reader error it is (atEOF forces ErrIncomplete to be an error).
	if strings.TrimSpace(pending.String()) != "" {
		d.dispatch(pending.String(), true)
	}
	if d.Prompts {
		fmt.Fprintln(d.out)
	}
	return nil
}

// dispatch reads every form in src and, if none is incomplete,
// evaluates and prints them in order, returning true (buffer consumed).
// If src ends mid-form and !atEOF it returns false so the caller reads
// more input before evaluating anything. A syntax error consumes the
// buffer: it is reported with its position and dispatch returns true.
func (d *Driver) dispatch(src string, atEOF bool) (consumed bool) {
	r := reader.New(strings.NewReader(src), reader.WithFilename("REPL"))
	var forms []any
	for {
		form, err := r.ReadOne()
		if errors.Is(err, reader.ErrEOF) {
			break
		}
		if errors.Is(err, reader.ErrIncomplete) && !atEOF {
			return false
		}
		if err != nil {
			d.reportError(err)
			return true
		}
		forms = append(forms, form)
	}
	for _, f := range forms {
		d.evalAndPrint(f)
	}
	return true
}

// evalAndPrint runs one top-level form through Analyze+Eval, set!s the
// session bindings of *1 *2 *3 on success (results shift) or *e on
// error, and prints the result with pr-str. EvalForm already recovers
// evaluator panics into errors; the deferred recover here additionally
// guards the driver's own seams (e.g. printing) so a panic never kills
// the loop. Only called under Run's session frame — Var.Set needs it.
func (d *Driver) evalAndPrint(form any) {
	defer func() {
		if r := recover(); r != nil {
			err, ok := r.(error)
			if !ok {
				err = fmt.Errorf("%v", r)
			}
			d.ve.Set(err)
			d.reportError(err)
		}
	}()
	res, err := d.ev.EvalForm(form)
	if err != nil {
		d.ve.Set(err)
		d.reportError(err)
		return
	}
	d.v3.Set(d.v2.Deref())
	d.v2.Set(d.v1.Deref())
	d.v1.Set(res)
	fmt.Fprintln(d.out, lang.PrintString(res))
}

func (d *Driver) reportError(err error) {
	// Reader and analyzer errors carry file:line:col in their message.
	fmt.Fprintf(d.errOut, "error: %v\n", err)
}

// EvalReader reads and evaluates every form from r (e.g. a .clj file),
// returning the value of the last form. No REPL affordances: results
// are not printed and *1/*e are not bound; errors return immediately
// with position. *ns* and *file* are bound for the load, as Clojure's
// load does (design/03 §7a) — an in-ns inside the file is undone when
// the load finishes. This is the `cljgo run` and conformance path.
func (d *Driver) EvalReader(r io.Reader, filename string) (any, error) {
	lang.PushThreadBindings(lang.NewMap(
		lang.VarCurrentNS, d.ev.CurrentNS(),
		lang.VarFile, filename,
	))
	defer lang.PopThreadBindings()

	rd := reader.New(bufio.NewReader(r), reader.WithFilename(filename))
	var last any
	for {
		form, err := rd.ReadOne()
		if errors.Is(err, reader.ErrEOF) {
			return last, nil
		}
		if err != nil {
			return nil, err
		}
		last, err = d.ev.EvalForm(form)
		if err != nil {
			return nil, err
		}
	}
}

// EvalString is EvalReader over a string (tests, future nREPL eval op).
func (d *Driver) EvalString(src, filename string) (any, error) {
	return d.EvalReader(strings.NewReader(src), filename)
}
