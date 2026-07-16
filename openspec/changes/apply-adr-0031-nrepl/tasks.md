# Tasks — apply-adr-0031-nrepl

## 1. Shared session helper

- [x] 1.1 `pkg/repl`: extract `repl.Session` (binding frame `*ns* *1 *2 *3
  *e`, `RecordResult` result shift, `RecordError`) from Driver; Driver
  fronts it. Gates green.

## 2. pkg/nrepl

- [x] 2.1 `bencode.go` + round-trip tests, adapted from
  spikes/s15-nrepl-minimal (read-only reference; never edited). Gates green.
- [x] 2.2 `server.go`: TCP server, session registry, one goroutine per
  session pushing `repl.Session.Bindings()` + `*out*` bound to a
  per-session `out`-message writer (no eval mutex — batch E made the print
  family honor `lang.VarOut`). All 13 ops; interrupt = honest stub. Gates
  green.
- [x] 2.3 Port the spike's scripted wire-session test: clone → describe →
  eval → out-streaming → `*1` → error shape → load-file → interrupt →
  lookup → complete → session isolation → ls-sessions → close. Ephemeral
  ports, no sleeps. Gates green.

## 3. CLI

- [x] 3.1 `cljgo nrepl [--port N]` in cmd/cljgo: default 0 = ephemeral,
  print the nrepl.cmdline-style banner, write/remove `.nrepl-port`
  (verified convention: nrepl.org/usage/server). Usage text updated. Gates
  green.

## 4. doc macro

- [x] 4.1 `core/repl.cljg` (embedded `clojure.repl` ns): `print-doc` +
  `doc`, `doc` referred into `user` at boot (JVM repl-requires parity);
  loader `loadClojureRepl` in pkg/eval. Gates green.
- [x] 4.2 `conformance/tests/doc-macro.clj`: with-out-str-captured doc
  output for a documented def, a bare def, and an unresolvable symbol,
  frozen against the real `clojure` CLI 1.12.5 (eval-harness waiver:
  clojure.repl is REPL tooling, no emitter surface). Gates green.

## 5. Docs

- [x] 5.1 README: Status table row + Try-it one-liner (`cljgo nrepl`,
  connect from Calva/CIDER). Gates green.
