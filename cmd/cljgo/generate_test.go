// generate_test.go — the anti-rot gate for `cljgo generate resource`
// (ADR 0073). A resource scaffold is a CODE GENERATOR, so the guarantee
// ADR 0047 buys for project templates is applied here to the generator's
// OUTPUT: a canonical resource is generated into a real web project, every
// emitted .cljg is reader-validated as source, the migration SQL is
// checked, and the marker splice into app.main is asserted (idempotent,
// force, missing-marker).
//
// The generated CRUD calls bri.core.data (ADR 0072) and a generated resource's
// own `cljgo test` is green out of the box (verified against a fresh
// in-memory DB). This gate proves the generator emits valid source and
// splices correctly; TestExampleWebApiSuite-style E2E covers the runtime.
package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/muthuishere/cljgo/pkg/reader"
)

// layDownWebApp renders the embedded web template into dir, so the test
// operates on the REAL app `cljgo new --template web` produces (markers and
// all) rather than a stub.
func layDownWebApp(t *testing.T, dir string) {
	t.Helper()
	src, err := templateFS("web")
	if err != nil {
		t.Fatalf("templateFS(web): %v", err)
	}
	files, err := renderTemplate(src, "blog")
	if err != nil {
		t.Fatalf("renderTemplate(web): %v", err)
	}
	for p, body := range files {
		full := filepath.Join(dir, filepath.FromSlash(p))
		if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(full, []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
}

// readsAsClojure fails the test if body is not valid Clojure/EDN source —
// the same check templates_test.go applies to the shipped templates.
func readsAsClojure(t *testing.T, label, body string) {
	t.Helper()
	if _, err := reader.New(strings.NewReader(body)).ReadAll(); err != nil {
		t.Errorf("%s does not read as Clojure: %v\n---\n%s", label, err, body)
	}
}

func readFile(t *testing.T, path string) string {
	t.Helper()
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	return string(b)
}

// TestGenerateResource is the end-to-end gate: generate a resource into a
// web app and assert every emitted file is valid source and the splice
// landed.
func TestGenerateResource(t *testing.T) {
	dir := t.TempDir()
	layDownWebApp(t, dir)
	t.Chdir(dir)

	if code := runGenerateResource([]string{"Note", "title:string", "body:text", "views:int", "done:bool"}); code != 0 {
		t.Fatalf("generate resource returned %d", code)
	}

	// --- the files the scaffold creates ------------------------------------
	resource := filepath.Join("src", "app", "notes.cljg")
	dbNs := filepath.Join("src", "app", "db.cljg")
	test := filepath.Join("test", "app", "notes_test.cljg")
	for _, f := range []string{resource, dbNs, test} {
		if _, err := os.Stat(f); err != nil {
			t.Fatalf("expected generated file %s: %v", f, err)
		}
		readsAsClojure(t, f, readFile(t, f))
	}

	// exactly one timestamped migration for the table
	migs, _ := filepath.Glob(filepath.Join("db", "migrations", "*_create_notes.sql"))
	if len(migs) != 1 {
		t.Fatalf("want 1 migration, got %v", migs)
	}
	sql := readFile(t, migs[0])
	for _, want := range []string{
		"CREATE TABLE notes",
		"id INTEGER PRIMARY KEY AUTOINCREMENT",
		"title TEXT NOT NULL",
		"body TEXT NOT NULL",
		"views INTEGER NOT NULL",
		"done INTEGER NOT NULL",
	} {
		if !strings.Contains(sql, want) {
			t.Errorf("migration missing %q:\n%s", want, sql)
		}
	}

	// --- the resource ns: model calls bri.core.data, handlers use bri.web.http --------
	rsrc := readFile(t, resource)
	for _, want := range []string{
		"(ns app.notes",
		"[bri.core.data :as db]",
		"[app.db :as adb]",
		`(db/query   ds "SELECT * FROM notes ORDER BY id DESC")`,
		"(db/insert! ds :notes row)",
		`(db/exec!   ds "DELETE FROM notes WHERE id = ?" id)`,
		"(http/param! req :id :int)",
		"(->long (get m :views))",
		"(->bool (get m :done))",
		"(auth/admin-only)",
		"#'delete-one",
	} {
		if !strings.Contains(rsrc, want) {
			t.Errorf("resource ns missing %q", want)
		}
	}

	// --- the splice into app.main ------------------------------------------
	main := readFile(t, filepath.Join("src", "app", "main.cljg"))
	readsAsClojure(t, "spliced main.cljg", main)
	if !strings.Contains(main, "[app.notes :as notes]") {
		t.Error("main.cljg was not spliced with the app.notes require")
	}
	if !strings.Contains(main, "notes/routes") {
		t.Error("main.cljg was not spliced with notes/routes")
	}
	// markers survive for the next resource
	if !strings.Contains(main, markerRequires) || !strings.Contains(main, markerRoutes) {
		t.Error("splice consumed a marker — a second resource could not be added")
	}
}

// TestGenerateResourceIdempotentSplice: a second resource splices cleanly,
// and re-running the SAME resource never duplicates the require/routes and
// refuses to clobber without --force.
func TestGenerateResourceIdempotentSplice(t *testing.T) {
	dir := t.TempDir()
	layDownWebApp(t, dir)
	t.Chdir(dir)

	if code := runGenerateResource([]string{"Note", "title:string"}); code != 0 {
		t.Fatalf("first resource: %d", code)
	}
	// a second, different resource
	if code := runGenerateResource([]string{"User", "email:string"}); code != 0 {
		t.Fatalf("second resource: %d", code)
	}
	main := readFile(t, filepath.Join("src", "app", "main.cljg"))
	readsAsClojure(t, "main after two resources", main)
	for _, want := range []string{"[app.notes :as notes]", "notes/routes", "[app.users :as users]", "users/routes"} {
		if strings.Count(main, want) != 1 {
			t.Errorf("want exactly one %q in main.cljg, got %d", want, strings.Count(main, want))
		}
	}

	// re-running Note without --force refuses (no clobber)
	if code := runGenerateResource([]string{"Note", "title:string"}); code == 0 {
		t.Error("re-generating an existing resource without --force should fail")
	}
	// with --force it succeeds and the splice stays single (idempotent)
	if code := runGenerateResource([]string{"--force", "Note", "title:string", "body:text"}); code != 0 {
		t.Error("--force should overwrite the resource")
	}
	main = readFile(t, filepath.Join("src", "app", "main.cljg"))
	if strings.Count(main, "notes/routes") != 1 {
		t.Errorf("--force re-run duplicated the routes splice: %d", strings.Count(main, "notes/routes"))
	}
}

// TestGenerateResourceOutsideWebApp: run outside a bri app is a clean
// error, not a panic or a half-written tree.
func TestGenerateResourceOutsideWebApp(t *testing.T) {
	dir := t.TempDir()
	t.Chdir(dir)
	if code := runGenerateResource([]string{"Note", "title:string"}); code == 0 {
		t.Error("generate resource with no src/app/main.cljg should fail")
	}
	if _, err := os.Stat(filepath.Join("src", "app", "notes.cljg")); err == nil {
		t.Error("a failed generate left files behind")
	}
}

// TestGenerateResourceMissingMarker: an app.main without the markers gets a
// named error and is NOT edited.
func TestGenerateResourceMissingMarker(t *testing.T) {
	d := resourceData{Ns: "app.notes", Alias: "notes"}
	if _, err := spliceMain("(ns app.main)\n(def routes (http/routes))\n", d); err == nil {
		t.Fatal("spliceMain with no markers should error")
	} else if !strings.Contains(err.Error(), markerRequires) {
		t.Errorf("the missing-marker error should name the marker, got: %v", err)
	}
}

func TestResolveField(t *testing.T) {
	cases := []struct {
		tok, col, decl string
		wantErr        bool
	}{
		{"title:string", "title", "title TEXT NOT NULL", false},
		{"body:text", "body", "body TEXT NOT NULL", false},
		{"views:int", "views", "views INTEGER NOT NULL", false},
		{"done:bool", "done", "done INTEGER NOT NULL", false},
		{"ref:uuid", "ref", "ref TEXT NOT NULL", false},
		{"at:timestamp", "at", "at TEXT NOT NULL", false},
		{"author:references", "author_id", "author_id INTEGER NOT NULL", false},
		{"bogus:nope", "", "", true},
		{"nocolon", "", "", true},
		{"Bad Name:string", "", "", true},
	}
	for _, c := range cases {
		f, err := resolveField(c.tok)
		if c.wantErr {
			if err == nil {
				t.Errorf("resolveField(%q) should error", c.tok)
			}
			continue
		}
		if err != nil {
			t.Errorf("resolveField(%q): %v", c.tok, err)
			continue
		}
		if f.Column != c.col || f.SQLDecl != c.decl {
			t.Errorf("resolveField(%q) = {col %q decl %q}, want {col %q decl %q}", c.tok, f.Column, f.SQLDecl, c.col, c.decl)
		}
	}
}

func TestReferencesGetsIndex(t *testing.T) {
	dir := t.TempDir()
	layDownWebApp(t, dir)
	t.Chdir(dir)
	if code := runGenerateResource([]string{"Post", "author:references"}); code != 0 {
		t.Fatalf("generate: %d", code)
	}
	migs, _ := filepath.Glob(filepath.Join("db", "migrations", "*_create_posts.sql"))
	if len(migs) != 1 {
		t.Fatalf("want 1 migration, got %v", migs)
	}
	sql := readFile(t, migs[0])
	if !strings.Contains(sql, "CREATE INDEX idx_posts_author_id ON posts (author_id)") {
		t.Errorf("a references column should get an index:\n%s", sql)
	}
}

func TestPluralize(t *testing.T) {
	cases := map[string]string{
		"note": "notes", "user": "users", "city": "cities", "box": "boxes",
		"class": "classes", "dish": "dishes", "match": "matches", "day": "days",
		"key": "keys", "buzz": "buzzes",
	}
	for in, want := range cases {
		if got := pluralize(in); got != want {
			t.Errorf("pluralize(%q) = %q, want %q", in, got, want)
		}
	}
}

// TestGeneratedTemplatesAreValidStructurally renders the resource against a
// canonical field set and reader-validates each emitted file directly (no
// filesystem), so a template edit that breaks the source cannot merge.
func TestGeneratedTemplatesReadAsSource(t *testing.T) {
	d, err := buildResourceData("Widget", []string{"name:string", "qty:int", "active:bool", "owner:references"}, "cljgo generate resource Widget ...")
	if err != nil {
		t.Fatal(err)
	}
	for _, tmpl := range []string{"resource.cljg.tmpl", "resource_test.cljg.tmpl", "db.cljg.tmpl"} {
		out, err := renderResourceTemplate(tmpl, d)
		if err != nil {
			t.Fatalf("render %s: %v", tmpl, err)
		}
		readsAsClojure(t, tmpl, out)
	}
}
