# s72 — the cost of hoisting project resolution to every subcommand: results

**Question.** `cljgo repl` and `cljgo nrepl` boot evaluators without calling
`resolveRunDeps`, so neither can require the project's own namespaces or its
declared dependencies while `cljgo run` and `cljgo build` can (#185). The
cause is structural: resolution lives at each call site, so every new entry
point forgets it independently — four of ~eight did. The proposed fix is the
shape let-go and Glojure both use: **ONE resolution in `run()` before the
subcommand switch**, with a deny-list (`new`, `version`, `help`, `explain`).

That is unambiguously the simpler design. The open question is what it
**costs**, because it makes every non-denied subcommand — `cache`, `config`,
`dist`, `publish`, `generate`, `routes`, `migrate` — pay project resolution
at startup, and cljgo's startup time is a published competitive number.

**Measured 2026-08-01**, darwin/arm64 (macOS 26.4), Apple M5 Pro, go1.26.3.
Harness: `cmd/cljgo/resolvecost_bench_test.go` — benchmarks `resolveRunDeps`
itself, the production function `resolveProjectForCommand` calls, in the
production package. End-to-end numbers time the real binary built from the
tree with the hoist applied.

```
go test ./cmd/cljgo/ -run xxxNONE -bench Benchmark -benchtime 10x -benchmem
```

## 1. `resolveRunDeps` by project shape

Local `:path` dependencies throughout, so nothing touches the network.

| shape | ns/op | ms/op | B/op | allocs/op |
|---|---|---|---|---|
| no build file (bare dir) | 3,735 | 0.0037 | 1,280 | 24 |
| `build.cljgo`, no deps, no lock | 156,249,715 | **156.2** | 221,531,027 | 3,556,462 |
| `build.cljgo` + 20 deps, no lock (#168) | 88,988,421 | 89.0 | 111,258,260 | 1,785,601 |
| `build.cljgo` + lock, 20 deps | 77,567,517 | **77.6** | 111,440,580 | 1,789,377 |

Read the second row twice. **The cheapest possible project — the default
`cljgo new` library template, whose whole build file is `(defn build [_b])` —
is the most expensive shape measured.** It costs *twice* what a
twenty-dependency locked project costs. That is not a load-related cost; see
§4.

## 2. Growth as N rises — flat, and it does not matter

Locked project, N local `:path` dependencies, warm steady state:

| N deps | ms/op | B/op | allocs/op |
|---|---|---|---|
| 1 | 76.73 | 110,980,487 | 1,779,080 |
| 5 | 77.35 | 111,039,213 | 1,781,292 |
| 20 | 77.57 | 111,440,580 | 1,789,377 |
| 50 | 78.34 | 112,267,921 | 1,805,554 |
| 200 | 82.00 | 116,696,951 | 1,886,469 |

No-lock (#168) path, same sizes: 76.5 / 77.2 / 89.0 / 79.5 ms.

**Growth is FLAT.** 200 dependencies cost 5.3 ms more than 1 — about **27 µs
and 29 KB per declared dependency**, against a fixed cost of ~76.7 ms and
111 MB. The marginal term is 0.03% of the constant. There is no scaling
problem here at all; there is a **constant-cost problem**, and N is not it.

## 3. Where the constant comes from

| piece | ms/op | B/op | allocs/op |
|---|---|---|---|
| `build.FindBuildFile` (miss) | 0.013 | 1,392 | 9 |
| `eval.New()` — fresh interpreter boot | **38.23** | 54,186,180 | 874,829 |
| `build.LoadPlan`, 0 deps | 39.05 | 55,428,955 | 889,077 |
| `build.LoadPlan`, 200 deps | 39.96 | 57,331,498 | 925,558 |

`LoadPlan` evaluates `build.cljgo` through a **fresh tree-walking
interpreter**, which loads `core.clj`. That boot is **38.2 ms and 54 MB**, and
it is 98% of `LoadPlan` at every size. Evaluating the user's actual build form
— even one declaring 200 dependencies — is under 2 ms.

Every number in §1 and §2 is therefore just a count of interpreter boots:

| shape | LoadPlan calls | predicted | measured |
|---|---|---|---|
| locked / no-lock, N deps | 2 | 78.1 ms | 77.6 ms |
| **no deps, no lock** | **4** | 156.2 ms | 156.2 ms |

Two boots because `addProjectSourceRoots` calls `LoadPlan` and then the
lock branch calls it again (`build.LoadPlan` for `ErrNoLock`, or
`ResolveProjectDeps` internally). **Four** because `resolveRunDeps` loops over
`[filepath.Dir(file), "."]`, and for every command without a script anchor
`file` is `""`, so `filepath.Dir("") == "."` — **the same directory is scanned
twice**. A project with dependencies `return`s on the first pass and never
notices; a dep-free project `continue`s and pays the whole thing again.

## 4. Cost as a fraction of startup — the real binary

20 invocations after 3 warm-ups, wall clock, same binary (hoist applied):

| command | bare dir | dep-free project | 20-dep locked project |
|---|---|---|---|
| `version` *(denied — the floor)* | 10.22 ms | 9.81 ms | 10.20 ms |
| `cache help` | 9.98 ms | **166.75 ms** | **89.73 ms** |
| `config` | 9.81 ms | 163.16 ms | 88.81 ms |
| `generate` | 9.65 ms | 163.52 ms | 90.01 ms |
| `publish` | — | — | 89.24 ms |
| `new` *(denied)* | — | 9.70 ms | — |
| `explain G5023` *(denied)* | 9.72 ms | 14.84 ms | — |

The deny-list works: `version` and `new` are flat at ~10 ms inside a project.
The `version` row IS the pre-hoist baseline for every command in the table —
before the hoist, none of these commands resolved anything.

So the hoist's cost, stated as a fraction of startup:

- **bare directory: 0.04%** of a 10 ms startup (3.7 µs). Free.
- **20-dep locked project: +78.6 ms — 8.7× startup, ~870% overhead.**
- **dep-free project (the default template): +153 ms — 16.6× startup,
  ~1560% overhead.**

For calibration against the published competitive figure: cljgo's benchmark
startup is ~5.1 ms for an AOT hello-world, a *different program* on a
*different measurement occasion* — per CLAUDE.md, absolute ms are comparable
only within one table, so treat the 10 ms `version` row as this table's floor
and the ratios above as the claim. Either way the resolution is one to one
and a half **orders of magnitude** larger than the whole startup budget.

## 5. The other regression: it is not only slow, it is wrong output

`cljgo cache help` and `cljgo publish`, inside a project with dependencies but
no lock, now print a dependency error before doing their unrelated job:

```
$ cljgo cache help
error: build.cljgo declares 20 dependencies but has no build.lock.edn
help: run `cljgo build` once to resolve and pin them, then `cljgo run` works
help: run `cljgo explain G5023`
usage: cljgo cache clean   remove the global dependency cache …
```

Exit codes are unchanged (`cache help` still 0, `config` still 1 in both a
bare dir and a project), so nothing scripted breaks on status — but stderr is
now polluted for every non-evaluating command, and `cljgo cache clean` is
precisely what a user reaches for when resolution is broken.

Worse, `cljgo build` on a fresh clone tells the user to run the command they
are already running:

```
$ cljgo build
error: build.cljgo declares 20 dependencies but has no build.lock.edn
help: run `cljgo build` once to resolve and pin them, then `cljgo run` works
```

The hoist resolves *before* dispatch, so `build` — the command whose job is
to create the lock — is diagnosed for not having one, then goes on to make it.
`build` also pays the 77 ms twice, since `runBuild` resolves again itself.

## 6. What this measurement EXCLUDES

- **The network, entirely.** Every dependency is a local `:path` dep. A real
  Maven/Clojars/git coordinate on a cold cache is one to two orders of
  magnitude more per coordinate (see s70). These numbers are a **floor** for
  a locked, warm, local project — the *best* case for the hoist.
- **Process spawn and dynamic-loader time** in §1–§3 (in-process benchmarks);
  §4 includes them, which is why §4 ≈ §1 + ~10 ms.
- **Disk cache state.** All fixtures are freshly written and OS-cached hot;
  a cold page cache is worse.
- **`cljgo run` / `cljgo build` / `cljgo test`** as a *change*: those already
  called `resolveRunDeps` before the hoist. Their cost is not new (except
  `build`'s newly doubled resolve, §5). The regression measured here is
  confined to commands that previously did no resolution.
- **The REPL's own boot.** `cljgo repl` boots an evaluator regardless, so a
  38 ms interpreter boot inside a 76 ms resolution is a smaller relative
  addition there than these tables suggest. The hoist is unambiguously right
  for `repl`/`nrepl` — that is the bug being fixed.

## 7. VERDICT

**Hoisting resolution to every non-denied subcommand is NOT affordable as
written.** It adds 78–153 ms and 111–221 MB of allocation to commands that
never evaluate a line of project code — 8.7× to 16.6× the entire startup
budget — and it makes three of them print an irrelevant dependency error. A
change justified as "fewer moving parts" must not cost 16× startup on the
default project template.

The *structure* of the fix is right and should ship. Two simple corrections
make it affordable, and neither adds a mechanism:

1. **Widen the deny-list to every command that does not evaluate project
   code.** Add `cache`, `config`, `dist`, `publish`, `generate`/`g`, `routes`,
   `migrate` to the existing `new`/`version`/`help`/`explain`. Cost of the
   fix: seven strings. It restores those commands to the 10 ms floor exactly
   as the deny-list already does for `version` and `new` (measured, §4), and
   it removes the spurious G5023 output (§5) at the same time.

   This keeps the property the deny-list exists for: a *new* subcommand
   resolves by default, so forgetting the list costs milliseconds, never a
   silently broken command. It does not weaken the #185 fix at all — `repl`,
   `nrepl`, `run`, `build`, `test`, `dev`, `check`, `suite` all still resolve
   through the one call.

2. **Stop `build` resolving twice.** `runBuild` resolves on its own terms;
   deny-listing `build` (or dropping the duplicate) removes 77 ms and the
   "run `cljgo build` once" self-contradiction. Prefer the deny-list entry —
   it is one string against an edit to the build path.

**Do not** reach for a cache, a lazy/deferred resolution layer, a
`needsProject` interface, or a resolution strategy. The measurement does not
call for one: the cost is a fixed 38 ms interpreter boot, and the fix is to
*not do the work* for commands that never needed it. Adding machinery to make
unnecessary work cheap is the failure mode CLAUDE.md names by hand.

### What surprised us, and it contradicts the proposed design

Three things, in ascending order of importance:

1. **The dep-free project is the worst case.** 156 ms against 78 ms for a
   200-dependency project. Every intuition about this change says cost tracks
   dependency count; it does not track it at all (§2 — flat, 27 µs/dep). The
   cost is boot count, and the *emptiest* project boots the most, because
   `resolveRunDeps` scans `.` twice and only a project with dependencies
   short-circuits out of the second pass. The default `cljgo new` template is
   the shape that pays most.

2. **`LoadPlan` is called twice on every path** — once by
   `addProjectSourceRoots`, once by the lock branch — for a plan that has
   just been computed and thrown away. Halving this is bracketed by the
   numbers already measured: one boot is 39.1 ms, so the reachable floor is
   **39 ms for any shape** (locked 78 → 39, dep-free 156 → 39). That is a 2×
   and a 4× available from *deleting* redundant work, with strictly fewer
   moving parts. It is a simplification, not an optimisation, and it stands
   on its own regardless of what the hoist does. Not implemented here — this
   spike measures, it does not change production code.

3. **Project resolution boots a whole tree-walking interpreter to read a
   build file** — 38.2 ms, 54 MB, 875k allocations, of which the user's build
   form accounts for under 2 ms. This is the real number behind every table
   above, and it is worth an ADR of its own. Recorded, deliberately not acted
   on here: it is out of scope for a call-site question, and any fix touches
   the boot path that everything else depends on.

**Ship the hoist with the deny-list widened. Do not ship it as written.**
