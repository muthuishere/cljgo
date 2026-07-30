# ADR 0097 — Clojure contrib tier-1 native ports: MERGE PLAN

Four libs were each built to green in its OWN git worktree. Each independently
edited the same shared registries (`pkg/bri/bri.go` `Specs()`, the `core/*.go`
embeds, `pkg/briloader/briloader.go`, the regenerated `pkg/briaot/briaot.go`
umbrella + AOT twins). Those edits **will conflict on a naive merge and must be
stitched by hand**, then the generated files re-generated ONCE.

All four are green in isolation. This plan tells the integrator exactly which
files are safe to copy verbatim, which shared edits to re-apply once, and the
single regenerate + full-gate run to finish.

---

## 0. Two things to reconcile BEFORE you start

### 0a. ADR number collision — the worktrees branched from a stale base

The worktrees were cut when `docs/adr/` ended at 0094. The repo has since
advanced to **0102**, and already contains a committed umbrella
`docs/adr/0097-clojure-contrib-tier1-native.md` (plus an unrelated
`0096-go-stdlib-interop.md`). Two worktrees wrote their OWN 0096:

- data.json → `docs/adr/0096-clojure-data-json-native.md`
- core.match → `docs/adr/0096-clojure-core-match.md`

tools.cli and data.csv deliberately did **not** author an ADR (they anticipated
the collision).

**Action:** do NOT copy either worktree's `0096-*.md`. Either (a) fold their
context/decision/consequences into the already-committed umbrella
`0097-clojure-contrib-tier1-native.md`, or (b) give data.json and core.match
their own fresh numbers from the current free range (**0103+**). Pick one; do
not land two files named `0096-*`.

### 0b. `providerLoad` reload fix — proposed THREE times, apply ONCE

`pkg/briloader/briloader.go` `providerLoad` (currently line ~69) still carries
the OLD guard:

```go
if loaded[s.Name] { mu.Unlock(); return }
```

tools.cli, data.csv, and data.json **each independently** discovered the same
latent bug: the process-global `loaded` map assumed `loaded ⇒ namespace
present`, which the conformance eval harness's `removeNewNamespaces` teardown
violates — so the 2nd+ conformance file that requires a provider-backed
namespace never reloads it. clojure.tools.cli / data.csv / data.json are the
FIRST provider-backed lazy namespaces with conformance tests, so they are first
to hit it.

**Action:** apply the fix EXACTLY ONCE. Gate the once-flag on the namespace
still existing (reload when `loaded[s.Name]` but `FindNamespace(name) == nil`).
This is a no-op in production (namespaces are never removed there) and is
confirmed regression-clean by the full eval harness in all three worktrees.
Do not apply it three times.

---

## 1. clojure.tools.cli — GREEN

Pure-Clojure native satellite, no Go host (arg parsing is one-shot, not a hot
path). Non-OptIn, lazy in interpreter / opt-in-linked in AOT.

### NEW files — safe to copy verbatim
- `core/tools_cli.cljg` (the port, EPL header kept)
- `core/contrib.go` (embed `ClojureToolsCLISource`) — **new file, no conflict**
- `conformance/tests/tools-cli.clj`
- `pkg/briaot/toolscli/toolscli.go` — generated twin; will be **overwritten by
  the regenerate in §5**, so copying is optional
- `spikes/s52-tools-cli-native/` (frozen spike, reference only)

### SHARED-registry edits — re-apply once
- `pkg/bri/bri.go` `Specs()`: append one row at the END —
  `{Name:"clojure.tools.cli", File:"tools_cli.cljg"(or its embed path), Pkg:"toolscli", Source:&core.ClojureToolsCLISource, install:nil}` — non-OptIn.
- `pkg/briaot/briaot.go`: regenerated, do NOT hand-merge (see §5).
- `pkg/briloader/briloader.go`: only the shared providerLoad fix (§0b) — nothing
  tools.cli-specific.

### Perf
n/a — one-shot argument parsing, no Go host primitive, no benchmark.

### Residual
Deferred (deliberate, per spike VERDICT): `:parse-fn` exception strings not
frozen; `:post` maps rewritten as explicit asserts (cljgo assert error, not JVM
AssertionError); legacy `cli` raises ex-info where upstream threw bare
Exception; `*assert*`-gated unknown-key warning not exercised. Did not run full
`./...` (integrator does at merge).

---

## 2. clojure.data.csv — GREEN

Native port of data.csv 1.1.0. No Go host primitive — pure Clojure over
`clojure.string/escape` (a boot namespace, always linked). `read-csv` takes a
String; `write-csv` RETURNS a String (Mandate A: byte-identical to upstream's
StringWriter output). Non-OptIn (zero dependency).

### NEW files — safe to copy verbatim
- `core/data_csv.cljg` (the port, EPL header from Jonas Enlund, opens with
  fully-qualified `in-ns` + `refer`, no `ns` form)
- `conformance/tests/data-csv-read.clj`
- `conformance/tests/data-csv-write.clj`
- `conformance/tests/data-csv-malformed.clj`
- `pkg/briaot/cljgdatacsv/cljgdatacsv.go` — generated twin, overwritten in §5

### SHARED-registry edits — re-apply once
- `core/cljg.go`: add embed `CljDataCSVSource`. **⚠ this edits an EXISTING
  shared file** (unlike tools.cli's new `contrib.go`) — hand-stitch the one
  `//go:embed` + `var` line; do not take the whole worktree's `cljg.go`.
- `pkg/bri/bri.go` `Specs()`: append one row at the END —
  `{Name:"clojure.data.csv", ..., Pkg:"cljgdatacsv", Source:&core.CljDataCSVSource, install:nil}` — non-OptIn.
- `pkg/briaot/briaot.go`: regenerated (§5).
- `pkg/briloader/briloader.go`: only the shared providerLoad fix (§0b).

### Perf
n/a per task (pure Clojure, no perf budget this increment).

### Residual
`write-csv` intentionally deviates from upstream's `(writer, data, ...)` to a
pure `(data, ...) -> String` (cljgo has no `java.io.Writer`) — documented in
source + write conformance file. All three data.csv conformance files carry
`;; oracle: skip` (data.csv is not on the plain `clojure` CLI classpath, and the
write API can't run on the JVM) with JVM-captured expectations cited inline.

---

## 3. clojure.data.json — GREEN

Go host JSON codec (`pkg/bri/cljson/cljson.go`) — hand-rolled scanner + writer
bridging JSON ↔ cljgo `lang.*` values, exposed as private `-json-read` /
`-json-write` prims under a thin `core/data_json.cljg`. Tier-1 realization of
**MANDATE A** (hot scan/emit path is a Go primitive). Astral-plane round-trips
byte-identically via `utf16.DecodeRune`/`EncodeRune`. **OPT-IN** Spec (its codec
is isolated in `pkg/bri/cljson`, ShimImport). This is the most invasive of the
four — it touches the most shared files.

### NEW files — safe to copy verbatim
- `pkg/bri/cljson/cljson.go` (the Go JSON codec + shim installer, isolated pkg)
- `core/data_json.cljg` (thin data.json API)
- `core/data_json.go` (embed `DataJSONSource`) — **new file, no conflict**
- `pkg/briaot/cljjson/cljjson.go` + `pkg/briaot/cljjson/provider.go` — generated, §5
- `pkg/briaot/optin_linking_test.go` — **⚠ NOT new: this is an EXISTING shared
  test file.** The worktree ADDED `TestDataJSONIsOptIn` to it. Hand-stitch that
  one test func in; do not overwrite the file (core.match's gate output shows
  the existing Otel/Db/Secrets OptIn tests must survive).
- 13 conformance files: `conformance/tests/data-json-{read-basic,write-basic,
  key-fn,value-fn,numbers,escape-default,escape-opts,indent,astral-roundtrip,
  error-eof,error-eof-object,error-unexpected-char,error-invalid-number}.clj`

### SHARED-registry edits — re-apply once
- `pkg/bri/bri.go` `Specs()`: add an **OptIn** row (alongside data/otel/secrets)
  — `{Name:"clojure.data.json", ..., Pkg:"cljjson", Source:&core.DataJSONSource, install:nil, OptIn:true, ShimImport:"github.com/muthuishere/cljgo/pkg/bri/cljson"}`.
- `pkg/briloader/briloader.go`: (a) the shared providerLoad fix (§0b), AND (b)
  a **data.json-specific** blank-import of `pkg/bri/cljson` so the shim installer
  registers. Both edits needed.
- `cmd/genbri/main.go`: blank-import `pkg/bri/cljson` so genbri interns the shims
  during regenerate. **data.json-specific, required before §5's `go generate`.**
- `pkg/briaot/briaot.go`: regenerated (§5).
- `docs/adr/0096-clojure-data-json-native.md`: DO NOT copy — see §0a.

### Perf — MANDATE A verdict: WRITE meets, READ misses
Representative 16 KB mixed payload, 10k iters, delta-of-two-runs wall clock:
- **WRITE (`write-str`):** cljgo ~97 µs/iter vs JVM data.json 2.5.1 ~94 µs/iter
  — **dead heat. MANDATE A met on write.** (Two opts landed a 3x write speedup:
  seq-per-entry → `IMapEntry.Key()/Val()`, `fmt.Fprintf` → manual hex escaper.)
- **READ (`read-str`):** cljgo ~149 µs vs JVM ~102 µs — **JVM ~1.46x faster.
  MANDATE A NOT met on read.** The gap is persistent-structure allocation +
  boxing (JVM uses transients), not the scanner; a transient map/vector builder
  would close it. This is the one place cljgo does not beat/tie the JVM.
- Combined round-trip on the shipped AOT binary ~270 µs vs JVM ~168 µs (~1.6x);
  cljgo figures are through the tree-walk loop, JVM's are JIT-compiled.

### Residual
`read-str` ~1.5x JVM (transients gap, above). Streaming `io.Reader`/`io.Writer`
paths work but aren't separately conformance-tested (string paths are).
`:default-write-fn` implemented but not conformance-frozen — verify its contract
before relying on it. Object insertion order matches JVM only for ≤8-key objects
(array-map); larger objects use host-specific hash-map ordering, so conformance
objects are kept small. Error strings frozen eval-only (one shared Go shim ⇒
identical in REPL + binary).

---

## 4. clojure.core.match — GREEN

The REAL Maranget decision-tree compiler (NOT the rejected s54 linear scan) as a
Go host primitive, per **MANDATE A**. `pkg/bri/match.go` interns one `:private`
macroexpand-time prim `-match-compile` into `clojure.core.match`. `core/match.cljg`
is the thin macro surface (`match`/`matchv`/`matchm`/`match-let`). The compiler
runs at macroexpand time only and emits PLAIN clojure.core (let/if/=/nth/get/…),
so a compiled binary that uses match links **ZERO** match runtime (MANDATE B).
Non-OptIn (pure Go, no dep).

### NEW files — safe to copy verbatim
- `core/match.cljg` (thin macros)
- `core/match.go` (embed `CoreMatchSource`) — **new file, no conflict**
- `pkg/bri/match.go` (Maranget compiler + `installMatchShims`/`-match-compile`)
- `pkg/bri/match_test.go` (24 JVM-oracle behaviors + no-clause throw)
- `pkg/bri/match_bench_test.go` (tree-shape structural proof + tree-vs-linear timing)
- `pkg/briaot/corematch/corematch.go` — generated twin, overwritten in §5

### SHARED-registry edits — re-apply once
- `pkg/bri/bri.go` `Specs()`: append one row at the END —
  `{Name:"clojure.core.match", ..., Pkg:"corematch", Source:&core.CoreMatchSource, install:installMatchShims}` — non-OptIn.
- `pkg/briaot/briaot.go`: regenerated (§5).
- core.match does NOT touch briloader (compiler is macroexpand-time, no lazy
  provider load path exercised beyond the standard registry) — only the shared
  providerLoad fix from §0b applies, no core.match-specific briloader edit.
- `docs/adr/0096-clojure-core-match.md`: DO NOT copy — see §0a.

### NO conformance/tests/*.clj (by design)
The compiled conformance harness does not link lazy briaot providers, so — like
every bri/cljg namespace — the oracle-frozen behaviors live as bri-style Go
tests (`pkg/bri/match_test.go`, 24 behaviors). AOT-app correctness is covered
because app-compile macroexpands through the exact same `-match-compile`.

### Perf — MANDATE A verdict: REAL decision tree PROVEN (algorithmic mandate met)
MANDATE A here = "a real Maranget decision tree, NOT the rejected linear scan."
- **Structural proof (authoritative):** wide match (100 two-column clauses
  `[a b]`) expands to 110 equality tests vs ~200 a linear scan needs — column 0
  is tested once per 10-clause group, NOT re-tested per clause = a genuine tree.
- **Timed tree-vs-linear (same interpreter, worst-case a=9, 40k calls):** tree
  95.97 ms vs hand-written linear `cond` 1.010 s = **10.52x faster.** ✅
- **vs JVM org.clojure/core.match 1.1.1** (same wide match, 40k calls): JVM
  ~1010 ns/call (JIT) vs cljgo interpreter ~2400 ns/call — a ~2.4x absolute gap
  that is **tree-walk interpreter overhead, NOT a linear-scan penalty**;
  AOT-lowering the plain-core output to Go closes most of it. So cljgo does not
  beat JVM wall-clock in the interpreter, but the mandated algorithm (real tree,
  not scan) is delivered and structurally proven.

### Residual
Guard back-tracking is correct (fail-continuation), but a guard deep inside a
constructor duplicates the shared default via the hoisted thunk — fine for
typical matrices, not theoretically-minimal (no impact on the 24). `:seq`/map
patterns supported structurally without upstream's `:only`/rest-map refinements
and `:guard`-on-seq. Non-linear `[x x]` rejected exactly as upstream. Scoped OUT
by design (matches S50): regex/`java.util.Date`/array/binary matcher namespaces.

---

## 5. After stitching — regenerate ONCE, then full gate

Once all four `Specs()` rows, the four embeds, the single providerLoad fix, the
data.json `cljson` blank-imports (briloader + genbri), and the `optin_linking_test.go`
`TestDataJSONIsOptIn` addition are stitched, do NOT hand-merge any generated
file. Regenerate the umbrella + all four twins in one shot:

```
go generate ./pkg/briaot
```

This overwrites `pkg/briaot/briaot.go` and the four twin dirs
(`toolscli/`, `cljgdatacsv/`, `cljjson/`, `corematch/`) consistently, resolving
all four worktrees' conflicting hand-copies of the generated umbrella.

Then run the full project gate (the ADR-0096 twins + AOT drift + OptIn +
NoInterpreter + dual-harness conformance all in one; use the long timeout and
`-p 1` the conformance suite needs):

```
CGO_ENABLED=0 go build ./... \
  && go vet ./... \
  && gofmt -l pkg cmd conformance templates core \
  && go test ./... -timeout 1800s -p 1
```

Expected green signals (each proven in isolation, must hold together):
- `pkg/briaot`  — `TestGeneratedBriIsUpToDate`, `TestDataJSONIsOptIn`,
  `TestOtelIsOptIn`/`TestDbIsOptIn`/`TestSecretsIsOptIn`
- `pkg/coreaot` — `TestGeneratedCoreIsUpToDate`, `TestNoInterpreterInCompiledBinary`
- `pkg/bri`     — `TestCoreMatch` (24/24 + no-clause)
- `conformance` — Eval + Compiled dual harness for tools-cli, the 3 data-csv,
  the 12 data-json files

No worktree ran the full `./...` (each ran build+vet+gofmt + a targeted gate +
the eval harness). The full suite is the integrator's job here.

---

## 6. Recommended merge order

Land in the order of decreasing shared-file blast radius, so the most invasive
stitch happens against a clean tree and the rest stack cleanly on top:

1. **clojure.data.json FIRST.** It is the only OptIn lib and touches the most
   shared files (`Specs()` OptIn row, `briloader` providerLoad fix + cljson
   blank-import, `cmd/genbri` blank-import, `optin_linking_test.go` addition).
   Apply the shared providerLoad fix (§0b) HERE, once, as part of this landing —
   the other three then need no further briloader change.
2. **clojure.core.match** — pure-Go host prim, one `Specs()` row + new embed +
   its own Go tests; no briloader touch, no shared-test edit. Clean stack.
3. **clojure.tools.cli** — pure Clojure, one `Specs()` row + new `contrib.go`
   embed + one conformance file. Trivial.
4. **clojure.data.csv** — pure Clojure, one `Specs()` row + an embed line into
   the EXISTING `core/cljg.go` + three conformance files. Do it LAST so its
   `core/cljg.go` line-edit lands against a settled tree.

Run `go generate ./pkg/briaot` + the full gate (§5) ONCE after all four are
stitched — not per-lib.

Also settle §0a (ADR numbering) before committing so no two `0096-*.md` land.
