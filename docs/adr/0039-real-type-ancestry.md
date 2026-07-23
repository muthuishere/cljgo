# ADR 0039 — our types have real ancestry; opaque class refs get one flattened Object super

Date: 2026-07-17 · Status: accepted · Extends ADR 0036 (addendum: supersedes
its "(parents String) is nil" consequence; everything else in 0036 stands)

## Context

ADR 0036 landed class refs but deliberately left `ancestors.cljc` /
`parents.cljc` red: past the class-ref stage they trip on the JVM's
GENERATED protocol class names used as values
(`clojure.core_test.ancestors.TestAncestorsProtocol` — namespace dashes
munged to underscores), and then on type-inheritance assertions in their
`:default` (portable) branches. 0036 said closing them "would need a real
host type-ancestry story (future ADR)". This is that ADR.

The key realization: most of what those `:default` branches assert is NOT
JVM-specific fabrication — it is knowledge cljgo actually has.

- A defrecord/deftype's protocol set is real: the defining form declares
  it. On the JVM the generated class implements the protocol's interface,
  so `(parents TheRecordClass)` contains it and `(isa? RecordClass P-class)`
  is true.
- A record's runtime interfaces are real: pkg/lang's `*Record` genuinely
  implements Associative/IPersistentMap/Counted/Seqable/IObj/IMeta/
  IPersistentCollection (compile-time-asserted in
  pkg/eval/protocols_ancestry_test.go), which are exactly the table
  members of the JVM record class's ancestry.
- "Every value is an Object" is already cljgo semantics:
  `classNameMatchesValue` (ADR 0026) answers `(instance? Object x)` true
  for every non-nil x. Reporting Object among a concrete class's supers
  states the same fact through the hierarchy fns.

Oracle evidence (JVM Clojure 1.12.5, `clojure` CLI, 2026-07-17 scratch
run): for `(defprotocol P) (defrecord R [] P) (deftype T [] P)` —
`(parents R)` ∩ our table = `#{P Object IPersistentMap IRecord IObj}`;
`(ancestors R)` adds `#{Associative Counted Seqable IMeta
IPersistentCollection}`; `(parents T)` = `(ancestors T)` =
`#{P IType Object}`; `(ancestors P)`/`(parents P)`/`(descendants P)` on
the protocol MAP are all nil; `(satisfies? P (->R))` is true with zero
method forms; `(isa? R P)` with the MAP is false, with the CLASS true;
`(class? P-map)` false, `(class? P-class)` true; hierarchy relationships
derived from a SUPER flow into `ancestors` (deriving
`clojure.lang.Associative` in h puts the derived tag in
`(ancestors h R)`); `extend-type` does NOT alter class ancestry;
`(parents Object)` nil; Object ∈ `(parents String)` and ∈
`(ancestors clojure.lang.PersistentHashSet)`; interfaces never have
Object among supers (`(supers clojure.lang.ISeq)` =
`#{Seqable IPersistentCollection}`).

## Decision

### A. Generated class names of OUR types resolve

A dotted symbol spelling the JVM-generated class name of a cljgo
defprotocol / defrecord / deftype (`my.name_space.TheName`, namespace
munged `-`→`_`) resolves — only after every normal lookup AND the ADR
0036 class-ref table miss — to the defining var itself
(`typeClassVar`, pkg/eval/class_refs.go). Fail-closed: the prefix must
name a LOADED namespace (as written or demunged) holding that var, and a
var bound to anything other than a `*Protocol`/`*TypeMarker` does not
resolve (an interned-but-unbound var is accepted because resolution runs
at analysis time, before defs earlier in the same top-level form have
evaluated). The protocol VALUE thus also stands in for the generated
INTERFACE — the same one-value conflation ADR 0026 documents for
designators.

### B. Our types have real ancestry

`TypeMarker` now carries its kind ("record"/"type") and the protocol
values DECLARED in the defining form (`-type-marker` gained both
arguments; `extend-type` additions deliberately excluded — the JVM's
`extend` never alters the class). Marker ancestry, exposed through new
`-type-bases` / `-type-supers` builtins:

- record bases: declared protocols + Object, IRecord, IPersistentMap,
  IObj; record supers add Associative, Counted, Seqable, IMeta,
  IPersistentCollection — each a real interface of `*Record`, each in the
  JVM record class's bases/ancestors respectively (oracle above).
- deftype bases = supers: declared protocols + IType, Object.

`parents` / `ancestors` / `isa?` (core/hierarchies.cljg) run
clojure.core's own bases/supers branches against these sets: parents
unions bases with the derived relationship, ancestors unions supers AND
each super's hierarchy-derived ancestors (so deriving a protocol's class
or an interface ref flows into its implementors), isa? gains the
isAssignableFrom-equivalent supers check. Declaring a METHOD-LESS
protocol now also registers the type with it, so `satisfies?` answers
true (JVM-faithful; previously the macros dropped it entirely).

### C. Class refs get exactly one flattened super: Object

A CONCRETE well-known class ref reports `#{Object}` as its bases and
supers; Object itself and interface refs (Comparable, CharSequence, the
clojure.lang `I*`/Named/Sequential/... names) report none — on the JVM
every concrete class has Object among its ancestors and no interface
does. This supersedes ADR 0036's "(parents String) is nil". It does NOT
reopen fabrication: no intermediate JVM superclass graph (Number,
Throwable chains, APersistentSet, String's CharSequence/Comparable, ...)
is encoded — cljgo reports only the one super it can vouch for from its
own semantics, and the table stays fail-closed.

### The protocol map/class conflation, pinned

cljgo's single protocol value follows the JVM's protocol-MAP reading
wherever the hierarchy fns see it directly — `class?` false,
`(ancestors P)`/`(descendants P)` nil, `(derive P ::x)` asserts — and the
INTERFACE reading inside a type's ancestry sets. Known deviations, both
sides oracle-checked: `(isa? R P)` is true here (JVM: false for the map,
true for the class); deriving the protocol's class as a hierarchy tag is
inexpressible (derive a ClassRef the type implements instead).

## Consequences

- `ancestors.cljc` and `parents.cljc` pass; suite 234 → 236/242.
  `descendants.cljc`/`derive.cljc` keep their protocol-MAP behaviors.
- Conformance: `type-ancestry.clj` and `type-ancestry-munged.clj`
  (eval-harness waivers per ADR 0036's deferred class-name emission; both
  oracle-verified file-for-file against 1.12.5), dual-harness
  `satisfies-declared-protocol.clj`; `class-refs-hierarchy.clj` updated
  to the new `(parents String)` ⇒ `#{java.lang.Object}`.
- The honesty contract is build-enforced: if `*Record` ever stops
  implementing a claimed interface, compile-time assertions in
  pkg/eval/protocols_ancestry_test.go fail.
- Parents of a concrete class whose JVM direct superclass is not Object
  (Exception → Throwable, Long → Number) report `#{Object}` here — the
  intermediate superclass is unencoded, so the set is flattened, not
  false: it only ever states what cljgo's own `instance?` already claims.

## Addendum (2026-07-23) — typed builtin exception classes in catch and instance?

Before this addendum only Throwable/Exception/RuntimeException/ExceptionInfo
matched in catch position, so `(try (/ 1 0) (catch ArithmeticException e …))`
let the throw escape — any ported JVM code catching a typed builtin broke.

**Decision.** The standard JVM exception-class names resolve (class-ref
table, `add("java.lang", …)`) and MATCH in catch position and in
`instance?`, each mapped to the cljgo error value that corresponds
semantically, with the real JVM ancestry honored as MATCHING semantics
(oracle 1.12.5, 2026-07-23, `.getSuperclass` chain):

| class name | cljgo error value (pkg/lang) | JVM superclass |
|---|---|---|
| ArithmeticException | `ArithmeticError` (divide by zero, overflow) | RuntimeException |
| ClassCastException | `ClassCastError` (Ops conversion on a non-number, compare on incomparables, string casts) | RuntimeException |
| NullPointerException | `NullPointerError` (Ops conversion on nil, nil where a string is required) | RuntimeException |
| IndexOutOfBoundsException | `IndexOutOfBoundsError` (nth, subs — StringIndexOutOfBoundsException folds in) | RuntimeException |
| IllegalArgumentException | `IllegalArgumentError` (ISeq conversion, assoc on a list, …) | RuntimeException |
| NumberFormatException | `NumberFormatError` ("Infinite or NaN") | IllegalArgumentException |
| IllegalStateException | `IllegalStateError` | RuntimeException |
| UnsupportedOperationException | `UnsupportedOperationError` | RuntimeException |
| clojure.lang.ArityException | `ArityError` / eval's `arityError` | IllegalArgumentException |
| clojure.lang.ExceptionInfo | anything wrapping `lang.IExceptionInfo` | RuntimeException |

One table (`throwableMatches`, pkg/corelib/exceptions.go) serves BOTH legs
— the tree-walk evaluator and the emitted `rt.CatchMatches` — and
`instance?` (`classNameMatchesValue` falls through to it for error
values), so REPL/binary parity holds by construction. Subclass edges are
encoded once, as `Is` methods on the error types themselves
(`ArityError`/`arityError`/`NumberFormatError` → IllegalArgumentError),
and reached through `errors.Is`, which also unwraps `EvalError`. The
three catch-all names still match every thrown value (cljgo v0 treats all
throws as unchecked). Raise sites were re-typed message-for-message
(byte-stable strings, diagnostic codes G5001/G5003/G5004 preserved on the
new types), so no frozen conformance output changed.

**What this does NOT change:** `parents`/`supers` on these class refs
still report the flattened `#{Object}` of §C — the Throwable chain is
encoded only as catch/instance? matching semantics, not as a reified
superclass graph. Fail-closed stands: names outside the table still never
resolve and never match.

Conformance: `catch-typed-builtins.clj`, `catch-hierarchy.clj`,
`catch-miss-falls-through.clj`, `instance-exception-classes.clj` (all
dual-harness, oracle-run file-for-file); `divide-by-zero-messages.clj`
and `numeric-bigint-from-double.clj` tightened from their catch-all
workarounds to the typed classes the JVM throws.
