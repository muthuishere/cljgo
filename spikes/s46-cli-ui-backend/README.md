# Spike s46 — bri.cli's interactive UI backend: Charm vs bespoke

ADR 0078 §7 froze the bri.cli Clojure surface and deferred the **rendering
backend** to this spike. The owner: *"i am saying bubble but i just want the
very best — if we want we can build our own."* This spike measures both against
the mandatory gates so the decision is evidence, not taste.

## The question

bri.cli increment 2 needs an interactive layer: when a param is missing on a
TTY, prompt for it with a **type-driven widget** (text · int · confirm for
`:bool` · select for `:enum`/`:one-of` · multi-select for `:multi` · password
for `:secret` · editor for `:multiline`), plus **progress**/**spinner** for the
output side (ADR 0078 §4/§5). Two candidates:

- **charm/** — the Charm stack: `huh` (forms/prompts), `bubbletea` (the
  Elm-architecture TUI runtime), `lipgloss` (styling), `bubbles` (components).
- **bespoke/** — a from-scratch pure-Go prompt kit (stdin + `golang.org/x/term`
  + ANSI), the minimum that could serve the same widgets.

## Gates (from ADR 0078 §7)

1. **Pure-Go / `CGO_ENABLED=0`** — mandatory (the static-binary + `cljgo dist`
   cross-compile guarantees, ADR 0023/0077). A `go list -deps` cgo check + a
   `CGO_ENABLED=0` build.
2. **Cross-compile** — builds for the ADR 0077 matrix from one host.
3. **Binary-size cost** — opt-in, so it is the CLI binary's alone; but smaller
   is better.
4. **Widget coverage + quality** — does it serve every widget the unified model
   needs, and how good is the UX (arrow-key select, filtering, inline
   validation, theming, `NO_COLOR`/non-TTY degradation).
5. **Integration effort** — how cleanly a `pkg/bri/cli` Go shim drives it from
   the Clojure layer, and how much code WE own/maintain.

## Layout

- `charm/` — imports huh + bubbletea + lipgloss, exercises every widget, and a
  `--measure` path that links them all without needing a TTY.
- `bespoke/` — a minimal pure-Go prompt kit (text/password/confirm/select),
  same measurement surface.
- `measure.sh` — builds both `CGO_ENABLED=0`, checks cgo, cross-compiles the
  matrix, and reports binary sizes + dep counts.
- `VERDICT.md` — findings + recommendation (feeds an ADR 0078 backend note).
