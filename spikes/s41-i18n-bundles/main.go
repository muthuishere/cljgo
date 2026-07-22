package main

// main.go — S41 proof harness. Loads embedded bundles (the single-binary
// path) plus a disk override, then asserts each exit criterion and prints
// PASS/FAIL. Throwaway (ADR 0027).

import (
	"embed"
	"fmt"
	"os"
	"strings"
)

// The whole point of criterion 5: locales are baked into the binary at
// build time. In real cljgo this is comptime `embed` (ADR 0021); here it
// is the stdlib directive, which is exactly the same mechanism.
//
//go:embed locales
var embedded embed.FS

var pass, fail int

func check(crit, desc string, got, want string) {
	ok := got == want
	tag := "PASS"
	if !ok {
		tag = "FAIL"
		fail++
	} else {
		pass++
	}
	fmt.Printf("[%s] C%s %s\n        got:  %q\n        want: %q\n", tag, crit, desc, got, want)
}

func main() {
	// ---- criteria 1,2,3,4,5: embedded + disk override -----------------
	m, err := Load(embedded, "locales", "override", "en")
	if err != nil {
		fmt.Println("load error:", err)
		os.Exit(1)
	}

	fmt.Println("== S41 bri.i18n probe ==")
	fmt.Println()

	// C1 — two formats, one lookup.
	// `greeting` comes from .properties; `tagline` lives ONLY in messages.edn.
	// Both resolve through the same T(). (override shadows en greeting, so
	// query the default bundle explicitly via locale "" for the raw form.)
	check("1", ".properties key via T (default locale)",
		mNoOverride(embedded).T("", "greeting", map[string]string{"name": "Muthu"}),
		"Hello, Muthu!")
	check("1", ".edn-only key via same T",
		m.T("en", "tagline", nil),
		"Clojure, hosted on Go.")

	// C2 — fallback chain en_US -> en -> default.
	// `help` exists ONLY in base en; requesting en_US must find it.
	check("2", "en_US falls back to en for base-only key",
		m.T("en_US", "help", nil),
		"Type '?' for help.")
	// most-specific wins: greeting differs in en_US (but override shadows it;
	// use the no-override store to show the en_US-vs-en precedence cleanly).
	check("2", "en_US most-specific bundle wins",
		mNoOverride(embedded).T("en_US", "greeting", map[string]string{"name": "Muthu"}),
		"Hi, Muthu!")
	// app.name lives ONLY in the default bundle -> reached from en_US.
	check("2", "default bundle reached as last resort",
		m.T("en_US", "app.name", nil),
		"cljgo")
	// missing key -> visible marker, not a crash.
	check("2", "missing key returns visible marker",
		m.T("en_US", "does.not.exist", nil),
		"⟦missing:does.not.exist⟧")

	// C3 — interpolation.
	check("3", "named-arg interpolation",
		mNoOverride(embedded).T("en", "greeting", map[string]string{"name": "Muthu"}),
		"Hello, Muthu!")
	check("3", "unsupplied placeholder stays visible",
		mNoOverride(embedded).T("en", "greeting", nil),
		"Hello, {name}!")
	check("3", "EDN (fr) interpolation",
		m.T("fr", "greeting", map[string]string{"name": "Muthu"}),
		"Bonjour, Muthu !")

	// C4 — pluralization, inline ICU-subset (en) and EDN map (fr).
	check("4a", "inline ICU plural: count=0 (explicit zero)",
		m.T("en", "items", map[string]string{"count": "0"}),
		"no items")
	check("4a", "inline ICU plural: count=1 (one, # substituted)",
		m.T("en", "items", map[string]string{"count": "1"}),
		"1 item")
	check("4a", "inline ICU plural: count=5 (other, # substituted)",
		m.T("en", "items", map[string]string{"count": "5"}),
		"5 items")
	check("4b", "EDN plural map (fr): count=1 -> one",
		m.T("fr", "items", map[string]string{"count": "1"}),
		"1 article")
	check("4b", "EDN plural map (fr): count=0 -> one (French rule)",
		m.T("fr", "items", map[string]string{"count": "0"}),
		"0 article")
	check("4b", "EDN plural map (fr): count=5 -> other",
		m.T("fr", "items", map[string]string{"count": "5"}),
		"5 articles")

	// C5 — disk override shadows embedded AND adds a new key.
	check("5", "disk override shadows embedded greeting",
		m.T("en", "greeting", map[string]string{"name": "Muthu"}),
		"[OVERRIDE] Hey Muthu!")
	check("5", "disk override adds a key absent from every embed bundle",
		m.T("en", "banner", nil),
		"Runtime override active.")

	// ---- criterion 6: locale-resolution source sketch -----------------
	fmt.Println()
	fmt.Println("== C6 locale resolution source (precedence sketch) ==")
	demoResolve(m)

	fmt.Println()
	fmt.Printf("== %d passed, %d failed ==\n", pass, fail)
	if fail > 0 {
		os.Exit(1)
	}
}

// mNoOverride loads ONLY the embedded bundles (no disk override) so the
// pure en_US->en precedence and raw .properties values are observable
// without the override shadowing `greeting`.
func mNoOverride(fsys embed.FS) *I18n {
	m, err := Load(fsys, "locales", "", "en")
	if err != nil {
		panic(err)
	}
	return m
}

// resolveLocale is criterion 6: how the *current* locale is chosen.
// Precedence (highest first):
//  1. an explicit call-site locale  (bri.i18n `with-locale`)
//  2. an HTTP `Accept-Language` header (web request scope)
//  3. the app config `APP_LOCALE`     (process default)
//  4. the built-in default locale
//
// Returns the first non-empty source. `with-locale` in cljg would bind a
// dynamic var that this consults as source (1).
func resolveLocale(explicit, acceptLanguage, appLocale, fallback string) string {
	if explicit != "" {
		return explicit
	}
	if tag := parseAcceptLanguage(acceptLanguage); tag != "" {
		return tag
	}
	if appLocale != "" {
		return appLocale
	}
	return fallback
}

// parseAcceptLanguage takes the first (highest-q, left-most) tag and
// normalises `en-US` -> `en_US`. A full impl would sort by q-value.
func parseAcceptLanguage(h string) string {
	if h == "" {
		return ""
	}
	first := strings.TrimSpace(strings.Split(h, ",")[0])
	first = strings.Split(first, ";")[0] // drop ;q=
	first = strings.TrimSpace(first)
	return strings.ReplaceAll(first, "-", "_")
}

func demoResolve(m *I18n) {
	cases := []struct {
		explicit, accept, appLocale, note string
	}{
		{"fr", "en-US,en;q=0.9", "en", "explicit with-locale wins over header+config"},
		{"", "fr-FR,fr;q=0.9", "en", "Accept-Language wins over config"},
		{"", "", "fr", "APP_LOCALE config default"},
		{"", "", "", "built-in fallback (en)"},
	}
	for _, c := range cases {
		loc := resolveLocale(c.explicit, c.accept, c.appLocale, "en")
		out := m.T(loc, "greeting", map[string]string{"name": "Muthu"})
		fmt.Printf("  explicit=%-3q accept=%-18q app=%-4q -> locale=%-6q greeting=%q  (%s)\n",
			c.explicit, c.accept, c.appLocale, loc, out, c.note)
	}
}
