# pkg/lang TODO

## Deferred: HAMT transients (S4 defect #2)

`PersistentHashMap` implements neither `AsTransient` nor
`IEditableCollection`; the only map "transient" is
`persistentarraymap.go`'s `TransientMap`, a fake wrapper that delegates
every op to persistent assoc. The Go port also dropped the `edit`
ownership token from all three HAMT node types, so nothing is prepared.
Pinned by `TestDefectNoHAMTTransients` in `s4_defects_test.go` — invert
that test when this lands.

**Plan** (design doc 02 §2.2; spike S4 estimate: **3–5 days**, a
well-trodden port of Java `PersistentHashMap$TransientHashMap`, no
design risk):

- Add `edit *atomic.Bool` tokens + `assoc(edit, ...)`/`without(edit, ...)`
  variants to bitmapIndexedNode / arrayNode / hashCollisionNode.
- `ensureEditable` per node; in-place mutation when owned; `persistent!`
  sealing (token invalidated; ops after sealing panic).
- Property-test transient-vs-persistent equivalence.
- Expected win: ~4–6× on `into`-style bulk builds (S4 smoke: persistent
  HAMT assoc ~338 ns/op).

Related (also deferred, <1 day per S4): Clojure-shaped
`conj!/assoc!/pop!` adapter over the existing elvish vector transients.

## Printer defects (found M1-A, 2026-07-12)
- EmptyList prints as `(nil)` via the ISeq branch in strconv.go — should print `()`.
- Inf/NaN floats print as `Infinity`/`NaN` — Clojure pr-str prints `##Inf` `##-Inf` `##NaN`.
