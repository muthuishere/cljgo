# ADR 0097 — Native Contrib Tier-1 Integration Runbook

A precise, sequenced runbook for the human integrator. Four contrib libraries
land as native satellites, **easiest-first**: `tools.cli` → `data.csv` →
`data.json` → `core.match`. Each follows the identical four-part satellite
contract; the tables below give the exact files and identifiers per lib.

## Gate command (run foreground, after EVERY lib)

```bash
CGO_ENABLED=0 go build ./... && go vet ./... && gofmt -l pkg cmd conformance templates && go test ./...
```

Conformance/emit runs additionally need a long timeout and serial execution
(per repo memory):

```bash
go test ./conformance/... -timeout 1800s -p 1
```

All green, no exceptions. Do NOT commit compiled binaries. Do NOT hand-write an
AOT twin — always run the generator (`go generate ./pkg/coreaot`).

## The satellite contract (applies to all four)

Each lib is landed by these five moves, in order:

1. **Port the `.cljg`** — copy the spike draft into `core/<file>.cljg`. Keep the
   EPL header. Use the satellite preamble (NO `ns` form): open with
   `(clojure.core/in-ns '<ns>)` then `(clojure.core/refer 'clojure.core)`. The
   loader creates the namespace bare and restores `*ns*`.
2. **Embed it** — add a `//go:embed <file>.cljg` → `var <Var> string` entry in
   `core/core.go` (mirror the existing `AsyncSource` / `StringSource` entries).
3. **Register the interpreted lazy loader** — add
   `pkg/eval/<lib>load.go`, a near-copy of `pkg/eval/asyncload.go`:
   `corelib.RegisterLibProvider("<ns>", …)` guarded by an interned-marker var,
   calling `e.loadBootSource(core.BootSource{NS, File, Source: &core.<Var>})`.
   It is **NOT** a boot source (ADR 0024). Register it from `eval.New` alongside
   `registerAsyncProvider`. No `pkg/corelib` change — these have no Go-native fn
   half.
4. **Generate the AOT twin** — run `go generate ./pkg/coreaot`; confirm the new
   `pkg/coreaot/clj<lib>` package appears and is wired into
   `pkg/coreaot/load.go`. Verify `pkg/coreaot/imports_test.go
   TestNoInterpreterInCompiledBinary` still holds.
5. **Land conformance** — copy the draft conformance file(s) into
   `conformance/tests/`; the harness auto-discovers `*.clj`. From M2 they run in
   BOTH the REPL and AOT binary (dual harness). REPL-vs-binary divergence is the
   release blocker — watch every frozen error string.

Per-lib identifiers:

| lib | ns | source draft → `core/` | embed var (in `core/core.go`) | loader file | marker sym | AOT twin | conformance dest |
|-----|----|-----|-----|-----|-----|-----|-----|
| tools.cli | `clojure.tools.cli` | `spikes/s52-tools-cli-native/draft-tools_cli.cljg` → `core/tools_cli.cljg` | `ToolsCliSource` | `parse-opts` | `pkg/eval/toolscliload.go` | `pkg/coreaot/cljtoolscli` | `conformance/tests/tools-cli.clj` |
| data.csv | `clojure.data.csv` | `spikes/s55-data-csv-native/draft-data_csv.cljg` → `core/data_csv.cljg` | `DataCsvSource` | `read-csv` | `pkg/eval/datacsvload.go` | `pkg/coreaot/cljdatacsv` | `conformance/tests/data-csv-*.clj` |
| data.json | `clojure.data.json` | `spikes/s53-data-json-native/draft-data_json.cljg` → `core/data_json.cljg` | `DataJsonSource` | `write-str` | `pkg/eval/datajsonload.go` | `pkg/coreaot/cljdatajson` | `conformance/tests/data-json-*.clj` |
| core.match | `clojure.core.match` | `spikes/s54-core-match-native/draft-core_match.cljg` → `core/match.cljg` | `MatchSource` | `match` | `pkg/eval/matchload.go` | `pkg/coreaot/cljmatch` | `conformance/tests/core-match-*.clj` |

(Marker symbols are illustrative — any var the satellite interns works; its
presence is what makes reload idempotent.)

---

## LIB 1 — `clojure.tools.cli` (S52, FEASIBLE) — easiest

The upstream single `.cljc` ports near-verbatim; the draft already carries the
three behavior-preserving adaptations (satellite preamble; reader conditionals
resolved to `:clj`/`:default` and inlined, dropping the `:cljs`
`goog.string.format` shim and `:cljr` branches; `compile-option-specs`' `:post`
map rewritten as explicit `(assert …)` calls; legacy `(Exception. …)` →
`(ex-info … {})` with identical message text).

1. Copy `draft-tools_cli.cljg` → `core/tools_cli.cljg`. Satellite preamble
   matches `core/string.cljg` + `core/async.cljg` (`in-ns` + `refer clojure.core`
   + `require [clojure.string :as s]`).
2. Add `//go:embed tools_cli.cljg` → `var ToolsCliSource string` to
   `core/core.go`.
3. Add `pkg/eval/toolscliload.go` from the `asyncload.go` template; marker
   `parse-opts`; register in `eval.New`. No `pkg/corelib` addition.
4. `go generate ./pkg/coreaot` → `pkg/coreaot/cljtoolscli`; confirm `load.go`
   wiring.
5. Copy `draft-conformance-tools.cli.clj` → `conformance/tests/tools-cli.clj`
   (15 frozen behaviors + the legacy `cli` banner).
6. **Optional** — add one **cljgo-oracle'd** `:parse-fn` exception-string case
   (residual #1: `(str e)` parse errors are host-specific and were deliberately
   NOT frozen — oracle a cljgo parse error, do not reuse the JVM
   `NumberFormatException` string). Optionally a `:no-such-key` dev-warning case.
7. Run the gate command. Verify an AOT binary that requires `clojure.tools.cli`
   links zero interpreter.

Notes / residual risks: `compile-option-specs` postconditions now throw a cljgo
`assert` error, not a JVM `AssertionError` (predicates identical, thrown type
differs — acceptable, these guard programmer error). The `*assert*`-gated
unknown-key dev warning (writes `*err*`) is kept verbatim but unexercised.

---

## LIB 2 — `clojure.data.csv` (S55, FEASIBLE, String surface)

`read-csv`/`write-csv` port pure-Clojure for String input; every option key
(`:separator :quote :quote? :newline`) verbatim. The draft already replaces
`PushbackReader`/`StringReader` with a pure one-slot-pushback string reader
(`pb-reader`/`rd`/`unrd`, mirroring size-1 pushback incl. pushing back eof),
`StringBuilder` with immutable accumulation, and the `java.io.Writer` sink with
`*out*` binding (captured via `with-out-str`). It reuses the already-shipped
`clojure.string/escape`.

1. Copy `draft-data_csv.cljg` → `core/data_csv.cljg` (EPL header; satellite
   preamble).
2. Add `//go:embed data_csv.cljg` → `var DataCsvSource string` to `core/core.go`.
3. Add `pkg/eval/datacsvload.go` (asyncload template); marker `read-csv`;
   register in `eval.New`.
4. `go generate ./pkg/coreaot` → `pkg/coreaot/cljdatacsv` (sibling to
   cljstring/cljedn); confirm `load.go` wiring. **Ensure `clojure.string` is
   linked whenever data.csv links** (the port depends on
   `clojure.string/escape`). Confirm `TestNoInterpreterInCompiledBinary` holds.
5. Split `draft-conformance-data.csv.clj` into per-behavior
   `conformance/tests/data-csv-*.clj` (one form + one `;; expect:` each,
   oracle-cited); 14 behaviors; run in BOTH harnesses.
6. Run the gate command. Add `cljdatacsv` to the satellite generator/manifest.

Notes / residual risks:
- **Streaming `java.io.Reader` input is unported** — String-only. A Reader arg
  gets a clear `ex-info`, not a wrong result (deferrable; seam present via
  `(string? input)` dispatch).
- **Malformed-input error MESSAGE bytes diverge** (no `EOFException`/`%c`
  format) — do NOT freeze the malformed-input path against the JVM oracle; if
  frozen, freeze cljgo's text.
- Perf: cell accumulation is immutable-string `str` (O(n²) on a giant unbroken
  cell) — fine for conformance; swap to a transient char vector + `apply str` if
  a perf budget is added (not a correctness risk).

---

## LIB 3 — `clojure.data.json` (S53, SCOPED, String surface)

Lands the **String-in / String-out** tier-1 surface: `read-str`, `read-json`,
`write-str`, `json-str`, and `read`/`write` reduced to their String forms — the
full parser, number state machine, escape decode/encode, indent, and all option
keys (`:key-fn :value-fn :bigdec :eof-error? :eof-value :escape-unicode
:escape-js-separators :escape-slash :indent :default-write-fn`). The draft
rewrites the JVM reader as a pure char cursor over a String and the writer's
`defprotocol JSONWriter` (~20 Java extensions) as a single `-write` cond over
cljgo predicates accumulating into an atom-held transient vector joined by
`(apply str)`.

1. Copy `draft-data_json.cljg` → `core/data_json.cljg`. EPL header; satellite
   preamble. **Intern `read` before the wholesale `refer` to avoid the
   `clojure.core/read` collision** (draft already does this). Keep the satellite
   ns-switch omitted (loader restores `*ns*`).
2. Add `//go:embed data_json.cljg` → `var DataJsonSource string` to
   `core/core.go`.
3. Add `pkg/eval/datajsonload.go` (asyncload template); marker `write-str`;
   register in `eval.New`.
4. `go generate ./pkg/coreaot` → `pkg/coreaot/cljdatajson`; confirm `load.go`
   wiring; verify a binary requiring data.json links zero interpreter.
5. Add `conformance/tests/data-json-*.clj` from `draft-conformance-data.json.clj`
   (15 frozen behaviors) into the dual harness. **The four error strings ARE
   frozen and match the oracle** — but the error TYPES are cljgo `ExceptionInfo`
   (`ex-message`), not `java.io.EOFException`/`Exception`; watch these strings
   for REPL-vs-binary divergence.
6. **Record in the ADR/spec the two first-cut limitations** so neither is
   silently claimed identical:
   - **Astral-plane** (> U+10000): cljgo strings are rune-indexed, the JVM is
     UTF-16 code-unit-indexed — emoji / rare-CJK differ on write (single 5-hex
     codepoint vs surrogate pair) and read (no surrogate recombination). All BMP
     behavior is byte-identical (15/15 frozen).
   - **Scoped-out surface**: `read` from `java.io.Reader` / `write` to
     `java.io.Writer`, `:extra-data-fn` + `on-extra-throw`, `pprint`/`pprint-json`
     (need `clojure.pprint`/`cl-format`), `print-json`/`write-json` (need
     `*out*`/host Writer), the UUID/Instant/Date/sql.Date JSONWriter arms.
7. Run the gate command.

Notes / residual risks: the number state machine ports without the upstream
char-array fast path (correctness-equivalent slow path; S50 measured this lib as
~100% Java, so the native port trades raw speed for portability). Later, once
`cljg.io` Reader/Writer + `clojure.pprint` + host temporal/UUID land, restore the
deferred arms and add their conformance freezes; optionally add a UTF-16 layer to
close the astral-plane residual.

---

## LIB 4 — `clojure.core.match` (S54, SCOPED, linear compiler) — hardest

**Zero upstream namespaces port verbatim** — the upstream engine is a `deftype`
tower on `clojure.lang.*` host interfaces + `Compiler/LOOP_LOCALS` +
`java.io.Writer`/`print-method` + `Class/forName` +
`IllegalArgumentException`/`AssertionError`, none of which resolve on the Go
host. The draft is a **from-scratch pure-Clojure LINEAR pattern compiler**: each
clause compiles in source order to a nested test/bind expr that yields the action
or tail-calls the next clause's fail continuation (a zero-arg fn — no code dup,
no backtracking exceptions). Public API (`match`/`matchv`/`matchm`/`match-let`)
and all option keywords preserved.

1. Copy `draft-core_match.cljg` → `core/match.cljg` (EPL header; satellite
   preamble `(in-ns 'clojure.core.match)` + refer core; confirm no public API
   name shadows core).
2. **Confirm cljgo exposes `&env` to `defmacro`.** If yes, add the `&env`
   local-shadowing rule (a pattern symbol naming a surrounding local → literal
   equality test). If no, document the divergence in the ns header (this cut
   treats every non-`_` symbol as a fresh binding — the one place a real program
   can observe a semantic difference from upstream).
3. Add `//go:embed match.cljg` → `var MatchSource string` to `core/core.go`.
4. Add `pkg/eval/matchload.go` (asyncload template); marker `match`; register in
   `eval.New`.
5. `go generate ./pkg/coreaot` → `pkg/coreaot/cljmatch` (mirroring
   cljstring/cljset). core.match is **macro-ONLY** (all expansion at compile
   time), so a compiled binary needs no runtime match code — **but the twin MUST
   exist** so the loader census + `TestNoInterpreterInCompiledBinary` + coreaot
   gates agree with the interpreted path. Verify an AOT binary using `match`
   links zero interpreter.
6. Move `draft-conformance-core.match.clj` → `conformance/tests/core-match-*.clj`
   (split the single combined vector into per-behavior files if the harness
   prefers granular freezes, keeping each `;; expect:` oracle value). 24
   behaviors; dual harness; confirm REPL-vs-binary parity.
7. Do NOT freeze the malformed-row error path against upstream's `AssertionError`
   prose — if frozen, freeze cljgo's `ex-info` text (diag doctrine for any new
   user-facing error).
8. **Record the honest deltas** in the ADR/spec + ns header: no decision-tree
   optimization, no redundancy/exhaustiveness warnings, `:when` == `:guard`, the
   `&env` local-shadowing status, and the scoped-out Java-only matcher
   namespaces (regex/date/java/array/binary), `binding :or` alts, and the
   `defpred` registry (all Java-only or niche; matches the S50 skip scope).
9. Run the gate command.

Notes / residual risks: perf — the linear expansion tests clauses top-to-bottom
(no single-occurrence-per-test DAG), so any attached perf budget must be set
against THIS cut, not upstream's tree; wide matches are slower (not a correctness
risk).

---

## Close-out (after all four)

1. Grep for double `RegisterLibProvider` registrations / duplicate interned
   markers (multi-batch merge hazard).
2. Update the satellite census / core gap-audit docs to list all four:
   tools.cli + data.csv FEASIBLE; data.json + core.match SCOPED with documented
   deltas.
3. Confirm no spike code merged verbatim into `pkg/` — the drafts land in
   `core/` only (ADR 0027). Update ADR 0097 status.
4. Final full gate green across all four (with `-timeout 1800s -p 1` for
   conformance).
5. `/opsx:archive` the change.

## Cross-cutting facts to remember

- **`load-file` is unbound under `cljgo run`** — a driver-only limitation. The
  spikes verified via source concatenation; the real loader path (the Go lib
  provider you wire in step 3) is the production path and does not use
  `load-file`. Do NOT wire a satellite via `load-file`.
- **No new Go host function is required** for the landing scope. All four
  namespaces are wholly Clojure over existing core builtins — nothing goes into
  `pkg/corelib`.
- **Exception types differ** (cljgo `ExceptionInfo` vs JVM classes) across all
  four; message strings may match (and are frozen only where they do). Matters
  only for catch-by-class downstream.
- **Every satellite is lazy, never a boot source** — the ADR 0024 boot budget
  stays untouched.
