// migrate.go — `cljgo migrate [up|status|new <name>]` (ADR 0072 / app-framework
// task 2.3). The library half lives in bri.core.data (migrate! / migrate-status
// over db/migrations); this is the CLI that drives it from the app directory,
// mirroring the runConfig/runRoutes pattern (a repl driver + a small bootstrap
// form that requires the bri namespaces and calls in). `new` is a Go-side file
// writer (a UTC-timestamped .sql stub, matching `cljgo generate`'s naming), so
// it needs no runtime.
package main

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"github.com/muthuishere/cljgo/pkg/repl"
)

// migrationsDir is where both `cljgo generate` and this command keep SQL
// migrations (ADR 0072). Kept relative — every subcommand runs from the app dir.
const migrationsDir = "db/migrations"

// migrateUpForm applies every pending migration and reports what changed. Shared
// with `cljgo dev` (which applies pending migrations on startup, task 2.3). The
// datasource is the app's own (get cfg :db) — the zero-install SQLite default
// (ADR 0057) unless conf.edn/APP_DB_URL points elsewhere.
const migrateUpForm = `(do
  (require 'bri.core.config 'bri.core.data 'clojure.string)
  (let [cfg    (bri.core.config/load!)
        db     (bri.core.data/connect (get cfg :db {}))
        before (count (:applied (bri.core.data/migrate-status db "db/migrations")))
        st     (bri.core.data/migrate! db "db/migrations")]
    (bri.core.data/close! db)
    (str "migrate: applied " (- (count (:applied st)) before) " new"
         " (" (count (:applied st)) " total, " (count (:pending st)) " pending)")))`

const migrateStatusForm = `(do
  (require 'bri.core.config 'bri.core.data 'clojure.string)
  (let [cfg (bri.core.config/load!)
        db  (bri.core.data/connect (get cfg :db {}))
        st  (bri.core.data/migrate-status db "db/migrations")]
    (bri.core.data/close! db)
    (str "applied (" (count (:applied st)) "): "
         (clojure.string/join ", " (:applied st)) "\n"
         "pending (" (count (:pending st)) "): "
         (clojure.string/join ", " (:pending st)))))`

func runMigrate(args []string) int {
	sub := "up"
	if len(args) > 0 {
		sub = args[0]
	}
	switch sub {
	case "up":
		return migrateRun(migrateUpForm)
	case "status":
		return migrateRun(migrateStatusForm)
	case "new":
		if len(args) < 2 {
			fmt.Fprintln(os.Stderr, "usage: cljgo migrate new <name>")
			return 2
		}
		return migrateNew(strings.Join(args[1:], " "))
	case "help", "--help", "-h":
		fmt.Fprintln(os.Stdout, "usage: cljgo migrate [up|status|new <name>]  (run from the app directory)")
		return 0
	default:
		fmt.Fprintf(os.Stderr, "cljgo migrate: unknown subcommand %q (known: up, status, new)\n", sub)
		return 2
	}
}

// migrateRun evaluates a bootstrap form (up or status) against the app's
// datasource and prints the result string.
func migrateRun(form string) int {
	if _, err := os.Stat("conf.edn"); err != nil {
		fmt.Fprintln(os.Stderr, "cljgo migrate: no conf.edn here — run from an app directory")
		return 1
	}
	if _, err := os.Stat(migrationsDir); err != nil {
		fmt.Fprintf(os.Stderr, "cljgo migrate: no %s/ — nothing to migrate (create one with `cljgo migrate new`)\n", migrationsDir)
		return 1
	}
	// The zero-install SQLite default lives under .dev/; ensure it exists so the
	// pool can open its file (harmless when a Postgres URL is configured instead).
	_ = os.MkdirAll(".dev", 0o755)
	d := repl.New(nil, os.Stdout, os.Stderr)
	out, err := d.EvalString(form, "cljgo-migrate")
	if err != nil {
		fmt.Fprintln(os.Stderr, "cljgo migrate:", err)
		return 1
	}
	if s, _ := out.(string); s != "" {
		fmt.Println(s)
	}
	return 0
}

var migrateSlugRe = regexp.MustCompile(`[^a-z0-9]+`)

// migrateNew writes a UTC-timestamped SQL stub into db/migrations, matching the
// naming `cljgo generate` uses (<yyyymmddHHMMSS>_<slug>.sql).
func migrateNew(name string) int {
	slug := strings.Trim(migrateSlugRe.ReplaceAllString(strings.ToLower(name), "_"), "_")
	if slug == "" {
		fmt.Fprintln(os.Stderr, "cljgo migrate new: name must contain a letter or digit")
		return 2
	}
	if err := os.MkdirAll(migrationsDir, 0o755); err != nil {
		fmt.Fprintln(os.Stderr, "cljgo migrate new:", err)
		return 1
	}
	ts := time.Now().Format("20060102150405")
	path := filepath.Join(migrationsDir, ts+"_"+slug+".sql")
	body := fmt.Sprintf("-- migration: %s\n-- created: %s (UTC)\n"+
		"-- Additive-only doctrine (ADR 0072): write forward DDL below; a new\n"+
		"-- migration reverses a mistake, migrations are never edited in place.\n\n",
		name, ts)
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		fmt.Fprintln(os.Stderr, "cljgo migrate new:", err)
		return 1
	}
	fmt.Println("created", path)
	return 0
}
