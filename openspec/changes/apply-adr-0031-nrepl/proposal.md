# apply-adr-0031-nrepl

## Why

ADR 0031 (docs/adr/0031-nrepl-minimal.md, accepted) settles cljgo's editor
story on the evidence of spike S15 (spikes/s15-nrepl-minimal): a minimal
nREPL server with babashka's proven 13-op surface passed a scripted wire
session AND a real nREPL 1.3.1 client. The ADR's prerequisite — print
builtins honoring the `*out*` dynamic var (`lang.VarOut`) — landed in
design/08 batch E (PR #22), so the spike's server-wide eval mutex around
the package-global `eval.Out` is obsolete: per-session output streaming is
now a plain thread binding. This change productionizes the ADR.

## What Changes

- New `pkg/nrepl`: TCP nREPL server, own bencode codec (adapted from the
  spike's ~163-line one — zero external deps, per the ADR), session
  registry. One goroutine per session holding the session binding frame
  (`*ns*` `*1` `*2` `*3` `*e` + `*out*`) — dynamic bindings are
  goroutine-keyed in pkg/lang, so isolation is free (the spike's
  load-bearing finding).
- Op surface v1 (all 13, what `describe` advertises): clone close describe
  eval load-file complete completions lookup info eldoc ls-sessions
  interrupt ns-list. `interrupt` is the honest per-spec stub (status
  `done` + `session-idle` when idle; the tree-walk evaluator has no
  cancellation checkpoint — babashka shipped this way for years).
- `*out*` streaming: each session's frame binds `lang.VarOut` to a writer
  that emits nREPL `out` messages; `println`/`print`/`printf` output
  streams per session with no global state and no mutex.
- Shared `repl.Session` helper in pkg/repl (smallest possible export): the
  session binding frame + `*1 *2 *3` result shift + `*e` error recording,
  extracted from Driver (which now fronts it) — the spike found Driver
  itself is not reusable (line-oriented stdin, unexported evalAndPrint,
  EvalString skips `*1`).
- New `cljgo nrepl [--port N]` subcommand: default port 0 = ephemeral +
  print `nREPL server started on port N on host 127.0.0.1 -
  nrepl://127.0.0.1:N` and write `.nrepl-port` (just the digits) in the
  cwd, removed on shutdown — verified against nrepl.org/usage/server
  (nrepl.cmdline writes the port file for editor auto-discovery; deletes
  on exit).
- `doc` macro (the gap the spike's real-client test surfaced): a new
  embedded `clojure.repl` namespace (`core/repl.cljg`) with `doc` +
  `print-doc`, `doc` referred into `user` at boot exactly as JVM
  clojure.main's repl-requires does. Output shape oracle-verified against
  `clojure.repl/doc` on JVM Clojure 1.12.5. Known fidelity limits, stated:
  core.clj defns carry no docstrings/arglists yet (defn's attr-map
  DEVIATION predates this change), so `(doc map)` prints only the
  separator + qualified name; user vars with docstrings print them.
- README: nREPL row in the Status table + a Try-it one-liner.

## Non-goals

- Real interrupt (evaluator cancellation checkpoints) — recorded in the
  ADR as deferred; the stub is spec-honest.
- cider-nrepl middleware ops (macroexpand, test, stacktrace, debugger) —
  CIDER degrades gracefully without them, same as against babashka.
- Docstrings/arglists for core.clj's own defns — separate core work.
