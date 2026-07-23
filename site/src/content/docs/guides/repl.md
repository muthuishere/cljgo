---
title: The REPL
description: A tree-walk evaluator that is also the macro engine, session journals with resume, an nREPL server for Calva and CIDER, and the dual-harness guarantee that REPL and binary never diverge.
---

`cljgo repl` starts a terminal REPL on stdin/stdout. Under it is a
tree-walk evaluator (`pkg/eval`) fed by the same reader, analyzer, and
AST as the compiler — the evaluator **is** the macro engine, for both
modes. There is no second implementation of the language: one analyzer,
two backends (interpreter and Go-source emitter), which is what makes
the parity guarantee below structural rather than aspirational.

```
$ cljgo repl
cljgo 0.1.0 (Go 1.26.3, Clojure 1.12.5)
user=> (defmacro unless [c a b] `(if ~c ~b ~a))
#=(var user/unless)
user=> (unless false :yes :no)
:yes
user=> (defn f [x] (* x 2))
#=(var user/f)
user=> (f 21)
42
```

REPL-driven development is priority 2 of the project: live re-`def`,
`defmacro` at the prompt, macro redefinition picked up by later calls —
all conformance-tested (`macro-redefine-live.clj`, `redef.clj`).

## Prompt conveniences

- `*1` `*2` `*3` hold the last three results and `*e` the last error —
  proper dynamic vars, as on JVM Clojure, reverting when the session ends.
- `exit`, `quit`, and `help` work as bare words at an interactive
  prompt. They are fallback-only: a var you define named `exit` always
  wins, and piped scripts see the historical semantics untouched
  (ADR 0018).
- Unresolved symbols get a did-you-mean suggestion.
- Errors render with names, locations, and expected-vs-found detail, and
  carry registered codes — the same renderer as `cljgo run` and compiled
  binaries, so an error reads identically everywhere. See
  [diagnostics](/cljgo/reference/diagnostics/).

## Sessions: journal and resume

Every successful top-level form in an interactive session is appended to
a journal at `~/.config/cljgo/sessions/<id>.journal` — plain, readable
Clojure (failed forms are journaled as comments, visible but never
replayed). Two in-session commands manage them (ADR 0016):

```
user=> :sessions            ; list journals
user=> :resume <id>         ; replay a journal, continue journaling to it
```

Journaling is on only when stdin is a terminal; `CLJGO_SESSION=1`
forces it on (and `=0` off), so scripts and pipes stay clean.

## nREPL for editors

```
$ cljgo nrepl [--port N]
nREPL server started on port 55123 on host 127.0.0.1 - nrepl://127.0.0.1:55123
```

The server listens on 127.0.0.1 (default: an ephemeral port) and writes
a `.nrepl-port` file in the cwd, removed on shutdown, so editors
auto-discover it. Connect Calva ("Connect to a running REPL") or CIDER
(`cider-connect-clj`) to the printed port.

The op surface is babashka's proven 13 (ADR 0031): `clone`, `close`,
`describe`, `eval`, `load-file`, `complete`, `completions`, `lookup`,
`info`, `eldoc`, `ls-sessions`, `interrupt`, `ns-list` — with per-session
`*ns*`/`*1`/`*out*` streaming. One honest caveat: `interrupt` is a
per-spec stub — the tree-walk evaluator has no cancellation checkpoint
yet (babashka shipped that way for years).

## The dual-harness guarantee

The same source runs interpreted at the prompt and compiles to a native
binary — **with byte-identical output on both paths**. This is not a
goal; it is a gate:

- Every semantic behavior is a file in `conformance/tests/` (416 files)
  with frozen expected output, verified against real JVM Clojure 1.12.5
  as the oracle, and run through **both** the evaluator and the compiled
  binary on every commit.
- A REPL-vs-binary divergence is classified as a release blocker.
- Where the two legs genuinely differ in capability — the interpreter
  cannot link a third-party Go module — the rule is a **hard error in
  the leg that cannot satisfy it**, never a silently different value
  (ADR 0053).

The practical consequence: what you explore at the REPL is what
`cljgo build` ships. See
[architecture](/cljgo/reference/architecture/) for how the single
analyzer feeds both backends.

## What the interpreter costs

The tree-walker boots in ~32 ms and is the slow leg by design — it
exists for interactivity, not throughput (`cljgo run hello.clj` is
~38 ms wall clock; the compiled hello is ~5 ms). When speed matters,
[compile it](/cljgo/guides/compile/). Interpreter boot time is
budget-gated in CI so it cannot silently regress.
