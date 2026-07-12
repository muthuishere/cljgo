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
	"os"
	"os/signal"
	"strings"
	"sync"
	"sync/atomic"

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

	// interrupted is set by Interrupt (SIGINT or a frontend op) and
	// consumed by Run's loop: the pending unfinished input is discarded.
	interrupted atomic.Bool
	// outMu serializes writes to out/errOut between Run's goroutine and
	// Interrupt (which runs on a signal goroutine).
	outMu sync.Mutex
	// promptNS is the namespace name last shown in the prompt; Interrupt
	// reads it (under outMu) because the session's *ns* binding is only
	// visible on Run's goroutine.
	promptNS string
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
// before the unfinished form is evaluated, so a form may span lines and
// one line may hold many forms. Each form is evaluated and printed AS
// IT COMPLETES: a syntax error later in the buffer never discards the
// result (or error) of a form that already closed. Reader syntax errors
// and eval errors are printed (with position when available) and the
// loop continues; only input exhaustion or an I/O error ends it.
//
// Interrupts: Run listens for SIGINT for its whole lifetime. Ctrl-C
// discards the pending unfinished input (the "  #_=> " continuation)
// and redraws a fresh prompt; it NEVER exits the session — like JVM
// Clojure's REPL under rlwrap (`clj`). The session ends on Ctrl-D
// (EOF at the prompt), exactly as clojure.main does.
func (d *Driver) Run() error {
	// The session frame (design/03 §7b): *ns* and the result/error vars
	// are thread-bound for the session's goroutine; in-ns and the per-eval
	// set!s below mutate the bindings, and everything reverts on exit.
	lang.PushThreadBindings(lang.NewMap(
		lang.VarCurrentNS, d.ev.CurrentNS(),
		d.v1, nil, d.v2, nil, d.v3, nil, d.ve, nil,
	))
	defer lang.PopThreadBindings()

	// SIGINT → Interrupt for the duration of the session (terminal
	// frontend). A future nREPL frontend calls Interrupt directly.
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, os.Interrupt)
	done := make(chan struct{})
	defer func() { signal.Stop(sigCh); close(done) }()
	go func() {
		for {
			select {
			case <-sigCh:
				d.Interrupt()
			case <-done:
				return
			}
		}
	}()

	sc := bufio.NewScanner(d.in)
	sc.Buffer(make([]byte, 0, 64*1024), 1<<20)
	pending := ""
	for {
		if d.Prompts {
			d.printPrompt(pending == "")
		}
		if !sc.Scan() {
			break
		}
		if d.interrupted.Swap(false) {
			pending = "" // Interrupt already redrew the prompt
		}
		pending += sc.Text() + "\n"
		pending = d.dispatch(pending, false)
	}
	if err := sc.Err(); err != nil {
		return err
	}
	if d.interrupted.Swap(false) {
		pending = ""
	}
	// Input ended with an unfinished form: report it as the positioned
	// reader error it is (atEOF forces ErrIncomplete to be an error).
	if strings.TrimSpace(pending) != "" {
		d.dispatch(pending, true)
	}
	if d.Prompts {
		d.outMu.Lock()
		fmt.Fprintln(d.out)
		d.outMu.Unlock()
	}
	return nil
}

// printPrompt writes the primary (current-namespace) or continuation
// prompt, remembering the namespace name for Interrupt's redraw.
func (d *Driver) printPrompt(primary bool) {
	d.outMu.Lock()
	defer d.outMu.Unlock()
	if primary {
		// The prompt names the CURRENT namespace (in-ns moves it).
		d.promptNS = d.ev.CurrentNS().Name().Name()
		fmt.Fprintf(d.out, "%s=> ", d.promptNS)
	} else {
		fmt.Fprint(d.out, "  #_=> ")
	}
}

// Interrupt aborts the input continuation in progress: the pending
// unfinished form is discarded and a fresh primary prompt is drawn.
// The session itself keeps running — at an empty prompt an interrupt
// is just a newline + new prompt. Safe to call from any goroutine
// (Run's SIGINT listener, a future nREPL interrupt op).
func (d *Driver) Interrupt() {
	d.interrupted.Store(true)
	d.outMu.Lock()
	defer d.outMu.Unlock()
	fmt.Fprintln(d.out)
	if d.Prompts {
		// promptNS, not CurrentNS(): the session's *ns* binding is only
		// visible on Run's goroutine.
		fmt.Fprintf(d.out, "%s=> ", d.promptNS)
	}
}

// dispatch reads forms from src, evaluating and printing each one AS IT
// COMPLETES, and returns the unconsumed rest of the buffer. If src ends
// mid-form and !atEOF, the rest is that incomplete tail — the caller
// appends more input to it; everything already evaluated is trimmed so
// it can never run twice. A syntax error is reported with its position
// and consumes the whole buffer (rest ""), but only AFTER the forms
// completed before it have been evaluated and printed.
func (d *Driver) dispatch(src string, atEOF bool) (rest string) {
	cs := &countingScanner{rs: strings.NewReader(src)}
	r := reader.New(cs, reader.WithFilename("REPL"),
		reader.WithResolver(d.ev.ReaderResolver()))
	consumed := 0
	for {
		form, err := r.ReadOne()
		if errors.Is(err, reader.ErrEOF) {
			return ""
		}
		if errors.Is(err, reader.ErrIncomplete) && !atEOF {
			return src[consumed:]
		}
		if err != nil {
			d.reportError(err)
			return ""
		}
		consumed = cs.off
		d.evalAndPrint(form)
	}
}

// countingScanner tracks the byte offset consumed from the underlying
// scanner so dispatch can trim evaluated forms off the pending buffer.
// One rune of pushback suffices: the reader never Unreads twice in a
// row (pkg/reader's scanner panics if it does).
type countingScanner struct {
	rs       io.RuneScanner
	off      int
	lastSize int
}

func (c *countingScanner) ReadRune() (r rune, size int, err error) {
	r, size, err = c.rs.ReadRune()
	if err == nil {
		c.off += size
		c.lastSize = size
	}
	return r, size, err
}

func (c *countingScanner) UnreadRune() error {
	err := c.rs.UnreadRune()
	if err == nil {
		c.off -= c.lastSize
	}
	return err
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
	s := lang.PrintString(res) // may panic — recovered above into *e
	d.outMu.Lock()
	fmt.Fprintln(d.out, s)
	d.outMu.Unlock()
}

func (d *Driver) reportError(err error) {
	// Reader and analyzer errors carry file:line:col in their message.
	d.outMu.Lock()
	fmt.Fprintf(d.errOut, "error: %v\n", err)
	d.outMu.Unlock()
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

	rd := reader.New(bufio.NewReader(r), reader.WithFilename(filename),
		reader.WithResolver(d.ev.ReaderResolver()))
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
