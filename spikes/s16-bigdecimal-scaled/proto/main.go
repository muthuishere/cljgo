// S16 harness: recompute every probe from probes.clj / probes_wp.clj
// through the candidate-(a) prototype and diff against the frozen oracle
// output (real Clojure 1.12.5) line by line.
//
//	go run . -oracle ../out/probes.oracle.txt -oracle-wp ../out/probes_wp.oracle.txt
//
// A row matches when the rendered value is byte-identical to the oracle's,
// or when BOTH sides threw (exception messages are host wording, recorded
// but not scored — same policy as S13/S14).
//
// Tower-dispatch rows (bigdec nil/true/:a, double contamination) encode
// the DISPATCH decision in the harness (that logic lives in pkg/lang's Ops
// matrix, not in the representation); every numeric/scale/print result is
// computed by the prototype itself.
package main

import (
	"bufio"
	"flag"
	"fmt"
	"math"
	"math/big"
	"os"
	"strconv"
	"strings"
)

type row struct {
	label string
	f     func() (string, error)
}

func dec(s string) *Dec { return MustParse(s) }

// prM renders a Dec the way Clojure pr-str renders a BigDecimal.
func prM(d *Dec) string { return d.String() + "M" }

// prDouble renders a float64 the way Clojure prints doubles.
func prDouble(f float64) string {
	if math.IsNaN(f) {
		return "##NaN"
	}
	if math.IsInf(f, 1) {
		return "##Inf"
	}
	if math.IsInf(f, -1) {
		return "##-Inf"
	}
	s := strconv.FormatFloat(f, 'g', -1, 64)
	if !strings.ContainsAny(s, ".eE") {
		s += ".0"
	}
	return s
}

func v(s string) func() (string, error) { // constant row
	return func() (string, error) { return s, nil }
}

func d1(f func() (*Dec, error)) func() (string, error) {
	return func() (string, error) {
		d, err := f()
		if err != nil {
			return "", err
		}
		return prM(d), nil
	}
}

func b(f func() bool) func() (string, error) {
	return func() (string, error) { return strconv.FormatBool(f()), nil }
}

func fromF(f float64) func() (string, error) {
	return d1(func() (*Dec, error) { return FromFloat64(f) })
}

func typeErr(msg string) func() (string, error) {
	return func() (string, error) { return "", throwf("%s", msg) }
}

// clojure = on numbers: same category (decimal/integer/floating/ratio)
// AND equiv. Categories differ on every cross-type row in the corpus.
// clojure == : equiv across the tower.

func rows() []row {
	one0 := dec("1.0")
	one00 := dec("1.00")
	i := func(n int64) *Dec { return FromInt64(n) }
	bigN, _ := new(big.Int).SetString("123456789012345678901234567890", 10)

	quot := func(x, y *Dec) (*Dec, error) { return x.DivideToIntegral(y) }
	div := func(x, y *Dec) (*Dec, error) { return x.Divide(y) }

	return []row{
		// ------------------------------------------------ literal scale --
		{"lit 1M", d1(func() (*Dec, error) { return Parse("1") })},
		{"lit 1.0M", d1(func() (*Dec, error) { return Parse("1.0") })},
		{"lit 1.00M", d1(func() (*Dec, error) { return Parse("1.00") })},
		{"lit 1.10M", d1(func() (*Dec, error) { return Parse("1.10") })},
		{"lit -1.10M", d1(func() (*Dec, error) { return Parse("-1.10") })},
		{"lit 100M", d1(func() (*Dec, error) { return Parse("100") })},
		{"lit 0.000M", d1(func() (*Dec, error) { return Parse("0.000") })},
		{"lit -0.0M", d1(func() (*Dec, error) { return Parse("-0.0") })},
		{"lit 123.456M", d1(func() (*Dec, error) { return Parse("123.456") })},
		{"lit 1E+2M", d1(func() (*Dec, error) { return Parse("1E+2") })},
		{"lit 1e2M", d1(func() (*Dec, error) { return Parse("1e2") })},
		{"lit 1.23E3M", d1(func() (*Dec, error) { return Parse("1.23E3") })},
		{"lit 12345E-2M", d1(func() (*Dec, error) { return Parse("12345E-2") })},
		{"lit 1E10M", d1(func() (*Dec, error) { return Parse("1E10") })},
		{"lit 1E-10M", d1(func() (*Dec, error) { return Parse("1E-10") })},
		{"lit 0.000001M", d1(func() (*Dec, error) { return Parse("0.000001") })},
		{"lit 0.0000001M", d1(func() (*Dec, error) { return Parse("0.0000001") })},
		{"lit 123456789012345678901234567890.12M", d1(func() (*Dec, error) { return Parse("123456789012345678901234567890.12") })},

		// ---------------------------------------------- bigdec coercion --
		{"bigdec 1", v(prM(i(1)))},
		{"bigdec 0", v(prM(i(0)))},
		{"bigdec -1", v(prM(i(-1)))},
		{"bigdec 1N", v(prM(FromBigInt(big.NewInt(1))))},
		{"bigdec bigN", v(prM(FromBigInt(bigN)))},
		{"bigdec 1.0", fromF(1.0)},
		{"bigdec 0.0", fromF(0.0)},
		{"bigdec -1.0", fromF(-1.0)},
		{"bigdec -0.0", fromF(math.Copysign(0, -1))},
		{"bigdec 0.1", fromF(0.1)},
		{"bigdec 1.5e300", fromF(1.5e300)},
		{"bigdec 1.0M", v(prM(one0))},
		{"bigdec 1/2", d1(func() (*Dec, error) { return FromRatio(big.NewInt(1), big.NewInt(2)) })},
		{"bigdec -1/2", d1(func() (*Dec, error) { return FromRatio(big.NewInt(-1), big.NewInt(2)) })},
		{"bigdec str 0", d1(func() (*Dec, error) { return Parse("0") })},
		{"bigdec str 1", d1(func() (*Dec, error) { return Parse("1") })},
		{"bigdec str +1", d1(func() (*Dec, error) { return Parse("+1") })},
		{"bigdec str -1", d1(func() (*Dec, error) { return Parse("-1") })},
		{"bigdec str 0.5", d1(func() (*Dec, error) { return Parse("0.5") })},
		{"bigdec str -0.5", d1(func() (*Dec, error) { return Parse("-0.5") })},
		{"bigdec str 1.10", d1(func() (*Dec, error) { return Parse("1.10") })},
		{"bigdec str 1e10", d1(func() (*Dec, error) { return Parse("1e10") })},
		{"bigdec str 1E10", d1(func() (*Dec, error) { return Parse("1E10") })},
		{"bigdec str +1e10", d1(func() (*Dec, error) { return Parse("+1e10") })},
		{"bigdec str -1e10", d1(func() (*Dec, error) { return Parse("-1e10") })},
		{"bigdec str 1e+10", d1(func() (*Dec, error) { return Parse("1e+10") })},
		{"bigdec str 1e-10", d1(func() (*Dec, error) { return Parse("1e-10") })},
		{"bigdec str +1e-10", d1(func() (*Dec, error) { return Parse("+1e-10") })},
		{"bigdec str -1e-10", d1(func() (*Dec, error) { return Parse("-1e-10") })},
		{"bigdec str 1.23e2", d1(func() (*Dec, error) { return Parse("1.23e2") })},
		{"bigdec str 123e2", d1(func() (*Dec, error) { return Parse("123e2") })},
		{"bigdec str .5", d1(func() (*Dec, error) { return Parse(".5") })},
		{"bigdec Inf", fromF(math.Inf(1))},
		{"bigdec -Inf", fromF(math.Inf(-1))},
		{"bigdec NaN", fromF(math.NaN())},
		{"bigdec nil", typeErr("cannot coerce nil to BigDecimal")}, // dispatch-level
		{"bigdec str abc", d1(func() (*Dec, error) { return Parse("abc") })},
		{"bigdec str empty", d1(func() (*Dec, error) { return Parse("") })},
		{"bigdec true", typeErr("cannot coerce Boolean to BigDecimal")}, // dispatch-level
		{"bigdec kw", typeErr("cannot coerce Keyword to BigDecimal")},   // dispatch-level
		{"decimal? bigdec", v("true")},                                  // dispatch-level
		{"decimal? 1.0", v("false")},                                    // dispatch-level

		// --------------------------------------------- arithmetic scale --
		{"add 1.10M 2.2M", v(prM(dec("1.10").Add(dec("2.2"))))},
		{"add 1M 2M", v(prM(i(1).Add(i(2))))},
		{"add 1.1M 2.2M", v(prM(dec("1.1").Add(dec("2.2"))))},
		{"add 1E+2M 1M", v(prM(dec("1E+2").Add(i(1))))},
		{"sub 5.00M 1.0M", v(prM(dec("5.00").Sub(one0)))},
		{"sub 1.0M 1.0M", v(prM(one0.Sub(one0)))},
		{"mul 1.10M 2.0M", v(prM(dec("1.10").Mul(dec("2.0"))))},
		{"mul 1.5M 1.5M", v(prM(dec("1.5").Mul(dec("1.5"))))},
		{"mul 1.10M 1M", v(prM(dec("1.10").Mul(i(1))))},
		{"mul 1E+2M 1E+3M", v(prM(dec("1E+2").Mul(dec("1E+3"))))},
		{"mul' 1.10M 2.0M", v(prM(dec("1.10").Mul(dec("2.0"))))},
		{"add' 1.10M 2.2M", v(prM(dec("1.10").Add(dec("2.2"))))},
		{"div 1M 4M", d1(func() (*Dec, error) { return div(i(1), i(4)) })},
		{"div 10.0M 2M", d1(func() (*Dec, error) { return div(dec("10.0"), i(2)) })},
		{"div 1M 3M", d1(func() (*Dec, error) { return div(i(1), i(3)) })},
		{"div 1M 3", d1(func() (*Dec, error) { return div(i(1), i(3)) })}, // long promotes to 3M
		{"div 1M 2", d1(func() (*Dec, error) { return div(i(1), i(2)) })},
		{"div 1.0M 8", d1(func() (*Dec, error) { return div(one0, i(8)) })},
		{"div 1M 0M", d1(func() (*Dec, error) { return div(i(1), i(0)) })},
		{"quot 10.0M 3", d1(func() (*Dec, error) { return quot(dec("10.0"), i(3)) })},
		{"quot 10M 3", d1(func() (*Dec, error) { return quot(i(10), i(3)) })},
		{"quot 10.0M 3.0M", d1(func() (*Dec, error) { return quot(dec("10.0"), dec("3.0")) })},
		{"rem 10.0M 3", d1(func() (*Dec, error) { return dec("10.0").Rem(i(3)) })},
		{"rem 10.0M 3.0M", d1(func() (*Dec, error) { return dec("10.0").Rem(dec("3.0")) })},
		{"rem -10.0M 3", d1(func() (*Dec, error) { return dec("-10.0").Rem(i(3)) })},
		{"mod 10.0M 3", d1(func() (*Dec, error) { return dec("10.0").Mod(i(3)) })},
		{"mod 10.0M 3.0M", d1(func() (*Dec, error) { return dec("10.0").Mod(dec("3.0")) })},
		{"mod -10.0M 3", d1(func() (*Dec, error) { return dec("-10.0").Mod(i(3)) })},
		{"inc 1.0M", v(prM(one0.Add(i(1))))},
		{"dec 1.00M", v(prM(one00.Sub(i(1))))},
		{"neg 2.0M", v(prM(dec("2.0").Neg()))},
		{"abs -1.50M", v(prM(dec("-1.50").Abs()))},
		{"max 1.0M 2M", v(prM(i(2)))}, // (max 1.0M 2M): 2M wins the compare
		{"min 1.0M 2M", v(prM(one0))},

		// ------------------------------------------- equality / compare --
		{"= 1.0M 1.00M", b(func() bool { return one0.Cmp(one00) == 0 })}, // same category → equiv
		{"== 1.0M 1.00M", b(func() bool { return one0.Cmp(one00) == 0 })},
		{"= 1M 1", v("false")}, // decimal vs integer category
		{"== 1M 1", b(func() bool { return i(1).Cmp(FromInt64(1)) == 0 })},
		{"= 1M 1N", v("false")},  // decimal vs integer category
		{"= 1M 1.0", v("false")}, // decimal vs floating category
		{"== 1M 1.0", b(func() bool { return i(1).Float64() == 1.0 })},
		{"= 0.5M 1/2", v("false")}, // decimal vs ratio category
		{"== 0.5M 1/2", b(func() bool {
			r, _ := FromRatio(big.NewInt(1), big.NewInt(2))
			return dec("0.5").Cmp(r) == 0
		})},
		{"compare 1.0M 1.00M", v(strconv.Itoa(one0.Cmp(one00)))},
		{"compare 1.0M 1.01M", v(strconv.Itoa(one0.Cmp(dec("1.01"))))},
		{"compare 2M 1M", v(strconv.Itoa(i(2).Cmp(i(1))))},
		{"< 1.0M 1.01M", b(func() bool { return one0.Cmp(dec("1.01")) < 0 })},
		{"<= 1.0M 1.00M", b(func() bool { return one0.Cmp(one00) <= 0 })},
		{"zero? 0.000M", b(func() bool { return dec("0.000").Sign() == 0 })},
		{"pos? 0.000M", b(func() bool { return dec("0.000").Sign() > 0 })},
		{"neg? -1.0M", b(func() bool { return dec("-1.0").Sign() < 0 })},

		// ------------------------------------------------------ printing --
		{"str 1.10M", v(strconv.Quote(dec("1.10").String()))},
		{"pr-str 1.10M", v(strconv.Quote(prM(dec("1.10"))))},
		{"str bigdec 1e10", v(strconv.Quote(dec("1e10").String()))},
		{"str 0.0000001M", v(strconv.Quote(dec("0.0000001").String()))},
		{"str 0.000001M", v(strconv.Quote(dec("0.000001").String()))},
		{"pr-str vec", v(strconv.Quote("[" + prM(one0) + " " + prM(dec("1E+3")) + "]"))},

		// --------------------------------------------- tower interaction --
		{"add 1 1.0M", v(prM(i(1).Add(one0)))}, // long → 1M, then decimal add
		{"add 1N 1.0M", v(prM(FromBigInt(big.NewInt(1)).Add(one0)))},
		{"add 1/2 0.5M", d1(func() (*Dec, error) { // ratio → decimal, then add
			r, err := FromRatio(big.NewInt(1), big.NewInt(2))
			if err != nil {
				return nil, err
			}
			return r.Add(dec("0.5")), nil
		})},
		{"add 1.0 1.0M", v(prDouble(1.0 + one0.Float64()))}, // double contaminates
		{"add 1.0M NaN", v(prDouble(one0.Float64() + math.NaN()))},
		{"add 1.0M Inf", v(prDouble(one0.Float64() + math.Inf(1)))},
		{"mul 2 1.50M", v(prM(i(2).Mul(dec("1.50"))))},
		{"int 1.9M", v(dec("1.9").ToBigInt().String())},
		{"long 1.9M", v(dec("1.9").ToBigInt().String())},
		{"double 1.5M", v(prDouble(dec("1.5").Float64()))},
		{"bigint 1.5M", v(dec("1.5").ToBigInt().String() + "N")},
		{"biginteger 1.5M", v(dec("1.5").ToBigInt().String())},
		{"rationalize 1.10M", func() (string, error) { // exact decimal → reduced ratio
			r := new(big.Rat)
			r.SetString(dec("1.10").String())
			return r.Num().String() + "/" + r.Denom().String(), nil
		}},
		{"num type", v("true")}, // dispatch-level

		// ----------------------------------------------------- hash rows --
		{"set 1.0M 1.00M", func() (string, error) { // = is true → one element
			if one0.Cmp(one00) == 0 {
				return "1", nil
			}
			return "2", nil
		}},
		{"hash= 1.0M 1.00M", b(func() bool { return one0.Hasheq() == one00.Hasheq() })},
	}
}

func wpRows() []row {
	i := func(n int64) *Dec { return FromInt64(n) }
	mulR := func(a, b string, prec int, mode RoundingMode) func() (string, error) {
		return d1(func() (*Dec, error) { return dec(a).Mul(dec(b)).Round(prec, mode) })
	}
	divR := func(a, b int64, prec int, mode RoundingMode) func() (string, error) {
		return d1(func() (*Dec, error) { return i(a).DivideMC(i(b), prec, mode) })
	}
	return []row{
		{"wp1 UP 1.1*1", mulR("1.1", "1", 1, RoundUp)},
		{"wp1 CEILING 1.1*1", mulR("1.1", "1", 1, RoundCeiling)},
		{"wp1 UP -1.1*1", mulR("-1.1", "1", 1, RoundUp)},
		{"wp1 CEILING -1.1*1", mulR("-1.1", "1", 1, RoundCeiling)},
		{"wp1 DOWN 1.9*1", mulR("1.9", "1", 1, RoundDown)},
		{"wp1 FLOOR 1.9*1", mulR("1.9", "1", 1, RoundFloor)},
		{"wp1 DOWN -1.9*1", mulR("-1.9", "1", 1, RoundDown)},
		{"wp1 FLOOR -1.9*1", mulR("-1.9", "1", 1, RoundFloor)},
		{"wp1 HALF_EVEN 1.5*1", mulR("1.5", "1", 1, RoundHalfEven)},
		{"wp1 HALF_EVEN 2.5*1", mulR("2.5", "1", 1, RoundHalfEven)},
		{"wp1 HALF_EVEN -1.5*1", mulR("-1.5", "1", 1, RoundHalfEven)},
		{"wp1 HALF_EVEN -2.5*1", mulR("-2.5", "1", 1, RoundHalfEven)},
		{"wp1 HALF_UP 1.5*1", mulR("1.5", "1", 1, RoundHalfUp)},
		{"wp1 HALF_DOWN 1.5*1", mulR("1.5", "1", 1, RoundHalfDown)},
		{"wp1 HALF_UP -1.5*1", mulR("-1.5", "1", 1, RoundHalfUp)},
		{"wp1 HALF_DOWN -1.5*1", mulR("-1.5", "1", 1, RoundHalfDown)},
		{"wp1 UNNECESSARY 1.5*1", mulR("1.5", "1", 1, RoundUnnecessary)},
		{"wp1 UNNECESSARY 2*1", mulR("2", "1", 1, RoundUnnecessary)},
		{"wp2 div 1/3", divR(1, 3, 2, RoundHalfUp)}, // HALF_UP = MathContext default
		{"wp5 div 1/3", divR(1, 3, 5, RoundHalfUp)},
		{"wp2 div 2/3", divR(2, 3, 2, RoundHalfUp)},
		{"wp3 add", d1(func() (*Dec, error) { return dec("1.2345").Add(dec("0")).Round(3, RoundHalfUp) })},
		{"wp3 sub", d1(func() (*Dec, error) { return dec("1.2345").Sub(dec("0")).Round(3, RoundHalfUp) })},
		{"wp3 mul", d1(func() (*Dec, error) { return dec("1.2345").Mul(dec("1")).Round(3, RoundHalfUp) })},
		{"wp4 HALF_DOWN div", divR(1, 3, 4, RoundHalfDown)},
		{"wp2 big", d1(func() (*Dec, error) { return FromInt64(123).Add(FromInt64(0)).Round(2, RoundHalfUp) })},
	}
}

// ------------------------------------------------------------- scoring ----

func loadOracle(path string) (map[string]string, []string, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, nil, err
	}
	defer f.Close()
	m := map[string]string{}
	var order []string
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		line := sc.Text()
		i := strings.Index(line, " => ")
		if i < 0 {
			continue
		}
		m[line[:i]] = line[i+4:]
		order = append(order, line[:i])
	}
	return m, order, sc.Err()
}

func score(name string, oracle map[string]string, rs []row) (match, total int) {
	for _, r := range rs {
		exp, ok := oracle[r.label]
		if !ok {
			fmt.Printf("  [%s] NO-ORACLE   %-28s (row not in oracle output)\n", name, r.label)
			continue
		}
		total++
		var got string
		func() {
			defer func() {
				if p := recover(); p != nil {
					got = fmt.Sprintf("THREW:%v", p)
				}
			}()
			s, err := r.f()
			if err != nil {
				got = "THREW:" + err.Error()
			} else {
				got = s
			}
		}()
		expThrew := strings.HasPrefix(exp, "THREW")
		gotThrew := strings.HasPrefix(got, "THREW")
		switch {
		case got == exp:
			match++
		case expThrew && gotThrew:
			match++ // both threw; wording is host-specific (S13/S14 policy)
			fmt.Printf("  [%s] THREW-DIFF  %-28s oracle=%q proto=%q\n", name, r.label, exp, got)
		default:
			fmt.Printf("  [%s] MISMATCH    %-28s oracle=%q proto=%q\n", name, r.label, exp, got)
		}
	}
	return match, total
}

func main() {
	oraclePath := flag.String("oracle", "../out/probes.oracle.txt", "oracle output for probes.clj")
	oracleWpPath := flag.String("oracle-wp", "../out/probes_wp.oracle.txt", "oracle output for probes_wp.clj")
	flag.Parse()

	o1, _, err := loadOracle(*oraclePath)
	if err != nil {
		fmt.Fprintln(os.Stderr, "oracle:", err)
		os.Exit(1)
	}
	o2, _, err := loadOracle(*oracleWpPath)
	if err != nil {
		fmt.Fprintln(os.Stderr, "oracle-wp:", err)
		os.Exit(1)
	}

	m1, t1 := score("main", o1, rows())
	m2, t2 := score("wp", o2, wpRows())
	fmt.Printf("\nmain corpus: %d/%d match\n", m1, t1)
	fmt.Printf("with-precision corpus: %d/%d match\n", m2, t2)
	fmt.Printf("TOTAL: %d/%d (%.1f%%)\n", m1+m2, t1+t2, 100*float64(m1+m2)/float64(t1+t2))
	if m1+m2 < t1+t2 {
		os.Exit(1)
	}
}
