## 1. Implementation

- [x] 1.1 `pkg/eval/format_builtins.go`: directive parser (regex-based,
  argument-index resolution, flag/width/precision extraction) adapted from
  spikes/s14-format-grammar/spec.go (read-only reference; not edited).
  Gates: build/vet/gofmt/test green.
- [x] 1.2 Translate-then-delegate renderer adapted from
  spikes/s14-format-grammar/translate.go + common_render.go: delegate
  `d x o c s f e` (+ upper variants) to `fmt.Sprintf` through the per-verb
  `goFlags` allow-list; hand-render `b`/`g`/`,`/`(`/`%n`/`%%`; cljgo types
  (`lang.Char`, `*lang.BigInt`, `*lang.Ratio`) substituted for the spike's
  plain `rune`/`*big.Int`/`*big.Rat` stand-ins; `%s` display text reuses
  `lang.ToString` (bit-exact Java-`Double.toString` already lives there,
  pkg/lang/strconv.go `formatFloat`) rather than re-deriving it. Gates:
  build/vet/gofmt/test green.
- [x] 1.3 Typed format errors (Java-exception-shaped messages: Unknown
  FormatConversionException / IllegalFormatConversionException / Missing
  FormatArgumentException / DuplicateFormatFlagsException / IllegalFormat
  FlagsException) — never let an unvetted flag/verb combo reach
  `fmt.Sprintf` (the ADR's stated invariant); add a Go unit test asserting
  this directly (not just corpus-driven). Gates: build/vet/gofmt/test green.
- [x] 1.4 Register `format` and `printf` as core builtins
  (`e.internFormatBuiltins(def)`, one line in `internBuiltins`, same wiring
  convention as every other builtins file); `printf` = `(print (format ...))`
  writing through the existing `eval.Out` (`fmt.Fprint(Out, ...)`, same path
  `println` uses at pkg/eval/builtins.go — no new write surface, no
  `*out*`/`with-out-str` work (another change owns that). Gates:
  build/vet/gofmt/test green.

## 2. Conformance

- [x] 2.1 Re-run the spike's 80-probe corpus against the real `clojure` CLI
  1.12.5 at freeze time (do not trust spike-recorded outputs verbatim);
  author `conformance/tests/format-*.clj` — one behavior grouping per file,
  `;; expect:` for passing probes (pr-str of the returned string) and
  `;; expect-error:` for the throwing probes, each citing the oracle
  verification per repo convention. Gates: build/vet/gofmt/test green,
  `go test ./conformance/...` green.
- [x] 2.2 Confirm dual-harness: at least one `format-*.clj` file runs
  through both the eval harness and the Go-emitter/compiled-binary harness
  with identical output (no emitter waiver needed — plain Var/IFn call).
  Gates: build/vet/gofmt/test green.
- [x] 2.3 `go run ./cmd/cljgo suite -v`: confirm the upstream
  `clojure-test-suite/test/clojure/core_test/format.cljc` assertions now
  pass (previously skipped/erroring — no `format` fn existed). Record
  before/after suite output in the PR description.

## 3. Wrap-up

- [x] 3.1 Full gate run: `go build ./... && go vet ./... && gofmt -l pkg cmd
  conformance && go test ./...`. `git fetch origin && git merge origin/main`
  before opening the PR (expect keep-both conflict resolution on
  `internBuiltins` registration lines against concurrently-landing numeric/
  hierarchies batches — do not touch their code).
