// templates_test.go — the templates are REAL source, and they stay
// that way (ADR 0041/0047, openspec app-framework task 0.1). These are
// the fast guards, and they are GENERIC: they walk the embedded FS, so
// a new template cannot slip in unchecked. The slow ones — every
// template GENERATED, tested, and run through the real binary — are in
// bri_test.go. Between them, no template can rot silently.
package main

import (
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"

	"github.com/muthuishere/cljgo/pkg/reader"
	"github.com/muthuishere/cljgo/templates"
)

// The manifests, one per built-in template. A tier (or a template) that
// adds a file to a blessed layout updates this map — deliberately, in
// the same change. The map's KEY SET is checked against the embedded FS,
// so adding templates/foo/ without a manifest is a test failure, not a
// silent gap.
var templateManifests = map[string][]string{
	// lib: the default — a library, no server, no main, no conf.
	"lib": {
		".gitignore",
		"README.md",
		"build.cljgo",
		"src/newapp/core.cljg",
		"test/newapp/core_test.cljg",
	},
	// cli: a tool — -main, args, a build plan that makes one binary.
	"cli": {
		".gitignore",
		"README.md",
		"build.cljgo",
		"src/newapp/core.cljg",
		"test/newapp/core_test.cljg",
	},
	// web: the bri app (the T0 manifest).
	"web": {
		".gitignore",
		"build.cljgo",
		"conf.edn",
		"conf.schema.edn",
		"public/app.css",
		"src/app/main.cljg",
		"test/app/main_test.cljg",
	},
}

// renderBuiltin renders one built-in template with a given app name.
func renderBuiltin(t *testing.T, name, appName string) map[string]string {
	t.Helper()
	src, err := templateFS(name)
	if err != nil {
		t.Fatalf("templateFS(%s): %v", name, err)
	}
	files, err := renderTemplate(src, appName)
	if err != nil {
		t.Fatalf("renderTemplate(%s): %v", name, err)
	}
	return files
}

// embeddedTemplates lists the template directories actually embedded —
// the source of truth this file checks the manifests against.
func embeddedTemplates(t *testing.T) []string {
	t.Helper()
	entries, err := fs.ReadDir(templates.FS, ".")
	if err != nil {
		t.Fatalf("ReadDir(templates.FS): %v", err)
	}
	var names []string
	for _, e := range entries {
		if e.IsDir() {
			names = append(names, e.Name())
		}
	}
	sort.Strings(names)
	return names
}

// Every embedded template is declared: in the manifest map (so its file
// list is reviewed) and in templates.Builtins (so --template's help and
// the "next:" block know it). No template is a stowaway.
func TestEveryEmbeddedTemplateIsDeclared(t *testing.T) {
	for _, name := range embeddedTemplates(t) {
		if _, ok := templateManifests[name]; !ok {
			t.Errorf("templates/%s is embedded but has no manifest in templateManifests", name)
		}
		if _, ok := templates.LookupBuiltin(name); !ok {
			t.Errorf("templates/%s is embedded but is not in templates.Builtins", name)
		}
	}
	embedded := strings.Join(embeddedTemplates(t), ",")
	var declared []string
	for name := range templateManifests {
		declared = append(declared, name)
	}
	sort.Strings(declared)
	if strings.Join(declared, ",") != embedded {
		t.Errorf("manifests %v do not match the embedded templates %v", declared, embedded)
	}
	for _, b := range templates.Builtins {
		if _, err := templateFS(b.Name); err != nil {
			t.Errorf("templates.Builtins names %q, which does not resolve: %v", b.Name, err)
		}
		if len(b.Next) == 0 {
			t.Errorf("template %q declares no Next commands", b.Name)
		}
	}
}

// The template files are valid source AS THEY SIT ON DISK — no
// substitution needed, for EVERY template. That is what lets CI (and a
// human) run a template in place, and it is why the app name is a real
// default name rather than a mustache.
func TestTemplatesAreValidSourceUnsubstituted(t *testing.T) {
	for _, name := range embeddedTemplates(t) {
		t.Run(name, func(t *testing.T) {
			files := renderBuiltin(t, name, templates.DefaultName) // identity render

			var got []string
			for p := range files {
				got = append(got, p)
			}
			sort.Strings(got)
			want := templateManifests[name]
			if strings.Join(got, ",") != strings.Join(want, ",") {
				t.Fatalf("template manifest changed:\n got %v\nwant %v", got, want)
			}

			for _, p := range want {
				if !strings.HasSuffix(p, ".cljg") && !strings.HasSuffix(p, ".clj") &&
					!strings.HasSuffix(p, ".edn") && !strings.HasSuffix(p, ".cljgo") {
					continue
				}
				if _, err := reader.New(strings.NewReader(files[p])).ReadAll(); err != nil {
					t.Errorf("%s does not read as Clojure/EDN: %v", p, err)
				}
			}
		})
	}
}

// The substitution mechanism: one sentinel — the default app name —
// replaced in contents and in path names, in every template. Nothing
// else varies.
func TestRenderTemplateRenamesTheApp(t *testing.T) {
	for _, name := range embeddedTemplates(t) {
		t.Run(name, func(t *testing.T) {
			for p, body := range renderBuiltin(t, name, "shipyard") {
				if strings.Contains(p, templates.DefaultName) || strings.Contains(body, templates.DefaultName) {
					t.Errorf("%s still carries the %q sentinel after rendering", p, templates.DefaultName)
				}
			}
		})
	}

	// Spot checks that the rename lands where the app's identity lives.
	for _, want := range []struct{ template, path, contains string }{
		{"lib", "src/shipyard/core.cljg", "(ns shipyard.core)"},
		{"cli", "build.cljgo", `{:name "shipyard"`},
		{"cli", ".gitignore", "/shipyard\n"},
		{"web", "src/app/main.cljg", `{:title "shipyard"}`},
		{"web", "build.cljgo", `{:name "shipyard"`},
		{"web", ".gitignore", "/shipyard\n"},
	} {
		files := renderBuiltin(t, want.template, "shipyard")
		if !strings.Contains(files[want.path], want.contains) {
			t.Errorf("%s/%s: expected %q\n%s", want.template, want.path, want.contains, files[want.path])
		}
	}
}

// The layering call (ADR 0047): the LANGUAGE's `new` hands you a
// library, not a web app. cljgo ships a great framework; it is not one.
func TestNewDefaultsToLib(t *testing.T) {
	if templates.DefaultTemplate != "lib" {
		t.Fatalf("DefaultTemplate = %q, want lib — `cljgo new` is framework-agnostic (ADR 0047)",
			templates.DefaultTemplate)
	}
	t.Chdir(t.TempDir())
	if code := runNew([]string{"mylib"}); code != 0 {
		t.Fatalf("cljgo new: exit %d", code)
	}
	if _, err := os.Stat(filepath.Join("mylib", "src", "mylib", "core.cljg")); err != nil {
		t.Errorf("default template did not generate a library: %v", err)
	}
	for _, unwanted := range []string{"conf.edn", filepath.Join("src", "app", "main.cljg"), "public"} {
		if _, err := os.Stat(filepath.Join("mylib", unwanted)); err == nil {
			t.Errorf("`cljgo new` handed a library author %s — that is the web template's", unwanted)
		}
	}
}

// --template takes a local directory (the Rails model, minus the
// network). Community templates are just directories of real files.
func TestNewWithLocalTemplate(t *testing.T) {
	tmpl := t.TempDir()
	if err := os.MkdirAll(filepath.Join(tmpl, "src", "app"), 0o755); err != nil {
		t.Fatal(err)
	}
	write := func(rel, body string) {
		if err := os.WriteFile(filepath.Join(tmpl, filepath.FromSlash(rel)), []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	write("src/app/main.cljg", `(ns app.main) ;; newapp`)
	write("newapp.edn", "{:name \"newapp\"}\n")

	t.Chdir(t.TempDir())
	if code := runNew([]string{"--template", tmpl, "mine"}); code != 0 {
		t.Fatalf("cljgo new --template: exit %d", code)
	}
	body, err := os.ReadFile(filepath.Join("mine", "src", "app", "main.cljg"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(body), ";; mine") {
		t.Errorf("content not substituted: %q", body)
	}
	if _, err := os.Stat(filepath.Join("mine", "mine.edn")); err != nil {
		t.Errorf("path not substituted: %v", err)
	}
}

func TestNewRejectsBadTemplates(t *testing.T) {
	t.Chdir(t.TempDir())
	for _, tmpl := range []string{
		"https://github.com/someone/bri-template.git", // git URLs: deferred, honestly
		"git@github.com:someone/bri-template.git",
		"nosuchtemplate",
		"./nowhere",
	} {
		if code := runNew([]string{"--template", tmpl, "app"}); code == 0 {
			t.Errorf("--template %s: expected a refusal, got exit 0", tmpl)
		}
	}
}
