package reader

import (
	"errors"
	"math/big"
	"regexp"
	"strconv"
	"strings"

	"github.com/muthuishere/cljgo/pkg/lang"
)

// Ports of clojure.lang.LispReader's intPat / floatPat / ratioPat.
// Alternative order matters and is kept verbatim: "08" falls through
// to the capture-less `0[0-9]+` branch, which matchNumber treats as
// an invalid number (exactly Clojure's behavior).
var (
	intPat   = regexp.MustCompile(`^([-+]?)(?:(0)|([1-9][0-9]*)|0[xX]([0-9A-Fa-f]+)|0([0-7]+)|([1-9][0-9]?)[rR]([0-9A-Za-z]+)|0[0-9]+)(N)?$`)
	floatPat = regexp.MustCompile(`^([-+]?[0-9]+(\.[0-9]*)?([eE][-+]?[0-9]+)?)(M)?$`)
	ratioPat = regexp.MustCompile(`^([-+]?[0-9]+)/([0-9]+)$`)
)

// matchNumber interprets a number token, mirroring
// LispReader.matchNumber. It returns (nil, nil) when the token is not
// a valid number ("Invalid number"), and a non-nil error only for
// tokens that match a pattern but are semantically broken (ratio with
// zero denominator, digits invalid for the radix).
func matchNumber(s string) (any, error) {
	if m := intPat.FindStringSubmatch(s); m != nil {
		if m[2] != "" { // literal 0 (with optional sign)
			if m[8] != "" {
				return lang.NewBigIntFromInt64(0), nil
			}
			return int64(0), nil
		}
		negate := m[1] == "-"
		var digits string
		radix := 10
		switch {
		case m[3] != "":
			digits = m[3]
		case m[4] != "":
			digits, radix = m[4], 16
		case m[5] != "":
			digits, radix = m[5], 8
		case m[7] != "":
			digits = m[7]
			radix, _ = strconv.Atoi(m[6])
		default:
			// Matched the trailing `0[0-9]+` alternative (e.g. "08"):
			// invalid, per Clojure. CLI check: (read-string "08") =>
			// "Invalid number: 08"; (read-string "06") => 6 (octal).
			return nil, nil
		}
		bn, ok := new(big.Int).SetString(digits, radix)
		if !ok {
			// e.g. 2r102 — digit out of range for the radix. Clojure
			// raises NumberFormatException ("For input string: \"102\"
			// under radix 2"); we report an invalid number.
			return nil, errors.New("radix digits out of range: " + s)
		}
		if negate {
			bn.Neg(bn)
		}
		if m[8] != "" {
			// N suffix always yields a BigInt, even when small:
			// CLI check: (read-string "0xFFN") => 255N.
			return lang.NewBigIntFromGoBigInt(bn), nil
		}
		if bn.IsInt64() {
			return bn.Int64(), nil
		}
		// No suffix but exceeds int64: BigInt, like Clojure.
		// CLI check: (read-string "99999999999999999999999") =>
		// 99999999999999999999999N (clojure.lang.BigInt).
		return lang.NewBigIntFromGoBigInt(bn), nil
	}
	if m := floatPat.FindStringSubmatch(s); m != nil {
		if m[4] != "" { // M suffix => BigDecimal
			bd, err := lang.NewBigDecimal(m[1])
			if err != nil {
				return nil, nil
			}
			return bd, nil
		}
		f, err := strconv.ParseFloat(s, 64)
		if err != nil {
			// Go still returns the correctly-signed value on ErrRange
			// (magnitude too large/small for float64): an exponent that
			// overflows saturates to +-Inf, one that underflows to 0 —
			// exactly Java's Double.parseDouble, which never throws for
			// this (oracle 1.12.5: (read-string "1e400") => ##Inf;
			// (read-string "-1e400") => ##-Inf; (read-string "1e-400") =>
			// 0.0). Any OTHER ParseFloat error is a genuine bad token.
			var numErr *strconv.NumError
			if errors.As(err, &numErr) && errors.Is(numErr.Err, strconv.ErrRange) {
				return f, nil
			}
			return nil, nil
		}
		return f, nil
	}
	if m := ratioPat.FindStringSubmatch(s); m != nil {
		// Clojure strips a leading + from the numerator before
		// BigInteger parsing.
		numStr := strings.TrimPrefix(m[1], "+")
		num, ok1 := new(big.Int).SetString(numStr, 10)
		den, ok2 := new(big.Int).SetString(m[2], 10)
		if !ok1 || !ok2 {
			return nil, nil
		}
		if den.Sign() == 0 {
			// CLI check: (read-string "1/0") => "Divide by zero".
			return nil, errors.New("Divide by zero")
		}
		rat := new(big.Rat).SetFrac(num, den)
		if rat.IsInt() {
			// Ratios reduce, and whole results collapse to integers:
			// CLI check: (read-string "4/2") => 2 (a Long),
			// (read-string "6/8") => 3/4.
			n := rat.Num()
			if n.IsInt64() {
				return n.Int64(), nil
			}
			return lang.NewBigIntFromGoBigInt(n), nil
		}
		return lang.NewRatioGoBigInt(rat.Num(), rat.Denom()), nil
	}
	return nil, nil
}

// readNumber accumulates a number token. Numbers terminate at
// whitespace or ANY macro character (including the non-terminating
// # ' % — Clojure's readNumber uses isMacro, not isTerminatingMacro).
func (r *Reader) readNumber(start Position, prefix string) (any, error) {
	var b strings.Builder
	b.WriteString(prefix)
	for {
		c, err := r.s.Read()
		if err != nil {
			break
		}
		if isWhitespace(c) || isMacro(c) {
			r.s.Unread()
			break
		}
		b.WriteRune(c)
	}
	tok := b.String()
	n, err := matchNumber(tok)
	if err != nil {
		return nil, r.errAt(start, "Invalid number: %s (%v)", tok, err)
	}
	if n == nil {
		return nil, r.errAt(start, "Invalid number: %s", tok)
	}
	return n, nil
}
