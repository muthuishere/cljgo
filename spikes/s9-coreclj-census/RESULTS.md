VERDICT: VALIDATED — 76% of upstream core.clj is inheritable as-is or via a mechanical token-substitution pass; only ~19% (137 forms, median 10 lines) needs genuine host-surface rewriting, ~5% is dropped. Glojure independently proves the same split empirically (38.9% byte-identical, 92% identical-or-near after mechanical canonicalization).

# S9 — Upstream core.clj reuse census

- **Source censused:** `clojure-master/src/clj/clojure/core.clj` (8292 lines, Clojure master ~1.13-snapshot)
- **Splitter:** `census.py` — paren-balanced top-level reader (handles strings, `#""` regex, char literals, `^meta`/`#_`/`#'` prefixes). Found **724 top-level forms**; sanity check `grep -c '^('` = 722 (the 2 extras are prefix-line forms). ✓
- **Glojure baseline:** `refs/glojure/scripts/rewrite-core/` (originals/core.clj 8229 lines → `pkg/stdlib/clojure/core.glj` 8054 lines) — comparison in `subclassify.py`.

## 1. Census table

Classification is heuristic (token-level detection of interop; C by curated name list); target accuracy ±5%.

| Class | Meaning | Forms | % | first-500 % |
|---|---|---:|---:|---:|
| **A** | Pure Clojure — loads as-is given our special forms | 271 | **37.4%** | 29.2% |
| **B-rt** | Interop is *only* `clojure.lang.*` (RT/Numbers/Var/ISeq… — i.e. references to the Clojure runtime itself, which is our `pkg/lang`) → **mechanically rewritable** | 281 | **38.8%** | 48.2% |
| **B-host** | Touches real `java.*` surface (String/Math/regex/IO/threads/UUID…) → hand rewrite | 137 | **18.9%** | 16.6% |
| **C** | JVM-only, drop or stub (`class?`/`bases`/`supers`/`cast`, `unchecked-*` ×23, `memfn`, `resultset-seq`, `enumeration-seq`, `add-classpath`, `compile`, `load-reader`, agent-errors pair) | 35 | **4.8%** | 6.0% |

Coarse A/B/C as originally specified: **A 37.4% / B 57.7% / C 4.8%.** The critical refinement: **two-thirds of B is B-rt** — the `{:inline (. clojure.lang.RT (op ~x))}` pattern and `clojure.lang.*` type hints, which map 1:1 onto our `pkg/lang` API and rewrite with pure token substitution. **Effective reuse = A + B-rt = 76.2%.**

B-reason histogram (a form can have several): java-class 349 · dot-form 165 · ^Class-hint 156 · .method 125 · Ctor. 31 · new 17 · Class/member 15 · definline 8 · ^prim-hint 6 · proxy/reify 3 · gen-class 2.

### Cumulative by file position

| First N forms | A | B | C |
|---:|---:|---:|---:|
| 100 | 30.0% | 69.0% | 1.0% |
| 200 | 27.0% | 65.5% | 7.5% |
| 300 | 29.0% | 65.3% | 5.7% |
| 400 | 29.0% | 64.0% | 7.0% |
| 500 | 29.2% | 64.8% | 6.0% |
| 600 | 34.2% | 60.0% | 5.8% |
| 724 | 37.4% | 57.6% | 4.8% |

The bootstrap head is *more* interop-dense than the tail — but it's almost all **B-rt** (48.2% of the first 500): the primordial defs (`list`/`cons`/`first`/`next`/`seq`/`assoc`/`meta`…) are one-line `(. clojure.lang.RT (op …))` bodies. That's the *easiest* rewrite class, not the hardest. Pure-A landmarks load unchanged from form 1: `ns`, `let`/`loop` shims, `second`..`nnext`, `when`/`when-not`, `not`, `concat`, and the whole macro tower once syntax-quote support exists (line 747 marker).

## 2. Top-30 most-referenced B/C defs (direct downstream users)

Rewrite these and the bulk of the file unlocks. (Transitive closure saturates at ~644 — everything reaches everything through `defn`/`fn` — so direct-use count is the ranking signal.)

| # | uses | cls | name | line | interop kind |
|--:|--:|---|---|--:|---|
| 1 | 504 | B-rt | `defn` (macro tower) | 285 | Ctor. on clojure.lang, ^hints |
| 2 | 196 | B-rt | `fn` | 42/4633 | ^hints, clojure.lang |
| 3 | 80 | B-rt | `first` | 49 | RT inline |
| 4 | 73 | B-rt | `instance?` | 141 | ^Class hint |
| 5 | 72 | B-rt | `defmacro` | 446 | dot-form |
| 6 | 70 | B-rt | `seq` | 128 | RT inline |
| 7 | 64 | B-rt | `next` | 57 | RT inline |
| 8 | 56 | B-rt | `apply` | 662 | ^hints |
| 9 | 49 | B-rt | `cons` | 22 | RT inline |
| 10 | 46 | B-host | `map` | 2741 | .method (chunks) |
| 11 | 40 | B-rt | `count` | 876 | RT inline |
| 12 | 38 | B-rt | `=` | 902/785 | Util.equiv |
| 13 | 34 | B-rt | `reduce1` | 932 | .method on ISeq |
| 14 | 33 | B-rt | `conj` | 75 | RT inline |
| 15 | 30 | B-host | `str` | 546 | StringBuilder |
| 16 | 29 | B-rt | `lazy-seq` | 685 | LazySeq. ctor |
| 17 | 26 | B-rt | `list` | 16 | PersistentList |
| 18 | 25 | B-rt | `val` | 1586 | IMapEntry |
| 19 | 25 | B-rt | `meta` | 204 | IMeta |
| 20 | 25 | B-rt | `assoc` | 183 | RT inline |
| 21 | 24 | B-rt | `with-meta` | 213 | IObj |
| 22 | 24 | B-rt | `vector?` | 176 | IPersistentVector |
| 23 | 24 | B-host | `name` | 1601 | ^String hint |
| 24 | 23 | B-rt | `nil?` | 438 | identical inline |
| 25 | 21 | B-rt | `rest` | 66 | RT inline |
| 26 | 20 | B-rt | `key` | 1579 | IMapEntry |
| 27 | 19 | B-rt | `get` | 1512 | RT inline |
| 28 | 19 | B-rt | `cond` | 576 | IllegalArgumentException |
| 29 | 18 | B-rt | `symbol?` | 564 | clojure.lang.Symbol |
| 30 | 16 | B-host | `ns` (macro) | 5881 | gen-class ref |

**26 of the top 30 are B-rt** — providing the `pkg/lang` RT surface (First/Next/Seq/Cons/Count/Get/Assoc/Equiv/Meta + the interface predicates) unlocks essentially the entire dependency graph. The only high-traffic true host rewrites are `map` (chunked-seq methods — still our own types really), `str` (StringBuilder → `strings.Builder`), `name`, and the `ns` macro.

### B-host: the real hand-porting inventory

137 forms, **median 10 lines, mean 14, total ~1,980 lines**; 75 forms ≤10 lines, 116 ≤20. Clusters: string/regex (`str`, `subs`, `re-*`, `parse-*`), numerics tower (`bigint`/`bigdec`/`biginteger`, casts), IO (`slurp`/`spit`, `*in*`/`*out*` plumbing), concurrency (`future-call`, `pmap`, `promise`, `seque` → goroutines/channels), UUID/random, and big macros with only incidental host touches (`for`, `case`, `condp`, `defmulti` — mostly `IllegalArgumentException` + a String hint; near-A in practice).

## 3. Glojure comparison

Glojure's answer to this exact problem is `scripts/rewrite-core/rewrite.clj` (78 KB of rewrite-clj zipper rules): **inherit upstream core.clj wholesale, patch mechanically, never fork by hand.** Rules are sexpr-level: type maps (`String`→`go/string`, `Long`→`go/int64`), `RT-replace` (each `clojure.lang.RT` method → a Go func in `pkg/runtime`), `omit-symbols` for the unreachable set. Output core.glj is 8054 lines vs 8229 original — 98% of the line count survives.

Form-by-form diff (matched on (head,name), whitespace-normalized; 646 common forms):

| Bucket | Forms | % |
|---|---:|---:|
| Byte-identical | 251 | **38.9%** |
| Identical after canonicalizing pkg paths + method-name case (pure mechanical) | +142 | → **60.8%** |
| ≥85% similar after canon (one sub-expression swapped) | +201 | → **92.0%** |
| Structurally rewritten | 52 | **8.0%** |
| Dropped by Glojure | 17 | (`tap>`/tap machinery, `eduction`/`Eduction`, `Inst`, `print-method`, `clojure-version`, annotation helpers, `when-class`) |

Cross-validation: Glojure's 38.9% byte-identical ≈ our heuristic A 37.4%; their 52 structural rewrites + 17 drops ≈ our B-host-hard + C tail. The classifier is calibrated.

Typical rewrite shapes (from 20 sampled diffs, in `glojure-diff.json`):
1. **Path substitution** (most common): `clojure.lang.RT/get` → `github.com:glojurelang:glojure:pkg:lang.Get`; hint `^clojure.lang.BigInt` → `^…pkg:lang.*BigInt`. Pure textual.
2. **Java type → Go type**: `BigInteger` → `math:big.*Int`, `String` → `go/string`.
3. **Drop-a-clause**: `assoc!` loses the `^clojure.lang.ITransientAssociative` hint, gains an explicit `instance?` guard + Go error ctor. Body logic unchanged.
4. **Full rebody** (rare, ~8%): `bigdec`, `re-pattern`, `pmap`, `future-call`, `slurp`/`spit` — host semantics genuinely differ.

## 4. Recommendation — bootstrap order for `core/`

**Strategy: inherit, don't port.** Vendor upstream core.clj; run our own (much smaller) rewrite pass; hand-write only the B-host tail. Two structural improvements over Glojure available to us:

1. **Resolve `clojure.lang.*` natively.** Instead of textually rewriting 281 B-rt forms, teach the analyzer that `clojure.lang.Foo` / `(. clojure.lang.RT (op …))` resolves to `pkg/lang` (a fixed alias table, ~60 entries). Then **A + B-rt = 76% loads verbatim from the unmodified upstream file** — zero-drift upgrades when upstream moves. The rewrite script shrinks to the B-host/C surface only.
2. **Drop hints, don't map them.** `^Class`/`^prim` hints are JVM reflection-avoidance; our emitter can ignore unknown hints in M1 (they're metadata, not semantics), deferring the type-hint story to the perf milestone.

**Phased order (by file section):**

| Phase | Section (lines) | What | Effort |
|---|---|---|---|
| 1 | 1–747 (through syntax-quote support) | Primordial defs + macro tower. Needs `pkg/lang` RT surface (~40 funcs: First/Next/Seq/Cons/Conj/Assoc/Get/Count/Equiv/Meta/WithMeta + predicates) and `sigs`/`defn`/`defmacro`. ~15 B-host forms here (`str`, `symbol`, `keyword`, `gensym`, `cond`'s exception). | The gate. ~1–2 wk incl. lang API |
| 2 | 747–2058 (seq fns, math, sequence library) | Mostly A/B-rt riding phase 1; math tower needs `pkg/lang.Numbers`. Skip `unchecked-*` (alias to checked or omit, as Glojure does). | days |
| 3 | 2058–3356 (refs/agents/fn-stuff/more seqs) | Atoms/agents map to our concurrency layer; `pmap`/`seque`/`future` → goroutine rewrites (hand, ~10 forms). | ~1 wk |
| 4 | 3356–6535 (transients, multimethods, ns/load machinery, printing vars) | `ns`/`refer`/`load-lib` need our load-path design (design 06); `defmulti`/`case`/`for` are near-A. Drop proxy/gen-class limbs. | ~1–2 wk |
| 5 | 6535–end (reduce/transducers, futures, data readers) + `load`-ed siblings (`core_print.clj`, `core_deftype.clj`, …) | Transducers are pure-A once `reduce` lands; futures section rewrites to goroutines; data-readers to our reader. Siblings are a separate (smaller) census. | ~1 wk |

**Expected total:** mechanical pass ≈ a fixed alias table + a few dozen substitution rules (vs Glojure's 78 KB, because of improvement 1); hand-ported code ≈ **~140 forms / ~2,000 source lines, most ≤20 lines each** — bounded weeks of work, not a rewrite of an 8,300-line stdlib. This confirms M1/M5 scoping: `core/` is an inheritance problem with a ~20% porting tail.

### Caveats
- Heuristic classification, ±5%: some B-host forms are near-A (`for`, `condp` — one exception ctor), some A forms may hide behind macros that expand to interop. Glojure cross-check bounds the error.
- Census covers core.clj only; `(load "core_print")` etc. pull ~6 sibling files (deftype/protocols/print/instant) that skew more host-heavy — separate, smaller censuses when their milestones arrive.
- `fn` appears def'd twice (bootstrap `def` at line 42, full macro later); dedupe counts once by first definition.

## Files
- `census.py` — splitter + A/B/C classifier + def→use graph (writes `census.json`)
- `subclassify.py` — B-rt/B-host split + Glojure form-by-form diff (writes `glojure-diff.json`)
- Run: `python3 census.py <core.clj> census.json && python3 subclassify.py <glojure-originals> <core.glj> <core.clj>`
