## ADDED Requirements

### Requirement: Curated Clojure contrib libraries ship as pure-Clojure native satellites

cljgo MUST be able to ship a curated tier-1 Clojure contrib library as a
**pure-Clojure native satellite namespace** — a `core/<lib>.cljg` source (no
Maven jar, no JVM bytecode, no deferred import) that reproduces the upstream
library's observable JVM semantics over cljgo's existing core builtins. Each
satellite MUST follow the ratified satellite contract used by `clojure.string`
and `clojure.core.async`:

1. an EPL-headered `core/<lib>.cljg` with the satellite preamble (`in-ns` +
   `refer clojure.core`, NO `ns` form), embedded via `//go:embed` in
   `core/core.go`;
2. a lazy interpreted lib provider in `pkg/eval` (`RegisterLibProvider`, guarded
   by an interned-marker var) that loads the source on first `(require '<lib>)`
   and MUST NOT be a boot source (the ADR 0024 boot budget stays untouched);
3. a **generated** `pkg/coreaot` AOT twin wired into `pkg/coreaot/load.go`, so a
   compiled binary that requires the satellite links it with zero interpreter;
4. oracle-frozen `conformance/tests/*.clj` running under the dual REPL+AOT
   harness.

A satellite whose namespace is wholly Clojure MUST NOT add anything to
`pkg/corelib` (there is no Go-native fn half).

#### Scenario: a satellite loads lazily and never spends the boot budget
- **WHEN** a program does not `(require '<lib>)`
- **THEN** the satellite source is not evaluated and the boot cost is unchanged;
  it evaluates only on the first `(require '<lib>)`

#### Scenario: a compiled binary requiring a satellite links zero interpreter
- **WHEN** a binary is AOT-compiled from a program that `(require '<lib>)`s a
  satellite
- **THEN** the generated AOT twin carries the namespace and
  `TestNoInterpreterInCompiledBinary` still holds — no tree-walk interpreter is
  linked

#### Scenario: REPL and compiled binary agree on every frozen behavior
- **WHEN** a satellite's conformance file runs under both the REPL and the AOT
  binary harness
- **THEN** every `;; expect:` output matches in both — REPL-vs-binary divergence
  is a release blocker

### Requirement: tools.cli and data.csv land as FEASIBLE (full) satellites

`clojure.tools.cli` MUST reproduce its 15 frozen JVM-oracle behaviors
byte-for-byte, including the legacy `cli` banner, ported near-verbatim from the
upstream `.cljc` with only behavior-preserving adaptations (satellite preamble,
reader conditionals resolved to `:clj`/`:default`, `:post` map rewritten as
explicit `assert`). `clojure.data.csv` (`read-csv`/`write-csv`) MUST reproduce
its 14 frozen behaviors byte-for-byte for **String** input, keeping every option
key (`:separator :quote :quote? :newline`) verbatim, reusing the shipped
`clojure.string/escape`. Neither MUST require any new Go host function.

#### Scenario: tools.cli parses options identically to the JVM
- **WHEN** `parse-opts` runs the 15 frozen scenarios (and the legacy `cli`
  banner)
- **THEN** each result matches the JVM Clojure oracle byte-for-byte

#### Scenario: data.csv reads and writes String CSV identically to the JVM
- **WHEN** `read-csv`/`write-csv` run the 14 frozen String-input scenarios
- **THEN** each result matches the JVM `data.csv` oracle byte-for-byte

#### Scenario: data.csv on a non-String Reader fails cleanly, never wrongly
- **WHEN** `read-csv` is given a non-String `java.io.Reader` argument
- **THEN** it raises a clear `ex-info` (streaming Reader input is a documented
  deferral), never returns a wrong result

#### Scenario: malformed-input error text is not frozen against the JVM
- **WHEN** a conformance case exercises data.csv malformed input
- **THEN** any frozen message text is cljgo's own (oracle'd on cljgo), never the
  JVM's `EOFException`/`%c` text

### Requirement: data.json lands SCOPED to the String surface with recorded deferrals

`clojure.data.json` MUST land its **String-in / String-out** surface
(`read-str`, `read-json`, `write-str`, `json-str`, the full parser, number state
machine, escape encode/decode, indent, `:key-fn`/`:value-fn` and all option
keys) reproducing 15 frozen JVM-oracle behaviors byte-for-byte, including all
four error strings. The change MUST record — in the ADR and the spec, not
silently — two limitations: (a) astral-plane (> U+10000) characters diverge
(rune-indexed cljgo emits a single 5-hex codepoint and does not recombine
surrogates on read; the JVM emits/recombines UTF-16 surrogate pairs), all BMP
behavior being byte-identical; and (b) the scoped-out `java.io.Reader`/`Writer`
arms, `:extra-data-fn`, `pprint`/`pprint-json`, `print-json`/`write-json`, and
the UUID/Instant/Date JSONWriter arms.

#### Scenario: BMP JSON round-trips identically to the JVM
- **WHEN** `read-str`/`write-str` process BMP JSON across the 15 frozen scenarios
- **THEN** each result, including the four error strings, matches the JVM
  `data.json` oracle byte-for-byte

#### Scenario: astral-plane divergence is documented, not hidden
- **WHEN** a JSON string contains a character above U+10000
- **THEN** the divergence (single codepoint escape vs surrogate pair; no
  recombination) is recorded as a known limitation and is NOT claimed as parity

### Requirement: core.match lands SCOPED as a linear pattern compiler

`clojure.core.match` MUST preserve the public API (`match`/`matchv`/`matchm`/
`match-let`) and all option keywords, reproducing 24 frozen JVM-oracle behaviors
byte-for-byte, implemented as a **from-scratch pure-Clojure linear pattern
compiler** (each clause compiles in source order to a nested test/bind expr
tail-calling the next clause's fail continuation) — NOT the upstream Maranget
`deftype`/host-interface DAG, which does not resolve on the Go host. The change
MUST record the honest deltas (none of which change a return value): no
decision-tree optimization, no redundancy/exhaustiveness warnings, `:when` ==
`:guard`, the `&env` local-shadowing rule deferred (a pattern symbol naming a
surrounding local is treated as a fresh binding unless cljgo exposes `&env`), and
the scoped-out Java-only matcher namespaces (regex/date/java/array/binary),
`binding :or` alts, and the `defpred` registry.

#### Scenario: match reproduces every frozen behavior
- **WHEN** `match`/`matchv`/`matchm`/`match-let` run the 24 frozen scenarios
- **THEN** each result matches the JVM `core.match` oracle byte-for-byte

#### Scenario: the linear-compiler deltas are recorded, not silently claimed
- **WHEN** documenting the core.match satellite
- **THEN** the deferred tree optimization, absent warnings, `:when`==`:guard`,
  `&env` local-shadowing status, and scoped-out matcher namespaces are recorded
  in the ADR, spec, and namespace header — not presented as full upstream parity

### Requirement: JVM-host surfaces the Go host lacks are honest deferrals, never silent divergence

Where a JVM-host dependency cannot be reproduced on the Go host, the satellite
MUST scope it out explicitly and record it as a deferral rather than ship a
silent divergence. Error objects raised by ported paths are cljgo
`ExceptionInfo`; their message strings MAY match the oracle (and be frozen only
then), but the exception **types** differ from the JVM's, which MUST be noted
where a downstream might catch by class. No new Go host function is required for
the landing scope; the optional future host work (`cljg.io` streaming
Reader/Writer, `clojure.pprint`/`cl-format`, host temporal/UUID types, a UTF-16
surrogate layer) is tracked as a follow-up that would close deferrals.

#### Scenario: a deferred JVM-host surface is documented rather than faked
- **WHEN** a library depends on `java.io.Reader`/`Writer`, `clojure.pprint`,
  host temporal/UUID types, or UTF-16 code-unit indexing that the Go host lacks
- **THEN** the satellite scopes that arm out and records it as a deferral, and
  MUST NOT return a wrong result in its place

#### Scenario: exception type differences are noted
- **WHEN** a ported error path raises a cljgo `ExceptionInfo` whose message
  matches the JVM
- **THEN** the message MAY be frozen, but the differing exception type is noted
  for any catch-by-class downstream
