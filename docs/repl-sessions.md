# REPL sessions — `:resume` and `:sessions`

cljgo's REPL never loses your work. Every `cljgo repl` session is **journaled**
— each successful top-level form is appended, with its namespace and a
timestamp, to a plain, readable Clojure file — and you can bring a whole
session back later, vars and all, exactly where you left off (ADR 0016, ADR
0070).

These are cljgo's own REPL commands (a "special form" of the prompt, alongside
`exit`/`help`); they are not part of Clojure.

## Resuming

When you leave a journaled session, the farewell prints the command to bring
it back:

```
user=> (def a 23)
#'user/a
user=> exit
Goodbye! Resume this session with:  cljgo repl :resume 20260724-000437-5497
  (or type  :resume 20260724-000437-5497  at the prompt)
```

Resume it **from the shell**:

```bash
cljgo repl :resume 20260724-000437-5497   # by id
cljgo repl :resume 1                       # by list index (see below)
cljgo repl 20260724-000437-5497            # bare id, same thing
```

or **from inside any REPL**:

```
user=> :resume 20260724-000437-5497
resumed session 20260724-000437-5497: 1 forms replayed — in ~/work/myproj
user=> a
23
```

Resume replays the journal through the normal read → analyze → eval path, so
vars, namespaces, and macros come back. It also **cds back into the folder the
session was started in**, so `require`, `load`, and relative paths resolve just
as they did.

## Listing — `:sessions` (or `:resume` with no id)

Forgot the id? Ask:

```bash
cljgo repl :sessions      # or:  cljgo repl :resume
```

```
sessions (newest first):
  #   id                     folder                         last active       forms
  1   20260724-000437-5497   ~/work/myproj                  2026-07-24 00:07  4
  2   20260723-104309-7435   ~/work/reqsume                 2026-07-23 10:44  4
resume with:  cljgo repl :resume <#>   (or the id)
```

The list is newest-first (by last-active), and the `#` is a shortcut so you
resume with `:resume 1` instead of copying a long id. The same command works at
the prompt (`:sessions`).

## What survives, and what doesn't

Resume restores everything that is a re-evaluated **value**: vars, function and
macro definitions, namespaces, dynamic bindings. It cannot restore live host
state — **running goroutines, open channels, and native handles do not survive
a resume**; re-run the forms that created them (resume prints this reminder).

Failed forms are journaled as comments (visible history, never replayed), so a
poisoned form won't re-break a resume.

## Where journals live

`~/.config/cljgo/sessions/<id>.journal` — plain Clojure, greppable and
editable. Journaling is on for an interactive terminal; set `CLJGO_SESSION=1`
to force it on (e.g. under a pipe) or `CLJGO_SESSION=0` to force it off.
