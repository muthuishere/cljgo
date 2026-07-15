# 09 — Competitive gap analysis: what the other Clojure-on-a-host impls have

Status: draft v1 (2026-07-15). Read after design/00 (identity/priorities) and
design/08 (build.cljgo + comptime + suite compliance). This dossier surveys the
sibling implementations under `../references/` and the jank suite at
`../clojure-test-suite/`, catalogues what they ship that cljgo does not, and
judges each gap against cljgo's five owner-mandated priorities plus the
Clojure-first precedence rule. It feeds the roadmap; it is not a spec.

Fit verdicts are tagged **ADOPT** / **ADAPT** / **DEFER** / **REJECT** and are
opinionated on purpose. Citations point at the reference file/dir the claim
came from (relative paths, per the standing rule — no absolute paths).

---

## 1. The landscape (one paragraph)

Four hosts, four architectures. **glojure** (`../references/glojure`) is a
pure tree-walk interpreter over a vendored `pkg/lang` of persistent structures,
with Go interop done by **reflection through a generated `pkgmap` registry**
(`../references/glojure/pkg/pkgmap/pkgmap.go`) on *both* the live and the
"compiled" path — cljgo's own `pkg/lang` is derived from this codebase, then
reshaped. **let-go** (`../references/let-go`) is the outlier and the strongest
technical donor: a **bytecode compiler + stack VM** (`pkg/bytecode`, `pkg/vm`,
`pkg/compiler`) with a mature numeric tower, transducers, chunked seqs,
transients, a global/custom multimethod hierarchy, real reify, an IR
"lower-go" pass that emits direct Go calls (`pkg/rt/native_direct.go`), AOT to
standalone binaries, and a WASM target — and it **passes the jank
clojure-test-suite**, the same suite cljgo just adopted as its yardstick (ADR
0022). **cljs2go** (`../references/cljs2go`) is a ClojureScript→Go emitter
overlay (deftype→Go struct, defprotocol→Go **interface**, everything a
`float64`) — historically interesting, architecturally a cautionary tale
(IIFE-heavy, no integers). **upstream clojure** (`../references/clojure`) is the
JVM semantic reference — the faithfulness bar, not an implementation to copy.
**cljgo sits alone in the dual quadrant**: one reader → one analyzer → one AST
with two consumers — a fast tree-walk evaluator (the REPL + macro engine) *and*
an AOT emitter that produces plain Go source (the ClojureScript model, Go as
JS) — linking one shared `pkg/lang` runtime, with dual-mode conformance
(interpreted result == compiled result) as the release gate (ADR 0002). That
gate is the lens every verdict below is judged through.

---

## 2. Gap table

*cljgo status* is measured against the current tree (`pkg/lang/*.go`,
`core/core.clj` — 991 lines, no transducers/transients/reify/derive yet) and the
design/08 backlog (suite baseline ~43%, ~101/235 vars). "partial" = Go-level
type or hook exists but core-fn wiring / fidelity is incomplete.

| Capability | Who has it | cljgo status | Fit verdict | Priority |
|---|---|---|---|---|
| **Numeric tower** — ratios, bigint, bigdec, int64-overflow→promotion, `bit-*`, `quot/rem/mod` edges | let-go (full: `pkg/vm/{ratio,bigint,bigdecimal,numbers_promote}.go`, `pkg/rt/math.go`); glojure (`pkg/lang/{bigint,ratio}.go`); upstream | **partial** — Go types present (`pkg/lang/{bigint,bigdecimal,ratio,numberops}.go`) but promotion + core wiring thin | **ADOPT** (design/08 §5 Batch 2) | **P1-high** |
| **Transients** — `transient`/`persistent!`/`conj!`/`assoc!`/`dissoc!`/`pop!` | let-go (`pkg/vm/transient.go`); upstream | **missing** — vendored PHM/PV lacks Clojure-shaped transients (known M0 gap, design/00 §5) | **ADOPT** (Batch 3) | **P4-high** |
| **Transducers** — `transduce`/`eduction`/`sequence`/`completing`, `reduced`, `volatile!` | let-go (`core.lg` + `pkg/vm/{reduced,volatile}.go`, `benchmark/transducers.clj`); upstream | **missing** — `reduced.go` exists, no `volatile!`/transduce in `core.clj` | **ADOPT** → new **ADR 0024** | **P3/P4-high** |
| **Chunked seqs** — chunk-aware `map`/`filter`/`take`/`range` | let-go (`pkg/vm/{chunk,chunked_seq}.go`); glojure (`chunkbuffer/chunkedcons/slicechunk`) | **partial** — chunk types vendored (`pkg/lang/{chunkbuffer,chunkedcons,slicechunk}.go`) but `core.clj` seq fns not chunk-aware | **ADOPT** (design/08 seq completeness) | **P4-med** |
| **Protocols / deftype / defrecord fidelity** | let-go (registry: `pkg/vm/protocol.go`, `deftype.go`); cljs2go (Go interface); upstream | **have (v0)** — ADR 0020 shared registry (`pkg/lang/instance.go`), byte-matched vs Clojure 1.12.5 | **ADAPT** — keep registry, add Go-interface emission only as a post-v0 *optimization* (see §5) | **P3-done/med** |
| **`reify`** | let-go (`test/reify_test.lg`, `core.lg`); upstream | **missing** — explicitly deferred in ADR 0020 v0 scope | **ADOPT** (M5, rides the ADR 0020 registry) | **P3-med** |
| **`letfn`** | let-go (`test/letfn_test.lg`); upstream | **missing** — scheduled M5 (`letfn*`, design/00 §6) | **ADOPT** (already M5) | **P3-med** |
| **`proxy`** | upstream (JVM class gen); *not* in let-go/glojure meaningfully | **missing** | **REJECT** — proxy is JVM concrete-class subclassing; no Go analogue, reify+protocols cover the need | **P3-none** |
| **Multimethod hierarchies** — `derive`/`isa?`/`ancestors`/`descendants`/`make-hierarchy`, global + custom | let-go (full: `pkg/rt/hierarchy.go`); upstream | **partial** — `defmulti` is flat `=` dispatch (design/08 Batch 4); suite has `make_hierarchy`/`underive` files | **ADOPT** (Batch 4; upgrades multifn past flat) | **P3-med** |
| **Metadata** — `with-meta`/`vary-meta`, meta on IObj | let-go (`core.lg` `vary-meta`, `test/metadata_test.lg`); upstream | **partial** — `with-meta` on IObj, `vary-meta` missing (design/08 Batch 5) | **ADOPT** (Batch 5) | **P3-med** |
| **Regex** | all (Go `regexp` RE2 in the Go-hosted ones; Java regex upstream) | **have** — `pkg/lang/regexp*.go` + cache | **REJECT** copying Java-regex semantics — RE2 is the honest, documented deviation (design/00 §1); note suite files needing lookaround/backrefs will legitimately fail | **P3-done** |
| **`pr`/`print` fidelity** — exact `pr-str` forms, `format`, `*-str` helpers | let-go (`pkg/rt/{pprint,print_method}.go`); upstream | **partial** — suite hammers exact forms (design/08 Batch 5) | **ADOPT** (Batch 5) | **P3-med** |
| **STM** — `ref`/`dosync`/`alter`/`commute`/`ensure` | *nobody real* — let-go **fakes** it (`core.lg`: `ref`=atom, `dosync`=`do`, `commute`=`alter`, `ensure`=`deref`) | **rejected** (design/05 §4) | **REJECT** as real STM; **ADAPT** = ship let-go's single-thread stubs so suite files load | **P3-low** |
| **`future`/`promise`/`delay`** | let-go (`core.lg`, `pkg/vm/delay.go`); glojure (`agent.go`); upstream | **partial** — `delay.go` present; future/promise are the M4+ reference types | **ADOPT** (post-M4, real goroutines) | **P4-med** |
| **`agent`** | let-go (`core.lg`, `restart-agent`/`shutdown-agents`); glojure (`pkg/lang/agent.go`) | **partial** — `pkg/lang/agent.go` vendored; post-M5 in design/00 | **DEFER** (agents/mult/pub/pipe, post-M5) | **P4-low** |
| **core.async-style channels** | let-go (`pkg/rt/async.lg`, buffer policies); cljgo | **have (design) — differentiator** — real goroutine chans (`pkg/lang/{chan,chan_alts}.go`), `alt!`→`select` emission (design/05, M4) | **keep** — cljgo's real-goroutine model beats let-go's VM-scheduled channels | **P4-strength** |
| **`ex-info`/error fidelity** — `ex-data`, exception class, `Throwable->map` | let-go (`pkg/vm/{exinfo,exception_class}.go`); upstream | **partial** — `pkg/lang/{exception_info,error,catch}.go` present; Go-error mapping settled (ADR 0005) | **ADOPT** (fidelity polish, Batch 5-adjacent) | **P3-med** |
| **ns / `require` / load model** | all | **have** — `in-ns`/aliases/refer/`require`/`load` (M1/M5) | — | **done** |
| **tail-calls / `recur` edges** | all | **have** — analysis-time tail+arity checks, constant-stack loop* (M1) | — | **done** |
| **Reader completeness** — namespaced maps, tagged literals, reader conditionals | let-go (`pkg/compiler/reader.go`, conditional-skip tests); upstream | **partial** — Phase 2 `.cljc` + `#?(:default)` (ADR 0022); namespaced maps / tagged literals post-M5 | **ADOPT** (suite needs `:default` in body + require; 233/235 files) | **P3-high** |
| **`clojure.spec`** | none of the Go hosts; upstream only | **missing** | **REJECT** (v0) / **DEFER** — huge surface, not suite-gated, no Go-host precedent; revisit only if a real user needs it | **P3-none** |
| **Interop: zero-binding, direct-call** | cljgo (emit direct from go/packages type facts); glojure/let-go use **reflection registries** | **have — differentiator** (ADR 0010, S2) | **keep**; treat glojure's reflect-in-emitted-code as an anti-pattern (§6) | **P1-strength** |
| **C FFI** — live `dlopen` + cgo | let-go (some cgo); none do purego `deflib` | **have (design)** — purego primary + cgo first-class (ADR 0011, S7) | **keep — differentiator** | **P5-strength** |
| **Bytecode VM perf model** | let-go (`pkg/bytecode`, `pkg/vm`, constant pool, `native_direct.go` lower-go) | **n/a (different model)** — cljgo is tree-walk + emit-Go | **ADAPT** ideas only: pre-resolved locals by index (already planned), constant interning, `native_direct`-style direct-call seeding for the emitter; **REJECT** adopting a bytecode VM (conflicts with emit-Go) | **P4-med** |
| **AOT → standalone binary** | let-go (`pkg/bundle`, single ~12MB binary); cljgo | **have** — emits Go, `go build`; stripped 4.4MB (ADR 0023) | **keep**; AOT-core.clj is the size prize (ADR 0023 / M5) | **P4-strength** |
| **Binary size / startup** | let-go (12MB tool, 8ms start); cljgo (10.4MB tool, 4.4MB bin) | **partial** — AOT-core.clj cuts interpreter from binaries (ADR 0023 proposed) | **ADOPT** (already ADR 0023 / M5) | **P4-med** |
| **WASM target** | let-go (`wasm/`, `pkg/rt/*_js_wasm.go`); cljs2go (JS→Go legacy) | **missing** — GOOS=js path noted (design/00 §6 post-M5) | **DEFER** — free-ish via emit-Go + GOOS=js/wasip1; low priority vs interop | **P4-low** |
| **Clojure→Go source metaprogramming** | let-go `gogen` (`pkg/rt/gogen.go`: Clojure builds `go/ast` nodes) | **partial/overlap** — cljgo *is* an emit-Go compiler; `comptime-step` can emit `.go` (design/08 §1) | **ADAPT** — fold the useful bit (comptime that emits a `.go`/`.cljg` asset) into `comptime-step`; don't build a separate `gogen` DSL | **P4-low** |
| **Bundled batteries** — http/json/transit/edn/pods builtins | let-go (`pkg/rt/{http,json,transit,pods}.go`); glojure stdlib | **missing (by design)** — the Go ecosystem is the stdlib (ADR 0010) | **REJECT** bundling http/json/transit; **ADOPT** only `edn` (suite has `edn_test`) + keep `clojure.string` (`core/string.cljg`) | **P1-med (edn only)** |
| **Zig-style build system** | *nobody* (`deps.edn`/lein elsewhere) | **have (design) — differentiator** — `build.cljgo` (ADR 0021) | **keep** | **strength** |
| **`comptime` value-embed** | *nobody* (Zig-only idea) | **have (design) — differentiator** — ADR 0009 | **keep** | **strength** |
| **Test-suite ratchet in CI** | let-go (bench-ratchet + passes jank suite) | **partial** — adopted (ADR 0022), harness/baseline pending (design/08 T1) | **ADOPT** (do first, design/08 sequencing) | **P3-high** |

---

## 3. Fit verdicts (rationale per gap)

**ADOPT — do it, maps to an existing batch/milestone.**
- **Numeric tower** (design/08 Batch 2). The single biggest faithfulness gap vs
  let-go and the area the suite "hammers hardest" (ADR 0022). Go types already
  vendored; the work is promotion (`inc'`/`+'` on int64 overflow →
  `pkg/lang/bigint.go`), ratio arithmetic wiring, `bit-*` completeness, and
  parse fns. Serves priority 3 (faithfulness) and the ADR 0022 metric directly.
- **Transients** (Batch 3). Structural — the vendored PHM/PV never grew
  Clojure-shaped transients. Unlocks the `persistent!`/`transient` suite files
  *and* is the substrate for fast `into`/`reduce`/transducers. let-go's
  `pkg/vm/transient.go` is the reference shape. Priorities 3 + 4.
- **Transducers** (new ADR 0024). Needs `volatile!` + `reduced` wiring first
  (`reduced.go` exists; `volatile!` missing). let-go's `core.lg` transduce/
  eduction/sequence/completing is a faithful, portable reference. Must run
  **dual-mode** (interpreted == emitted) — the reducing-fn machinery is plain
  Clojure over `pkg/lang`, so it does. Priorities 3 + 4.
- **Chunked-seq awareness** in core seq fns (Batch-adjacent). Chunk types exist;
  making `map`/`filter`/`take`/`range` chunk-aware is a perf-faithfulness win
  (priority 4) and matches upstream behavior tests.
- **Multimethod hierarchies** (Batch 4). `derive`/`isa?`/`ancestors` + a global
  hierarchy upgrades the flat v0 multifn; let-go's `pkg/rt/hierarchy.go` is a
  clean, host-neutral port. Suite has `make_hierarchy`/`underive`/etc.
- **`vary-meta` / print fidelity / ex-info fidelity** (Batch 5). Cheap breadth,
  high suite yield. Priority 3.
- **`reify` + `letfn`** (M5). reify rides the ADR 0020 registry with zero new
  dispatch mechanism (dual-mode-safe); letfn is already M5-scheduled.
- **Reader Phase-2 fidelity** — verify `:default` elision in body *and* require
  conditionals (233/235 suite files depend on it, design/08 T1). Gate-critical.
- **`edn` read/write** — small, suite has `edn_test`; the one "battery" worth
  shipping as Clojure over `pkg/lang`.
- **Suite harness + baseline %** (design/08 T1) — do first; it yields the metric
  that directs everything else.
- **AOT-core.clj / binary size** (ADR 0023, M5) — already decided; the single
  biggest lever for both binary size and REPL-boot cost.

**ADAPT — good idea, reshape for emit-Go / dual-mode / Clojure-first.**
- **Protocols as Go interfaces** (cljs2go/let-go partly). cljgo already chose the
  *shared registry* (ADR 0020) precisely because two dispatch mechanisms
  (interface for AOT, registry for REPL) is the divergence risk. **Keep the
  registry as the semantic truth; emit Go interfaces only as a later,
  behavior-preserving perf optimization**, never as the primary path. Notably,
  **let-go's own `pkg/vm/protocol.go` is also a type→method registry, not a Go
  interface** — independent corroboration that the registry is the right rung.
- **let-go's bytecode-VM perf ideas.** Don't adopt the VM (it contradicts
  emit-Go and the `any` value model — design/00 §5). Do adapt: (a)
  pre-resolved locals by index (already in design/00 §1); (b) constant/keyword
  interning (already on `unique.Handle`); (c) `native_direct.go`'s "seed a
  registry so the lowerer emits a direct Go call instead of a var-lookup
  trampoline" — a direct analogue of cljgo's emitter already calling directly,
  worth mirroring for core-fn call sites in the interpreter's hot path.
- **STM stubs.** Ship let-go's single-thread `ref`/`dosync`/`alter`/`ensure`
  stubs (`ref`=atom, `dosync`=`do`) so suite/library files *load*, while being
  honest that there is no coordinated STM (design/05 §4). Adapt, don't fake
  concurrency guarantees.
- **`gogen`.** cljgo *is* an emit-Go compiler, so a separate Clojure→`go/ast`
  DSL is redundant — but the useful capability (comptime that produces a `.go`
  or `.cljg` asset before emission) folds into `comptime-step` (design/08 §1).

**DEFER — fits, low priority.**
- **agents / mult / pub / pipe** (post-M5, design/00). Real but not suite-central.
- **WASM target.** Nearly free via emit-Go + `GOOS=js/wasip1`; sequence after
  interop and the suite number. let-go's `wasm/` + `*_js_wasm.go` is the map.

**REJECT — conflicts with the model / priorities.**
- **`proxy`** — JVM concrete-class subclassing; no Go analogue; reify+protocols
  cover it. (Priority 3 doesn't require host-class emulation.)
- **`clojure.spec`** — enormous surface, not suite-gated, no Go-host precedent.
  Revisit only on concrete demand.
- **Java-regex semantics** — RE2 is the deliberate, documented deviation; do not
  chase lookaround/backreferences (design/00 §1).
- **Bundled http/json/transit/pods batteries** (let-go/glojure) — the Go
  ecosystem *is* the standard library (ADR 0010). Bundling them re-creates
  Joker's frozen-stdlib trap and undercuts priority 1. (edn + clojure.string are
  the exceptions — pure Clojure, suite-tested.)
- **A bytecode VM** — see ADAPT; wrong architecture for the emit-Go north star.

---

## 4. Top-10 recommended additions (ranked)

1. **Suite harness + baseline % + reader `:default` verification** — design/08 T1
   (Batch 0). Yields the north-star metric; unblocks the ratchet. Prereq for
   everything measurable.
2. **Numeric tower** — design/08 Batch 2. Biggest faithfulness gap vs let-go;
   the suite's hardest-hit area. Types exist; wire promotion + ratios + `bit-*`.
3. **Transients** — design/08 Batch 3 (**new OpenSpec on PHM/PV transients**).
   Structural substrate; unlocks `into`/transducers perf and suite files.
4. **Transducers + `volatile!`** — **new ADR 0024** (`/opsx:propose transducers`).
   Depends on transients + reduced/volatile; ports cleanly from let-go `core.lg`,
   dual-mode by construction.
5. **Multimethod hierarchies (`derive`/`isa?`)** — design/08 Batch 4. Upgrades
   flat multifn; port `pkg/rt/hierarchy.go`.
6. **`vary-meta` + `pr-str`/`format` fidelity + `ex-data`** — design/08 Batch 5.
   Cheap breadth, high suite yield.
7. **Chunked-seq-aware core seq fns** — design/08 seq-completeness. Perf +
   behavior fidelity; chunk types already vendored.
8. **`reify` + `letfn`** — M5. reify on the ADR 0020 registry (no new dispatch);
   letfn already scheduled.
9. **AOT-core.clj (cut `main → eval.New`)** — ADR 0023 / M5. Binary size to ~2MB
   and REPL-boot fix in one move; the emit-Go story's capstone.
10. **`edn` read/write + Batch-1 predicate/coercion breadth** — design/08 Batch 1.
    Highest count-per-effort; many one-liners turn suite files green.

---

## 5. Anti-patterns to avoid (what the others do that cljgo must not copy)

- **Two dispatch mechanisms for one abstraction** (the emit-a-Go-interface-for-
  AOT-but-registry-for-REPL temptation, cljs2go-style). This is the exact
  REPL-vs-binary divergence ADR 0020 was written to prevent. One semantic
  mechanism (the shared registry); Go-interface emission only as a proven-
  equivalent optimization. **This is the single most important anti-pattern** —
  every ADOPT above must land in analyzer + eval + emit together (ADR 0002) or
  it breaks the release gate.
- **Reflection as the *emitted-code* calling convention** (glojure's
  `pkgmap.go` on both paths). cljgo emits direct, signature-coerced Go calls;
  the reflect registry is interpreter-only and must never leak into binaries
  (design/00 §5, ADR 0010). Copying glojure here would forfeit priority 1 and
  bloat every binary.
- **Bundling a frozen host stdlib** (Joker's mistake; let-go's http/json/transit
  builtins). It competes with — and freezes against — the live Go ecosystem
  that is supposed to *be* the stdlib. Reach modules through interop; keep only
  pure-Clojure, suite-tested namespaces (edn, clojure.string).
- **Renaming Clojure to fit the host.** cljs2go made everything a `float64` (no
  integers) and mangled names for Go's export rules; let-go's `dosync`=`do` is a
  benign stub but the category is dangerous. Clojure is first-class — additions
  (comptime, build.cljgo, RE2) rename *around* Clojure and are documented as
  deviations, never silently approximated (design/00 §1, precedence rule).
- **eval-then-serialize-the-namespace compilation** (glojure's model, already
  rejected in design/04 §0 / design/00 §5) and **IIFE expression emission**
  (cljs2go, design/04 §3) — both reappear as tempting shortcuts; both break the
  "compile forms, never runtime values" contract and readable emitted Go.
