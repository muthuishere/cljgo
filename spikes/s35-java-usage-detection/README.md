# Spike S35 — Can cljgo reliably detect that Clojure source uses JAVA interop?

Opened 2026-07-22. Feeds **ADR 0054** (purity validation — not yet
written). Follows ADR 0010 (Go interop surface), ADR 0026 (`instance?`
class position is syntax), ADR 0036 (reader features + interned class
refs), ADR 0052 §6/§6a (dependency purity + the unforgivable
silent-`nil` host divergence).

## Context — the verified current state

cljgo runs Clojure on a Go host with **no JVM**. Its host-interop surface
(ADR 0010, verified in `pkg/analyzer/analyzer.go` + `pkg/eval/host.go`):

- **Namespaced call `Ns/member`** (`(str/upper-case s)`, `(os/Open! p)`) —
  `parseHostCall` → `resolveHost(op)`. Resolves to a Go package member
  **only if `Ns` is a `require-go` alias in the current namespace** (or a
  seed-registry package). Otherwise `ok=false` and the analyzer falls
  through to ordinary invoke → Clojure var resolution → resolution error.
  Precedence: a Clojure ns/alias always wins.
- **Dot method `(.method recv args…)`** — `parseHostMethod`. **NOT resolved
  at analysis time** (`analyzer.go:984`: "Host-independent: the receiver's
  type is only known at runtime for v0, so no ResolveHost is consulted").
  Becomes `OpHostMethod`, dispatched reflectively via `corelib.CallGoMethod`
  in BOTH legs. `(.toUpperCase s)` (Java) and `(.Do client req)` (Go) are
  **syntactically indistinguishable** and both reach the same node.
- **Field `(.-Field recv)`** — same host-independent story.
- **Constructor `Type.` / `(Type. …)`** — `parseHostNew` → `resolveHostType`,
  gated on the type registry / require-go alias, else falls through to a
  Clojure `->Type` ctor.
- **Bare class symbol** `java.util.UUID` as a value — ADR 0036: resolves to
  an interned opaque `ClassRef` **only** for a fixed table of well-known
  JVM names; anything else fails to resolve (fail-closed, no wildcard
  `java.*`).

So on a Go host there is no Java. The question is whether cljgo can *tell*
that a given form was *written for* the JVM, so a purity validator can
reject it up front instead of letting it fail late (or worse — ADR 0052
§6a — return `nil` silently).

## The one question

**Given Clojure source, is there a sound, low-false-positive predicate
`uses-java?(form | namespace)` that a purity validator (ADR 0054) can call
at analysis time on a Go host that has no Java — reliably distinguishing
JAVA interop from GO interop and from pure Clojure?**

The suspected complication, to be resolved not assumed: Java and Go interop
**share syntax** (`(.method obj)`, `Ns/sym`, class-ish symbols). If they
differ only by whether the target *resolves* to a known Go binding, then
"Java taint" may really mean "host interop whose target is not backed by a
`require-go`/Go binding or a cljgo var." This spike determines whether that
reframing is correct and sufficient, or whether Java has distinguishing
*surface markers* a purely syntactic scan can catch.

## Exit criterion (written before any code, per ADR 0027)

Build a **labeled corpus** of ≥ 30 forms in three classes — `pure`,
`go-interop`, `java-interop` — each snippet's TRUE class confirmed by
running it against the real `clojure` CLI (the oracle for "is this really
Java"). The criterion is met iff:

1. A prototype predicate `uses-java?` classifies every **obvious JVM
   marker** (`java.*`/`javax.*` package prefixes, `(import '[java.x Y])`,
   `(new java…)`, `clojure.java.*` requires, bare JVM classes
   `System`/`Math`/`Thread`/`String`/…) with **zero false negatives** on
   the java-interop class.
2. Its **false-positive rate on the go-interop and pure classes is stated
   and bounded** — every false positive enumerated with its cause.
3. The **residual ambiguous set** — host-neutral interop that pure syntax
   cannot classify (chiefly bare `(.method recv)` and `(.-field recv)`) —
   is enumerated, with *how each is resolved* (resolution-based fallback,
   or declared genuinely undecidable), and the consequence for ADR 0054
   stated plainly.
4. For every corpus snippet, MEASURE what cljgo actually does today
   (reads / analyzes / errors / silently returns nil / panics), captured
   as real output, so the ADR knows what signal exists.

Closing "no" is a legitimate outcome: if reliable Java detection is NOT
achievable on a Go host, that changes ADR 0054 and must be surfaced.

## Method

Throwaway prototype in `proto/` — a Go program that shells the real
`cljgo` binary (interpreter) on each corpus snippet and records the
outcome, plus a standalone syntactic + resolution-based `uses-java?`
predicate run over the same corpus. `pkg/` is READ, never modified. The
`clojure` CLI is the oracle for snippet MEANING only. All VERDICT claims
backed by captured output in `results/`.

## Results

See `VERDICT.md`.
