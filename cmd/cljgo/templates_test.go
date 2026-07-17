// templates_test.go — the templates are REAL source, and they stay
// that way (ADR 0041, openspec app-framework task 0.1). These are the
// fast guards; the slow one — the whole template GENERATED, tested,
// booted and curled through the real binary — is TestKeelNewDevTest in
// keel_test.go. Between them, the template cannot rot silently.
package main

import (
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"

	"github.com/muthuishere/cljgo/pkg/reader"
	"github.com/muthuishere/cljgo/templates"
)

// The T0 manifest. A tier that adds a file to the blessed layout
// updates this list — deliberately, in the same change.
var wantTemplateFiles = []string{
	".gitignore",
	"build.cljgo",
	"conf.edn",
	"conf.schema.edn",
	"public/app.css",
	"src/app/main.cljg",
	"test/app/main_test.cljg",
}

func webTemplate(t *testing.T) map[string]string {
	t.Helper()
	src, err := templateFS(templates.DefaultTemplate)
	if err != nil {
		t.Fatalf("templateFS: %v", err)
	}
	files, err := renderTemplate(src, templates.DefaultName) // identity render
	if err != nil {
		t.Fatalf("renderTemplate: %v", err)
	}
	return files
}

// The template files are valid source AS THEY SIT ON DISK — no
// substitution needed. That is what lets CI (and a human) run the
// template in place, and it is why the app name is a real default name
// rather than a mustache.
func TestTemplateIsValidSourceUnsubstituted(t *testing.T) {
	files := webTemplate(t)

	var got []string
	for p := range files {
		got = append(got, p)
	}
	sort.Strings(got)
	if strings.Join(got, ",") != strings.Join(wantTemplateFiles, ",") {
		t.Fatalf("template manifest changed:\n got %v\nwant %v", got, wantTemplateFiles)
	}

	for _, p := range wantTemplateFiles {
		if !strings.HasSuffix(p, ".cljg") && !strings.HasSuffix(p, ".clj") &&
			!strings.HasSuffix(p, ".edn") && !strings.HasSuffix(p, ".cljgo") {
			continue
		}
		if _, err := reader.New(strings.NewReader(files[p])).ReadAll(); err != nil {
			t.Errorf("%s does not read as Clojure/EDN: %v", p, err)
		}
	}
}

// The substitution mechanism: one sentinel — the default app name —
// replaced in contents and in path names. Nothing else varies.
func TestRenderTemplateRenamesTheApp(t *testing.T) {
	src, err := templateFS(templates.DefaultTemplate)
	if err != nil {
		t.Fatal(err)
	}
	files, err := renderTemplate(src, "shipyard")
	if err != nil {
		t.Fatal(err)
	}
	for p, body := range files {
		if strings.Contains(p, templates.DefaultName) || strings.Contains(body, templates.DefaultName) {
			t.Errorf("%s still carries the %q sentinel after rendering", p, templates.DefaultName)
		}
	}
	for _, want := range []struct{ path, contains string }{
		{"src/app/main.cljg", `{:title "shipyard"}`},
		{"build.cljgo", `{:name "shipyard"`},
		{".gitignore", "/shipyard\n"},
	} {
		if !strings.Contains(files[want.path], want.contains) {
			t.Errorf("%s: expected %q\n%s", want.path, want.contains, files[want.path])
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
		"https://github.com/someone/keel-template.git", // git URLs: deferred, honestly
		"git@github.com:someone/keel-template.git",
		"nosuchtemplate",
		"./nowhere",
	} {
		if code := runNew([]string{"--template", tmpl, "app"}); code == 0 {
			t.Errorf("--template %s: expected a refusal, got exit 0", tmpl)
		}
	}
}
