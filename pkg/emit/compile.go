package emit

import (
	"bufio"
	"errors"
	"fmt"
	"io"
	"os"

	"github.com/muthuishere/cljgo/pkg/ast"
	"github.com/muthuishere/cljgo/pkg/eval"
	"github.com/muthuishere/cljgo/pkg/lang"
	"github.com/muthuishere/cljgo/pkg/reader"
)

// CompileFile reads, analyzes AND EVALUATES a .clj file, returning the
// analyzed top-level nodes for emission. Compile time = eval time
// (ADR 0002): each form is evaluated as it is compiled — through the
// same evaluator that expands macros during analysis — so a defmacro or
// def earlier in the file affects the analysis of later forms, exactly
// the JVM AOT model. Top-level side effects (println at top level) run
// at compile time too, as on the JVM; Load() re-runs them at binary
// startup.
func CompileFile(path string) ([]*ast.Node, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	return CompileReader(f, path)
}

// CompileReader is CompileFile over an io.Reader. It is the SINGLE-FILE
// path: a require that would load another source file refuses with an
// error naming CompileProgram (ADR 0042 §5) — silently evaluating the
// dep without capturing its forms would emit a binary missing it.
func CompileReader(r io.Reader, filename string) ([]*ast.Node, error) {
	ev := eval.New()
	ev.LibLoader = func(_ *eval.Evaluator, lib *lang.Symbol, path string) {
		panic(fmt.Errorf("namespace %s resolves to source file %s — single-file compilation cannot emit it (multi-namespace programs compile via CompileProgram / `cljgo build`)", lib.FullName(), path))
	}
	return compileStream(ev, r, filename)
}

// compileStream reads and compiles one source stream through ev under a
// pushed load frame, as repl.Driver.EvalReader: *ns* and *file* are
// thread-bound so an in-ns inside the file is undone afterwards.
func compileStream(ev *eval.Evaluator, r io.Reader, filename string) ([]*ast.Node, error) {
	lang.PushThreadBindings(lang.NewMap(
		lang.VarCurrentNS, ev.CurrentNS(),
		lang.VarFile, filename,
	))
	defer lang.PopThreadBindings()

	rd := reader.New(bufio.NewReader(r), reader.WithFilename(filename),
		reader.WithResolver(ev.ReaderResolver()))
	var nodes []*ast.Node
	for {
		form, err := rd.ReadOne()
		if errors.Is(err, reader.ErrEOF) {
			return nodes, nil
		}
		if err != nil {
			return nil, err
		}
		if nodes, err = compileForm(ev, form, nodes); err != nil {
			return nil, err
		}
	}
}

// compileForm analyzes and evaluates one top-level form, splitting a
// top-level (do ...) form-by-form first (design/03 §6 — matching
// eval.EvalForm, so earlier defs are visible to later siblings).
func compileForm(ev *eval.Evaluator, form any, nodes []*ast.Node) ([]*ast.Node, error) {
	if seq := asTopLevelDo(form); seq != nil {
		var err error
		for s := seq; s != nil; s = s.Next() {
			if nodes, err = compileForm(ev, s.First(), nodes); err != nil {
				return nodes, err
			}
		}
		return nodes, nil
	}
	n, err := ev.Analyzer().Analyze(form)
	if err != nil {
		return nodes, err
	}
	if err := evalNode(ev, n); err != nil {
		return nodes, err
	}
	return append(nodes, n), nil
}

// asTopLevelDo returns the body seq of a (do ...) form, or nil.
// (Mirrors pkg/eval's unexported helper.)
func asTopLevelDo(form any) lang.ISeq {
	seq, ok := form.(lang.ISeq)
	if !ok || lang.Seq(seq) == nil {
		return nil
	}
	sym, ok := seq.First().(*lang.Symbol)
	if !ok || sym.HasNamespace() || sym.Name() != "do" {
		return nil
	}
	return seq.Next()
}

// evalNode evaluates an analyzed node at compile time, recovering
// panics into errors (the IFn-boundary convention, design/00 §4.2).
func evalNode(ev *eval.Evaluator, n *ast.Node) (err error) {
	defer func() {
		if r := recover(); r != nil {
			if rerr, ok := r.(error); ok {
				err = rerr
				return
			}
			err = fmt.Errorf("%v", r)
		}
	}()
	_, err = ev.Eval(n, eval.NewScope())
	return err
}

// Build is the whole `cljgo build` pipeline: compile srcPath (plus any
// file-backed namespaces it requires — ADR 0042), write the generated
// module into genDir (a temp dir when empty), `go build` it to outPath.
// Returns the generated module dir actually used.
func Build(srcPath, outPath, genDir string, opts Options) (string, error) {
	prog, err := CompileProgram(srcPath)
	if err != nil {
		return "", err
	}
	if genDir == "" {
		genDir, err = os.MkdirTemp("", "cljgo-build-*")
		if err != nil {
			return "", err
		}
	} else if err := os.MkdirAll(genDir, 0o755); err != nil {
		// A user-supplied -gen dir may not exist yet; WriteModule's own
		// MkdirAll only runs after EmitMain's host-fact load (below), so a
		// missing dir must be created here first — go/packages.Load needs
		// a directory that exists (it doesn't need a go.mod, per S17).
		return "", err
	}
	// ADR 0033: host facts always resolve against the generated module,
	// never FindRuntimeDir()'s repo walk-up — stdlib resolves fine with
	// no go.mod yet (spike S17), and this is the only path a downloaded
	// release binary has for Go-interop fact loading.
	opts.HostFactsDir = genDir
	if err := WriteProgram(genDir, prog, opts); err != nil {
		return genDir, err
	}
	return genDir, GoBuild(genDir, outPath)
}

// CompileSource is CompileReader against a CALLER-SUPPLIED evaluator:
// read + analyze + evaluate + capture, with no evaluator construction
// and no LibLoader opinion. It is the seam the AOT core compiler needs
// (cmd/gencore, ADR 0046) — core's sources must compile through ONE
// evaluator whose clojure.core grows source by source, exactly as the
// interpreter's boot grows it.
func CompileSource(ev *eval.Evaluator, r io.Reader, filename string) ([]*ast.Node, error) {
	return compileStream(ev, r, filename)
}
