# s55 — `clojure.data.csv` native port · VERDICT

**Status: MET** (per ADR 0027 §2). The planned strategy — "port/rewrite
`read-csv` over a char reader and `write-csv`, matching oracle quoting /
separator / quote-char / newline behavior exactly" — is **confirmed as
executed**, for **String** input and any `*out*`-bindable sink. All 20 probed
behaviors reproduce JVM `data.csv` 1.1.0 byte-for-byte on cljgo.

## What lands in tier-1
- Full public API, exact names, exact option keys:
  - `(read-csv input & {:keys [separator quote]})` — **String** input, lazy seq
    of vectors.
  - `(write-csv writer data & {:keys [separator quote quote? newline]})` —
    emits to `*out*` (bind `writer`); `with-out-str` captures.
- The entire reader state machine and writer quoting/escaping logic, ported
  verbatim in shape. Reuses cljgo's already-shipped `clojure.string/escape`.

## Confirmed pure/Java split
Single namespace; no ns-level split. Within the file: read state machine +
write path are **pure Clojure**; the JVM `PushbackReader`/`StringReader` become
a pure one-slot-pushback string reader; the JVM `Writer` becomes `*out*`. See
README table.

## Go host needs
**None required for the landing scope.** Two OPTIONAL follow-ups, each a small
host fn, both out of tier-1 scope for this cut:
1. A streaming char-reader (`cljg.io`) to accept a non-String `java.io.Reader`
   in `read-csv` (line-by-line / chunked). The port already has the clean seam:
   `read-csv` dispatches on `(string? input)`.
2. A first-class streaming `Writer` sink for `write-csv` (file/socket) beyond
   what `*out*` binding already gives. Not blocking — file output via `*out*`
   already exists in cljgo.

## Oracle behaviors frozen
**14** in `draft-conformance-data.csv.clj` (9 read + 5 write), each copied from
the JVM oracle and reproduced by the port. The wider spike driver exercised 20.

## Integration steps (for the openspec change under ADR 0096)
1. **`core/`**: add `core/data_csv.cljg` from `draft-data_csv.cljg` (keep the
   EPL header). Follow the satellite convention (CLAUDE.md / MEMORY): open with
   `(clojure.core/in-ns 'clojure.data.csv)` + `(clojure.core/refer 'clojure.core)`;
   the loader creates the ns bare. Embed it in `core/` (Go `embed`, like
   `AsyncSource`).
2. **Interpreted loader (`pkg/eval`)**: add a lazy lib provider mirroring
   `pkg/eval/asyncload.go` — register `clojure.core.async`-style provider for
   `clojure.data.csv`, guarded by an interned marker var (e.g. `read-csv`), so
   `(require 'clojure.data.csv)` loads the embedded source once. NOT a boot
   source (ADR 0024 — boot budget untouched). Wire it in `eval.New` alongside
   `registerAsyncProvider`.
3. **coreaot AOT twin**: generate `pkg/coreaot/cljdatacsv/` (the satellite AOT
   package, sibling to `cljstring`/`cljdata`/`cljedn`/…) and register it in
   `pkg/coreaot/load.go` so AOT binaries that `(require 'clojure.data.csv)` link
   the compiled namespace. Keep it opt-in per ADR 0074/0076 — programs that
   never require it pay nothing; verify `pkg/coreaot/imports_test.go`
   TestNoInterpreterInCompiledBinary still holds.
4. **Conformance harness**: split `draft-conformance-data.csv.clj` into
   per-behavior `conformance/tests/data-csv-*.clj` files (one form + one
   `;; expect:` each, string-test style), with the oracle citation header.
   Ensure they run in BOTH harnesses (REPL + AOT) from M2 — REPL-vs-binary
   divergence is a release blocker.
5. **Gates**: `go build ./... && go vet ./... && gofmt -l pkg cmd conformance
   templates && go test ./...` all green (integrator's job; not run in this
   spike). Add the new `cljdatacsv` package to whatever generator/manifest lists
   the satellites.
6. **Docs**: note `clojure.data.csv` in the native-contrib list (batteries),
   with the two documented scope caveats (String-only read input; malformed-
   input message text not byte-frozen).

## Risks / residual unknowns
- **Reader input**: streaming `java.io.Reader` is unported (String-only). A user
  passing a Reader gets a clear error, not a wrong result. Low risk; deferrable.
- **Malformed-input messages**: `ex-info` text diverges from the JVM's
  `EOFException`/`%c` wording. If any conformance test freezes an error string,
  it must freeze cljgo's, not the oracle's. Do not freeze the byte-text of the
  malformed-input path against the JVM.
- **`str/escape` dependency**: the port leans on cljgo's `clojure.string/escape`
  — already shipped and oracle-verified, so no new risk, but the AOT twin must
  ensure `clojure.string` is linked when `data.csv` is.
- **Perf**: cell accumulation is immutable-string `str` (O(n²) worst case on a
  giant unbroken cell). Fine for the conformance corpus; if a perf budget is
  added, swap to a transient char vector + `apply str`. Not a correctness risk.
- **`load-file` unusable in cljgo**: irrelevant to integration (the loader is
  the real path) but noted so no one wires the satellite via `load-file`.
