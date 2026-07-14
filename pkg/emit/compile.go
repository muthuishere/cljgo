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

// CompileReader is CompileFile over an io.Reader.
func CompileReader(r io.Reader, filename string) ([]*ast.Node, error) {
	ev := eval.New()
	// The load frame, as repl.Driver.EvalReader: *ns* and *file* are
	// thread-bound so an in-ns inside the file is undone afterwards.
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

// Build is the whole `cljgo build` pipeline: compile srcPath, write the
// generated module into genDir (a temp dir when empty), `go build` it
// to outPath. Returns the generated module dir actually used.
func Build(srcPath, outPath, genDir string, opts Options) (string, error) {
	forms, err := CompileFile(srcPath)
	if err != nil {
		return "", err
	}
	if genDir == "" {
		genDir, err = os.MkdirTemp("", "cljgo-build-*")
		if err != nil {
			return "", err
		}
	}
	if err := WriteModule(genDir, forms, opts); err != nil {
		return genDir, err
	}
	return genDir, GoBuild(genDir, outPath)
}
