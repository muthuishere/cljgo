# ADR 0003 — Vendor Glojure's pkg/lang as the runtime seed, own it fully
Date: 2026-07-11 · Status: accepted

## Context
Persistent data structures (HAMT, 32-way trie vector, lazy seqs, vars) are
~16k lines of subtle code. Glojure's pkg/lang passed the Clojure compat suite.
Owner: "I have a separate taste and it should go like it, but we will use it."

## Decision
Hard-fork (vendor) Glojure pkg/lang @ c74bc07d (EPL-1.0 headers kept), then
reshape freely: interpreter glue deleted, all external deps removed
(go4.org/intern → unique.Handle), Equiv/Equals split fixed (M0-A), transients
pending (TODO.md). The language surface and design remain entirely ours.

## Consequences
Months saved; EPL applies file-scoped to vendored files only. Surgery logged
in PROVENANCE.md / spike S4's SURGERY.md. Known upstream defects fixed by us,
not waited on.
