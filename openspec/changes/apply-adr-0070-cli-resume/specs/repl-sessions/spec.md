## ADDED Requirements

### Requirement: `cljgo repl :resume <id>` resumes a session from the command line
`cljgo repl` SHALL accept `:resume <id>` (and a lone `<id>` as sugar) as
command-line arguments and resume that saved session on boot — the
command-line leg ADR 0016 §2 specified — using the SAME `:resume <id>` token
as the farewell line and the in-REPL command. The journal SHALL be replayed
once before the first prompt, through the normal read→analyze→eval path
(restoring vars, namespaces, and macros), the ADR 0016 §4 honesty notice
printed, and journaling continued to that id. Any other argument shape SHALL
print `usage: cljgo repl [:resume <session-id>]` and exit non-zero.

#### Scenario: pasting the farewell command restores the world
- **WHEN** a session defines `(def a 23)` and ends, printing its id
- **AND** a new process runs `cljgo repl :resume <id>` with that id
- **THEN** the journal replays before the first prompt, a `resumed session
  <id>` notice is printed, and `a` evaluates to `23`

#### Scenario: bare id is accepted
- **WHEN** `cljgo repl <id>` is run with a saved session id and no `:resume`
- **THEN** the session resumes exactly as `:resume <id>` would

#### Scenario: a malformed invocation is rejected, not silently ignored
- **WHEN** `cljgo repl` is given arguments that are neither `:resume <id>`
  nor a lone id (e.g. `:resume` with no id, or two positional args)
- **THEN** it prints `usage: cljgo repl [:resume <session-id>]` and exits
  with a non-zero status, rather than starting a fresh session

### Requirement: `:resume`/`:sessions` with no id list the saved sessions
`cljgo repl :resume`, `cljgo repl :sessions`, and the in-REPL `:resume` /
`:sessions` with no argument SHALL print a numbered, newest-first table — a
1-based index, the session id, the session's folder, last-active, and form
count — followed by a resume hint, rather than erroring. Ordering SHALL be by
last-active (file mtime), and the index SHALL map to the row shown so
`:resume <#>` resumes exactly that row.

#### Scenario: no id lists a numbered table
- **WHEN** `cljgo repl :resume` is run with saved sessions present
- **THEN** it prints `sessions (newest first)`, a numbered row per session
  with its folder, and a `resume with:` hint, newest first

#### Scenario: resume by index
- **WHEN** a user runs `cljgo repl :resume 1`
- **THEN** the most-recently-active session is resumed, identically to naming
  its id

#### Scenario: no sessions yet
- **WHEN** `:sessions` is run and no journals exist
- **THEN** it prints a `no saved sessions` message, not an error

### Requirement: sessions record their folder and resume returns to it
Each session journal SHALL record the working directory it was started in (a
header comment, skipped on replay). `:sessions` SHALL display that folder, and
resuming SHALL change the process working directory back to it before replay,
so requires, loads, and relative paths resolve as they did. A recorded folder
that no longer exists SHALL be reported as a note and resume SHALL continue in
the current directory.

#### Scenario: resume cds back into the session's folder
- **WHEN** a session started in folder `A` (defining a var) is resumed from a
  different folder `B`
- **THEN** the var is restored AND the working directory is `A` again

#### Scenario: a removed folder does not break resume
- **WHEN** a session whose folder was deleted is resumed
- **THEN** a note says the folder is gone and the journal's vars still replay

### Requirement: the farewell shows the exact resume command
When a session journaled at least one form, the farewell SHALL print the full
shell command `cljgo repl :resume <id>` and, on a second line, the in-REPL
`:resume <id>` form, so the printed instruction works verbatim at the shell
(closing the trap where the old wording read as a shell command the CLI
ignored).

#### Scenario: farewell copy is runnable
- **WHEN** a journaling session ends via `exit`
- **THEN** the farewell contains the literal string `cljgo repl :resume `
  followed by the session id
