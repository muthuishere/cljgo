# 03 — Analyzer + Tree-Walk Evaluator

Component: forms → AST → evaluation. The evaluator serves the REPL and
compile-time macroexpansion; the **same AST** later feeds the Go emitter
(design 04) — a second consumer, never a second analyzer.

**A real Clojure REPL is a first-class requirement.** Go cannot eval at
runtime, so the tree-walk evaluator IS the REPL engine — its Clojure
fidelity *is* the REPL's fidelity (re-def live fns, interactive macros,
namespace hopping, file loading, exactly as on JVM Clojure). See §7.

```
reader (02) ──forms──▶ Analyzer ──*ast.Node──▶ TreeWalkEval   (REPL, macros)
                                      └───────▶ GoEmitter     (AOT, design 04)
```

Evidence base (read, not guessed): `clojure/lang/Compiler.java` — `specials`
map (l.110), `analyze*` (l.7335–7900), `macroexpand1` (l.7568), `eval`
(l.7713); `refs/glojure/pkg/{ast,compiler,runtime}` — a working Go port of
tools.analyzer + tree-walk eval; `refs/cljs2go/src/cljs/go/compiler.clj` —
`(defmulti emit* :op)` over cljs.analyzer's map AST, proof that a tagged-op
AST cleanly decouples analysis from emission.

---

## 1. AST node design (Go)

### Decision: one uniform `*Node` with an integer `Op` + typed per-op `Sub` struct

This is Glojure's shape (`ast.go`), which is itself cljs.analyzer's `{:op ...}`
map translated to Go. We adopt it with small changes.

```go
package ast

type Op uint8

const (
    OpConst Op = iota + 1
    OpVector; OpMap; OpSet          // collection literals (analyzed children)
    OpVar; OpLocal; OpTheVar        // symbol resolutions
    OpDo; OpIf; OpDef; OpLet; OpLoop; OpBinding
    OpFn; OpFnMethod; OpInvoke; OpRecur
    OpQuote; OpSetBang; OpLetFn
    OpThrow; OpTry; OpCatch
    OpNew; OpHostCall; OpHostField; OpHostInterop; OpMaybeClass
)

type Node struct {
    Op   Op
    Form any            // original form (line/col metadata rides on it)
    Sub  any            // pointer to the Op-specific struct below

    IsLiteral    bool   // constant-foldable
    IsAssignable bool   // set! target
}
```

Per-op payloads (full list in §2; representative sketches):

```go
type ConstNode  struct { Value any }                      // OpConst
type IfNode     struct { Test, Then, Else *Node }         // OpIf
type DoNode     struct { Statements []*Node; Ret *Node }  // OpDo
type DefNode    struct { Name *lang.Symbol; Var *lang.Var; Init *Node; Meta *Node }
type LetNode    struct { Bindings []*Node /*OpBinding*/; Body *Node; LoopID string }
type BindingNode struct { Name *lang.Symbol; Init *Node; Kind BindKind /*let|arg|fn|letfn|catch*/; ArgID int; IsVariadic bool }
type VarNode    struct { Var *lang.Var }
type LocalNode  struct { Name *lang.Symbol; Binding *BindingNode }
type InvokeNode struct { Fn *Node; Args []*Node }
type RecurNode  struct { Exprs []*Node; LoopID string }
type FnNode     struct { Methods []*Node /*OpFnMethod*/; IsVariadic bool; MaxFixedArity int; Local *Node /*self-name binding*/ }
type FnMethodNode struct { Params []*Node /*OpBinding*/; FixedArity int; IsVariadic bool; Body *Node; LoopID string }
type QuoteNode  struct { Value any }                      // unanalyzed datum
type SetBangNode struct { Target, Val *Node }
type TryNode    struct { Body *Node; Catches []*Node; Finally *Node }
type CatchNode  struct { TypeSym *lang.Symbol; Local *Node; Body *Node }
type ThrowNode  struct { Exception *Node }
type LetFnNode  struct { Bindings []*Node; Body *Node }
type NewNode    struct { TypeSym *lang.Symbol; Args []*Node }
type HostCallNode struct { Target *Node; Method *lang.Symbol; Args []*Node }
```

**Why structs, not generic maps** (cljs.analyzer uses maps):
1. **Two consumers, both in Go.** cljs's map AST buys open extension —
   valuable in Clojure, expensive in Go where every read is a type
   assertion. We own both consumers (eval + emit), so closed, typed payloads
   win: the compiler checks `IfNode.Else` exists; a map fails at runtime.
2. **Integer `Op` + `Sub`, not a sealed interface hierarchy:** Glojure
   benchmarked this (`ast.go` l.9–11) — switching on an int beats a type
   switch — and both consumers become one flat `switch n.Op`, trivially
   audited against the specials list. One node type also gives one uniform
   home for `Form`, position info, and flags.
3. **Keep cljs's *vocabulary*.** Op names and payload shapes track
   cljs.analyzer/tools.analyzer (`:op :if` → `OpIf{Test Then Else}`) so the
   emitter can crib cljs2go's per-`:op` emit catalog one-to-one.

Contract: the analyzer is the only writer; both consumers are read-only.
Passes needing annotation get side tables keyed by `*Node`, not mutation.

---

## 2. Special forms — analysis of each, ordered by implementation phase

Reference: `Compiler.java specials` (l.110) + Glojure `parse` dispatch
(analyze.go l.384). The seq path is Compiler.java's `analyzeSeq`:
macroexpand-1; if changed, re-analyze; else dispatch on the operator
symbol's full name; anything not in these tables is `parseInvoke`.

### Phase 1 (v0 REPL)

| form | analysis |
|---|---|
| `quote` | `(quote x)` → `OpQuote{Value: x}` — the datum is **not analyzed**; it is a constant. `IsLiteral=true`. |
| `if` | 2 or 3 args, else error. Test analyzed in expression context; Then/Else analyzed; missing else → `OpConst{nil}`. Truthiness (nil/false are false) is the **evaluator's** job, not the analyzer's. |
| `do` | `OpDo{Statements: all-but-last, Ret: last}`; empty body → Ret = const nil. Statements analyzed in statement context (emitter cares; eval ignores). |
| `def` | `(def sym)`, `(def sym init)`, `(def sym doc init)` (Compiler.java DefExpr.Parser). Name must be an **unqualified** symbol (or qualified into the *current* ns). **Interns the Var at analysis time** in `*ns*` — this is load-bearing: it makes forward references and self-recursion resolvable. Init analyzed if present; symbol metadata (`:dynamic`, `:macro`, `:doc`) analyzed onto the var. |
| `let*` | Even-count binding vector of simple (non-namespaced, non-dotted) symbols. Analyze each init in the env-so-far, then extend the analysis env with an `OpBinding` — sequential (`let*` is Scheme `let*`). Shadowing = later binding wins. Body as implicit `do`. `LoopID=""` (not a recur target). |
| `fn*` | See §5. Produces `OpFn` with one `OpFnMethod` per arity. |
| *invoke* | Non-special, non-macro seq → `OpInvoke{Fn, Args}`, everything in expression context. `(nil ...)` is an analysis error ("Can't call nil", Compiler.java l.7681). |

### Phase 2 (loops, vars)

| form | analysis |
|---|---|
| `loop*` | Same parser as `let*` (Compiler.java uses `LetExpr.Parser` for both) but `LoopID = gensym("loop_")` and the **body** is analyzed with `env.RecurFrame = {LoopID, arity: len(bindings), isLoop: true}`. Body context is `return` (recur must be in tail position). |
| `recur` | Legal only when `env.RecurFrame != nil` **and** context is `return` (tail position) — otherwise "Can only recur from tail position". Arg count must equal the frame's arity, checked **at analysis time** (Compiler.java RecurExpr.Parser does this; Glojure defers to runtime — we follow Clojure). Emits `OpRecur{Exprs, LoopID}`. Args analyzed with RecurFrame cleared (no recur inside recur args). |
| `var` | `(var sym)` → resolve to an existing Var or error ("Unable to resolve var"); `OpTheVar{Var}`. Evaluates to the Var object itself, not its value. |
| `set!` | `(set! target val)`. Target analyzed; must have `IsAssignable` — Phase 2: only `OpVar` of a **dynamic var with a thread binding** (evaluator enforces the binding check at runtime, as Clojure does); Phase 4 adds host fields. |

### Phase 3 (macros — see §4; no new special forms, but `def` grows `setMacro`)

### Phase 4 (exceptions, letfn, interop)

| form | analysis |
|---|---|
| `letfn*` | Bindings are `[name (fn* ...)]*`. Unlike `let*`, **all names enter the analysis env first**, then all inits are analyzed — mutual recursion. `OpLetFn`. |
| `throw` | One arg, analyzed in expression context → `OpThrow`. Thrown value must satisfy Go `error` at runtime (our Throwable analogue). |
| `try` | Body exprs until first `(catch ...)`/`(finally ...)`; after that only catch/finally allowed ("Only catch or finally clause can follow catch..."), at most one finally, must be last. Each catch: `(catch TypeSym binding-sym body...)` — binding-sym enters a fresh scope for that catch body. `catch`/`finally` are specials **only inside try** (in `specials` with nil parser, error if seen elsewhere). Body analyzed with RecurFrame cleared (no recur across try). |
| `new` | `(new T args...)` → `OpNew`. In our Go world `T` names a registered Go type/constructor in the host registry (design 05); analyzer just records the symbol + analyzed args. |
| `.` | `(. target member args...)` / `(. target (member args...))` → `OpHostCall` (args present) or `OpHostField`/`OpHostInterop` (bare member, resolution deferred to runtime/emitter). Sugar `(.m x a)` and `(T. a)` are rewritten **in macroexpand1** (Compiler.java l.7615–7649), not here. |

Not planned initially: `case*`, `deftype*`, `reify*`, `monitor-enter/exit`,
`import*`. `ns`/`in-ns` come from core as macros/fns.

---

## 3. Environments

Two distinct environments — do not conflate them:

### 3a. Analysis-time env (compile-time, immutable, threaded through analyze)

```go
type AnalyzeEnv struct {
    Locals     map[string]*ast.BindingNode // copied-on-extend (small; fine)
    Context    Ctx                         // CtxExpr | CtxStatement | CtxReturn
    RecurFrame *RecurFrame                 // nil unless inside loop*/fn method
    IsTopLevel bool
}
type RecurFrame struct { LoopID string; Arity int }
```

Glojure threads a persistent map (`Env IPersistentMap` with `:locals`) to
stay close to tools.analyzer; we use a plain struct with copy-on-extend of
the locals map at each binder — valuewise-immutable, invariants visible.

**Symbol resolution** (`analyzeSymbol`, Compiler.java l.7867 / Glojure l.66):
1. If the symbol has **no namespace**: check `env.Locals` — hit → `OpLocal`
   pointing at the *analysis-time* BindingNode (locals always shadow vars).
2. Else resolve against namespaces (Compiler.java `lookupVar`):
   - qualified `ns/name`: resolve ns (real name or alias in current ns), find
     interned var; missing → "No such var" / "No such namespace".
   - unqualified: current ns mapping (interned or referred var); missing →
     "Unable to resolve symbol: x in this context".
3. Resolved var → `OpVar`. A symbol naming a registered Go value/type →
   `OpConst` / `OpMaybeClass` (host registry, design 05).

### 3b. Namespaces and Vars (the global, mutable world)

```go
type Namespace struct {
    Name     *Symbol
    mappings atomic.Value // map[string]any: name → *Var (interned or referred)
    aliases  atomic.Value // map[string]*Namespace
}
type Var struct {
    NS *Namespace; Sym *Symbol
    root atomic.Value        // root binding
    dynamic, macro bool      // from metadata
    meta atomic.Value
}
```

- `def` semantics: **intern** = create-or-find var `ns/sym` in the current ns,
  set root binding if init supplied; re-`def` replaces the root, never the
  identity — existing references see the new value (that's the whole point of
  vars for REPL development).
- `*ns*` is itself a dynamic var; `in-ns` swaps it. The runtime holds a
  thread-binding stack for dynamic vars (`push/popThreadBindings`) — needed
  for `binding`, `set!`, and the compiler's own `*ns*`.
- Var deref in eval: thread binding if present and dynamic, else root.

### 3c. Runtime lexical env (evaluator only)

Glojure's `scope.go` verbatim — a parent-linked mutable frame:

```go
type Scope struct { parent *Scope; vals map[string]any }
func (s *Scope) Lookup(name string) (any, bool) // walk up
```

The emitter never sees this: for AOT, locals become Go variables. Closure
capture = an `*evalFn` holding the `*Scope` live at `fn*` evaluation (§5).

---

## 4. Macroexpansion

- **Where defmacro lives:** in `core.clj`, as in Clojure — bootstrapped by a
  tiny pre-interned hand-built `defmacro` (`(def name (fn* ...))` +
  `setMacro`). A macro is just a Var whose value is an IFn and whose meta
  has `:macro true`; `setMacro` flips the flag.
- **The loop:** exactly Compiler.java. `analyzeSeq` calls `macroexpand1`
  once; if the form changed, recurse into `analyze` on the result (this *is*
  the fixed-point loop, and re-entering analyze means intermediate expansions
  that produce specials stop expanding). `macroexpand1(form)`:
  1. non-seq → unchanged.
  2. operator is a special → unchanged (specials are not macros).
  3. operator resolves to a var with `macro=true` → **invoke it now**:
     `applyTo(cons(form, cons(&env, rest(form))))` — the two hidden args
     `&form` and `&env` are prepended, per Compiler.java l.7583 (Glojure
     passes nil for `&env`; we pass the analysis-env locals map so tooling
     macros work). Arity errors subtract the 2 hidden params for the message.
  4. else, interop sugar rewrites: `(.m x a) → (. x m a)`,
     `(T. a) → (new T a)` (Compiler.java l.7615–7649).
- **How macros execute at compile time:** the macro var's value is an
  `*evalFn` (or, once core is AOT-compiled, a native Go func — both satisfy
  `IFn`). The analyzer holds an injected callback
  `Macroexpand1 func(form) (any, error)` (Glojure's `Analyzer` struct,
  analyze.go l.30), so the **analyzer package never imports the evaluator**;
  the runtime wires eval → analyze → (macro? → eval the macro fn) → analyze.
  The AOT compiler links the identical evaluator: compile time = eval time
  for macros.
- User-visible `macroexpand-1`/`macroexpand` expose the same fns in core.

---

## 5. fn: arities, variadics, closures, recur

`parseFnStar` (Glojure l.1278; Compiler.java `FnExpr.parse`):

- Normalize `(fn* name? [params] body...)` → `(fn* name? ([params] body...))`.
- Optional **self-name** becomes `FnNode.Local`, visible only inside the
  fn's own bodies (self-recursion by name without a var).
- Each method: params are simple symbols; at most one `&` followed by exactly
  one rest param (`IsVariadic`); params enter a fresh analysis scope; body is
  implicit `do` analyzed in `return` context with
  `RecurFrame = {LoopID: gensym("fn_method_"), Arity: len(params)}` — **each
  method is its own recur target**, rebinding params, never re-dispatching
  arities.
- Whole-fn checks (Compiler.java): at most one variadic method; no two
  methods with the same fixed arity; no fixed arity greater than the variadic
  method's fixed-arity prefix. Record `MaxFixedArity`, `IsVariadic`.
- **Destructuring is not the analyzer's problem**: `fn`/`let` (no star) are
  core macros that expand destructuring into `fn*`/`let*` with simple symbols.

**Evaluation** (Glojure fn.go is the model):

```go
type evalFn struct {
    node *ast.FnNode
    env  *Scope        // captured lexical scope — this IS the closure
    meta lang.IPersistentMap
}
func (f *evalFn) Invoke(args ...any) any
```

`Invoke`: pick method — exact `FixedArity == len(args)` wins, else the
variadic method if `len(args) >= its FixedArity` (rest packed into a seq;
zero rest → nil binding), else arity error. Push a scope on the **captured**
env (not the caller's — lexical scoping), bind self-name and params, eval
body; on a recur signal for this method's LoopID, rebind and loop.

**Recur without unwinding machinery abuse:** eval returns `(any, error)`.
`OpRecur` evaluates its args (recur target cleared) and returns a sentinel
error `*recurSignal{LoopID string; Vals []any}`. The enclosing `loop*` eval
or fn-method `Invoke` matches **LoopID**: match → rebind, `goto Recur`
(plain Go loop — constant stack); no match → propagate to the outer frame.
Glojure's mechanism, with LoopID matching instead of pointer-identity
targets — self-describing, and it mirrors the emitter (recur → `continue`
to the labeled loop with the same LoopID).

---

## 6. Evaluator shape

One flat dispatch, mirrored by the emitter (cljs2go's `emit*` multimethod):

```go
func (e *Evaluator) Eval(n *ast.Node, s *Scope) (any, error) {
    switch n.Op {
    case ast.OpConst:  return n.Sub.(*ast.ConstNode).Value, nil
    case ast.OpIf:     // eval Test; nil/false → Else, else Then
    case ast.OpVar:    // deref (thread binding else root)
    case ast.OpLocal:  // s.Lookup(name)
    case ast.OpInvoke: // eval fn + args; fnVal.(IFn).Invoke(args...)
    ...
    }
}
```

- Top-level loop = read → `Eval(Analyze(form))` per form; a top-level `do`
  is split and evaluated form-by-form (Compiler.java `eval` l.7744) so
  earlier defs/macros are visible to later siblings in one file.
- Errors: analysis errors carry source/line/col from form metadata; runtime
  errors wrap with a Clojure-frame stack à la Glojure's `RTEvalError`.
- `try/catch`: on error, match catches by Go type of the error against the
  resolved catch type (design 05 registry; `Throwable`-analogue = `error`);
  finally always runs, value discarded.

---

## 7. The REPL is the evaluator — fidelity requirements

Consequences of the design above, pinned here as acceptance criteria — a
REPL that "mostly works" is worthless.

### 7a. Semantics the REPL depends on (must never regress)

- **Var redefinition.** Symbol resolution compiles to `OpVar{*Var}` — a
  pointer, **never** an inlined value (§3b); deref happens at call time. So
  re-`def f` and every existing caller — closures made before the re-def,
  even AOT-compiled fns linked against the var — sees the new root at once.
  Direct-linking-style optimizations are forbidden in the evaluator and
  default-off in the emitter.
- **defmacro at the REPL.** `setMacro` flips a live var's flag (§4); the
  analyzer consults it per form, caching nothing per-symbol — a macro
  (re)defined at the prompt takes effect on the very next form.
- **`in-ns` / on-the-fly namespaces.** `in-ns` creates-if-absent and rebinds
  `*ns*` (dynamic var, §3b); the analyzer reads `*ns*` per form. `ns` is a
  core macro over `in-ns` + `refer` + `require`.
- **`require` / `load`.** A load-path list (classpath equivalent, design 06)
  maps `foo.bar` → `foo/bar.clj` (munged). `load` = read + eval
  form-by-form with `*ns*`/`*file*` bound; `require` adds `*loaded-libs*`
  once-only tracking and `:as`/`:refer` — built in core.clj over the `load`
  primitive, as in Clojure. Prior art: Glojure `pkg/runtime/nsloaders.go`.

### 7b. REPL affordances — where they live

- `*1 *2 *3 *e` — plain **dynamic vars in core**, not REPL magic: the REPL
  driver rebinds them after each eval (results shift; `*e` on error).
- `*ns*`, `*file*`, `*print-length*` … — dynamic vars bound per session via
  `push/popThreadBindings` (one goroutine per session).
- Result printing = `pr-str` (core, over design 01's print protocol) — REPL
  output, program `pr-str`, and emitted-binary printing are one code path.
- The **REPL driver** is one small package — `Read → Analyze → Eval → bind
  *1/*e → print` over injected reader/writer; terminal and nREPL are two
  frontends of it.

### 7c. nREPL endgame (plan, not protocol design)

Editors (CIDER/Calva) connect via nREPL. Glojure ships a self-contained one
— `refs/glojure/pkg/nrepl/`: `bencode.go` (227 l.), `server.go` (221 l.),
`ops.go` (471 l.: eval/clone/describe/load-file), no heavy deps.
**Assessment: reuse the bencode codec and server/session skeleton nearly
as-is; rewrite `ops.go`'s eval glue against our REPL driver** so
`*1`/`*e`/session bindings come free. Milestone v5; zero analyzer/evaluator
changes required — which is itself the test of §7b's abstraction.

### 7d. Dual-mode consistency — the unforgivable failure mode

The same `*ast.Node` drives both the evaluator (REPL/dev) and the Go emitter
(AOT/prod). **Divergence between REPL behavior and compiled-binary behavior
is the one unforgivable failure mode** — it silently invalidates everything
developed at the REPL. The discipline:

1. **One analyzer.** The emitter never re-analyzes and has no private
   special-form knowledge; any new Op lands in *both* consumers before merge
   (exhaustive `switch n.Op` with `default: panic` keeps misses loud).
2. **Shared conformance suite, two harnesses.** Every semantic test is a
   `.clj` file + expected output run twice in CI: tree-walk eval, and
   AOT-compile + execute. A test that can't run both ways needs a written
   waiver in the file.
3. **Macros expand identically by construction** — both paths use the same
   analyzer, which uses the same evaluator for expansion (§4).
4. **Shared runtime.** Collections, numerics, printing, vars/namespaces are
   one package linked by both the interpreter and emitted binaries — drift
   can't hide in a second implementation.

---

## 8. Milestone plan

- **v0 — REPL evals arithmetic and functions.**
  Nodes: Const, Vector/Map/Set, Var, Local, Do, If, Def, Let, Binding, Fn,
  FnMethod, Invoke, Quote. Runtime: Symbol/Keyword/lists from design 01/02,
  one `user` namespace, Var intern/deref, Scope, evalFn (multi-arity +
  variadic), `+ - * / = < >` etc. as pre-interned native Go IFns. REPL driver
  with `pr-str` printing; re-def visible to existing callers from day one.
  Exit test: `(def fact (fn* fact [n] (if (< n 2) 1 (* n (fact (- n 1)))))) (fact 10)` → `3628800`;
  then re-def `fact` and a captured reference picks up the new version.
- **v1 — loop/recur + vars.** OpLoop/OpRecur (analysis-time tail+arity
  checks), recurSignal, `var`, dynamic vars + push/popThreadBindings, `set!`,
  full multi-namespace resolve (`in-ns`, aliases, refer), `*1 *2 *3 *e` in
  the REPL driver.
  Exit: iterative fact via `loop*`/`recur` with constant Go stack at n=100000;
  `(in-ns 'scratch)` + def + qualified call-back works at the prompt.
- **v2 — macros.** macroexpand1 in analyzeSeq, `setMacro`, hidden
  `&form`/`&env` args, bootstrap `defmacro`; start loading a minimal
  `core.clj` (`defn`, `let` w/ destructuring, `when`, `and`, `or`, `->`).
  Exit: `(defmacro unless [t e] (list 'if t nil e))` typed at the REPL is
  usable on the next form.
- **v3 — the rest of the specials + load.** letfn*, throw/try/catch/finally,
  new/`.` against the Go host registry, interop sugar in macroexpand1;
  `load`/`require` over load paths (§7a).
  Exit: `(require 'my.lib)` from disk; run unmodified early chunks of
  clojure.core.
- **v4 — emitter joins.** No analyzer changes by design; acceptance test is
  literally "the emitter consumes v0–v3 ASTs with zero re-analysis". The §7d
  dual-harness conformance suite gates every merge from here on. Add `case*`
  only when core's `case` lands.
- **v5 — nREPL.** Port Glojure's `pkg/nrepl` bencode/server onto our REPL
  driver (§7c). Exit: CIDER/Calva connect, eval, load-file, and see `*1`.

---

## 9. Key decisions (summary)

1. Glojure-style AST: uniform `*Node{Op, Form, Sub}` + typed per-op structs —
   cljs.analyzer's vocabulary, Go's type safety; analyzer is sole writer.
2. Analyzer is pure + dependency-injected (`Macroexpand1`, var hooks) — no
   import cycle; the runtime wires analyze↔eval; the AOT compiler links the
   same evaluator to run macros at compile time.
3. Two envs: immutable analysis env (locals/context/recur-frame, resolution
   done **once** at analysis) vs mutable runtime scope chain (evaluator-only;
   the emitter maps locals to Go vars instead).
4. `def` interns at analysis time; vars are the mutable indirection layer;
   locals always shadow vars.
5. recur = analysis-checked (tail position, arity) + LoopID-tagged sentinel
   error caught by the owning loop/fn-method as a plain Go loop.
6. The evaluator is the REPL engine: symbol → `OpVar` pointer (never
   inlined) so re-def is live; `*1/*2/*3/*e` and printing live in a shared
   REPL driver (Glojure `pkg/nrepl` reused, ops rewritten onto the driver).
7. Dual-mode consistency is enforced, not hoped for: one analyzer, one
   shared runtime, and a `.clj` conformance suite CI runs through BOTH the
   evaluator and the emitted-Go path — REPL/binary divergence is a release
   blocker (§7d).
