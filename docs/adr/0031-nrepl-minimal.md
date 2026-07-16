# ADR 0031 — nREPL: babashka's op surface, own bencode, sessions on dynamic bindings
Date: 2026-07-16 · Status: accepted · Evidence: spike S15 (spikes/s15-nrepl-minimal/VERDICT.md)

## Context

Editor adoption requires nREPL (owner priority 2). S15's prototype passed
a scripted wire-level session AND a real nREPL 1.3.1 client connect/eval.
Calva needs zero ops beyond babashka's proven 13; CIDER connects and
degrades gracefully without cider-nrepl middleware.

## Decision

- **Op surface v1**: clone close describe eval load-file complete
  completions lookup info eldoc ls-sessions interrupt ns-list.
- **Bencode**: our own ~163-line codec. cljgo keeps zero external deps.
- **Architecture**: new `pkg/nrepl` fronting a shared `repl.Session`
  helper (Driver is not directly reusable). The load-bearing win from the
  spike: dynamic bindings are goroutine-keyed, so one session = one
  goroutine holding the same *ns*/*1/*2/*3/*e frame — isolation is free.
- **Prerequisite fix**: println/print write to package-global eval.Out,
  not the *out* dynamic var — print builtins must honor lang.VarOut
  first (also unblocks with-out-str).
- **Interrupt**: honest per-spec stub (the tree-walk evaluator has no
  cancellation checkpoint yet; babashka shipped that way for years).

## Consequences

CIDER/Calva connect to cljgo. A `doc` macro gap surfaced during the real-
client test rides along with the implementation.
