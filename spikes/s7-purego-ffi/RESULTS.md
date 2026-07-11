VERDICT: PASS — purego binds and calls system sqlite3 on darwin/arm64 with CGO_ENABLED=0, including Go callbacks into sqlite3_exec; the only hard wall hit is variadic C functions (confirmed empirically). The cgo baseline (priority-5 mandate) also builds and runs, costing ~0.4–0.5s of clang per changed cgo package. The `ffi/deflib` surface is implementable exactly as doc 05 sketches it.

# S7 — purego FFI spike (darwin/arm64, go1.26.3, purego v0.10.1, sqlite 3.51.0)

Run it: `cd spikes/s7-purego-ffi && CGO_ENABLED=0 go build -o s7 . && ./s7`

## 1. What worked (all with CGO_ENABLED=0)

| Step | Result |
|---|---|
| `Dlopen("/usr/lib/libsqlite3.dylib")` | OK. The file does **not** exist on disk (dyld shared cache), but `dlopen` resolves cache paths transparently — no special handling needed beyond a bare-name fallback (`"libsqlite3.dylib"`). |
| `sqlite3_libversion` | `"3.51.0"` via a plain `func() string` binding. |
| `sqlite3_open(":memory:", &db)` | OK — `sqlite3**` bound as `*uintptr`. |
| `sqlite3_exec` CREATE+INSERT (NULL cb) | OK — callback slot declared `uintptr`, pass `0`. |
| `sqlite3_exec` SELECT with a **Go callback** | **OK** — `purego.NewCallback` works on darwin/arm64; callback received both rows, walked `char**` values/names. |
| `sqlite3_prepare_v2` / `step` / `column_text` / `column_int` / `finalize` | OK — read the row back (`name="cljgo" host="go"`, `count(*)=2`). |
| `sqlite3_close` | OK. |
| Cross-compile | The same spike builds `CGO_ENABLED=0` for linux/arm64 and linux/amd64 (purego has no-cgo dlopen paths for linux). The REPL FFI story survives cross-compilation. |

### String pattern (C `char*` ↔ Go `string`)
- **Return**: declare the Go signature return as `string` — purego copies the NUL-terminated C string into Go memory (GC-owned; the C side is not freed). Used for `sqlite3_libversion`, `sqlite3_column_text`, `sqlite3_errmsg`.
- **Argument**: declare the param as `string` — purego appends a NUL and copies per call (no copy if the Go string already ends in `\x00`, then you own its lifetime via `runtime.KeepAlive`). Used for paths and SQL text.
- **Where auto-conversion is unavailable** (inside callbacks, `char**` elements, or when you must free — e.g. `sqlite3_exec`'s errmsg needing `sqlite3_free`): take `uintptr`, read bytes to NUL manually (`goString` helper in main.go — purego does not export one).

### Pointer-to-pointer pattern (`sqlite3**`, `sqlite3_stmt**`, `char**` out-params)
Declare the param as `*uintptr`, pass `&handle`; C writes the new pointer through it. purego marshals `*T` as `void*` and pins the Go pointer for the call. Opaque handles then travel by value as `uintptr`. `char**` errmsg out-params work the same way, read with the manual C-string helper, freed with `sqlite3_free`.

### Return-code pattern
C `int` bound as `int32` (C int is 32-bit; states intent even where Go `int` would happen to work on 64-bit).

## 2. purego limits found (darwin/arm64)

1. **Variadic C functions: broken, silently.** `sqlite3_mprintf("%d", 42)` returned `"variadic test: 0"` — Apple's arm64 ABI passes variadic args on the *stack*, purego passes them in registers as if fixed. No error, just garbage. purego's `...any` last-arg is explicitly "not the same as C variadic" (it's argument splicing). ⇒ `ffi/deflib` must **reject variadic declarations at expansion time** with a "wrap it in Go/cgo" error, never emit a wrong-answer call.
2. **Callbacks: work, with constraints.** `NewCallback` succeeded for `sqlite3_exec`'s `int(*)(void*,int,char**,char**)`. Constraints (from purego source, v0.10.1 `syscall_sysv.go`): arg kinds must be int/uint/pointer/float-ish — **no string, struct, slice, map, func, interface args**; exactly ≤1 return, non-float kind; **max 2000 callbacks per process, never freed** ⇒ `ffi/callback` must cache by fn identity, not mint per call. Strings inside callbacks are manual `uintptr` walks.
3. **Struct-by-value args/returns**: supported per purego docs on darwin/linux amd64+arm64 for `RegisterFunc`, but purego does **not** verify layout — caller owns padding/alignment fidelity. Not exercised here (sqlite3's public API is handle-based). Treat as "advanced, manifest-gated" in deflib; the documented escape hatch stays "wrap in a Go package via cgo, import normally".
4. **Float returns** are fine on all major arches (guard only trips on exotic ones).
5. **≤1 return value** per registered fn — fine for C.
6. **Lifetime rules are the cgo rules**: C must not retain Go pointers past the call; deflib docs must say so.

## 3. `ffi/deflib` sketch (Clojure surface)

```clojure
(ffi/deflib sqlite "libsqlite3"                    ; resolved per-OS: libsqlite3.dylib / .so.0 / .dll
  (version   "sqlite3_libversion" []                                       :string)
  (open      "sqlite3_open"       [path :string, db :ptr!out]              :rc)
  (exec      "sqlite3_exec"       [db :ptr, sql :string, cb :callback,
                                   arg :ptr, err :cstr!out]                :rc)
  (prepare   "sqlite3_prepare_v2" [db :ptr, sql :string, n :int,
                                   stmt :ptr!out, tail :ptr!out]           :rc)
  (step      "sqlite3_step"       [stmt :ptr]                              :int)
  (col-text  "sqlite3_column_text"[stmt :ptr, i :int]                      :string)
  (close     "sqlite3_close"      [db :ptr]                                :rc))

(sqlite/version)                        ; => "3.51.0"
(let [[db err] (sqlite/open ":memory:")]  ; :ptr!out + :rc ⇒ [v err]
  (when err (println (:code err))))
(def db (sqlite/open! ":memory:"))      ; ! sugar: throws on rc≠0, returns out-param

;; one-off, REPL-style (doc 05):
(def strlen (ffi/fn "libsqlite3" "sqlite3_libversion" [] :string))

;; callbacks:
(def on-row (ffi/callback [:ptr :int :ptr :ptr] :int
              (fn [arg ncols vals names] 0)))     ; cached ⇒ one NewCallback slot
```

**Type keyword → purego/Go mapping** (this spike verified every row marked ✓):

| keyword | Go binding | C type | verified |
|---|---|---|---|
| `:string` | `string` | `char*` (arg: copy+NUL; ret: copy to Go) | ✓ |
| `:int` `:long` `:size-t` | `int32` / `int64` / `uintptr` | `int` / `long` / `size_t` | ✓ (`:int`) |
| `:float` `:double` | `float32` / `float64` | `float` / `double` | (S7 not needed; purego-supported) |
| `:ptr` | `uintptr` | `void*` / opaque handle | ✓ |
| `:ptr!out` | `*uintptr`, auto-allocated, returned as value | `T**` out-param | ✓ |
| `:cstr!out` | `*uintptr` + manual NUL-walk (+lib free fn) | `char**` errmsg | ✓ |
| `:callback` | `uintptr` from `ffi/callback` (NewCallback, cached) | fn ptr; `0` = NULL | ✓ |
| `:rc` | `int32` return code with error semantics | `int` status | ✓ |
| `:void` | no return | `void` | — |
| variadic | **rejected at expansion** | `...` | ✓ (proved broken) |

**How `[v err]` applies to C int return codes** — the `:rc` return type is the bridge to doc 05 §2's convention:
- Plain call: out-params become the value(s); the rc maps to the err slot — `nil` when 0, else a small error map `{:code rc :lib sqlite :fn "sqlite3_open" :msg ...}` (msg populated when deflib is told the lib's strerror fn, e.g. `{:errmsg sqlite3_errmsg}` option). One `:ptr!out` ⇒ `[v err]`; several ⇒ `[a b err]`; no out-params ⇒ plain rc as a value (errors-are-values, matches Go-interop).
- `!` suffix (same sugar as Go interop): throws our exception wrapping the code+msg when rc≠0, returns the bare out-param — `(sqlite/open! ":memory:") ⇒ db`.
- `:int` (e.g. `sqlite3_step`) stays a plain number — not every C int is an error code; the author opts in via `:rc`.
- Runtime binding = REPL-live (declare at the prompt, call immediately, no rebuild); AOT emits the identical `RegisterLibFunc` calls into the generated Go — one semantics, two paths, per the architecture's non-negotiable.

## 4. cgo baseline (priority-5 mandate sanity check)

`CGO_ENABLED=1` works on this machine (Apple clang 21.0.0). `cgobaseline/` (one inline C fn) vs `purebaseline/` (pure Go twin):

| build | time |
|---|---|
| cold, empty Go cache — pure | ~1.4 s (stdlib compile dominates) |
| cold-ish (stdlib cached) — cgo | ~0.42 s |
| incremental (real 1-line edit) — cgo | **~0.57 s** |
| incremental (real 1-line edit) — pure | **~0.13 s** |
| no-op rebuild (content unchanged) | ~0.07–0.09 s both (content-hash cache; `touch` doesn't invalidate) |

Ran correctly: `cgo add42(100) = 142`. So the cgo tax is roughly +0.4–0.5 s per changed cgo package on this machine — negligible for `cljgo build`; the mandate that emitted projects build with `CGO_ENABLED=1` is safe here. The purego spike itself also builds and runs identically under `CGO_ENABLED=1` (purego needs no cgo but tolerates it), so the two C stories coexist in one binary.

## Files

- `main.go` — the full sqlite walk-through, each marshaling pattern annotated `PATTERN n`.
- `cgobaseline/`, `purebaseline/` — the priority-5 timing pair.

Note: `go vet` flags the `uintptr → unsafe.Pointer` conversions in the manual
C-string/array helpers (`goString`, `deref`). That is inherent to dlopen-style
FFI — those uintptrs are C-owned addresses, never derived from Go pointers, so
the unsafeptr rule's GC-movement concern doesn't apply. Vet clean with
`go vet -unsafeptr=false ./...`; the generated deflib runtime will carry the
same annotation/waiver.
