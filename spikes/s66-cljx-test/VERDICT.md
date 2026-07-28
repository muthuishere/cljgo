# s66 — cljx.test feasibility (ADR 0105)

**Verdict: MET.** The whole Bun-flavoured surface (mock, spy, stub, output
capture) is buildable in **pure Clojure over existing cljgo semantics** — no
Go shim needed — and the ADR's one real risk is closed.

## The risk, closed

ADR 0105 flagged: *do spies/mocks behave the same in a compiled binary as in
the interpreter?* (var indirection surviving AOT under the sealed-core fast
path, ADR 0066.)

**Answer: yes, byte-identically.** Both the primitive probe and the full
12-assertion prototype produce output that `diff`s clean between
`cljgo run` and a compiled binary:

```
cljx.test prototype: 12/12 passed      # interpreted
cljx.test prototype: 12/12 passed      # compiled — diff: no differences
```

Proven identical in both harnesses: `with-redefs` stubbing, spy
pass-through + call recording, `with-out-str` capture (including composed
with `with-redefs`), var metadata reads, dynamic `binding`, atoms.

## Findings

1. **BUG — cljgo cannot attach metadata to a function (JVM divergence).**
   `(with-meta (fn [] 1) {:tag :mock})` throws
   `error: value of type *eval.evalFn can't have metadata`.
   JVM Clojure 1.12.5 (verified via the `clojure` CLI) returns the fn with
   its metadata: `[1 {:tag :mock}]`. Functions implement `IObj` on the JVM.
   → Needs a conformance test + fix; until then the library must not rely
   on fn metadata. **Not a blocker for cljx.test** (workaround below), but a
   genuine conformance gap that will bite other ports.
2. **Workaround proven:** a registry `atom` keyed by the fn object — cljgo
   compares fns by identity, so functions work as map keys (verified).
   The prototype's `mock`/`calls` use this and pass in both harnesses.
   *For the real library:* prefer scoping the registry per-test (clear after
   each test) so long-running REPL sessions don't accumulate mocks — a
   global registry is a slow leak. Alternative: a sentinel-arg protocol
   (`(m :cljx/calls)`), no registry at all.
3. **Runner gap (from the 2026-07-28 QA sweep, still true):** a compiled
   test binary **exits 0 even when tests fail** — CI would go green on red.
   Interpreted `cljgo test` exits 1 correctly. `cljx.test`'s runner must
   wire the summary into the process exit code on BOTH paths; there is also
   no `cljgo test --compiled` yet.
4. `clojure.string/includes?` is the portable literal-substring check for
   `printed?` — `java.util.regex.Pattern/quote` does not exist on the Go
   host.

## What the prototype demonstrates

`prototype.cljg` (runs standalone, 12/12 both harnesses):

- `(mock)` / `(mock f)` / `(mock {:returns v})` + `calls` / `call-count` /
  `called?` / `called-with?`
- `spy-on` — records **and** forwards to the real fn
- stubbing via `with-redefs`, correctly restored afterwards
- `capturing` → `{:value v :printed s}` and `printed?` with string or regex
- stub + capture composing

## Recommendation

**GO.** Build `cljx.test` as a pure-Clojure lazy namespace registered in
`bri.Specs()` (no Go shim, no dependency, zero bytes when unused). Land the
fn-metadata fix and the exit-code fix alongside — the first is a conformance
bug, the second is a correctness bug that makes CI lie.
