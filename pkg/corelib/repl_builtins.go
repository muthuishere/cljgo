// repl_builtins.go — the host seam behind clojure.repl/source-fn
// (core/repl.cljg, fundamentals audit 2026-07). JVM source-fn reads the
// var's defining form back out of its source file using :file/:line
// metadata; cljgo's reader annotates every top-level list with
// :line/:column/:end-line/:end-column, so the seam reads the file,
// skips to the line, reads ONE form, and slices the exact source text
// by the form's position metadata.
package corelib

import (
	"os"
	"strings"

	"github.com/muthuishere/cljgo/pkg/lang"
	"github.com/muthuishere/cljgo/pkg/reader"
)

// internReplBuiltins registers -source-fn-text: (file, line) -> the text
// of the first form starting at that line, or nil when the file is
// unreadable / the line is out of range / the form does not parse.
// Never throws — clojure.repl/source-fn's contract is nil on any miss
// (oracle JVM 1.12.5: (source-fn 'no.such/thing) => nil).
func internReplBuiltins(defPrivate func(name string, fn func(args ...any) any)) {
	defPrivate("-source-fn-text", func(args ...any) any {
		if len(args) != 2 {
			return nil
		}
		file, ok := args[0].(string)
		if !ok {
			return nil
		}
		line, ok := lang.AsInt(args[1])
		if !ok || line < 1 {
			return nil
		}
		return sourceFormText(file, line)
	})
}

// sourceFormText returns the source text of the first form beginning at
// 1-based line `line` of file, or nil.
func sourceFormText(file string, line int) any {
	data, err := os.ReadFile(file)
	if err != nil {
		return nil
	}
	lines := strings.Split(string(data), "\n")
	if line > len(lines) {
		return nil
	}
	rest := lines[line-1:]
	src := strings.Join(rest, "\n")
	form, err := reader.ReadString(src, reader.WithResolver(NSResolver()))
	if err != nil {
		return nil
	}
	im, ok := form.(lang.IMeta)
	if !ok || im.Meta() == nil {
		return nil
	}
	meta := im.Meta()
	startLine, ok1 := lang.AsInt(lang.Get(meta, lang.KWLine))
	startCol, ok2 := lang.AsInt(lang.Get(meta, lang.KWColumn))
	endLine, ok3 := lang.AsInt(lang.Get(meta, lang.KWEndLine))
	endCol, ok4 := lang.AsInt(lang.Get(meta, lang.KWEndColumn))
	if !ok1 || !ok2 || !ok3 || !ok4 {
		return nil
	}
	// Positions are 1-based relative to `src` (line 1 = the def's line);
	// end-column points one past the closing paren.
	if startLine < 1 || endLine > len(rest) || endLine < startLine {
		return nil
	}
	if startLine == endLine {
		l := rest[startLine-1]
		if endCol-1 > len(l) || startCol-1 > len(l) {
			return nil
		}
		return l[startCol-1 : endCol-1]
	}
	first := rest[startLine-1]
	if startCol-1 > len(first) {
		return nil
	}
	parts := []string{first[startCol-1:]}
	parts = append(parts, rest[startLine:endLine-1]...)
	last := rest[endLine-1]
	if endCol-1 > len(last) {
		return nil
	}
	parts = append(parts, last[:endCol-1])
	return strings.Join(parts, "\n")
}
