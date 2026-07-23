# Tasks — apply-adr-0070-cli-resume

## 1. Driver boot-time resume

- [x] 1.1 `pkg/repl/driver.go`: add `Driver.ResumeID`; in `Run`, after the
  session frame is pushed and `journalOn`/`sessionID` are set, when
  `ResumeID != ""` call `resumeSession(ResumeID)` before the input loop.
  Reuses the existing replay path (no new replay logic). Gates green.

## 2. CLI wiring

- [x] 2.1 `cmd/cljgo/main.go`: `case "repl"` passes `args[1:]` to `runREPL`;
  `runREPL(args)` parses `:resume <id>` and a bare `<id>` into
  `d.ResumeID`, else prints `usage: cljgo repl [:resume <session-id>]` and
  returns 2. Gates green.

## 3. Farewell copy

- [x] 3.1 `pkg/repl/ergonomics.go` `farewell`: print
  `Goodbye! Resume this session with:  cljgo repl :resume <id>` plus a second
  line with the in-REPL `:resume <id>` form. Gates green.

## 4. List on no id + folder-aware resume

- [x] 4.1 `pkg/repl/session.go`: `:resume`/`:sessions` with no id list a
  numbered, newest-first (by mtime) table with the folder; `resolveSessionRef`
  maps a small index to that order; journal header records `dir=<cwd>`;
  `resumeSession` cds back into it (note on a missing folder). CLI parse in
  `cmd/cljgo` extracted to `parseReplArgs`. Gates green.

## 5. Test (behavioral 100%)

- [x] 5.1 `pkg/repl/session_test.go` + `cmd/cljgo/repl_args_test.go`: boot
  resume, resume-by-index, list-on-no-id, folder record/read, cd-back,
  removed-folder note, unknown-ref error, malformed/failed replay, journal
  disabled, sessionCommand branches, and the full `parseReplArgs` table.
  Every logic branch covered (residual % is OS-error defensiveness only).
  Gates green.
  (No conformance .clj: REPL session tooling, not language semantics with an
  emitter surface — same waiver class as ADR 0031's `doc`.)

## 6. Docs

- [x] 6.1 `docs/repl-sessions.md`: the session commands documented as cljgo's
  own REPL affordance (`:resume`, `:sessions`, `cljgo repl :resume <#|id>`),
  what survives resume, and where journals live. Gates green.
