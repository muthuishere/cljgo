VERDICT: PASS — Glojure pkg/lang is cleanly severable and modernizable; ~3.3% of it was interpreter glue, the rest compiles standalone on go1.26/darwin-arm64 with ZERO external deps (not even x/), keyword interning ports to stdlib `unique.Handle` with the §4.4 `k1 == k2` contract intact, and all 74 tests pass. Adopt this as the seed of the real `pkg/lang`.

# S4 — Vendor-prune Glojure pkg/lang

Machine: darwin/arm64 (Apple M5 Pro), go1.26.3. Module: `cljgo-spike-s4`
(this directory). Full change-by-change log: `SURGERY.md`.

## 1. Inventory

Copied from `refs/glojure`: `pkg/lang` (91 files, 15,818 LOC) + the four
`internal/` packages it needs — `murmur3`, `seq`, `persistent/vector`
(elvish port, own BSD LICENSE), `goid` — 1,441 LOC. **Total in: 17,259 LOC.**

External deps it dragged in as copied: `go4.org/intern` (keyword),
`bitbucket.org/pcastools/hash` (number/bigint/bigdec hashing),
`mitchellh/hashstructure` (string hasheq), `stretchr/testify` (one test
file), plus the internal `pkg/pkgmap` host-class registry. All five removed.

## 2. What was cut / changed (see SURGERY.md for the full log)

- **Deleted 560 LOC of interpreter glue**: `builtins.go` (+ its test),
  `class.go`, `environment.go` — reflect Go-interop builtins, JVM-style
  Class wrapper, and the Environment/Eval interface. Nothing else in the
  package referenced them except three small sites (namespace.go ×2,
  multifn.go ×1), each severed with a local rewrite.
- **`go4.org/intern` → stdlib `unique` (Go 1.23+)**: `Keyword` now holds a
  `unique.Handle[string]`; still a comparable 2-word struct, `==` still O(1)
  identity. Strict improvement: maintained stdlib, weak (GC-reclaimable)
  canonical storage.
- **pcastools/hashstructure → local stdlib hashing** (murmur3-fmix64 fold for
  numbers, Java `String.hashCode` analog for string hasheq — the latter is a
  step *toward* JVM hash parity). Category invariants preserved
  (int64/uint64/big.Int agree; float32 widens to float64).
- **testify → stdlib testing** (one file, assertions 1:1).
- **Kept**: reflect fallback paths (apply/equal/hashes — doc-02-sanctioned
  slow paths, no interpreter imports), goid dynamic-var registry (doc 02
  §3.3 v0 plan), all collections/numeric tower/concurrency refs.

**Bottom line: kept 16,671 of 17,259 original LOC (96.6%), cut ~588 (3.4%),
added 381 LOC of new S4 verification tests.**

## 3. Compile + tests

- `go build ./...` and `go vet ./...`: clean. `go.mod` has **no require
  directives at all** — zero external deps, zero `golang.org/x/` deps.
- Tests: **74 top-level test funcs, 0 failures** (146 incl. subtests):
  - `lang`: 60 (54 inherited Glojure tests + 6 new S4 tests) — PASS
  - `internal/persistent/vector`: 9 inherited — PASS
  - `identity` (new): 5 — PASS
- Trivial fixes required: rewrite testify assertions in
  `persistenthashmap_test.go`; delete 4 `TestGoSliceString*` tests that
  tested deleted builtins. **No nontrivial breaks.** (The fuzz seed corpus
  for `FuzzPersistentHashMap` also passes.)

## 4. §4.4 identity contract under unique.Handle — VERIFIED

`identity/identity_test.go` with two separately compiled packages
(`identity/pkga`, `identity/pkgb`) each hoisting the same keyword literals
to package-level vars, exactly as the emitter will:

- `pkga.KwFoo == pkgb.KwFoo` and namespaced `:my.ns/bar` — plain Go `==`,
  true across packages and vs locally interned values.
- 200 goroutines interning concurrently → all `==` the main-goroutine value.
- Different constructors (`InternKeyword`/`NewKeyword`/`InternKeywordString`)
  converge on identical values; distinct names stay distinct.
- Keywords work as native Go map keys across packages; `Equals` and `HashEq`
  agree for identical keywords.
- Symbols confirmed structural (not interned), per doc 02 §3.1.
- Cost: `k1 == k2` benches at **1.6 ns** (register compare); interning at
  331 ns — irrelevant since interning happens once per literal at package init.

Caveat noted for the real fork: `unique.Handle` identity is per-process, so
the contract holds across separately-compiled packages *linked into one
binary* (our model). Anything crossing process boundaries must re-intern —
same rule JVM Clojure has.

## 5. Known defects — both CONFIRMED (pinned by `lang/s4_defects_test.go`)

1. **Equiv aliased to Equals** — `equal.go` lines 7–9, verbatim:
   `func Equiv(a, b any) bool { return Equals(a, b) }`. One nuance vs the
   doc-02 wording: `NumbersEqual` *is* category-aware
   (`category(x) == category(y) && ...`), so Equiv's `=` semantics for
   numbers are largely correct today (`(= 1 1.0)` → false, `(= 1 1N)` → true,
   both verified). The live damage is the other direction: **Equals is far
   too loose for a Java-`.equals` analog** — `Equals(int32(1), int64(1))`
   and `Equals(float32(1.5), float64(1.5))` return true. There is one
   relation where Clojure requires two.
   **Fix effort: 1–2 days.** Write a type-strict `Equals`, keep Equiv on the
   category path, then audit the ~87 non-test `Equals`/`Equiv` call sites for
   which relation each means (map/set lookup → Equiv; interop/identity paths
   → Equals). Mechanical but must be done with conformance tests in hand.
2. **No HAMT transients** — `PersistentHashMap` (944 LOC) implements neither
   `AsTransient` nor `IEditableCollection` (verified by type assertion). The
   only map "transient" is `persistentarraymap.go`'s `TransientMap`, a fake
   wrapper around the persistent array map (`// TODO: implement transients`)
   that delegates every op to persistent assoc. The Go port also dropped the
   `edit` ownership field from all HAMT node types, so nothing is prepared.
   **Fix effort: 3–5 days.** Port Java `PersistentHashMap$TransientHashMap`:
   add `edit` tokens + `assoc(edit,...)/without(edit,...)` variants to the
   three node types, `ensureEditable`, persistent! sealing; property-test
   transient-vs-persistent equivalence. Well-trodden port, no design risk.
   (Vector transients already exist via the elvish port — Clojure-shaped
   `conj!/assoc!/pop!` naming is a thin adapter on top.)

## 6. Smoke benchmarks (Apple M5 Pro, go1.26.3)

| benchmark | result | per-element | expectation check |
|---|---|---|---|
| Vector `Cons` ×10k | 874 µs | ~87 ns/conj | ✓ typical persistent 32-way-trie cost (JVM ~50–100 ns) |
| Vector transient conj ×10k | 152 µs | ~15 ns | ✓ 5.7× over persistent — transients work (vector only) |
| HAMT `Assoc` ×10k | 3.38 ms | ~338 ns/assoc | ✓ plausible for persistent HAMT, upper end; transients (defect 2) should give ~4–6× for `into`-style bulk builds |
| HAMT `ValAt` | 51 ns | — | ✓ hash + ≤3-level trie walk |
| LazySeq realize ×10k | 579 µs | ~58 ns/elem | ✓ mutex + closure per cell; correctness verified (lazy until walked, realize-at-most-once on re-walk) |
| Atom `Swap` (uncontended) | 57 ns | — | ✓ CAS loop + IFn invoke; 8×1000 contended swaps lose no updates, `CompareAndSet` identity semantics correct |
| Keyword intern | 331 ns | — | ✓ once per literal at init, off any hot path |
| Keyword `==` | 1.6 ns | — | ✓ the whole point of §4.4 |

All four smoke behaviors (vector build, HAMT assoc, lazy realization, atom
CAS) also have correctness tests, including persistence checks
(assoc-doesn't-mutate, without-doesn't-mutate).

## 7. Does this become our real pkg/lang?

**Yes.** The bet holds: severing took hours, not weeks — 4 file deletions,
3 small local rewrites, 2 dependency swaps, and the result is a
zero-dependency, fully-tested 16.7k-LOC runtime we own outright (EPL-1.0
preserved: `LICENSE-glojure.md`, elvish `LICENSE` kept in-tree). The doc 02
§4 "fix in place" list is confirmed accurate and bounded: Equiv/Equals split
(1–2 d), HAMT transients (3–5 d), Clojure-shaped vector-transient adapter
(<1 d), plus optional later passes to shrink reflect fallbacks. Recommended
path: lift this spike's `lang/` + `internal/` as the starting tree for the
real `pkg/lang` (fresh copy at a pinned upstream SHA, replaying SURGERY.md),
then land the two defect fixes as the first two PRs, using
`s4_defects_test.go`'s inverted assertions as their acceptance tests.
