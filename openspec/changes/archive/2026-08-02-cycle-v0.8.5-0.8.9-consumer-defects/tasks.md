# tasks — cycle-v0.8.5-0.8.9-consumer-defects

**Backfilled 2026-08-02 from shipped code** (`2afcf83` v0.8.6, `3a924a3`
v0.8.8). Boxes are checked because the work is released.

Gate: `CGO_ENABLED=0 go build ./... && go vet ./... && gofmt -l pkg cmd conformance templates core && go test ./... -timeout 1800s -p 1`

## 1. One source-extension list (#182)
- [x] 1.1 `pkg/eval.SourceExts` exported, most-specific-first `{.cljgo, .cljg, .clj, .cljc}`.
- [x] 1.2 `ResolveLibPath` reads it (it was already the resolver's own list).
- [x] 1.3 `nsNameFor` in `cmd/cljgo/bri.go` reads it — the hand-written `TrimSuffix(TrimSuffix(rel, ".cljg"), ".clj")` deleted, with the reason recorded at the call site.
- [x] 1.4 Unit: `cmd/cljgo/testharness_cljc_test.go` pins every accepted extension.
- [x] 1.5 End-to-end: a `.cljc` suite through the real binary on BOTH legs — asserting only the compiled one would miss a fix that broke the interpreted one. Confirmed to fail with the reported error when the old line is restored.

## 2. Maven repository order (#187)
- [x] 2.1 `DefaultMvnRepos` = Central, then Clojars, matching tools.deps; the false comment claiming the old order was the tools.deps default deleted.
- [x] 2.2 `TestDefaultReposMatchToolsDeps` pins the order WITH the citation (tools.deps 0.22.1492, `clojure/tools/deps/util/maven.clj:161-165`) so it cannot silently drift back.
- [x] 2.3 `(mvn-repo …)` still prepends — a project can override.
- [x] 2.4 Exclusion stated in `spikes/s79-deps-edn-deps-translation/RESULTS.md`: this is a source-to-source argument plus a pinned invariant, NOT an observed mis-resolution. The consequence is fetch-time and needs the network to demonstrate.

## 3. Gates
- [x] 3.1 Full gate green for both fixes; released in v0.8.6 and v0.8.8.
- [x] 3.2 Network integration test for the deps change: `CLJGO_CLOJARS_IT=1 go test ./pkg/deps/ -run TestClojarsIT -v` (required before any release touching dependency resolution).
