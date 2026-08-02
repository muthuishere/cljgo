# AGENTS.md — cljgo

Instructions for coding agents working in this repository. Kept short on
purpose: it points at the authoritative documents rather than restating them,
so there is one copy of each rule to keep true.

**`CLAUDE.md` is the full agent brief.** Read it first. Everything below is
either a pointer or a rule that has been broken often enough to deserve
repeating here.

## Authority chain — read in this order

1. **`CLAUDE.md`** — conventions, doctrine, hard rules.
2. **`docs/adr/`** — decisions. Binding until superseded by a newer ADR.
3. **`design/00-architecture.md`** — cross-component contracts and roadmap.
4. **`design/01`–`07`** — component internals.
5. **`openspec/`** — active change proposals (`openspec list`).

## Releasing

**Follow [`docs/release-process.md`](docs/release-process.md) step by step. Do
not work from memory.**

It is the authority for shipping a version, and it exists because across one
release cycle every headline defect was found by a consumer or a spike and
none by this project's gate — while the gate stayed green throughout.

The steps most often skipped, all of which have shipped a defect:

- the **Clojars network integration test** before any release touching
  dependency resolution (skipped by default, so nothing runs it for you);
- the **Windows CI leg** for path, file-handle or process-spawning changes
  (Windows runs weekly, not per-push);
- the **conformance oracle** when a test's expectations change;
- the **published docs site** — an ADR is not a docs update, and a page under
  `site/` changes nothing a user sees until the Pages workflow redeploys from
  `main`;
- the **post-release claims re-check** — `benchmark/` and `CLAUDE.md`'s
  competitive numbers go stale on every release and get quoted into public
  copy.

## Process for a non-trivial change

1. **ADR first** if it is a new decision or reverses one —
   `docs/adr/NNNN-slug.md`. Supersede; never edit history.
2. **`/opsx:propose`** — an OpenSpec proposal with spec deltas under
   `openspec/changes/`.
3. **Apply** via tasks, and **`openspec archive`** when done.

Trivial fixes skip OpenSpec. **Nothing skips the gates.**

## The gate, before every commit

```bash
CGO_ENABLED=0 go build ./... && go vet ./... \
  && gofmt -l pkg cmd conformance templates core \
  && go test ./... -timeout 1800s -p 1 > /tmp/gate.txt 2>&1; echo "EXIT=$?"
```

Run it **from the repository root** — a gate run from a subdirectory matches no
packages and proves nothing. **Never pipe `go test` into `head`/`grep` and read
`$?`**: that is the pipeline's exit code, and it has already produced one false
green.

## Rules that are not negotiable

- **Verify Clojure behaviour against the real `clojure` CLI, not memory.**
- **REPL-vs-binary divergence is a release blocker** (ADR 0007), not a known
  issue.
- **Never commit compiled binaries.** Never add `Co-authored-by:` to a commit.
- `pkg/lang` is vendored from Glojure — keep EPL headers, log surgery in
  `pkg/lang/PROVENANCE.md`.
- `refs/` is read-only history; spikes with a `VERDICT.md` are frozen.
- **A spike ships numbers at 3–4 sizes, with allocation and its exclusions
  named.** No numbers, no scaling statement, no answer.
- **Simplicity before performance.** A measured 8% does not earn a second code
  path. If the only argument for a mechanism is a benchmark, drop it.
- **Competitive claims about Glojure / let-go / gloat** must come from their
  source or from binaries you measured — never READMEs, never memory. One
  corpus per table; never diff a timing across sessions.

## Working with consumers

Downstream projects (koine, the toolnexus Clojure port) find what our gate
does not. When reporting to them: state provenance before content, and prefer
the answer that embarrasses the fix. A consumer who reports "this still fails"
is worth more than one who confirms.
