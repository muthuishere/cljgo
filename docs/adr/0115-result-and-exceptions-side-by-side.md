# ADR 0115 -- ok/err (and the rest of ADR 0014 Result/Option family) move out of clojure.core

Date: 2026-07-31 -- Status: accepted (owner-directed, issue #171)

## Context

cljgo clojure.core carried 16 names with no JVM Clojure counterpart: ok
err ok? err? just just? none none? option? result? unwrap unwrap-or
map-ok map-err and-then let? (ADR 0014 Result/Option primitives, plus the
let? railway-binding macro built on them). Because they lived in
clojure.core, they were auto-referred into every namespace exactly like
map or reduce -- so legitimate user code defining its own (defn ok ...)
/ (defn err ...) tripped a shadow WARNING on cljgo that could never fire
on the JVM, since the JVM has neither var. Verified against the real
clojure CLI 1.12.5 (2026-07-31): (resolve (quote clojure.core/ok)) is
nil on the JVM, a live var on cljgo.

This is the precedence principle (CLAUDE.md) read in its other
direction: an addition may not shadow or change the semantics of
anything in clojure.core, and the flip side is cljgo must not put NEW
names into clojure.core where the JVM has none -- doing so makes
JVM-legal code break (or silently diverge) the moment it runs on cljgo.

## Decision

Core parity with JVM Clojure is an adoption decision, not a taste
preference. The closer cljgo clojure.core stays to the JVM one, the more
existing Clojure code and habits move over unchanged; every cljgo-only
core name is a small, compounding tax against that, and #171 is proof
the tax is already being paid for real (a real Clojure port hit this
exact collision, see the issue). A Go-flavoured ok/err convention
spreading through user code on the strength of it being auto-available
would be a much larger version of the same tax, pulling people away from
try/catch/throw -- which stays THE idiom for error handling on cljgo,
exactly as on the JVM.

Therefore: ok/err and the rest of the ADR 0014 family move out of
clojure.core into their own namespace, cljx.meta, and are no longer
auto-referred anywhere. A project that wants them requires and refers
cljx.meta explicitly, same as any other library -- never mentioned in a
getting-started path as THE cljgo way to handle errors.

Namespace choice: ADR 0014 never committed to a placement (it originally
put these straight into clojure.core), so this was genuinely open.
cljx.meta is deliberately the LEAST committal option available under the
existing taxonomy (cljg.* = Go-host mechanism, cljx.* = DX addition,
bri.* = app framework) -- a parking spot chosen to be easy to move
later, not a considered permanent home. Implementation:
pkg/corelib/builtins.go interns the 15 Go-builtin members directly into
cljx.meta instead of clojure.core; core/core.clj switches *ns* to
cljx.meta (referring clojure.core first) to define let?, then switches
back -- both legs (interpreter and AOT-compiled binary) share the
identical registration code, so the relocation is byte-identical in
both.

No interop bridge. Result/Option already composes with exceptions via
unwrap (throws an ex-info on err/none) -- the seam already exists, so
nothing new was added.

## Consequences

- A project relying on ok/err/etc. being available unqualified in every
  namespace must add (require (quote cljx.meta)) (refer (quote
  cljx.meta)) (or an explicit :refer list) wherever it uses them. The 6
  conformance/tests/result-*.clj fixtures were updated accordingly.
- try/catch/throw/ex-info remain unchanged and remain THE default
  error-handling story on cljgo.

## Open question (owner-gated, NOT decided here)

Whether the ADR 0014 Result/Option family survives long-term at all,
whether its ergonomics are instead expressed some other way through
try/catch, and where it permanently lives, is explicitly UNDECIDED. This
ADR only fixes the precedence violation (#171); it does not settle
Result future. Revisit when the owner has an opinion.

## Out of scope (found during the #171 sweep, not acted on)

A full clojure.core name-parity sweep (cljgo vs JVM Clojure 1.12.5, both
via the real clojure CLI) turned up, beyond the 16 moved here:

- 53 dash-prefixed internal helpers (-all-seqs, -defmulti, -reify,
  -satisfies?, ...) are public clojure.core vars with no :private
  metadata -- almost certainly meant to be private and simply missing
  the tag. Cheap parity win, not attempted here.
- nan? exists ALONGSIDE the JVM NaN? -- a lowercase alias for an
  existing core name is a rename in disguise, which the precedence
  principle also forbids. Not touched here.
- 5 more: *cljgo-version*, cljgo-version, require-go, lazy-seq*, pr-on,
  print-initialized -- host/version/tooling surface with no JVM
  analogue; left as-is (no evidence they cause collisions in practice).
- 47 real JVM clojure.core names cljgo still lacks (the other direction
  of the parity gap) -- unrelated to #171, not enumerated here.

The owner is separately building a core-parity ratchet test to freeze
both directions; this ADR does not duplicate that effort.
