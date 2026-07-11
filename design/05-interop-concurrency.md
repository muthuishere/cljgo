# 05 — Universal Interop & Concurrency

Status: draft v2. **Universal interop is the #1 design goal of the language.**
Sources studied: `refs/glojure` (pkgmap/genpkg, `internal/deps/*` gljdeps flow,
`lang/struct.go` FieldOrMethod, `runtime/evalast.go`, `runtime/codegen.go`),
`refs/let-go` (`rt/lang.go` go*/>!/<!, `rt/async.go`, `vm/boxed.go`), and
Clojure's `Reflector.java` design language.

---

## 1. The goal: adopt the entire Go + C ecosystem with zero-ceremony imports

The bar is JVM Clojure. There, `(:import (java.util Date))` + `(.getTime d)`
reaches **every jar on the classpath** with no bindings, no wrappers, no code
generation the user ever sees. That seamlessness — "the host's whole ecosystem
is my standard library" — is what made Clojure viable in 2008, and it is the
differentiator here: a Clojure whose stdlib is every Go module on
proxy.golang.org, plus the C ecosystem behind them.

The JVM gets this free because it loads classes at runtime. Go statically
links, so we must engineer the equivalent — differently per execution path:

- **AOT (the product):** trivial by construction. `(:require-go [github.com/gorilla/websocket :as ws])`
  makes the compiler emit a real `import "github.com/gorilla/websocket"` and
  direct calls into it; the generated module's `go.mod` names the dep; `go mod tidy`
  + `go build` do the rest. **Any module, zero bindings, zero registry** — the
  Go toolchain is our classpath.
- **Interpreted (REPL/scripts):** Go has no `dlopen` for Go packages, so a
  running interpreter can only see packages compiled into it. Glojure's
  solution (verified in `cmd/glj/main.go` + `internal/deps/`) is the honest
  one and we adopt it: a project `deps.edn` listing Go deps causes the CLI to
  `go get` each dep, generate a registration file (genpkg walks `go/types`,
  emits `_register("pkg.Sym", pkg.Sym)` for every export), generate a
  project-local main, and **`syscall.Exec` a `go run` of itself** — the process
  transparently replaces itself with a project-local interpreter binary that
  statically links every dep. First run pays a compile; Go's build cache makes
  reruns near-instant.

**UX contract (the whole loop):**

```clojure
;; deps.edn
{:go-deps {github.com/gorilla/websocket {:version "v1.5.3"}}}
```
```
$ clgo repl        # detects deps.edn changed → go get, regen, self-rebuild, exec
user=> (require-go '[github.com/gorilla/websocket :as ws])
user=> (ws/DefaultDialer)
```

One file edit, one command, any module. `clgo deps sync` runs the same
regen explicitly for CI. AOT builds read the same `deps.edn` to pin versions
in the emitted `go.mod`. There is **no curated package list**: the stdlib
registry is pre-generated for REPL snappiness, but it is a cache, not a wall.

### 1.1 Syntax: Go packages are namespaces

Glojure has no import form — you write munged globals like
`net:http.MethodGet` (`/`→`:`). That munge is its weakest surface. We surface
packages through `ns` aliases, the idiomatic Clojure shape:

```clojure
(ns app.core
  (:require-go [net/http :as http]
               [github.com/gorilla/websocket :as ws]
               [fmt]))

(fmt/Println "hello")            ; => fmt.Println("hello")
http/MethodGet                   ; => http.MethodGet (const/var access)
(ws/Dialer. {:HandshakeTimeout t}) ; third-party struct, same syntax
```

`go/` is a reserved pseudo-namespace for interop operators (`go/new`,
`go/slice`, `go/instantiate`, ...) — safe because `go` is a Go keyword and can
never be a package name.

**Methods, fields, structs** — Clojure's dot forms verbatim:

```clojure
(def client (http/Client. {:Timeout timeout}))  ; => &http.Client{Timeout: timeout}
(.-Timeout client)                    ; field read   => client.Timeout
(set! (.-Timeout client) 0)           ; field write  => client.Timeout = 0
(.Do client req)                      ; method call  => client.Do(req)
(go/new http/Client)                  ; => new(http.Client)
```

- `(T. {:Field v})` emits the pointer literal `&T{...}` (Go method sets on
  `*T` are a superset; `(go/val (T. ...))` for the value form).
- Unlike Glojure's auto-capitalizing `FieldOrMethod`, we do **not** rewrite
  `.do` → `.Do`. Go's export rule is part of its surface; hiding it invites
  collisions with unexported members.
- Go slices/maps/chans participate in `seq`/`get`/`count` via a runtime bridge
  (glojure's `gomapseq.go` proves this works).

### 1.2 Beyond Go: the C ecosystem

Two routes, both supported:

1. **Via Go bindings (free, today):** the huge cgo-based binding ecosystem
   (`mattn/go-sqlite3`, DuckDB, SDL, ...) is just Go modules — it flows
   through §1's zero-ceremony import with nothing extra from us. This alone
   makes most of C-land reachable on day one.
2. **Raw C libraries with no Go binding — `ffi/` on purego:**
   [`github.com/ebitengine/purego`](https://github.com/ebitengine/purego) does
   `dlopen`/`dlsym` with pure-Go trampolines. Comparison:

   | | cgo | purego |
   |---|---|---|
   | binds at | compile time | **runtime (dlopen)** |
   | C toolchain needed | yes | **no** |
   | cross-compile | painful (needs target CC) | easy (pure Go) |
   | REPL story | impossible without rebuild | **works live** |
   | struct-by-value, macros, inline fns | full (via wrappers) | limited/none |

   purego's runtime binding is exactly what a REPL needs — declare a C
   function at the prompt, call it immediately, no rebuild. So: **purego is
   the exposed FFI**; cgo is never exposed directly (when you need cgo-grade
   fidelity, wrap it in a small Go package and import that via §1 — which is
   also the Go community's own answer).

```clojure
(ffi/deflib libm "libm.so.6"           ; dlopen; "libm.dylib" resolved per-OS
  (cos  [:double] :double)
  (sqrt [:double] :double))

(libm/cos 1.0)                          ; => 0.5403...

;; one-off, REPL-style:
(def strlen (ffi/fn "libc.so.6" "strlen" [:pointer] :size-t))
```

`ffi/deflib` registers a namespace whose vars are purego-bound fns; type
keywords map to purego's marshaling (`:int :double :pointer :string
:size-t ...`), callbacks via `(ffi/callback ...)` → `purego.NewCallback`.
Identical in interpreted and AOT modes (purego is itself just a Go module we
emit calls into). Struct-by-value C APIs are documented as "wrap in Go" —
honest about purego's limits.

## 2. The hard problem: `(value, error)` and multiple returns

Survey: **Glojure** returns N results as a Clojure **vector**, no `error`
special-casing (`wrapGoFunc`). **let-go**'s native fns return `(Value, error)`
and non-nil errors propagate as VM exceptions — throws, but only for builtins.
(Gloat was not available to inspect; Joker's hand wrappers mostly throw.)

Neither pure choice is right: always-vector makes the 90% happy path noisy;
always-throw destroys Go's errors-are-values idiom (`errors.Is`, sentinel
errors, error-means-try-fallback).

**Recommendation: vector by default, `!` suffix to auto-throw.** `!` can never
appear in a Go identifier, so the suffix is unambiguous, purely compiler-level
sugar, and works on package fns and `.Method!` calls alike:

```clojure
(let [[f err] (os/Open "config.edn")]   ; explicit: error is a value
  (if err (fallback) (read-it f)))

(let [f (os/Open! "config.edn")]        ; throwing: the 90% path
  (read-it f))
```

| Go signature           | plain call returns | `!` call returns            |
|------------------------|--------------------|-----------------------------|
| `T`                    | `T`                | same (no-op)                |
| `(T, error)`           | `[v err]`          | `v`, throws if `err != nil` |
| `error` only           | `err` or nil       | `nil`, throws if non-nil    |
| `(T, bool)` (comma-ok) | `[v ok]`           | `v`, throws if `ok` false   |
| `(A, B, error)`        | `[a b err]`        | `[a b]`, throws on err      |
| `(A, B)`               | `[a b]`            | same                        |

The thrown value wraps the Go `error` in our exception type, original
retrievable via `(ex-go-error e)` so `errors.Is/As` compose inside `catch`.
Trailing-`error`/`bool` detection is by *type* — static in AOT (`go/types`),
`reflect.Type.Out(n)` in the interpreter — identical semantics on both paths.

## 3. Generics, boxing, nil

**Generics.** Glojure punts (genpkg rejects any generic symbol) because
`reflect` cannot instantiate type parameters. Two-tier stance:

- **AOT**: explicit instantiation compiles to real Go instantiation:
  `(go/instantiate maps/Keys [string int])` → `maps.Keys[string, int]`; usable
  inline. Type args are our type-designator forms (`string`, `http/Client`,
  `(go/slice-of byte)`).
- **Interpreted**: `go/instantiate` works for instantiations listed in
  `deps.edn` (`:go-instantiations`), which the §1 regen bakes into the rebuilt
  binary; otherwise a clear "requires AOT or a manifest entry" error. Honest
  about the reflect limitation instead of hiding the API.

**Boxing at the boundary.** Runtime values are Go `any`. Crossing into Go we
coerce (glojure's `coerceGoValue` table is the model): int64→`int`/`int32`…,
our fns → `reflect.MakeFunc` proxies for Go func params (let-go's
`Func.Unbox` pattern), seqs → slices when the target is a slice. In AOT, when
the argument's static type already matches, **no boxing is emitted at all**.
Type hints (`^int`, `^string`) force this in dynamic positions — JVM Clojure's
hint economy, unchanged.

**nil.** The classic trap: a typed-nil pointer inside an interface is `!= nil`
in Go. Rule: every value crossing back is **nil-normalized** — if
`reflect.ValueOf(v).IsNil()` (ptr/map/slice/chan/func/interface), Clojure sees
`nil`, so `nil?`/`if`/`when` behave like Clojure, and the `err` slot of
`[v err]` is truthy-testable. Accepted losses, documented: typed-nil identity
(escape hatch `(go/zero T)`), and Clojure `nil` passed into Go becomes the
parameter type's zero value.

## 4. Concurrency

### The structural win: no IOC transform

core.async's `go` macro is a CPS/state-machine rewrite that exists solely
because JVM threads are expensive and `<!` must *park*. On Go, goroutines
**are** the cheap thing core.async emulates. Both references confirm the
collapse: glojure's `(go ...)` special form is literally `go lang.Apply(fn,
args)`; let-go's `go*` runs the body in a real goroutine and `<!`/`>!` simply
block it. We do the same:

```clojure
(def c (chan 10))

(go (>! c (compute)))          ; real goroutine; returns a result channel
(let [v (<! c)] ...)           ; blocks this goroutine; closed chan => nil

(go
  (let [[v port] (alts! [c (timeout 500)])]
    (if (= port c) v :timed-out)))
```

- `(go body...)` → `go func(){...}()`, returning a 1-value result channel that
  receives the body's value then closes (let-go's contract — makes
  `(<! (go ...))` compose). Panics in the block route to the error hook.
- `<!!`/`>!!`/`alts!!`/`thread` are **aliases** of `<!`/`>!`/`alts!`/`go`
  (let-go does exactly this; without parking there is no distinction). All
  core.async names kept for source compatibility.
- Channels are first-class: `(chan)` → `make(chan any)`, `(chan n)` buffered.
  A `chan T` obtained from any Go API works directly with `<!`/`>!`/`alts!`
  (reflection wraps it; AOT emits typed ops) — **interop and core.async are
  the same fabric**, something JVM Clojure never had.
- nil puts rejected (core.async parity; keeps closed→nil unambiguous).

### Free vs needs-runtime

| Free from Go | Needs runtime support |
|---|---|
| goroutines; unbuffered/fixed buffers | `dropping-buffer`/`sliding-buffer`: side-table policy honored on every put (let-go's `chanPolicy`) |
| `select` — static `alt!` AOT-emits a real `select` | `alts!` with dynamic port vectors → `reflect.Select` (let-go's impl carries over nearly verbatim, incl. `:default`) |
| close, comma-ok receive | `timeout`, `promise-chan` (latch struct), `mult`/`pub`/`pipe`/`onto-chan!` (goroutine pumps) |
| memory model, GOMAXPROCS parallelism | closed-channel put/take must return `false`/`nil`, not panic (recover shim) |
| | goroutine tracking + drain for clean shutdown and REPL cancellation (let-go's scope/context threading — copy it) |

### Reference types

- **Atoms**: immutable values + `atomic.Pointer` CAS retry loop; watches and
  validators on the atom struct (glojure `lang/atom.go` already does this).
- **Agents**: one goroutine + buffered mailbox channel per agent; `send` and
  `send-off` both enqueue (no thread-pool split — the Go scheduler makes it
  moot); `await` via flush marker; errors set agent-failed state.
- **STM (`ref`/`dosync`): skipped.** (a) STM is the least-used reference type
  in real Clojure — atoms won; (b) faithful MVCC-STM is a big runtime with no
  Go primitive to lean on — the opposite of our host-does-the-work thesis;
  (c) multi-ref coordination is served by an atom over a map or an
  owner-goroutine + channel. Can ship later as a library; no syntax reserved.
  Glojure and let-go both ship without it.
- `future` = `go` behind a caching `IDeref`; `promise` = promise-chan behind
  `IDeref`; `locking` → `sync.Mutex` via interop; `pmap` = goroutine fan-out.

## 5. Two execution paths, one semantics

**Interpreted.** Glojure's architecture, adopted: a generated registry maps
`"fmt.Println"` → live values; third-party deps enter by the §1 self-rebuild.
Calls run through `reflect.Value.Call` behind typed fast-path wrappers
(glojure's `wrapGoFunc` switch kills reflection cost on hot shapes); member
access via cached `FieldOrMethod`-style lookup. §2's shaping uses
`reflect.Type` info.

**AOT.** The compiler resolves `ws/Dial` against `go/types` data
(`golang.org/x/tools/go/packages`), adds the import, emits a direct call:

```clojure
(let [f (os/Open! "x")] (.Close f))
```
```go
f, err := os.Open("x")
if err != nil { panic(rt.GoError(err)) }
_ = f.Close()
```

`.method`/`.-field` with hinted or inferred target types emit direct
selectors; unknown targets fall back to the interpreter's reflection helper
and `*warn-on-reflection*` flags it — exactly Clojure's contract. Static
`alt!` emits `select`; `(go ...)` emits a `go func(){}` literal. Both paths
share §2/§3 rules, so behavior is identical; only speed differs. (Glojure's
own codegen still routes through pkgmap+reflect — direct emission is our
improvement, and it is what "ClojureScript for Go" means.)

## 6. Milestones

1. **M1 — Registry interop, stdlib (interpreted).** genpkg-equivalent
   generator, `:require-go` aliases, fn/const/var access, §2 vector+`!`
   shaping, cached member reflection, nil normalization. Exit: `(os/Open! ...)`,
   `.Read`, `[v err]` destructuring at the REPL.
2. **M2 — Universal deps: any Go module.** `deps.edn` `:go-deps` → `go get` +
   regen + self-exec rebuild (Glojure's proven flow); `clgo deps sync`. Exit:
   `gorilla/websocket` imported and driven from the REPL with zero bindings.
3. **M3 — Channels & go.** `chan/close!/>!/<!/alts!/timeout`, buffer policies,
   `go`/`thread` result channels, goroutine tracking/drain, atoms. Exit:
   let-go's async semantics suite passes.
4. **M4 — AOT direct calls.** `go/types` resolution, direct call/selector
   emission, boxing elision, `panic(rt.GoError)` for `!`, emitted `go.mod`
   from `deps.edn`, `*warn-on-reflection*`. Exit: emitted Go for an HTTP-fetch
   program contains zero interop `reflect`.
5. **M5 — Structs, generics, select, FFI.** Struct-literal ctors, `set!`,
   `go/instantiate` (AOT full; interpreted manifest-gated), static `alt!` →
   `select`, typed `chan T` interop, `ffi/deflib` on purego. Exit: call
   `libm/cos` from the REPL with no C toolchain installed.
6. **M6 — Reference types & async library.** Agents, future/promise,
   mult/pub/pipe/promise-chan, pmap. STM explicitly deferred.
