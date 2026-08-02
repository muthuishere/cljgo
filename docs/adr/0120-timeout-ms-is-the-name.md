# ADR 0120 — `:timeout-ms` is the name, and a silently-ignored option is the real defect

- Status: accepted
- Date: 2026-08-02
- Relates to: ADR 0087 (`cljg.net.http`), ADR 0015 (diagnostics), issue #192
- Partially deferred: the unknown-opts-key check (see *Not decided here*)

## Context

The toolnexus Clojure port traced a dead HTTP timeout in koine to a key
spelling. cljgo's own stdlib disagrees with itself:

| namespace | key |
|---|---|
| `cljg.net.http` | `:timeout` |
| `bri.web.openapi` | `:timeout` (forwards to the above) |
| `cljg.socket` | `:timeout-ms` |
| `cljg.io` / process | `:timeout-ms` |

koine guessed the majority spelling and hit the one outlier. That is not a
user error — three of four namespaces say `:timeout-ms`, and so does
`cljg.net.http`'s **own Go shim**, whose parameter list reads
`(method url headers body timeout-ms)` (`pkg/bri/net_http.go:78`). `:timeout`
was only ever the surface spelling of one namespace.

**The invisible half is the serious one.** `(:timeout opts default-timeout)`
means an unrecognised key falls through to the 30 s default with no error and
no warning. So the failure has no symptom: the request still succeeds, the
caller's timeout budget has simply evaporated, and **every test passes on fast
hardware** — a server that answers in 5 ms never reaches any timeout, correct
or not. koine's suite was green throughout, and so was ours: we had no test
for the timeout option at all.

## Decision

**1. `:timeout-ms` is the name.** Documented that way in `cljg.net.http` and
`bri.web.openapi`, matching the rest of `cljg.*` and the shim underneath.

**2. `:timeout` stays accepted, indefinitely, as the older alias.** Not
deprecation politeness — a correctness requirement given the second defect.
Removing `:timeout` while unknown keys are ignored would not fail loudly for
existing callers; it would **silently restore the 30 s default**, converting
a working program into the exact bug this ADR is about. The alias cannot be
removed before an unknown-key check exists, and once one exists, removing it
is unnecessary.

Two accepted names for one concept is a real cost, paid knowingly. The
alternative costs more.

## Not decided here — the class fix

The instance is fixed; **the class is not**. Any misspelled opts key in any
`cljg.*` namespace still takes the default in silence.

The fix is to reject unknown opts keys with a coded diagnostic and a
did-you-mean `Fix`. It is not in this ADR because it is a bigger decision than
it looks:

- it is **breaking** — an open map is a legitimate Clojure idiom, and callers
  may be passing extra keys deliberately;
- coded errors are raised via `lang.NewCodedError`, which is Go-side, so a
  Clojure-level check needs either a new shim interned for `cljg.*` or the
  check pushed into Go — a new mechanism either way;
- it needs a new registry code, an explain page, a site row and a lock update.

It is worth doing, and it passes the *simplicity before performance* test on
its own terms: a silently-ignored option is precisely what the diagnostics
doctrine exists to prevent, so this is a **correctness** fix, not a mechanism
added for a benchmark. It is tracked in #192 and wants its own ADR.

## Consequences

- A caller writing `:timeout-ms` against `cljg.net.http` now gets the timeout
  they asked for. A caller writing `:timeout` is unaffected.
- `bri.web.openapi` accepts both and forwards under the canonical name.
- The AOT twin (`pkg/briaot/cljgnethttp`, `pkg/briaot/briopenapi`) is
  regenerated, so interpreted and compiled agree.
- **A test now exists where none did.** `TestCljgNetHTTPTimeoutKey` asserts
  the option *bounds a call that would otherwise outlast it*, for both
  spellings — not merely that the option is accepted, which is the assertion
  that cannot fail when the key is ignored. Confirmed to fail on the old code
  by waiting the server's full 10 s, which is the silent-default behaviour
  made visible.

## Verification

- `pkg/bri/net_http_test.go` — `TestCljgNetHTTPTimeoutKey`, both spellings,
  against a local `httptest` server that hangs for 10 s. Verified failing
  before the change and passing after.
- Found by a consumer, not by us. Recorded because that is the fifth
  consecutive defect of which this is true, and the pattern is the finding.
