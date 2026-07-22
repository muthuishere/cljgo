package main

// Minimal EDN subset: maps, vectors, strings, keywords, integers, nil,
// booleans. Enough for build.lock.edn and a dep manifest. Deterministic
// emitter (sorted keys) so two machines produce byte-identical bytes.

import (
	"fmt"
	"sort"
	"strconv"
	"strings"
	"unicode"
)

type Kw string

type Val interface{}

// ---------- emit ----------

// EmitEDN renders v deterministically. Maps emit with keys sorted by their
// printed form — this is what makes the lockfile byte-comparable.
func EmitEDN(v Val, indent string) string {
	switch t := v.(type) {
	case nil:
		return "nil"
	case bool:
		return strconv.FormatBool(t)
	case Kw:
		return ":" + string(t)
	case string:
		return strconv.Quote(t)
	case int:
		return strconv.Itoa(t)
	case []Val:
		if len(t) == 0 {
			return "[]"
		}
		inner := indent + " "
		parts := make([]string, len(t))
		for i, e := range t {
			parts[i] = EmitEDN(e, inner)
		}
		return "[" + strings.Join(parts, "\n"+inner) + "]"
	case map[Kw]Val:
		if len(t) == 0 {
			return "{}"
		}
		keys := make([]string, 0, len(t))
		for k := range t {
			keys = append(keys, string(k))
		}
		sort.Strings(keys)
		inner := indent + " "
		parts := make([]string, 0, len(keys))
		for _, k := range keys {
			parts = append(parts, ":"+k+" "+EmitEDN(t[Kw(k)], inner+strings.Repeat(" ", len(k)+2)))
		}
		return "{" + strings.Join(parts, "\n"+inner) + "}"
	}
	panic(fmt.Sprintf("EmitEDN: unsupported %T", v))
}

// ---------- parse ----------

type parser struct {
	s string
	i int
}

func ParseEDN(s string) (Val, error) {
	p := &parser{s: s}
	p.ws()
	v, err := p.value()
	if err != nil {
		return nil, err
	}
	return v, nil
}

func (p *parser) ws() {
	for p.i < len(p.s) {
		c := p.s[p.i]
		if c == ';' {
			for p.i < len(p.s) && p.s[p.i] != '\n' {
				p.i++
			}
			continue
		}
		if c == ',' || unicode.IsSpace(rune(c)) {
			p.i++
			continue
		}
		return
	}
}

func (p *parser) value() (Val, error) {
	if p.i >= len(p.s) {
		return nil, fmt.Errorf("edn: unexpected eof at %d", p.i)
	}
	switch c := p.s[p.i]; {
	case c == '{':
		return p.mapv()
	case c == '[':
		return p.vec()
	case c == '"':
		return p.str()
	case c == ':':
		p.i++
		return Kw(p.token()), nil
	default:
		t := p.token()
		switch t {
		case "nil":
			return nil, nil
		case "true":
			return true, nil
		case "false":
			return false, nil
		}
		if n, err := strconv.Atoi(t); err == nil {
			return n, nil
		}
		return nil, fmt.Errorf("edn: unsupported token %q", t)
	}
}

func (p *parser) token() string {
	start := p.i
	for p.i < len(p.s) {
		c := p.s[p.i]
		if unicode.IsSpace(rune(c)) || strings.ContainsRune("{}[]()\",;", rune(c)) {
			break
		}
		p.i++
	}
	return p.s[start:p.i]
}

func (p *parser) str() (Val, error) {
	start := p.i
	p.i++ // opening quote
	for p.i < len(p.s) {
		if p.s[p.i] == '\\' {
			p.i += 2
			continue
		}
		if p.s[p.i] == '"' {
			p.i++
			return strconv.Unquote(p.s[start:p.i])
		}
		p.i++
	}
	return nil, fmt.Errorf("edn: unterminated string at %d", start)
}

func (p *parser) vec() (Val, error) {
	p.i++ // [
	out := []Val{}
	for {
		p.ws()
		if p.i >= len(p.s) {
			return nil, fmt.Errorf("edn: unterminated vector")
		}
		if p.s[p.i] == ']' {
			p.i++
			return out, nil
		}
		v, err := p.value()
		if err != nil {
			return nil, err
		}
		out = append(out, v)
	}
}

func (p *parser) mapv() (Val, error) {
	p.i++ // {
	out := map[Kw]Val{}
	for {
		p.ws()
		if p.i >= len(p.s) {
			return nil, fmt.Errorf("edn: unterminated map")
		}
		if p.s[p.i] == '}' {
			p.i++
			return out, nil
		}
		k, err := p.value()
		if err != nil {
			return nil, err
		}
		kw, ok := k.(Kw)
		if !ok {
			return nil, fmt.Errorf("edn: map key must be a keyword, got %T", k)
		}
		p.ws()
		v, err := p.value()
		if err != nil {
			return nil, err
		}
		out[kw] = v
	}
}

// ---------- helpers ----------

func mget(m Val, k Kw) Val {
	mm, ok := m.(map[Kw]Val)
	if !ok {
		return nil
	}
	return mm[k]
}

func mstr(m Val, k Kw) string {
	if s, ok := mget(m, k).(string); ok {
		return s
	}
	return ""
}

func mvec(m Val, k Kw) []Val {
	if v, ok := mget(m, k).([]Val); ok {
		return v
	}
	return nil
}

func strs(v []Val) []string {
	out := make([]string, 0, len(v))
	for _, e := range v {
		if s, ok := e.(string); ok {
			out = append(out, s)
		}
	}
	return out
}

func vals(ss []string) []Val {
	out := make([]Val, 0, len(ss))
	for _, s := range ss {
		out = append(out, s)
	}
	return out
}
