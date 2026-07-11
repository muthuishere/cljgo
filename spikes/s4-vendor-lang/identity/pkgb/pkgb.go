// Package pkgb simulates a second, independently-compiled emitted
// package interning the same keyword literals as pkga. Its package-level
// vars must be Go-== identical to pkga's.
package pkgb

import "cljgo-spike-s4/lang"

var (
	KwFoo    = lang.InternKeyword("", "foo")
	KwNsBar  = lang.InternKeyword("my.ns", "bar")
	SymBaz   = lang.NewSymbol("baz")
	KwStatus = lang.NewKeyword("status")
)
