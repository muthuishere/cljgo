# ADR 0047 — `cljgo new` is a language scaffolder, not a web-framework scaffolder

Date: 2026-07-17 · Status: accepted (owner call 2026-07-17) · Executes
**ADR 0041** (keel) — it does not supersede it: keel's design is
untouched, only where the LANGUAGE's `new` command points.

## Context

ADR 0041 tier 0 shipped `cljgo new`, and the generator's only template
was the keel web app: `templates.DefaultTemplate = "web"`. So `cljgo
new myapp` — the language's project command — handed everyone a server,
a `conf.edn`, and a stylesheet.

The layering is backwards. cljgo is a language that SHIPS a great
framework; it is not a web framework. Someone writing a library or a
command-line tool is not an edge case here — a tool that starts in
milliseconds as one static binary is arguably cljgo's best story, and
`cljgo new` was handing that author a web server to delete.

Every comparable ecosystem made the same call, and none of them
defaults a language tool to a web app:

- **Rust** — `cargo new` is a binary crate, `cargo new --lib` a
  library; the web frameworks (axum, actix) ship no generator that owns
  `cargo new`.
- **Elixir** — `mix new` is a bare project. Phoenix, the batteries-
  included web framework, is a SEPARATE generator: `mix phx.new`. The
  language's `new` never grew Phoenix knowledge.
- **Clojure** — `clj -Tnew :template lib` (or `app`); Luminus/Kit are
  templates you name, never the default.
- **Go** — `go mod init` makes a module, nothing more.

The precedent is unanimous: the language's scaffolder is
framework-agnostic, and the framework is a named template.

## Decision

1. **`cljgo new` knows about TEMPLATES, never about keel.** Its default
   is `lib`. The command walks a template FS, renames the app, and
   writes; per-template metadata (summary, "next:" commands) lives in
   `templates/`, beside the templates, not in the command.
2. **Three built-ins**, `--template <name>`:
   - **`lib` (the default)** — a library: `src/<name>/core.cljg`, a
     test, `build.cljgo`, `README.md`, `.gitignore`. No server, no
     keel, no `conf.edn`. Nothing runs at load; `cljgo test` is a
     library's build.
   - **`cli`** — a command-line tool: `-main`, argument handling, and a
     `build.cljgo` that produces one binary. This template is the home
     of the single-static-binary / fast-startup pitch.
   - **`web`** — today's keel app, content unchanged.
3. **`--template <path>`** keeps taking a local directory; a git URL is
   still refused honestly (ADR 0041 follow-up).
4. **keel stays a library that ships in the box.** ADR 0041 is
   otherwise intact: keel is a framework, shipped as plain libraries
   with the toolchain, reached by `--template web`.
5. **A plan that declares no artifacts is a library, not a broken build
   file.** `cljgo build` in a `lib` project says "nothing to build —
   build.cljgo declares no artifacts" and exits 0, rather than failing
   with "no install step", which reads as a typo.
6. **CI runs all three generated projects** — the anti-rot property ADR
   0041 task 0.3 bought for `web` now covers every shipped template:
   generate → `cljgo test`; `cli` additionally compiles and EXECUTES
   its binary; `web` additionally boots and curls its page.

## Consequences

- The keel tutorial opens with `cljgo new myapp --template web`. One
  extra flag on the framework's first line is the price of not lying
  about what the language is; the guide says so in place.
- `cljgo dev`, `cljgo config` and `cljgo routes` remain keel-SHAPED
  commands sitting in the language's CLI (`dev` requires
  `src/app/main.cljg`; `config` requires `conf.edn`; `routes` evaluates
  `keel.http/describe`). This ADR does not move them — `new` was the
  one that mattered, because it is what a first-time user types — but
  their layering is an open question. `mix` did not grow `phx.server`;
  Phoenix did.
- Each future tier of ADR 0041 updates the `web` template. The
  generator/page contract of ADR 0041 §2 now reads "the `web`
  template's page", not "the generated app".
- Not chosen: a `bin`/`app` default (Rust's `cargo new` default is a
  binary; a library is the smaller claim and the more common Clojure
  artifact); a separate `keel new` command (one generator, named
  templates — Rails' model, not Phoenix's, because keel ships with the
  toolchain rather than as an installed package).
