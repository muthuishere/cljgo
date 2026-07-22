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

Binding doctrine (owner: *"very detailed error messages like rust and very
llm friendly"*). The bar is Rust's diagnostic anatomy; the data model is
`pkg/diag.Diagnostic` (ADR 0015). Full map + plan:
`docs/error-messages-audit-2026-07.md`. The overhaul itself is **ADR 0048**
(reserved) + **spike s28** (reserved) — until it lands, follow these rules
for any *new* error and do not add bare `fmt.Errorf`/`panic` strings to the
user-facing path.

- **Every user-facing error carries a registered code** from the banded
  registry (`pkg/diag/registry.go`: R1xxx reader · A2xxx analyzer · E3xxx
  emitter · I4xxx interop · G5xxx general). Codes are append-only; each gets
  an explain page `docs/diagnostics/<CODE>.md`. Attach the code **at the
  raise site**, never by prose-matching after the fact.
- **If the error has a source position, it carries `Location`** (with the
  end-span) and renders as the Rust-style block: `error[CODE]: message`, the
  `--> file:line:col` locus, the **source snippet with line numbers**, and a
  `^^^^` **caret on the exact span** with an inline label. No snippet-less
  positioned error.
- **State expected vs found** (`Expected`/`Found`) whenever the shape is
  expected-vs-actual (arity, type, arg count).
- **Where a fix is knowable, attach a concrete `Fix`** — the *replacement
  text* + `ByteRange`, not prose. did-you-mean is a `Fix`
  (`Replacement: "X"`), not a `did you mean …?` string, and it fires in
  every context, not just the REPL.
- **Name the thing.** Arity errors name the fn like the JVM — `passed to:
  user/f`, never `passed to: fn`. Same for vars, namespaces, protocols.
- **Read the same in REPL, `cljgo run`, and compiled binaries** (and as the
  nREPL `err` string). One renderer (`diag.Render`), every context calls it;
  the emitted `func main()` recovers and routes through it too. REPL-vs-
  binary error divergence is the unforgivable failure mode (same bar as
  conformance). No raw Go panic + stack trace ever reaches a user.
- **The `--json` `diag.Envelope` carries every field** (code, location,
  expected/found, fixes, related, explain URL) so agents consume errors
  without parsing prose.

Before → after (the arity error, the canonical case):

```
error: wrong number of args (3) passed to: fn          ← today (bare, unnamed)

error[A2004]: wrong number of args (3) passed to: user/f
  --> demo.clj:4:1
   |
 4 | (f 1 2 3)
   | ^^^^^^^^^ user/f is defined with 1 arg [x], got 3
   |
help: user/f takes 1 argument — call it as (f 1)
for more information, run `cljgo explain A2004`
```

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
`pkg/repl` · `cmd/cljgo` · `core/` (core.clj, Clojure-in-Clojure) ·
`templates/` (real, runnable project templates `cljgo new` embeds —
lib (default) / cli / web; never string literals) · `conformance/` · `design/` · `docs/adr/` · `openspec/` ·
`spikes/` (frozen) · `refs/` (gitignored clones).

## The precedence principle (owner, 2026-07-12)

**Clojure is first-class.** Everything we add (comptime, Result/Option, ffi,
testing forms, diagnostics) exists to make it BETTER, never different: an
addition may not shadow, rename, or change the semantics of anything in
clojure.core or the reader. When a new feature's natural name collides with
Clojure (e.g. `some`), the NEW feature renames (=> `just`/`none`), never
Clojure. Ratified example: ADR 0014 constructors are `just`/`none`.
