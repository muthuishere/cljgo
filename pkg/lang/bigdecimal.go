package lang

import (
	"fmt"
	"math"
	"math/big"
	"strconv"
	"strings"
)

// BigDecimal is an immutable arbitrary-precision signed decimal:
// value = unscaled × 10^(-scale). This is java.math.BigDecimal's exact
// model (unscaled BigInteger + 32-bit scale), so every javadoc rule —
// arithmetic preferred scales, exact-or-throw division, the
// plain-vs-scientific toString boundary — ports 1:1 and stays checkable
// against the JVM oracle.
//
// cljgo ADR 0032 surgery: the original Glojure type wrapped *big.Float
// (binary mantissa — lost scale, represented Inf/NaN, silently
// truncated long literals). Replaced wholesale with the scaled-decimal
// model proven by spike S16 (spikes/s16-bigdecimal-scaled/VERDICT.md,
// 159/159 vs real Clojure 1.12.5). See pkg/lang/PROVENANCE.md.
type BigDecimal struct {
	unscaled *big.Int
	scale    int32
}

var bigDecTen = big.NewInt(10)

func bigDecPow10(n int64) *big.Int {
	return new(big.Int).Exp(bigDecTen, big.NewInt(n), nil)
}

func clampBigDecScale(s int64) int32 {
	if s < math.MinInt32 || s > math.MaxInt32 {
		panic(NewArithmeticError("Scale out of range."))
	}
	return int32(s)
}

// NewBigDecimal parses a string per the java.math.BigDecimal(String)
// grammar: [sign] digits [. digits] [(e|E) [sign] digits], or
// [sign] . digits [exp]. Scale and E-notation are preserved exactly.
func NewBigDecimal(s string) (*BigDecimal, error) {
	orig := s
	if s == "" {
		return nil, fmt.Errorf("empty String")
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
			return nil, fmt.Errorf("invalid exponent in %q", orig)
		}
		exp = e
	}
	intPart, fracPart := mant, ""
	if i := strings.IndexByte(mant, '.'); i >= 0 {
		intPart, fracPart = mant[:i], mant[i+1:]
	}
	if intPart == "" && fracPart == "" {
		return nil, fmt.Errorf("no digits in %q", orig)
	}
	digits := intPart + fracPart
	for _, c := range digits {
		if c < '0' || c > '9' {
			return nil, fmt.Errorf("Character %c is neither a decimal digit number, decimal point, nor \"e\" notation exponential mark.", c)
		}
	}
	u, ok := new(big.Int).SetString(digits, 10)
	if !ok {
		return nil, fmt.Errorf("invalid number %q", orig)
	}
	if neg {
		u.Neg(u)
	}
	scale := int64(len(fracPart)) - exp
	if scale < math.MinInt32 || scale > math.MaxInt32 {
		return nil, fmt.Errorf("Scale out of range.")
	}
	return &BigDecimal{unscaled: u, scale: int32(scale)}, nil
}

// MustBigDecimal parses a BigDecimal string or panics. It backs the
// emitter's constant literal reconstruction (pkg/emit constExpr) over a
// value cljgo itself printed, so failure is a compiler bug. Java's
// toString round-trips scale by design, so REPL and AOT constants stay
// identical.
func MustBigDecimal(s string) *BigDecimal {
	bd, err := NewBigDecimal(s)
	if err != nil {
		panic(err)
	}
	return bd
}

// NewBigDecimalFromFloat64 follows BigDecimal.valueOf(double) =
// parse(Double.toString(d)): the shortest decimal representation, with
// Java's trailing ".0" for integral values. Non-finite doubles throw
// "Infinite or NaN" exactly like the JVM ctor.
func NewBigDecimalFromFloat64(x float64) *BigDecimal {
	if math.IsInf(x, 0) || math.IsNaN(x) {
		panic(NewIllegalArgumentError("Infinite or NaN"))
	}
	s := strconv.FormatFloat(x, 'g', -1, 64)
	if !strings.ContainsAny(s, ".eE") {
		s += ".0" // Java Double.toString always has a fraction
	}
	return MustBigDecimal(s)
}

// NewBigDecimalFromInt64 gives scale 0 — like BigDecimal.valueOf(long).
func NewBigDecimalFromInt64(x int64) *BigDecimal {
	return &BigDecimal{unscaled: big.NewInt(x), scale: 0}
}

// NewBigDecimalFromBigInt gives scale 0 — like new BigDecimal(BigInteger).
func NewBigDecimalFromBigInt(x *big.Int) *BigDecimal {
	return &BigDecimal{unscaled: new(big.Int).Set(x), scale: 0}
}

// NewBigDecimalFromRatio implements Clojure's Ratio→BigDecimal
// (Ratio.decimalValue with MathContext.UNLIMITED): numerator divided by
// denominator exactly; panics on a non-terminating expansion.
func NewBigDecimalFromRatio(x *Ratio) *BigDecimal {
	num := NewBigDecimalFromBigInt(x.val.Num())
	den := NewBigDecimalFromBigInt(x.val.Denom())
	return num.Divide(den)
}

// Sign reports the sign of the value: -1, 0, or +1.
func (n *BigDecimal) Sign() int { return n.unscaled.Sign() }

// Scale returns the scale (fraction digits; negative = trailing zeros).
func (n *BigDecimal) Scale() int32 { return n.scale }

// precision = number of digits in the unscaled value (Java: 1 for zero).
func (n *BigDecimal) precision() int {
	if n.unscaled.Sign() == 0 {
		return 1
	}
	return len(new(big.Int).Abs(n.unscaled).String())
}

// adjExp is the adjusted exponent = precision - scale - 1 (the exponent
// of the leading digit).
func (n *BigDecimal) adjExp() int64 {
	return int64(n.precision()) - int64(n.scale) - 1
}

// bigDecAlign returns both unscaled values brought to the common (max)
// scale.
func bigDecAlign(x, y *BigDecimal) (ux, uy *big.Int, scale int32) {
	if x.scale == y.scale {
		return x.unscaled, y.unscaled, x.scale
	}
	if x.scale > y.scale {
		return x.unscaled, new(big.Int).Mul(y.unscaled, bigDecPow10(int64(x.scale)-int64(y.scale))), x.scale
	}
	return new(big.Int).Mul(x.unscaled, bigDecPow10(int64(y.scale)-int64(x.scale))), y.unscaled, y.scale
}

// ToBigInteger truncates toward zero (Java toBigInteger).
func (n *BigDecimal) ToBigInteger() *big.Int {
	switch {
	case n.scale == 0:
		return new(big.Int).Set(n.unscaled)
	case n.scale > 0:
		return new(big.Int).Quo(n.unscaled, bigDecPow10(int64(n.scale)))
	default:
		return new(big.Int).Mul(n.unscaled, bigDecPow10(-int64(n.scale)))
	}
}

// Float64 is Java doubleValue.
func (n *BigDecimal) Float64() float64 {
	f, _ := strconv.ParseFloat(n.String(), 64)
	return f
}

// Rat returns the exact rational value (backs rationalize).
func (n *BigDecimal) Rat() *big.Rat {
	if n.scale >= 0 {
		return new(big.Rat).SetFrac(n.unscaled, bigDecPow10(int64(n.scale)))
	}
	return new(big.Rat).SetFrac(new(big.Int).Mul(n.unscaled, bigDecPow10(-int64(n.scale))), big.NewInt(1))
}

// String ports the java.math.BigDecimal.toString javadoc algorithm:
//
//	If scale >= 0 and adjusted exponent >= -6: plain notation
//	  (insert a decimal point scale digits from the right, zero-padding;
//	   scale 0 = the digits themselves).
//	Otherwise: scientific notation — one digit, '.', remaining digits,
//	  'E', explicitly signed adjusted exponent.
func (n *BigDecimal) String() string {
	u := n.unscaled
	neg := u.Sign() < 0
	digits := new(big.Int).Abs(u).String()
	adj := n.adjExp()
	var body string
	if n.scale >= 0 && adj >= -6 {
		s := int(n.scale)
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

// PlainString forces plain (never scientific) notation at a non-negative
// scale — java.math.BigDecimal.toPlainString, minus the negative-scale
// case (format's callers always SetScale to precision >= 0 first). Backs
// format %f, which shows the full plain magnitude even for huge/tiny
// values where String() would switch to E-notation.
func (n *BigDecimal) PlainString() string {
	u := n.unscaled
	neg := u.Sign() < 0
	digits := new(big.Int).Abs(u).String()
	s := int(n.scale)
	var body string
	switch {
	case s <= 0:
		body = digits + strings.Repeat("0", -s)
	case len(digits) > s:
		body = digits[:len(digits)-s] + "." + digits[len(digits)-s:]
	default:
		body = "0." + strings.Repeat("0", s-len(digits)) + digits
	}
	if neg {
		return "-" + body
	}
	return body
}

// StripTrailingZeros returns the numerically-equal value with trailing
// fractional zeros moved into the scale (Java stripTrailingZeros). It
// backs hasheq: Clojure `=` on BigDecimals is compareTo-based, so
// =-equal values must hash identically.
func (n *BigDecimal) StripTrailingZeros() *BigDecimal {
	if n.unscaled.Sign() == 0 {
		return &BigDecimal{unscaled: big.NewInt(0), scale: 0}
	}
	u := new(big.Int).Set(n.unscaled)
	s := n.scale
	q, r := new(big.Int), new(big.Int)
	for {
		q.QuoRem(u, bigDecTen, r)
		if r.Sign() != 0 {
			break
		}
		u.Set(q)
		s--
	}
	return &BigDecimal{unscaled: u, scale: s}
}

// Hash hashes the scale-normalized value so that (= a b) implies
// (= (hash a) (hash b)) — oracle: (= (hash 1.0M) (hash 1.00M)) is true.
func (n *BigDecimal) Hash() uint32 {
	if n.unscaled.Sign() == 0 {
		return 0
	}
	norm := n.StripTrailingZeros()
	s := norm.unscaled.String() + "/" + strconv.FormatInt(int64(norm.scale), 10)
	return hashByteSlice([]byte(s))
}

// Equals is Clojure equiv for two BigDecimals: compareTo-based, i.e.
// scale-insensitive ((= 1.0M 1.00M) is TRUE per the 1.12.5 oracle);
// false for any non-BigDecimal (cross-category = is false).
func (n *BigDecimal) Equals(v interface{}) bool {
	other, ok := v.(*BigDecimal)
	if !ok {
		return false
	}
	return n.Cmp(other) == 0
}

func (n *BigDecimal) AddInt(x int) *BigDecimal {
	return n.Add(NewBigDecimalFromInt64(int64(x)))
}

// Add: scale = max(x.scale, y.scale) (Java BigDecimal.add).
func (n *BigDecimal) Add(other *BigDecimal) *BigDecimal {
	ux, uy, s := bigDecAlign(n, other)
	return &BigDecimal{unscaled: new(big.Int).Add(ux, uy), scale: s}
}

func (n *BigDecimal) AddP(other *BigDecimal) *BigDecimal {
	return n.Add(other)
}

// Sub: scale = max(x.scale, y.scale).
func (n *BigDecimal) Sub(other *BigDecimal) *BigDecimal {
	ux, uy, s := bigDecAlign(n, other)
	return &BigDecimal{unscaled: new(big.Int).Sub(ux, uy), scale: s}
}

func (n *BigDecimal) SubP(other *BigDecimal) *BigDecimal {
	return n.Sub(other)
}

// Multiply: scale = x.scale + y.scale.
func (n *BigDecimal) Multiply(other *BigDecimal) *BigDecimal {
	return &BigDecimal{
		unscaled: new(big.Int).Mul(n.unscaled, other.unscaled),
		scale:    clampBigDecScale(int64(n.scale) + int64(other.scale)),
	}
}

// Divide is Java divide(BigDecimal): the exact quotient at preferred
// scale x.scale - y.scale (zero-padded when the exact result is
// shorter); panics with ArithmeticException when the exact quotient has
// a non-terminating decimal expansion (denominator not 2^a·5^b) or the
// divisor is zero — a BigDecimal can never be Inf.
func (n *BigDecimal) Divide(other *BigDecimal) *BigDecimal {
	if other.unscaled.Sign() == 0 {
		panic(NewArithmeticError("Divide by zero"))
	}
	if n.unscaled.Sign() == 0 {
		pref := clampBigDecScale(int64(n.scale) - int64(other.scale))
		return &BigDecimal{unscaled: big.NewInt(0), scale: pref}
	}
	// exact quotient as reduced fraction num/den (of the unscaled values)
	num := new(big.Int).Set(n.unscaled)
	den := new(big.Int).Set(other.unscaled)
	g := new(big.Int).GCD(nil, nil, new(big.Int).Abs(num), new(big.Int).Abs(den))
	num.Quo(num, g)
	den.Quo(den, g)
	if den.Sign() < 0 {
		num.Neg(num)
		den.Neg(den)
	}
	// terminating iff den = 2^a * 5^b; the needed extra fractional digits
	// = max(a, b) (multiply num by (10^max)/den, exact).
	a, b := int64(0), int64(0)
	dd := new(big.Int).Set(den)
	two, five := big.NewInt(2), big.NewInt(5)
	for {
		q, rem := new(big.Int).QuoRem(dd, two, new(big.Int))
		if rem.Sign() != 0 {
			break
		}
		dd.Set(q)
		a++
	}
	for {
		q, rem := new(big.Int).QuoRem(dd, five, new(big.Int))
		if rem.Sign() != 0 {
			break
		}
		dd.Set(q)
		b++
	}
	if dd.Cmp(big.NewInt(1)) != 0 {
		panic(NewArithmeticError("Non-terminating decimal expansion; no exact representable decimal result."))
	}
	k := a
	if b > k {
		k = b
	}
	// exact: x.u/y.u = num/den = (num * 10^k / den) * 10^-k, so
	// value = uMin × 10^-(k + x.scale - y.scale)
	uMin := new(big.Int).Mul(num, bigDecPow10(k))
	uMin.Quo(uMin, den)
	sMin := k + int64(n.scale) - int64(other.scale)
	pref := int64(n.scale) - int64(other.scale)
	// pad to the preferred scale when the exact result is shorter
	if sMin < pref {
		uMin.Mul(uMin, bigDecPow10(pref-sMin))
		sMin = pref
	}
	return &BigDecimal{unscaled: uMin, scale: clampBigDecScale(sMin)}
}

// Quotient is Java divideToIntegralValue (Clojure quot): the integer
// part of the quotient, preferred scale x.scale - y.scale, padded with
// trailing zeros when representable — (quot 10.0M 3) is 3.0M but
// (quot 10.0M 3.0M) is 3M.
func (n *BigDecimal) Quotient(other *BigDecimal) *BigDecimal {
	if other.unscaled.Sign() == 0 {
		panic(NewArithmeticError("Divide by zero"))
	}
	ux, uy, _ := bigDecAlign(n, other)
	i := new(big.Int).Quo(ux, uy) // truncates toward zero
	pref := int64(n.scale) - int64(other.scale)
	if pref > 0 {
		return &BigDecimal{unscaled: i.Mul(i, bigDecPow10(pref)), scale: clampBigDecScale(pref)}
	}
	// negative preferred scale: representable only if i has the trailing
	// zeros; not exercised by Clojure-reachable paths — keep scale 0
	// (exact value; recorded limit in S16 VERDICT).
	return &BigDecimal{unscaled: i, scale: 0}
}

// Remainder is Java remainder (Clojure rem):
// x - (x divideToIntegralValue y)*y. Clojure mod builds on it in core.
func (n *BigDecimal) Remainder(other *BigDecimal) *BigDecimal {
	q := n.Quotient(other)
	return n.Sub(q.Multiply(other))
}

// Cmp compares values numerically, ignoring scale (Java compareTo).
func (n *BigDecimal) Cmp(other *BigDecimal) int {
	ux, uy, _ := bigDecAlign(n, other)
	return ux.Cmp(uy)
}

func (n *BigDecimal) LT(other *BigDecimal) bool {
	return n.Cmp(other) < 0
}

func (n *BigDecimal) LTE(other *BigDecimal) bool {
	return n.Cmp(other) <= 0
}

func (n *BigDecimal) GT(other *BigDecimal) bool {
	return n.Cmp(other) > 0
}

func (n *BigDecimal) GTE(other *BigDecimal) bool {
	return n.Cmp(other) >= 0
}

func (n *BigDecimal) Negate() *BigDecimal {
	return &BigDecimal{unscaled: new(big.Int).Neg(n.unscaled), scale: n.scale}
}

func (n *BigDecimal) Abs() *BigDecimal {
	if n.unscaled.Sign() < 0 {
		return n.Negate()
	}
	return n
}

// ------------------------------------------------------- with-precision ---
//
// ADR 0032 follow-on (S16 items 13-14, spikes/s16-bigdecimal-scaled/
// proto/decimal.go RoundingMode/Round/DivideMC, ported unchanged): a
// MathContext (precision + RoundingMode) drives `with-precision` /
// *math-context*. Precision here is SIGNIFICANT DIGITS (java.math.
// MathContext), distinct from `scale` (fraction digits) used by SetScale.

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

// roundingModeNames maps the bare-symbol names `with-precision` accepts
// (:rounding UP, CEILING, ...) to RoundingMode, matching
// java.math.RoundingMode.valueOf.
var roundingModeNames = map[string]RoundingMode{
	"UP":          RoundUp,
	"DOWN":        RoundDown,
	"CEILING":     RoundCeiling,
	"FLOOR":       RoundFloor,
	"HALF_UP":     RoundHalfUp,
	"HALF_DOWN":   RoundHalfDown,
	"HALF_EVEN":   RoundHalfEven,
	"UNNECESSARY": RoundUnnecessary,
}

// ParseRoundingMode looks up a RoundingMode by its Java enum name; error
// mirrors java.lang.IllegalArgumentException: valueOf on an unknown name.
func ParseRoundingMode(name string) (RoundingMode, error) {
	if m, ok := roundingModeNames[name]; ok {
		return m, nil
	}
	return 0, fmt.Errorf("No enum constant java.math.RoundingMode.%s", name)
}

// MathContext is java.math.MathContext: precision (0 = unlimited) +
// RoundingMode. *math-context* holds one of these (or nil = unbound =
// unlimited precision, today's default arithmetic).
type MathContext struct {
	Precision int
	Mode      RoundingMode
}

// NewMathContext builds a MathContext from `with-precision`'s (precision,
// rounding-mode-name) pair.
func NewMathContext(precision int, roundingName string) (*MathContext, error) {
	mode, err := ParseRoundingMode(roundingName)
	if err != nil {
		return nil, err
	}
	return &MathContext{Precision: precision, Mode: mode}, nil
}

// divRound divides |num|/|den| (den != 0) to an exact quotient + remainder,
// then rounds the quotient per mode using the TRUE remainder (no guard
// digits) — Java's BigDecimal rounding decision. `neg` is the true sign of
// the mathematical result (num/den before taking absolute values).
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
		return nil, NewArithmeticError("Rounding necessary")
	}
	if inc {
		q.Add(q, big.NewInt(1))
	}
	return q, nil
}

// Precision is the number of digits in the unscaled value (Java: 1 for
// zero) — exported alias of the package-private precision() used by
// MathContext rounding (Round/DivideMC) and format %e/%g.
func (n *BigDecimal) Precision() int { return n.precision() }

// Round applies a MathContext (precision, mode) — Java BigDecimal.
// plus(MathContext)/round: if the value has more significant digits than
// precision, discard the excess and round; precision <= 0 means unlimited
// (a no-op).
func (n *BigDecimal) Round(precision int, mode RoundingMode) (*BigDecimal, error) {
	if precision <= 0 {
		return n, nil
	}
	drop := n.precision() - precision
	if drop <= 0 || n.unscaled.Sign() == 0 {
		return n, nil
	}
	neg := n.unscaled.Sign() < 0
	q, err := divRound(n.unscaled, bigDecPow10(int64(drop)), mode, neg)
	if err != nil {
		return nil, err
	}
	s := int64(n.scale) - int64(drop)
	// rounding may add a digit (99 -> 10 x 10): renormalize
	if len(q.String()) > precision {
		q.Quo(q, bigDecTen)
		s--
	}
	if neg {
		q.Neg(q)
	}
	return &BigDecimal{unscaled: q, scale: clampBigDecScale(s)}, nil
}

// DivideMC is Java divide(divisor, MathContext): the quotient rounded to
// `precision` significant digits (never throws non-terminating; that's
// only for the no-MathContext exact Divide). precision <= 0 falls back to
// the exact Divide (still throws on non-termination, like MathContext.
// UNLIMITED).
func (n *BigDecimal) DivideMC(other *BigDecimal, precision int, mode RoundingMode) (*BigDecimal, error) {
	if other.unscaled.Sign() == 0 {
		return nil, NewArithmeticError("Divide by zero")
	}
	if precision <= 0 {
		return n.Divide(other), nil
	}
	if n.unscaled.Sign() == 0 {
		return &BigDecimal{unscaled: big.NewInt(0), scale: clampBigDecScale(int64(n.scale) - int64(other.scale))}, nil
	}
	nAbs := new(big.Int).Abs(n.unscaled)
	dAbs := new(big.Int).Abs(other.unscaled)
	neg := (n.unscaled.Sign() < 0) != (other.unscaled.Sign() < 0)
	// adjusted exponent of q = x/y: start from digit counts, correct by one
	// exact magnitude comparison.
	e := n.adjExp() - other.adjExp()
	shift := int64(len(dAbs.String())) - int64(len(nAbs.String()))
	nCmp := new(big.Int).Set(nAbs)
	dCmp := new(big.Int).Set(dAbs)
	if shift >= 0 {
		nCmp.Mul(nCmp, bigDecPow10(shift))
	} else {
		dCmp.Mul(dCmp, bigDecPow10(-shift))
	}
	if nCmp.Cmp(dCmp) < 0 {
		e-- // leading digit of the quotient is one place lower
	}
	s := int64(precision) - 1 - e // result scale
	exp := s + int64(other.scale) - int64(n.scale)
	num := new(big.Int).Set(nAbs)
	den := new(big.Int).Set(dAbs)
	if exp >= 0 {
		num.Mul(num, bigDecPow10(exp))
	} else {
		den.Mul(den, bigDecPow10(-exp))
	}
	q, err := divRound(num, den, mode, neg)
	if err != nil {
		return nil, err
	}
	if len(q.String()) > precision { // carry added a digit
		q.Quo(q, bigDecTen)
		s--
	}
	if neg {
		q.Neg(q)
	}
	return &BigDecimal{unscaled: q, scale: clampBigDecScale(s)}, nil
}

// Sci renders the value in forced scientific notation with exactly
// `fracDigits` digits after the decimal point (fracDigits+1 total
// significant digits), HALF_UP-rounded. Backs format %e/%g (S14/S16
// follow-on, ADR 0032 item 14): the mantissa (sign-less digit string with
// its decimal point already placed) + the adjusted exponent + whether the
// original value was negative.
func (n *BigDecimal) Sci(fracDigits int) (mantissa string, exp int64, neg bool, err error) {
	rounded, err := n.Round(fracDigits+1, RoundHalfUp)
	if err != nil {
		return "", 0, false, err
	}
	neg = rounded.unscaled.Sign() < 0
	digits := new(big.Int).Abs(rounded.unscaled).String()
	// Zero-pad on the right up to fracDigits+1 digits: only needed when
	// rounded is exactly zero (precision() is always 1 for zero,
	// regardless of fracDigits).
	if len(digits) < fracDigits+1 {
		digits += strings.Repeat("0", fracDigits+1-len(digits))
	}
	if fracDigits == 0 {
		mantissa = digits
	} else {
		mantissa = digits[:1] + "." + digits[1:]
	}
	exp = rounded.adjExp()
	return mantissa, exp, neg, nil
}

// SetScale rounds/pads to an exact fraction-digit SCALE (java.math.
// BigDecimal.setScale(int, RoundingMode)) — distinct from Round's
// significant-digit precision. Used by format %f (always plain notation
// at a caller-chosen fraction-digit count, independent of *math-context*).
func (n *BigDecimal) SetScale(newScale int32, mode RoundingMode) (*BigDecimal, error) {
	if newScale == n.scale {
		return n, nil
	}
	if newScale > n.scale {
		return &BigDecimal{
			unscaled: new(big.Int).Mul(n.unscaled, bigDecPow10(int64(newScale)-int64(n.scale))),
			scale:    newScale,
		}, nil
	}
	drop := int64(n.scale) - int64(newScale)
	neg := n.unscaled.Sign() < 0
	q, err := divRound(n.unscaled, bigDecPow10(drop), mode, neg)
	if err != nil {
		return nil, err
	}
	if neg {
		q.Neg(q)
	}
	return &BigDecimal{unscaled: q, scale: newScale}, nil
}
