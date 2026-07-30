## Why

ADR 0097 curates a **tier-1** set of the most-reached-for Clojure contrib
libraries and asks: can each land in cljgo as a **pure-Clojure native satellite
namespace** — no Maven jar, no JVM bytecode, no deferred import — following the
same house pattern as `clojure.string` / `clojure.core.async` (satellite
`.cljg` + lazy interpreted loader + generated AOT twin + oracle-frozen
conformance)? Four spikes (S52–S55) each took one library, re-derived its
observable JVM semantics over cljgo's existing core builtins, and froze the
result against real JVM Clojure. This change consolidates those four verdicts
into one landing plan.

The precedence principle holds throughout: these ports reproduce the JVM
library **byte-for-byte** on every frozen behavior; where a JVM-host dependency
(a `java.io.Reader`, `clojure.pprint`, host temporal types, UTF-16 code-unit
indexing) cannot be reproduced on the Go host, the spike **scoped it out
honestly** rather than shipping a silent divergence — and this proposal records
those scopes as documented deferrals, never as claimed parity.

## What Changes

Four contrib namespaces land as native satellites, **easiest-first**:

- **`clojure.tools.cli` (S52) — FEASIBLE (full).** Near-verbatim source port of
  the upstream single `.cljc` file, with three behavior-preserving adaptations
  (ns form → satellite preamble; reader conditionals resolved to `:clj`/`:default`
  and inlined; `:post` assertion map rewritten as explicit `assert` calls).
  15 oracle behaviors frozen, byte-identical incl. the legacy `cli` banner. No
  Go host work. Pure-Clojure whole namespace — nothing added to `pkg/corelib`.
- **`clojure.data.csv` (S55) — FEASIBLE (full, String surface).** `read-csv` /
  `write-csv` port to pure Clojure for String input; every option key
  (`:separator :quote :quote? :newline`) kept verbatim. JVM
  `PushbackReader`/`StringReader` → pure one-slot-pushback string reader;
  `StringBuilder` → immutable accumulation; `java.io.Writer` sink → `*out*`
  binding; reuses the already-shipped `clojure.string/escape`. 14 oracle
  behaviors frozen byte-for-byte. Deferrals: streaming `java.io.Reader` input
  (String-only; a Reader arg gets a clear `ex-info`, not a wrong result) and
  the malformed-input error **message text** (not byte-frozen against the JVM).
- **`clojure.data.json` (S53) — SCOPED.** The **String-in / String-out**
  surface (`read-str`, `read-json`, `write-str`, `json-str`, full parser +
  number state machine + escape encode/decode + indent + `:key-fn`/`:value-fn`
  and all option keys) ports ~verbatim and reproduces the oracle byte-for-byte,
  including all four error strings — 15 behaviors frozen. **Scoped out of the
  first cut:** `java.io.Reader`/`java.io.Writer` streaming arms,
  `:extra-data-fn`, `pprint`/`pprint-json` (need `clojure.pprint/cl-format`),
  `print-json`/`write-json` (need `*out*`/host Writer), and the
  UUID/Instant/Date JSONWriter arms (no host temporal/UUID types). **Known
  divergence:** astral-plane (> U+10000) characters — cljgo strings are
  rune-indexed, the JVM is UTF-16 code-unit-indexed, so emoji / rare-CJK differ
  on both write (single `ὠ0` vs surrogate pair `😀`) and read (no
  surrogate recombination). All BMP behavior is byte-identical.
- **`clojure.core.match` (S54) — SCOPED.** The upstream engine is **not** pure —
  its whole matrix/DAG (Maranget) implementation is a `deftype` tower on
  `clojure.lang.*` host interfaces (`ISeq`/`ILookup`/`IFn`/`Associative`/…),
  `Compiler/LOOP_LOCALS`, `java.io.Writer`/`print-method`, `Class/forName`,
  `IllegalArgumentException`/`AssertionError`. **Zero upstream namespaces port
  verbatim.** The spike re-derived the same observable semantics with a
  **from-scratch pure-Clojure LINEAR pattern compiler** (each clause compiles in
  source order to a nested test/bind expr tail-calling the next clause's fail
  continuation). Public API (`match`/`matchv`/`matchm`/`match-let`) and all
  option keywords preserved. All 24 oracle behaviors reproduce byte-for-byte.
  **Traded away** (none change a return value): the decision-tree optimization,
  redundancy/exhaustiveness warnings, the `&env` local-shadowing rule (every
  non-`_` symbol is treated as a fresh binding), and the Java-only matcher
  namespaces (regex/date/java/array/binary).

## Capabilities

### New Capabilities
- `native-contrib`: cljgo ships curated Clojure contrib libraries as
  pure-Clojure native satellite namespaces — each a `core/<lib>.cljg` loaded
  lazily on first `(require …)` via a `pkg/eval` lib provider, carried into
  AOT-compiled binaries by a generated `pkg/coreaot` twin (zero interpreter),
  and pinned by oracle-frozen `conformance/tests/*.clj` running under the dual
  REPL+AOT harness. The capability defines the satellite contract, the
  full-vs-scoped feasibility labelling, and the honest-deferral discipline for
  JVM-host surfaces that cannot be reproduced on the Go host.

### Modified Capabilities
<!-- None. This is a purely additive capability; each satellite loads lazily and
     is NOT a boot source, so the boot budget (ADR 0024) is untouched. -->

## Impact

- `core/`: four new satellite sources — `tools_cli.cljg`, `data_csv.cljg`,
  `data_json.cljg`, `match.cljg` — each with EPL header, satellite preamble
  (`in-ns` + `refer clojure.core`), embedded via `//go:embed` in `core/core.go`.
- `pkg/eval/`: four lazy lib providers modeled on `pkg/eval/asyncload.go`
  (`RegisterLibProvider` + interned-marker guard), registered from `eval.New`
  alongside `registerAsyncProvider`. **None is a boot source.** Unlike
  `clojure.core.async` there is **no Go-native fn half** — the namespaces are
  wholly Clojure, so nothing is added to `pkg/corelib`.
- `pkg/coreaot/`: four generated AOT twins (`cljtoolscli`, `cljdatacsv`,
  `cljdatajson`, `cljmatch`) produced by the same generator that made
  `cljstring`/`cljedn`/`cljwalk`, wired into `load.go`, so compiled binaries
  that `(require …)` a satellite link it with **zero interpreter**
  (`imports_test.go TestNoInterpreterInCompiledBinary` must still hold).
- `conformance/tests/`: the four frozen conformance files (68 oracle behaviors
  total: 15 + 14 + 15 + 24), auto-discovered, run in **both** REPL and AOT
  harnesses (dual harness from M2 — REPL-vs-binary divergence is the release
  blocker to watch, especially every frozen error string).
- ADR 0097: records the two SCOPED verdicts (data.json, core.match), the full
  verdicts (tools.cli, data.csv), and the enumerated deferrals so nothing is
  silently claimed as identical.
- No new Go host functions are **required** for the landing scope. Optional
  future host work the spikes flagged (would close deferrals, not blocking):
  `cljg.io` streaming char Reader/Writer, `clojure.pprint` (`cl-format`), host
  temporal/UUID types, and a UTF-16 surrogate layer for the astral-plane residual.
