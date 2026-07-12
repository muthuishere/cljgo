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

## Printer divergences found but deferred (M1-A sweep vs clojure 1.12.5 CLI, 2026-07-12)

Fixed in this sweep (see PROVENANCE.md): double formatting
(Double.toString semantics incl. subnormal quirk), `##Inf`/`##-Inf`/
`##NaN`, empty list `()`. Spot-checked OK against the oracle: ratios
(`1/3`), BigInt `N` suffix, BigDecimal `M` suffix, map `, ` separator,
string quoting/escapes in pr, keywords/symbols. Still open (not
printer-internal, so out of scope for the printer fix):

- **Reader has no character literals** — `\a` / `\newline` fail at read
  time ("unable to resolve symbol: ewline"), so `Print`'s Char branch
  (`CharLiteralFromRune`: `\a`, `\newline`, `\space`, `\tab`, ...) is
  unreachable from source and has no conformance file yet. Fix lives in
  `pkg/reader`; add `print-char.clj` when it lands (oracle: `(pr-str \a)`
  => `\a`, `(pr-str \newline)` => `\newline`).
- **`str`, `prn`, `print` not yet in core** (M1 core surface) — the
  ToString/non-readably print path (`(str ##Inf)` => `"Infinity"`,
  `(str 42N)` => `"42"`, `(println "s")` unquoted) can't be pinned by
  conformance files until they exist; behavior is implemented in
  `ToString`/`Print(readably=false)`.
- **Regex literals** `#"..."` unverified end-to-end (reader support
  unclear); printer side (`#"` + pattern + `"`) matches the oracle shape.
