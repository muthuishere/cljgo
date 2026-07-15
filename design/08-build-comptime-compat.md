# 08 — The Zig model (build.cljgo + comptime) and clojure.core compliance

Status: draft v1 (2026-07-15). Owning ADRs: 0021 (build.cljgo), 0009 (comptime),
0022 (test-suite compliance), and 0011/0012/0013 (C FFI / testing / library
kinds, all surfaced *through* build.cljgo). This doc is the how; the ADRs are
the decisions. Supersedes design/05 §1's `deps.edn` for the AOT product.

The strategic shift (owner, 2026-07-15): cljgo's "batteries" are **Zig's**, not
Leiningen's/deps.edn's — a build system that is code, comptime as the
metaprogramming spine alongside Clojure macros, and an external compliance
target (the jank clojure-test-suite) as the yardstick. Three tracks below;
they are independent enough to build in parallel.

---

## 1. `build.cljgo` — build is a program (ADR 0021)

`build.cljgo` at the project root defines `(defn build [b] …)`. `cljgo build`
evaluates it (interpreted — comptime context), constructs a step DAG, executes
the requested steps. Sketch of the target surface (names settle in the OpenSpec):

```clojure
;; build.cljgo
(defn build [b]
  (let [opt (option b :optimize :enum {:default :fast})   ; -Doptimize=…
        app (exe b {:name "app"
                    :main "src/app/core.cljg"
                    :target (host-target b)               ; or a cross tuple
                    :optimize opt})]

    ;; third-party Go — REPLACES deps.edn. go get + go.mod pin are emitted.
    (go-require app "github.com/gorilla/websocket" "v1.5.3")

    ;; C via cgo (ADR 0011); sets CGO_ENABLED=1 for the graph (priority #5)
    (c-link app {:pkg-config "sqlite3"})

    ;; Zig-style comptime build step (ADR 0009): run cljgo at build time to
    ;; generate a source/asset before emission.
    (comptime-step b {:out "src/app/gen_table.cljg"
                      :fn (fn [] (build-lookup-table))})

    ;; library artifacts from the SAME codebase (ADR 0013)
    (lib b {:name "applib" :kind :c-shared :main "src/app/ffi.cljg"})

    ;; testing (ADR 0012): interpreted + compiled
    (test-step b {:paths ["test"] :both true})

    (install b app)
    (run b app)))
```

**Semantics.**
- The builder `b` accumulates artifacts + steps into a DAG; `cljgo build`
  runs the default step (install), `cljgo build run` / `cljgo test` / a named
  step run their subgraph. Mirrors `zig build`, `zig build run`, `zig build test`.
- `go-require` entries across all artifacts form the emitted module's `go.mod`
  requires; the emitter (design/04) already produces direct calls from
  go/packages type facts (spike S2) — build.cljgo just supplies the pinned
  module set instead of a `deps.edn`.
- `option` values are readable at build time (comptime) and can gate deps,
  targets, and generated code — the Zig `-D` ergonomic.
- **Bootstrapping:** the builder API and `build.cljgo` load through the
  interpreter (the emitter doesn't exist for build.cljgo until much later), so
  the builder is a small interpreted library + Go step-runners. Keep it in
  `core/build.cljg` (embedded) + `pkg/build` (step execution: go get, go.mod
  synth, go build, cgo env, cross-compile).

**Milestones.**
- **B1 — graph + single exe.** `build.cljgo` with `exe`/`install`/`run`;
  no deps. `cljgo build` runs it and produces the same binary as
  `cljgo build src/…`. Exit: hello-world via build.cljgo.
- **B2 — go-require (third-party, the priority-#1 payoff).** `go get` + go.mod
  synthesis + emitted direct calls for a real module (gorilla/websocket).
  Exit: a websocket client built from build.cljgo with zero bindings.
- **B3 — test-step + options.** `cljgo test` through the graph (ADR 0012 dual
  run); `option`/`-D`. Exit: the suite runner (track 3) invoked via a step.
- **B4 — C/FFI + library kinds.** `c-link` (cgo), `ffi`, `lib` c-shared/
  c-archive/go-archive (ADR 0011/0013). Exit: a cgo-sqlite build + a c-shared .so.
- **B5 — comptime-step + cljgo deps + cross-compile.** `comptime-step`
  codegen, `dep` (cljgo packages), `:target` matrix.

## 2. comptime — Zig metaprogramming alongside Clojure macros (ADR 0009)

ADR 0009 stands: Clojure macros are untouched; `comptime` adds value-level
compile-time execution. This track makes it real and wires it into build.cljgo.
- `(comptime <body>)` → body runs once at compile time; its *value* is embedded
  as a literal (embeddability checker + literal emitter in pkg/emit).
- `(comptime-assert pred msg)`, `(embed-file "path")` (build-cache-honest).
- In the REPL, compile-time == eval-time (dual-mode, ADR 0002).
- **Build integration:** `comptime-step` (track 1) is the same evaluator run to
  emit a source/asset; comptime and the build system share one execution model.
- Guidance doc: macros transform *syntax*, comptime computes *values* — lead
  with the split (ADR 0009 §4).

**Milestones.** C1 `comptime` value-embed + embeddability checker (post depends
on nothing external). C2 `embed-file` + `comptime-assert` + build-cache
invalidation. C3 `comptime-step` in build.cljgo (joins B5).

## 3. clojure.core compliance — the jank test-suite (ADR 0022)

The yardstick. Runner + shim + ratchet, then close core gaps until the number
climbs.

**Harness (T1).**
- `clojure.core-test.portability`: `when-var-exists` (macroexpand-time resolve
  check → body or nothing) + the suite's `thrown?` hook. Lives in
  `compat/clojure-test-suite/` in our repo (or contributed upstream as
  `doc/cljgo.md` + a bb task).
- Runner: load every `test/**/*.cljc` under the suite, run clojure.test, write a
  per-file `{:pass :fail :error :skipped}` scoreboard (EDN + JSON).
- Reader: verify Phase-2 takes `:default` and elides `:cljs`/`:clj` branches in
  BOTH `ns` `:require` conditionals and body forms (233/235 files need it).
- **Baseline run** sets the starting %. That number is the milestone metric.

**Coverage ratchet (T2).** CI gate: passing-file count may not decrease
(let-go's ratchet). Scoreboard committed; a drop fails the build.

**Gap-closing (T3, the long tail).** Driven by the scoreboard, in ADR→OpenSpec
units, each closing a batch of suite files + a cljgo conformance test:
- **Numeric tower** — ratios, `bigint`/`bigdec`, integer overflow → promotion,
  full `bit-*`, `quot`/`rem`/`mod` edge cases, `==` vs `=` numeric. (Biggest
  gap vs let-go; the suite hammers it.)
- **Seq/coll completeness** — the fns not yet in core.clj, chunked-seq behavior,
  transducer arities (ties to a future transducers ADR).
- **Reference/'watch/hierarchy** — `add-watch`/`remove-watch`, `derive`/`isa?`/
  `ancestors` + a global hierarchy (upgrades multimethods past the flat v0).
- **Metadata, printing, chars, arrays** — `with-meta`/`vary-meta`, exact
  `pr-str` forms, char ops, `aclone`/array seams (skip host-array internals via
  `when-var-exists`).

**Milestones.** T-M1 harness + baseline %; T-M2 ratchet in CI + numeric-tower
batch; T-M3 named % target (owner-set) with the gap batches sequenced by the
scoreboard.

---

## Sequencing (parallelizable)
Track 3 T1 (harness + baseline) needs nothing new and yields the metric that
directs everything — **do it first**. Track 1 B1–B2 (build.cljgo + third-party
Go) delivers the priority-#1 payoff and is independent of track 3. Track 2 C1
(comptime value-embed) is independent. B/C converge at B5/C3. The gap-closing
(T3) runs continuously against the ratchet regardless of B/C progress.

---

## 4. Cross-compilation — build for any OS/arch (ADR 0021)

Go cross-compiles for free (`GOOS`/`GOARCH`), and cljgo emits plain Go, so a
cljgo program builds for any target with **no target toolchain** — as long as
it stays pure-Go (or uses purego FFI, which is pure-Go by design, ADR 0011).

- **Single-file path:** `cljgo build --target linux/arm64 app.clj` → sets
  `GOOS=linux GOARCH=arm64` on the `go build`. `--target` takes `os/arch` (or a
  list to build a matrix); `cljgo targets` lists the supported tuples (`go tool
  dist list`).
- **build.cljgo:** `:target` on an artifact — `(exe b {:name "app" :target
  {:os "windows" :arch "amd64"}})`; a matrix step
  `(cross b app [:linux/amd64 :linux/arm64 :darwin/arm64 :windows/amd64])`
  fans out one artifact per tuple into `zig build`-style install paths
  (`out/<os>-<arch>/app`).
- **The cgo caveat (documented honestly):** a `c-link` (cgo) artifact
  cross-compiling needs a target C cross-toolchain (`CC=<target>-gcc`,
  `CGO_ENABLED=1`) — cljgo surfaces this as a clear error naming the missing CC,
  not a mystery. **purego FFI has no such limit** — it `dlopen`s at runtime, so
  a purego program cross-compiles anywhere. This is the ADR 0011 reason purego
  is the *exposed* FFI and cgo is wrapped-in-a-Go-package: cross-compile
  ergonomics. Pure-Go + purego = "build for whatever OS they want", trivially.

## 5. Suite remediation — the ordered backlog (baseline **~43%**, ADR 0022)

Measured 2026-07-15: cljgo resolves ~101/235 tested `clojure.core` vars.
Batches below are ordered by (unlock-count ÷ effort); each is an
ADR→OpenSpec→apply unit that turns a set of suite files green and adds a cljgo
conformance test, gated by the T2 ratchet.

- **Batch 0 — harness (do first).** `resolve`/`find-var`/`ns-resolve`/`eval`
  (the shim needs them), `when-var-exists`, the `.cljc` runner + scoreboard,
  baseline %. No new language semantics beyond var reflection.
- **Batch 1 — cheap breadth (biggest count/effort).** Predicates
  (`any?` `coll?` `ifn?` `fn?` `seqable?` `counted?` `associative?`
  `reversible?` `sorted?` `set?` `list?` `uuid?` `var?` `nan?` `pos-int?`
  `neg-int?` `number?` `rational?` `ratio?` `decimal?` `double?` `float?`
  `int?` `integer?` `boolean?` `char?` `*-ident?` `*-keyword?` `*-symbol?`),
  coercions (`int` `long` `double` `float` `boolean` `char` `byte` `short`
  `num`), simple seq/coll (`butlast` `last` `peek` `pop` `subvec` `rseq`
  `find` `key` `val` `set` `disj` `empty` `drop-last` `take-last` `ffirst`
  `nfirst` `fnext` `not=` `compare` `identical?`). Mostly one-liners.
- **Batch 2 — numeric tower.** `bigint`/`bigdec` (math/big is already linked),
  ratios + `numerator`/`denominator`/`rationalize`/`rational?`, integer
  overflow→promotion, full `bit-*` + `unsigned-bit-shift-right`, `quot`/`rem`/
  `mod` edges, `parse-long`/`parse-double`/`parse-boolean`/`parse-uuid`,
  `rand`/`rand-int`/`rand-nth`/`random-sample`/`random-uuid`. (cljgo's thinnest
  area vs let-go; the suite hammers it.)
- **Batch 3 — transients.** `transient`/`persistent!`/`assoc!`/`conj!`/`disj!`/
  `dissoc!`/`pop!` — needs the PHM/PV transient support that Glojure's vendored
  `pkg/lang` lacks (a known gap since M0).
- **Batch 4 — watches + hierarchy.** `add-watch`/`remove-watch`, `derive`/
  `underive`/`isa?`/`ancestors`/`descendants`/`parents`/`make-hierarchy` — and
  this upgrades multimethods (ADR, Wave 4) from flat `=` to `isa?` dispatch.
- **Batch 5 — printing/meta/misc.** `format`/`with-out-str`/`print-str`/
  `println-str`/`prn-str` + exact `pr-str` fidelity, `with-meta`/`vary-meta`/
  metadata, `bound-fn`/`when-first`/`realized?`/`intern`/`comment`/`taps`.

Metric: report a single climbing % per batch. Non-goal: host-array internals
(`aclone`, `bean`) — `when-var-exists` skips them; the denominator is "vars
cljgo claims".
