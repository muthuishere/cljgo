# ADR 0008 — Process: ADR → OpenSpec propose/design → apply, gates always
Date: 2026-07-12 · Status: accepted

## Context
Owner wants decisions and specs captured before code, on everything.

## Decision
Non-trivial changes: (1) ADR in docs/adr/ when a decision is made or reversed;
(2) OpenSpec change (proposal + design + spec deltas) in openspec/changes/;
(3) apply via tasks; archive on completion. Trivial fixes skip OpenSpec.
Nothing skips gates: go build/vet/gofmt/test green + conformance for new
semantics. Milestone stages (M2+) each get an OpenSpec change up front.

## Consequences
docs/adr is the decision log (append-only, supersede not edit); openspec/specs
accumulates the living spec of cljgo as changes archive.
