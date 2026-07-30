Ordered EASIEST-FIRST. Each lib is one satellite: port `.cljg` → register
interpreted loader → generate/author AOT twin → land conformance → gates green
(dual harness). The gate command (foreground, long timeout per repo memory) is:

```
CGO_ENABLED=0 go build ./... && go vet ./... && gofmt -l pkg cmd conformance templates && go test ./...
```
Conformance/emit runs additionally need `-timeout 1800s -p 1`. All green, no
exceptions. Do NOT hand-write an AOT twin — run the generator.

## 0. Prep

- [ ] 0.1 Land the ADR 0097 note: two FEASIBLE verdicts (tools.cli, data.csv),
  two SCOPED verdicts (data.json String-surface, core.match linear-compiler),
  and the enumerated deferrals (astral-plane, Reader/Writer/pprint/temporal,
  core.match tree-opt/warnings/`&env`, malformed-input message text).

## 1. `clojure.tools.cli` (S52) — FEASIBLE, easiest

- [ ] 1.1 Copy `spikes/s52-tools-cli-native/draft-tools_cli.cljg` →
  `core/tools_cli.cljg` (EPL header; satellite preamble `in-ns` + `refer
  clojure.core` + `require [clojure.string :as s]`, matching string/async style).
- [ ] 1.2 Add the `//go:embed tools_cli.cljg` → `var ToolsCliSource string`
  entry to `core/core.go`.
- [ ] 1.3 Add `pkg/eval/toolscliload.go` modeled on `asyncload.go`:
  `RegisterLibProvider("clojure.tools.cli", …)` → `loadBootSource` the embedded
  source, guarded by an interned-marker var (e.g. `parse-opts`); NOT a boot
  source (ADR 0024); register from `eval.New` alongside `registerAsyncProvider`.
  No `pkg/corelib` addition (whole ns is Clojure).
- [ ] 1.4 Generate the AOT twin `pkg/coreaot/cljtoolscli` via `go generate
  ./pkg/coreaot`; confirm it is wired into `pkg/coreaot/load.go`.
- [ ] 1.5 Drop `spikes/s52-tools-cli-native/draft-conformance-tools.cli.clj` →
  `conformance/tests/tools-cli.clj` (15 frozen behaviors incl. the legacy `cli`
  banner).
- [ ] 1.6 Optional: add one **cljgo-oracle'd** `:parse-fn` exception-string case
  (residual #1 — parse-error `(str e)` is host-specific, was NOT frozen; oracle a
  cljgo parse error, do not reuse the JVM `NumberFormatException` string), plus
  optionally a `:no-such-key` dev-warning case.
- [ ] 1.7 Gates green (dual harness). Verify an AOT binary that requires
  `clojure.tools.cli` links zero interpreter.

## 2. `clojure.data.csv` (S55) — FEASIBLE, String surface

- [ ] 2.1 Copy `spikes/s55-data-csv-native/draft-data_csv.cljg` →
  `core/data_csv.cljg` (EPL header; satellite preamble `in-ns` + `refer
  clojure.core`).
- [ ] 2.2 Add `//go:embed data_csv.cljg` → `var DataCsvSource string` to
  `core/core.go`.
- [ ] 2.3 Add `pkg/eval/datacsvload.go` mirroring `asyncload.go`:
  `RegisterLibProvider("clojure.data.csv", …)` guarded by an interned marker
  (e.g. `read-csv`); NOT a boot source; register in `eval.New`.
- [ ] 2.4 Generate AOT twin `pkg/coreaot/cljdatacsv` (sibling to
  cljstring/cljedn); register in `pkg/coreaot/load.go`; **ensure `clojure.string`
  is linked when data.csv links** (the port uses `clojure.string/escape`).
  Verify `imports_test.go TestNoInterpreterInCompiledBinary` still holds.
- [ ] 2.5 Split `spikes/s55-data-csv-native/draft-conformance-data.csv.clj` into
  per-behavior `conformance/tests/data-csv-*.clj` (one form + one `;; expect:`,
  oracle-cited); 14 behaviors; run in BOTH harnesses.
- [ ] 2.6 Do NOT freeze the malformed-input path against the JVM oracle — its
  message text (no `EOFException`/`%c` format) is cljgo-specific; if frozen at
  all, freeze cljgo's text. String-only read input; a Reader arg gets a clear
  `ex-info` (seam already present, `(string? input)` dispatch).
- [ ] 2.7 Gates green (dual harness). Add `cljdatacsv` to the satellite
  generator/manifest.

## 3. `clojure.data.json` (S53) — SCOPED (String surface)

- [ ] 3.1 Copy `spikes/s53-data-json-native/draft-data_json.cljg` →
  `core/data_json.cljg` (EPL header; satellite preamble; **intern `read` before
  the wholesale `refer` to avoid the `clojure.core/read` collision**; keep the
  satellite ns-switch omitted — loader restores `*ns*`).
- [ ] 3.2 Add `//go:embed data_json.cljg` → `var DataJsonSource string` to
  `core/core.go`.
- [ ] 3.3 Add `pkg/eval/datajsonload.go` mirroring `asyncload.go`:
  `RegisterLibProvider("clojure.data.json", …)` guarded by an interned marker
  (e.g. `write-str`); NOT a boot source; register in `eval.New`.
- [ ] 3.4 Generate AOT twin `pkg/coreaot/cljdatajson`; wire into
  `pkg/coreaot/load.go`; verify a binary requiring data.json links zero
  interpreter.
- [ ] 3.5 Add `conformance/tests/data-json-*.clj` from
  `draft-conformance-data.json.clj` (15 frozen behaviors) into the dual harness.
  The **four error strings** ARE frozen and match the oracle (types differ —
  cljgo ExceptionInfo — but message bytes match); watch these for REPL-vs-binary
  divergence.
- [ ] 3.6 Record in ADR/spec the two first-cut limitations: astral-plane
  (> U+10000) divergence (single 5-hex codepoint vs surrogate pair; no
  recombination on read) and the scoped-out Reader/Writer/pprint/temporal
  surface — so neither is silently claimed identical.
- [ ] 3.7 Gates green (dual harness).

## 4. `clojure.core.match` (S54) — SCOPED (linear compiler)

- [ ] 4.1 Copy `spikes/s54-core-match-native/draft-core_match.cljg` →
  `core/match.cljg` (EPL header; satellite preamble `(in-ns
  'clojure.core.match)` + refer core; confirm no public API name shadows core).
- [ ] 4.2 Confirm cljgo exposes `&env` to `defmacro`. If yes, add the `&env`
  local-shadowing rule (a pattern symbol naming a surrounding local → literal
  equality test). If no, document the divergence in the ns header (every non-`_`
  symbol is a fresh binding).
- [ ] 4.3 Add `//go:embed match.cljg` → `var MatchSource string` to
  `core/core.go`.
- [ ] 4.4 Add `pkg/eval/matchload.go` mirroring `asyncload.go`:
  `RegisterLibProvider("clojure.core.match", …)` guarded by an interned marker
  (e.g. `match`); NOT a boot source; register in `eval.New`.
- [ ] 4.5 Generate AOT twin `pkg/coreaot/cljmatch` (mirroring cljstring/cljset).
  core.match is macro-ONLY, so a compiled binary needs no runtime match code —
  but the twin MUST exist so the loader census + `TestNoInterpreterInCompiledBinary`
  + coreaot gates agree with the interpreted path. Verify an AOT binary using
  match links zero interpreter.
- [ ] 4.6 Move `draft-conformance-core.match.clj` into
  `conformance/tests/core-match-*.clj` (split per-behavior if the harness
  prefers; keep each `;; expect:` oracle value). 24 behaviors; dual harness.
- [ ] 4.7 Do NOT freeze the malformed-row error path against upstream's
  `AssertionError` prose — if frozen, freeze cljgo's `ex-info` text (diag
  doctrine for any new user-facing error).
- [ ] 4.8 Record the honest deltas in ADR/spec + ns header: no tree
  optimization, no redundancy/exhaustiveness warnings, `:when` == `:guard`,
  `&env` local-shadowing status, and the scoped-out matcher namespaces
  (regex/date/java/array/binary), `binding :or` alts, `defpred` registry.
- [ ] 4.9 Gates green (dual harness).

## 5. Close-out

- [ ] 5.1 Grep for double `RegisterLibProvider` registrations / duplicate
  interned markers after the multi-batch merge.
- [ ] 5.2 Update the satellite census / core gap-audit docs to list all four
  (tools.cli + data.csv FEASIBLE; data.json + core.match SCOPED with documented
  deltas).
- [ ] 5.3 No spike code merged verbatim into `pkg/`; drafts land in `core/` only
  (ADR 0027). Update ADR 0097 status.
- [ ] 5.4 Full gates green one final time across all four:
  `CGO_ENABLED=0 go build ./... && go vet ./... && gofmt -l pkg cmd conformance templates && go test ./...`
  (conformance `-timeout 1800s -p 1`).
- [ ] 5.5 `/opsx:archive` this change.

## Deferred follow-ups (tracked, not blocking)

- **`cljg.io` streaming Reader/Writer** — restores data.csv Reader input,
  data.json Reader/Writer arms + `:extra-data-fn` + `print-json`/`write-json`.
- **`clojure.pprint` / `cl-format`** — restores data.json `pprint`/`pprint-json`.
- **Host temporal/UUID types** — restores the data.json UUID/Instant/Date
  JSONWriter arms.
- **UTF-16 surrogate layer** — closes the data.json astral-plane residual.
- **core.match `&env` local-shadowing** — add if/when cljgo exposes `&env` to
  `defmacro`.
- **Perf** — transient-backed accumulation for data.json/data.csv, decision-tree
  for core.match, if a perf budget is attached (set against THIS cut, not upstream).
