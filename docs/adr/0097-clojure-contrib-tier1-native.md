# ADR 0097 — `org.clojure/*` contrib, natively: curated tier-1

Date: 2026-07-27 · Status: **proposed** (drafted ahead of its implementation
spikes s52–s55, per ADR 0027 — the per-lib feasibility claims below exist for
those spikes to confirm or kill). Complements **ADR 0095** (Clojars consumption):
0095 is the escape hatch for the long tail; this ADR is the *native* answer for
the handful of contrib libraries everyone reaches for. Extends the satellite-
namespace program that already ships `clojure.string`/`set`/`edn`/`walk`/`zip`/
`data`/`pprint`/`test`/`repl` and `clojure.core.async` natively.

## Context

S50 (spike, 2026-07-27) measured the real portability of `org.clojure/*` from
Clojars and found the load-as-source path is **utility-shaped and Java-blocked**
for the exact libraries people use most: `data.json` is 100% Java (unusable as
source), `core.match` is 5-pure/5-Java, `core.cache`/`core.memoize` are
Java-blocked. So *consuming* them (ADR 0095) does not deliver them. The owner's
call (2026-07-27): *"make all org.clojure implemented in cljgo as well … that's
core"* — then, on scope, *"we don't need all, find only high value."*

The precedent is already in the tree and proven: cljgo reimplements the
`clojure.*` satellites and `clojure.core.async` (over **real goroutines**)
natively, under their exact names, frozen against the JVM oracle. This ADR
extends that program to a **curated tier-1 of the contrib libraries**, no more.

**This is faithful reimplementation, not shadowing.** The precedence principle
(CLAUDE.md, owner 2026-07-12) forbids *changing* `clojure.core`/reader semantics;
it does not forbid providing `clojure.tools.cli/parse-opts` with byte-identical
behavior — that is exactly what `clojure.string` already is. Every namespace here
keeps its **exact upstream name and public API**; the only thing that changes is
that the Java guts become pure Clojure or a Go host fn.

## Decision

### 1. Tier-1 scope — four libraries, curated by value × reach

| namespace | value | measured (S50) | native strategy |
|---|---|---|---|
| `clojure.core.async` | ⭐⭐⭐ | — | **already shipped** (`core/async.cljg`) — listed for completeness |
| `clojure.tools.cli` | ⭐⭐⭐ | fully pure (1 ns) | **pure-Clojure port** — arg parsing is one-shot, not hot; the pure port IS the faithful full version, no perf concern (S52: 15 behaviors byte-identical) |
| `clojure.data.json` | ⭐⭐⭐ | 1 ns, all Java (reader/writer) | **Go host parse/serialize primitives** (fast path + full Unicode incl. astral-plane) under a thin Clojure option layer — the `clojure.string` model. NOT the pure-Clojure BMP-only/slow-path draft (S53), which is rejected as tier-1 per decision-2 mandate 2. |
| `clojure.core.match` | ⭐⭐ | 5 pure / 5 Java | **the real Maranget decision-tree compiler** — implemented as a Go host primitive invoked at macroexpand time. NOT the linear substitute the pure spike produced (S54), which drops the optimization and is rejected per decision-2 mandate 2. |
| `clojure.data.csv` | ⭐⭐ | small | **pure-Clojure port** for the String surface (S55: 14 behaviors byte-identical); a Go host char-reader adds streaming `Reader` input + throughput if a perf/feature need is shown. |

**Explicitly out** (owner: not all): `core.typed` (a whole JVM type analyzer),
`core.rrb-vector` (7/11 Java, reaches into `PersistentVector` internals),
`core.logic`/`core.unify`/`core.contracts` (niche), the archived
`core.incubator`, and every `clr.*` CLR port. These stay ADR-0095 consume
candidates if a user wants them and they are pure enough.

### 2. Doctrine — how a contrib lib is reimplemented

1. **Exact namespace, exact public API.** `clojure.data.json/write-str` is
   `clojure.data.json/write-str`, same arglist, same option keys, same return.
   A consumer's `(:require [clojure.data.json :as json])` must not know or care
   that it is cljgo-native.
2. **Performance is never compromised — no degraded "secondary" version ships
   (owner mandate, 2026-07-27).** A native contrib lib must match *or beat* the
   upstream on both correctness and speed. Pure Clojure is used only where it is
   already as fast and as complete as the original (e.g. `tools.cli` arg parsing,
   which is one-shot and not hot). Where a faithful port cannot match the
   upstream in pure Clojure — an algorithm (core.match's Maranget decision tree),
   a hot loop (data.json's char-scan parser/serializer), full Unicode (astral
   plane) — the hot/algorithmic path becomes a **Go host primitive under a thin
   Clojure API**, exactly the `clojure.string` model. A slower "linear" matcher
   or a "BMP-only" JSON writer is **not** an acceptable tier-1 substitute; it may
   exist only as reference. "Performance is a feature" (design/00 §1.4) — these
   ports are CI perf-budgeted like conformance, benchmarked against the JVM
   original. *If a feature is genuinely not needed, it is left out (lazy/opt-in,
   mandate 5) — but nothing that ships is slow.*
3. **Oracle-frozen conformance, dual-harness.** Every behavior is a
   `conformance/tests/*.clj` file with a `;; expect:` output **captured from the
   real library on the JVM** (`clojure -Sdeps '{:deps {<coord>}}' -M -e …`,
   Clojure 1.12.5) and cited. From M2 the same files run AOT-compiled too;
   REPL-vs-binary divergence is a release blocker (unchanged bar).
4. **Attribution + license.** Upstream is EPL-1.0. A ported file keeps an EPL
   header crediting the original (same discipline as `pkg/lang` vendoring), and
   `PROVENANCE`-style notes record what was rewritten vs copied.
5. **All lazy, opt-in, good in both modes (owner mandate, 2026-07-27).** None of
   these is a boot source. They follow the `clojure.core.async` / `bri.*`
   pattern, not the always-boot satellites:
   - **Interpreter:** a lazy lib provider (`pkg/eval` / `pkg/briloader` shape)
     loads the namespace on first `(require …)` — the boot budget (ADR 0024) is
     untouched, so adding libraries never slows REPL/`cljgo run` startup.
   - **AOT:** a generated twin (the `genbri`/`pkg/briaot` opt-in-linking machinery,
     ADR 0074/0076) links **only when the program requires the namespace** — a
     binary that never uses `data.json` pays **zero** bytes for it. This is
     verified, not hoped: measured live 2026-07-27, a hello that requires nothing
     is 6.4 MB with **0** `cljg.net.http` symbols; `TestDbIsOptIn` /
     `TestOtelIsOptIn` / `TestNoInterpreterInCompiledBinary` enforce it in CI.
   - Net: everything is **available by default** (`require` and go — no dependency
     to add, Bun-style), yet **nothing unused ships and nothing unused slows
     boot**. This is the resolution of "all core available" + "remove all non-used"
     + "interpreted must be fast."

### 3. Process — spike per lib, then one openspec change

Per ADR 0027: a feasibility spike per library (**s52** tools.cli, **s53**
data.json, **s54** core.match, **s55** data.csv) captures the real oracle
behavior and proves the port path, closing with a `VERDICT.md`. Then **one**
openspec change implements the tier, archived when the conformance suite is
green in both harnesses. ADR numbers **0097 (this ADR); the program owns no other reserved block** for this program
(0097 = this umbrella; per-lib ADRs split out only if a library's port raises
its own decision).

## Consequences

- **cljgo gains the everyday contrib surface natively** — JSON, CLI parsing,
  pattern matching, CSV — no Java, AOT-compilable, oracle-frozen. This is the
  "batteries" the Bun-of-Clojure direction promises, delivered where consumption
  (ADR 0095) structurally cannot.
- **0095 and 0097 are complementary, not redundant:** native for the tier-1
  everyone uses; consume for the pure long tail. Documented as one story.
- **Maintenance cost is real and bounded:** four libraries tracked against
  upstream. The conformance freeze makes drift detectable; we pin to a stated
  upstream version per lib and bump deliberately.
- **No precedence violation:** exact names, exact APIs, semantics frozen to the
  oracle — the same contract `clojure.string` already honors.
- **Risk, per lib:** `core.match` (a non-trivial macro compiler) is the most
  likely to need more than one pass or a scoped-down first cut; s54 decides how
  much of it lands in tier-1 vs a follow-up.

## Spikes

| spike | library | question | status |
|---|---|---|---|
| **s52** | `clojure.tools.cli` | Does the pure upstream source run on cljgo unmodified, and does `parse-opts` match the oracle across the option-spec surface? | pending |
| **s53** | `clojure.data.json` | Can the reader/writer be rewritten pure-Clojure/Go-host with byte-identical `read-str`/`write-str` vs oracle (incl. escaping, bignum, key-fn options)? | pending |
| **s54** | `clojure.core.match` | How much of the match macro compiler ports cleanly, and where is the pure/Java line — full tier-1 or a scoped first cut? | pending |
| **s55** | `clojure.data.csv` | Does the reader/writer port with oracle-matching quoting/separator/newline behavior? | pending |

Each closes with a `VERDICT.md` per ADR 0027 §2. Implementation follows via one
`/opsx:propose` change once the spikes confirm the port paths.
