# Spike s47 — VERDICT: build the native, portable TUI (no Charm) — feasible, and it scales

Date: 2026-07-24 · Owner override of s46. Feeds a revision of ADR 0078 §7.

## Recommendation

**Build our own** minimal Elm-architecture TUI, in **portable Clojure** over a
thin per-host terminal primitive. It is ~5× smaller than Charm, pure-Go on
cljgo, and — the decisive win — it makes bri.cli **work on the JVM too**, which a
Go-only Charm dependency structurally cannot. The fundamentals are small and
proven here; they scale from a prompt to an editor. This matches the owner's
"size + performance + reach, expand from right fundamentals."

## Measured (M5 Pro, go1.26.3, `CGO_ENABLED=0 -s -w -trimpath`)

| | **Native (s47)** | Charm (s46) | bespoke-prompts (s46) |
|---|---|---|---|
| Marginal size (over empty Go bin) | **+0.53 MB** | +2.66 MB | +0.66 MB |
| cgo in closure | 0 | 0 | 0 |
| Cross-compile (matrix) | ✅ | ✅ | ✅ |
| Dep modules | 3 | 47 | 3 |
| LOC | **288** (loop + diff render + input + **select + editor**) | 91 (to use) | 177 (prompts only) |

The native runtime is **~5× smaller** than Charm and, unlike the s46 bespoke
prompt kit, it includes the real **fundamentals** (the Elm loop + a diff
renderer) — so it does NOT stop at prompts. The same 288 LOC already drives a
multi-line **editor** (buffer, cursor, insert/delete/newline, arrow-nav): the
proof that "an editor like opencode" is the *same loop*, not a different
technology.

## Reframing finding: opencode IS the Elm loop

opencode's TUI is built on **Bubble Tea** (Elm model→update→view; the SST rewrite
keeps a Bubble Tea Go TUI as one frontend over a TS server). So "opencode-class"
== owning the Elm loop + widgets — precisely what this spike builds in ~150 LOC
of core. We are not out-inventing Charm; we are building the same well-understood
loop, minimal and ours.

## Portability (the payoff Charm can't match)

- **Portable half (Clojure):** the Elm loop, the diff renderer, the input-event
  model, and every widget are plain data + fns — pure Clojure, running
  identically on cljgo and the JVM. bri.cli's existing deterministic core
  (increment 1) is already pure Clojure; the interactive layer joins it.
- **Platform half (~30 LOC each):** the terminal primitive — enter raw mode,
  read key bytes, write ANSI. On **cljgo** it is Go `x/term` via interop (pure-Go,
  +0.53 MB, cross-compiles). On the **JVM** it is JLine (or `System.console` +
  a tiny native shim). One small interface, two implementations.
- Result: a bri.cli app publishes to BOTH ecosystems (ADR 0054 ethos) — a
  cljgo static binary AND a JVM-Clojure library — from one codebase. Charm makes
  the interactive layer cljgo-only forever.

## Scale-to-opencode assessment

opencode-class = this TUI core + an LLM/tool layer (bri.ai / toolnexus, deferred).
The TUI core needed: the Elm loop (have it), a diff renderer (have it), a widget
protocol (have it), and richer widgets built on top — a scrollable viewport, a
list with filtering, a text editor (prototyped), a status/prompt line. Each is a
`(model,update,view)` value; none needs new fundamentals. So the ceiling is a
real TUI, reached incrementally on one small runtime.

## Honest costs / risks (bounded, not free)

Owning the renderer means owning the edge cases Charm already solved: full
escape-sequence coverage (home/end/page/meta), **SIGWINCH resize**, **unicode
display width** (CJK/emoji — the one place `go-runewidth` earns its keep; we'd
vendor or port a width table), **bracketed paste**, and mouse (if ever wanted).
These are incremental hardening, not blockers — ship prompts first, grow the
widget set. The renderer stays a line-diff (simple) until a full-screen app needs
a cell grid; that upgrade is contained to the renderer.

## Roadmap (revises ADR 0078 §7 — native, not Charm)

1. **Terminal primitive** — `pkg/bri/cli` (cljgo) wrapping `x/term`: raw
   mode, read-key, write, size, is-tty. Opt-in linked (ADR 0074/0076); a
   `go list -deps` zero-cgo gate. (JVM/JLine primitive is a later, additive host.)
2. **Portable TUI core in Clojure** (`bri.cli` / a `bri.term` ns): Key events,
   the Elm `run` loop, the line-diff renderer, the widget protocol.
3. **bri.cli prompts as widgets** — input/select/multiselect/confirm/password/
   editor, keyed by param `:type`, each running the param's validators (ADR 0078
   §3). Wire into resolution slot 3 (prompt-if-TTY) + `:env`.
4. **Grow** — viewport, filtering, resize, unicode width — as needed; the
   opencode-class ceiling is reached on this one runtime when the AI layer lands.

## Not chosen

- **Charm** (s46 recommendation) — best-in-class but Go-only (no JVM reach),
  +2.66 MB, 47 deps. Overridden on size + performance + portability. s46 remains
  a valid measurement; this is a different objective function (reach + minimalism
  over convenience).
