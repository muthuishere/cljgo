package format14

import "math/big"

// clojureStringer stands in for any cljgo value whose %s rendering is its
// own .String()/pr-str-ish text (e.g. a keyword `:kw`) but which is NOT a Go
// string and must not be treated as one by numeric conversions.
type clojureStringer string

func (c clojureStringer) String() string { return string(c) }

func bigIntFromString(s string) *big.Int {
	n, ok := new(big.Int).SetString(s, 10)
	if !ok {
		panic("bad bigint literal: " + s)
	}
	return n
}

func ratioFromString(s string) *big.Rat {
	r, ok := new(big.Rat).SetString(s)
	if !ok {
		panic("bad ratio literal: " + s)
	}
	return r
}
