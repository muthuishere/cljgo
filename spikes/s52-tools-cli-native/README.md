# s52-tools-cli-native — native port of `clojure.tools.cli`

**Spike:** s52-tools-cli-native · **ADR:** 0096 (org.clojure contrib, natively)
· **Date:** 2026-07-27 · **Upstream:** `org.clojure/tools.cli` **1.1.230**
(`clojure/tools/cli.cljc`, EPL-1.0, authors Gareth Jones / Sung Pae / Sean
Corfield).

## Goal

Prove a **faithful, near-verbatim** native port of `clojure.tools.cli` into
cljgo and freeze its behavior against the real JVM library. Nothing here is
integrated into `core/` or `pkg/` — everything lives under this spike dir.

## Starting point (from S50)

S50 measured `tools.cli` as **fully pure — 1 namespace, 1/1 pure** (its only
non-Clojure surface is `#?(:cljs …)` / `#?(:cljr …)` reader-conditional
branches that cljgo skips). This spike confirms that: the port is the upstream
source with the reader conditionals resolved to their `:clj`/`:default` branch
and three small, well-understood adaptations.

## Pure / Java namespace split

| namespace           | verdict            | notes |
|---------------------|--------------------|-------|
| `clojure.tools.cli` | **pure** (1 of 1)  | ports ~verbatim; see the 3 adaptations below |

There is **no Java-backed namespace** to scope out. The library is a single
`.cljc` file over `clojure.string` + `clojure.core` regex/format, all of which
cljgo already supplies (`clojure.string/{join,split,replace,trimr,starts-with?}`,
`re-find`/`re-seq`, `format`, `pr-str`, `with-out-str`).

## What ports verbatim vs. rewritten

**Verbatim** (every fn body unchanged): `make-format`, `tokenize-args`,
`spec-keys`, `compile-spec`, `distinct?*`, `wrap-val`, `default-option-map`,
`missing-errors`, `find-spec`, `pr-join`, the error builders, `validate`,
`parse-value`, `allow-no?`, `neg-flag?`, `parse-optarg`,
`parse-option-tokens`, `make-summary-part`*, `format-lines`,
`required-arguments`, `summarize`, `get-default-options`, `parse-opts`, and the
whole legacy API (`build-doc`, `banner-for`, `name-for`, `flag-for`, `opt?`,
`flag?`, `end-of-args?`, `spec-for`, `default-values-for`, `apply-specs`*,
`switches-for`, `generate-spec`, `normalize-args`, `cli`).

**Adapted** — three changes, each behavior-preserving:

1. **ns form → satellite preamble.** Upstream `(ns clojure.tools.cli
   (:require [clojure.string :as s] #?(:cljs goog.string.format)))` becomes the
   house-style satellite header used by `core/string.cljg` / `core/async.cljg`:
   ```clojure
   (clojure.core/in-ns 'clojure.tools.cli)
   (clojure.core/refer 'clojure.core)
   (clojure.core/require '[clojure.string :as s])
   ```
2. **Reader conditionals resolved & inlined.** All `#?`/`#?@` forms collapse to
   their `:clj`/`:default` branch; the `:cljs` (incl. the `goog.string.format`
   shim) and `:cljr` branches are dropped. cljgo's `clojure.core/format` covers
   the `:cljs`-only `format` shim, so nothing is lost. `(catch #?(:clj Throwable
   …) _)` → `(catch Throwable _)`; `#?(:cljr (pr-str default) :default (str
   default))` → `(str default)`.
3. **`compile-option-specs` `:post` map → explicit `assert`s.** cljgo does not
   bind `%` inside a `:pre`/`:post` conditions map (verified: `unable to resolve
   symbol: %`). The six postconditions (every `:id` set; distinct `:id`s among
   `:default`/`:default-fn`; distinct `:short-opt`/`:long-opt`; never both
   `:assoc-fn` and `:update-fn`) run as explicit `(assert …)` calls on the
   realized result — same predicates, same throw-on-violation.

One further host detail: legacy `cli`'s invalid-argument path threw
`(Exception. …)`; cljgo has no bare Java constructors, so it raises
`(ex-info … {})` with the **identical message text**. Type differs, message
does not.

## Verification

`cljgo run` cannot `load-file` (the var resolves but is unbound in the
`run` context — a harness limitation, not a port issue), so the port was
verified by concatenating `draft-tools_cli.cljg` + a driver into one file and
running it. **All 15 representative behaviors + the legacy `cli` banner are
byte-identical between the cljgo run and the JVM oracle.** See
`draft-conformance-tools.cli.clj` for the frozen vector.

### Known JVM-string dependence (deliberately NOT frozen)

`:parse-fn` exceptions surface as `(str e)` inside the error string, e.g. a
failing `Integer/parseInt` yields `"… java.lang.NumberFormatException: For input
string: \"abc\""` on the JVM. That text is host-specific and would diverge on
cljgo, so no conformance behavior freezes a parse-fn *exception* string; the
frozen validation/parse cases use predictable messages. Integrators should add a
cljgo-oracle'd parse-error case rather than reuse the JVM string.

## Files

- `draft-tools_cli.cljg` — the native port (public API + option keys identical).
- `draft-conformance-tools.cli.clj` — 15 frozen behaviors, oracle-cited.
- `VERDICT.md` — MET, honest scope, residual unknowns.
