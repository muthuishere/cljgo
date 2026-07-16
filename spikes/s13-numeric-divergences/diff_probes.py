#!/usr/bin/env python3
"""Align oracle vs cljgo probe output by LABEL and flag mismatches.

Each line of output looks like "LABEL => VALUE-OR-THREW:message". We parse
by the first " => " and match rows by LABEL (not position), since cljgo may
abort partway through a file and emit fewer lines than the oracle.
"""
import sys
from pathlib import Path

DIR = Path(__file__).parent
OUT = DIR / "out"


def parse(path):
    rows = {}
    order = []
    if not path.exists():
        return rows, order
    for line in path.read_text().splitlines():
        if " => " not in line:
            continue  # stray stderr / compiler-error line
        label, val = line.split(" => ", 1)
        rows[label] = val
        order.append(label)
    return rows, order


def main():
    total = 0
    mismatches = 0
    for stem in ("probes", "probes_abs"):
        oracle, order = parse(OUT / f"{stem}.oracle.txt")
        cljgo, _ = parse(OUT / f"{stem}.cljgo.txt")
        print(f"\n=== {stem} ===")
        for label in order:
            total += 1
            ov = oracle.get(label, "<MISSING>")
            cv = cljgo.get(label, "<MISSING-or-aborted-before-this-line>")
            if ov != cv:
                mismatches += 1
                print(f"DIVERGE  {label}")
                print(f"   oracle: {ov}")
                print(f"   cljgo : {cv}")
    print(f"\n{mismatches}/{total} probes diverge")


if __name__ == "__main__":
    main()
