package diag

// The published error-code reference is part of the product, so it is tested
// like the rest of the product.
//
// The registry is append-only, which makes it very easy to add a code, ship
// it, and never touch the docs — and that is exactly what happened: the site
// table sat at G5009 while the registry had grown through G5020, so every
// diagnostic the Clojars work raised (I4002, R1012, the G501x band) rendered
// `help: run cljgo explain <CODE>` for a code the public reference did not
// list. The lock test below catches a changed code; this one catches an
// undocumented one.

import (
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"testing"
)

// siteRow matches a row of the "Registered codes" table:  | G5010 | title | since |
var siteRow = regexp.MustCompile(`(?m)^\|\s*([A-Z][0-9]{4})\s*\|`)

func TestSiteDiagnosticsPageListsEveryCode(t *testing.T) {
	page := filepath.Join("..", "..", "site", "src", "content", "docs", "reference", "diagnostics.md")
	b, err := os.ReadFile(page)
	if err != nil {
		// The site is checked in; a missing page is a real failure, not a
		// reason to skip.
		t.Fatalf("read %s: %v", page, err)
	}

	documented := map[string]bool{}
	for _, m := range siteRow.FindAllStringSubmatch(string(b), -1) {
		documented[m[1]] = true
	}

	var missing, extra []string
	registered := map[string]bool{}
	for _, e := range Entries() {
		registered[e.Code] = true
		if !documented[e.Code] {
			missing = append(missing, e.Code+" — "+e.Title)
		}
	}
	for code := range documented {
		if !registered[code] {
			extra = append(extra, code)
		}
	}
	sort.Strings(missing)
	sort.Strings(extra)

	if len(missing) > 0 {
		t.Errorf("codes registered but absent from the published reference (%s):\n  %s\n"+
			"add a row to the \"Registered codes\" table — a code whose help line points at an "+
			"undocumented page is worse than no code at all",
			page, strings.Join(missing, "\n  "))
	}
	if len(extra) > 0 {
		t.Errorf("codes documented at %s but not in the registry: %s", page, strings.Join(extra, ", "))
	}
}

// TestEveryRegisteredCodeHasAnExplainPage guards the other half of the
// promise: the renderer tells the user to run `cljgo explain <CODE>`, so the
// page that command prints has to exist.
func TestEveryRegisteredCodeHasAnExplainPage(t *testing.T) {
	for _, e := range Entries() {
		p := filepath.Join("..", "..", "docs", "diagnostics", e.Code+".md")
		if _, err := os.Stat(p); err != nil {
			t.Errorf("%s (%s): no explain page at docs/diagnostics/%s.md", e.Code, e.Title, e.Code)
		}
	}
}
