# ADR 0024 — Perf budgets are host-relative, not absolute
Date: 2026-07-15 · Status: accepted · Refines: ADR 0019

## Context

ADR 0019 set `TestBootUnderBudget` (pkg/eval/boot_test.go) to a **250ms**
wall-clock ceiling, calibrated on the owner's Apple-silicon laptop. That number
was never run anywhere else — the repo had no CI. Adding CI (this change) ran it
on shared GitHub runners for the first time, and the budget failed everywhere:

| host | boot | vs 250ms |
|------|------|----------|
| owner's Mac (local) | 181ms | passes, 38% headroom |
| `macos-latest` runner | 349ms | **fails** (1.4×) |
| `ubuntu-latest` runner | 3.55s | **fails** (14×) |

The code is identical; only the host changed. A wall-clock threshold calibrated
on one machine is not a property of the code, so enforcing it verbatim on a
shared, contended, unspecified runner tests the runner, not cljgo.

This collides with two standing commitments:

- **design/00 §1.4 / owner mandate** — "performance is a feature"; perf budgets
  are CI-checked like tests. Deleting the gate, or skipping it on CI, would
  quietly retire that mandate the moment CI existed.
- **ADR 0019's own stated purpose** — the budget exists to catch a
  *pathological* regression (an O(n²) blowup, a runaway realization, a per-form
  network/disk hit), explicitly *not* to police the expected linear cost of a
  larger core. Pathology is orders of magnitude; runner variance is a small
  constant factor. One threshold cannot separate them across hosts.

## Decision

Keep the gates on CI (the mandate holds), but make their ceilings
**host-relative**: each reads an env override and keeps its locally-calibrated
default when unset.

**Boot budget** — `CLJGO_BOOT_BUDGET` (any `time.ParseDuration` string),
defaulting to ADR 0019's **250ms**.

- **Local / dev machines**: unchanged — 250ms, tight, the real regression alarm.
- **CI**: `CLJGO_BOOT_BUDGET=5s`. Chosen as ~1.4× the worst observed runner
  (3.55s) — loose enough to absorb runner variance, still 20× under any genuine
  pathology, which manifests as seconds-to-minutes, not a constant factor.

The gate therefore still fails CI on an O(n²) blowup, which is what ADR 0019
says it is for.

## Consequences

The precise instrument for tracking boot cost is `BenchmarkBoot`, not this test —
a benchmark is comparable only against itself on the same host, which is exactly
the property a wall-clock budget lacks. If boot regression tracking ever needs to
be *quantitative* on CI, the answer is benchmark-vs-baseline comparison on a
pinned runner, not a tighter absolute number.

**Open question, deliberately not resolved here:** `ubuntu-latest` boots ~20×
slower than local, where `macos-latest` is only 2×. That is far outside plausible
single-core variance and is *unexplained*.

It is also **reproducible, not runner noise**: two independent runs, on separate
workflows, measured 3.55s and 3.00s. A contended-runner fluke would not land
twice in the same narrow band. That weakens the "the runner was busy"
explanation and strengthens the possibility of a real Linux-specific pathology
in the boot path — plausibly GC behavior on an allocation-heavy tree-walk boot
under a low-core-count Linux runner.

This ADR deliberately does not paper over it: the 5s CI budget keeps the gate
meaningful today, and the anomaly is tracked as its own investigation. If it
turns out to be a real defect, the fix belongs in the boot path and this ceiling
should drop back toward the local number. Next step: `BenchmarkBoot -benchmem`
on an ubuntu runner vs locally — if allocations/op match while wall-time diverges
20×, it is the host; if allocations diverge, it is ours.
