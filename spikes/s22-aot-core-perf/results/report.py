import json, os, sys

BENCHES = ["tak", "fib", "loop-recur", "persistent-map", "map-filter", "transducers", "reduce"]
ORDER = ["cljgo", "let-go", "babashka", "joker", "clojure-jvm"]


def load(f):
    p = f"full-{f}.json"
    if not os.path.exists(p):
        return None
    out = {}
    for r in json.load(open(p))["results"]:
        out[r["command"] if r.get("command") else "?"] = r
    # hyperfine stores the -n name in "command" when --style basic + -n used
    return out


def fmt(ms):
    if ms is None:
        return "—"
    if ms >= 1000:
        return f"{ms/1000:.2f} s"
    return f"{ms:.1f} ms"


rows = {}
for f in BENCHES + ["startup"]:
    d = load(f)
    if not d:
        continue
    rows[f] = {k: v["mean"] * 1000 for k, v in d.items()}

print("| Benchmark | " + " | ".join(ORDER) + " |")
print("|---" * (len(ORDER) + 1) + "|")
for f in ["startup"] + BENCHES:
    if f not in rows:
        continue
    r = rows[f]
    best = min(r.values())
    cells = []
    for rt in ORDER:
        v = r.get(rt)
        s = fmt(v)
        if v is not None and v == best:
            s = f"**{s}**"
        cells.append(s)
    print(f"| `{f}` | " + " | ".join(cells) + " |")
