# ADR 0048 — Richer, LLM-friendly error rendering

Date: 2026-07-22 · Status: accepted (owner call 2026-07-22) · Builds on
**ADR 0015** (diagnostics data model). Closes **spike s28**
(`spikes/s28-rust-diagnostics/`, VERDICT 2026-07-22).

## Context

cljgo's user-facing errors were a single bare line — `error: %v` — that
threw away most of what `pkg/diag.Diagnostic` already models (code,
location, expected/found, fixes, explain URL). The error-messages audit
(`docs/error-messages-audit-2026-07.md`) measured the damage: ~86
positioned, code-mappable compile-time sites versus ~1,100 bare runtime
`panic`/`fmt.Errorf` sites that carry no code, no position, and bypass the
diagnostic system entirely. Three consequences the owner cared about:

- **Arity errors were anonymous** — `passed to: fn`, where the JVM says
  `passed to: user/f`. You could not tell *which* function you miscalled.
- **Compiled binaries had no error boundary** — the emitted `func main()`
  had no `recover()`, so a runtime error surfaced as a raw Go panic and a
  goroutine stack trace. Same code in the REPL printed `error: …`. That is
  a REPL-vs-binary divergence, the unforgivable failure mode (CLAUDE.md).
- **The one useful affordance, did-you-mean, was REPL-only** — it never
  fired under `cljgo run` or in a compiled binary, which is exactly where
  scripts and agents hit errors.

The owner's bar, stated and then narrowed mid-spike: *"very detailed error
messages like rust and very llm friendly"* → *"no need exactly like rust,
just some more details, that's enough."* So the target is **one richer
error line**, consistent across REPL / `cljgo run` / compiled — NOT a Rust
snippet-and-caret block.

## Decision

1. **One renderer, `diag.Render`.** Every context (REPL, `cljgo run`, the
   emitted binary, and the nREPL `err` string) routes its error through
   `pkg/diag.Render`, which degrades gracefully: a diagnostic with only a
   message still renders; one with a code, location, expected/found, and a
   Fix renders the richer line plus `help:` lines. No new data model — ADR
   0015's `Diagnostic` already carried every field; the win is populating
   and rendering them.

2. **The richer line is: name it, locate it, expected/found, code.** The
   target output (not a multi-line block):

   ```
   error: wrong number of args (3) passed to: user/f (expects 1: [x]) at demo.clj:2:1
   help: run `cljgo explain A2004`
   ```

   Snippet-with-line-numbers and the `^^^^` caret are explicitly OUT — an
   optional future rung, not the bar. The owner walked back the full Rust
   anatomy; the CLAUDE.md doctrine was dialed back to match.

3. **Runtime errors carry their own diagnostic via a `Carrier` seam.** A
   runtime error that knows its code/name/span computes a `Diagnostic` at
   the raise site and implements `diag.Carrier`; `diag.FromError` takes it
   verbatim, winning over after-the-fact prose classification. The
   interface inverts the dependency edge, so `pkg/diag` stays free of the
   runtime packages. `pkg/eval`'s `*arityError` is the first Carrier.

4. **The emitted `func main()` recovers.** A compiled binary catches a
   runtime throw, renders it through the same `diag.Render`, and exits 1 —
   never a Go panic + stack trace. This closes the worst divergence.

5. **`.Error()` stays byte-stable.** All new detail lives on the
   diagnostic and surfaces only through `diag.Render`. Conformance matches
   `err.Error()` via `strings.Contains`, so raw error strings are
   unchanged — the rendered line is the only thing that gains `user/f`.

6. **Rollout is graceful-degradation first, then per-error enrichment.**
   The renderer + boundary + `Driver.RenderError` landed already (spike
   s28, PR #74) and improve every existing error on day one with zero
   migration. From here, the top ~10 runtime errors get a code + Location +
   name registered at their raise sites (the Carrier pattern); the long
   tail keeps working through `classify()` / the `G5000` general code until
   individually enriched. No big-bang rewrite of all ~1,100 sites.

## Consequences

- **Shipped (PR #74):** `diag.Render`, the emit `recover()` boundary,
  `Driver.RenderError`, named+located+arity-detailed arity errors
  (interpreted), and did-you-mean as a structured `Fix` firing under
  `run`. Gates green; conformance byte-stable.
- **Known gap, tracked:** the *compiled* arity error renders cleanly but
  still lacks name/location — its panic is a separate bare `fmt.Errorf` in
  the emitter, not the interpreter's Carrier. Making compiled == interpreted
  for arity (and the other enriched errors) is the first follow-up batch.
- **Surprise the spike found:** `cljgo build` evaluates top-level forms, so
  a top-level runtime error dies at *build* time — which narrows what the
  compiled `main()` boundary actually guards (in-function runtime throws).
  Documented in the VERDICT; the boundary is still correct, just scoped.
- **Codes:** runtime arity reuses `A2004` for now. Whether to open a
  dedicated runtime band vs reuse analyzer codes is deferred to the first
  enrichment batch, decided per-error as raise-site codes are registered.
- Snippet+caret rendering remains available to revisit if the owner ever
  wants it; nothing here forecloses it.
