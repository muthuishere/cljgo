// Package pkga simulates one separately-compiled emitted package that
// hoists keyword literals to package-level vars, exactly as the cljgo
// emitter will (design doc 00 §4.4).
package pkga

import "cljgo-spike-s4/lang"

var (
	KwFoo    = lang.InternKeyword("", "foo")
	KwNsBar  = lang.InternKeyword("my.ns", "bar")
	SymBaz   = lang.NewSymbol("baz")
	KwStatus = lang.InternKeywordString("status")
)
