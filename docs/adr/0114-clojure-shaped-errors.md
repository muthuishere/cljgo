# ADR 0114 — Clojure-shaped errors, Go detail in ex-data

Date: 2026-07-31 · Status: **accepted** · Closes issue #174. Refines ADR 0015
(structured diagnostics) and ADR 0048 (richer error rendering); reconciled
with ADR 0111 (composed diagnostics). ADR 0048 remains the reserved home for
the general compile-time/runtime *diagnostic rendering* overhaul; this ADR is
narrower and does not reopen it.

## Context

Issue #174: cljg.io/cljg.process errors panic with `fmt.Errorf(...)`, which
means the string a caller sees through `ex-message` is Go's own error
grammar — lowercase, the doubled `op "path": syscall path: reason` shape,
syscall names (`open`, `remove`, `mkdir`) instead of the cljgo operation the
user actually called. Measured (issue table, via koine):

| operation | cljgo today |
|---|---|
| `slurp` missing file | `slurp: open .../nothing-here.txt: no such file or directory` |
| `cljg.io/read-bytes` missing | `cljg.io/read-bytes: cannot read ...: open ...: no such file or directory` |
| `cljg.io/delete!` non-empty dir | `cljg.io: delete ".../realdir": remove .../realdir: directory not empty` |
| `cljg.io/mkdirs` over a file | `cljg.io: mkdirs ".../target.txt": mkdir .../target.txt: not a directory` |

None of these are `ex-info` — they are plain wrapped Go errors, so
`(ex-data e)` is always `nil` today (`lang.GetExData` only unwraps
`IExceptionInfo`, and `*fmt.wrapError` never implements it). The failure
*kind* (missing vs. permission vs. not-a-directory vs. non-empty) is
reachable only by substring-matching an English sentence that differs by
host and by Go version.

**Precedent already in the codebase**, cited to show this is not a new
mechanism: `core/cljg/io.cljg`'s `sh!` already throws
`(ex-info "cljg.io: command failed ..." (assoc r :command ...))` — an
ex-info with structured data alongside a human message. This ADR extends
that same shape to the Go-raised (not Clojure-raised) sites.

### JVM oracle, verified against the real `clojure` CLI (1.12.5.1645, 2026-07-31)

```clojure
(try (slurp "missing.txt") (catch Exception e (println (.getMessage e))
                                               (println (class e))
                                               (println (ex-data e))))
;; missing.txt (No such file or directory)
;; java.io.FileNotFoundException
;; nil

(try (java.nio.file.Files/delete (java.nio.file.Paths/get "realdir" (make-array String 0)))
     (catch Exception e (println (.getMessage e)) (println (class e))))
;; realdir
;; java.nio.file.DirectoryNotEmptyException
```

Two things this proves: (1) the JVM's message is genuinely simpler than
cljgo's — it names the path once and stops, no operation name, no syscall
noise; (2) **the JVM never populates `ex-data`** for these — it distinguishes
failure kind by **exception class** (`FileNotFoundException`,
`DirectoryNotEmptyException`, `NoSuchFileException`, …), a mechanism cljgo
does not have and the issue explicitly declines to add ("Not asking for:
Exception classes" — `ex-info` + `Throwable` is the portable shape koine
already relies on). So cljgo cannot literally match the JVM's message *and*
keep its own reason-branching story; the two hosts solve "what kind of
failure was this" with different mechanisms, and cljgo's mechanism is
`ex-data`, not the class hierarchy. Verdict: **approximate the JVM's
economy of message** (name it once, state the failure, stop — no doubled
path/op), **deliberately diverge on where the "kind" lives** (a keyword in
`ex-data`, not a subclass), and **do not attempt to match Java exception
class names or hierarchies**.

## Decision

1. **One shared classifier, `pkg/lang.NewIOError`.** A single function,
   `NewIOError(op string, opKind Keyword, path string, err error) *ExceptionInfo`,
   builds the ex-info every raise site in scope throws. It is not a
   translation-table abstraction — it is one pure function each call site
   calls with its own op name and kind, matching the "simplicity first"
   test: the same simple thing (an `ex-info` call), reused, not a pluggable
   strategy layer.

2. **The message is `"<op>: <phrase>"` — named once, stated once.** No
   doubled path, no syscall name, no repeated operation. E.g.
   `"cljg.io/delete!: directory is not empty"`. This is the exact shape the
   issue proposed and the JVM's own economy of message (path/description
   once, not the Go doubling).

3. **`:reason` is a small, closed, append-only keyword set**, classified by
   `errors.Is` against Go's own sentinel/errno values — never string
   matching:
   - `:not-found` ← `io/fs.ErrNotExist`
   - `:permission-denied` ← `io/fs.ErrPermission`
   - `:already-exists` ← `io/fs.ErrExist`
   - `:not-a-directory` ← `syscall.ENOTDIR`
   - `:directory-not-empty` ← `syscall.ENOTEMPTY`
   - `:loop` ← `syscall.ELOOP`
   - `:unknown` ← anything else (message falls back to the raw Go text for
     this case only — graceful degradation, ADR 0048's rollout precedent)

   **Ordering matters and was verified, not assumed**: Go's own
   `syscall.Errno.Is` maps `ENOTEMPTY` to `fs.ErrExist` as well as to itself
   (removing requires both "doesn't already not-exist" and "is empty"), so
   checking `fs.ErrExist` before the errno-specific cases would misclassify
   a non-empty-directory delete as `:already-exists`. Verified directly:
   `os.Remove` on a populated dir on macOS/arm64 returns an error for which
   both `errors.Is(err, fs.ErrExist)` and `errors.Is(err, syscall.ENOTEMPTY)`
   are `true`. The classifier checks the errno-specific reasons first.

   **Not cross-platform-verified**: the errno checks (`ENOTDIR`/`ENOTEMPTY`/
   `ELOOP`) build on Windows (confirmed by cross-compile) but Windows' own
   `os` errors do not necessarily carry a matching POSIX errno, so those
   three reasons may fall through to `:unknown` there — a documented gap,
   not a fabricated match. `:not-found`/`:permission-denied`/`:already-exists`
   ride Go's portable `io/fs` sentinels and hold on every host.

4. **`:go/error` carries the original Go error text verbatim, always.**
   Nothing is lost — it is the metadata half of the ask, and it is what
   makes reshaping the message safe.

5. **`:op` is a namespaced keyword naming the operation kind** (`:fs/read`,
   `:fs/delete`, `:fs/mkdir`), not the public symbol — the public symbol is
   already in the message text. `:path` is the path the operation acted on.

## Scope (the smallest coherent slice)

Applied to exactly the four call sites the issue measured and demonstrated:

- `slurp` (`pkg/corelib/io_builtins.go`) — `:op :fs/read`
- `cljg.io/read-bytes` (`pkg/bri/io_fs.go` `-fs-read-bytes`) — `:op :fs/read`
- `cljg.io/delete!` / `cljg.io/delete-tree!` (`-fs-delete`) — `:op :fs/delete`
  (the op name in the message is picked from the `recursive?` arg, so a
  `delete-tree!` failure says `delete-tree!`, not `delete!`)
- `cljg.io/mkdirs` (`-fs-mkdir`) — `:op :fs/mkdir`

This settles the principle (Clojure-shaped message + `ex-data` reason +
preserved Go detail, one shared classifier) on the exact examples the issue
raised, with a coherent test.

### Not in scope (deferred, tracked here so it is not silently forgotten)

- The rest of `cljg.io`'s filesystem shims in the same file — `copy!`,
  `move!`, `glob`, `walk`, `temp-file`, `temp-dir`, `home`, `cwd`,
  `write-bytes`, `list-files` — still `panic(fmt.Errorf(...))`. Converting
  them is mechanical (call `NewIOError` instead) and is a good first PR for
  whoever picks this up next, but is not needed to settle the decision.
- `cljg.io`'s process exec (`io_proc.go`, `-proc-exec`: a missing/unrunnable
  binary) and `spit` — same story, same mechanical conversion, deferred.
- Any general "error translation layer" with a mapping table across the
  whole codebase — explicitly refused, per CLAUDE.md's *Simplicity first*
  doctrine; this ADR is one function, called at raise sites, not a layer.
- Exception classes / a class hierarchy — explicitly out per the issue.
- Wiring these into `pkg/diag`'s `Carrier`/code-registry machinery (ADR 0015
  /0048). Not needed here: the new `.Error()` text (via `ExceptionInfo`,
  which has no `cause` set so `Error()` is exactly the new message, no Go
  suffix) is already the right rendered line with no enrichment. A future
  ADR can register `G5023`+ codes for these if `cljgo explain` value is
  wanted; this ADR does not gate on it.

## Frozen strings this changes

**No previously-frozen `;; expect:` line changes.** Audited every
conformance test and Go `_test.go` touching `slurp`/`spit`/`cljg.io`:

- `conformance/tests/slurp-spit-roundtrip.clj` only asserts
  `(some? (ex-message e))` on the missing-file throw — never the text.
- `pkg/bri/io_test.go`/`io_compiled_test.go` never assert the delete/mkdirs/
  read-bytes error text; the only message substring checks
  (`strings.Contains(msg, "exit 3")`, `strings.Contains(msg, "exec")`) are
  on `sh!`/`exec`, which are **out of scope** here and untouched.

So this ADR adds a **new** conformance file
(`conformance/tests/cljg-io-clojure-shaped-errors.clj`) rather than changing
an existing frozen expectation. A caller who was already substring-matching
the OLD Go-shaped text (e.g. `"no such file or directory"` inside the raw
message) will still find that substring — it now lives in the `:reason`
phrase — but callers matching the old doubled shape (`"remove .../: directory
not empty"`, the syscall name, the quoted path-in-message) will not, which is
the whole point: those are the strings issue #174 asked to stop leaking.

## Consequences

- A cljgo user catching one of the four in-scope operations gets a message
  that names the operation once and states the failure once, plus a stable
  `:reason` keyword they can `case`/`cond` on portably across hosts, plus
  the original Go text under `:go/error` for whoever needs it (unchanged
  from what was in the message before — just relocated).
- The four sites share one classifier function; adding the remaining
  deferred sites is copy-and-call, not new design.
- `ExceptionInfo.Error()` for these throws no longer contains `%w`-wrapped
  Go cause text (no `cause` is set), so nothing downstream that formats via
  `.Error()` regresses into printing Go detail twice.
