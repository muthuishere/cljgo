# ADR 0050 — `tagged-literal` / `reader-conditional` value types + their clojure.core fns

Date: 2026-07-22 · Status: accepted

## Context

The fundamentals audit flagged four clojure.core publics as deferred
"reader + printer" work:

- `tagged-literal` / `tagged-literal?`
- `reader-conditional` / `reader-conditional?`

These are the DATA constructors and predicates over the two reader value
types the JVM exposes as `clojure.lang.TaggedLiteral` and
`clojure.lang.ReaderConditional`. They are what Clojure's data readers and
`read` / `read-string` with `{:read-cond :preserve}` produce, and they are
plain values otherwise — buildable directly, printable, keyword-lookable,
and `=`-comparable.

### Evidence gathered (oracle = JVM Clojure 1.12.5, `clojure` CLI, 2026-07-22)

TaggedLiteral:
- `(tagged-literal 'foo 42)` → prints readably as `#foo 42`;
  `(:tag …)` ⇒ `foo`, `(:form …)` ⇒ `42`; `(get … :nope :DEF)` ⇒ `:DEF`.
- `(tagged-literal? (tagged-literal 'foo 42))` ⇒ true; `(tagged-literal? 42)`
  ⇒ false.
- `(= (tagged-literal 'foo 42) (tagged-literal 'foo 42))` ⇒ true; differing
  form ⇒ false. `(pr-str (tagged-literal 'my/tag [1 2 3]))` ⇒ `#my/tag [1 2 3]`;
  `(tagged-literal 'foo nil)` ⇒ `#foo nil`.
- It is NOT a collection: `(assoc tl :k v)` throws (ClassCastException),
  `(count tl)` throws, `(keys tl)` throws "Don't know how to create ISeq".
  Only `ILookup` (keyword lookup) and `=` apply.
- `(str tl)` on the JVM yields the identity form
  (`clojure.lang.TaggedLiteral@<hash>`) — non-deterministic, not frozen.

ReaderConditional:
- `(reader-conditional '(:clj 1 :cljs 2) false)` → prints `#?(:clj 1 :cljs 2)`;
  `(:form …)` ⇒ `(:clj 1 :cljs 2)`, `(:splicing? …)` ⇒ `false`.
- `splicing?` true → prints `#?@(…)`. `splicing?` is coerced to boolean
  (`(:splicing? (reader-conditional '(:clj 1) 5))` ⇒ `true`).
- `(reader-conditional? …)` true only for the type; `=` is same-form +
  same-splicing.

Reader `:read-cond :preserve` (JVM):
- `(read-string {:read-cond :preserve} "#?(:clj 1 :cljs 2)")` ⇒ a
  `ReaderConditional` (prints `#?(:clj 1 :cljs 2)`); nested and `#?@` bodies
  are preserved in place, and their inner forms stay unread lists.

## Decision

### A. Two new runtime value types + their four clojure.core publics

`pkg/lang/taggedliteral.go` (a cljgo extension, NOT vendored Glojure — no EPL
header) adds `*TaggedLiteral{Tag, Form}` and `*ReaderConditional{Form,
Splicing}`, each implementing:

- `ILookup` — `:tag`/`:form` for TaggedLiteral, `:form`/`:splicing?` for
  ReaderConditional, with a default for unknown keys (so `(:tag x)`,
  `(get x :form)`, `(:splicing? x)` all work; unknown ⇒ nil/default).
- `Equiv` — value equality: same concrete type, `Equiv` on each field
  (pointer types, so the `a == b` fast path in `lang.Equiv` is identity-safe
  and falls through to `Equiver`, exactly like the ADR 0014 Result/Option
  routing but without an incomparable-payload hazard).

`pkg/lang/strconv.go` `Print` gains two cases (after `*regexp.Regexp`, before
the `ToString` fallback): `#tag form` and `#?(…)` / `#?@(…)`. The live print
path is this Go switch — `pr-on` / `print-initialized` are never bound, so no
Clojure-level `print-method` is involved.

`pkg/corelib/misc_builtins.go` registers the four publics as Go-native
builtins (ADR 0043): `tagged-literal` (2-arg), `tagged-literal?` (1-arg),
`reader-conditional` (2-arg, `splicing?` via `BooleanCast`),
`reader-conditional?` (1-arg). All are real clojure.core names — additive,
precedence-safe (CLAUDE.md precedence principle), no shadowing.

### B. Reader `:read-cond :preserve` integration is scoped OUT (follow-up)

cljgo's Phase-2 reader (ADR 0036, `pkg/reader/readcond.go`) always SELECTS or
ELIDES reader conditionals during normal file/REPL reading — it has no
`:read-cond` mode flag and does not construct `ReaderConditional` /
`TaggedLiteral` values. `read-string`'s opts arity already documents that it
honors `:eof` only, not `:read-cond`/`:features` (misc_builtins.go).

Wiring a `:preserve` mode is a genuinely separate reader change — a mode
threaded through the `Reader` struct and `ReadString` options, a
non-selecting branch in `readConditional`, a value-constructing branch in
`readTaggedLiteral`, and `read-string`/`read` opt plumbing — with its own
conformance surface (nested preservation, `#?@` splicing preservation, tagged
literals). It is deliberately deferred: this ADR delivers the four DATA
publics (the audit's actual scope) and the value types they build, which are
also the exact types a future `:preserve` mode will emit. That mode is a
clean follow-up on top of these types, not a prerequisite for them.

## Consequences

- cljgo gains `tagged-literal`, `tagged-literal?`, `reader-conditional`,
  `reader-conditional?` and first-class `#tag form` / `#?(…)` values with
  keyword lookup, `=`, and readable printing — matching JVM 1.12.5 on every
  oracle-checked behavior.
- Deviation, documented: `(str tagged-literal)` on the JVM is the
  non-deterministic identity string; cljgo's `str`/`ToString` falls through to
  the readable `#tag form` form. Not frozen in conformance (JVM's is
  unreproducible); no user-visible correctness impact.
- Deviation: these values are not collections on either platform — `assoc` /
  `count` / `keys` throw; cljgo matches (they implement `ILookup` only).
- Reader `:read-cond :preserve` still does not produce these values; recorded
  as a follow-up (Decision B). Any change there must supersede this note.
- Acceptance: dual-harness, oracle-verified —
  `conformance/tests/tagged-literal.clj` +
  `conformance/tests/reader-conditional.clj`. Surgery logged in
  `pkg/lang/PROVENANCE.md`.
