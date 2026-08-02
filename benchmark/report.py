#!/usr/bin/env python3
"""Render benchmark/.build/results/*.json (hyperfine exports) into a markdown
table. Columns are the two cljgo legs first, then comparables. Best wall-clock
per row is bolded. Missing cells (runtime not installed / skipped) show as —."""
import json, os, sys

AOT = "--aot" in sys.argv
OUT = os.path.join(os.path.dirname(__file__), ".build", "results-aot" if AOT else "results")
BENCHES = ["tak", "fib", "loop-recur", "persistent-map", "map-filter", "transducers", "reduce"]
if AOT:
    ORDER = ["cljgo-aot", "glojure-aot", "letgo-aot"]
else:
    ORDER = ["cljgo-run", "cljgo-aot", "let-go", "glojure", "babashka", "joker", "clojure-jvm"]


def load(name):
    p = os.path.join(OUT, f"{name}.json")
    if not os.path.exists(p):
        return None
    return {r.get("command", "?"): r["mean"] * 1000 for r in json.load(open(p))["results"]}


def fmt(ms):
    if ms is None:
        return "—"
    return f"{ms/1000:.2f} s" if ms >= 1000 else f"{ms:.1f} ms"


rows = {n: load(n) for n in ["startup"] + BENCHES}
rows = {k: v for k, v in rows.items() if v}
present = [rt for rt in ORDER if any(rt in r for r in rows.values())]

print("| Benchmark | " + " | ".join(present) + " |")
print("|" + "---|" * (len(present) + 1))
for n in ["startup"] + BENCHES:
    r = rows.get(n)
    if not r:
        continue
    best = min(v for v in r.values() if v is not None)
    cells = []
    for rt in present:
        v = r.get(rt)
        s = fmt(v)
        if v is not None and v == best:
            s = f"**{s}**"
        cells.append(s)
    print(f"| `{n}` | " + " | ".join(cells) + " |")

if AOT:
    print("""
All three columns are native binaries compiled from the same programs —
let-go's own benchmark suite (github.com/nooga/let-go), vendored unmodified
(hyperfine, 3 warmup / 10 timed runs, wall-clock mean, startup included;
compile time not measured). `cljgo-aot` = `cljgo build`. `glojure-aot` =
gloat `-E glj` (Glojure Clojure→Go→native). `letgo-aot` = gloat `-E lglvm`
(let-go IR lowered to Go with the VM runtime linked — gloat's pure `lgl`
engine is not implemented yet). Interpreted legs (cljgo run, glj, lg,
babashka, joker, Clojure JVM) are deliberately absent here; see
`results.md` for that comparison.

Measured 2026-08-02 on darwin/arm64 (Apple M5 Pro, go1.26.3): cljgo
**post-v0.8.9**, commit `f46b9a8`, rebuilt from this repo at HEAD with the
repo Go toolchain. **All three columns were rebuilt for this run** — the
2026-07-24 competitor artifacts no longer exist, so `glojure-aot` and
`letgo-aot` were re-built from source with gloat v0.1.62 (pinning Glojure
v0.7.0 and let-go v1.12.2, gloat's own pinned Go toolchain) and timed in the
same hyperfine session on the same machine. let-go's `transducers` again
needed gloat's pure-retry fallback (its LG-overrides pass failed to build).

Run-to-run σ in this session was 0.3–1.0 ms, so four rows — `startup`,
`loop-recur`, `persistent-map`, `map-filter` — are **ties inside the noise**
between cljgo and Glojure; the bolding marks the arithmetic minimum, not a
win. The rows that are outside the noise: cljgo wins `tak` (1.45×) and `fib`
(1.54×); Glojure wins `transducers` (1.47×) and `reduce` (3.67×).

Compare cells WITHIN this table only. The rebuilt competitor binaries do not
reproduce the 2026-07-24 artifacts (Glojure's `reduce` read 23.1 ms then and
7.4 ms now; its stripped binary was recorded at 7.5 MB then and measures
19.0 MB now — same pinned versions, same `-tags glj_aot_runtime -ldflags
"-s -w"` build). The old artifacts are gone, so the discrepancy cannot be
resolved; only the numbers above were actually measured.

None of the seven programs prints its result, so nothing in the harness
proves a compiler did not elide the work. Checked by hand for the one row
where it mattered: a printing variant of `reduce` at N = 1M / 4M / 16M gives
the correct sum on both and scales linearly on both — Glojure ~1.0 ns per
element, cljgo ~21.8 ns. The `reduce` gap is real, not dead-code
elimination.""")

if not AOT:
    print("""
Measured 2026-08-02 on darwin/arm64 (Apple M5 Pro, go1.26.3), hyperfine
3 warmup / 10 timed runs, wall-clock mean, startup included. Versions:
cljgo **post-v0.8.9**, commit `f46b9a8`, rebuilt from this repo at HEAD ·
let-go `main` @ `0e56abd` (2026-07-14, untagged — after v1.11.1, includes
the loop-fusion pass), built from source with the same toolchain and flags ·
babashka v1.12.218 · joker v1.9.0 · Clojure CLI 1.12.5.1645 on OpenJDK
26.0.1. joker has no `transducers` and is skipped on `fib`/`tak`. Glojure
is absent from this table by construction; its AOT leg is in
`results-aot.md`. Run-to-run σ was under 1 ms on every sub-10 ms cell, so
`startup` (cljgo-aot 5.50 vs let-go 5.51) and `map-filter` are ties, not
wins. Compare cells WITHIN this table only.""")
