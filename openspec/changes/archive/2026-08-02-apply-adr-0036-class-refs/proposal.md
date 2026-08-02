# apply-adr-0036-class-refs

## Why

ADR 0036 (docs/adr/0036-reader-features-and-class-refs.md, accepted)
settles the two decisions blocking the hierarchy cluster of the jank
clojure-test-suite (ADR 0022; baseline 217/242):

- **Reader features**: cljgo answers exactly `#{:cljgo :default}` —
  ratifying the existing `pkg/reader/readcond.go` behavior as a binding
  decision. Never `:clj`: the suite's own files show `:clj` branches
  carry JVM class-inheritance assertions that would ADD failures while
  unlocking none.
- **Bare class references**: `derive.cljc`/`descendants.cljc` (and
  friends) use `String`/`Object`/`clojure.lang.PersistentHashSet` as
  VALUES. cljgo has no JVM classes; the ADR introduces interned, opaque
  ClassRef values from a fixed fail-closed name table, resolved only
  after normal var resolution misses — with NO fabricated inheritance
  (precedence principle).

## What Changes

- `pkg/eval/class_refs.go` (new): `ClassRef` type, canonicalizing name
  table (`String` ≡ `java.lang.String`), interning, `classRefVar`
  resolution fallback, and the shared `classNameMatchesValue` designator
  matcher (extracted from `-instance-of-name?`).
- `pkg/eval/eval.go`: `resolveVar` tries `classRefVar` as the LAST
  resort (user definitions always win; vars interned in `cljgo.classes`).
- `pkg/eval/misc_builtins.go`: new clojure.core `class?` — true for
  ClassRefs and deftype/defrecord TypeMarkers.
- `pkg/eval/protocols.go`: `-instance?` accepts a ClassRef in hand via
  the shared name matcher.
- `core/hierarchies.cljg`: derive's asserts aligned to the JVM's real
  ones (classes valid as tags, never as global parents; 3-arity allows
  non-namespaced Named); `descendants` throws "Can't get descendants of
  classes" on class tags — all oracle-verified (clojure 1.12.5).
- Conformance: `conformance/tests/class-refs-hierarchy.clj` (eval-only,
  reasons recorded in-file) and the no-match selecting elision case in
  `conformance/tests/reader-conditionals.clj`.
- No reader or suite-harness code change (Decision A is a ratification).

## Impact

- Suite: 217 → 219 passing (derive.cljc, descendants.cljc green).
  ancestors/parents/add_watch/remove_watch remain red with causes
  recorded in the ADR (JVM type-inheritance ancestry / refs+agents —
  out of honest reach of this change).
- `(pr-str String)` ⇒ `"java.lang.String"`; class refs usable as map/set
  keys and hierarchy tags. Names outside the table still fail to resolve.
