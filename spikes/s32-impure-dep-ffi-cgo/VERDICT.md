# S32 VERDICT

**VERDICT: PASS on all four exit criteria — with three findings that force
rework if ADR 0048 decisions 1–5 are implemented as written.**

Impurity *is* detectable at resolve time, cheaply, from a small declarative
manifest — the prototype in `exp4-resolve-check/` does it and refuses a bad
graph before fetching or building anything. But getting there requires
correcting three things that are currently wrong or unstated in the ADRs:

1. **ADR 0044 decision 2's conditional-inclusion rule does not fire for a
   dependency's FFI** (measured, §1). It is written in terms of "that
   program uses `ffi/`", and nothing in the implemented chain looks at a
   dependency at all. A consumer with zero FFI in its own source, depending
   on a library with FFI, gets a **failed build** — or worse, §1.3.
2. **`cljgo run` and the built binary disagree, silently, on the impure-dep
   path** (measured, §1.3). Same source, exit 0 both times, different
   output. This is the failure mode CLAUDE.md calls unforgivable, and an
   impure dependency is the shortest route to it that exists today.
3. **cgo `c-link` against a third-party system library is not
   cross-compilable, and zig-cc does not rescue it** (measured, §2.1).
   ADR 0011 decision 3 names zig-cc as "the cross-compile escape hatch";
   that is true for libc-only cgo and false for exactly the case `c-link`
   exists to serve.

Host: darwin/arm64 (Apple), go1.26.3, purego v0.10.1, zig 0.16.0, cljgo
built from this worktree at `specs/toolkit`. Every number below is
reproducible with the command shown beside it.

**Labelling discipline.** `MEASURED` = real command output captured here.
`PROJECTED` = a consequence derived from measured host/toolchain behavior
plus the written text of an ADR. `ffi/`, `c-link` and `dep` do not exist in
the tree (README §Context), so §2 is measured on equivalent raw Go/cgo
programs standing in for what those verbs would emit.

---

## 1. The purego lane — ADR 0044 decision 2 has a hole [exit criterion 1: MET]

### 1.1 Setup

`exp1-purego-transitive/` is a real cljgo project built by the real
mechanism (`build.cljgo` → `plan.GoRequires` → `pkg/build/build.go:241` →
`emit.SynthGoMod` → `go build`). Structure:

- `src/main.cljg` — the consumer. Pure Clojure. No `require-go`, no FFI.
- `src/depffi/core.clj` — the "dependency" library namespace, which
  `(require-go '["github.com/ebitengine/purego" :as purego])`.
- `build.cljgo` — the consumer's own build description, declaring **no**
  `go-require`, because the consumer does not use FFI.

purego is the real module ADR 0044 is about, not a stand-in.

### 1.2 MEASURED — the rule does not fire

```
$ cljgo build
consumer says: dep is impure; purego RTLD_NOW=
error: go interop: package github.com/ebitengine/purego: no required module
provides package github.com/ebitengine/purego: go.mod file not found in
current directory or any parent directory; see 'go help modules'
EXIT=1
```

Generated `go.mod` (`cljgo build -gen keep`):

```
module cljgo.gen/main

go 1.26

require (
github.com/muthuishere/cljgo v0.0.0
)

replace github.com/muthuishere/cljgo => /…/wrktree0/
```

**No purego.** The identical project with one line added to the
*consumer's* `build.cljgo` —
`(go-require app "github.com/ebitengine/purego" "v0.10.1")`
(`exp1b-consumer-declares/`) — produces:

```
require (
	github.com/ebitengine/purego v0.10.1
	github.com/muthuishere/cljgo v0.0.0
)
```

and builds and runs.

**Reading it.** ADR 0044 decision 2 says a compiled program's `go.mod`
"gains `purego` only when that program actually uses `ffi/`". That sentence
is satisfiable only if something derives the requirement from the
dependency. Nothing does: `p.GoRequires` is populated **exclusively** by
`go-require` calls in the consumer's own `build.cljgo`
(`pkg/build/build.go:241-252`), and per ADR 0048 decision 5 a dependency's
build fn is never evaluated — so there is no path by which a dependency's
`go-require` reaches the consumer's `go.mod`, today or under the ADR-0048
design as drafted. The conditional-inclusion rule is not wrong so much as
**underspecified for the transitive case, which is the case that matters**:
libraries, not applications, are what carry FFI.

The error itself is also poor: it names a Go import path and talks about
"current directory", and names neither the cljgo namespace (`depffi.core`)
that asked for it nor `build.cljgo`, which is where the fix goes.

### 1.3 MEASURED — the worse outcome: silent REPL-vs-binary divergence

The failing build is the *good* case. With purego correctly declared
(`exp1b`), the two legs disagree and both exit 0:

```
$ ./consumer                    # AOT binary
consumer says: dep is impure; purego RTLD_NOW=2
EXIT=0

$ cljgo run src/main.cljg       # interpreter
consumer says: dep is impure; purego RTLD_NOW=
EXIT=0
```

`RTLD_NOW` is `2` in the binary and the empty string in the interpreter.
The interpreter does not link third-party Go modules, and instead of
erroring it resolves every member of an unlinked module to nil/"".
Confirmed general, not specific to purego, against the repo's own example:

```
$ cljgo run examples/build-websocket/src/main.cljg
gorilla/websocket close-normal code: nil
FormatCloseMessage returned a nil close frame
EXIT=0
```

This predates S32 and belongs to the `require-go` interpreter path
(design/05 §1's unimplemented self-rebuild), **but ADR 0048 makes it much
more likely to be hit**: today you only meet it by writing `require-go`
yourself, so you know what you did. With dependencies, a library you did
not write and cannot see silently returns nils under `cljgo run` and
correct values from `cljgo build`. That is the dual-mode divergence
CLAUDE.md calls a release blocker, arriving through the dependency door.

### 1.4 MEASURED — the purego dep tax is negligible

Identical cljgo programs, both via `cljgo build` (so both `-trimpath
-ldflags="-s -w"` per ADR 0023):

| binary | bytes | delta |
|---|---:|---:|
| `exp6-size/pureconsumer` (no purego) | 5,020,258 | — |
| `exp1b-consumer-declares/consumer` (purego linked) | 5,139,954 | **+119,696 (+2.4%)** |

**Conclusion for ADR 0023:** carrying purego costs ~120KB on a 5MB binary.
This is small enough that ADR 0044's careful conditional-inclusion
machinery is buying very little. The *cgo* lane (§2) is where size and
portability actually change.

---

## 2. The cgo lane — four consequences, measured [exit criterion 2: MET]

`exp2-cgo-lane/` is one Go program in four build-tag variants: `pure` (no
C), `cgolink` (`#cgo LDFLAGS: -lsqlite3` against the system SQLite —
exactly what `(c-link art {:pkg-config "sqlite3"})` would emit), `libconly`
(cgo against libc only), `missinglib` (links a library that does not exist).

### 2.1 (a) Cross-compilation — MEASURED: broken, and zig-cc does NOT fix it

| # | build | result |
|---|---|---|
| a1 | `CGO_ENABLED=0 GOOS=linux GOARCH=amd64`, pure | **ok** — `ELF 64-bit …, statically linked` |
| a2 | `CGO_ENABLED=1 GOOS=linux GOARCH=amd64`, cgo+sqlite3, host cc | **fail** |
| a3 | `CGO_ENABLED=0`, cgo variant | **fail** — `build constraints exclude all Go files` |
| a4 | a2 + `CC=zig cc -target x86_64-linux-gnu` | **fail** |
| a5 | zig-cc, cgo against **libc only** | **ok** |

a2's diagnostic is the worst thing measured in this spike — ~60 lines of
assembler errors from `runtime/cgo`, of which the first is:

```
# runtime/cgo
gcc_amd64.S:27:8: error: unknown token in expression
 pushq %rbx
       ^
```

It mentions no dependency, no C library, and no missing cross-compiler. A
user who added an impure dependency and then tried to ship a Linux build
sees this.

a4 — the escape hatch ADR 0011 decision 3 relies on:

```
$ CGO_ENABLED=1 GOOS=linux GOARCH=amd64 CC="$PWD/zcc" go build -tags cgolink …
# s27/cgolane
./cgo.go:10:10: fatal error: 'sqlite3.h' file not found
   10 | #include <sqlite3.h>
```

**This is the load-bearing finding of §2.** zig-cc supplies a cross
*toolchain* and a cross *libc*; it does not supply a target **sysroot** for
third-party libraries. a5 proves the distinction is exactly that: the same
zig-cc invocation cross-compiles a cgo program that only needs `stdlib.h`.

So: **cgo against libc → cross-compilable with zig-cc. cgo against any
third-party system library (`c-link`'s entire purpose) → not
cross-compilable without a full target sysroot, which cljgo does not have
and should not try to acquire.** ADR 0011 decision 3's "documents zig-cc as
the cross-compile escape hatch" is true only of the case `c-link` is not
for, and should be narrowed when ADR 0021 is ratified.

### 2.2 (b) A machine without the C library — MEASURED: buried diagnostic

```
$ CGO_ENABLED=1 go build -tags missinglib …
# s27/cgolane
/opt/homebrew/Cellar/go/1.26.3/libexec/pkg/tool/darwin_arm64/link: running cc failed: exit status 1
/usr/bin/cc -arch arm64 -Wl,-S -Wl,-x -o $WORK/b001/exe/a.out -Qunused-arguments /var/folders/…/go.o /var/folders/…/000000.o [×12 object files] -O2 -g -lnosuchlib_s27 -O2 -g -framework CoreFoundation -lresolv
ld: library 'nosuchlib_s27' not found
clang: error: linker command failed with exit code 1
EXIT=1
```

The one actionable line — `ld: library 'nosuchlib_s27' not found` — is
line 11 of 12, after a full `clang` command line with twelve temp-file
paths. It names the C library but not the cljgo dependency that wanted it,
not the version required, and not what to install.

Headers-missing (library present, `-dev` package absent) is better because
it comes from the compiler rather than the linker:

```
./noheader.go:6:10: fatal error: 'sqlite3_that_does_not_exist.h' file not found
```

**Also MEASURED, and relevant to ADR 0021's surface:** `pkg-config` is
**not installed** on this machine (`which pkg-config` → not found), while
`cc`, `zig` and the SQLite library all are. ADR 0021 decision 2's
`(c-link art {:pkg-config "sqlite3"})` therefore names a tool that a normal
developer Mac does not have. A `c-link` spec must either carry raw
`:libs`/`:headers`/`:cflags` as a first-class alternative to `:pkg-config`,
or cljgo must check for pkg-config and say so — the prototype in §3 does
the latter.

### 2.3 (c) Binary size — MEASURED: the flag is free, the C library is not

darwin/arm64, all `-trimpath -ldflags="-s -w"`:

| binary | bytes |
|---|---:|
| `out-pure` (`CGO_ENABLED=0`, no C) | 1,641,346 |
| `out-pure-cgoon` (`CGO_ENABLED=1`, **no C**) | 1,641,346 |
| `out-cgo` (`CGO_ENABLED=1`, links sqlite3) | 1,627,746 |

Two results worth stating precisely:

- **`CGO_ENABLED=1` on its own costs exactly zero bytes** — byte-identical
  size, and identical `otool -L` output (§2.4). The flag is not the cost.
- The sqlite3-linking binary is *13,600 bytes **smaller***, because the C
  library is dynamically linked rather than compiled in. **Binary size is
  the wrong metric for cgo impurity** — a cgo binary can be smaller while
  being strictly less portable. ADR 0023 treats size as first-class; for
  this decision, *linkage* is the property to track, not bytes.

On linux/amd64 the size delta is real and goes the other way:

| binary | bytes | linkage |
|---|---:|---|
| `out-pure-linux` (CGO_ENABLED=0) | 1,585,314 | **statically linked** |
| `out-libc-linux-zig` (cgo, libc only) | 1,758,736 | **dynamically linked** |

**+173,422 bytes (+10.9%)**, plus a runtime loader dependency.

### 2.4 (d) The static-binary property — MEASURED: cgo destroys it on Linux

```
$ file out-pure-linux
ELF 64-bit LSB executable, x86-64, …, statically linked, …, stripped

$ file out-libc-linux-zig
ELF 64-bit LSB executable, x86-64, …, dynamically linked,
interpreter /lib64/ld-linux-x86-64.so.2, for GNU/Linux 2.0.0, …
```

darwin, `otool -L`:

```
--- out-pure           /usr/lib/libSystem.B.dylib, /usr/lib/libresolv.9.dylib
--- out-pure-cgoon     /usr/lib/libSystem.B.dylib, /usr/lib/libresolv.9.dylib   (identical)
--- out-cgo            /usr/lib/libsqlite3.dylib  ← the new, breakable link
                       CoreFoundation, libresolv, libSystem
```

**Answers.** Does `CGO_ENABLED=1` change the static-binary property? **On
its own, no** — proven by `out-pure-cgoon` being identical to `out-pure`.
Does *cgo with a C dependency*? **Yes, decisively, on Linux**: static → a
`ld-linux` interpreter dependency. On darwin the "static binary" claim was
never literally true (`libSystem` is always dynamic), but cgo still adds a
**third-party** dylib dependency that can be absent on the user's machine —
which is the portability property users actually care about.

So `CGO_ENABLED=1` is not the thing to gate on. **A `c-link` entry is.**

---

## 3. Resolve-time detection [exit criterion 3: MET]

`exp4-resolve-check/` — a runnable prototype (`go build -o s27check .`)
that reads manifests and refuses a graph **before fetching or building
anything**. Properties, all deliberate:

- the manifest is parsed with **cljgo's own `pkg/reader`** — no new parser,
  and it demonstrates the shape is ordinary EDN a cljgo program can read;
- **no dependency's `build` fn is evaluated** (ADR 0048 decision 5 holds);
- the `:ffi` host probe is a **real `purego.Dlopen` + `Dlsym`** — the exact
  call the program would make at run time, executed at resolve time
  instead;
- the `:cgo` probe deliberately does *not* invoke a compiler; it reasons
  from the manifest plus the project's cross-compile requirement.

### 3.1 Proposed manifest shape

Emitted at **publish** time from a library's own `build.cljgo` (that is the
one moment its code may legitimately run — the author's own machine), then
read as data forever after:

```clojure
{:name    "sqlite-lite"
 :version "1.2.0"
 :capabilities #{:ffi}                     ; :pure is the empty set
 :cljgo-requires [{:name "edn-plus" :version "0.3.1"}]
 :go-requires    [{:path "github.com/ebitengine/purego" :version "v0.10.1"}]
 :ffi [{:lib     "sqlite3"
        :soname  {:darwin  "libsqlite3.dylib"     ; §4: names differ per OS
                  :linux   "libsqlite3.so.0"
                  :windows "sqlite3.dll"}
        :min-version "3.35.0"
        :symbols ["sqlite3_libversion" "sqlite3_open_v2" "sqlite3_close"]}]}
```

and for the cgo lane:

```clojure
 :capabilities #{:cgo}
 :c-link [{:pkg-config "sqlite3" :libs ["sqlite3"] :headers ["sqlite3.h"]
           :min-version "2.0.0"}]
```

Four fields carry their weight, each justified by a measurement above:

- **`:capabilities`** — the whole policy decision reduces to a set
  comparison, readable without understanding FFI at all.
- **`:go-requires`** — the missing link from §1.2. This is what merges into
  the consumer's `go.mod`; without it the transitive purego case cannot
  work at all.
- **`:soname` per OS** — §4 measures `libsqlite3.so` failing on darwin
  while `libsqlite3.dylib` succeeds. A single soname string is a bug.
- **`:symbols` + `:min-version`** — §4 measures `RegisterLibFunc` on a
  missing symbol **panicking**. Listing symbols converts that panic into a
  resolve-time message.

### 3.2 MEASURED — default-strict policy refuses the graph

```
$ ./s27check
cljgo resolve: project consumer-app, host darwin/arm64
               policy: allow=[] require-cross-compile=true

  fastjson-cgo 0.4.0     IMPURE: cgo
  sqlite-lite 1.2.0      IMPURE: ffi
  edn-plus 0.3.1         pure

error: cljgo resolve refused the dependency graph (4 problem(s)), before fetching or building anything.

  fastjson-cgo [:cgo]
    declares capability :cgo, which this project does not allow
    fix: add :cgo to the project's allowed capabilities, or drop the dependency

  fastjson-cgo [:cgo]
    c-link "nosuchlib_s27" forces CGO_ENABLED=1; this project requires cross-compilation,
    which cgo against a third-party system library cannot do (S32 exp2: zig-cc supplies
    libc, not the library's headers)
    fix: drop the dependency, replace it with a pure-Go or ffi equivalent, or drop the
         cross-compile requirement and build on each target

  fastjson-cgo [:cgo]
    needs pkg-config "nosuchlib_s27" (>= 2.0.0) to locate its C library; pkg-config is not
    installed on this host
    fix: install pkg-config and the nosuchlib_s27 development package

  sqlite-lite [:ffi]
    declares capability :ffi, which this project does not allow
    fix: add :ffi to the project's allowed capabilities, or drop the dependency

EXIT=1
```

### 3.3 MEASURED — opt-in passes; host problems still caught

```
$ ./s27check policy-ffi-optin.edn        # :allow [:ffi], no cross-compile
  sqlite-lite 1.2.0      IMPURE: ffi
  edn-plus 0.3.1         pure
resolve ok: dependency graph satisfies the project's purity policy.
EXIT=0

$ ./s27check policy-missing-lib.edn      # dep's system library absent
  absentlib [:ffi]
    needs the system library "libtotally_absent_s27.dylib" (>= 1.0.0) at run time;
    it is not loadable on this host
    fix: install totally-absent (libtotally_absent_s27.dylib)
EXIT=1

$ ./s27check policy-ffi-badversion.edn   # library present but too old
  sqlite-future [:ffi]
    declares symbol "sqlite3_from_the_future_s27" in libsqlite3.dylib, which the
    installed copy does not export
    fix: upgrade sqlite3 to >= 99.0.0
EXIT=1
```

### 3.4 Side-by-side — what the check replaces

| situation | today | with the check |
|---|---|---|
| dep's FFI lib absent | `panic: dlopen(…): tried: …` ×6 paths, at **`init()`**, after a successful build and install (§4.2) | `absentlib [:ffi] needs the system library "…"; it is not loadable on this host / fix: install …` |
| dep's C lib absent | 12-line clang dump, actionable line 11th (§2.2) | `needs pkg-config "…" (>= 2.0.0); pkg-config is not installed on this host` |
| dep needs cgo, project cross-compiles | ~60 lines of `runtime/cgo` assembler errors (§2.1) | `c-link "…" forces CGO_ENABLED=1; this project requires cross-compilation, which cgo against a third-party system library cannot do` |
| dep's purego not in consumer go.mod | `no required module provides package … go.mod file not found in current directory` (§1.2) | merged from `:go-requires`; never arises |

### 3.5 MEASURED — the Go-module merge question, answered

ADR 0048 decision 6 bullet 1 asks whether delegating to `go mod tidy`
inherits MVS "through the back door". It does, silently:

```
$ cat go.mod           # exp5-mvs-backdoor, both versions required
require (
	github.com/ebitengine/purego v0.9.0
	github.com/ebitengine/purego v0.10.1
)
$ go mod tidy
EXIT=0                 # no warning, no output
$ cat go.mod
require github.com/ebitengine/purego v0.10.1
```

**The Go toolchain picks the higher version and says nothing.** So ADR 0048
decision 4's "hard error on conflict, never silently pick" is **false for
the Go-module lane** unless cljgo detects the conflict itself, before
emitting `go.mod`. The prototype does:

```
$ ./s27check policy-goconflict.edn
  <graph> [:go-require]
    Go module github.com/ebitengine/purego is pinned at conflicting versions:
    v0.9.0 (oldffi), v0.10.1 (sqlite-lite)
    fix: pin one version explicitly in build.cljgo
EXIT=1
```

This is cheap — it is a group-by over the manifests already being read —
and it is the only way decisions 4 and 6 can both be true.

---

## 4. Platform matrix and dlopen diagnostics [exit criterion 4: MET]

`exp3-dlopen-diag/` calls purego for real on this host and distinguishes
"returned an error" from "panicked" via `recover`.

### 4.1 MEASURED

```
host: darwin/arm64

absent library                     error: dlopen(libtotally_absent_s27.dylib, 0x000A): tried:
                                   'libtotally_absent_s27.dylib' (no such file), '/System/Volumes/
                                   Preboot/Cryptexes/OS…' (no such file), '/usr/lib/…' (no such
                                   file, not in dyld cache), … [6 paths]
wrong-OS name (libsqlite3.so)      error: dlopen(libsqlite3.so, 0x000A): tried: … [6 paths]
correct name (libsqlite3.dylib)    ok: handle=0x3674c1b78
absent symbol in present lib       error: dlsym(0x3674c1b78, sqlite3_no_such_symbol_s27): symbol not found
RegisterLibFunc absent symbol      PANIC: dlsym(0x3674c1b78, sqlite3_no_such_symbol_s27): symbol not found
RegisterLibFunc on nil handle      PANIC: dlsym(0x0, sqlite3_libversion_number): invalid handle
working call                       ok: sqlite3_libversion_number()=3051000
```

Three things follow.

- **`Dlopen` and `Dlsym` return errors** — verbose (six candidate paths)
  but genuinely diagnostic, and wrappable into an ADR 0015 structured
  diagnostic without loss. The verbosity is worth keeping in a `:detail`
  field, not in the headline.
- **`RegisterLibFunc` PANICS.** This matters because ADR 0044 decision 1
  makes `RegisterLibFunc` the **AOT/static path's** registration primitive,
  and ADR 0044 decision 3 claims failures are "named, positioned errors …
  never a raw panic". That claim holds **only if `ffi/deflib`'s emitted
  code calls `Dlsym` itself first and never lets `RegisterLibFunc` perform
  the lookup.** As stated it is not true of the primitive. This should be
  written into ADR 0044 as an implementation constraint, not left as an
  aspiration.
- **The wrong-OS name fails cleanly but unhelpfully.** A dependency that
  hardcodes `libsqlite3.so` is simply broken on darwin, and the error says
  "no such file" six times rather than "this library declares no darwin
  soname". Per-OS `:soname` in the manifest (§3.1) turns it into a
  resolve-time message naming the OS.

### 4.2 MEASURED — the AOT shape fails at `init()`, before `main`

`exp3-dlopen-diag/initpanic/` is S21's `emit-sketch.go.txt` shape (package
`var` + `Dlopen`/`RegisterLibFunc` in `init()`) against an absent library —
i.e. a consumer running a binary on a machine lacking a *dependency's* C
library. Built with cljgo's own release flags (`-trimpath -ldflags="-s -w"`):

```
$ ./s27init
panic: dlopen(libabsent_dep_s27.dylib, 0x000A): tried: … [6 paths]

goroutine 1 [running]:
main.init.0()
	s27/initpanic/main.go:21 +0x6c
EXIT=2
```

`init()` has no error return, so **even the polite implementation can only
panic.** The build succeeded, the binary installed, and it dies before
`main` with a Go stack frame (`main.init.0`) that names generated Go, not
the `.cljg` source or the dependency. For an FFI-carrying *dependency*,
the consumer wrote none of that code and does not know the library exists.

This is the single strongest argument for resolve-time detection: it is not
merely nicer, it is the **only** layer at which a good message is possible
for the static/AOT path.

### 4.3 Platform matrix summary

| axis | finding |
|---|---|
| soname | differs per OS (`.dylib`/`.so.N`/`.dll`); MEASURED that the Linux name fails on darwin; a manifest must carry all three |
| absent library | `Dlopen` → error (good), but at `init()` in AOT → **panic** (§4.2) |
| absent symbol | `Dlsym` → error; `RegisterLibFunc` → **panic** |
| purego platform tier | unchanged from S21 §5 — darwin/linux/windows × amd64/arm64 conformance-tested, rest best-effort |
| cgo cross-compile | libc-only: works with zig-cc. Third-party system library: **does not work** (§2.1) |
| pkg-config | **not present** on this developer machine; ADR 0021's `:pkg-config` surface cannot be the only option |

---

## 5. Recommendation for ADR 0048 decision 6

**Adopt (c) — a declared capability set — with (b)'s explicit consumer
opt-in as the enforcement mechanism. Default deny.** (a) "allowed silently"
is refuted by the measurements.

Concretely, four clauses:

**6.1 Every published cljgo library carries a `:capabilities` set**, drawn
from `#{:go-module :ffi :cgo}` (empty = pure), in a declarative
`cljgo.manifest.edn` emitted at publish time and read at resolve time
without evaluating the library's build fn. The manifest carries
`:go-requires` (path+version), `:ffi` entries (per-OS `:soname`,
`:min-version`, `:symbols`) and `:c-link` entries (`:pkg-config` **and**
raw `:libs`/`:headers` fallback, `:min-version`). §3.1 is the shape; §3.2–
§3.4 show it working.

**6.2 A consumer must opt in per capability**, in `build.cljgo`
(`(allow-capabilities b [:ffi])`). Resolution fails on any capability in
the transitive closure that the consumer has not allowed, naming the
dependency, the capability, and the path by which it entered. Default is
the empty set: **pure by default.**

*Why (c)+(b) and not (a).* §1.3: an impure dep can make `cljgo run` and
`cljgo build` disagree silently. §4.2: an impure dep can panic the
consumer's binary before `main` with a message naming nothing the consumer
recognises. §2.1: an impure dep can make the project uncross-compilable
with a 60-line assembler error. Every one of those is a property of the
*consumer's* artifact acquired from *someone else's* code. A property that
changes what the consumer ships must be consented to by the consumer.

*Why (c) and not (b) alone.* Capabilities differ in what they cost, and
the difference is measured, not aesthetic: `:go-module` and `:ffi` cost
~120KB (§1.4) and stay cross-compilable and statically linked; `:cgo`
costs cross-compilation entirely (§2.1) and the static-binary property on
Linux (§2.4). A single boolean "allow impure deps" would let a `:cgo`
dependency in through a door opened for an `:ffi` one. They are not the
same risk and must not share a switch.

**6.3 `:cgo` is gated on the project's target set, not just on consent.**
If the project declares any cross-compilation target, a `:cgo` dependency
is **refused**, with §2.1's finding as the reason — zig-cc supplies libc,
not the target's third-party headers. ADR 0011 decision 3 should be
narrowed to say exactly this. This is what makes the refusal a *fact*
rather than a policy preference.

**6.4 A dependency's `:go-requires` merge into the consumer's `go.mod`,
and cljgo detects version conflicts itself before writing it.** §1.2 shows
the merge does not happen today, which makes decision 6's whole
"go-require merging" bullet moot until it does; §3.5 shows `go mod tidy`
silently applies MVS, so decision 4's no-silent-pick promise is only
keepable if the conflict is caught one layer up. The check is a group-by
over data already in hand.

### Additional required changes flagged by this spike

- **ADR 0044 decision 2** must be restated in terms of the transitive
  closure ("a program's `go.mod` gains purego when the program **or any
  dependency in its resolved closure** uses `ffi/`"), and decision 3's
  "never a raw panic" must become an explicit implementation constraint
  (`Dlsym` first, never let `RegisterLibFunc` do the lookup; §4.1) plus an
  acknowledgement that `init()`-time failures can only panic (§4.2) — which
  is precisely why resolve-time checking is mandatory rather than optional.
- **ADR 0011 decision 3**'s zig-cc claim narrows to libc-only cgo (§2.1).
- **ADR 0021 decision 2**'s `:pkg-config` needs a raw `:libs`/`:headers`
  alternative — pkg-config is absent on a normal macOS dev machine (§2.2).
- **ADR 0021 decision 5**'s `CGO_ENABLED=1` framing is the wrong gate:
  measured, the flag alone costs zero bytes and changes no linkage (§2.3,
  §2.4). Gate on the presence of a `c-link`.
- **ADR 0023**: for impure deps, track *linkage*, not bytes — a cgo binary
  measured 13,600 bytes **smaller** while being strictly less portable
  (§2.3).
- **The `require-go` interpreter divergence (§1.3) should be a release
  blocker in its own right**, tracked separately from ADR 0048. Silently
  resolving members of an unlinked Go module to nil is the unforgivable
  failure mode, and dependencies make it reachable without the user ever
  writing `require-go`.

---

## 6. Exit criterion met?

| # | criterion | met | evidence |
|---|---|---|---|
| 1 | purego conditional-inclusion decided by evidence, generated `go.mod` shown | **YES** | §1.2 — rule does **not** fire transitively; both `go.mod`s captured |
| 2 | four cgo consequences with numbers | **YES** | §2.1 (a, incl. zig-cc failure), §2.2 (b), §2.3 (c, exact bytes), §2.4 (d, `file`/`otool -L`) |
| 3 | resolve-time check exists, beats the linker, side-by-side | **YES** | §3.2–§3.4, runnable `exp4-resolve-check/s27check` |
| 4 | dlopen failure mode observed, error vs panic | **YES** | §4.1 — `Dlopen`/`Dlsym` error, `RegisterLibFunc` **panics**; §4.2 `init()` panic |

**Overall: PASS.** Decision 6 can be written, and should be written as §5.
Decisions 1–5 are **not** safe to implement unchanged: §6.4 and the ADR
0044 restatement are prerequisites, and decision 4's no-silent-pick promise
is currently contradicted by `go mod tidy` (§3.5).

---

## 7. Reproducing

```
cd spikes/s32-impure-dep-ffi-cgo

# §1 — purego transitivity (needs a cljgo binary on PATH)
(cd exp1-purego-transitive   && cljgo build; cljgo run src/main.cljg)
(cd exp1b-consumer-declares  && cljgo build && ./consumer && cljgo run src/main.cljg)
(cd exp6-size                && cljgo build)

# §2 — the cgo lane
cd exp2-cgo-lane
CGO_ENABLED=0 go build -tags pure     -trimpath -ldflags="-s -w" -o out-pure .
CGO_ENABLED=1 go build -tags cgolink  -trimpath -ldflags="-s -w" -o out-cgo .
CGO_ENABLED=1 go build -tags pure     -trimpath -ldflags="-s -w" -o out-pure-cgoon .
CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -tags pure -trimpath -ldflags="-s -w" -o out-pure-linux .
CGO_ENABLED=1 GOOS=linux GOARCH=amd64 go build -tags cgolink -o /dev/null .            # fails (a2)
CGO_ENABLED=1 GOOS=linux GOARCH=amd64 CC="$PWD/zcc" go build -tags cgolink -o /dev/null . # fails (a4)
CGO_ENABLED=1 GOOS=linux GOARCH=amd64 CC="$PWD/zcc" go build -tags libconly -trimpath -ldflags="-s -w" -o out-libc-linux-zig .
CGO_ENABLED=1 go build -tags missinglib -o /dev/null .                                  # fails (b)
cd ..

# §3 — the resolve-time check
cd exp4-resolve-check && go build -o s27check . && ./s27check; \
  ./s27check policy-ffi-optin.edn; ./s27check policy-missing-lib.edn; \
  ./s27check policy-ffi-badversion.edn; ./s27check policy-goconflict.edn; cd ..

# §3.5 — MVS back door
(cd exp5-mvs-backdoor && GOFLAGS=-mod=mod go mod tidy && cat go.mod)

# §4 — dlopen diagnostics
(cd exp3-dlopen-diag && CGO_ENABLED=0 go build -o s27dl . && ./s27dl)
(cd exp3-dlopen-diag/initpanic && CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" -o s27init . && ./s27init)
```

Built binaries are deliberately not committed (CLAUDE.md).
