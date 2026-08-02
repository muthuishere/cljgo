# Design — multi-namespace emission (AOT-core piece 1)

Everything semantic below was verified against real Clojure 1.12.5
(`clojure` CLI, 2026-07-17; see ADR 0042 for the oracle transcripts): deps
load **at the require site**, interleaved with the requiring file's side
effects, exactly once; macros defined in one ns expand fine when used from
another (expansion is analysis-time); a require cycle throws
`Cyclic load dependency: [ /cyc/a ]->/cyc/b->[ /cyc/a ]`.

## 1. Package layout

One Clojure namespace → one Go package under the generated module
(design/04 §1, `nsToPath` munging: `.`→`/`, `-`→`_` per segment):

```
gen/
  go.mod                      module cljgo.gen/main (unchanged synth rules)
  main.go                     package main — the ENTRY namespace's forms +
                              bootstrap main() (unchanged single-file shape)
  multi/util/util.go          package util  — ns multi.util
  multi/data/data.go          package data  — ns multi.data
```

- Import path: `<ModuleName>/<munged ns dirs>`; package name = munged last
  segment (suffixed `_pkg` if it collides with a Go keyword).
- Two namespaces munging to the same directory is a compile error (lossy
  munge; explicit beats silent overwrite).
- The **entry** namespace stays in `package main`. Rationale: it keeps the
  single-file path and the dual-harness `PrintLastValue` contract untouched,
  and `main` only genuinely shrinks in piece 3 (AOT-core cutover), when the
  bootstrap stops constructing the interpreter. Follow-up noted in §7.

## 2. Load() chaining — registry-triggered, exactly once, cycle-free

The design/04 §1 sketch (dep `Load()` calls at the top of the requiring
`Load()`) is **rejected**: it reorders load-time side effects relative to
the interpreter (oracle: `before` / `loading multi.data` / `after`), which
is REPL-vs-binary divergence (ADR 0002/0007 release blocker).

Instead (ADR 0042 §2), a dependency package emits:

```go
package util

import (
    rt "github.com/muthuishere/cljgo/pkg/emit/rt"
    lang "github.com/muthuishere/cljgo/pkg/lang"
    _ "cljgo.gen/main/multi/data"   // linker edge for util's own require
)

var ( /* hoisted interns, per package */ )

var loaded = false

func init() { rt.RegisterLib("multi.util", Load) }

func Load() {
    if loaded { return }
    loaded = true
    lang.PushThreadBindings(lang.NewMap(
        lang.VarCurrentNS, lang.VarCurrentNS.Deref(),
        lang.VarFile, "…/multi/util.clj"))
    defer lang.PopThreadBindings()
    // top-level forms, source order — including the replayed
    // (in-ns 'multi.util) (refer 'clojure.core) (require '[multi.data :as d])
}
```

- `init()` only writes a map entry (`eval.RegisterLibProvider` via the `rt`
  re-export) — safe before `rt.Boot()`.
- The requiring package's **replayed `(require …)` form** is what triggers
  the load: `loadLib`, finding no such namespace, consults the provider
  registry and calls the registered `Load()` at exactly the source position
  where the interpreter loads the file. Side-effect order is byte-identical
  by construction; the `loaded` bool gives exactly-once (diamond deps load
  once, as oracled).
- The thread-binding push/pop around the body mirrors the interpreter's
  load frame (`repl.Driver.EvalReader` shape): the dep's `in-ns` must not
  leak into the requiring namespace's remaining forms, and `*file*` must
  read as the dep's source path in both modes.
- Blank imports (`_`) of each **file-backed require recorded at compile
  time** are what make the Go linker keep (and thus `init`-register) the
  dependency packages. Requires of embedded namespaces (clojure.string …)
  record no edge — they exist at boot.
- **Cycle detection is compile-time**: the module compiler keeps an
  in-progress stack; a require hitting an in-progress lib fails with
  `cyclic load dependency: multi.a -> multi.b -> multi.a`. Runtime replay
  re-runs the same require sequence, so it cannot cycle.

## 3. Cross-namespace var references at emit time

Registry-interned, not direct Go symbols (ADR 0042 §3). Package A referring
to `multi.util/offset` hoists the same package-level intern shape it uses
for its own vars:

```go
var v_multi_util_offset = lang.InternVarName(lang.NewSymbol("multi.util"), lang.NewSymbol("offset"))
```

Interning is global, idempotent, order-free (design/00 §4.4): A's package
init and B's `Load()` yield the **same `*lang.Var`**, whatever the Go init
order; the require replay guarantees B's `Load()` bound the root before any
A form reads it. Per-call cost is one atomic `v.Get()` — identical to an
intra-ns reference, because ADR 0004 mandates per-call deref for liveness
parity anyway. The "direct is the perf point" question therefore dissolves
for **references**: a direct Go symbol saves zero instructions per call and
breaks redefinition. Direct **calls** (bypassing the Var when the analyzer
proves the target) are design/04 §5 rung 1 and orthogonal.

## 4. Def ordering and compile model

Unchanged: within a namespace, top-level forms emit into `Load()` in source
order (def interns-then-binds as it runs). Compile time = eval time
(ADR 0002) now extends across files: when the module compiler's loader
compiles `multi/data.clj`, its `defmacro twice` is **evaluated** in the
shared compile-time evaluator, so `multi.util`'s later forms macroexpand
against it — cross-ns macros need no new machinery (oracled; proven by the
conformance test).

## 5. Source resolution (the no-classpath rule)

`require` of `x.y` with no such namespace resolves (ADR 0042 §4):

```
root  = dir(*file*)  minus the requiring ns's own directory suffix, if it
        matches (src/my_app/core.clj in ns my-app.core → src/); else dir(*file*)
cands = <root>/x/y.clj, <root>/x/y.cljg   (segments munged - → _)
```

Both consumers use the same resolver in `pkg/eval`:
- **Interpreter** (`Evaluator` default loader): read + `EvalForm` loop under
  a pushed `*ns*`/`*file*` frame; the namespace must exist afterwards.
- **Module compiler** (`pkg/emit.CompileProgram` sets `Evaluator.LibLoader`):
  same, but per form analyze → eval → **capture** into that lib's
  `CompiledNS{Name, Path, Forms, Requires}`. The requiring ns (top of the
  compile stack) records the edge for §2's blank imports.
- **`CompileFile`/`CompileReader`** (the single-file AOT API, used directly
  by the conformance harness and pkg/emit tests) install a **refusing**
  loader: a file-backed require under single-file compilation is an error
  naming `CompileProgram`, never a silently-broken binary.

At binary runtime the resolver is effectively dead: dep namespaces come from
the provider registry, embedded ones from boot. (If a require misses both,
the filesystem fallback still runs — matching the interpreter — and fails
with the existing "could not locate namespace" when nothing is found.)

## 6. Cross-package contract changes (every consumer)

| Contract | Consumers touched |
|---|---|
| `eval.requireSpec`/`loadLib` gain the `*Evaluator` receiver + seams | `require` builtin (only caller) |
| `eval.RegisterLibProvider` (new) | `pkg/emit/rt.RegisterLib` (new), emitted code |
| `emit.CompileProgram`/`WriteProgram` (new) | `emit.Build`, `pkg/build.buildArtifact`, `conformance/compiled_test.go` |
| `emit.EmitMain`, `emit.WriteModule`, `emit.CompileFile` — **unchanged signatures & single-file output** | pkg/emit tests, gomod/hostfacts/perf tests, conformance (via WriteProgram's zero-dep delegation) |

## 7. Follow-ups for pieces 2/3 (findings)

- **Piece 3 wants the entry ns as its own package too**: once core.clj is an
  emitted package, `main.go` should become pure bootstrap (`rt.Init()` —
  no interpreter — + blank imports + entry `Load()` + `-main`). The emitter
  core added here (`emitPackage` parameterized by package name / register /
  main-ness) already supports that split; it is a call-site change.
- **The registry seam is the cutover point**: `rt.Boot()` today loads
  embedded core via `eval.New()`. Piece 3 replaces that with the compiled
  core package registering/loading itself; `loadLib`'s provider lookup
  already covers `clojure.core`-as-provider with no further changes.
- **Piece 2 (builtins → pkg/lang)** is unaffected by this change, but note:
  emitted dep packages import only `rt` + `lang` (+ blank dep imports), so
  once builtins live in `pkg/lang`, dependency packages are already
  interpreter-free; only `package main`'s `rt.Boot()` keeps the edge.
- `*file*` in dep `Load()` is emitted as the compile-time resolved path
  string; interpreter and binary agree as long as the program is invoked
  with the same entry path — acceptable for the harness, worth revisiting
  if `*file*`-dependent programs surface.
