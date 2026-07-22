package main

// i18n.go — locale resolution, the embed+override loader, and the `t`
// translate entrypoint that ties parsing, fallback, interpolation and
// plurals together. This is the Go core a `bri.i18n` cljg layer sits on.

import (
	"io/fs"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

// I18n is a loaded, ready-to-query message store.
type I18n struct {
	cat           Catalog
	defaultLocale string
}

// localeChain expands "en_US" -> ["en_US", "en", ""] (ResourceBundle
// order: most specific first, empty = the default bundle last).
func localeChain(locale string) []string {
	locale = strings.ReplaceAll(locale, "-", "_") // accept en-US or en_US
	parts := strings.Split(locale, "_")
	chain := []string{}
	for i := len(parts); i >= 1; i-- {
		chain = append(chain, strings.Join(parts[:i], "_"))
	}
	chain = append(chain, "") // default bundle
	// de-dup (e.g. "en" requested -> ["en","", ""])
	seen := map[string]bool{}
	out := chain[:0]
	for _, c := range chain {
		if !seen[c] {
			seen[c] = true
			out = append(out, c)
		}
	}
	return out
}

// langOf returns the language subtag used for plural rules ("en_US"->"en").
func langOf(locale string) string {
	locale = strings.ReplaceAll(locale, "-", "_")
	return strings.Split(locale, "_")[0]
}

// localeOfFile extracts the locale tag from a bundle filename:
// messages.properties -> "", messages_en.edn -> "en",
// messages_en_US.properties -> "en_US".
func localeOfFile(name string) (tag string, ok bool) {
	base := filepath.Base(name)
	ext := filepath.Ext(base)
	if ext != ".properties" && ext != ".edn" {
		return "", false
	}
	stem := strings.TrimSuffix(base, ext)
	if stem == "messages" {
		return "", true
	}
	if rest, found := strings.CutPrefix(stem, "messages_"); found {
		return rest, true
	}
	return "", false
}

// mergeFile parses one bundle file's bytes into the catalog under its
// locale tag, letting later calls shadow earlier keys (override wins).
func (m *I18n) mergeFile(name string, data []byte) error {
	tag, ok := localeOfFile(name)
	if !ok {
		return nil
	}
	var b Bundle
	var err error
	if strings.HasSuffix(name, ".edn") {
		b, err = parseEDN(string(data))
	} else {
		b = parseProperties(string(data))
	}
	if err != nil {
		return err
	}
	if m.cat[tag] == nil {
		m.cat[tag] = Bundle{}
	}
	for k, v := range b {
		m.cat[tag][k] = v // last writer wins (override shadows embed)
	}
	return nil
}

// Load builds an I18n from an embedded FS (baked into the binary) and,
// optionally, an on-disk override directory that shadows/extends it.
// Embedded is read first, override second, so disk wins key-by-key.
func Load(embedded fs.FS, embedDir, overrideDir, defaultLocale string) (*I18n, error) {
	m := &I18n{cat: Catalog{}, defaultLocale: defaultLocale}

	if embedded != nil {
		err := fs.WalkDir(embedded, embedDir, func(p string, d fs.DirEntry, err error) error {
			if err != nil || d.IsDir() {
				return err
			}
			data, e := fs.ReadFile(embedded, p)
			if e != nil {
				return e
			}
			return m.mergeFile(p, data)
		})
		if err != nil {
			return nil, err
		}
	}

	if overrideDir != "" {
		if entries, err := os.ReadDir(overrideDir); err == nil {
			for _, d := range entries {
				if d.IsDir() {
					continue
				}
				data, e := os.ReadFile(filepath.Join(overrideDir, d.Name()))
				if e != nil {
					return nil, e
				}
				if e := m.mergeFile(d.Name(), data); e != nil {
					return nil, e
				}
			}
		}
	}
	return m, nil
}

// lookup walks the fallback chain for `locale` and returns the first
// bundle that carries `key`.
func (m *I18n) lookup(locale, key string) (Message, bool) {
	for _, tag := range localeChain(locale) {
		if b, ok := m.cat[tag]; ok {
			if msg, ok := b[key]; ok {
				return msg, true
			}
		}
	}
	return Message{}, false
}

// T is the translate primitive. Missing keys return a VISIBLE marker
// (never a crash). `args` supplies interpolation values and, for
// plurals, the `count` operand.
func (m *I18n) T(locale, key string, args map[string]string) string {
	if locale == "" {
		locale = m.defaultLocale
	}
	msg, ok := m.lookup(locale, key)
	if !ok {
		return "⟦missing:" + key + "⟧" // ⟦missing:key⟧
	}
	lang := langOf(locale)

	// EDN plural-map form.
	if msg.Forms != nil {
		n := 0
		if c, ok := args["count"]; ok {
			if v, err := strconv.Atoi(c); err == nil {
				n = v
			}
		}
		chosen, ok := selectPlural(lang, n, msg.Forms)
		if !ok {
			return "⟦plural:" + key + "⟧"
		}
		chosen = strings.ReplaceAll(chosen, "#", strconv.Itoa(n))
		return interpolate(chosen, args)
	}

	// Plain text (may carry an inline ICU-subset plural block).
	return renderICU(msg.Text, lang, args)
}
