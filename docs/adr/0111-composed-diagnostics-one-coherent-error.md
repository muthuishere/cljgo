# ADR 0111 — composed diagnostics render as ONE coherent error

Date: 2026-07-30 · Status: **accepted** · Refines: ADR 0015 (structured
diagnostics), ADR 0048 (error-message overhaul)

## Context

Consuming a real library from Clojars surfaced a rendering defect the committed
tests never reach (they use `httptest` and hand-written fixtures). Building a
project against `hiccup/hiccup 1.0.5` printed:

```
error: namespace hiccup.core came from a maven dependency and failed to compile on cljgo — … I4002 …
help: in hiccup/hiccup 1.0.5, 6 other namespaces have no Java interop …
note: it came from the maven dependency hiccup/hiccup 1.0.5
help: run `cljgo explain I4002` (expects interop-free at resolve: … got it does not compile on cljgo)
help: report it with the namespace and the error above …
note: it came from the maven dependency hiccup/hiccup 1.0.5     <-- DUPLICATE
note: the resolve report's interop-free count is a READ-time measurement …
note: this is a gap in cljgo, not evidence that the library is JVM-only
help: run `cljgo explain G5020`
```

A G5020 wrapper (a Maven-origin namespace that passed the read-time
interop-free classification and then failed to compile) wrapped an ALREADY
fully rendered I4002. Both layers appended their own notes, fixes and explain
pointer; one note was printed twice, two `explain` pointers competed, and the
outer's `(expects …, got …)` was spliced into the middle of the inner's help
line because the outer message was multi-line.

`medley/medley 1.4.0` composes differently and is the other half of the case:
it classifies as fully interop-free (1 namespace usable) and then fails in the
compiler (`macroexpanding instance?: wrong number of args (1)`), so the wrapper
sits over an **A2004**, not an I4002.

## Decision

1. **Composition is a render-layer concern, not a raise-site one.** A new
   `diag.Normalize` collapses a diagnostic and everything it wraps into one
   value; `diag.Render` and `diag.NewEnvelope` both call it. Raise sites keep
   saying what they know.
2. **`Diagnostic.Causes []Diagnostic`** carries the inner diagnostic
   structurally. The older `%v`-on-a-`DiagError` shape (an inner diagnostic's
   rendered block inside the outer message) is parsed back into structure by
   `Normalize`, so legacy wrapping is fixed too.
3. **Notes and fixes are deduplicated across the chain**, first mention wins,
   order preserved. Only exact repeats are removed — a wrapper's own honesty
   notes survive verbatim.
4. **Exactly one explain pointer, and it is the INNERMOST registered code.**
   The outer code names the CATEGORY the failure falls into; the inner one
   names what actually went wrong, and its page is the one that helps. The
   outer layer's substance is not lost: its message, notes and fixes still
   render, and its code is still the diagnostic's `error_code`.
5. **`--json` carries every code.** The envelope keeps the outer `error_code`
   and the whole chain under `causes`, so tooling sees both even though a human
   reads one pointer.
6. Enrichment (`(expects …, got …)`, the locus) always attaches to the FIRST
   line, and a wrapper with no position of its own inherits its cause's.

`G5020 — maven dependency namespace failed to compile` is registered with an
explain page, and `pkg/emit` raises it when a Maven-origin namespace fails to
compile.

## Consequences

- Uncomposed diagnostics render byte-identically — `Normalize` is a no-op on a
  single-line, cause-free diagnostic, so no frozen error text moves.
- Wrapping an already-diagnosed error is now cheap and safe; the previously
  latent `codedf(..., "%v", err)` sites in `pkg/deps` stop duplicating.
- A user reading a composed error is pointed at the specific page (I4002,
  A2004), which means the general G5020 page must carry its own reasoning in
  `note:` lines rather than relying on being linked.
