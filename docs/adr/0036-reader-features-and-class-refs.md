# ADR 0036 — reader-conditional features stay `:cljgo`+`:default`; bare class references become interned class refs

Date: 2026-07-16 · Status: accepted

## Context

The last cluster of jank clojure-test-suite failures (ADR 0022; baseline
217/242) is blocked on two intertwined questions.

**A. Which reader-conditional feature keywords does cljgo answer to?**
The suite's `.cljc` files gate per-dialect code with
`#?(:clj … :cljs … :cljr … :lpy … :phel … :bb … :default …)`. cljgo's
Phase-2 reader (`pkg/reader/readcond.go`) already answers `:cljgo` and
`:default` — a decision recorded only as a code comment, never as an ADR.
Other non-JVM Clojures split: babashka answers `:bb` AND `:clj` (defensible
for bb — it literally runs JVM Clojure via SCI on a JVM); ClojureScript
answers only `:cljs`; jank answers `:jank` + `:default`.

**B. Bare class references as values.** `ancestors`/`derive`/`descendants`/
`parents` suite files error on bare JVM class names used as VALUES —
`(def AncestorT #?(… :default Object))`, `(derive String ::object)`,
`(descendants Object)`. cljgo has no JVM classes (design/05, ADR 0010) and
`instance?`'s class position is deliberately syntax, not a value (ADR 0026).

### Evidence gathered (oracle = JVM Clojure 1.12.5, `clojure` CLI)

Reader:
- A conditional with NO matching feature and no `:default` reads as
  NOTHING — `(read-string {:read-cond :allow :features #{:clj}}
  "[1 #?(:cljs 2) 3]")` ⇒ `[1 3]`. cljgo's reader already does exactly
  this (elision), first-match-wins, `#?@` splicing included
  (`pkg/reader/phase2_test.go`, per-case oracle citations).
- Answering `:clj` was checked against the actual failing files, not in
  the abstract: the ONLY `:clj` branch that would help is `add_watch.cljc`'s
  `(catch #?(… :clj clojure.lang.ExceptionInfo) e …)` (cljgo's catch
  matches class names by string, so it would work) — but that file is
  still blocked on `ref`/`agent`, so `:clj` unlocks nothing. Meanwhile
  `ancestors.cljc` has a `:clj`-only branch
  (`(is (contains? (ancestors ChildT) AncestorT))`, ChildT =
  `clojure.lang.PersistentHashSet`, AncestorT = `Object`) asserting JVM
  class-inheritance ancestry cljgo does not have — answering `:clj` would
  PULL IN new failures. `:clj` loses on the evidence, not just on principle.

Hierarchies with classes (oracle-verified, scratch run 2026-07-16):
- `(derive String ::object)` (global) succeeds; class is a valid TAG.
- `(derive ::tag String)` THROWS (parent must be Named — CCE from
  `(namespace parent)` on a Class).
- 2-arity derive's tag assert is
  `(or (class? tag) (and (instance? Named tag) (namespace tag)))`;
  3-arity derive asserts parent `(instance? Named parent)` and allows
  non-namespaced keywords.
- `(descendants Object)` and `(descendants h Object)` BOTH throw
  "Can't get descendants of classes"; so does `(descendants SomeRecord)` —
  record types are classes.
- `(parents Object)` ⇒ nil. `(parents String)` ⇒ a set including
  `java.lang.Object` etc. — JVM type-inheritance ancestry.
- `(class? String)` ⇒ true; deriving a defrecord's class as a tag in the
  global hierarchy works.

## Decision

### A. cljgo's reader features are exactly `#{:cljgo :default}` — ratified

The existing reader behavior is elevated from code comment to ADR:
- `:cljgo` is cljgo's platform feature; `:default` always matches last;
  first match wins; a no-match conditional elides (all oracle-verified).
- cljgo NEVER answers `:clj` (or any other dialect's feature). `:clj`
  branches routinely contain JVM interop (`clojure.lang.*` classes, Java
  methods) that cannot work here; the suite evidence above shows `:clj`
  would add failures and unlock none. babashka's precedent does not
  transfer — bb IS JVM Clojure under an interpreter; cljgo is not a JVM.
  This is the same call jank made (`:jank` + `:default`).
- No reader or suite-harness change is needed for Decision A; the
  conformance file `conformance/tests/reader-conditionals.clj` gains the
  no-match *selecting* elision case (`[1 #?(:cljs 2) 3]` ⇒ `[1 3]`).

### B. Well-known class names resolve to interned, opaque class refs

A fixed, fail-closed table of well-known JVM class names (the ADR 0026
designator vocabulary — `String`, `Long`, `Object`, `clojure.lang.Keyword`,
`java.util.UUID`, … — simple or fully qualified, canonicalized so `String`
and `java.lang.String` are the SAME value) resolves, at symbol-resolution
level, to interned `*eval.ClassRef` values when — and only when — normal
var resolution fails (user definitions always win; nothing is shadowed).
The vars live in a dedicated `cljgo.classes` namespace, interned lazily.

What a ClassRef IS: an opaque named constant with identity equality that
prints as its canonical name (`java.lang.String`). What it is NOT: a class.
**No inheritance is fabricated** (precedence principle, 2026-07-12):
`(parents String)` is nil unless something was explicitly derived;
`(isa? String ::object)` is true only after `(derive String ::object)`.
cljgo will not pretend `String`'s superclass is `Object`.

Supporting semantics, each oracle-verified above:
- `class?` (new clojure.core var, missing until now) ⇒ true for ClassRefs
  and for deftype/defrecord `*TypeMarker*`s (cljgo's class analog), false
  otherwise.
- `core/hierarchies.cljg` aligns to the JVM's real asserts: global derive
  accepts `(or (class? tag) (namespaced keyword/symbol))` as tag (so
  deriving a defrecord type or a ClassRef works) while parent stays
  Named-and-namespaced; 3-arity derive asserts tag `(or class? named)`
  and parent named; `descendants` throws "Can't get descendants of
  classes" when the tag is a class (ClassRef or TypeMarker), both arities.
- `-instance?` (the non-literal-symbol `instance?` path) also accepts a
  ClassRef in hand, matched via the same ADR 0026 name table — so
  `(instance? (identity String) "x")` now works. (A literal LOCAL symbol
  still takes the macro's name fast path — ADR 0026's known limitation,
  unchanged.)

Class refs are interpreter-resolution-level for now; AOT emission of a
bare class-name symbol is deferred (the suite runs interpreted per
ADR 0022 decision 4), so the conformance file carries a
`;; harness: eval` waiver with this reason.

### What each option unblocks (measured)

- `derive.cljc`, `descendants.cljc` — unblocked by B (String/Object as
  hierarchy tags + JVM-faithful asserts/throws). Target: green.
- `ancestors.cljc`, `parents.cljc` — NOT unblocked, deliberately: after
  class refs land they next trip on munged dotted protocol names
  (`clojure.core_test.parents.TestParentsProtocol` as a value), and past
  that their `:default` branches assert JVM type-inheritance ancestry
  (`(contains? (parents String) Object)`,
  `(contains? (ancestors ChildT) AncestorT)`, records having
  `clojure.lang.Associative` among ancestors) that only a faked class
  hierarchy could satisfy. Documented as failing; would need a real host
  type-ancestry story (future ADR) — reader features and class refs
  cannot honestly close them.
- `add_watch.cljc`, `remove_watch.cljc` — NOT unblocked: blocked on
  `ref`/`dosync`/`agent`/`send` (their `:default` branches exercise vars,
  refs, and agents). Separate backlog item; neither Decision A nor B is
  the obstacle. (Their `(catch #?(…))` with no matching feature collapsing
  to a malformed `catch` is correct Clojure reading behavior — the JVM
  reads the same file only because `:clj` matches.)

## Consequences

- The `:cljgo`/`:default` contract is now binding and citable; any future
  "just take `:clj`" shortcut must supersede this ADR with evidence.
- cljgo gains `class?` and first-class-but-opaque class-ref values;
  `(def x String)`, class refs as map/set keys and hierarchy tags all
  work. `(pr-str String)` ⇒ `java.lang.String` (matches JVM).
- Deviations, documented: cljgo's `(parents String)` is nil (JVM: type
  ancestry); assert MESSAGE text for derive's tag assert differs slightly
  (cljgo spells the check with `keyword?`/`symbol?` — no `Named`
  interface); `descendants`-on-class throws `ex-info`, not
  `UnsupportedOperationException` (suite only requires *thrown*).
- A `cljgo.classes` namespace appears in `(all-ns)` once a class name has
  been resolved. Names outside the fixed table still fail to resolve —
  fail-closed, no wildcard `java.*` resolution.
- Suite expectation: 217 → 219 (derive, descendants); ancestors/parents/
  add_watch/remove_watch remain red with causes recorded here.
