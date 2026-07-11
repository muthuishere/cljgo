# 04 — The Go Source Emitter

Component: analyzed AST → `.go` files → `go build`.
References studied: `refs/cljs2go` (Håkan Råberg, 2014–15; ClojureScript→Go overlay, passed most of the cljs test suite), `refs/glojure/pkg/runtime/codegen.go` (living Go codegen, ~2500 lines), and `clojure/lang/Compiler.java` (JVM semantic ground truth).

---

## 0. What the references teach us

| | cljs2go | Glojure | JVM Clojure |
|---|---|---|---|
| Input | cljs.analyzer AST, per top-level form | **evaluated, live namespace** — snapshots Var values after eval | analyzed form, per top-level form |
| Expr→stmt | IIFE: `func() T { ... }()` in `:expr` context (`emit-wrap`, compiler.clj:475,660) | statement flattening into temp vars (ANF-ish); every `generate*` returns an r-value string | bytecode is expression-friendly; not an issue |
| fn repr | `*AFn` struct with `Arity0..Arity21` + typed `Arity2FFF` fields, filled by reflective `Fn(...)` (rt.go:141,593) | `lang.FnFunc func(args ...any) any` + `FnFunc0..FnFunc4` fast paths (codegen.go:930) | class extends `AFunction`/`RestFn`; `invoke()` overloads 0–20 (`MAX_POSITIONAL_ARITY = 20`) |
| recur | `for` + `continue` / IIFE trouble | `goto recur_LABEL` in fn bodies, `for {}` + `continue` in loops, temp vars for simultaneous rebind (codegen.go:1590) | `GOTO loopLabel` after rebinding locals |
| Emission | string emission → `goimports` → `go build` | `bytes.Buffer` + `fmt.Fprintf` → **`go/format.Source`** (codegen.go:418) | ASM bytecode |

Two hard-won lessons:

1. **Glojure's "eval then serialize the namespace" model is a trap for us.** It must reverse-engineer live values back into Go source — it needs `liftedValues` for closed-over data, topological sorting of `valueInits` with a cycle-breaking fallback (codegen.go:344–406), and it **panics on opaque function values** (`generateFnFunc`, codegen.go:926). It works for Glojure because its compiler *is* its interpreter. Ours is a plain AOT compiler: we compile **forms**, never runtime values. That is the ClojureScript/cljs2go model and it is the right one.
2. **cljs2go's IIFE technique is elegant but poisonous in Go**: `recur` inside an `if`-expression cannot `continue`/`goto` across a `func(){}()` boundary, closures allocate, and defer/panic frames pile up. Glojure's statement flattening avoids all of it. We flatten.

---

## 1. Compilation model

### Namespace → package

**One Clojure namespace → one Go package (one directory), one `.go` file per source file**, mirroring `nsToPath` munging (`.`→`/`, `-`→`_`, same as both refs):

```
src/my_app/core.clj        →  gen/my_app/core/core.go        package core
src/my_app/util.clj        →  gen/my_app/util/util.go        package util
clojure/core (precompiled) →  runtime module, imported as clj/core
```

A Go package is the natural unit: exported names, `import` = `require`, and `go build` parallelizes per package. Deftypes and protocols (later) also want package scope.

### Top-level forms → one `Load()` function, source order

Clojure's semantic is *sequential top-level evaluation*: `(def x 1)` then `(def y (inc x))` must run in that order. Go's `init()` order within a package is file-name order and across packages is import order — close, but implicit, un-reentrant, and unhookable. Both refs outgrew raw `init()` (Glojure registers `LoadNS` with the runtime; cljs2go's `main.go` wires `*main-cli-fn*` by hand). We make loading explicit:

*[EDIT NOTE: `rt` below is not a second runtime — it is the same single runtime package docs 01–03 call `pkg/lang`, imported under an alias (see 00-architecture §4.2). `gclj` is a name placeholder.]*

```go
// gen/my_app/core/core.go
package core

import (
    "github.com/you/gclj/rt"       // the runtime: values, Var, Apply, ...
    util "github.com/you/gclj-out/my_app/util"
)

var ns = rt.FindOrCreateNamespace("my-app.core")

var loaded = false

// Load evaluates the namespace's top-level forms exactly once,
// in source order, after loading everything it requires.
func Load() {
    if loaded { return }
    loaded = true
    util.Load()                    // (:require [my-app.util])
    // --- top-level forms, in order ---
    // (def x 1)
    v_x := rt.InternVar(ns, "x")
    v_x.BindRoot(int64(1))
    // (defn greet [n] ...)
    v_greet := rt.InternVar(ns, "greet")
    var tmp1 rt.Value
    { /* fn emission, §4 */ }
    v_greet.BindRoot(tmp1)
    // (println "side effect at load")  — a bare top-level expr
    _ = rt.Apply1(rt.CoreVar("println").Get(), "side effect at load")
}
```

Guarding with a bool (not `sync.Once`) keeps it cheap and lets a future REPL re-enter deliberately. Var *interning* happens inside `Load()` right before each `def` runs — matching JVM Clojure, where `def` interns then binds, so earlier forms can't see later vars.

### Bootstrap: `main`

The compiler emits one extra `main` package when given an entry namespace:

```go
// gen/main.go
package main

import (
    "os"
    "github.com/you/gclj/rt"
    core "github.com/you/gclj-out/my_app/core"
)

func main() {
    rt.Init()                       // clojure.core vars, *out*, etc.
    core.Load()                     // transitively loads requires
    mainVar := rt.Var("my-app.core", "-main")
    args := make([]rt.Value, len(os.Args)-1)
    for i, a := range os.Args[1:] { args[i] = a }
    rt.Apply(mainVar.Get(), args)
}
```

Whole pipeline: `read → analyze → emit .go → go/format → write → go build`. `go build` is our verifier and linker; sub-second for small programs (cljs2go's README brags `go run main.go` < 1s, binary startup < 50ms — that startup number is the whole reason this project exists).

---

## 2. Universal Go interop — the #1 design goal

The emitted project is a **normal Go module**. Any third-party dependency in its `go.mod` must be importable from Clojure code with **zero hand-written bindings**, and calls must compile to **direct (non-reflective) Go calls**.

### What the references actually do

Glojure has two interop layers, and the split is instructive:

- Its *interpreter* needs a runtime name→value registry: `pkg/pkgmap` (a `map[string]any` keyed `"net/http.Get"`), populated by generated `gljimports_<GOOS>_<GOARCH>.go` files — 9,600+ lines *each*, one per platform, built by `internal/genpkg`, which loads packages through **`go/types` + `importer.ForCompiler(fset, "source", nil)`** and walks every exported object. Third-party deps come in via `glj deps get` (internal/deps/get.go): it runs `go get dep@version`, then regenerates `./glj/gljimports/gljimports.go` for those packages. Universal, no hand bindings — but a registry the binary must carry.
- Its *codegen*, by contrast, emits **direct package references**: `generateGoExportedName` (codegen.go:1844–1872) adds a real aliased Go `import` and writes `alias.ExportedName` — the comment at codegen.go:1828 even notes compiled code can reach packages *absent from the registry*, "because the import will cause the go toolchain to pull in the package". But *invocation* still goes through `lang.FieldOrMethod` + reflect-driven `Apply` (`generateHostCall`, codegen.go:1874–1913) — direct linking of the symbol, reflective calling of it.

cljs2go is all reflection (`Native_invoke_func` / `MethodByName`, rt.go:53–80) — a documented pain point.

**Our position:** an AOT compiler needs no runtime registry and no reflection — it needs *type facts at compile time*. We keep Glojure's registry-free direct-import path and extend it to direct *calls*.

### `(:require-go ...)` → Go imports

*[EDIT NOTE: was `(:import ...)`; surface form resolved to design 05 §1.1's `(:require-go ...)` — Go packages are namespaces, not classes; `:import` stays reserved. Emission below is unchanged.]*

```clojure
(ns my-app.core
  (:require-go [net/http :as http]
               [github.com/gin-gonic/gin :as gin]))
```

```go
import (
    http "net/http"
    gin "github.com/gin-gonic/gin"
)
```

The alias becomes the Clojure namespace prefix: `(http/Get url)`, `gin/Default`. Symbols may also be fully qualified without an alias; either way the emitter's import map (§6 conventions) collects the path and renders the header last, exactly like Glojure's `addImportWithAlias`.

### Signature knowledge: `go/packages` in the compiler

The compiler loads **`golang.org/x/tools/go/packages`** (mode `NeedTypes|NeedTypesInfo`) for every imported path — the supported successor of the `go/types` source importer Glojure's genpkg uses, and it works uniformly for stdlib and any module dependency. This is *not* deferred to the endgame: it runs from v0, because it is what makes calls direct instead of reflective. For each referenced export the emitter knows:

- **Functions/consts/vars** — full signature → direct call with coercions derived from parameter types:

  ```clojure
  (strings/Repeat s 3)          ; s is a boxed rt.Value
  ```
  ```go
  tmp1 := strings.Repeat(rt.AsString(s1), int(rt.AsInt64(int64(3))))
  ```

- **Multi-returns** — *[EDIT NOTE, superseded by design 05 §2 / 00-architecture §4.3]:* the original draft here mapped `(T, error)` unconditionally to panic. The resolved design is dual-mode: a **plain call returns `[v err]`** (trailing `error`/`bool` detected by type via go/types); a **`!`-suffixed call** (`http/Get!`) unwraps and panics with the wrapped Go error. Other multi-values map to a vector. The emitter implements doc 05's shaping table; only the `!` variant emits the panic pattern:

  ```clojure
  (http/Get! "https://x.dev")     ; throwing variant
  ```
  ```go
  tmp2, err3 := http.Get("https://x.dev")
  if err3 != nil { panic(rt.NewGoError(err3)) }
  ```
  ```clojure
  (http/Get "https://x.dev")      ; plain: error is a value → [v err]
  ```
  ```go
  tmp4, err5 := http.Get("https://x.dev")
  tmp6 := rt.Vector(tmp4, rt.NilNormalize(err5))
  ```

- **Types** — `(http.Client.)` / `(new http/Client)` → `&http.Client{}`; struct field read `(.-Timeout c)` and method call `(.Do c req)` compile to direct selectors **when the analyzer knows the receiver's Go type** (flowing from constructor/signature returns or `^http.Client` hints).

The reflective path (`rt.FieldOrMethod` + `Apply`, Glojure's generateHostCall pattern) remains only as the **fallback for untyped receivers** — `(.Foo x)` where `x` is a bare `rt.Value` — and shrinks as type propagation improves. v0 ships types-driven direct calls for package-level functions (the common case; go/packages does all the work) and the reflect fallback for instance members; the endgame is receiver-type inference making the fallback rare, plus warning diagnostics when it fires.

### `go.mod` generation and management

The output directory is a real module the user owns:

```
gen/
  go.mod            module my-app; require gclj-rt vX.Y; deps appended below
  main.go
  my_app/core/core.go
```

The build driver: emit sources → ensure `go.mod` exists (create with module name + runtime require on first build, never overwrite) → `go get pkg@version` for any pinned dep from the project's deps config (same mechanic as Glojure's `internal/deps`) → `go mod tidy` to resolve everything else the emitted imports pulled in → `go build ./...`. Because the module is ordinary, vendoring, `replace` directives, private module proxies, and IDE tooling on the emitted code all just work — that is the whole point of emitting source instead of driving reflection.

---

## 3. Expression → statement mismatch

Clojure: everything is an expression (`if`, `let`, `loop`, `do`, `try` all yield values). Go: those are statements. The standard technique — used by Glojure throughout — is **flattening**: every emitter function *writes statements* to the current buffer and *returns the name of an r-value* (a temp var or literal). Compound expressions declare a result temp, emit their control flow as statements assigning it, and hand back the temp's name.

### `if` as expression

```clojure
(def y (if (pos? x) (* x 2) 0))
```

```go
// (if ...) — generateIf pattern, cf. glojure codegen.go:1331
var tmp3 rt.Value
tmp4 := rt.Apply1(v_pos_QMARK_.Get(), v_x.Get())
if rt.IsTruthy(tmp4) {
    tmp3 = rt.Apply2(v_STAR_.Get(), v_x.Get(), int64(2))
} else {
    tmp3 = int64(0)
}
v_y.BindRoot(tmp3)
```

Note the truthiness call: only `nil` and `false` are falsy (Compiler.java `IfExpr` emits exactly the null-check + `Boolean.FALSE` comparison; `rt.IsTruthy` is its Go twin).

### `let` as expression

```clojure
(println (let [a 1 b (+ a 2)] (* a b)))
```

```go
var tmp1 rt.Value
{ // let — new lexical block = new scope, shadowing is free
    var a1 rt.Value = int64(1)
    _ = a1
    var b2 rt.Value = rt.Apply2(v_PLUS_.Get(), a1, int64(2))
    _ = b2
    tmp1 = rt.Apply2(v_STAR_.Get(), a1, b2)
}
_ = rt.Apply1(v_println.Get(), tmp1)
```

The emitter keeps a stack of scopes mapping Clojure names → suffixed Go names (`a` → `a1`), so re-binding `a` in a nested `let` allocates `a3` instead of colliding — exactly Glojure's `varScope` stack (codegen.go:26, 2165–2246). `_ = a1` suppresses Go's unused-variable error for bindings only used conditionally.

### Why not IIFE

cljs2go emits `func() T { if ... }()` for `:expr` context. Looks like ClojureScript's JS output, but in Go: (a) `recur` compiled as `continue`/`goto` cannot cross a function-literal boundary — an `(if c (recur ...) x)` in loop tail position breaks; (b) every IIFE is a closure allocation + call; (c) `panic` unwinding through stacks of IIFEs wrecks stack traces. Flattening has none of these; its only cost is emitter bookkeeping (a monotonic temp counter and a scope stack). **Decision: flatten, always. No IIFEs in emitted code.**

`do` falls out for free: emit all statements assigning to `_`, return the last expression's r-value (glojure codegen.go:1314).

---

## 4. fn emission

### v0 representation

```go
// in rt:
type Value = any
type Fn func(args ...Value) Value            // implements rt.IFn via Invoke
```

A Go closure captures lexical bindings by reference automatically — no environment struct, no lifting. This is the single biggest thing Go gives us for free relative to JVM bytecode (Compiler.java spends hundreds of lines hoisting closed-overs into fields).

### Single arity + closure

```clojure
(defn adder [n] (fn [x] (+ x n)))
```

```go
v_adder := rt.InternVar(ns, "adder")
var tmp1 rt.Value
tmp1 = rt.Fn(func(args ...rt.Value) rt.Value {
    rt.CheckArity(args, 1)
    n1 := args[0]; _ = n1
    var tmp2 rt.Value
    tmp2 = rt.Fn(func(args ...rt.Value) rt.Value {
        rt.CheckArity(args, 1)
        x2 := args[0]; _ = x2
        return rt.Apply2(v_PLUS_.Get(), x2, n1)   // n1 captured — plain Go closure
    })
    return tmp2
})
v_adder.BindRoot(tmp1)
```

### Multi-arity + variadic → dispatch switch

```clojure
(defn greet
  ([] (greet "world"))
  ([name] (str "hello " name))
  ([name & more] (str name " and " (count more))))
```

```go
tmp1 = rt.Fn(func(args ...rt.Value) rt.Value {
    switch len(args) {
    case 0:
        return rt.Apply1(v_greet.Get(), "world")
    case 1:
        name1 := args[0]; _ = name1
        return rt.Apply2(v_str.Get(), "hello ", name1)
    default:
        rt.CheckArityGTE(args, 1)
        name2 := args[0]; _ = name2
        var more3 rt.Value                 // nil, not empty seq, when no rest args
        if len(args) > 1 { more3 = rt.NewList(args[1:]...) }
        _ = more3
        t4 := rt.Apply1(v_count.Get(), more3)
        return rt.Apply3(v_str.Get(), name2, " and ", t4)
    }
})
```

This is Glojure's exact scheme (codegen.go:1010–1043) and semantically mirrors `RestFn`: fixed arities dispatch exactly, the variadic method is the `default` with a floor check. One `switch` beats cljs2go's 60-field `AFn` struct + reflective `Fn(...)` constructor, which was its heaviest piece of machinery (rt.go:141–660) and a documented source of pain (reflect `MakeFunc` bridges on every fn).

Named `fn`s (`(fn fact [n] ...)`) bind their own name as a local before the body so self-calls skip the Var (glojure codegen.go:968).

### `recur` → rebind + jump

Semantics from Compiler.java: `recur` is tail-only (analyzer enforces), rebinds the loop locals *simultaneously*, jumps to `loopLabel`. Two cases:

**fn-level recur** (the fn method is itself the loop target) — `goto`:

```clojure
(defn sum-to [n] (loop-free-style...))   ; (fn [n acc] ... (recur (dec n) (+ acc n)))
```

```go
tmp1 = rt.Fn(func(args ...rt.Value) rt.Value {
    rt.CheckArity(args, 2)
    n1 := args[0]; _ = n1
    acc2 := args[1]; _ = acc2
recur_1:
    tmp3 := rt.Apply2(v_LT_.Get(), n1, int64(1))
    var tmp4 rt.Value
    if rt.IsTruthy(tmp3) {
        tmp4 = acc2
    } else {
        var tmp5 rt.Value = rt.Apply1(v_dec.Get(), n1)       // temps first:
        var tmp6 rt.Value = rt.Apply2(v_PLUS_.Get(), acc2, n1) // simultaneous rebind
        n1 = tmp5
        acc2 = tmp6
        goto recur_1
    }
    return tmp4
})
```

**`loop` expression** — `for {}` + `continue` (goto can't jump over variable declarations in Go; a `for` avoids the restriction inside nested blocks):

```clojure
(loop [i 0] (if (< i 10) (recur (inc i)) i))
```

```go
var tmp1 rt.Value
{ // loop
    var i1 rt.Value = int64(0); _ = i1
    for {
        t2 := rt.Apply2(v_LT_.Get(), i1, int64(10))
        var t3 rt.Value
        if rt.IsTruthy(t2) {
            var t4 rt.Value = rt.Apply1(v_inc.Get(), i1)
            i1 = t4
            continue
        } else {
            t3 = i1
        }
        tmp1 = t3
        break
    }
}
```

The emitter keeps a `recurContext` stack `{loopID, bindingVars, useGoto}` (glojure codegen.go:33, 2343); the analyzer's loop-id on each `recur` node picks the right frame. The dead-`t3`-after-`continue` wrinkle is handled by having `generateRecur` return no r-value, so the `if` branch that recurs emits no assignment (glojure returns `""`, codegen.go:1636). Emit `goto` only when the analyzer says the body actually recurs (`nodeRecurs`, codegen.go:2432) — otherwise Go rejects the unused label.

---

## 5. Calling convention: boxed v0, hinted endgame

**v0: everything is `rt.Value = any`.** Ints are `int64`, doubles `float64`, strings `string`, everything else runtime types. Every call goes through `rt.Apply0..Apply4` (fixed-arity helpers avoid the `[]any` varargs allocation — glojure codegen.go:1292–1307) or `rt.Apply(f, []Value{...})` beyond 4. `Apply*` type-switches: `rt.Fn` → call it; `IFn` (deftype implementing Invoke) → interface call; Go `func` value → reflect bridge; keyword/map/vector/set → their IFn behavior.

Costs accepted in v0: interface boxing of every intermediate, a type switch per call, arithmetic through `clojure.core/+` var indirection. Correctness and coverage first — cljs2go proved the semantics work; its phase-3 "Performance: revisit all the basic assumptions, generate cleaner code" note is exactly where we defer this too.

**The endgame ladder** (each step independent, driven by the analyzer's tags):

1. **Direct static calls.** When the invoke target is a Var whose root is a compile-time-known fn and the Var isn't dynamic or redefed, emit a direct call to the emitted Go function instead of `Apply` — JVM Clojure's "direct linking" equivalent.
2. **Fixed-arity fn types.** Emit single-arity fns as `rt.Fn2 func(a, b Value) Value` etc. (glojure's `FnFunc0..4`); call sites that statically know the arity call directly, no slice, no switch.
3. **Primitive signatures on hints.** `^long`/`^double` hints select unboxed params/returns — the moral equivalent of JVM `IFn$LL` and cljs2go's `Arity2FFF` typed fields, but as *additional emitted Go functions* (`greet_L(int64) int64`) beside the boxed one, chosen at call sites where the analyzer proved the types. Boxed wrapper always exists for dynamic callers.
4. **Open-coded intrinsics.** `(+ x y)` with both operands hinted `long` → `x + y` with overflow check; `if` on a hinted boolean skips `IsTruthy`. Compiler.java does precisely this with `MaybePrimitiveExpr` / intrinsics.

None of this changes emitted-code *shape* — flattening and temp vars stay; only the temps' Go types and the call instructions sharpen. That's why it's safe to defer.

---

## 6. Emission mechanism: text + `go/format`, not `go/ast`

**Decision: emit text through a small writer (`fmt.Fprintf` on `bytes.Buffer`), then run the result through `go/format.Source` before writing the file. Do not build `go/ast` trees.**

Evidence: Glojure — the living, working implementation — does exactly this (`writef`, buffer assembly, `format.Source` at codegen.go:418, falling back to writing unformatted source *with* the error so you can debug). cljs2go likewise emitted strings and shelled to `goimports`. Neither ever moved to `go/ast`, and Compiler.java's ASM `GeneratorAdapter` is the same idea one level down: a linear instruction writer, not a tree builder.

Why text wins here:

- **The flattening emitter is naturally sequential.** Each `generate*` appends statements and returns an r-value name. With `go/ast` you'd build `[]ast.Stmt` slices, thread `*ast.BlockStmt` parents around, and construct 5 nested struct literals to say `tmp3 := rt.Apply1(f, x)`. Roughly 4–6× the code for zero semantic gain.
- **`go/format.Source` gives us the two things `go/ast` promises anyway**: canonical formatting and a hard syntax check (it parses). Malformed output fails at generation time with a Go parse error, not mysteriously at `go build`.
- **Golden-file tests are trivial** with text: `testdata/codegen/*.glj` → expected `.go` (Glojure has exactly this suite, plus `codegengotest` compiling and running the output).
- go/ast's real advantages — programmatic rewriting, type-checked construction — belong to *transformation* tools. We never re-read our own output; we generate once, linearly.

Structural conventions the emitter enforces (all visible in both refs): deterministic output (sort vars/symbols/keywords before emitting — glojure sorts everything, codegen.go:218,304,316,330 — so diffs are stable and builds reproducible); interned symbols/keywords hoisted to **package-level vars** created once (`var kw_foo = rt.InternKeyword("", "foo")`) instead of allocating at each use — *[EDIT NOTE: was "per-`Load` locals"; resolved to package-level per design 02 §3.2 / 00-architecture §4.4 — interning is idempotent and side-effect free, so package-init interning doesn't violate the explicit-`Load()` ordering, and closures need no capture]*; imports collected in a map during generation and rendered into the header last; JVM-Clojure-compatible munging (`-`→`_`, `?`→`_QMARK_`, `!`→`_BANG_`, `*`→`_STAR_`) plus cljs2go's extra rule for Go: names starting with `_` get an `X` prefix so they can be exported (`-main` → `X_main`, README "How?" section).

---

## 7. Milestone plan

**v0 — "hello, factorial" (the vertical slice).** Input language: `ns` (require-less, but with `(:require-go ...)` of Go packages), `def`, `defn`/`fn` (single + multi-arity + variadic), `if`, `do`, `let`, `loop`/`recur`, literals (nil, bool, int64, float64, string, keyword), invoke — including **direct calls to imported Go package-level functions via go/packages signatures** (`(fmt/Println ...)`, `(strings/ToUpper ...)`) — and a micro-core provided by hand-written `rt`: `+ - * / < > <= >= = inc dec not println str`. Deliverable: `gclj build src/hello/core.clj && ./hello` prints and exits, from a generated `go.mod` module.

1. `rt` package: `Value`, `Fn`, `Apply0..4/N`, `IsTruthy`, `CheckArity(GTE)`, `Var`/`Namespace` (interning, `BindRoot`, `Get`), micro-core vars, `rt.Init()`.
2. Emitter skeleton: `Generator{buf, scopes, temps, recurStack, imports}`; `generate(node) string` dispatch on AST op; `Load()`/package/header assembly; `format.Source` gate.
3. Ops in order: const → def → invoke → fn(single) → if → do → let → fn(multi/variadic) → loop/recur. Golden test per op; compile-and-run test per milestone (Glojure's two-tier test layout).
4. Interop v0: `(:require-go ...)` → aliased imports; go/packages loader in the compiler; direct calls with doc 05 §2 result shaping (`[v err]` plain / `!` throws) for package-level functions; `go.mod` creation + `go mod tidy` in the driver.
5. `main` package emission + `go build` driver (write to `gen/`, run `go build ./...`, surface Go errors mapped back to the offending top-level form — keep `// (defn greet ...)` provenance comments on each form's block, as both refs do).

**v0.5** — multiple namespaces + `:require` (per-package `Load()` chaining), third-party deps with version pinning (`go get pkg@version` from deps config), struct construction + typed-receiver method calls, reflective fallback for untyped receivers, `defn-`/metadata on vars, `comment`, top-level side effects. **v1** — `deftype`→struct, `defprotocol`→interface + boxed fallback, `try/catch/throw` via panic/recover (glojure codegen.go:1640–1728 is the template), maps/vectors/sets literals backed by real persistent collections from the runtime component. **v2** — the §5 performance ladder, receiver-type inference to retire the reflect fallback, and start compiling `clojure/core.clj` itself instead of the hand-written micro-core.

The non-goal fence for v0: no `eval` (cljs2go's README enumerates why runtime codegen in Go is a research project — plugins/cgo/RPC; not our fight), no `binding`/dynamic vars, no lazy seqs (runtime component's problem), no macros in user code (v0 programs use only builtin special forms; macroexpansion lands with the analyzer's compile-time interpreter).
