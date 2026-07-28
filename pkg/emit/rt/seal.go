package rt

import "github.com/muthuishere/cljgo/pkg/lang"

// Hard-sealed arithmetic intrinsics — ADR 0066 alternative 1, OPT-IN only.
//
// These are the `Add2`/`LTBool`/… bodies with the liveness machinery
// removed ENTIRELY: no operator var parameter, no lang.CoreArithDirty
// load, no interface-compare, no lang.Apply2 fallback. The emitter emits
// calls to them ONLY under the opt-in `cljgo build --seal-core` flag
// (emit.Options.SealCore / CLJGO_SEAL_CORE=1); the default build keeps
// emitting the guarded rt.Add2(v, x, y) family and its full ADR 0004
// liveness, byte-for-byte as before.
//
// WHAT SEALING COSTS. At a sealed call site a redefinition of the
// operator — `(def + …)`, `(alter-var-root #'+ …)`, `(with-redefs [+ …]
// …)` — is NEVER observed: the site IS the int64 op. That is exactly what
// JVM Clojure does (`+` carries :inline, so a direct 2-arg site compiles
// to Numbers.add and the var is never consulted — verified against
// clojure 1.12.5, ADR 0066 §Context), which is why the sealed program is
// MORE JVM-conformant, not less. It is nonetheless a change in cljgo's
// own observable behavior, so it is opt-in and never the default.
//
// Semantics for every non-redefined input are identical to the guarded
// helpers — same int64 fast path, same lang tower tail, same overflow
// throw, same printed error text.

// Add2S is the hard-sealed (+ x y).
func Add2S(x, y any) any {
	xi, xok := x.(int64)
	yi, yok := y.(int64)
	if xok && yok {
		s := xi + yi
		if (xi^s)&(yi^s) >= 0 { // no signed overflow
			return s
		}
	}
	return lang.Add(x, y)
}

// Sub2S is the hard-sealed (- x y).
func Sub2S(x, y any) any {
	xi, xok := x.(int64)
	yi, yok := y.(int64)
	if xok && yok {
		d := xi - yi
		if (xi^yi)&(xi^d) >= 0 { // no signed overflow
			return d
		}
	}
	return lang.Sub(x, y)
}

// Mul2S is the hard-sealed (* x y).
func Mul2S(x, y any) any {
	xi, xok := x.(int64)
	yi, yok := y.(int64)
	if xok && yok {
		return mulChecked(xi, yi)
	}
	return lang.Multiply(x, y)
}

// Div2S is the hard-sealed (/ x y) — ratio semantics stay in the tower.
func Div2S(x, y any) any { return lang.Divide(x, y) }

// LT2S is the hard-sealed (< x y).
func LT2S(x, y any) any {
	xi, xok := x.(int64)
	yi, yok := y.(int64)
	if xok && yok {
		return xi < yi
	}
	return lang.LT(x, y)
}

// GT2S is the hard-sealed (> x y).
func GT2S(x, y any) any {
	xi, xok := x.(int64)
	yi, yok := y.(int64)
	if xok && yok {
		return xi > yi
	}
	return lang.GT(x, y)
}

// LE2S is the hard-sealed (<= x y).
func LE2S(x, y any) any {
	xi, xok := x.(int64)
	yi, yok := y.(int64)
	if xok && yok {
		return xi <= yi
	}
	return lang.LTE(x, y)
}

// GE2S is the hard-sealed (>= x y).
func GE2S(x, y any) any {
	xi, xok := x.(int64)
	yi, yok := y.(int64)
	if xok && yok {
		return xi >= yi
	}
	return lang.GTE(x, y)
}

// EQ2S is the hard-sealed (= x y).
func EQ2S(x, y any) any { return lang.Equiv(x, y) }

// LTBoolS/GTBoolS/LEBoolS/GEBoolS/EQBoolS are the hard-sealed unboxed
// comparison variants the emitter uses directly in `if` tests.

func LTBoolS(x, y any) bool {
	xi, xok := x.(int64)
	yi, yok := y.(int64)
	if xok && yok {
		return xi < yi
	}
	return lang.LT(x, y)
}

func GTBoolS(x, y any) bool {
	xi, xok := x.(int64)
	yi, yok := y.(int64)
	if xok && yok {
		return xi > yi
	}
	return lang.GT(x, y)
}

func LEBoolS(x, y any) bool {
	xi, xok := x.(int64)
	yi, yok := y.(int64)
	if xok && yok {
		return xi <= yi
	}
	return lang.LTE(x, y)
}

func GEBoolS(x, y any) bool {
	xi, xok := x.(int64)
	yi, yok := y.(int64)
	if xok && yok {
		return xi >= yi
	}
	return lang.GTE(x, y)
}

func EQBoolS(x, y any) bool { return lang.Equiv(x, y) }
