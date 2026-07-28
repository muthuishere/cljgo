# ADR 0105 — `cljx.test`: Bun-flavoured first-class testing (mocks, spies, output capture built in)

Date: 2026-07-28 · Status: **proposed** (owner-directed: *"cljgo tests should
be first class, the fruitful flavour of bun.test — mock, spies and all built
in, even println spies by default … anyone who comes here will know how easy
to test. We will call it cljx.test."*).

## Context

ADR 0012 already made testing a toolchain primitive: `cljgo test` (zero-config
discovery, colocated tests, dual REPL-vs-binary harness) and a `clojure.test`
satellite (`deftest`/`is`/`testing`/`use-fixtures`) so JVM idioms port
unchanged. What's missing is the **modern ergonomic layer** that makes Bun,
Vitest and Jest feel effortless: mocks and spies as one-liners, automatic
output capture, rich matchers, lifecycle hooks — today a cljgo user hand-rolls
`with-redefs` + `binding *out*` + string surgery for every stub and every
"did it print?" check.

Naming: the natural name `cljg.test` would sit in the mechanism tier, but the
owner names it **`cljx.test`** — introducing the **`cljx.*` prefix — *clj extensions* (owner's
reading, 2026-07-28): the developer-experience tier** (things that wrap the *practice* of programming —
testing today; possibly bench/lint/fmt later). It extends ADR 0085's taxonomy:
`clojure.*` language · `cljg.*` mechanism · `bri.*` framework · `cljx.*`
developer experience. Per the precedence principle, `clojure.test` is
untouched and remains fully supported; `cljx.test` is built ON it (a
`cljx.test` deftest IS a clojure.test test — one runner, one report, dual
harness for free).

## Decision

One namespace, `cljx.test`, batteries-included:

1. **Tests & suites** — `deftest`-compatible plus Bun-style grouping:
   `(describe "cache" (it "expires" …))` sugar over `deftest`/`testing`;
   `before-all`/`before-each`/`after-each`/`after-all` lifecycle hooks
   (fixtures sugar).
2. **Expect matchers** — `(expect actual :to-be x)`, `:to-equal`,
   `:to-contain`, `:to-throw`, `:to-be-nil`, `:to-match #"re"`, negation
   `:not-to-…` — thin over `is` so failure reporting stays unified, but
   failure messages name expected-vs-found per the error doctrine.
3. **Mocks** — `(mock)` / `(mock (fn [x] …))` / `(mock {:returns v})` creates
   a callable that records every call (args, order, results); inspectors
   `(calls m)`, `(called? m)`, `(called-with? m & args)`, `(call-count m)`.
4. **Spies** — `(spy #'ns/f)` wraps an EXISTING var: passes through to the
   real fn while recording calls; `(stub #'ns/f replacement)` replaces it.
   Both scoped: inside `(with-spies [f (spy #'ns/f)] …)` or auto-restored
   per-test — never leaks across tests. (Implementation: `with-redefs`
   machinery, already proven live under the sealed-core dirty-flag, ADR 0066.)
5. **Output capture BY DEFAULT (the owner's explicit want)** — every
   `cljx.test` test transparently captures `*out*`/`*err*`; `(printed)`
   returns captured lines, `(printed? "substr"|#"re")` asserts on them,
   captured output replays on failure for context. No `binding` boilerplate —
   the println spy is just there.
6. **Time & randomness control** — `(with-frozen-time t …)` freezing
   `cljg.date/now`/`nano-time`, `(advance! ms)`; seeded `(with-seed n …)`
   where cljgo controls the source. (Bun's fake timers, adapted.)
7. **Snapshots (follow-on, reserved)** — `(expect x :to-match-snapshot)`
   storing EDN snapshots beside tests; `cljgo test --update-snapshots`.

Runner integration: `cljgo test` runs cljx.test tests with zero config (they
ARE clojure.test tests); dual harness (`--compiled`, `--both`) applies
unchanged — **mocks/spies/capture must behave identically interpreted and
compiled** (the unforgivable-divergence bar applies to the testing library
itself). Registered lazily in `bri.Specs()` like every satellite; a binary
that never requires it pays zero bytes.

Templates teach it: `cljgo new` test files use cljx.test idioms (describe/
expect/mock) so the first thing a newcomer sees is the easy path — this is a
marketing surface (ADR 0104's Book gets a "Testing" chapter built on it).

## Consequences

- Testing becomes a headline feature ("bun:test ergonomics, Clojure soul")
  instead of a ported JVM API; `clojure.test` compatibility is unaffected.
- A new taxonomy tier `cljx.*` exists and needs an ADR 0085 forward-pointer;
  the bar for adding more `cljx.*` members is an ADR each.
- Spy/stub scoping must be leak-proof across parallel tests — the runner
  serializes tests that hold live spies (or spies are test-local only);
  design settles this before implementation.
- Compiled-mode spies rely on var indirection surviving AOT for spied vars —
  the design must verify against the sealed-core fast path (ADR 0066's
  dirty-flag makes redefs visible; conformance must prove it in --both mode).

## Process

Spike first (`spikes/s66-cljx-test/`): mock/spy/capture semantics in BOTH
harnesses — the compiled-mode spy question is the only real risk. Then
openspec (this supersedes the dormant `testing-first-class` change's
remaining scope or folds into it), then implement: core surface (describe/it/
expect/mock/spy/capture) → time control → snapshots. QA-sweep findings
(2026-07-28 agent run) feed the pain-point checklist the design must erase.
