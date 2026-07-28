# adr-0105-cljx-test — Bun-flavoured first-class testing

## Why

ADR 0105, de-risked by spike s66 (MET, dual-harness identical). Today a cljgo
user hand-rolls `with-redefs` + `binding *out*` + string surgery for every stub
and every "did it print?" check. The 2026-07-28 QA sweep also found the runner
lies: a compiled test binary **exits 0 when tests fail**.

## What Changes

1. **New namespace `cljx.test`** — pure Clojure, lazy, registered in
   `bri.Specs()` (no Go shim, no dependency, zero bytes when unused). Built ON
   `clojure.test` so a cljx test IS a clojure.test test: one runner, one
   report, dual harness free, `clojure.test` untouched (precedence principle).
   Introduces the **`cljx.*` developer-experience tier** (4th tier, extends
   ADR 0085) — needs a forward-pointer in 0085.
2. **Surface** (spike-proven shapes):
   - grouping: `describe` / `it` over `deftest`/`testing`; hooks
     `before-each` / `after-each` / `before-all` / `after-all`.
   - matchers: `expect` with `:to-be` `:to-equal` `:to-contain` `:to-throw`
     `:to-be-nil` `:to-match`, plus `:not-to-*`. Delegates to `is` so failures
     report through one path; messages name expected-vs-found (error doctrine).
   - mocks: `(mock)` / `(mock f)` / `(mock {:returns v})`, inspectors `calls`
     `call-count` `called?` `called-with?`.
   - spies/stubs: `(spy #'ns/f)` records + forwards; `(stub #'ns/f impl)`
     replaces; `with-spies` scopes and auto-restores. Over `with-redefs`.
   - **output capture by default**: every cljx test captures `*out*`/`*err*`;
     `(printed)` / `(printed? "s"|#"re")`; captured output replays on failure.
   - time control: `with-frozen-time` / `advance!` over `cljg.date`.
3. **Runner fixes (correctness, not sugar)** — the test summary MUST set the
   process exit code in compiled binaries as it already does interpreted, and
   `cljgo test --compiled` lands so compiled semantics are testable at all.
4. **fn-metadata conformance gap** — `(with-meta (fn [] 1) {..})` throws on
   cljgo but works on JVM Clojure (s66, oracle-verified). Fix it, or if the fix
   is out of scope here, freeze the divergence in a conformance test and have
   `cljx.test` use the proven registry workaround (fns are valid map keys).

## Impact

- New Specs() row + `core/cljx/test.cljg` + embed; regenerated briaot twin.
- `cmd/cljgo` test runner: exit-code wiring + `--compiled` flag.
- Templates' generated tests move to cljx.test idioms (the first thing a
  newcomer sees) — bri/lib/cli/web template tests updated.
- Docs: a "Testing" chapter in the book (ADR 0104) built on this.
