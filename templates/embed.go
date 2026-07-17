// Package templates carries the app templates `cljgo new` generates
// from (ADR 0041, openspec app-framework T0) — as REAL FILES on disk,
// not Go string literals: a template that is a string literal is never
// compiled, never linted, never run, and rots the first time keel's API
// shifts. These files are exemplary source in their own right — the
// generated app IS the framework's tutorial (S20 VERDICT's golden path,
// trimmed to the shipped tiers).
//
// The whole tree is embedded into the binary, so `cljgo new` stays
// zero-install, offline, and version-matched to the toolchain that
// generates with it (a first-run network fetch is a failure inside the
// first 15 minutes — the ADR 0041 review round called it disqualifying).
//
// # Directly runnable, in place
//
// Every template is valid, runnable source WITHOUT substitution: the
// app name is not a mustache, it is a REAL default app name — DefaultName
// ("newapp") — which the generator renames. `cd templates/web && cljgo
// dev` boots the page; that is exactly why CI can run the template
// (cmd/cljgo/keel_test.go generates from it and curls the result).
//
// # Adding a template
//
// Drop a directory under templates/ (one `all:` embed covers it), name
// the app DefaultName wherever the app's own name appears, and expose it
// via `cljgo new --template <name-or-path>`.
package templates

import "embed"

// FS holds every template tree, rooted at the template name:
// "web/conf.edn", "web/src/app/main.cljg", ... The `all:` prefix is
// load-bearing — a plain embed skips dotfiles, and the template's
// .gitignore is part of the app.
//
//go:embed all:web
var FS embed.FS

// DefaultTemplate is the template `cljgo new` uses when --template is
// not given.
const DefaultTemplate = "web"

// DefaultName is the app name the template files are written with. The
// generator replaces this token (in file contents and in path names)
// with the name the user asked for. It is a plain, distinctive word on
// purpose: the templates stay runnable as-is, and the substitution is
// one obvious mechanism with nothing to escape.
const DefaultName = "newapp"
