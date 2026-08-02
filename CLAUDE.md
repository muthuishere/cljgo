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

## What a spike is for (owner, 2026-07-31)

**A spike is not for deciding whether something WORKS. It is for deciding
whether it SCALES and PERFORMS.**

"Can this be built" is almost never the open question — it usually can, and a
prototype that only proves feasibility has told the ADR nothing it did not
already assume. What an ADR cannot decide without measurement is the shape
under load: cost per unit as N grows, allocation per operation, where the
work belongs (compile time vs runtime), and which of two designs is the one
you can still afford at 100×.

So every spike ships **numbers, at more than one size**:

- **Vary the input.** One data point is an anecdote. Measure at 3–4 scales
  and state the growth — linear, superlinear, flat. "No superlinear blowup"
  is itself a result, and often the one that makes a simple design
  affordable.
- **Report allocation, not just time.** For anything on a server path,
  B/op and allocs/op decide scalability long before ns/op does. 1440 B/op
  versus 24 B/op is the finding; the 6× on time is the footnote.
- **Measure through the production code path**, not a parallel toy. s70 used
  the same httptest Maven double the correctness tests use, so the thing
  benchmarked is the thing that ships.
- **Name what the measurement EXCLUDES.** s70's numbers exclude network
  entirely, which makes them a floor, not a cost. A benchmark quoted without
  its exclusions becomes a false claim the moment someone reuses it.
- **Bracket what you cannot measure directly.** Minimal re-resolve was not
  implemented, so it was bounded by the warm and full legs — a defensible
  number beats an unmeasured plan.

Feasibility findings still get recorded when the prototype turns one up (s71
found a silent mis-format Go's layout language makes structural), but that is
a by-product. If a spike's verdict contains no numbers and no scaling
statement, **it has not answered the question it was opened for**.

And read the result the right way round: the numbers exist to say **which
simple design to pick**, never to license a complicated one — see *Simplicity
first, then performance* below. A verdict that concludes "therefore add a
layer" has usually been misread.

Reference shape: `spikes/s70-lock-policy/RESULTS.md` (cost per coordinate
across four graph shapes) and `spikes/s71-date-patterns/RESULTS.md` (three
strategies, time + allocation, deciding compile-time vs runtime placement).

## Gates (before every commit)

```
CGO_ENABLED=0 go build ./... && go vet ./... \
  && gofmt -l pkg cmd conformance templates core \
  && go test ./... -timeout 1800s -p 1
```
All green, no exceptions. `refs/` is fenced with a stub go.mod — leave it.

`-p 1` and the long timeout are not optional: the emit/conformance packages
build real binaries and both deadlock the default 10 m and thrash when run
in parallel.

**Never pipe `go test` into `head`/`grep` and read `$?`.** That is the
pipeline's exit code, not the tests' — and the SIGPIPE can kill the test
binary mid-run. It has already produced one false green here. Redirect and
capture the real code:

```
go test ./... -timeout 1800s -p 1 > /tmp/gate.txt 2>&1; echo "EXIT=$?"
```

The gate is macOS-local; **CI is the real gate**. On every push and PR it runs
**ubuntu + macos**. **Windows is WEEKLY** (Mondays 06:00 UTC) and on
`workflow_dispatch` — owner call, 2026-07-31: the Windows leg takes several
times as long as the other two and its perf-budget test is measurably flaky.

That trade is real and you should know its shape: Windows finds what POSIX
hides (open-file-handle deletes, path separators, file modes), and the v0.8.0
cycle shipped a leak only `windows-latest` caught. Under a weekly schedule that
class of bug can sit on main for a week. **Run CI manually on Windows before
tagging** when a change touched paths, file handles or process spawning:

```
gh workflow run CI --ref main
```

A green local gate is necessary, never sufficient.

## Releasing — what changes when a version ships

> **The full checklist is [`docs/release-process.md`](docs/release-process.md).
> Follow it, do not work from memory or from the summary below.** It is the
> authority; this section is orientation. It carries the conditional gates
> that are easiest to skip (the Clojars network IT, the Windows leg, the
> conformance oracle) and the post-release claims re-check — every one of
> which has been missed at least once, and the claims re-check five releases
> running.

**The version is the git tag.** `pkg/version.Version` stays `"0.1.0-dev"` in
source forever; goreleaser injects the real value at build time and fires on
any `v*` tag. Never "bump" that constant.

Three files hardcode the *previous* release and must be bumped by hand — they
are what a user's `docker build` actually downloads, so a missed bump ships a
template pinned to a stale binary:

- `templates/web/Dockerfile` (`ARG CLJGO_VERSION=`)
- `site/src/content/docs/guides/deploy.md` (the same ARG, quoted)
- `docs/guides/bri-deploy.md` (prose: "default `vX.Y.Z`")

Order of operations, every time:

1. **Gate green locally, then CI green** for the exact commit you are about to
   tag — do not tag a commit whose CI is still `in_progress`. Push/PR CI covers
   ubuntu + macos only; if the release touched paths, file handles or process
   spawning, trigger the Windows leg by hand (`gh workflow run CI --ref main`)
   and wait for it too.
2. Bump the three files above → commit → push → wait for CI again.
3. `git tag -a vX.Y.Z <sha> -m "<the release notes>"`. **The annotation IS
   the changelog** — GitHub renders it as the release body and
   `site/src/content/docs/reference/releases.md` is distilled from it. Write
   it for a user: what shipped, what broke, what was measured. Name the
   regressions.
4. Push the tag; wait for the Release workflow; confirm **7 assets** (six
   platform archives + `checksums.txt`).
5. **Update the published docs site, and confirm it deployed.** Add the
   release to `site/src/content/docs/reference/releases.md` — newest first,
   heading linked to the GitHub tag, distilled from the annotation so the two
   cannot drift (there is deliberately **no root `CHANGELOG.md`**: a third
   copy would just rot). Then re-read every *other* page the release changed:
   the site at <https://muthuishere.github.io/cljgo/> is what users read, and
   **an ADR is not a docs update** — `docs/adr/` is unpublished decision
   history, so a decision that changes behaviour has to be re-said on the page
   a user would look at. Build it (`cd site && npm run build`) before pushing;
   a new page under `guides/` or `reference/` also needs a hand-added sidebar
   entry in `site/astro.config.mjs`. The Pages workflow fires only on a push
   to `main` touching `site/**` — tagging does not deploy the site, and a doc
   change driven by a non-`site/` edit deploys nothing at all
   (`gh workflow run "Deploy Pages" --ref main`).
6. Re-check the docs claims the release invalidates — see below.

### Claims that go stale on a release, and must be re-checked

- **`benchmark/` + `site/.../reference/benchmarks.md`.** Rebuild the cljgo
  column and re-run; obey *Competitive claims discipline* below. Each table
  carries its OWN date and cljgo version — they are not all measured at once,
  and the caution banner must say exactly which are current. Publishing an
  estimate instead of a measurement is never acceptable.
- **`CLAUDE.md`'s own competitive-claims numbers** (binary size, startup,
  win/loss count). They are quoted by agents into public copy, so a stale
  figure here becomes a false public claim.
- Any doc stating a capability the release changed — **on the published site,
  not only in `docs/`**. v0.8.7 taught cljgo to read `deps.edn` `:paths` while
  `reference/roadmap.md` went on telling users cljgo reads "no `deps.edn` in
  either direction (shipped)". Grep the site for what the release *changed*,
  not for a version number: a stale claim rarely names one.
- `openspec archive` for whatever the cycle applied.

### Before a release that touches dependency resolution

Run the network integration test. It is skipped by default so a green build
never depends on Clojars being up, which also means **nothing runs it for
you**:

```
CLJGO_CLOJARS_IT=1 go test ./pkg/deps/ -run TestClojarsIT -v
```

It asserts both directions against the live repositories: a pure library
resolves and classifies clean, and a Java-dependent one is refused with a
coded, located diagnostic. A gate that only ever says yes is not a gate.

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
`pkg/deps` (dependency resolution: git · local path · Maven/Clojars
coordinates, ADR 0095) · `pkg/diag` (diagnostics + the append-only code
registry) · `pkg/coreaot` (AOT core twin; its `imports_test.go` is the
CI proof that a compiled binary links zero interpreter) · `pkg/version` ·
`templates/` (real, runnable project templates `cljgo new` embeds —
lib (default) / cli / web; never string literals) · `conformance/` · `design/` · `docs/adr/` · `openspec/` ·
`site/` (the Astro/Starlight docs book published to GitHub Pages — the
release notes live at `site/src/content/docs/reference/releases.md`) ·
`benchmark/` (the measured suite + `report.py`; `.build/` is gitignored and
holds the competitor binaries) ·
`spikes/` (frozen) · `refs/` (gitignored clones).

## Competitive claims discipline (owner, 2026-07-25)

Any public claim about Glojure / let-go / gloat (FAQ, benchmarks page, Slack)
must be verified against their SOURCE or the actual measured binaries — never
READMEs, never memory. Verified facts as of **2026-08-02**, cljgo
**post-v0.8.9** (commit `f46b9a8`), all three columns rebuilt and timed in one
hyperfine session (re-verify before reuse):

- **What ships in an AOT binary — say "evaluator", not "interpreter".** A
  cljgo AOT binary links zero `pkg/eval` / `pkg/analyzer` / `pkg/ast` /
  `pkg/repl` (CI: `pkg/coreaot/imports_test.go`
  TestNoInterpreterInCompiledBinary; `strings aot_fib | grep -c cljgo/pkg/eval`
  → 0). It DOES link **`pkg/reader`** — 121 symbols in a measured binary,
  because `read`/`read-string` are runtime core fns — plus `pkg/emit/rt` (the
  allowed bootstrap, 26). So "cljgo links zero interpreter" is too strong:
  the correct claim is **no evaluator, no analyzer; the reader is there**.
  Glojure's shipping AOT mode (`-tags glj_aot_runtime -ldflags "-s -w"`, what
  gloat uses) retains BOTH — measured on `fib-glj`, `grep -c EvalAST` → 57,
  `grep -c glojure/pkg/reader` → 61 (stripped binaries keep the pclntab).
  let-go's lowered binaries retain the VM (`let-go/pkg/vm` → 1939). Do NOT
  claim "only let-go includes its runtime".
- **Size claims:** one corpus per table. Benchmark-suite binaries, all three
  rebuilt 2026-08-02: cljgo **7.1 MB** (7,083,298 B; `tak` 7,099,810) /
  Glojure **19.0 MB** (19,021,826 B) / let-go **12.8 MB** (12,838,082 B).
  The cljgo tool itself is 28.6 MB stripped. **The old 7.5 MB Glojure figure
  does not reproduce** — same gloat v0.1.62, same pinned glj v0.7.0, same
  strip flags, and it now measures 19.0 MB; the 2026-07-24 artifacts are gone
  so it cannot be fully resolved. **But let-go is the control and it
  reproduces** — 12,838,082 B against the previously recorded 12.8 MB, same
  run, same tooling. That points at the 7.5 MB figure as the outlier rather
  than at this run, most likely a Glojure build without
  `-tags glj_aot_runtime` (hypothesis, not a measurement). Quote 19.0 MB, say
  it is a fresh build rather than a re-time, and **never say Glojure's binary
  grew** — nothing here measured a change over time. **The hello-vs-suite caveat is now obsolete**:
  `(println "hi")` compiles to 7,083,298 B — byte-identical to the suite
  binaries, because the linked AOT core dominates. The old 5.3 / 6.7 MB
  hello figures are dead; do not quote them.
- **Speed:** the "Glojure wins 6 of 8" line is **retired — it no longer
  holds**. With σ at 0.3–1.0 ms in-session, four rows are ties inside the
  noise (`startup`, `loop-recur`, `persistent-map`, `map-filter`). Outside
  the noise it is **2 clear wins each**: cljgo takes `tak` (36.6 vs 53.1 ms,
  1.45×) and `fib` (25.6 vs 39.5, 1.54×); Glojure takes `transducers`
  (11.7 vs 17.2, 1.47×) and **`reduce` (7.4 vs 27.2, 3.67×)**. `reduce` is
  the one real hole: hand-checked at N = 1M/4M/16M with printing variants,
  both give the correct sum and both scale linearly — Glojure ~1.0 ns per
  element, cljgo ~21.8 ns. That is a ~21× per-element loss and a roadmap gap,
  never a deliberate trade-off.
- **Startup is no longer a measured loss to Glojure.** 2026-08-02:
  cljgo 6.1 ms · Glojure 6.5 · let-go 6.0, σ 0.5–1.0 — a three-way tie, and
  a confirmation re-run read 5.0 / 5.4 / 4.6 with the same ordering. The
  v0.8.2-era "5.1 vs 3.9, cljgo loses" claim is **not reproduced** against
  freshly built binaries. Do not resurrect it, and do not upgrade the tie
  into a win either.
- **Never diff a timing across two sessions.** The v0.8.1 run read cljgo
  6.6 ms / Glojure 3.0 ms and the v0.8.2 run read 5.1 / 3.9 — on the SAME
  unchanged Glojure binary. The machine moved, not the code. Absolute ms are
  comparable only within one table; quote the within-table ratio instead.
- **Competitor binaries are rebuildable — rebuild them, don't estimate.**
  `benchmark/.build/aotcmp/` is gitignored and starts empty. gloat lives at
  `~/muthu/gitworkspace/clojure-workspace/references/gloat/bin/gloat`
  (v0.1.62, pinning glj v0.7.0 / lg v1.12.2) and the entry-point sources it
  needs are `references/let-go/benchmark/gloat/*.clj`:
  `gloat -E glj -o <n>-glj <src>` and `gloat -E lglvm -o <n>-lg <src>`.
  Claiming anything about a NEWER Glojure or let-go release still requires a
  newer gloat.

## Simplicity first, then performance (owner, 2026-07-31)

**"Scalable" does not mean `EnterpriseBeanAbstractFactory`.** Scaling by
adding layers, indirection, strategy objects and configurable engines is the
Java-enterprise failure mode, and it is a *worse* outcome than the slow thing
it replaced. The order is fixed and it is not negotiable:

1. **Simplicity — which is the hard part.** The design a reader can hold in
   their head, with the fewest moving parts that can be correct.
2. **Then performance — and it must not cost simplicity.**

Read that second clause strictly. Performance work is welcome when it makes
the same simple thing faster (a better algorithm, less allocation, work moved
to compile time through a seam that already exists). It is **refused** when
its price is a second code path, a pluggable strategy, a cache with an
invalidation story, or an abstraction whose only justification is a benchmark.

The operational test, applied to every optimisation:

- **Would you keep this if it were the same speed?** If the only argument is
  the number, and it adds a mechanism, drop it.
- **How much does it actually buy?** A measured 8% does not earn a second
  code path. A measured 6× with 60× the allocation does.
- **Is it independently justified?** The best optimisations are ones
  correctness wanted anyway — those are free. Say so explicitly in the ADR,
  or the reader cannot tell them apart from perf-driven complexity.
- **Count the moving parts, not the lines.** A 200-line function with one
  entry point is simpler than three 40-line ones behind an interface.

This binds the spike doctrine above: a spike measures scale so it can tell you
**which simple design to pick** — not so it can justify a complicated one.
A spike result that concludes "therefore add a layer" has usually been
misread.

## The precedence principle (owner, 2026-07-12)

**Clojure is first-class.** Everything we add (comptime, Result/Option, ffi,
testing forms, diagnostics) exists to make it BETTER, never different: an
addition may not shadow, rename, or change the semantics of anything in
clojure.core or the reader. When a new feature's natural name collides with
Clojure (e.g. `some`), the NEW feature renames (=> `just`/`none`), never
Clojure. Ratified example: ADR 0014 constructors are `just`/`none`.
