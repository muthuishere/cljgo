# ADR 0078 — bri.cli: the CLI app-shape of bri (one parameter → both a CLI arg and an interactive prompt; best-in-class UI backend)

Date: 2026-07-24 · Status: proposed (owner-directed: *"start with cli"*; *"the
inputs i want is both interactive and cli args as well by default … people can
add name description type parameter … progress and all stuff should be the
best … i am saying bubble but i just want the very best, if we want we can build
our own"*). Depends on ADR 0074/0076 (opt-in linking), ADR 0077 (`cljgo dist`),
ADR 0047 (real templates). First of the bri.cli block (0078–0081).

## Context

bri is the batteries-included **application** framework for cljgo, and web is
only one app-shape (bri.http/auth/db/otel). The other first-class shape is the
**CLI** — and cljgo is an exceptional CLI host: `cljgo build` yields one static
`CGO_ENABLED=0` binary with ~5 ms startup, and `cljgo dist` (ADR 0077)
cross-compiles it to every platform from one machine. What's missing is the
framework layer that makes writing a good CLI as easy as `bri.http` makes writing
a web service.

There is already a `cljgo new --template cli` scaffold (ADR 0047), but it is a
bare `-main` with hand-rolled arg handling. bri.cli is the framework behind it.

Two owner constraints shape the design:

1. **Inputs are dual-surface by default.** A parameter should be declared **once**
   — a name, a description, a type — and *automatically* be usable **both** as a
   non-interactive CLI argument/flag **and** as an interactive prompt, with type
   validation, with no extra wiring for either. Most toolkits make you choose (a
   flags library *or* a prompt library, wired separately); bri.cli unifies them:
   one declaration, two synchronized surfaces.
2. **Best-in-class UI, backend not pre-committed.** The owner named Bubble Tea but
   clarified the real requirement is *"the very best … if we want we can build our
   own."* So this ADR commits to the **Clojure surface and behavior**, and defers
   the rendering **backend** to a spike (s46): the leading candidate is the
   **Charm** stack (`bubbletea`/`bubbles`/`lipgloss`/`huh`) — best-in-class and
   verified **pure Go** / `CGO_ENABLED=0` (so it does not threaten the static
   binary or the ADR 0077 cross-compile guarantee) — with a bespoke renderer on
   the table if it proves better. The surface below does not depend on the choice.

## Decision

### 1. `bri.cli` is an opt-in bri namespace

`core/bri/cli.cljg` + an isolated Go shim package `pkg/bri/cli`, marked `OptIn`
in `bri.Specs()` exactly like bri.db (ADR 0076) and bri.otel (ADR 0074): the UI
backend's dependencies link **only** when an app requires `bri.cli`, so they cost
a web app (or a plain cljgo program) nothing. Reuses the standard opt-in
machinery (`ShimImport` + `RegisterInstaller` + genbri `provider.go` +
`OptInBriPkgs`) with zero new mechanism — the third namespace to prove it.

### 2. The unified parameter model (the load-bearing idea)

A command declares its **parameters** once. Each parameter is a small map — a
`:name`, a `:type`, a human `:about` — and from that single declaration bri.cli
derives the CLI flag/positional, the parse + validation, the `--help` entry, AND
the interactive prompt widget:

```clojure
(require '[bri.cli :as cli])

(cli/defcli app
  {:name "deploy" :version "1.0" :about "Ship a release"}
  (cli/command "release" {:about "Cut and upload a release"
    :params
    [{:name :version  :type :string  :about "semver tag"        :required true}   ; positional or --version
     {:name :channel  :type :enum    :about "release channel"   :one-of [:stable :beta] :default :stable}
     {:name :notes    :type :string  :about "release notes"     :multiline true}
     {:name :token    :type :string  :about "GitHub token"      :secret true :env "GITHUB_TOKEN"}
     {:name :yes      :type :bool    :about "skip confirmation"}]}
    (fn [{:keys [version channel notes token yes]}]
      ...)))

(defn -main [& args] (cli/run app args))
```

**Per-parameter resolution order**, applied automatically by `cli/run`:

1. the CLI **flag/positional** if the user passed it (`--channel beta`, or the
   bare positional for the first param);
2. the **`:env`** var if named and set;
3. an **interactive prompt** — *only* when attached to a TTY and the value is
   still missing (and the param is `:required` or `:prompt true`); the prompt
   **widget is chosen by `:type`**: `:enum`/`:one-of` → a **select**, `:bool` → a
   **confirm**, `:secret` → a **masked password**, `:multiline` → an editor,
   otherwise a validated **text input**;
4. the **`:default`**;
5. otherwise, for a `:required` param in a **non-interactive** context, a named
   error (`diag.Render`, ADR 0015) with the flag name and expected type — never a
   silent hang.

This is what "both interactive and CLI args by default" means concretely: the
same `release --channel beta v1.2.3` a human types in a script, an agent invokes
non-interactively (all params as flags, no prompts), and a human runs bare
(`release` → prompted for each missing param) — **all from one declaration**.
Types (`:string :int :bool :keyword :enum :file :path :multiline`) drive parsing,
validation, help, and the prompt widget uniformly. The precedence principle
holds: every public is under the `cli/` alias; nothing shadows clojure.core.

### 3. Subcommands, help, and errors — free from the tree

`cli/run` parses argv against the command tree and, for free, generates
`--help`/`-h` at every level (built from each param's `:about`), `--version`,
usage-on-error, and did-you-mean suggestions rendered as `diag` `Fix`es — so a
bri.cli app's errors read like the rest of cljgo, in every context.

### 4. Progress, status, and output — best-in-class, as data

First-class, non-negotiable-quality progress reporting:

```clojure
(cli/spinner "Building…" (fn [] (do-work)))          ; auto start/stop, TTY-aware
(cli/progress {:total n} (fn [tick] (doseq … (tick))))  ; a real progress bar
(cli/steps ["compile" "link" "upload"] …)             ; multi-step status
(cli/table rows {:cols […]})                          ; aligned tables
```

Styling is expressed as cljgo data (`{:bold true :fg :green}`), composed rather
than escaped by hand, and honors `NO_COLOR`/non-TTY automatically (a progress bar
degrades to plain line output when piped). These map onto the chosen backend
(Charm's `bubbles`/`lipgloss` if selected) behind a stable Clojure surface.

### 5. Full TUIs when wanted — the escape hatch

For an app that needs a live TUI (a picker, a dashboard), bri.cli exposes the
Elm-architecture loop as data — `{:init … :update (fn [model msg] …) :view (fn
[model] …)}` driven by `(cli/run-tui model)`. Opt-in depth: simple CLIs never
touch it; the ceiling is a real TUI, not a println loop.

### 6. Backend selected by spike s46, surface frozen here

s46 evaluates the Charm stack vs a bespoke renderer against: pure-Go /
`CGO_ENABLED=0` (mandatory — a `go list -deps` zero-cgo gate, like ADR 0074/0076),
binary-size cost (opt-in, so it's the CLI binary's alone), cross-compile parity
(ADR 0077), and how cleanly it serves the §2 unified model and §4 progress. The
DSL in §2–§5 is the contract and does not change with the outcome.

## Consequences

- Declaring a CLI in Clojure becomes turnkey and *correct by construction*: one
  parameter list yields a scriptable flag interface, a friendly interactive mode,
  type validation, and generated help — the dual-surface behavior the owner
  wants, not two libraries stitched together.
- The same command is first-class for humans (prompts fill the gaps) and agents
  (all params as flags, non-interactive, `--json` per ADR 0081) — because
  resolution is one ordered pipeline over one declaration.
- Pure-Go throughout regardless of backend (mandatory gate), so a bri.cli app
  still AOT-compiles to a `CGO_ENABLED=0` static binary and still cross-compiles
  via `cljgo dist`.
- Opt-in linking keeps the UI deps out of every non-CLI binary; bri.cli is the
  third user of that mechanism, hardening it as the battery standard (ADR 0075).
- Dual-mode parity applies to the deterministic half (parse/dispatch/help/
  validation — conformance-testable, identical interpreted vs compiled); the
  interactive prompts/progress/TUI are tested against scripted stdin + the
  non-interactive resolution path (no JVM oracle, like the rest of bri).
- Sets up the block: 0079 ships bri.cli apps to npm+Go, 0080 gives them built-in
  API auth, 0081 makes them agent-skill-native.
- Not chosen: separate flags-vs-prompt libraries the user wires by hand (the
  unification is the point); hard-committing to Bubble Tea before the spike (the
  owner wants the best, even if bespoke); a flags-only CLI with no interactivity;
  making bri.cli always-linked (it would bloat every web binary).
