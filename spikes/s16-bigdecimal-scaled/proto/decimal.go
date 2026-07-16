// Package main — S16 candidate (a) prototype: a scaled-decimal BigDecimal
// as unscaled *big.Int + int32 scale, java.math.BigDecimal's exact model.
// value = unscaled × 10^(-scale). Stdlib only (math/big) — no external deps.
//
// Implements the pieces the probe corpus exercises:
//   - literal parsing (Java BigDecimal(String) grammar)
//   - construction from int64 / big.Int / float64 (valueOf semantics) / ratio
//   - add/sub (scale = max), mul (scale = sum), negate, abs, compare
//   - divide: exact with preferred scale sx-sy, ArithmeticException
//     "Non-terminating decimal expansion" / "Divide by zero"
//   - divideToIntegralValue (quot), remainder (rem), Clojure mod on top
//   - MathContext rounding: all 8 RoundingModes, round-to-precision,
//     divide under a MathContext (exact integer-level rounding, no guard
//     digits — the rounding decision compares the true remainder)
//   - toString: the plain-vs-scientific algorithm ported from the
//     java.math.BigDecimal.toString javadoc
//   - stripTrailingZeros (backs the hasheq row: = is compareTo-based in
//     Clojure, so hasheq must normalize scale)
package main

import (
	"fmt"
	"math"
	"math/big"
	"strconv"
	"strings"
)

// Dec is an immutable arbitrary-precision signed decimal:
// value = unscaled × 10^(-scale). Java's exact model (BigDecimal:
// unscaled BigInteger + 32-bit scale).
type Dec struct {
	unscaled *big.Int
	scale    int32
}

// RoundingMode mirrors java.math.RoundingMode.
type RoundingMode int

const (
	RoundUp RoundingMode = iota
	RoundDown
	RoundCeiling
	RoundFloor
	RoundHalfUp
	RoundHalfDown
	RoundHalfEven
	RoundUnnecessary
)

type arithErr struct{ msg string }

func (e arithErr) Error() string { return e.msg }

func throwf(format string, args ...any) error { return arithErr{fmt.Sprintf(format, args...)} }

var (
	big10 = big.NewInt(10)
)

func pow10(n int64) *big.Int {
	return new(big.Int).Exp(big10, big.NewInt(n), nil)
}

// ---------------------------------------------------------------- parse ---

// Parse implements the java.math.BigDecimal(String) grammar:
// [sign] digits [. digits] [ (e|E) [sign] digits ], or [sign] . digits [exp].
func Parse(s string) (*Dec, error) {
	orig := s
	if s == "" {
		return nil, throwf("empty string")
	}
	neg := false
	switch s[0] {
	case '+':
		s = s[1:]
	case '-':
		neg = true
		s = s[1:]
	}
	mant := s
	var exp int64
	if i := strings.IndexAny(s, "eE"); i >= 0 {
		mant = s[:i]
		e, err := strconv.ParseInt(s[i+1:], 10, 32)
		if err != nil {
			return nil, throwf("invalid exponent in %q", orig)
		}
		exp = e
	}
	intPart, fracPart := mant, ""
	if i := strings.IndexByte(mant, '.'); i >= 0 {
		intPart, fracPart = mant[:i], mant[i+1:]
	}
	if intPart == "" && fracPart == "" {
		return nil, throwf("no digits in %q", orig)
	}
	digits := intPart + fracPart
	for _, c := range digits {
		if c < '0' || c > '9' {
			return nil, throwf("Character %c is neither a decimal digit number, decimal point, nor \"e\" notation exponential mark.", c)
		}
	}
	u, ok := new(big.Int).SetString(digits, 10)
	if !ok {
		return nil, throwf("invalid number %q", orig)
	}
	if neg {
		u.Neg(u)
	}
	scale := int64(len(fracPart)) - exp
	if scale < math.MinInt32 || scale > math.MaxInt32 {
		return nil, throwf("Scale out of range.")
	}
	return &Dec{unscaled: u, scale: int32(scale)}, nil
}

func MustParse(s string) *Dec {
	d, err := Parse(s)
	if err != nil {
		panic(err)
	}
	return d
}

// FromInt64 gives scale 0 — like BigDecimal.valueOf(long).
func FromInt64(x int64) *Dec { return &Dec{unscaled: big.NewInt(x), scale: 0} }

// FromBigInt gives scale 0 — like new BigDecimal(BigInteger).
func FromBigInt(x *big.Int) *Dec { return &Dec{unscaled: new(big.Int).Set(x), scale: 0} }

// FromFloat64 follows BigDecimal.valueOf(double) = parse(Double.toString(d)):
// the SHORTEST decimal representation, with Java's trailing ".0" for
// integral values. Errors on Inf/NaN like Java ("Infinite or NaN").
//
// Caveat (recorded in VERDICT): Go's shortest-repr formatting and Java's
// Double.toString agree on shortestness but can differ in exponent
// switch-over points (e.g. Java prints 1.0E23, Go 1e+23) — the resulting
// Dec VALUE and scale are identical; only intermediate strings differ.
func FromFloat64(f float64) (*Dec, error) {
	if math.IsInf(f, 0) || math.IsNaN(f) {
		return nil, throwf("Infinite or NaN")
	}
	s := strconv.FormatFloat(f, 'g', -1, 64)
	if !strings.ContainsAny(s, ".eE") {
		s += ".0" // Java Double.toString always has a fraction
	}
	return Parse(s)
}

// FromRatio implements Clojure's Ratio→BigDecimal (Ratio.decimalValue):
// numerator BigDecimal divided by denominator BigDecimal (exact divide;
// throws on non-terminating expansion).
func FromRatio(num, den *big.Int) (*Dec, error) {
	return FromBigInt(num).Divide(FromBigInt(den))
}

// ------------------------------------------------------------ accessors ---

func (d *Dec) Sign() int    { return d.unscaled.Sign() }
func (d *Dec) Scale() int32 { return d.scale }

// Precision = number of digits in the unscaled value (Java: 1 for zero).
func (d *Dec) Precision() int {
	if d.unscaled.Sign() == 0 {
		return 1
	}
	return len(new(big.Int).Abs(d.unscaled).String())
}

// adjusted exponent = precision - scale - 1 (exponent of the leading digit).
func (d *Dec) adjExp() int64 { return int64(d.Precision()) - int64(d.scale) - 1 }

// align returns both unscaled values brought to the common (max) scale.
func align(x, y *Dec) (ux, uy *big.Int, scale int32) {
	if x.scale == y.scale {
		return x.unscaled, y.unscaled, x.scale
	}
	if x.scale > y.scale {
		return x.unscaled, new(big.Int).Mul(y.unscaled, pow10(int64(x.scale)-int64(y.scale))), x.scale
	}
	return new(big.Int).Mul(x.unscaled, pow10(int64(y.scale)-int64(x.scale))), y.unscaled, y.scale
}

// --------------------------------------------------------------- arith ----

// Add: scale = max(x.scale, y.scale). (Java BigDecimal.add)
func (x *Dec) Add(y *Dec) *Dec {
	ux, uy, s := align(x, y)
	return &Dec{unscaled: new(big.Int).Add(ux, uy), scale: s}
}

// Sub: scale = max(x.scale, y.scale).
func (x *Dec) Sub(y *Dec) *Dec {
	ux, uy, s := align(x, y)
	return &Dec{unscaled: new(big.Int).Sub(ux, uy), scale: s}
}

// Mul: scale = x.scale + y.scale.
func (x *Dec) Mul(y *Dec) *Dec {
	return &Dec{
		unscaled: new(big.Int).Mul(x.unscaled, y.unscaled),
		scale:    x.scale + y.scale,
	}
}

func (x *Dec) Neg() *Dec {
	return &Dec{unscaled: new(big.Int).Neg(x.unscaled), scale: x.scale}
}

func (x *Dec) Abs() *Dec {
	if x.unscaled.Sign() < 0 {
		return x.Neg()
	}
	return x
}

// Cmp compares values numerically, ignoring scale (Java compareTo).
func (x *Dec) Cmp(y *Dec) int {
	ux, uy, _ := align(x, y)
	return ux.Cmp(uy)
}

// EqualsJava is Java .equals: same unscaled AND same scale (1.0 ≠ 1.00).
// NOTE: Clojure `=` on two BigDecimals is equiv (compareTo-based) — the
// oracle proves (= 1.0M 1.00M) is TRUE; .equals matters only for interop.
func (x *Dec) EqualsJava(y *Dec) bool {
	return x.scale == y.scale && x.unscaled.Cmp(y.unscaled) == 0
}

// StripTrailingZeros — needed for hasheq (Clojure hashes BigDecimal
// via stripTrailingZeros so that = values hash alike).
func (x *Dec) StripTrailingZeros() *Dec {
	if x.unscaled.Sign() == 0 {
		return &Dec{unscaled: big.NewInt(0), scale: 0}
	}
	u := new(big.Int).Set(x.unscaled)
	s := x.scale
	q, r := new(big.Int), new(big.Int)
	for {
		q.QuoRem(u, big10, r)
		if r.Sign() != 0 {
			break
		}
		u.Set(q)
		s--
	}
	return &Dec{unscaled: u, scale: s}
}

// ------------------------------------------------------------- division ---

// Divide is Java divide(BigDecimal): the exact quotient, preferred scale
// x.scale - y.scale; throws if the exact quotient has a non-terminating
// decimal expansion.
func (x *Dec) Divide(y *Dec) (*Dec, error) {
	if y.unscaled.Sign() == 0 {
		return nil, throwf("Divide by zero")
	}
	if x.unscaled.Sign() == 0 {
		pref := clampScale(int64(x.scale) - int64(y.scale))
		return &Dec{unscaled: big.NewInt(0), scale: pref}, nil
	}
	// exact quotient as reduced fraction n/d (of the unscaled values)
	n := new(big.Int).Set(x.unscaled)
	d := new(big.Int).Set(y.unscaled)
	g := new(big.Int).GCD(nil, nil, new(big.Int).Abs(n), new(big.Int).Abs(d))
	n.Quo(n, g)
	d.Quo(d, g)
	if d.Sign() < 0 {
		n.Neg(n)
		d.Neg(d)
	}
	// terminating iff d = 2^a * 5^b; the needed extra fractional digits
	// = max(a, b) (multiply n by (10^max)/d, exact).
	a, b := int64(0), int64(0)
	dd := new(big.Int).Set(d)
	two, five := big.NewInt(2), big.NewInt(5)
	r := new(big.Int)
	for {
		q, rem := new(big.Int).QuoRem(dd, two, r)
		if rem.Sign() != 0 {
			break
		}
		dd.Set(q)
		a++
	}
	for {
		q, rem := new(big.Int).QuoRem(dd, five, r)
		if rem.Sign() != 0 {
			break
		}
		dd.Set(q)
		b++
	}
	if dd.Cmp(big.NewInt(1)) != 0 {
		return nil, throwf("Non-terminating decimal expansion; no exact representable decimal result.")
	}
	k := a
	if b > k {
		k = b
	}
	// exact: x.u/y.u = n/d = (n * 10^k / d) * 10^-k, so
	// value = u_min × 10^-(k + x.scale - y.scale)
	uMin := new(big.Int).Mul(n, pow10(k))
	uMin.Quo(uMin, d)
	sMin := int64(k) + int64(x.scale) - int64(y.scale)
	pref := int64(x.scale) - int64(y.scale)
	// pad to the preferred scale when the exact result is shorter
	if sMin < pref {
		uMin.Mul(uMin, pow10(pref-sMin))
		sMin = pref
	}
	return &Dec{unscaled: uMin, scale: clampScale(sMin)}, nil
}

func clampScale(s int64) int32 {
	if s < math.MinInt32 || s > math.MaxInt32 {
		panic(throwf("Scale out of range."))
	}
	return int32(s)
}

// DivideToIntegral is Java divideToIntegralValue (Clojure quot): the
// integer part of the quotient, preferred scale x.scale - y.scale
// (padded with trailing zeros when representable, e.g. 10.0/3 → 3.0).
func (x *Dec) DivideToIntegral(y *Dec) (*Dec, error) {
	if y.unscaled.Sign() == 0 {
		return nil, throwf("Divide by zero")
	}
	ux, uy, _ := align(x, y)
	i := new(big.Int).Quo(ux, uy) // truncates toward zero
	pref := int64(x.scale) - int64(y.scale)
	if pref > 0 {
		return &Dec{unscaled: i.Mul(i, pow10(pref)), scale: clampScale(pref)}, nil
	}
	// negative preferred scale: representable only if i has the trailing
	// zeros; the corpus doesn't exercise it — keep scale 0 (exact value).
	return &Dec{unscaled: i, scale: 0}, nil
}

// Rem is Java remainder (Clojure rem): x - (x divideToIntegral y)*y.
func (x *Dec) Rem(y *Dec) (*Dec, error) {
	q, err := x.DivideToIntegral(y)
	if err != nil {
		return nil, err
	}
	return x.Sub(q.Mul(y)), nil
}

// Mod is Clojure mod: floored — rem, then add y if signs differ.
func (x *Dec) Mod(y *Dec) (*Dec, error) {
	r, err := x.Rem(y)
	if err != nil {
		return nil, err
	}
	if r.Sign() != 0 && r.Sign() != y.Sign() {
		r = r.Add(y)
	}
	return r, nil
}

// ------------------------------------------------------------- rounding ---

// divRound divides |num| by |den| (both > 0 expected as magnitudes) and
// rounds the quotient per mode. neg is the sign of the true result; the
// returned magnitude is rounded as Java rounds (CEILING/FLOOR are
// direction-, not magnitude-, sensitive). The remainder comparison is
// exact — no guard digits.
func divRound(num, den *big.Int, mode RoundingMode, neg bool) (*big.Int, error) {
	q, r := new(big.Int).QuoRem(new(big.Int).Abs(num), new(big.Int).Abs(den), new(big.Int))
	if r.Sign() == 0 {
		return q, nil
	}
	inc := false
	switch mode {
	case RoundUp:
		inc = true
	case RoundDown:
		inc = false
	case RoundCeiling:
		inc = !neg
	case RoundFloor:
		inc = neg
	case RoundHalfUp, RoundHalfDown, RoundHalfEven:
		cmp := new(big.Int).Lsh(r, 1).Cmp(new(big.Int).Abs(den)) // 2r vs den
		switch {
		case cmp > 0:
			inc = true
		case cmp < 0:
			inc = false
		default: // exactly half
			switch mode {
			case RoundHalfUp:
				inc = true
			case RoundHalfDown:
				inc = false
			case RoundHalfEven:
				inc = q.Bit(0) == 1
			}
		}
	case RoundUnnecessary:
		return nil, throwf("Rounding necessary")
	}
	if inc {
		q.Add(q, big.NewInt(1))
	}
	return q, nil
}

// Round applies a MathContext (precision, mode) — Java doRound: if the
// value has more digits than precision, discard and round; precision 0
// means unlimited.
func (x *Dec) Round(precision int, mode RoundingMode) (*Dec, error) {
	if precision <= 0 {
		return x, nil
	}
	drop := x.Precision() - precision
	if drop <= 0 || x.unscaled.Sign() == 0 {
		return x, nil
	}
	neg := x.unscaled.Sign() < 0
	q, err := divRound(x.unscaled, pow10(int64(drop)), mode, neg)
	if err != nil {
		return nil, err
	}
	s := int64(x.scale) - int64(drop)
	// rounding may add a digit (99 → 10 × 10): renormalize
	if len(q.String()) > precision {
		q.Quo(q, big10)
		s--
	}
	if neg {
		q.Neg(q)
	}
	return &Dec{unscaled: q, scale: clampScale(s)}, nil
}

// DivideMC is Java divide(divisor, MathContext): quotient rounded to
// precision significant digits. Result scale = precision - 1 - adjExp(q),
// computed exactly (integer division with true-remainder rounding).
func (x *Dec) DivideMC(y *Dec, precision int, mode RoundingMode) (*Dec, error) {
	if y.unscaled.Sign() == 0 {
		return nil, throwf("Divide by zero")
	}
	if precision <= 0 { // unlimited context = exact divide
		return x.Divide(y)
	}
	if x.unscaled.Sign() == 0 {
		return &Dec{unscaled: big.NewInt(0), scale: clampScale(int64(x.scale) - int64(y.scale))}, nil
	}
	nAbs := new(big.Int).Abs(x.unscaled)
	dAbs := new(big.Int).Abs(y.unscaled)
	neg := (x.unscaled.Sign() < 0) != (y.unscaled.Sign() < 0)
	// adjusted exponent of q = x/y: start from digit counts, correct by
	// one exact magnitude comparison.
	e := x.adjExp() - y.adjExp()
	// compare mantissas: nAbs·10^(digits(d)-digits(n)) vs dAbs
	shift := int64(len(dAbs.String())) - int64(len(nAbs.String()))
	nCmp := new(big.Int).Set(nAbs)
	dCmp := new(big.Int).Set(dAbs)
	if shift >= 0 {
		nCmp.Mul(nCmp, pow10(shift))
	} else {
		dCmp.Mul(dCmp, pow10(-shift))
	}
	if nCmp.Cmp(dCmp) < 0 {
		e-- // leading digit of the quotient is one place lower
	}
	s := int64(precision) - 1 - e // result scale
	// unscaled = round( x.u · 10^(s + y.scale - x.scale) / y.u )
	exp := s + int64(y.scale) - int64(x.scale)
	num := new(big.Int).Set(nAbs)
	den := new(big.Int).Set(dAbs)
	if exp >= 0 {
		num.Mul(num, pow10(exp))
	} else {
		den.Mul(den, pow10(-exp))
	}
	q, err := divRound(num, den, mode, neg)
	if err != nil {
		return nil, err
	}
	if len(q.String()) > precision { // carry added a digit
		q.Quo(q, big10)
		s--
	}
	if neg {
		q.Neg(q)
	}
	return &Dec{unscaled: q, scale: clampScale(s)}, nil
}

// --------------------------------------------------------- conversions ----

// ToBigInt truncates toward zero (Java toBigInteger).
func (x *Dec) ToBigInt() *big.Int {
	if x.scale == 0 {
		return new(big.Int).Set(x.unscaled)
	}
	if x.scale > 0 {
		return new(big.Int).Quo(x.unscaled, pow10(int64(x.scale)))
	}
	return new(big.Int).Mul(x.unscaled, pow10(-int64(x.scale)))
}

// Float64 (Java doubleValue).
func (x *Dec) Float64() float64 {
	f, _ := strconv.ParseFloat(x.plainOrSci(), 64)
	return f
}

func (x *Dec) plainOrSci() string { return x.String() }

// Hasheq mirrors Clojure: hash the scale-normalized value so that
// (= a b) → (= (hash a) (hash b)). (Clojure hasheq for BigDecimal uses
// stripTrailingZeros().hashCode().)
func (x *Dec) Hasheq() string {
	n := x.StripTrailingZeros()
	return n.unscaled.String() + "/" + strconv.FormatInt(int64(n.scale), 10)
}

// ------------------------------------------------------------- toString ---

// String ports the java.math.BigDecimal.toString javadoc algorithm:
//
//	If scale >= 0 and adjusted exponent >= -6: plain notation
//	  (insert a decimal point scale digits from the right, zero-padding;
//	   scale 0 = the digits themselves).
//	Otherwise: scientific notation — one digit, '.', remaining digits,
//	  'E', explicitly signed adjusted exponent.
func (x *Dec) String() string {
	u := x.unscaled
	neg := u.Sign() < 0
	digits := new(big.Int).Abs(u).String()
	adj := x.adjExp()
	var body string
	if x.scale >= 0 && adj >= -6 {
		s := int(x.scale)
		switch {
		case s == 0:
			body = digits
		case len(digits) > s:
			body = digits[:len(digits)-s] + "." + digits[len(digits)-s:]
		default:
			body = "0." + strings.Repeat("0", s-len(digits)) + digits
		}
	} else {
		if len(digits) == 1 {
			body = digits
		} else {
			body = digits[:1] + "." + digits[1:]
		}
		if adj >= 0 {
			body += "E+" + strconv.FormatInt(adj, 10)
		} else {
			body += "E" + strconv.FormatInt(adj, 10)
		}
	}
	if neg {
		return "-" + body
	}
	return body
}
