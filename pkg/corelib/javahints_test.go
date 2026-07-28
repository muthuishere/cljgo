package corelib

import (
	"strings"
	"testing"

	"github.com/muthuishere/cljgo/pkg/diag"
	"github.com/muthuishere/cljgo/pkg/lang"
)

// TestJavaStaticDidYouMean — a Java static (`Thread/sleep`, `System/nanoTime`)
// still fails LOUD (ADR 0054 decision 4) with the byte-stable
// "no such namespace: X" message conformance freezes, but the rendered line now
// carries the registered A2009 code and a did-you-mean Fix naming the cljgo
// replacement (CLAUDE.md error doctrine: name the thing, suggestions are Fixes).
func TestJavaStaticDidYouMean(t *testing.T) {
	for _, tc := range []struct{ sym, want string }{
		{"Thread/sleep", "cljg.system/sleep"},
		{"System/nanoTime", "cljg.date/nano-time"},
		{"System/currentTimeMillis", "cljg.date/now"},
		{"System/getenv", "cljg.system/getenv"},
		{"System/exit", "cljg.system/exit"},
	} {
		_, err := ResolveVar(lang.NewSymbol(tc.sym))
		if err == nil {
			t.Fatalf("%s must not resolve (cljgo has no Java host)", tc.sym)
		}
		ns := tc.sym[:strings.Index(tc.sym, "/")]
		if got, want := err.Error(), "no such namespace: "+ns; got != want {
			t.Errorf("%s: message must stay byte-stable: got %q, want %q", tc.sym, got, want)
		}
		d := diag.FromError(err)
		if d.ErrorCode != "A2009" {
			t.Errorf("%s: want code A2009, got %q", tc.sym, d.ErrorCode)
		}
		rendered := diag.Render(d)
		if !strings.Contains(rendered, tc.want) {
			t.Errorf("%s: rendered line must suggest %s:\n%s", tc.sym, tc.want, rendered)
		}
		if !strings.Contains(rendered, "help:") {
			t.Errorf("%s: suggestion must render as a help: line:\n%s", tc.sym, rendered)
		}
	}
}

// TestUnknownJavaStaticNotesTheHost — a Java static with no cljgo twin gets the
// generic "cljgo runs on Go" note rather than a wrong did-you-mean.
func TestUnknownJavaStaticNotesTheHost(t *testing.T) {
	_, err := ResolveVar(lang.NewSymbol("System/lineSeparator"))
	if err == nil {
		t.Fatal("System/lineSeparator must not resolve")
	}
	rendered := diag.Render(diag.FromError(err))
	if !strings.Contains(rendered, "no Java host") {
		t.Errorf("want the generic host note, got:\n%s", rendered)
	}
	if strings.Contains(rendered, "did you mean") {
		t.Errorf("must not invent a replacement, got:\n%s", rendered)
	}
}

// TestNoSuchNamespaceKeepsItsLocation — the enriched diagnostic is raised
// without a position; the analyzer supplies one by wrapping the error in a
// lang.CompilerError. The carrier must not trade its locus for its fixes.
func TestNoSuchNamespaceKeepsItsLocation(t *testing.T) {
	_, err := ResolveVar(lang.NewSymbol("System/nanoTime"))
	if err == nil {
		t.Fatal("System/nanoTime must not resolve")
	}
	wrapped := lang.NewCompilerError("demo.clj", 7, 3, err)
	d := diag.FromError(wrapped)
	if d.Location.File != "demo.clj" || d.Location.Line != 7 || d.Location.Column != 3 {
		t.Fatalf("location lost: %+v", d.Location)
	}
	if got := diag.Render(d); !strings.Contains(got, "at demo.clj:7:3") {
		t.Errorf("rendered line must carry the locus:\n%s", got)
	}
}

// TestPlainMissingNamespaceIsCoded — an ordinary missing require (not a Java
// class) is coded A2009 too, with no suggestion invented.
func TestPlainMissingNamespaceIsCoded(t *testing.T) {
	_, err := ResolveVar(lang.NewSymbol("nope.zzz/thing"))
	if err == nil {
		t.Fatal("nope.zzz/thing must not resolve")
	}
	d := diag.FromError(err)
	if d.ErrorCode != "A2009" {
		t.Errorf("want A2009, got %q", d.ErrorCode)
	}
	if len(d.Fixes) != 0 {
		t.Errorf("no suggestion should be invented: %+v", d.Fixes)
	}
}
