# ADR 0027 — Spike lifecycle: spike → close → ADR → spec → apply
Date: 2026-07-16 · Status: accepted (owner mandate)

## Context

CLAUDE.md froze `spikes/` as read-only history after the S1–S11 exploration
phase. That rule protected closed spikes from being edited, but it also
banned NEW spikes — and the owner's adoption mandate (2026-07-16) is
explicit: people will move to cljgo only if decisions are de-risked in the
open, in order — "create spikes first and then close it and put adr and
then spec properly." Feature batches landing straight on main answer
"can we", never "should we, and at what cost" — a spike does.

## Decision

Every non-trivial feature runs this pipeline. Nothing skips a stage; only
bug fixes and ADR-0022-style breadth execution (decisions already made) go
straight to a branch.

1. **Spike** — `spikes/sNN-slug/` (numbering continues from S11).
   Throwaway code answering ONE question. The exit criterion — what
   measurable result closes the spike — is written in its README BEFORE
   any code.
2. **Close** — `VERDICT.md` in the spike dir: what was tried, what was
   measured, the recommendation. A closed spike is frozen forever — this
   is what the old rule protected, and it still holds.
3. **ADR** — `docs/adr/` records the decision, citing the spike as
   evidence. A spike without a subsequent ADR is a spike that closed "no".
4. **Spec** — `/opsx:propose` turns the ADR into OpenSpec deltas + tasks.
5. **Apply** — implementation branch → PR → gates → merge. Spike code
   NEVER merges into pkg/; it only informs.

## Consequences

CLAUDE.md's fence changes from "spikes/ is read-only history — new work
never goes there" to "CLOSED spikes are frozen; new spikes follow ADR
0027". Reserved next: S12 build-from-binary (→ ADR 0028), S13 numeric
tower divergences (→ 0029), S14 format grammar (→ 0030), S15 nREPL
(→ 0031).
