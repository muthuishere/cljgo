## Context

ADR 0050 is the authority; this is its implementation design. Its dependencies
have both landed: ADR 0049 (never silent `nil`) and ADR 0048 §6 (purity walk).
The producer side of ADR 0013 is unbuilt except `exe`. Spikes S29 (transitive
purity/taint classifier) and S30 (`certain-java?`) are closed, MET, and read-only
against `pkg/` — their `driver.go`/`proto/main.go` are re-authored here, not
copied (ADR 0027).

Verified reuse points (Explore pass, at `d522d8a`):
- **`emit.CompileProgram(srcPath) (*Program, error)`** (`pkg/emit/module.go:83`)
  already performs the transitive-require traversal (dependency-first via
  `moduleCompiler.load`, `module.go:49-78`) and returns
  `Program{Entry *CompiledNS, Deps []*CompiledNS}` (`module.go:33-36`). Each
  `CompiledNS{Name, Path, Forms []*ast.Node, Requires}` (`module.go:24-29`)
  carries the analyzed forms. This IS the `map[ns]→forms` the ADR's classifier
  runs over — **no new walk**.
- **The five host ops** `OpHostRef/OpHostCall/OpHostMethod/OpHostField/OpHostNew`
  (`pkg/ast/ast.go:47-51`); the canonical child-walker `eachChild`
  (`pkg/emit/emit.go:313`, unexported — so the classifier lives IN `package emit`
  to use it directly, rather than S29's copied `eachChildS29`).
- **No `publish` command** (`cmd/cljgo/main.go:52-92`); `Artifact{Name, Main,
  Kind}` (`build.go:55-58`) — `Kind` is where `"go"`/`"clojars"` attach. `(exe b
  spec)` (`core/build.cljg:30-37`) is the verb pattern a `publish`/lib target
  mirrors.
- **Go source emission** today: `WriteProgram` (`module.go:124`) emits one Go
  package per namespace with real `go/packages`-resolved signatures
  (`hostfacts.go:87`), in `package main` executable shape — `publish go` is a new
  *library* emission mode reusing that per-ns layout + signature machinery.
- **`publish clojars` source set** = `Program.Entry.Path` + every
  `Program.Deps[i].Path` — the exact transitive `.cljg`/`.clj` files to copy.
- **Java statics already hard-error at analysis** with `file:line`:
  `(System/…)`/`(Math/…)`/`java.*` → `no such namespace` (`pkg/corelib/resolve.go:24`);
  `import`/`new` → `unable to resolve symbol`. The undecidable bare dot-form
  `(.method obj)` consults no resolver (`analyzer.go:985-986`) — `certain-java?`
  must never flag it.
- **`pkg/deps/manifest.go:6` already cites ADR 0050** as the publish-time emitter
  of the impurity manifest `pkg/deps` reads at resolve time — the two sides meet
  there.

## Goals / Non-Goals

**Goals:**
- A per-namespace Go-interop taint classifier in `pkg/emit` (re-authored from
  S29), one pass over `CompileProgram`'s capture, exposing the whole-library OR
  gate and the per-namespace lookup, with `file:line`.
- A `certain-java?` courtesy predicate (re-authored from S30), certain-only,
  zero-FP, never a gate.
- `cljgo publish go` and `cljgo publish clojars` producers, gated by the
  classifier, with a `publish`/lib verb in `build.cljgo`.
- Decision 4: a Java-tainted namespace fails loud + per-namespace (mostly already
  true via analyzer errors + ADR 0049 — verify and close any gap), never `nil`.

**Non-Goals:**
- Consuming Maven/Clojars libraries (deferred import — ADR 0050 out of scope).
- A JVM bytecode backend (explicitly never).
- `c-shared`/`c-archive` producers (ADR 0013's later work).
- A *total* `uses-java?` predicate (S30: not achievable, not needed — the gate is
  `uses-go-interop?`).
- Clojars *distribution* mechanics beyond a git-coordinate `deps.edn` (a
  source-jar/coordinate step is a scoping item, git-coord first).

## Decisions

1. **Classifier in `package emit`** — new `pkg/emit/purity.go`:
   `type Taint struct { Class string; NS, Path string; Line int; Detail string }`
   and `func ClassifyGoInterop(p *Program) map[string]*Taint` (per-ns, keyed by
   NS name; entry recovered textually since `Entry.Name==""`). It walks each
   `CompiledNS.Forms` via `eachChild`, switching on the five `OpHost*` ops,
   recording the first offending `file:line`. A pluggable
   `type Predicate func(*CompiledNS) *Taint` slot is reserved (S29 proved N
   predicates compose) so `ffi`/`c-link` taint can be added without touching the
   traversal. Whole-library gate = OR over the map; per-namespace = lookup.

2. **`certain-java?` in `pkg/publish`** (re-authored from S30 `javaSyntactic`),
   working on reader forms: flags `import`/`new` heads, `java.*`/`javax.*` and
   `clojure.java.*` in **call-namespace** position, and the bare-JVM-class table
   (`System Math Thread Integer …`) in call-ns position — and **nothing else**
   (never bare dot-forms, `instance?`, `catch`, or class-ref values). Zero-FP by
   construction. It is a diagnostic, never a gate.

3. **`pkg/publish` orchestration + producers**:
   - `publish clojars`: `CompileProgram` → `ClassifyGoInterop` → if any NS tainted,
     **fail naming the `file:line`**; else copy every `CompiledNS.Path` into a
     source-tree layout + write a `deps.edn` git-coordinate stub. Java is allowed.
   - `publish go`: `CompileProgram` → validate the exported surface is
     Go-expressible (reuse `hostfacts.go` signature resolution; fail `file:line`
     on an inexpressible export) → emit a go-gettable **library** package
     (per-ns layout from `WriteProgram`, but library-shaped: named packages,
     exported wrappers, a `go.mod` with the library module path, no `main()`).
     Go-interop is *allowed* here.

4. **CLI + surface**: `cmd/cljgo/main.go` gains `case "publish": runPublish`;
   `core/build.cljg` gains a target verb (mirroring `exe`) that stamps
   `Artifact.Kind` (`"go"`/`"clojars"`) — with its AOT mirror regenerated via
   `go generate` (parity by construction, as ADR 0048 did). A library needs a
   **module path** and optionally an **exports** list — add the minimal plan
   fields (`Artifact` gains what's needed; keep it small).

5. **Decision 4 (loud per-namespace Java failure)**: verify the existing
   analyzer errors (`no such namespace`/`unable to resolve symbol`) already fire
   with `file:line` and never `nil` (S30 measured they do); add a conformance
   case pinning it, and the optional strict resolve-time rejection hook in
   `pkg/deps` (a manifest `Java` taint → default-deny), reusing the ADR 0048
   `checkPurity` shape. No new interpreter divergence surface (ADR 0049 covers Go;
   this extends the same guarantee to Java, which already holds for statics).

## Risks / Trade-offs

- **`publish go` library-emission scope** → the classifier + clojars are fully
  spiked (S29); the go *library* mode is new emitter work. Mitigation: reuse
  `WriteProgram`/`hostfacts` per-ns layout + signatures; scope the first cut to a
  correct go-gettable package for pure + go-interop libraries, and be explicit in
  the report about which export shapes are supported vs deferred. Do not
  gold-plate signature inference beyond what `hostfacts.go` already resolves.
- **Entry NS name is `""`** → recover it textually (S29 `readNSName`) when keying
  the taint map so the entry is addressable in gate output.
- **certain-java? at reader vs AST level** → S30 ran it on reader forms; keep it
  reader-level and self-contained so it needs no analyzer, matching S30's zero-FP
  measurement. It only ever *upgrades* an error message; correctness never
  depends on it.
- **No gold-plated clojars coordinate** → git-coord `deps.edn` first; a
  Clojars-coordinate/source-jar step is explicitly deferred (ADR 0050 scoping).

## Open Questions

- Exact `build.cljgo` publish verb surface (one `(publish b {:target …})` vs
  per-target `(lib …)`/`(go-lib …)`) — resolve during task authoring; keep it one
  verb stamping `:kind`, code not a second manifest.
- Library module path source (a `:module` on the artifact vs `Options.ModuleName`)
  — minimal field, decided in task 4.
