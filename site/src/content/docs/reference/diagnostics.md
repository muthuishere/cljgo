---
title: Error codes & diagnostics
description: The cljgo diagnostic model — one richer error line with a name, location, expected-vs-found, and a help pointer — plus the banded code registry and cljgo explain.
---

cljgo errors follow one doctrine: **one richer error line** — named, located,
expected-vs-found, with a cheap `help:` pointer. Not a Rust-style
snippet-and-caret block; just enough detail that you never see a bare
`wrong number of args passed to: fn`.

## The model

Every user-facing error is a structured `Diagnostic` (`pkg/diag`, ADR 0015)
rendered by a single renderer, `diag.Render`. The rules:

- **Name the thing.** Arity errors name the fn like the JVM —
  `passed to: user/f`, never `passed to: fn`. Same for vars, namespaces,
  protocols.
- **Location when known.** If the error has a source position, the locus is
  appended: `at file:line:col`. No source snippet, no caret.
- **Expected vs found.** Whenever the shape is expected-vs-actual (arity,
  type, arg count), both are stated: `(expects 1: [x])`.
- **A registered code with an explain pointer.** Codes come from an
  append-only banded registry; the renderer appends
  ``help: run `cljgo explain <CODE>` ``.
- **Suggestions are fixes, not prose.** Did-you-mean renders as a `help:`
  line and fires in every context, not just the REPL.
- **Identical everywhere.** The REPL, `cljgo run`, compiled binaries, and the
  nREPL `err` string all call the same renderer; emitted binaries recover
  panics and route them through it too. A raw Go panic with a goroutine stack
  trace reaching a user is treated as an unforgivable failure, same bar as a
  conformance divergence.

Before and after, on the canonical case:

```
error: wrong number of args (3) passed to: fn                              ← bare

error: wrong number of args (3) passed to: user/f (expects 1: [x]) at demo.clj:2:1
help: run `cljgo explain A2004`
```

## Tooling

```
cljgo check file.clj [--json]   # analyze, report diagnostics
cljgo explain A2004 [--json]    # long-form explain page for a code
```

`--json` emits the full `diag.Envelope` — code, location, expected/found,
fixes, related notes, explain URL — so editors and agents consume errors
without parsing prose. Explain pages are embedded in the binary at build time
from [`docs/diagnostics/`](https://github.com/muthuishere/cljgo/tree/main/docs/diagnostics).

## The banded registry

Codes live in
[`pkg/diag/registry.go`](https://github.com/muthuishere/cljgo/blob/main/pkg/diag/registry.go),
one band per compiler stage so a code's origin is readable at a glance:

| Band | Range | Stage |
|---|---|---|
| R | R1xxx | Reader |
| A | A2xxx | Analyzer |
| E | E3xxx | Emitter |
| I | I4xxx | Interop |
| G | G5xxx | General (runtime errors carry raise-site codes here) |

The registry is **append-only**: codes are never removed, renumbered, or
retitled, and a committed lock file (`docs/diagnostics/registry.lock`) is
enforced by a test. Every registered code ships an explain page.

## Registered codes

All codes below have explain pages (`cljgo explain <code>`). The E3xxx and
I4xxx bands are reserved but have no registered codes yet.

| Code | Title | Since |
|---|---|---|
| R1001 | unterminated form | M2 |
| R1002 | unmatched delimiter | M2 |
| R1003 | map literal with odd number of forms | M2 |
| R1004 | duplicate key in map or set literal | M2 |
| R1005 | invalid token | M2 |
| R1006 | invalid number literal | M2 |
| R1007 | invalid escape sequence in string | M2 |
| R1008 | invalid character literal | M2 |
| R1009 | invalid metadata | M2 |
| A2001 | unable to resolve symbol | M2 |
| A2002 | recur outside tail position | M2 |
| A2003 | recur argument count mismatch | M2 |
| A2004 | wrong number of forms in special form | M2 |
| A2005 | def name is not a symbol | M2 |
| A2006 | malformed binding vector | M2 |
| A2007 | invalid binding form | M2 |
| A2008 | conflicting fn overloads | M2 |
| G5000 | uncategorized compiler error | M2 |
| G5001 | value is not a number | M5 |
| G5002 | value is not a function | M5 |
| G5003 | value is not seqable | M5 |
| G5004 | index out of bounds | M5 |
| G5005 | value is not a collection | M5 |
| G5006 | divide by zero | M5 |
| G5007 | no value supplied for key | M5 |

One stability guarantee worth knowing: the rendered `.Error()` string stays
byte-stable — the conformance suite freezes it — and the extra detail (locus,
help lines) is added at the render layer.
