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
All three columns are native binaries compiled from the same programs
(hyperfine, 3 warmup / 10 timed runs, wall-clock mean, startup included;
compile time not measured). `cljgo-aot` = `cljgo build`. `glojure-aot` =
gloat `-E glj` (Glojure Clojure→Go→native). `letgo-aot` = gloat `-E lglvm`
(let-go IR lowered to Go with the VM runtime linked — gloat's pure `lgl`
engine is not implemented yet). Interpreted legs (cljgo run, glj, lg,
babashka, joker, Clojure JVM) are deliberately absent here; see
`results.md` for that comparison.

Measured 2026-07-24: cljgo @HEAD (repo Go toolchain) · gloat v0.1.62
pinning Glojure v0.7.0 and let-go v1.12.2 (gloat builds with its own
pinned Go toolchain). let-go's `transducers` needed gloat's pure-retry
fallback (its LG-overrides pass failed to build).""")
