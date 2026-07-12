# ADR 0018 — REPL ergonomics: Ruby-grade programmer happiness, fallback-only
Date: 2026-07-12 · Status: accepted (first slice implements now; rest staged)

## Context
Ruby's IRB treats the human kindly: `exit` exits, help exists, errors
suggest. JVM Clojure's REPL errors on bare `exit` (ours inherited that).
Owner: Ruby-like ergonomics. Constraint: the precedence principle — the
LANGUAGE gains no new names; all affordances live in the REPL front-end and
must yield to user definitions.

## Decision
1. **Graceful exit words**: bare `exit`, `quit` (and `(exit)`/`(quit)`) at
   the prompt end the session with a friendly farewell — ONLY when the
   symbol does not resolve in the current namespace; a user-defined
   `exit` var always wins. Ctrl-D unchanged.
2. **`help`** (same fallback rule): prints REPL affordances (exit words,
   *1 *2 *3 *e, interrupt behavior, session id when ADR 0016 lands).
3. **Did-you-mean**: "unable to resolve symbol: pritnln" gains
   "did you mean println?" — nearest interned candidates by edit distance
   (≤2) across current-ns mappings; shipped as part of the diagnostic
   (related[] per ADR 0015), so editors get it too.
4. Staged next (design with ADR 0016/0017 rounds): result coloring on tty,
   `doc`/`source` (these are real clojure.repl vars — implemented as the
   genuine article, not sugar), startup tip line.

## Consequences
Zero language surface change; scripts and pipes see identical semantics
(affordances gate on interactive tty + unresolvable-symbol fallback).
Friendliness becomes a stated product value for every future REPL feature.
