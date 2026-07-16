package lang

import "testing"

// TestBigDecimalStringRoundTrip is the emitter's REPL-vs-binary guard
// (ADR 0032, S16 inventory item 7): pkg/emit reconstructs BigDecimal
// constants as lang.MustBigDecimal(x.String()), so Java's toString must
// round-trip both the value and the scale exactly.
func TestBigDecimalStringRoundTrip(t *testing.T) {
	inputs := []string{
		"1", "1.0", "1.00", "1.10", "-1.10", "100", "0.000", "123.456",
		"1E+2", "1.23E+3", "123.45", "1E+10", "1E-10",
		"0.000001", "1E-7", "-0.0",
		"123456789012345678901234567890.12",
		"1.5E+300", "9.999999E-7", "-4.56E-8",
	}
	for _, in := range inputs {
		d := MustBigDecimal(in)
		s := d.String()
		rt := MustBigDecimal(s)
		if rt.Cmp(d) != 0 || rt.Scale() != d.Scale() {
			t.Errorf("round-trip %q: got %q (scale %d), want value/scale of original (scale %d)",
				in, rt.String(), rt.Scale(), d.Scale())
		}
		if rt.String() != s {
			t.Errorf("toString not a fixed point for %q: %q -> %q", in, s, rt.String())
		}
	}
}

// TestBigDecimalOracleToString pins javadoc toString outputs frozen from
// the S16 oracle corpus (real Clojure 1.12.5).
func TestBigDecimalOracleToString(t *testing.T) {
	cases := map[string]string{
		"1.10":      "1.10",     // scale preserved
		"-0.0":      "0.0",      // no negative zero
		"1e2":       "1E+2",     // E-notation normalized
		"12345E-2":  "123.45",   // plain when in range
		"0.000001":  "0.000001", // adjusted exponent -6: still plain
		"0.0000001": "1E-7",     // adjusted exponent -7: scientific
	}
	for in, want := range cases {
		if got := MustBigDecimal(in).String(); got != want {
			t.Errorf("String(%q) = %q, want %q", in, got, want)
		}
	}
}
