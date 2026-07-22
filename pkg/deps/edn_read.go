package deps

// EDN reading via cljgo's own pkg/reader — no second parser (ADR 0052). The
// lock and dependency manifests are ordinary EDN data; we read them as cljgo
// values and navigate with the lang data interfaces. Crucially this is DATA
// only: a dependency's (defn build …) is never evaluated (ADR 0052 decision 5).

import (
	"fmt"
	"os"
	"strings"

	"github.com/muthuishere/cljgo/pkg/lang"
	"github.com/muthuishere/cljgo/pkg/reader"
)

// readEDNFile parses a single top-level EDN form from a file.
func readEDNFile(path string) (any, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	form, err := reader.ReadString(string(b))
	if err != nil {
		return nil, fmt.Errorf("%s: %w", path, err)
	}
	return form, nil
}

// ednGet looks up a keyword key (e.g. "git/sha") in an EDN map.
func ednGet(m any, name string) any {
	l, ok := m.(lang.ILookup)
	if !ok {
		return nil
	}
	return l.ValAt(lang.NewKeyword(name))
}

// ednStr coerces an EDN scalar to a Go string. A leading ':' from a keyword is
// stripped so keyword and string forms read alike.
func ednStr(v any) string {
	if v == nil {
		return ""
	}
	if s, ok := v.(string); ok {
		return s
	}
	return strings.TrimPrefix(fmt.Sprint(v), ":")
}

// ednInt coerces an EDN integer (pkg/reader yields int64) to a Go int.
func ednInt(v any) int {
	switch n := v.(type) {
	case int64:
		return int(n)
	case int:
		return n
	}
	return 0
}

// ednSlice returns the elements of an EDN vector/set/map as a Go slice.
func ednSlice(v any) []any {
	if v == nil {
		return nil
	}
	return lang.ToSlice(v)
}

// ednStrs returns an EDN vector/set of scalars as a []string.
func ednStrs(v any) []string {
	var out []string
	for _, e := range ednSlice(v) {
		out = append(out, ednStr(e))
	}
	return out
}
