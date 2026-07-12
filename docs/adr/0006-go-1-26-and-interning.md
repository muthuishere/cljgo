# ADR 0006 — Pin Go 1.26; modern-Go features are load-bearing
Date: 2026-07-11 · Status: accepted

## Context
Owner: use the latest Go. Machine has go1.26.3; vendored refs compile under it.

## Decision
Our module and every generated go.mod pin go 1.26. Deliberate uses: unique.Handle
for keyword/symbol interning (identity ==, weakly held — replaces go4.org/intern);
iter.Seq as the ISeq↔Go bridge; generics for typed fast paths; testing/synctest
for async conformance; swiss maps free for internal tables.

## Consequences
No external interning dep; S4 verified identity across packages/goroutines at
1.6ns/compare. Emitted code requires a current toolchain — acceptable.
