# Release process

The complete checklist for shipping a cljgo version. **This file is the
authority**; `CLAUDE.md` and `AGENTS.md` point here rather than restating it,
so there is one copy to keep true.

Every step exists because skipping it has produced a real defect. Where that
is so, the incident is named — a checklist item nobody can trace to a failure
is the first one dropped under pressure.

---

## 0. Before you start

**The version is the git tag.** `pkg/version.Version` stays `"0.1.0-dev"` in
source forever; goreleaser injects the real value at build time. Never "bump"
that constant.

Decide the number. Patch for fixes and measurements; minor for features or
behaviour changes.

---

## 1. Gates

```bash
CGO_ENABLED=0 go build ./... && go vet ./... \
  && gofmt -l pkg cmd conformance templates core \
  && go test ./... -timeout 1800s -p 1 > /tmp/gate.txt 2>&1; echo "EXIT=$?"
```

- **Run it from the repository root.** A gate run from a subdirectory matches
  no packages, prints `no packages to vet`, and exits non-zero for the wrong
  reason — or worse, appears to pass. *(Happened: a gate run from `site/`
  after an npm build proved nothing, and the commit had already landed.)*
- **Never pipe `go test` into `head`/`grep` and read `$?`.** That is the
  pipeline's exit code, not the tests', and SIGPIPE can kill the test binary
  mid-run. *(Happened: one false green.)*
- `-p 1` and the long timeout are not optional — the emit/conformance packages
  build real binaries.

## 2. Conditional gates — check each, skip none silently

| If the release touched… | Run |
|---|---|
| **dependency resolution** (Maven, Clojars, POM, lock, repos) | `CLJGO_CLOJARS_IT=1 go test ./pkg/deps/ -run TestClojarsIT -v` |
| **paths, file handles, process spawning** | `gh workflow run CI --ref <branch>` — the Windows leg |
| **any conformance test's expectations** | verify against the real `clojure` CLI and cite it |
| **`core/**` or `core/bri/**`** | `go generate ./pkg/briaot` and commit the regenerated twin |
| **a diagnostic code** | explain page in `docs/diagnostics/`, row in the site table, `CLJGO_WRITE_REGISTRY_LOCK=1 go test ./pkg/diag/ -run TestRegistryLockMatches` |

**The network integration test is skipped by default, so nothing runs it for
you.** It asserts both directions against live repositories: a pure library
resolves and classifies clean, and a Java-dependent one is refused with a
coded diagnostic. A gate that only ever says yes is not a gate.
*(Happened: v0.8.8 changed the Maven repository order and v0.8.9 changed POM
parsing — both squarely dependency resolution — and the IT was run only
during a later audit.)*

**Windows is weekly**, not per-push, so a path or file-handle change reaches
`main` unverified unless you trigger it. *(Happened: the v0.8.0 cycle shipped
a leak only `windows-latest` caught.)*

## 3. CI green on the exact commit

Push, then wait. **Do not tag a commit whose CI is still `in_progress`.**

If a job fails, do not reason past it:

- Re-run it. A perf-budget test that passes on one run and fails on another
  **for the same commit** is flaky under runner contention, not a regression —
  but establish that from two runs of the same SHA, not from confidence.
- Check whether another run of the *same commit* already covered that
  platform. Combined green across two runs is weaker evidence than one clean
  run; prefer the clean run.

## 4. Bump the three hardcoded files

These are what a user's `docker build` actually downloads, so a missed bump
ships a template pinned to a stale binary:

- `templates/web/Dockerfile` — `ARG CLJGO_VERSION=`
- `site/src/content/docs/guides/deploy.md` — the same ARG, quoted
- `docs/guides/bri-deploy.md` — prose: "default `vX.Y.Z`"

Then re-run the gate and push.

## 5. Tag

**The annotation IS the changelog.** GitHub renders it as the release body and
`site/src/content/docs/reference/releases.md` is distilled from it.

```bash
git tag -a vX.Y.Z <sha> -F /path/to/annotation.txt
git push origin vX.Y.Z
```

Write it for a user, not for the commit log:

- **Lead with the consequence, not the symptom.** "Your local changes were
  silently ignored" beats "IsRelease() returned true".
- **Name the regressions**, including ones this project introduced.
- **State what the release does NOT contain.** If a fix is argued from source
  rather than observed, say so. *(v0.8.8's repository-order fix could not be
  demonstrated offline; the notes say that rather than implying a
  reproduction.)*
- **Carry known-unfixed items forward** so nobody calibrates on a silence.

## 6. Confirm the artifacts

Wait for the Release workflow, then confirm **7 assets** — six platform
archives plus `checksums.txt`.

## 7. Acceptance-test the published archive, not your build

```bash
go install github.com/muthuishere/cljgo/cmd/cljgo@vX.Y.Z
cljgo version                      # clean tag, no dev suffix
cljgo new -template cli app && cd app
env -u CLJGO_SRC cljgo build && ./app
```

**Verify the path this release could most plausibly have broken**, not a
generic smoke test. A change that narrows release detection should be checked
in *both* directions; a change to dependency resolution should resolve a real
dependency.

*(Happened: v0.8.5 was announced from a local build while its own release
notes told users the opposite of what it fixed.)*

## 8. Update the published docs site — and confirm it deployed

**The site is a release artifact, not a folder of markdown.** Editing a file
under `site/` changes nothing a user sees until GitHub Pages redeploys it, and
the docs a user actually reads are at
<https://muthuishere.github.io/cljgo/> — not in the repository.

**8a. `site/src/content/docs/reference/releases.md`.** Newest first, heading
linked to the GitHub tag, distilled from the annotation so the two cannot
drift. There is deliberately **no root `CHANGELOG.md`** — a third copy would
rot.

**8b. Every published page the release changed.** A fix is not shipped to
users while the site still documents the old behaviour. Walk the pages this
release touches, not just the ones you happened to edit:

| the release changed… | published pages to re-read |
|---|---|
| project resolution, source roots, project files | `guides/dual-host.md`, `guides/compile.md`, `guides/repl.md`, `guides/deps-publish.md` |
| a diagnostic code | `reference/diagnostics.md` (the table is CI-checked against the registry) |
| a capability, or completed a roadmap item | `reference/architecture.md`, `reference/compatibility.md`, `reference/roadmap.md` |
| version-pinned instructions | `guides/deploy.md` (already bumped in step 4) |
| a competitive number | `reference/benchmarks.md` — see step 9 |

An **ADR is not a docs update**. `docs/adr/` is this project's decision
history and is not published; a decision that changes user-visible behaviour
has to be re-said on the site in the user's terms, on the page they would look
at. The ADR is the *why*, the site is the *what you do*.

Sidebar entries are hand-declared in `site/astro.config.mjs` for `Guides` and
`Reference` — a **new** page under those directories is invisible until it is
added there. Only `coming-from/`, `by-example/` and `choosing/` autogenerate.

**8c. Build it before pushing.**

```bash
cd site && npm ci && npm run build
```

This catches broken internal links and bad frontmatter, which a markdown diff
does not. Then run the repo gate **from the repository root** — never from
`site/`.

**8d. Confirm the deploy.** `.github/workflows/pages.yml` fires only on a push
to `main` whose paths touch `site/**` (or the workflow itself), plus
`workflow_dispatch`. Two consequences worth holding on to:

- **A site edit that lands on a branch and is merged is fine; a site edit that
  never reaches `main` is never published.** Tagging does not deploy the site
   — the Release workflow and the Pages workflow are independent.
- A change to a *non*-`site/` file that alters what the site should say (a new
  diagnostic, a renamed flag) triggers no deploy at all. Re-run it by hand:

```bash
gh run list --workflow "Deploy Pages" --limit 3   # did it run, and go green?
gh workflow run "Deploy Pages" --ref main         # force a redeploy
```

Then load the changed page over HTTPS and read it. A green workflow means the
artifact uploaded, not that the page says what you meant.

## 9. Re-check the claims this release invalidated

**This is the step that gets skipped.** *(Happened: five consecutive releases
shipped while `benchmark/` and `CLAUDE.md` still cited v0.8.0–v0.8.2.)*

- **`benchmark/` and `site/.../reference/benchmarks.md`** — rebuild the cljgo
  column and re-run. Each table carries its OWN date and cljgo version; they
  are not all measured at once, and the caution banner must say exactly which
  are current. **Publishing an estimate instead of a measurement is never
  acceptable.**
- **`CLAUDE.md`'s competitive-claims numbers** (binary size, startup,
  win/loss count). Agents quote these into public copy, so a stale figure here
  becomes a false public claim.
- Any doc stating a capability this release changed — **including the
  published site**, which is where users read it. *(Happened: v0.8.7 taught
  cljgo to read `deps.edn` `:paths`, and `reference/roadmap.md` went on saying
  "no `deps.edn` in either direction (shipped)" — a published claim the
  release made false. Found by an audit, not by the release.)*
- `openspec archive` whatever the cycle applied.

A quick way to catch this class: grep the site for the thing the release
changed, not for the version number. A stale claim rarely mentions a version.

```bash
rg -n "deps\.edn|build\.clj\b|Clojars, then" site/src/content/docs/ docs/
```

### Competitive claims discipline

Any public claim about Glojure / let-go / gloat must be verified against their
**source** or **actual measured binaries** — never READMEs, never memory.

- **One corpus per table.** Never mix a hello-world binary into a suite row.
- **Never diff a timing across two sessions.** Absolute ms are comparable only
  *within* one table; quote the within-table ratio. *(Two runs on the same
  unchanged competitor binary read 3.0 ms and 3.9 ms — the machine moved, not
  the code.)*
- **Measure sequentially.** Parallel benchmark processes measure the
  scheduler.
- Re-timing existing competitor artifacts is fair; claiming anything about a
  **newer** Glojure or let-go release requires rebuilding them with gloat
  first. If `benchmark/.build/aotcmp/` is empty, there is nothing to re-time.
- Report losses as roadmap gaps. Never spin them as deliberate trade-offs.

## 10. Tell the consumers

Downstream projects (koine, the toolnexus Clojure port) are the ones who find
what our gate does not. Tell them what shipped, what to re-verify, and what is
still broken.

State provenance before content, and prefer the answer that embarrasses the
fix: a consumer who reports "this still fails" is worth more than one who
confirms.

---

## Why this file exists

Across the v0.8.5–v0.8.9 cycle, **every headline defect was found by a
consumer or a spike, and none by this project's own gate** — while the gate
stayed green throughout. Two were regressions introduced by earlier releases
in the same cycle.

A green gate is necessary and never sufficient. These steps are the parts that
are not automated, which is exactly why they need writing down.
