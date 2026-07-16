# 0038 — STM-lite refs, queue-backed agents, cooperative future-cancel, and reader position-metadata scope

Status: accepted (2026-07-16)

## Context

The clojure-test-suite exercises the reference-type surface beyond atoms:
`ref`/`dosync`/`alter` (remove_watch.cljc), `agent`/`send`/`await`
(remove_watch.cljc), and `future-cancel` (realized_qmark.cljc). cljgo had
atoms, vars, delays, promises, and goroutine futures, but no refs, no
agents, and no future cancellation. A full JVM-faithful STM (MVCC snapshots,
retries, commute reordering, agent send-in-transaction holdback) is a large
subsystem that nothing in the suite — or in cljgo's Go-hosted concurrency
story (design/05 §4: channels + goroutines are the primary model) — needs
yet.

JVM oracle (Clojure 1.12.5), cited in the conformance tests:

- `(alter (ref 0) inc)` outside `dosync` throws IllegalStateException
  "No transaction running".
- `(dosync (alter (ref 1) + 5))` => 6; watches added with `add-watch` fire
  with (old, new) on commit.
- `future-cancel` on a completed future => false; on a running one => true,
  after which `realized?` and `future-cancelled?` are true and deref throws
  CancellationException.

## Decision

**STM-lite**: `ref` is a mutex-guarded cell with watches (`pkg/lang/ref.go`).
`dosync` (macro over the private `-tx-run` builtin) takes ONE global
transaction lock and marks the goroutine in-transaction through a dynamic
var (conveyed like any binding; nested `dosync` joins the outer
transaction). `alter`/`ref-set`/`commute` require the in-transaction mark —
outside it they throw "No transaction running" — and mutate in place under
the global lock.

**Agents**: an agent is a value cell plus an unbounded action queue drained
by one dedicated goroutine (`pkg/lang/agent.go`), so actions for one agent
are serialized in send order. `send` and `send-off` are the same operation
(goroutines have no thread-pool distinction — same collapse as `go`/
`thread`, design/05 §4). `await` enqueues a latch action and blocks until
the queue drains to it.

**future-cancel**: cancellation is COOPERATIVE-ONLY — it settles the future
(deref then panics with a cancellation error; `realized?`/`future-done?`/
`future-cancelled?` become true) but cannot interrupt the running
goroutine, which runs to completion and discards its result. Whichever of
body-completion and cancel settles first wins (sync.Once).

**Reader position metadata narrows to lists + symbols** (this batch also
ratifies a reader-contract change): design/00 §4.5 said the reader
annotates EVERY IObj form with :file/:line/:column/:end-line/:end-column.
JVM Clojure attaches reader positions to LISTS ONLY (oracle 1.12.5,
file-loaded: `(meta '(1 2))` => `{:line 1 :column 9}`; `(meta [1 2])`,
`(meta {:a 1})`, `(meta #{1})`, `(meta 'abc)` => nil) — and the suite
observes the difference: `^:foo [1]`'s metadata must be exactly
`{:foo true}`, not polluted with position keys (group_by.cljc,
edn read_string.cljc). cljgo now annotates lists (ISeq) and — as a
deliberate, diagnostics-serving deviation — SYMBOLS, which is what
`cljgo check` errors point at (A2001's exact column); no conformance
behavior observes symbol metadata. Vector/map/set literals read clean.

## Consequences

- The suite's single-goroutine ref/agent/future tests behave exactly like
  the JVM oracle; `remove_watch.cljc` and `realized_qmark.cljc` pass.
- Deviations (documented here, revisit if anything ever needs them):
  transactions serialize globally instead of retrying optimistically;
  ref watches fire per in-transaction mutation, not once per ref per
  commit; `commute` is `alter`; agent errors are re-raised on the draining
  goroutine rather than entering a failed-agent state; a cancelled future's
  body goroutine is not interrupted.
- `pkg/lang/agent.go` surgery is logged in `pkg/lang/PROVENANCE.md`.
