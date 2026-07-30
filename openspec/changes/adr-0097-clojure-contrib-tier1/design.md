## Context

ADR 0097 is the authority; this is its implementation design. Four spikes are
closed, MET, and read-only against `pkg/` (ADR 0027 — their drafts are
re-authored into `core/`, not the interpreter, and no spike code merges verbatim
into `pkg/`):

- `spikes/s52-tools-cli-native/` — `clojure.tools.cli` 1.1.230, FEASIBLE.
- `spikes/s53-data-json-native/` — `clojure.data.json` 2.5.1, SCOPED (String surface).
- `spikes/s54-core-match-native/` — `clojure.core.match` 1.1.1, SCOPED (linear compiler).
- `spikes/s55-data-csv-native/` — `clojure.data.csv` 1.1.0, FEASIBLE (String surface).

The satellite pattern is already ratified and in production for
`clojure.string`, `clojure.core.async`, `clojure.set`, `clojure.data`,
`clojure.edn`, `clojure.walk`, `clojure.zip`, `clojure.pprint`. This change is
four more instances of the **same** contract — the design work is confirming
each library fits it and recording where the JVM host surface forced a scope.

Verified reuse points (Explore pass):
- **Lazy interpreted loader**: `pkg/eval/asyncload.go` is the canonical shape —
  `corelib.RegisterLibProvider("<ns>", fn)` guarded by an interned-marker var
  (`asyncMarker = lang.NewSymbol("go-loop")`), calling
  `e.loadBootSource(core.BootSource{NS, File, Source})`; registered from
  `eval.New` alongside `registerAsyncProvider`. The load fires on first
  `(require '<ns>)`, NOT at boot — the boot budget (ADR 0024) stays untouched.
- **Embed registry**: `core/core.go` declares each source with `//go:embed
  <file>.cljg` → `var XxxSource string` (e.g. `AsyncSource`, `StringSource`).
- **Satellite `.cljg` house style**: NO `ns` form; open with
  `(clojure.core/in-ns '<ns>)` + `(clojure.core/refer 'clojure.core)`; the
  loader creates the namespace bare and restores `*ns*`. EPL header preserved on
  ported source.
- **AOT twin**: `pkg/coreaot/clj*` packages are **generated** (`go generate
  ./pkg/coreaot`, `cmd/gencore`) — one `Load()` per boot source — and wired into
  `pkg/coreaot/load.go`. `imports_test.go TestNoInterpreterInCompiledBinary`
  proves a compiled binary that requires a satellite links **zero** interpreter.
  **Do NOT hand-write a twin** — run the generator.
- **Conformance**: `conformance/tests/*.clj` is auto-discovered; each file froze
  its `;; expect:` output against the real `clojure` CLI oracle at authoring
  time. From M2 the same files run in the **dual** harness (REPL + AOT binary);
  REPL-vs-binary divergence is the unforgivable, release-blocking failure.

## Goals / Non-Goals

**Goals:**
- Land four contrib namespaces as native satellites, each: `core/<lib>.cljg` +
  `pkg/eval` lazy provider + generated `pkg/coreaot` twin + oracle-frozen
  conformance, running green under the dual harness.
- Preserve the public API and every option key of each library exactly.
- Record the SCOPED verdicts (data.json, core.match) and every deferral so no
  divergence is silently claimed as parity.

**Non-Goals:**
- Reproducing JVM-host surfaces the Go host lacks: `java.io.Reader`/`Writer`
  streaming, `clojure.pprint`/`cl-format`, host temporal/UUID types, UTF-16
  code-unit string indexing. Each is a documented deferral, not this change.
- The Maranget decision-tree optimizer + exhaustiveness/redundancy warnings for
  core.match (the linear compiler is observably equivalent on return values).
- Any new Go host function. None is required for the landing scope; the optional
  host work below is explicitly later.
- Making any satellite a boot source (would spend the ADR 0024 budget).

## Decisions

### D1 — One satellite contract, four instances

Each library lands via the identical four-part contract; the per-library design
is only *which source ports* and *what scope it lands at*:

| lib | ns | source file | embed var | marker sym | loader | AOT twin | conformance |
|-----|----|-----|-----|-----|-----|-----|-----|
| tools.cli | `clojure.tools.cli` | `core/tools_cli.cljg` | `ToolsCliSource` | `parse-opts` | `pkg/eval/toolscliload.go` | `pkg/coreaot/cljtoolscli` | `tools-cli.clj` |
| data.csv | `clojure.data.csv` | `core/data_csv.cljg` | `DataCsvSource` | `read-csv` | `pkg/eval/datacsvload.go` | `pkg/coreaot/cljdatacsv` | `data-csv-*.clj` |
| data.json | `clojure.data.json` | `core/data_json.cljg` | `DataJsonSource` | `write-str` | `pkg/eval/datajsonload.go` | `pkg/coreaot/cljdatajson` | `data-json-*.clj` |
| core.match | `clojure.core.match` | `core/match.cljg` | `MatchSource` | `match` | `pkg/eval/matchload.go` | `pkg/coreaot/cljmatch` | `core-match-*.clj` |

Marker symbols are illustrative — pick any var the satellite interns (its
presence means the source already evaluated this process; the guard makes reload
idempotent under a test harness that recreates namespaces).

### D2 — Naming collisions with `clojure.core` (precedence principle)

Two satellites intern a var whose name collides with `clojure.core`, so the
satellite MUST intern its own **before** the wholesale `refer`:
- **data.json** — intern `read` before `(refer 'clojure.core)` to avoid the
  `clojure.core/read` collision (spike-confirmed).
- **core.match** — public API is macro-only; confirm `matchv`/`matchm`/
  `match-let`/`match` do not shadow core; `match` itself is new.
No satellite renames or reimplements a `clojure.core` var — the precedence
principle stands (Clojure is first-class; these are additions on top).

### D3 — No Go-native fn half

Unlike `clojure.core.async` (whose fn half is Go-native in `pkg/corelib`), all
four namespaces are **wholly Clojure** over existing core builtins. Nothing is
added to `pkg/corelib`. The spikes individually confirmed every needed primitive
is already present: `format`, `re-find`/`re-seq`, `condp`, `with-out-str`,
`pr-str`, `clojure.string/*`, `*err*`/`*out*`, `binding`, `ex-info`, `defn-`,
`parse-long`/`parse-double`/`bigint`/`bigdec`, `NaN?`/`infinite?`, transients,
`nth`/`count` on strings, and (data.csv) `clojure.string/escape`.

### D4 — Error objects are cljgo ExceptionInfo, and error-string freezing rules

Ported error paths raise cljgo `ex-info` (ExceptionInfo), not JVM
`EOFException`/`Exception`/`AssertionError`. Two consequences to freeze
correctly:
- **data.json** — the four JSON error **strings** match the oracle and ARE
  frozen; but the exception **types** differ (matters only if a downstream
  catches by class — noted, not fixed).
- **data.csv / core.match** — the malformed-input / malformed-row error
  **message text** is host-specific and MUST NOT be frozen against the JVM. Any
  error-string conformance freezes **cljgo's** text (oracle a cljgo run), never
  the JVM's. Per the diag doctrine, no bare `fmt.Errorf`/panic reaches a user;
  these are Clojure-level `ex-info`, which the renderer handles.

### D5 — data.csv scope: String surface, `*out*` sink

`read-csv` accepts String input via a pure one-slot-pushback reader
(`pb-reader`/`rd`/`unrd`, faithfully mirroring size-1 pushback incl. pushing
back eof). `write-csv` binds `*out*` to the writer and emits with `print`,
captured via `with-out-str`; file output already works through `*out*`. A
non-String `java.io.Reader` arg dispatches (seam: `(string? input)`) to a clear
`ex-info`, never a wrong result. The AOT twin MUST link `clojure.string` when
data.csv links (the port depends on `clojure.string/escape`).

### D6 — data.json scope: String-in/String-out tier-1, deferrals recorded

The reader is a pure char cursor over a String (`{:s :len :pos(atom)}` with
`read-char`/`unread-char` returning int codes / -1); the writer is a single
`-write` cond over cljgo predicates accumulating into an atom-held transient
vector joined by `(apply str)` (replacing the JVM `defprotocol JSONWriter` that
extends ~20 Java classes). `read`/`write` are reduced to their String forms (no
host `java.io.Reader`/`Writer`). The number state machine ports without the
upstream char-array fast path (correctness-equivalent slow path). Two known
limitations are recorded in the ADR/spec, NOT hidden:
1. **Astral-plane divergence** (> U+10000): rune-indexed cljgo emits a single
   5-hex codepoint and does not recombine surrogates; JVM emits/recombines
   UTF-16 surrogate pairs. All BMP behavior is byte-identical (15/15).
2. **Scoped-out surface**: Reader/Writer arms, `:extra-data-fn`,
   `pprint`/`pprint-json`, `print-json`/`write-json`, and the
   UUID/Instant/Date JSONWriter arms.

### D7 — core.match scope: linear compiler, not the Maranget DAG

The from-scratch linear pattern compiler reproduces all 24 oracle behaviors
byte-for-byte. It is macro-**only** (all expansion at compile time), so a
compiled binary needs no runtime match code — but the AOT twin MUST still exist
so the loader census, `TestNoInterpreterInCompiledBinary`, and the coreaot gates
agree with the interpreted path. Honest deltas to record in the ns header +
ADR/spec (none change a return value): no tree optimization, no
redundancy/exhaustiveness warnings, `:when` == `:guard`, and the **`&env`
local-shadowing rule is deferred** — a pattern symbol naming a surrounding local
is treated as a fresh binding, not a literal equality test. If cljgo exposes
`&env` to `defmacro`, the rule MAY be added; otherwise the divergence is
documented in the ns header. The Java-only matcher namespaces
(regex/date/java/array/binary), `binding :or` alts, and the `defpred` registry
are scoped out (all Java-only or niche; matches the S50 skip scope).

### D8 — Dual-harness parity is the gate

Every conformance file runs in BOTH the REPL and the AOT binary. Per repo memory,
emit/conformance runs need `-timeout 1800s -p 1`. REPL-vs-binary divergence is
the release blocker — watch every frozen error string especially. After merge,
grep for double `RegisterLibProvider` / duplicate interned markers (multi-batch
merge hazard), and update the satellite census / core gap-audit docs to list all
four (two FEASIBLE, two SCOPED with their documented deltas).

## Risks / Trade-offs

- **data.json astral-plane divergence** (highest semantic risk) — must be
  documented, never hidden. Mitigation: recorded in ADR + spec + ns header;
  optional future UTF-16 surrogate layer closes it. All BMP is byte-identical.
- **core.match `&env` local-shadowing** — the one place a real program can
  observe a semantic difference from upstream. Mitigation: add the rule if cljgo
  provides `&env`; else document the divergence explicitly in the ns header.
- **Perf** — data.json omits the char-array fast path (S50 measured it as ~100%
  Java; native port trades raw speed for portability); data.csv cell
  accumulation is O(n²) on a giant unbroken cell; core.match's linear expansion
  tests clauses top-to-bottom (no DAG). Any attached perf budget MUST be set
  against THIS cut, not upstream — swap to transients if a budget lands (not a
  correctness risk).
- **Exception TYPE (not message) mismatch** — cljgo `ex-info` vs JVM exception
  classes across all four; matters only for catch-by-class downstream.
- **`load-file` unavailable under `cljgo run`** — a driver-only limitation; the
  real loader path (Go lib provider) does not use it. Spikes verified via source
  concatenation; the integration wiring is the real path.

## Open Questions

- Whether cljgo exposes `&env` to `defmacro` (decides core.match D7 — add the
  local-shadowing rule vs document the divergence). Resolve during task 4.2.
- Whether the harness prefers one combined conformance file per lib or
  per-behavior split files (`data-csv-*.clj`, `core-match-*.clj`) — keep each
  `;; expect:` oracle value intact either way. Resolve during each lib's task 4.
- Optional host work (all later, none blocking): `cljg.io` streaming
  Reader/Writer (restores data.csv Reader input + data.json Reader/Writer arms +
  `:extra-data-fn` + `print-json`), `clojure.pprint`/`cl-format` (restores
  `pprint-json`), host temporal/UUID types (restores JSONWriter temporal arms),
  UTF-16 surrogate layer (closes the astral-plane residual).
