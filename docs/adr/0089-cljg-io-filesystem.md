# ADR 0089 — `cljg.io` (filesystem first): cgo-free files, paths, dirs; process exec follows

Date: 2026-07-25 · Status: accepted (owner: *"the core must solve … files,
directories, running processes"*). Third `cljg.*` stdlib namespace (after ADR 0087's
`cljg.net.http` and ADR 0088's `cljg.os`); realizes the `cljg.io` tier of ADR 0085.

## Context

A cljgo CLI or script constantly touches the filesystem: check a path, list a
directory, make/delete trees, copy/move, glob, walk, join platform paths, find the
home dir or a temp file. `clojure.core` gives only `slurp`/`spit` (text read/write of
one file) — everything *structural* about the filesystem is missing. ADR 0085 placed
files · paths · directories · process under **`cljg.io`**; all of it is pure-Go
(`os` + `path/filepath` + `os/exec`), `CGO_ENABLED=0`, cross-compilable.

The tier splits by testability, exactly like `cljg.os`: **filesystem** (stat, list,
mkdir, delete, copy, move, glob, walk, path math, temp, home, cwd) is pure,
deterministic, fully unit-testable against a `t.TempDir()`. **Process exec** (spawn a
subprocess, capture stdout/stderr/exit, feed stdin, set env/dir, timeout) is also
testable but is a distinct concern with its own surface. So `cljg.io` lands in two
increments; this ADR ships the filesystem and reserves process exec.

## Decision

### 1. `cljg.io` — filesystem (this increment)

The structural filesystem surface `clojure.core` lacks. A thin Go shim layer over
`os` + `path/filepath` (`-fs-stat`, `-fs-list`, `-fs-mkdir`, `-fs-delete`, `-fs-copy`,
`-fs-move`, `-fs-glob`, `-fs-walk`, `-fs-temp-file`, `-fs-temp-dir`, `-fs-home`,
`-fs-cwd`, `-path-abs`, `-path-join`, `-path-base`, `-path-dir`, `-path-ext`);
the ergonomic API is portable Clojure.

Surface:
```clojure
(require '[cljg.io :as io])
(io/exists? "x")          (io/file? "x")     (io/directory? "d")
(io/list-files "d")       (io/walk "d")      (io/glob "src/*.clj")
(io/mkdirs "a/b/c")       (io/delete! "f")   (io/delete-tree! "d")
(io/copy! "a" "b")        (io/move! "a" "b")
(io/size "f")             (io/modified "f")            ; nil if absent
(io/path "a" "b" "c")     (io/parent "a/b")  (io/filename "a/b.txt")
(io/extension "a/b.txt")  (io/absolute "x")
(io/home)                 (io/cwd)
(io/temp-file)            (io/temp-dir)                ; created, path returned
```
`slurp`/`spit` stay `clojure.core` (the precedence principle — `cljg.io` never shadows
them); the read/write bang ops here are the *structural* additions, not text I/O.

### 2. Non-OptIn, pure-Go

`os` + `path/filepath` are stdlib — no dependency to isolate, so `cljg.io` is a normal
namespace, and `CGO_ENABLED=0` + `cljgo dist` hold. It rides the same name-generic
embedded registry (ADR 0087 §1). Path shims use `filepath`, so join/parent/ext follow
host separators; a Clojure-hosted build would reimplement them the same way.

### 3. Process exec (realized — increment 2)

`cljg.io` adds `exec` / `sh` / `sh!` — run a subprocess and capture
`{:out :err :exit :timed-out?}` — over pure-Go `os/exec` + `context` (one shim,
`-proc-exec`; the sugar is portable Clojure). opts: `:in` (stdin), `:env` (merged onto
the current environment), `:dir` (working directory), `:timeout-ms` (kill after n ms →
`:timed-out? true`, `:exit -1`). A **non-zero exit is a normal result** (`exec`/`sh`
never throw on it); `sh!` is the throwing variant for the script style where a failed
step aborts. A missing/unrunnable binary throws (it never ran). Fully CI-testable (run
`echo`/`cat`, assert capture, stdin, exit code, env, timeout).

## Consequences

- cljgo gains a real filesystem stdlib — the structural ops a CLI/script needs, none
  of which `clojure.core` offers — with a `t.TempDir()`-testable core.
- `cljg.io` opens as the files/paths/process tier; process exec is additive on the
  same namespace next.
- Pure-Go keeps the static binary + `cljgo dist`; path semantics are host `filepath`,
  so cross-compiled binaries behave per target OS.

## Not chosen

- **Shadowing/relocating `slurp`/`spit`** — the precedence principle forbids it;
  `cljg.io` adds structure around them, never replaces them.
- **A `java.io.File`-style object model** — cljgo is Go-hosted; plain string paths +
  functions match Clojure-on-Go and stay data-first.
- **Process exec in this increment** — its own surface (stdin/env/timeout); split out
  so the filesystem ships fully green now.
