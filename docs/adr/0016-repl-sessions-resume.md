# ADR 0016 — Persistent REPL sessions: every session has an ID, resume restores everything
Date: 2026-07-12 · Status: proposed (owner-directed; design via OpenSpec; journal lands with the S3 self-rebuild flow, M3)

## Context
REPL state is precious and currently dies with the process. Spike S3 already
designed the replay-journal for surviving the deps self-rebuild (re-eval
logged top-level forms through the normal eval path). Owner mandate: sessions
are saved and resumable by id — nothing is lost.

## Decision
1. Every `cljgo repl` session gets an ID and journals every SUCCESSFUL
   top-level form (with its namespace context, timestamped) to
   ~/.config/cljgo/sessions/<id>.journal — append-only, plain readable
   Clojure forms (greppable, editable, diffable).
2. `cljgo repl --resume <id>` (and `:resume <id>` inside a session) replays
   the journal through the normal read→analyze→eval path, restoring vars,
   namespaces, macros, dynamic bindings — then continues journaling to the
   same id. `cljgo repl --sessions` lists (id, started, last-active, form
   count, project dir). Project-local override via .cljgo/ dir.
3. One journal, three consumers: resume-by-id (this ADR), the deps
   self-rebuild state carry (S3/ADR 0010), and crash recovery (journal is
   written before result print — a crash loses at most the in-flight form).
4. Honestly unsurvivable (printed as a notice on resume, per S3): running
   goroutines, open channels, Go object handles, purego dlopen handles —
   re-established by re-running the forms that made them if re-runnable.
5. Failed forms are journaled as comments (visible history, not replayed).
   `:forget` / journal editing is the escape hatch for poisoned state.

## Consequences
Replay = re-evaluation, so resume fidelity inherits evaluator fidelity (no
serialization format for the world — the JOURNAL is the state, same
philosophy as event sourcing). Long sessions replay in ms (M1 boot is ~5ms;
journals are small). Side-effecting forms re-run on resume — documented
loudly; design round adds `:no-replay` marking. rlwrap line-history and the
journal are complementary (keystrokes vs state).
