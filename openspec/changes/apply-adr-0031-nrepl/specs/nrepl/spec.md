## ADDED Requirements

### Requirement: nREPL server with babashka's 13-op surface
The system SHALL provide a TCP nREPL server (`pkg/nrepl`, fronted by
`cljgo nrepl [--port N]`) speaking bencode (own codec, zero external deps,
ADR 0031) and implementing exactly the ops `describe` advertises: clone,
close, describe, eval, load-file, complete, completions, lookup, info,
eldoc, ls-sessions, interrupt, ns-list. Responses SHALL echo the request
`id` and `session` and terminate each request with a `done` status.

#### Scenario: clone, eval, result
- **WHEN** a client sends `clone`, then `eval` with code `(+ 1 2)` in the
  new session
- **THEN** it receives a `new-session` id, a `value` message `"3"` with
  the session's `ns`, and a `done` status

#### Scenario: eval error shape
- **WHEN** a client evals `(unresolvable-xyz)`
- **THEN** it receives an `err` message, an `eval-error` status with `ex`,
  a final `done` status, and `*e` in that session holds the error

### Requirement: sessions are isolated binding frames
Each nREPL session SHALL run on its own goroutine holding a thread-binding
frame of `*ns*`, `*1`, `*2`, `*3`, `*e` (via the shared `repl.Session`
helper) and `*out*`, so namespaces, result history, and printed output are
per-session with one shared evaluator (the var/namespace world stays
process-global, as on a JVM nREPL server).

#### Scenario: result history is per session
- **WHEN** session A evals `(+ 40 2)` and a fresh session B evals `*1`
- **THEN** A's later `*1` is `42` and B's `*1` is `nil`

#### Scenario: printed output streams to the evaluating session
- **WHEN** a session evals `(println "hi") :ok`
- **THEN** that session's client receives an `out` message containing
  `"hi"` before the final `value` `":ok"`, with no server-wide locking

### Requirement: interrupt is an honest per-spec stub
`interrupt` SHALL answer `done` + `session-idle` for an idle session,
`done` + `interrupt-id-mismatch` for an unknown session, and for a busy
session answer `done` with an `err` note that the eval continues (the
tree-walk evaluator has no cancellation checkpoint; ADR 0031 defers real
interrupt).

#### Scenario: idle session
- **WHEN** `interrupt` is sent to an idle session
- **THEN** the reply statuses are `done` and `session-idle`

### Requirement: cljgo nrepl writes the editor-discovery port file
`cljgo nrepl` SHALL default to an ephemeral port (`--port 0`), print
`nREPL server started on port N on host 127.0.0.1 - nrepl://127.0.0.1:N`,
and write the port digits to `.nrepl-port` in the cwd (removed on
shutdown) — the nrepl.cmdline convention editors auto-discover.

#### Scenario: default startup
- **WHEN** `cljgo nrepl` starts with no flags
- **THEN** it listens on an ephemeral 127.0.0.1 port, prints the banner
  with the actual port, and `.nrepl-port` contains that port

### Requirement: doc macro in clojure.repl, referred into user
The system SHALL provide `clojure.repl/doc` (a macro printing a var's
documentation to `*out*`: separator line, qualified name, arglists when
present, `Macro` when applicable, docstring indented two spaces — the JVM
`clojure.repl/doc` shape, oracle-verified) and refer `doc` into `user` at
boot, as JVM clojure.main's repl-requires does. An unresolvable symbol
prints nothing and yields nil.

#### Scenario: documented var
- **WHEN** `(def answer "The answer to everything." 42)` then
  `(with-out-str (clojure.repl/doc answer))` is evaluated
- **THEN** the result is
  `"-------------------------\nuser/answer\n  The answer to everything.\n"`
  (oracle: JVM Clojure 1.12.5)

#### Scenario: unresolvable symbol
- **WHEN** `(with-out-str (clojure.repl/doc nosuchsym))` is evaluated
- **THEN** the result is `""`
