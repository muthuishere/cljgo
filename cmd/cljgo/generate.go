// generate.go — `cljgo generate resource <Name> <field:type>...` (ADR
// 0073): the DHH-style scaffold that turns a resource description into a
// whole authenticated CRUD slice inside a bri web app — a migration, a
// model, five handlers, a routes value, and a test — then splices the
// routes into src/app/main.cljg at documented comment markers.
//
// It is a CODE GENERATOR, not a file copy: the output is field-
// parametrized. The emission templates are REAL FILES (resource_tmpl/*,
// text/template), and generate_test.go generates a canonical resource,
// reader-validates every emitted file, and checks the splice — the ADR
// 0047 anti-rot guarantee applied to the generator's output.
//
// bri.db (ADR 0072) is the data layer the generated model calls; this
// command does NOT implement it. Every db call site is confined to the
// generated resource's model section plus the one generated app.db ns, so
// a reconciliation pass aligns exact names in one place.
package main

import (
	"embed"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"text/template"
	"time"
)

//go:embed resource_tmpl/*.tmpl
var resourceTmplFS embed.FS

// --- dispatch ----------------------------------------------------------------

func runGenerate(args []string) int {
	if len(args) == 0 {
		fmt.Fprintln(os.Stderr, "usage: cljgo generate <kind> ...\n\nkinds:\n  resource <Name> <field:type>...  scaffold a CRUD resource (ADR 0073)")
		return 2
	}
	switch args[0] {
	case "resource":
		return runGenerateResource(args[1:])
	default:
		fmt.Fprintf(os.Stderr, "cljgo generate: unknown kind %q (known: resource)\n", args[0])
		return 2
	}
}

// --- the field-type grammar --------------------------------------------------

// genField is one resolved column: its SQL declaration and the Clojure
// expression that coerces an incoming JSON value to the column's type.
type genField struct {
	Column     string // the SQL column / JSON key, e.g. "title" or "author_id"
	Keyword    string // ":title" — used as the map key in coerce
	SQLDecl    string // "title TEXT NOT NULL"
	CoerceExpr string // "(some-> (get m :title) str)"
	IsRef      bool   // a :references column (gets an index)
	sample     string // a plausible JSON value for the generated test body
}

// fieldTypes maps each grammar type to how it becomes a column. references
// is handled specially (the column is <name>_id). Keep this table and the
// ADR 0073 §2 table in lockstep.
var knownTypes = map[string]struct {
	sqlType string
	numeric bool // coerces through ->long
	boolean bool // coerces through ->bool
}{
	"string":     {"TEXT", false, false},
	"text":       {"TEXT", false, false},
	"int":        {"INTEGER", true, false},
	"bool":       {"INTEGER", false, true},
	"uuid":       {"TEXT", false, false},
	"timestamp":  {"TEXT", false, false},
	"references": {"INTEGER", true, false},
}

func knownTypeNames() string {
	return "string, text, int, bool, uuid, timestamp, references"
}

// resolveField parses one "name:type" token into a genField.
func resolveField(tok string) (genField, error) {
	parts := strings.SplitN(tok, ":", 2)
	if len(parts) != 2 || parts[0] == "" || parts[1] == "" {
		return genField{}, fmt.Errorf("field %q is not name:type (e.g. title:string)", tok)
	}
	name, typ := strings.ToLower(parts[0]), strings.ToLower(parts[1])
	if !validIdent(name) {
		return genField{}, fmt.Errorf("field name %q is not a valid identifier (lowercase letter, then letters/digits/_)", parts[0])
	}
	spec, ok := knownTypes[typ]
	if !ok {
		return genField{}, fmt.Errorf("unknown field type %q in %q (known: %s)", typ, tok, knownTypeNames())
	}
	col := name
	if typ == "references" {
		col = name + "_id"
	}
	kw := ":" + col
	f := genField{
		Column:  col,
		Keyword: kw,
		SQLDecl: col + " " + spec.sqlType + " NOT NULL",
		IsRef:   typ == "references",
	}
	switch {
	case spec.numeric:
		f.CoerceExpr = "(->long (get m " + kw + "))"
		f.sample = "1"
	case spec.boolean:
		f.CoerceExpr = "(->bool (get m " + kw + "))"
		f.sample = "true"
	default:
		f.CoerceExpr = "(some-> (get m " + kw + ") str)"
		f.sample = "sample"
	}
	return f, nil
}

// --- template data -----------------------------------------------------------

type resourceData struct {
	Singular   string // "note"
	Plural     string // "notes"
	Ns         string // "app.notes"
	Alias      string // "notes"
	Table      string // "notes"
	RouteBase  string // "/api/notes"
	CollKw     string // ":notes"
	MemberKw   string // ":note"
	TableKw    string // ":notes"
	Fields     []genField
	NeedsLong  bool
	NeedsBool  bool
	SetClause  string // "title = ?, body = ?"
	SetArgs    string // "(get row :title) (get row :body)"
	SampleBody string // "{:title \"sample\" :body \"sample\"}"
	Timestamp  string
	Command    string
}

func buildResourceData(name string, fieldToks []string, command string) (resourceData, error) {
	singular := strings.ToLower(name)
	if !validIdent(singular) {
		return resourceData{}, fmt.Errorf("resource name %q is not a valid identifier (lowercase letter, then letters/digits/_)", name)
	}
	plural := pluralize(singular)
	fields := make([]genField, 0, len(fieldToks))
	for _, tok := range fieldToks {
		f, err := resolveField(tok)
		if err != nil {
			return resourceData{}, err
		}
		fields = append(fields, f)
	}
	if len(fields) == 0 {
		return resourceData{}, fmt.Errorf("a resource needs at least one field, e.g. `cljgo generate resource %s title:string`", name)
	}

	d := resourceData{
		Singular:  singular,
		Plural:    plural,
		Ns:        "app." + plural,
		Alias:     plural,
		Table:     plural,
		RouteBase: "/api/" + plural,
		CollKw:    ":" + plural,
		MemberKw:  ":" + singular,
		TableKw:   ":" + plural,
		Fields:    fields,
		Timestamp: time.Now().Format("20060102150405"),
		Command:   command,
	}

	sets := make([]string, len(fields))
	setArgs := make([]string, len(fields))
	sampleParts := make([]string, len(fields))
	for i, f := range fields {
		sets[i] = f.Column + " = ?"
		setArgs[i] = "(get row " + f.Keyword + ")"
		sampleParts[i] = f.Keyword + ` "` + f.sample + `"`
		if strings.Contains(f.CoerceExpr, "->long") {
			d.NeedsLong = true
		}
		if strings.Contains(f.CoerceExpr, "->bool") {
			d.NeedsBool = true
		}
	}
	d.SetClause = strings.Join(sets, ", ")
	d.SetArgs = strings.Join(setArgs, " ")
	d.SampleBody = "{" + strings.Join(sampleParts, " ") + "}"
	return d, nil
}

// --- the command -------------------------------------------------------------

func runGenerateResource(args []string) int {
	flags := flag.NewFlagSet("generate resource", flag.ContinueOnError)
	flags.SetOutput(os.Stderr)
	force := flags.Bool("force", false, "overwrite an existing resource ns (never touches user edits otherwise)")
	flags.Usage = func() {
		fmt.Fprintln(os.Stderr, "usage: cljgo generate resource <Name> <field:type>...")
		fmt.Fprintln(os.Stderr, "\n  field types:", knownTypeNames())
		fmt.Fprintln(os.Stderr, "  example: cljgo generate resource Note title:string body:text")
		flags.PrintDefaults()
	}
	if err := flags.Parse(args); err != nil {
		return 2
	}
	if flags.NArg() < 1 {
		flags.Usage()
		return 2
	}
	name := flags.Arg(0)
	fieldToks := flags.Args()[1:]

	// Must run inside a bri web app: main.cljg is where the routes splice.
	if _, err := os.Stat(appMain); err != nil {
		fmt.Fprintf(os.Stderr, "cljgo generate resource: no %s here — run from a bri web app (create one with `cljgo new --template web <name>`)\n", appMain)
		return 1
	}

	command := "cljgo generate resource " + name + " " + strings.Join(fieldToks, " ")
	command = strings.TrimSpace(command)
	d, err := buildResourceData(name, fieldToks, command)
	if err != nil {
		fmt.Fprintln(os.Stderr, "cljgo generate resource:", err)
		return 2
	}

	resourcePath := filepath.Join("src", "app", d.Plural+".cljg")
	if _, err := os.Stat(resourcePath); err == nil && !*force {
		fmt.Fprintf(os.Stderr, "cljgo generate resource: %s already exists (pass --force to overwrite)\n", resourcePath)
		return 1
	}

	// Render the three field-parametrized files.
	resource, err := renderResourceTemplate("resource.cljg.tmpl", d)
	if err != nil {
		fmt.Fprintln(os.Stderr, "cljgo generate resource:", err)
		return 1
	}
	test, err := renderResourceTemplate("resource_test.cljg.tmpl", d)
	if err != nil {
		fmt.Fprintln(os.Stderr, "cljgo generate resource:", err)
		return 1
	}
	migration, err := renderResourceTemplate("migration.sql.tmpl", d)
	if err != nil {
		fmt.Fprintln(os.Stderr, "cljgo generate resource:", err)
		return 1
	}

	// The splice is validated BEFORE any file is written — a missing
	// marker is a clean error, not a half-generated resource.
	mainBytes, err := os.ReadFile(appMain)
	if err != nil {
		fmt.Fprintln(os.Stderr, "cljgo generate resource:", err)
		return 1
	}
	splicedMain, spliceErr := spliceMain(string(mainBytes), d)
	if spliceErr != nil {
		fmt.Fprintln(os.Stderr, "cljgo generate resource:", spliceErr)
		return 1
	}

	// --- write everything ---------------------------------------------------
	created := []string{}
	write := func(path, body string) error {
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			return err
		}
		if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
			return err
		}
		created = append(created, path)
		return nil
	}

	migrationPath := filepath.Join("db", "migrations", d.Timestamp+"_create_"+d.Table+".sql")
	dbPath := filepath.Join("src", "app", "db.cljg")
	testPath := filepath.Join("test", "app", d.Plural+"_test.cljg")

	// app.db is created ONCE (the shared datasource); never clobbered.
	if _, err := os.Stat(dbPath); os.IsNotExist(err) {
		dbNs, err := renderResourceTemplate("db.cljg.tmpl", d)
		if err != nil {
			fmt.Fprintln(os.Stderr, "cljgo generate resource:", err)
			return 1
		}
		if err := write(dbPath, dbNs); err != nil {
			fmt.Fprintln(os.Stderr, "cljgo generate resource:", err)
			return 1
		}
	}
	for _, f := range []struct{ path, body string }{
		{migrationPath, migration},
		{resourcePath, resource},
		{testPath, test},
	} {
		if err := write(f.path, f.body); err != nil {
			fmt.Fprintln(os.Stderr, "cljgo generate resource:", err)
			return 1
		}
	}
	if err := os.WriteFile(appMain, []byte(splicedMain), 0o644); err != nil {
		fmt.Fprintln(os.Stderr, "cljgo generate resource:", err)
		return 1
	}

	// --- report -------------------------------------------------------------
	fmt.Printf("generated resource %s (%s)\n", d.Singular, d.RouteBase)
	for _, p := range created {
		fmt.Printf("  create  %s\n", p)
	}
	fmt.Printf("  splice  %s  (require %s + routes)\n", appMain, d.Ns)
	fmt.Printf("\nnext:\n  cljgo routes   # see the new endpoints\n  cljgo test     # the generated CRUD suite (green)\n  cljgo dev      # serve it\n")
	return 0
}

// renderResourceTemplate executes one embedded template against the data.
func renderResourceTemplate(name string, d resourceData) (string, error) {
	body, err := resourceTmplFS.ReadFile("resource_tmpl/" + name)
	if err != nil {
		return "", err
	}
	t, err := template.New(name).Parse(string(body))
	if err != nil {
		return "", fmt.Errorf("template %s: %w", name, err)
	}
	var sb strings.Builder
	if err := t.Execute(&sb, d); err != nil {
		return "", fmt.Errorf("template %s: %w", name, err)
	}
	return sb.String(), nil
}

// --- the marker splice -------------------------------------------------------

const (
	markerRequires = ";; cljgo:resource-requires"
	markerRoutes   = ";; cljgo:resource-routes"
)

// spliceMain inserts the resource's require and routes value into
// app.main above the two documented comment markers (ADR 0073 §4). It is
// idempotent (a re-run for an already-wired resource is a no-op) and never
// parses s-expressions — the markers are the seam. A missing marker is a
// named error listing the exact lines to add.
func spliceMain(content string, d resourceData) (string, error) {
	requireLine := "[" + d.Ns + " :as " + d.Alias + "]"
	routesLine := d.Alias + "/routes"

	out := content
	if !strings.Contains(out, requireLine) {
		spliced, ok := insertAboveMarker(out, markerRequires, requireLine)
		if !ok {
			return "", markerMissingErr(markerRequires, "            ")
		}
		out = spliced
	}
	if !strings.Contains(out, routesLine) {
		spliced, ok := insertAboveMarker(out, markerRoutes, routesLine)
		if !ok {
			return "", markerMissingErr(markerRoutes, "    ")
		}
		out = spliced
	}
	return out, nil
}

// insertAboveMarker puts `insert` on its own line, at the marker's own
// indentation, directly above the marker line.
func insertAboveMarker(content, marker, insert string) (string, bool) {
	idx := strings.Index(content, marker)
	if idx < 0 {
		return content, false
	}
	lineStart := strings.LastIndex(content[:idx], "\n") + 1
	indent := content[lineStart:idx] // whitespace before the marker
	return content[:lineStart] + indent + insert + "\n" + content[lineStart:], true
}

func markerMissingErr(marker, indent string) error {
	return fmt.Errorf("%s has no `%s` marker to splice into — add these lines and re-run:\n"+
		"  in the (:require ...) of app.main:  %s%s\n"+
		"  and inside (http/routes ...):       %s%s",
		appMain, marker,
		indent, markerRequires,
		indent, markerRoutes)
}

// --- small helpers -----------------------------------------------------------

func validIdent(s string) bool {
	if s == "" {
		return false
	}
	for i, c := range s {
		switch {
		case c >= 'a' && c <= 'z':
		case c == '_':
		case c >= '0' && c <= '9' && i > 0:
		default:
			return false
		}
	}
	return s[0] >= 'a' && s[0] <= 'z'
}

func isVowel(b byte) bool {
	switch b {
	case 'a', 'e', 'i', 'o', 'u':
		return true
	}
	return false
}

// pluralize is the minimal English ruleset (ADR 0073 §5): consonant+y →
// ies, sibilant → es, else +s. Enough for identifier pluralization.
func pluralize(s string) string {
	if s == "" {
		return s
	}
	if strings.HasSuffix(s, "y") && len(s) >= 2 && !isVowel(s[len(s)-2]) {
		return s[:len(s)-1] + "ies"
	}
	for _, suf := range []string{"s", "x", "z", "ch", "sh"} {
		if strings.HasSuffix(s, suf) {
			return s + "es"
		}
	}
	return s + "s"
}
