# cljgo — agent instructions

Clojure hosted on Go: compiler in Go, AOT-emits plain Go source (CLJS model),
tree-walk evaluator = the REPL + macro engine. Module `github.com/muthuishere/cljgo`, go 1.26.

## Authority chain (read in this order when deciding anything)

1. `docs/adr/` — decisions. Binding until superseded by a newer ADR.
2. `design/00-architecture.md` — cross-component contracts + M0–M5 roadmap.
3. `design/01–07` — component internals (reader, data structures, analyzer/eval,
   emitter, interop/concurrency, spikes).
4. `openspec/` — active change proposals (`openspec list`).

## Process — ADR → propose → apply

For any non-trivial change (new capability, contract change, milestone stage):
1. **ADR first** if it involves a new decision or reverses one — `docs/adr/NNNN-slug.md`
   (context / decision / consequences; supersede, don't edit history).
2. **`/opsx:propose`** — OpenSpec proposal + design + spec deltas under `openspec/changes/`.
3. **Apply** via tasks; **archive** when done.
Trivial fixes skip OpenSpec; nothing skips gates.

## Gates (before every commit)

```
go build ./... && go vet ./... && gofmt -l pkg cmd conformance templates && go test ./...
```
All green, no exceptions. `refs/` is fenced with a stub go.mod — leave it.

## Conformance discipline

- Every semantic behavior = a `conformance/tests/*.clj` file with frozen
  `;; expect:` output, verified against real JVM Clojure 1.12.5 (`clojure` CLI —
  the semantic oracle, needed at authoring time only) and cited in a comment.
- From M2 the same files also run AOT-compiled (dual harness). REPL-vs-binary
  divergence is THE unforgivable failure mode — release blocker.
- Perf budgets are CI-checked like tests (owner mandate: performance is a
  feature; see design/00 §1.4).

## How to write error messages

Binding doctrine (owner, rescoped 2026-07-22: *"no need exactly like rust,
just some more details, that's enough."*). The target is ONE richer error
line — named, located, expected-vs-found, with a cheap `help:` pointer — NOT
Rust's full snippet+caret block. The data model is `pkg/diag.Diagnostic`
(ADR 0015). Full map: `docs/error-messages-audit-2026-07.md`. The overhaul is
**ADR 0048** (reserved) + **spike s28** (`spikes/s28-rust-diagnostics/`,
prototype + VERDICT) — until it lands, follow these rules for any *new* error
and do not add bare `fmt.Errorf`/`panic` strings to the user-facing path.

- **Name the thing.** Arity errors name the fn like the JVM — `passed to:
  user/f`, never `passed to: fn`. Same for vars, namespaces, protocols. This
  is the #1 win.
- **Location when known.** If the error has a source position, append the
  locus: ` at file:line:col`. No source snippet, no `^^^^` caret (owner
  rescope — those are *optional, future*, not required).
- **State expected vs found** (`Expected`/`Found`) whenever the shape is
  expected-vs-actual (arity, type, arg count): `(expects 1: [x])`.
- **Carry a registered code + explain pointer where it's cheap.** Codes come
  from the banded registry (`pkg/diag/registry.go`: R1xxx reader · A2xxx
  analyzer · E3xxx emitter · I4xxx interop · G5xxx general), append-only, each
  with an explain page `docs/diagnostics/<CODE>.md`. Prefer attaching the code
  **at the raise site**; the renderer appends `help: run \`cljgo explain
  <CODE>\``.
- **Suggestions are `Fix`es, not prose.** did-you-mean is a
  `Fix{Title: "did you mean X?", Replacement: "X"}` rendered as a `help:`
  line, firing in every context, not just the REPL.
- **Read the same in REPL, `cljgo run`, and compiled binaries** (and as the
  nREPL `err` string). One renderer (`diag.Render`), every context calls it;
  the emitted `func main()` recovers and routes through it too. **No raw Go
  panic + goroutine stack trace ever reaches a user** — that is the
  unforgivable failure mode (same bar as conformance).
- **The `--json` `diag.Envelope` carries every field** (code, location,
  expected/found, fixes, related, explain URL) so agents consume errors
  without parsing prose.

Before → after (the arity error, the canonical case — the lighter one-line
target, not a Rust block):

```
error: wrong number of args (3) passed to: fn          ← today (bare, unnamed)

error: wrong number of args (3) passed to: user/f (expects 1: [x]) at demo.clj:2:1
help: run `cljgo explain A2004`
```

The rendered `.Error()` string stays byte-stable (conformance freezes it);
the extra detail is added at the render layer by `diag.Render`.

## Hard rules

- Never commit compiled binaries (`/cljgo`, spike artifacts).
- `pkg/lang` is vendored from Glojure — keep EPL headers on vendored files,
  log meaningful surgery in `pkg/lang/PROVENANCE.md` / `TODO.md`.
- Never add `Co-authored-by:` to commits.
- `refs/` is read-only history. CLOSED spikes (those with a VERDICT.md) are
  frozen; NEW spikes follow the ADR 0027 lifecycle (spike → close → ADR →
  spec → apply).
- Verify Clojure behavior against the real `clojure` CLI, not memory.

## Layout

`pkg/lang` runtime · `pkg/corelib` Go-native core builtins (ADR 0043) ·
`pkg/reader` · `pkg/ast` · `pkg/analyzer` · `pkg/eval` ·
`pkg/repl` · `pkg/bri` (bri runtime shims: http/db/auth/otel host fns) ·
`pkg/briaot` (AOT-compiled bri + opt-in per-namespace linking, ADR 0074) ·
`pkg/briloader` · `cmd/genbri` · `cmd/cljgo` ·
`core/` (core.clj, Clojure-in-Clojure) · `core/bri/` (bri namespaces —
http/db/auth/otel/html/config/audit) ·
`templates/` (real, runnable project templates `cljgo new` embeds —
lib (default) / cli / web; never string literals) · `conformance/` · `design/` · `docs/adr/` · `openspec/` ·
`spikes/` (frozen) · `refs/` (gitignored clones).

## Competitive claims discipline (owner, 2026-07-25)

Any public claim about Glojure / let-go / gloat (FAQ, benchmarks page, Slack)
must be verified against their SOURCE or the actual measured binaries — never
READMEs, never memory. Verified facts as of 2026-07-25 (re-verify before
reuse):

- **What ships in an AOT binary:** cljgo links zero interpreter (CI:
  `pkg/coreaot/imports_test.go` TestNoInterpreterInCompiledBinary). Glojure's
  shipping AOT mode (`-tags glj_aot_runtime`, what gloat uses) RETAINS the
  evaluator and reader — its README says "retains evaluation", and
  `strings <bin> | grep EvalAST` / `grep glojure/pkg/reader` proves it on the
  binary (stripped binaries keep the pclntab, so function names survive
  `-s -w`). let-go's lowered binaries retain the VM. Do NOT claim "only
  let-go includes its runtime" and do NOT claim Glojure is interpreter-free.
- **Size claims:** one corpus per table. Benchmark-suite binaries: cljgo
  6.7 MB / Glojure 7.5 MB / let-go 12.8 MB. hello-world 5.3 MB is a DIFFERENT
  program — never mix it into the suite row. Don't attribute the whole size
  delta to the interpreter; say "it's in theirs, not in ours" and stop.
- **Speed:** Glojure AOT wins 6 of 8 suite rows (fusion + int64
  specialization); cljgo wins tak/fib. Losses are roadmap gaps, not design
  costs — never spin them as deliberate trade-offs.

## The precedence principle (owner, 2026-07-12)

**Clojure is first-class.** Everything we add (comptime, Result/Option, ffi,
testing forms, diagnostics) exists to make it BETTER, never different: an
addition may not shadow, rename, or change the semantics of anything in
clojure.core or the reader. When a new feature's natural name collides with
Clojure (e.g. `some`), the NEW feature renames (=> `just`/`none`), never
Clojure. Ratified example: ADR 0014 constructors are `just`/`none`.
