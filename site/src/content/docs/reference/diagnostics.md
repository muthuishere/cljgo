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

All codes below have explain pages (`cljgo explain <code>`). The E3xxx band is
reserved but has no registered codes yet.

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
| R1010 | reader conditional splicing at top level | v0.8.0 |
| R1011 | conditional read not allowed | v0.8.0 |
| R1012 | reader conditional supplies no branch for this platform | v0.8.0 |
| A2001 | unable to resolve symbol | M2 |
| A2002 | recur outside tail position | M2 |
| A2003 | recur argument count mismatch | M2 |
| A2004 | wrong number of forms in special form | M2 |
| A2005 | def name is not a symbol | M2 |
| A2006 | malformed binding vector | M2 |
| A2007 | invalid binding form | M2 |
| A2008 | conflicting fn overloads | M2 |
| A2009 | no such namespace | v0.8.0 |
| I4001 | Java class used as a namespace | v0.8.0 |
| I4002 | namespace requires Java interop and cannot load on cljgo | v0.8.0 |
| G5000 | uncategorized compiler error | M2 |
| G5001 | value is not a number | M5 |
| G5002 | value is not a function | M5 |
| G5003 | value is not seqable | M5 |
| G5004 | index out of bounds | M5 |
| G5005 | value is not a collection | M5 |
| G5006 | divide by zero | M5 |
| G5007 | no value supplied for key | M5 |
| G5008 | sql params passed as a collection | M5 |
| G5009 | collection value in a row map | M5 |
| G5010 | maven coordinate not found | v0.8.0 |
| G5011 | unsupported Maven POM feature | v0.8.0 |
| G5012 | maven artifact checksum mismatch | v0.8.0 |
| G5013 | maven version conflict | v0.8.0 |
| G5014 | offline: maven coordinate unavailable | v0.8.0 |
| G5015 | conflicting dependency coordinates | v0.8.0 |
| G5016 | replacement fn did not return a string | v0.8.0 |
| G5017 | options argument is not a map | v0.8.0 |
| G5018 | unsupported dependency version syntax | v0.8.0 |
| G5019 | maven dependency source file cannot be read | v0.8.0 |
| G5020 | maven dependency namespace failed to compile on cljgo | v0.8.0 |
| G5021 | frozen build: build.lock.edn does not match build.cljgo | v0.8.2 |
| G5022 | unsupported or unmatched java.time date pattern | v0.8.2 |
| G5023 | cljgo run: project declares dependencies but has no build.lock.edn | v0.8.3 |
| G5024 | cannot resolve real-path (missing path or symlink cycle) | v0.8.3 |
| G5025 | build file defines no build entry point | v0.8.5 |
| G5026 | file loaded but did not define the required namespace | v0.8.6 |

### The dependency-resolution codes

`I4002`, `R1012` and the `G501x` band are what you meet when
[consuming a Clojars library](/cljgo/guides/deps-publish/). They exist so a
library that cannot work on cljgo **fails loudly at `require`** rather than
half-loading:

- **`I4002`** — the namespace genuinely needs Java interop. Gating is per
  *namespace*, not per library: one jar routinely mixes both, so the pure
  namespaces in the same jar stay usable.
- **`R1012`** — a `.cljc` whose real body is `:clj`-only, i.e. the reader
  conditional leaves cljgo nothing. Without this it would silently load an
  empty namespace.
- **`G5011`** — a Maven POM feature the resolver does not implement.
  Unimplemented means *name-error*, never half-resolve.

One stability guarantee worth knowing: the rendered `.Error()` string stays
byte-stable — the conformance suite freezes it — and the extra detail (locus,
help lines) is added at the render layer.
