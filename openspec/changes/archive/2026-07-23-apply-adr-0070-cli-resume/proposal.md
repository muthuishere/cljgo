# apply-adr-0070-cli-resume

## Why

ADR 0070 (docs/adr/0070-cli-resume-flag.md, accepted) completes the
command-line resume leg that ADR 0016 §2 specified but never shipped. Today
`cmd/cljgo` runs `case "repl": return runREPL()` with no arguments, so
`cljgo repl :resume <id>` — the exact string the farewell line prints —
silently starts a FRESH session and drops the `:resume <id>`. The owner hit
this (2026-07-24): after `(def a 23)` in one session and
`cljgo repl :resume <id>` in the next, `a` was `unable to resolve symbol`.
The journal and the in-REPL `:resume` were fine; only the CLI entry point and
the ambiguous farewell copy were wrong.

## What Changes

- `pkg/repl`: new `Driver.ResumeID` field. When non-empty, `Driver.Run`
  replays that journal once at start — after the session frame is pushed and
  `journalOn`/`sessionID` are decided, before the first prompt — via the
  existing `resumeSession` (restores vars/namespaces/macros, prints the ADR
  0016 §4 honesty notice, continues journaling to that id). Boot resume and
  typed `:resume` share one path.
- `cmd/cljgo`: `case "repl"` now passes `args[1:]` to `runREPL`, which reads
  `:resume <id>` (or a bare `<id>`) into `Driver.ResumeID`; any other arg
  shape prints `usage: cljgo repl [:resume <session-id>]` and returns 2.
- `pkg/repl` farewell: print the full shell command
  `cljgo repl :resume <id>` plus, on a second line, the in-REPL `:resume <id>`
  form — unambiguous now that both work.
- Tests: `TestResumeIDReplaysOnBoot` (a driver with `ResumeID` set replays on
  boot and restores the var, no `:resume` typed).

## Non-goals

- A `--resume` flag — ADR 0070 chose the `:resume` token to match the two
  surfaces that already shipped (farewell + in-REPL command); `--resume` is
  deliberately not added.
- Any change to the journal format, `sessionEnabled`, `:sessions`, or the
  in-REPL `:resume`/`:forget` commands — this is only the missing CLI entry
  point plus clearer farewell copy.
- Project-local `.cljgo/` session override (ADR 0016 §2) — still deferred,
  untouched here.
