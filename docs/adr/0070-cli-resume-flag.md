# ADR 0070 — `cljgo repl :resume <id>` on the command line (completing ADR 0016 §2)
Date: 2026-07-24 · Status: accepted (owner-directed; extends ADR 0016)

## Context
ADR 0016 §2 specified two ways to resume a saved session: **`cljgo repl
--resume <id>` on the command line** *and* `:resume <id>` inside a running
session. Only the in-REPL command shipped (`pkg/repl/session.go`
`sessionCommand`); the command-line leg was never wired — `cmd/cljgo`
`case "repl"` called `runREPL()` with no arguments, so any args after `repl`
were silently discarded.

Two things then conspired into a real papercut (hit by the owner, 2026-07-24):

1. The farewell line prints `Goodbye! Resume this session with :resume <id>`
   — which reads like a **shell command**.
2. Running `cljgo repl :resume <id>` therefore looked correct but started a
   **fresh** session and dropped the `:resume <id>` on the floor. The vars
   from the journal (`(def a 23)`) were absent, with a confusing
   `unable to resolve symbol: a`.

The journal itself was fine and the replay mechanism works — the only gap was
the missing CLI entry point promised by ADR 0016 §2, plus a farewell message
that pointed at it in a way the CLI did not honor.

## Decision
1. **Deliver the command-line resume leg of ADR 0016 §2**, using the SAME
   `:resume <id>` token as the farewell line and the in-REPL command — one
   spelling everywhere — rather than ADR 0016's tentative `--resume <id>`
   flag. So `cljgo repl :resume <id>` resumes on boot. A bare
   `cljgo repl <id>` (a lone session-id argument) is accepted as sugar for
   the same. Any other argument shape prints
   `usage: cljgo repl [:resume <session-id>]` and exits non-zero.
2. **Mechanism:** a new `Driver.ResumeID` field. When set, `Driver.Run`
   replays that journal once at start — after the session frame is pushed and
   journaling is decided, before the first prompt — by calling the existing
   `resumeSession`, which restores the world, prints the ADR 0016 §4 honesty
   notice, and continues journaling to that id. Boot-time resume and
   typed-`:resume` share one code path.
3. **Sharpen the farewell** so it is unambiguous now that both forms work:
   it prints the full shell command
   (`cljgo repl :resume <id>`) and, on a second line, the in-REPL form.
4. **List on no id.** `cljgo repl :resume` / `:sessions` (and the in-REPL
   `:resume` / `:sessions` with no argument) print a numbered, newest-first
   table — `#`, id, **folder**, last-active, form count — and a resume hint,
   instead of erroring. You resume with the short `#` (`:resume 1`) or the id;
   the index maps to exactly what the table shows.
5. **Record the folder; come back as it is.** Each journal records the
   working directory as a header comment (`;; cljgo session <id> dir=<cwd>`,
   skipped on replay). `:sessions` shows it, and **resume cds back into that
   folder** before replaying, so `require`/load and relative paths resolve as
   they did. A folder that has since been removed is a printed note, not a
   failure (the vars still replay in place).
6. **Order by recency, not id.** The listing and the `#` index order by
   last-active (file mtime), because two sessions started in the same second
   get random id suffixes — recency is the honest "newest".

## Consequences
The exact string the REPL tells you to run now works when pasted at the
shell, closing the trap. Resume has one spelling (`:resume <id>`) in all
three places it appears (farewell, prompt, CLI), so there is nothing new to
learn. No change to the journal format, `sessionEnabled`, `:sessions`, or the
in-REPL command — this is purely the missing entry point plus clearer copy.
`--resume` is deliberately NOT added: ADR 0016 floated it, but the `:resume`
token won by matching the two surfaces that already shipped. Supersedes ADR
0016 only on the surface syntax of the CLI resume (`:resume`, not `--resume`);
ADR 0016 otherwise stands.
