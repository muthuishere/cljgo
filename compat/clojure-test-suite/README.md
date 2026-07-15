# clojure-test-suite compliance (ADR 0022, design/08 §5)

cljgo is measured against the external jank
[`clojure-test-suite`](https://github.com/jank-lang/clojure-test-suite) — the
north-star clojure.core compatibility metric. The suite is **not** vendored;
point the runner at a local clone.

## Run

```
cljgo suite --dir ../clojure-test-suite            # or set $CLJGO_SUITE_DIR
cljgo suite --dir ../clojure-test-suite \
  --json compat/clojure-test-suite/scoreboard.json \
  --edn  compat/clojure-test-suite/scoreboard.edn
```

Each `test/**/*.cljc` file loads into a shared evaluator; `clojure.test`
runs that file's namespace; the per-file `{:pass :fail :error :skipped}`
outcome lands in the scoreboard.

- **skipped** — the var under test is unimplemented, so the suite's
  `when-var-exists` gate elided the file body (cljgo's
  `clojure.core-test.portability` shim + `resolve`).
- **error** — a form failed to load (often a reference to *another*
  unimplemented core fn) or an assertion threw.
- **fail** — an assertion returned false.
- **pass** — tests ran with no failures or errors.

## Baseline (2026-07-15, Batch 0)

242 test files: **34 pass · 8 fail · 82 error · 118 skipped**.
North-star metric (files passing / total) = **34/242 = 14.0%**.
Vars resolved (non-skipped) = 124/242 = 51.2%.

`scoreboard.{edn,json}` here is the committed baseline for the T2 ratchet
(passing-file count must not decrease). Regenerate with the commands above.
