// Package templates carries the project templates `cljgo new` generates
// from (ADR 0041, ADR 0047, openspec app-framework T0) — as REAL FILES
// on disk, not Go string literals: a template that is a string literal
// is never compiled, never linted, never run, and rots the first time an
// API shifts. These files are exemplary source in their own right.
//
// The whole tree is embedded into the binary, so `cljgo new` stays
// zero-install, offline, and version-matched to the toolchain that
// generates with it (a first-run network fetch is a failure inside the
// first 15 minutes — the ADR 0041 review round called it disqualifying).
//
// # cljgo new belongs to the LANGUAGE, not to a framework
//
// The default is "lib" (ADR 0047): cljgo is a language that ships a
// great web framework, not a web framework — someone writing a library
// or a tool must not be handed a server. bri is one template among
// three, exactly like `mix new` vs `mix phx.new`. Nothing in this
// package, or in `cljgo new`, knows what bri is; it knows TEMPLATES.
//
// # Directly runnable, in place
//
// Every template is valid, runnable source WITHOUT substitution: the
// app name is not a mustache, it is a REAL default name — DefaultName
// ("newapp") — which the generator renames. `cd templates/cli && cljgo
// test` passes on the tree as it sits; that is exactly why CI can run
// the templates (cmd/cljgo/bri_test.go generates each one, tests it,
// and runs what it produces).
//
// # Adding a template
//
// Drop a directory under templates/, add it to the go:embed line and to
// Builtins below, name the app DefaultName wherever the app's own name
// appears, and add its manifest to cmd/cljgo/templates_test.go.
package templates

import (
	"embed"
	"strings"
)

// FS holds every template tree, rooted at the template name:
// "web/conf.edn", "cli/src/newapp/core.cljg", ... The `all:` prefix is
// load-bearing — a plain embed skips dotfiles, and the template's
// .gitignore is part of the project.
//
//go:embed all:lib all:cli all:web
var FS embed.FS

// DefaultTemplate is the template `cljgo new` uses when --template is
// not given: a library — the smallest honest default, and the one every
// comparable ecosystem picked (`cargo new` is a lib, `mix new` is bare,
// `clj -Tnew :template lib`). ADR 0047.
const DefaultTemplate = "lib"

// DefaultName is the app name the template files are written with. The
// generator replaces this token (in file contents and in path names)
// with the name the user asked for. It is a plain, distinctive word on
// purpose: the templates stay runnable as-is, and the substitution is
// one obvious mechanism with nothing to escape.
const DefaultName = "newapp"

// Builtin is what `cljgo new` knows about a built-in template: its
// name, the one-line summary help prints, and the commands the
// generator suggests next. This is template metadata — it lives here,
// with the templates, so the language's `new` command never grows
// knowledge of any particular framework.
type Builtin struct {
	Name    string
	Summary string
	Next    []string // suggested commands, in order, for the "next:" block
}

// Builtins lists the built-in templates in the order help prints them:
// the default first, then up the stack.
var Builtins = []Builtin{
	{
		Name:    "lib",
		Summary: "a library: namespaces, tests, no main (the default)",
		Next:    []string{"cljgo test    # the generated test"},
	},
	{
		Name:    "cli",
		Summary: "a command-line tool: -main, args, one static binary",
		Next: []string{
			"cljgo test        # the generated test",
			"cljgo build run   # compile it and run it",
		},
	},
	{
		Name:    "web",
		Summary: "a bri web app: routes, config, a styled page",
		Next: []string{
			"cljgo dev     # server + nREPL",
			"cljgo test    # the generated test",
		},
	},
}

// LookupBuiltin returns the named built-in template's metadata.
func LookupBuiltin(name string) (Builtin, bool) {
	for _, b := range Builtins {
		if b.Name == name {
			return b, true
		}
	}
	return Builtin{}, false
}

// BuiltinNames renders the built-in names for an error or help line:
// "lib, cli, web".
func BuiltinNames() string {
	names := make([]string, len(Builtins))
	for i, b := range Builtins {
		names[i] = b.Name
	}
	return strings.Join(names, ", ")
}
