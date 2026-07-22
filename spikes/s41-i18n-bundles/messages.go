package main

// messages.go — bundle model, .properties + .edn parsers, the fallback
// resolver, interpolation, and the ICU-subset plural renderer.

import (
	"bufio"
	"fmt"
	"strconv"
	"strings"

	"olympos.io/encoding/edn"
)

// Message is one bundle entry: either a plain Text (which may itself
// carry an inline ICU-subset `{count, plural, ...}` block) or an EDN
// plural-map (Forms). Exactly one is populated.
type Message struct {
	Text  string
	Forms map[PluralCategory]string
}

// Bundle is a flat key→Message map for one (format, locale) file.
type Bundle map[string]Message

// Catalog holds every loaded bundle keyed by locale tag ("" = default,
// "en", "en_US", "fr", ...). Override bundles are merged on top of
// embedded ones per locale, so disk shadows embed key-by-key.
type Catalog map[string]Bundle

// --- .properties parser (java.util.Properties subset) -------------------
//
// java.util.Properties: lines are `key=value` (also `:` and whitespace
// separators); `#` or `!` begin a comment; a trailing `\` continues the
// value onto the next line; keys/values are ISO-8859-1 with `\uXXXX`
// escapes. We implement the common subset: `#`/`!` comments, `=`/`:`
// separators, and `\` line continuation. (A production bri would use
// github.com/magiconair/properties for full fidelity.)
func parseProperties(src string) Bundle {
	b := Bundle{}
	sc := bufio.NewScanner(strings.NewReader(src))
	sc.Buffer(make([]byte, 0, 64*1024), 1024*1024)
	var pending string
	continued := false
	for sc.Scan() {
		line := sc.Text()
		if !continued {
			t := strings.TrimLeft(line, " \t")
			if t == "" || strings.HasPrefix(t, "#") || strings.HasPrefix(t, "!") {
				continue
			}
			line = t
		}
		if strings.HasSuffix(line, "\\") {
			pending += strings.TrimSuffix(line, "\\")
			continued = true
			continue
		}
		full := pending + line
		pending = ""
		continued = false

		sep := strings.IndexAny(full, "=:")
		if sep < 0 {
			continue
		}
		key := strings.TrimSpace(full[:sep])
		val := strings.TrimSpace(full[sep+1:])
		b[key] = Message{Text: val}
	}
	return b
}

// --- .edn parser -------------------------------------------------------
//
// A bundle is `{:keyword "string" ...}`; a value may itself be a plural
// map `{:one "..." :other "..."}`. EDN keywords lose their leading `:`
// as the lookup key ("greeting"), matching how a `.properties` key reads.
func parseEDN(src string) (Bundle, error) {
	var raw map[edn.Keyword]interface{}
	if err := edn.NewDecoder(strings.NewReader(src)).Decode(&raw); err != nil {
		return nil, err
	}
	b := Bundle{}
	for k, v := range raw {
		key := string(k)
		switch vv := v.(type) {
		case string:
			b[key] = Message{Text: vv}
		case map[edn.Keyword]interface{}:
			b[key] = Message{Forms: formsOf(ed05keys(vv))}
		case map[interface{}]interface{}:
			b[key] = Message{Forms: formsOf(vv)}
		default:
			b[key] = Message{Text: fmt.Sprintf("%v", vv)}
		}
	}
	return b, nil
}

// formsOf turns a decoded EDN sub-map (keyword->string) into a plural
// forms table. Keys may arrive as edn.Keyword regardless of the outer
// map's static type.
func formsOf(vv map[interface{}]interface{}) map[PluralCategory]string {
	forms := map[PluralCategory]string{}
	for fk, fv := range vv {
		s, ok := fv.(string)
		if !ok {
			continue
		}
		var cat string
		switch k := fk.(type) {
		case edn.Keyword:
			cat = string(k)
		default:
			cat = fmt.Sprintf("%v", k)
		}
		forms[PluralCategory(cat)] = s
	}
	return forms
}

// ed05keys widens a map[edn.Keyword]interface{} to map[interface{}]interface{}.
func ed05keys(m map[edn.Keyword]interface{}) map[interface{}]interface{} {
	out := make(map[interface{}]interface{}, len(m))
	for k, v := range m {
		out[k] = v
	}
	return out
}

// --- interpolation -----------------------------------------------------

// interpolate replaces `{name}` placeholders from args. Unknown
// placeholders are LEFT VISIBLE (`{name}`) rather than blanked, so a
// missing arg is diagnosable instead of silently empty.
func interpolate(s string, args map[string]string) string {
	var out strings.Builder
	for i := 0; i < len(s); {
		if s[i] == '{' {
			if end := strings.IndexByte(s[i:], '}'); end > 0 {
				name := s[i+1 : i+end]
				if !strings.ContainsAny(name, "{ ,") { // a bare {name}
					if v, ok := args[name]; ok {
						out.WriteString(v)
					} else {
						out.WriteString(s[i : i+end+1]) // leave {name} visible
					}
					i += end + 1
					continue
				}
			}
		}
		out.WriteByte(s[i])
		i++
	}
	return out.String()
}

// --- ICU-subset plural renderer ---------------------------------------
//
// Renders `{arg, plural, cat {sub} cat {sub} ...}` blocks: pick the CLDR
// category for args[arg] in `lang`, substitute `#` with the count, then
// run ordinary `{name}` interpolation on the chosen sub-message. Nested
// plurals are not supported (named gap vs full ICU). Everything outside
// a plural block passes through to interpolate().
func renderICU(s, lang string, args map[string]string) string {
	i := strings.Index(s, ", plural,")
	if i < 0 {
		return interpolate(s, args)
	}
	// Walk back to the '{' that opens this block, capturing the arg name.
	open := strings.LastIndexByte(s[:i], '{')
	if open < 0 {
		return interpolate(s, args)
	}
	arg := strings.TrimSpace(s[open+1 : i])
	// Find the matching close brace for the whole plural block.
	depth := 0
	close := -1
	for j := open; j < len(s); j++ {
		switch s[j] {
		case '{':
			depth++
		case '}':
			depth--
			if depth == 0 {
				close = j
			}
		}
		if close >= 0 {
			break
		}
	}
	if close < 0 {
		return interpolate(s, args)
	}
	body := s[i+len(", plural,") : close] // "  cat {sub} cat {sub} "
	forms := parsePluralForms(body)

	n, _ := strconv.Atoi(args[arg])
	chosen, ok := selectPlural(lang, n, forms)
	if !ok {
		chosen = fmt.Sprintf("{%s}", arg)
	}
	chosen = strings.ReplaceAll(chosen, "#", strconv.Itoa(n)) // ICU '#'
	rendered := interpolate(chosen, args)

	// Reassemble: prefix + rendered + (recurse on the suffix).
	return interpolate(s[:open], args) + rendered + renderICU(s[close+1:], lang, args)
}

// parsePluralForms turns "zero {no items} one {# item} other {# items}"
// into {zero:"no items", one:"# item", other:"# items"}.
func parsePluralForms(body string) map[PluralCategory]string {
	forms := map[PluralCategory]string{}
	i := 0
	for i < len(body) {
		// skip whitespace
		for i < len(body) && (body[i] == ' ' || body[i] == '\t' || body[i] == '\n') {
			i++
		}
		if i >= len(body) {
			break
		}
		// read category word up to '{'
		start := i
		for i < len(body) && body[i] != '{' {
			i++
		}
		cat := strings.TrimSpace(body[start:i])
		if i >= len(body) {
			break
		}
		// read braced sub-message with depth
		depth := 0
		j := i
		for ; j < len(body); j++ {
			if body[j] == '{' {
				depth++
			} else if body[j] == '}' {
				depth--
				if depth == 0 {
					break
				}
			}
		}
		sub := body[i+1 : j]
		if cat != "" {
			forms[PluralCategory(cat)] = sub
		}
		i = j + 1
	}
	return forms
}
