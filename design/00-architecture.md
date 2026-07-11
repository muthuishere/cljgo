# 00 — Consolidated Architecture

Synthesis of design docs 01–05. Where this doc and a component doc disagree,
**this doc wins** — it records the cross-doc reconciliations. Each component
doc remains authoritative for its own internals.

Project/CLI name: **`cljgo`** (settled by the owner 2026-07-11; repo
`github.com/muthuishere/cljgo`). The reader-conditional feature key becomes
`:cljgo`; docs written earlier may still say `gclj`/`clgo`/`gljgo` — treat
all as `cljgo`.

---

## 1. Vision & priorities

A Clojure implementation hosted on Go, compiled **in Go** (single toolchain),
that AOT-emits plain Go source — the ClojureScript model, with Go as the
JavaScript — plus a tree-walk evaluator that IS the REPL and the macro engine.

Priorities, in order:

1. **Universal interop.** Any Go module is importable and callable with zero
   hand-written bindings — the Go ecosystem is our standard library, the way
   the JVM was Clojure's. C reaches in two ways: cgo-based Go bindings flow
   through ordinary imports; raw C libraries via `ffi/` on purego
   (`dlopen`/`dlsym`, no C toolchain, works live at the REPL). Doc 05 owns
   this surface; doc 04 owns the AOT mechanics (`go/packages` type facts →
   direct, non-reflective calls).
2. **Full REPL-driven development.** The tree-walk evaluator is a real
   Clojure REPL: live re-`def` visible to existing callers, `defmacro`
   effective on the next form, `in-ns`, `require`/`load`, `*1 *2 *3 *e`,
   nREPL for CIDER/Calva. Doc 03 §7 is the acceptance contract.
3. **Faithful Clojure principles.** Persistent data structures with real
   structural sharing, `=`/`hash` category semantics, vars as the mutable
   indirection layer, macros as plain fns, seq abstraction everywhere.
   Where Go forces a deviation (typed nil, no STM, RE2 regex) we document it
   loudly rather than approximate silently.
4. **High performance is a feature, not an option** (owner mandate
   2026-07-11: "no compromise"). Applies to BOTH modes: the evaluator must be
   a fast tree-walk (pre-resolved locals by index, no per-eval map lookups,
   Apply0..4 non-allocating fast paths), and the emitter's performance ladder
   (doc 04 §5 — direct calls, typed/unboxed locals, primitive hints,
   intrinsics) is core roadmap, not "post-M5 someday": S6 benchmarks gate M2,
   and every milestone carries a perf budget (M0: REPL eval within ~5x of
   Glojure's interpreter or better; M2: emitted factorial within ~10x of
   handwritten Go, ladder shrinking it after). Perf regressions are treated
   like conformance failures — benchmarked in CI, not vibes.
5. **cgo builds are mandatory.** Emitted projects must build with
   `CGO_ENABLED=1` so cgo-based Go modules (sqlite drivers, sensors,
   GUI/audio bindings — a large slice of the real ecosystem) work like any
   other import; `cljgo build` must pass through the C toolchain env
   (CC/CXX, build tags) and document the cross-compile implications (zig cc
   as the escape hatch). Pure-Go stays the default for painless
   cross-compilation; cgo is a first-class supported mode, not an accident.
   purego/ffi (doc 05) remains the REPL-friendly complement, not a substitute.

The one unforgivable failure mode (doc 03 §7d): **REPL behavior diverging
from compiled-binary behavior.** Everything below is arranged to make that
structurally hard — one reader, one analyzer, one AST, one runtime package,
and a conformance suite that runs every semantic test through both paths.

---

## 2. Dual-mode pipeline

```
                    ┌──────────────────────────────────────────────┐
                    │              compile-time macros              │
                    │   (analyzer calls evaluator to run macro fns) │
                    │                      ▲                        │
                    ▼                      │                        │
  UTF-8 text ──▶ Reader ──forms──▶ Analyzer ──*ast.Node──┬──▶ Tree-walk Evaluator ──▶ REPL / nREPL / scripts
   (pkg/reader)   (pkg/lang data    (pkg/analyzer)       │      (pkg/eval)
                   + line/col meta)                      │
                                                         └──▶ Go Emitter ──.go──▶ go/format ──▶ go build ──▶ binary
                                                                (pkg/emit)

                    both consumers link the SAME pkg/lang runtime
```

- **One analyzer, two consumers.** The emitter never re-analyzes and holds no
  private special-form knowledge. Any new Op lands in both consumers before
  merge (exhaustive `switch n.Op`, `default: panic`).
- **Macros expand identically by construction**: both paths run macroexpansion
  through the same analyzer, which invokes macro fns via the same evaluator.
  The AOT compiler links the evaluator for compile time; compile time = eval
  time for macros.
- **Shared conformance suite**: every semantic test is a `.clj` file +
  expected output, executed in CI through tree-walk eval AND through
  AOT-compile-and-run (doc 03 §7d). A test that can't run both ways needs a
  written waiver in the file. Divergence is a release blocker. The suite
  starts eval-only at M0 and gates every merge dual-harness from M2.
- Interop has a per-path mechanism with one semantics (doc 05 §5): the
  interpreter uses a generated name→value registry + reflection (with the
  deps.edn `go get` → regen → self-rebuild flow for third-party modules);
  the emitter uses `go/packages` type facts and direct calls. Same shaping
  rules ([v err], nil-normalization, coercions) on both.

---

## 3. Repo / package layout

Single Go module (path placeholder `github.com/muthuishere/cljgo`):

```
pkg/lang        THE runtime. Vendored+owned Glojure pkg/lang (EPL headers kept),
                reshaped freely: Equiv/Equals split, HAMT+vector transients,
                murmur3 flattened in, interpreter glue deleted. Values, colls,
                Symbol/Keyword/Var/Namespace/Atom/LazySeq, IFn, Apply0..4/N,
                IsTruthy, print. Emitted code and the evaluator both link
                ONLY this. (Doc 04 calls it `rt` — same package; see §4.2.)
pkg/reader      Text → pkg/lang data with position metadata (doc 01).
                Depends only on pkg/lang; Resolver injected.
pkg/ast         Node{Op, Form, Sub} + per-op payload structs (doc 03 §1).
                No dependencies beyond pkg/lang. Analyzer is sole writer.
pkg/analyzer    Forms → AST. Pure, dependency-injected (Macroexpand1 hook,
                var/host resolution hooks). Never imports pkg/eval.
pkg/eval        Tree-walk evaluator + runtime Scope. Wires analyze↔eval for
                macros. The REPL engine.
pkg/emit        AST → Go source text → go/format.Source. Owns munging, temp/
                scope bookkeeping, Load() assembly, go.mod driver, go build.
pkg/host        Interop machinery shared by both paths: go/packages loader +
                signature cache (AOT), genpkg-style registry generator +
                reflect call shaping (interpreted), coercion/nil-normalization
                tables, ffi/ (purego).
pkg/repl        REPL driver (Read→Analyze→Eval→bind *1/*e→print); terminal
                frontend. pkg/nrepl later (bencode/server skeleton vendored
                from Glojure, ops rewritten onto the driver).
core/           clojure/core.clj etc. — the Clojure-in-Clojure standard
                library, loaded by the evaluator and (later) AOT-compiled.
cmd/cljgo       CLI: repl / run / build / deps sync.  (name = placeholder)
```

Not yet written: a **doc 06 (project layout / deps / load-path
conventions)** — doc 03 §7a already cites "design 06" for the load-path
(classpath equivalent), and doc 05's `deps.edn` format belongs there too.
Flagged here so the reference doesn't dangle silently.

Emitted output is a **separate, user-owned Go module** (`gen/` by default):
one namespace → one Go package, `go.mod` created once and never overwritten,
deps pinned from `deps.edn`, `go mod tidy` + `go build ./...` as the driver
(doc 04 §1–2).

**Toolchain: pin `go 1.26`** in both our module and every generated
`go.mod` (machine has go1.26.3; let-go already targets 1.26, Glojure 1.24 —
vendored code compiles fine under 1.26).

### 3.1 Modern Go we exploit

Recent Go removes work the older refs had to hand-roll:

- **`unique` package (1.23)** — `unique.Handle[T]` gives canonical, identity-
  comparable, weakly-held interning. Keyword (and symbol-name) interning in
  pkg/lang can sit on it instead of a grow-forever `sync.Map` table: the
  §4.4 contract (`k1 == k2`, package-level vars) is unchanged, and unused
  keywords become GC-able — the very thing Glojure pulled in `go4.org/intern`
  (with its assume-no-moving-gc hack) to approximate. That dep drops for free.
- **Range-over-func iterators (1.23)** — `iter.Seq[any]` is the natural
  Go-side view of ISeq: pkg/lang exposes an `ISeq → iter.Seq[any]` bridge
  (and `iter.Seq2` for maps), so hand-written Go and emitted code can
  `for v := range lang.Iter(coll)` — and interop hands Clojure collections
  to Go APIs that accept iterators with zero copying.
- **`weak` package + `runtime.AddCleanup` (1.24)** — proper weak refs and
  finalization for caches (e.g. the go/packages signature cache, memoized
  regex compilation) without `SetFinalizer` footguns.
- **Swiss-table builtin maps (1.24)** — our internal tables (analyzer locals,
  emitter scopes, intern/registry maps) get faster for free; irrelevant to
  persistent-collection semantics, which stay HAMT.
- **Generics (+ generic type aliases, 1.24)** — the emitter's performance
  ladder (§4.2: fixed-arity fn types, typed fast paths, `chan T` interop
  ops) emits generic helpers instead of per-type codegen.
- **`testing/synctest` (stable 1.25)** — deterministic virtual-time testing
  of goroutine/channel semantics: the M4 async conformance suite (timeouts,
  buffer policies, alts!) runs fast and flake-free.
- **Container-aware `GOMAXPROCS` (1.25)** — emitted binaries behave well in
  containers with no runtime tuning by us.
- 1.26-specific runtime work (e.g. the Green Tea GC becoming default) —
  **verify against the 1.26 release notes** before claiming; we take it as
  "binaries get faster", not as a design input.

---

## 4. Resolved cross-doc contracts (authoritative statements)

### 4.1 AST node shape

`pkg/ast` defines exactly doc 03 §1: one uniform `*Node{Op uint8-enum, Form
any, Sub any}` with typed per-op payload structs (`IfNode`, `FnNode`, ...),
op vocabulary tracking cljs.analyzer/tools.analyzer. The analyzer is the only
writer; evaluator and emitter are read-only consumers dispatching on
`n.Op`; passes that need annotations use side tables keyed by `*Node`.
Doc 04's emitter consumes this AST as-is ("generate(node) dispatch on AST
op") — it compiles **forms, never runtime values** (Glojure's
eval-then-serialize model is explicitly rejected, doc 04 §0).

### 4.2 Value model & calling convention

- Values are Go `any` (doc 02 §1.1): `nil bool int64 float64 string
  lang.Char`, collections as pointer types implementing the small interface
  set. `rt.Value` in doc 04 is a type alias for `any` in this same package.
- **There is one runtime package: `pkg/lang`.** Doc 04's `rt` and docs
  01–03's `lang` are the same import; emitted code uses whichever alias we
  standardize (default `lang`).
- Fns: closures emit as `lang.Fn func(args ...any) any` (satisfies IFn via
  an Invoke method); multi-arity = one `switch len(args)` inside, variadic
  as `default` with floor check. The evaluator's `*evalFn` also satisfies
  IFn. Call sites go through `lang.Apply0..Apply4` fast paths (no varargs
  slice) or `Apply` beyond 4; `Apply*` dispatches: `lang.Fn` → call, `IFn` →
  interface call, Go `func` → reflect bridge, keyword/coll → their IFn
  behavior. Doc 02 M2's "compiled-fn struct (arity-switch Invoke)" is
  realized as this func type + switch — same thing, one representation.
- Seam to pin during M0: internal evaluation returns `(any, error)` (recur
  sentinel, exceptions); `IFn.Invoke` returns `any` only — at the IFn
  boundary the evaluator converts its error to a panic (matching emitted
  code, where exceptions are panics). One conversion point, in pkg/eval.
- Truthiness is a single helper `lang.IsTruthy` used by both paths.
- Direct linking (bypassing Var indirection) is **forbidden in the evaluator
  and default-off in the emitter** (docs 03 §7a / 04 §5) — REPL re-def must
  stay live. Spike S6 proved per-call atomic Var deref costs ~2% (1.7ns,
  0 allocs): liveness is effectively free and stays on everywhere.
- **Fixed-arity fn fields are M2's DEFAULT emission, not ladder rung**
  (spikes S1+S6, 2026-07-11): the variadic `func(...any) any` convention
  heap-allocates an args slice per call and lands at 20–22× raw Go —
  failing the §1.4 perf budget — while known-arity call sites through
  fixed-arity closure fields (`f.Deref1()(x)`) with per-call var deref hit
  3.5–7.8× raw with ~650× fewer allocs. Variadic `Fn`/`Apply` remains the
  apply/HOF/interop fallback only. The rest of the doc 04 §5 ladder
  (primitive hints, intrinsics, opt-in direct linking) stays post-M2.

### 4.3 Error mapping for Go interop — doc 05 wins

Go multi-returns shape as **doc 05 §2**: plain call returns `[v err]` (a
vector; trailing `error`/`bool` detected by type), and a **`!`-suffixed call**
(`os/Open!`, `.Method!`) is compiler sugar that unwraps and throws on
non-nil error / false ok, wrapping the Go error in our exception type
(original via `(ex-go-error e)`). Detection is static via `go/types` in AOT
and via `reflect.Type` in the interpreter — identical semantics both paths.
Doc 04's original unconditional `(T, error) → panic` mapping is superseded
(corrected in doc 04 §2 and §7). Exceptions in emitted code propagate as
`panic`/`recover` under `try`/`catch`; thrown values satisfy Go `error`.

### 4.4 Keyword (and symbol) interning — doc 02 wins

Keywords are globally interned, identity-comparable (`k1 == k2`). The
emitter hoists every keyword/symbol literal to a **package-level var**:
`var kw_foo = lang.InternKeyword("", "foo")`. This is safe alongside doc
04's explicit `Load()` model because interning is idempotent, side-effect
free, and order-independent — package-init interning does not violate
sequential top-level semantics, and closures reference the var without
capturing Load-locals. Doc 04 §6's "per-`Load` locals" is superseded
(corrected in place). Constructor name is `InternKeyword`, not `NewKeyword`.

### 4.5 Reader metadata flow

The reader attaches `:file :line :column :end-line :end-column` to every
IObj form (keys defined in pkg/lang, doc 01 §3); primitives inherit the
enclosing form's position downstream. The analyzer carries the original form
on `Node.Form` — position rides on its metadata — and analysis errors report
from it; the emitter uses it for provenance comments and for mapping
`go build` errors back to source forms. One metadata convention, three
consumers.

### 4.6 Interop surface syntax — doc 05 wins

Go packages enter via `(:require-go [net/http :as http] ...)` in `ns` (and
`require-go` at the REPL), aliases becoming namespace prefixes. Doc 04's
`(:import ...)` examples are superseded (noted in place); `:import` stays
reserved for a possible future type-import form. `go/` is the reserved
pseudo-namespace for interop operators (`go/new`, `go/instantiate`,
`go/slice-of`, ...). Dot forms (`(.Do c req)`, `(.-Timeout c)`,
`(http/Client. {...})`) per doc 05 §1.1; no auto-capitalization of member
names.

### 4.7 Concurrency forms in the shared AST

`(go body)`, channel ops and `alt!` are language-level forms both consumers
must understand (evaluator: real goroutines + runtime helpers; emitter:
`go func(){}` literals and, for static `alt!`, a real `select`). Per §4.1
discipline, they enter as ops/intrinsics added to analyzer + both consumers
together — doc 05 defines the semantics (blocking `<!`/`>!`, `<!!` as
aliases, result channel from `go`, buffer policies, nil-normalization).
STM is skipped (doc 05 §4); atoms/agents/future/promise are the reference
types.

---

## 5. Glojure: what we vendor, what we never copy

We vendor with full ownership — EPL-1.0 headers preserved, then reshaped and
bug-fixed freely (file-scoped weak copyleft; the rest of the compiler is
unencumbered).

**Vendor (saves months):**
- `pkg/lang` persistent structures + numeric tower + murmur3 + vars/atoms/
  lazy-seqs (doc 02 §4, option 3) — then fix in place: split Equiv/Equals,
  add HAMT + Clojure-shaped vector transients, drop `go4.org/intern`/
  `hashstructure`/`pcastools`, delete interpreter glue (`builtins.go`,
  `class.go`, reflect FnFuncs).
- Reader scaffolding ideas: `trackingRuneScanner`, posStack metadata, ErrEOF
  contract, pending-forms queue (doc 01 §4 "copy" list).
- `pkg/nrepl` bencode codec + server/session skeleton (ops rewritten).
- genpkg's `go/types`-walking registry generator and the deps.edn
  `go get → regen → self-exec rebuild` flow for the interpreter (doc 05 §1).
- Codegen *techniques* as reference: statement flattening, varScope stack,
  recur goto/continue patterns, `format.Source` gate, golden-test layout.

**Never copy:**
- The eval-then-serialize-the-namespace compilation model (doc 04 §0 —
  liftedValues, valueInits toposort, panics on opaque fns).
- `Equiv ≡ Equals` conflation; per-map-length gensym counter; keyword
  auto-namespacing; `strconv.Unquote` string reading; `[tag form]` fallback
  for unknown tagged literals (doc 01 §4 "redo" list).
- The `net:http.MethodGet` munged-global interop surface (doc 05 §1.1).
- pkgmap registry + reflection as the *emitted-code* calling mechanism —
  our emitter calls directly; the registry is interpreter-only.
- IIFE expression emission (cljs2go's mistake, doc 04 §3).

let-go is a semantics cross-check donor only (async suite, chanPolicy,
goroutine tracking, HAMT cross-checks) — its boxed `Value` model contradicts
the `any` decision.

---

## 6. Build-order roadmap

Each milestone has a concrete demo; the shared conformance suite starts at M0
(eval-only) and runs dual-harness from M2 onward.

**M0 — REPL evaluates arithmetic.**
Reader Phase 0 (doc 01); pkg/lang M0+M1 vendored+pruned, compiles with zero
external deps (doc 02); analyzer/eval v0 nodes (Const, coll literals, Var,
Local, Do, If, Def, Let, Binding, Fn, FnMethod, Invoke, Quote); one `user`
ns; `+ - * / = < >` as pre-interned native IFns; REPL driver with `pr-str`.
Demo: `(def fact (fn* fact [n] (if (< n 2) 1 (* n (fact (- n 1))))))`
`(fact 10)` → `3628800`; re-`def` visible to a captured reference.

**M1 — fn/let/loop/macros at the REPL.**
Reader Phase 1 (syntax-quote with global gensym counter, `#()`, `#'`, regex);
eval v1 (loop*/recur with analysis-time tail+arity checks, dynamic vars +
push/popThreadBindings, set!, in-ns/aliases/refer, `*1 *2 *3 *e`) and v2
(macroexpand1 in analyzeSeq, `&form`/`&env`, bootstrap defmacro; begin
core.clj: defn, destructuring let, when, and, or, ->).
Demo: `(defmacro unless [t e] (list 'if t nil e))` typed at the prompt works
on the next form; iterative `loop*` factorial at n=100000, constant stack.

**M2 — first emitted Go binary.**
Emitter v0 (doc 04 §7): flattening generator, Load()-per-namespace, fn
emission (single/multi/variadic), recur via goto/continue, munging,
format.Source gate, `main` emission, go.mod creation + `go build` driver;
golden tests + compile-and-run tests; dual-harness conformance suite gates
every merge from here (doc 03 v4: emitter consumes M0–M1 ASTs with zero
re-analysis).
Demo: `cljgo build src/hello/core.clj && ./hello` — factorial prints from a
static binary; startup < 50ms.

**M3 — Go interop, both modes.**
Interpreted: registry generator for stdlib, `:require-go` aliases, cached
member reflection, [v err] + `!` shaping, nil normalization; deps.edn
`:go-deps` → go get + regen + self-rebuild (doc 05 M1–M2). AOT: go/packages
signatures → direct calls, coercions, boxing elision, emitted go.mod pinning
(doc 05 M4 / doc 04 interop v0).
Demo: `gorilla/websocket` driven from the REPL with zero bindings; the same
program AOT-compiles with zero interop `reflect` in the emitted Go.

**M4 — goroutines & channels.**
`chan close! >! <! alts! timeout`, buffer policies, `go`/`thread` result
channels, goroutine tracking/drain (REPL cancellation), atoms hardened;
emitter: `(go ...)` → `go func(){}`, static `alt!` → `select` (doc 05 M3+M5
concurrency half).
Demo: a producer/consumer pipeline with `alts!` + `timeout` runs identically
at the REPL and as a compiled binary; let-go's async semantics suite passes.

**M5 — self-hosting core.clj subset.**
letfn*/throw/try/catch/finally + new/`.` complete (eval v3); `load`/`require`
over load paths; grow core.clj to a real subset (seq library over lazy-seqs,
chunked later); AOT-compile core.clj itself replacing the hand-written
micro-core (doc 04 v2); deftype→struct + defprotocol→interface begin; nREPL
lands on the REPL driver (eval v5).
Demo: the same `core.clj` source loads interpreted at the REPL and links
AOT-compiled into binaries; CIDER connects, evals, and sees `*1`.

Post-M5 (sequenced, not scheduled): performance ladder (doc 04 §5),
receiver-type inference retiring the reflect fallback, `ffi/deflib` on
purego, `go/instantiate` generics, agents/mult/pub/pipe, reader Phase 2
fidelity (namespaced maps, reader conditionals, tagged literals).
