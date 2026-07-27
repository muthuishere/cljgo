// Package cljson is the Go host codec behind clojure.data.json (ADR 0097):
// a hand-rolled JSON scanner + writer bridging Go/JSON text and cljgo lang.*
// values (maps, vectors, strings, longs, doubles, bigint, bigdec, bool, nil).
// It is the PERFORMANCE tier-1 realization of data.json's mandate: the pure
// -Clojure spike (BMP-only, slow-path) is rejected — the hot scan/emit path is
// a Go primitive under a thin Clojure API (core/data_json.cljg), the
// clojure.string model (MANDATE A).
//
// clojure.data.json is registered as an OPT-IN, separately-linked namespace
// (ADR 0097 / ADR 0074 mechanism): a compiled binary that never
// (require 'clojure.data.json) links ZERO bytes of this codec (MANDATE B). Its
// shims live in THIS isolated package, whose init() registers the installer
// via bri.RegisterInstaller — so bri.InstallShimsInto resolves the private
// -json-read / -json-write vars exactly like every other namespace once this
// package is linked (the interpreter path via pkg/briloader's blank import; the
// AOT path via the emitter blank-importing pkg/briaot/cljjson only when the app
// requires the namespace).
//
// Like the rest of bri's Go half this package must NOT import pkg/eval — it
// links into AOT binaries. It imports only pkg/lang (the value model) and
// pkg/bri (RegisterInstaller). Astral-plane text (runes > U+FFFF) round-trips
// byte-identically to the JVM: read decodes \uXXXX\uXXXX surrogate pairs into
// one rune, write re-encodes with utf16.EncodeRune when :escape-unicode is on
// (the pure draft's known divergence — closed here).
package cljson

import (
	"bufio"
	"fmt"
	"io"
	"math/big"
	"strconv"
	"strings"
	"unicode/utf16"
	"unicode/utf8"

	"github.com/muthuishere/cljgo/pkg/bri"
	"github.com/muthuishere/cljgo/pkg/lang"
)

// init registers the shim installer so it is present exactly when this
// package is linked (ADR 0074): the interpreter blank-imports it in
// pkg/briloader; the AOT sub-package pkg/briaot/cljjson blank-imports it from
// its generated provider.go, which the emitter pulls in only when the app
// requires clojure.data.json.
func init() { bri.RegisterInstaller("clojure.data.json", installJSONShims) }

// installJSONShims interns clojure.data.json's private Go primitives:
// -json-read (text/reader -> cljgo value) and -json-write (cljgo value ->
// text/writer). The thin Clojure layer owns the option keys and hands each
// primitive an already-resolved option map.
func installJSONShims(def func(name string, fn func(args ...any) any)) {
	// -json-read (input opts) -> parsed value. input is a String or a Go
	// io.Reader (a stream, e.g. *in*); exactly ONE JSON value is consumed,
	// the rest of the stream left intact (data.json read semantics). opts:
	//   :bigdec     parse decimal literals as BigDecimal (else double)
	//   :key-fn     applied to each object key string
	//   :value-fn   applied as (value-fn key value) to each object value
	//   :eof-error? throw on empty input (default true)
	//   :eof-value  value returned at EOF when :eof-error? is false
	def("-json-read", func(args ...any) any {
		if len(args) != 2 {
			panic(lang.NewError(fmt.Sprintf("-json-read expects 2 args (input opts), got %d", len(args))))
		}
		opts := args[1]
		rd := &jsonReader{
			bigdec:  truthy(lang.Get(opts, kw("bigdec"))),
			keyFn:   fnOrNil(lang.Get(opts, kw("key-fn"))),
			valueFn: fnOrNil(lang.Get(opts, kw("value-fn"))),
		}
		switch in := args[0].(type) {
		case string:
			// read-str's hot path: scan the string's bytes by index, with no
			// bufio per-byte call overhead.
			rd.r = &stringSource{s: in}
		case io.Reader:
			rd.r = bufio.NewReader(in)
		default:
			panic(lang.NewError(fmt.Sprintf("clojure.data.json/read: expected a string or reader, got %s", lang.PrintString(args[0]))))
		}
		if rd.skipWS() {
			// Nothing but whitespace/EOF.
			eofError := true
			if containsKey(opts, kw("eof-error?")) {
				eofError = truthy(lang.Get(opts, kw("eof-error?")))
			}
			if eofError {
				panic(lang.NewError("JSON error (end-of-file)"))
			}
			return lang.Get(opts, kw("eof-value"))
		}
		return rd.readValue()
	})

	// -json-write (value opts writer) -> string when writer is nil, else nil
	// (the JSON is streamed to the Go io.Writer). opts:
	//   :escape-unicode        \uXXXX-escape non-ASCII (default true)
	//   :escape-js-separators   escape U+2028/U+2029 (default true)
	//   :escape-slash           escape '/' as '\/' (default true)
	//   :indent                 pretty-print, 2-space (default false)
	//   :key-fn                 applied to each map key before writing
	//   :value-fn               applied as (value-fn key value) to each value
	//   :default-write-fn       applied to a value of unknown type; its result
	//                           is written as JSON (else an error is thrown)
	def("-json-write", func(args ...any) any {
		if len(args) != 3 {
			panic(lang.NewError(fmt.Sprintf("-json-write expects 3 args (value opts writer), got %d", len(args))))
		}
		opts := args[1]
		w := &jsonWriter{
			escapeUnicode:      optBool(opts, "escape-unicode", true),
			escapeJSSeparators: optBool(opts, "escape-js-separators", true),
			escapeSlash:        optBool(opts, "escape-slash", true),
			indent:             truthy(lang.Get(opts, kw("indent"))),
			keyFn:              fnOrNil(lang.Get(opts, kw("key-fn"))),
			valueFn:            fnOrNil(lang.Get(opts, kw("value-fn"))),
			defaultWriteFn:     fnOrNil(lang.Get(opts, kw("default-write-fn"))),
		}
		w.writeValue(args[0], 0)
		if args[2] == nil {
			return w.b.String()
		}
		out, ok := args[2].(io.Writer)
		if !ok {
			panic(lang.NewError(fmt.Sprintf("clojure.data.json/write: expected a writer, got %s", lang.PrintString(args[2]))))
		}
		if _, err := io.WriteString(out, w.b.String()); err != nil {
			panic(lang.NewError("clojure.data.json/write: " + err.Error()))
		}
		return nil
	})
}

// --- reader ------------------------------------------------------------------

// byteSource is the minimal byte cursor the scanner drives — satisfied by
// *bufio.Reader (a stream) and by *stringSource (the read-str fast path).
type byteSource interface {
	ReadByte() (byte, error)
	UnreadByte() error
}

// stringSource scans a string's bytes by index — the read-str fast path,
// avoiding *bufio.Reader's per-byte call overhead.
type stringSource struct {
	s string
	i int
}

func (b *stringSource) ReadByte() (byte, error) {
	if b.i >= len(b.s) {
		return 0, io.EOF
	}
	c := b.s[b.i]
	b.i++
	return c, nil
}

func (b *stringSource) UnreadByte() error {
	if b.i > 0 {
		b.i--
	}
	return nil
}

type jsonReader struct {
	r       byteSource
	bigdec  bool
	keyFn   any
	valueFn any
}

// skipWS consumes JSON whitespace; returns true at EOF (nothing left).
func (rd *jsonReader) skipWS() bool {
	for {
		b, err := rd.r.ReadByte()
		if err != nil {
			return true
		}
		switch b {
		case ' ', '\t', '\n', '\r':
			continue
		default:
			_ = rd.r.UnreadByte()
			return false
		}
	}
}

// peek returns the next non-whitespace byte without consuming it; the bool is
// false at EOF.
func (rd *jsonReader) peek() (byte, bool) {
	if rd.skipWS() {
		return 0, false
	}
	b, err := rd.r.ReadByte()
	if err != nil {
		return 0, false
	}
	_ = rd.r.UnreadByte()
	return b, true
}

func (rd *jsonReader) readValue() any {
	b, ok := rd.peek()
	if !ok {
		panic(lang.NewError("JSON error (end-of-file)"))
	}
	switch {
	case b == '{':
		return rd.readObject()
	case b == '[':
		return rd.readArray()
	case b == '"':
		return rd.readString()
	case b == 't' || b == 'f':
		return rd.readBool()
	case b == 'n':
		return rd.readNull()
	case b == '-' || (b >= '0' && b <= '9'):
		return rd.readNumber()
	default:
		panic(lang.NewError(fmt.Sprintf("JSON error (unexpected character): %c", b)))
	}
}

func (rd *jsonReader) mustByte() byte {
	b, err := rd.r.ReadByte()
	if err != nil {
		panic(lang.NewError("JSON error (end-of-file)"))
	}
	return b
}

func (rd *jsonReader) readObject() any {
	rd.mustByte() // '{'
	var kvs []any
	if b, ok := rd.peek(); ok && b == '}' {
		rd.mustByte()
		return lang.NewMap()
	}
	for {
		if b, ok := rd.peek(); !ok {
			panic(lang.NewError("JSON error (EOF in object)"))
		} else if b != '"' {
			panic(lang.NewError(fmt.Sprintf("JSON error (non-string key in object), found `%c`, expected `\"`", b)))
		}
		rawKey := rd.readString()
		if b, ok := rd.peek(); !ok {
			panic(lang.NewError("JSON error (EOF in object)"))
		} else if b != ':' {
			panic(lang.NewError("JSON error (missing `:` in object)"))
		}
		rd.mustByte() // ':'
		var key any = rawKey
		if rd.keyFn != nil {
			key = lang.Apply1(rd.keyFn, key)
		}
		val := rd.readValue()
		if rd.valueFn != nil {
			val = lang.Apply2(rd.valueFn, key, val)
		}
		kvs = append(kvs, key, val)
		b, ok := rd.peek()
		if !ok {
			panic(lang.NewError("JSON error (EOF in object)"))
		}
		switch b {
		case ',':
			rd.mustByte()
			continue
		case '}':
			rd.mustByte()
			return lang.NewMap(kvs...)
		default:
			panic(lang.NewError(fmt.Sprintf("JSON error (invalid object): %c", b)))
		}
	}
}

func (rd *jsonReader) readArray() any {
	rd.mustByte() // '['
	var out []any
	if b, ok := rd.peek(); ok && b == ']' {
		rd.mustByte()
		return lang.NewVectorOwning(out)
	}
	for {
		out = append(out, rd.readValue())
		b, ok := rd.peek()
		if !ok {
			panic(lang.NewError("JSON error (EOF in array)"))
		}
		switch b {
		case ',':
			rd.mustByte()
			continue
		case ']':
			rd.mustByte()
			return lang.NewVectorOwning(out)
		default:
			panic(lang.NewError("JSON error (invalid array)"))
		}
	}
}

func (rd *jsonReader) readString() string {
	rd.mustByte() // opening '"'
	var sb strings.Builder
	for {
		b := rd.mustByte()
		switch b {
		case '"':
			return sb.String()
		case '\\':
			rd.readEscape(&sb)
		default:
			if b < 0x20 {
				panic(lang.NewError("JSON error (invalid string)"))
			}
			sb.WriteByte(b) // raw UTF-8 byte, passes through untouched
		}
	}
}

func (rd *jsonReader) readEscape(sb *strings.Builder) {
	b := rd.mustByte()
	switch b {
	case '"':
		sb.WriteByte('"')
	case '\\':
		sb.WriteByte('\\')
	case '/':
		sb.WriteByte('/')
	case 'b':
		sb.WriteByte('\b')
	case 'f':
		sb.WriteByte('\f')
	case 'n':
		sb.WriteByte('\n')
	case 'r':
		sb.WriteByte('\r')
	case 't':
		sb.WriteByte('\t')
	case 'u':
		r := rune(rd.readHex4())
		// A high surrogate must pair with a following \uXXXX low surrogate to
		// form one astral-plane rune (the JVM-identical round-trip).
		if utf16.IsSurrogate(r) {
			if b0, err := rd.r.ReadByte(); err == nil {
				if b0 == '\\' {
					if b1, err := rd.r.ReadByte(); err == nil && b1 == 'u' {
						r2 := rune(rd.readHex4())
						if dec := utf16.DecodeRune(r, r2); dec != utf8.RuneError {
							sb.WriteRune(dec)
							return
						}
						sb.WriteRune(utf8.RuneError)
						sb.WriteRune(utf8.RuneError)
						return
					}
				}
				_ = rd.r.UnreadByte()
			}
			sb.WriteRune(utf8.RuneError)
			return
		}
		sb.WriteRune(r)
	default:
		panic(lang.NewError(fmt.Sprintf("JSON error (unexpected escape): %c", b)))
	}
}

func (rd *jsonReader) readHex4() int {
	var v int
	for i := 0; i < 4; i++ {
		b := rd.mustByte()
		var d int
		switch {
		case b >= '0' && b <= '9':
			d = int(b - '0')
		case b >= 'a' && b <= 'f':
			d = int(b-'a') + 10
		case b >= 'A' && b <= 'F':
			d = int(b-'A') + 10
		default:
			panic(lang.NewError("JSON error (invalid unicode escape)"))
		}
		v = v*16 + d
	}
	return v
}

func (rd *jsonReader) readBool() any {
	if b, _ := rd.peek(); b == 't' {
		rd.expect("true")
		return true
	}
	rd.expect("false")
	return false
}

func (rd *jsonReader) readNull() any {
	rd.expect("null")
	return nil
}

func (rd *jsonReader) expect(word string) {
	for i := 0; i < len(word); i++ {
		if rd.mustByte() != word[i] {
			panic(lang.NewError("JSON error (unexpected character): " + word[:1]))
		}
	}
}

// readNumber scans a JSON number literal and returns a long (int64) when it is
// an integer that fits, a BigInt when an integer that overflows, a double for
// a fractional/exponent literal, or a BigDecimal when :bigdec is set.
func (rd *jsonReader) readNumber() any {
	var lit strings.Builder
	isFloat := false
	if b, _ := rd.peek(); b == '-' {
		lit.WriteByte(rd.mustByte())
	}
	intStart := lit.Len()
	rd.scanDigits(&lit)
	// JSON forbids a leading zero on a multi-digit integer part (01, -00).
	if intDigits := lit.String()[intStart:]; len(intDigits) > 1 && intDigits[0] == '0' {
		panic(lang.NewError("JSON error (invalid number literal)"))
	}
	if b, err := rd.r.ReadByte(); err == nil {
		if b == '.' {
			isFloat = true
			lit.WriteByte(b)
			rd.scanDigits(&lit)
		} else {
			_ = rd.r.UnreadByte()
		}
	}
	if b, err := rd.r.ReadByte(); err == nil {
		if b == 'e' || b == 'E' {
			isFloat = true
			lit.WriteByte(b)
			if s, err := rd.r.ReadByte(); err == nil {
				if s == '+' || s == '-' {
					lit.WriteByte(s)
				} else {
					_ = rd.r.UnreadByte()
				}
			}
			rd.scanDigits(&lit)
		} else {
			_ = rd.r.UnreadByte()
		}
	}
	s := lit.String()
	if s == "" || s == "-" {
		panic(lang.NewError("JSON error (invalid number literal)"))
	}
	if !isFloat {
		if n, err := strconv.ParseInt(s, 10, 64); err == nil {
			return n
		}
		if bi, err := lang.NewBigInt(s); err == nil {
			return bi
		}
		panic(lang.NewError("JSON error (invalid number literal)"))
	}
	if rd.bigdec {
		if bd, err := lang.NewBigDecimal(s); err == nil {
			return bd
		}
		panic(lang.NewError("JSON error (invalid number literal)"))
	}
	f, err := strconv.ParseFloat(s, 64)
	if err != nil {
		panic(lang.NewError("JSON error (invalid number literal)"))
	}
	return f
}

// scanDigits appends the run of ASCII digits at the cursor to lit; a number
// with no digit where one is required is an invalid literal.
func (rd *jsonReader) scanDigits(lit *strings.Builder) {
	n := 0
	for {
		b, err := rd.r.ReadByte()
		if err != nil {
			break
		}
		if b < '0' || b > '9' {
			_ = rd.r.UnreadByte()
			break
		}
		lit.WriteByte(b)
		n++
	}
	if n == 0 {
		panic(lang.NewError("JSON error (invalid number literal)"))
	}
}

// --- writer ------------------------------------------------------------------

type jsonWriter struct {
	b                  strings.Builder
	escapeUnicode      bool
	escapeJSSeparators bool
	escapeSlash        bool
	indent             bool
	keyFn              any
	valueFn            any
	defaultWriteFn     any
}

func (w *jsonWriter) writeValue(v any, depth int) {
	switch x := v.(type) {
	case nil:
		w.b.WriteString("null")
	case bool:
		if x {
			w.b.WriteString("true")
		} else {
			w.b.WriteString("false")
		}
	case string:
		w.writeString(x)
	case lang.Keyword:
		w.writeString(x.Name())
	case *lang.Symbol:
		w.writeString(x.Name())
	case int64:
		w.b.WriteString(strconv.FormatInt(x, 10))
	case int:
		w.b.WriteString(strconv.Itoa(x))
	case float64:
		w.writeFloat(x)
	case float32:
		w.writeFloat(float64(x))
	case *lang.BigInt:
		w.b.WriteString(x.ToBigInteger().String())
	case *big.Int:
		w.b.WriteString(x.String())
	case *lang.BigDecimal:
		w.b.WriteString(x.String())
	case *lang.Ratio:
		// data.json writes a ratio as its double value (e.g. 1/3 -> 0.333…).
		w.writeFloat(ratioFloat(x))
	default:
		if w.writeComposite(v, depth) {
			return
		}
		if w.defaultWriteFn != nil {
			w.writeValue(lang.Apply1(w.defaultWriteFn, v), depth)
			return
		}
		panic(lang.NewError(fmt.Sprintf("Don't know how to write JSON of %s", lang.PrintString(v))))
	}
}

// writeComposite handles the collection types (maps and sequential values);
// returns false when v is not a collection this writer renders.
func (w *jsonWriter) writeComposite(v any, depth int) bool {
	if m, ok := v.(lang.IPersistentMap); ok {
		w.writeObject(m, depth)
		return true
	}
	switch v.(type) {
	case lang.IPersistentVector, lang.ISeq, lang.IPersistentList, lang.IPersistentSet:
		w.writeArray(v, depth)
		return true
	}
	// A seqable that is not one of the concrete cases above (e.g. a lazy
	// range) is written as an array.
	if _, ok := v.(lang.Seqable); ok {
		w.writeArray(v, depth)
		return true
	}
	return false
}

func (w *jsonWriter) writeObject(m lang.IPersistentMap, depth int) {
	if lang.Seq(m) == nil {
		w.b.WriteString("{}")
		return
	}
	w.b.WriteByte('{')
	first := true
	for s := lang.Seq(m); s != nil; s = lang.Next(s) {
		var k, val any
		switch e := lang.First(s).(type) {
		case lang.IMapEntry:
			// The common path: a map seq yields IMapEntry — read key/value
			// directly, allocating no per-entry seq.
			k, val = e.Key(), e.Val()
		default:
			es := lang.Seq(e)
			k = lang.First(es)
			val = lang.First(lang.Next(es))
		}
		if !first {
			w.b.WriteByte(',')
		}
		first = false
		w.newlineIndent(depth + 1)
		wk := k
		if w.keyFn != nil {
			wk = lang.Apply1(w.keyFn, k)
		}
		w.writeKey(wk)
		w.b.WriteByte(':')
		if w.indent {
			w.b.WriteByte(' ')
		}
		wv := val
		if w.valueFn != nil {
			wv = lang.Apply2(w.valueFn, wk, val)
		}
		w.writeValue(wv, depth+1)
	}
	w.newlineIndent(depth)
	w.b.WriteByte('}')
}

func (w *jsonWriter) writeArray(v any, depth int) {
	s := lang.Seq(v)
	if s == nil {
		w.b.WriteString("[]")
		return
	}
	w.b.WriteByte('[')
	first := true
	for ; s != nil; s = lang.Next(s) {
		if !first {
			w.b.WriteByte(',')
		}
		first = false
		w.newlineIndent(depth + 1)
		w.writeValue(lang.First(s), depth+1)
	}
	w.newlineIndent(depth)
	w.b.WriteByte(']')
}

// newlineIndent emits a newline + 2*depth spaces when :indent is on; nothing
// otherwise (compact output).
func (w *jsonWriter) newlineIndent(depth int) {
	if !w.indent {
		return
	}
	w.b.WriteByte('\n')
	for i := 0; i < depth; i++ {
		w.b.WriteString("  ")
	}
}

// writeKey renders a map key as a JSON string. Keywords/symbols use their
// name; strings pass through; other scalars are coerced to their textual form
// (data.json coerces, e.g. the integer key 1 -> "1").
func (w *jsonWriter) writeKey(k any) {
	switch x := k.(type) {
	case string:
		w.writeString(x)
	case lang.Keyword:
		w.writeString(x.Name())
	case *lang.Symbol:
		w.writeString(x.Name())
	default:
		w.writeString(lang.PrintString(k))
	}
}

func (w *jsonWriter) writeFloat(f float64) {
	// Match Clojure/JVM double printing: shortest round-trippable form with a
	// decimal point (strconv 'g' without a forced point can drop the ".0").
	s := strconv.FormatFloat(f, 'g', -1, 64)
	if !strings.ContainsAny(s, ".eE") {
		s += ".0"
	}
	w.b.WriteString(s)
}

func (w *jsonWriter) writeString(s string) {
	w.b.WriteByte('"')
	for _, r := range s {
		switch r {
		case '"':
			w.b.WriteString("\\\"")
		case '\\':
			w.b.WriteString("\\\\")
		case '/':
			if w.escapeSlash {
				w.b.WriteString("\\/")
			} else {
				w.b.WriteByte('/')
			}
		case '\b':
			w.b.WriteString("\\b")
		case '\f':
			w.b.WriteString("\\f")
		case '\n':
			w.b.WriteString("\\n")
		case '\r':
			w.b.WriteString("\\r")
		case '\t':
			w.b.WriteString("\\t")
		case '\u2028', '\u2029':
			if w.escapeJSSeparators {
				w.writeUnicodeEscape(r)
			} else {
				w.b.WriteRune(r)
			}
		default:
			switch {
			case r < 0x20:
				w.writeUnicodeEscape(r)
			case r < 0x80:
				w.b.WriteByte(byte(r))
			case w.escapeUnicode:
				w.writeUnicodeEscape(r)
			default:
				w.b.WriteRune(r)
			}
		}
	}
	w.b.WriteByte('"')
}

// writeUnicodeEscape emits \uXXXX for a BMP rune, or a \uXXXX\uXXXX surrogate
// pair for an astral rune (> U+FFFF) — the JVM-identical astral escape.
func (w *jsonWriter) writeUnicodeEscape(r rune) {
	if r > 0xFFFF {
		hi, lo := utf16.EncodeRune(r)
		w.writeU16(uint16(hi))
		w.writeU16(uint16(lo))
		return
	}
	w.writeU16(uint16(r))
}

const lowerHex = "0123456789abcdef"

// writeU16 appends a single \uXXXX escape without fmt overhead (the hot path
// when :escape-unicode is on).
func (w *jsonWriter) writeU16(u uint16) {
	w.b.WriteByte('\\')
	w.b.WriteByte('u')
	w.b.WriteByte(lowerHex[(u>>12)&0xF])
	w.b.WriteByte(lowerHex[(u>>8)&0xF])
	w.b.WriteByte(lowerHex[(u>>4)&0xF])
	w.b.WriteByte(lowerHex[u&0xF])
}

// --- helpers -----------------------------------------------------------------

func kw(name string) lang.Keyword { return lang.NewKeyword(name) }

func truthy(v any) bool { return v != nil && v != false }

// optBool reads a boolean option that defaults to def when the key is ABSENT;
// a present nil/false means false (an explicit :escape-slash false is off).
func optBool(opts any, name string, def bool) bool {
	if !containsKey(opts, kw(name)) {
		return def
	}
	return truthy(lang.Get(opts, kw(name)))
}

func containsKey(opts any, k any) bool {
	if m, ok := opts.(lang.IPersistentMap); ok {
		return m.ContainsKey(k)
	}
	return false
}

// fnOrNil returns v unless it is nil (an absent option). The option values it
// guards (:key-fn/:value-fn/:default-write-fn) are callables dispatched via
// lang.Apply*.
func fnOrNil(v any) any {
	if v == nil {
		return nil
	}
	return v
}

// ratioFloat is the double value of a Ratio (num/den), matching how data.json
// coerces a ratio for output.
func ratioFloat(r *lang.Ratio) float64 {
	f, _ := new(big.Rat).SetFrac(r.Numerator(), r.Denominator()).Float64()
	return f
}
