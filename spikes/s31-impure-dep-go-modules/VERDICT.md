# Spike S31 verdict — impurity does not merge cleanly, and it already breaks dual-mode parity

Closed 2026-07-22. Feeds **ADR 0048** decisions **4**, **5** and **6**.

**Exit criterion: MET on all five clauses.** All output under `results/`,
produced against real `go1.26.3` and a real `cljgo 0.1.0-dev` binary.

The headline is not the merge algorithm. It is this:

> **A third-party `go-require` already produces a silent REPL-vs-binary
> divergence today — the failure mode CLAUDE.md calls unforgivable and a
> release blocker — and it does so with no `dep` verb and no transitive
> dependency in sight.** Impure dependencies do not introduce this problem;
> they multiply an existing one.

## 1. What was tried

| # | experiment | file |
|---|---|---|
| A | one `go.mod`, same module path twice at two versions | `results/A-duplicate-require.txt` |
| B | a REAL Go module graph disagreeing (contrast case) | `results/B-module-graph-mvs.txt` |
| C | merged require set driven through `emit.SynthGoMod` | `results/C-flatten.txt` |
| D | `SynthGoMod` re-called with a CHANGED require set | `results/D-never-overwrite.txt` |
| E | conflict merged via `SynthGoMod` + `go mod tidy` | `results/E-conflict.txt` |
| F | transitive discovery from S33's lock schema, 3 merge policies | `results/F-lock-discovery.txt` |
| G | end-to-end `cljgo build` with a real `go-require` | `results/G-e2e-real-build.txt` |
| H | interpreted vs AOT on the same impure source | `results/H-divergence.txt` |
| I | falsifiable re-resolution test after editing `go-require` | `results/I-reresolution.txt` |
| J | **the conflict, end to end through the real pipeline** | `results/J-conflict-e2e.txt` |

`driver.go` (cases C/D/E/F) imports `pkg/emit` read-only. Nothing under
`pkg/`, `cmd/`, `core/`, `templates/` was modified. Gates green:
`go build ./... && go vet ./... && gofmt -l … && go test ./...`.

**Simulated vs real.** There is no `dep` verb (`core/build.cljg` has no
`dep` fn), so no experiment fetched a cljgo library. What is **real**:
every `go-require` → `go.mod` → `go build` path, and every Go-toolchain
behavior. What is **simulated**: the *origin* of a require — two
`(go-require …)` calls in one `build.cljgo` stand in for two deps
contributing one each. That simulation is exact, because
`core/build.cljg:38` accumulates **module-wide and discards the artifact**,
so a `dep` verb has no other join point available to it.

## 2. MEASURED

### 2.1 The divergence (the finding that outranks the merge question)

Same source file, `cljgo run` vs the AOT binary (`results/H-divergence.txt`):

```
$ cljgo run src/main.cljg     (interpreter)
consumer reached a dep's third-party Go module; uuid: nil
$ ./consumerapp               (AOT binary, same source)
consumer reached a dep's third-party Go module; uuid: 3d91365f-c353-436e-be9f-6c01c60f719a
```

Control, identical shape against a **stdlib** package — no divergence:

```
$ cljgo run src/main.cljg   ->  control: stdlib require-go works: OK
$ ./ctlapp                  ->  control: stdlib require-go works: OK
```

So the divergence is specific to **third-party** (i.e. impure) modules. The
interpreter cannot reach a Go package that is not linked into the `cljgo`
binary, and it **returns `nil` instead of erroring**. It is not even loud.
The same effect corrupts a boolean in experiment J: interpreted
`(= "" (cmp/Diff 1 1))` printed `false`, the binary printed `true`.

This also fires during every `cljgo build`, because `pkg/emit/module.go`
evaluates require forms through the interpreter — the `uuid: nil` line in
`results/G-e2e-real-build.txt` and `results/I-reresolution.txt` is the
compiler's own interpreted pass silently getting the wrong answer.

design/05 §1's self-rebuild (a project-local interpreter binary linking the
project's Go deps) is the mechanism that closes this, and it is not wired.

### 2.2 The conflict: Go silently applies MVS, and cljgo inherits it

End to end through the real pipeline, dep A pinning `go-cmp v0.6.0` and dep
B pinning `v0.7.0` (`results/J-conflict-e2e.txt`):

```
$ cljgo build
cljgo build: installed /tmp/s26-conflict-e2e/conflictapp     <- exit 0, NO diagnostic
$ go version -m ./conflictapp | grep go-cmp
	dep	github.com/google/go-cmp	v0.7.0	h1:wk8382ETsv4JYUZwIsn6YpYiWiBsYLSJiTsyBybVuN8=
```

The generated `go.mod` really does contain the path twice
(`results/E-conflict.txt`); `go mod tidy` collapses it to the higher version
and rewrites the file. Order-independent — listing `v0.7.0` first or
`v0.6.0` first both yield `v0.7.0` (`results/A-duplicate-require.txt`).

**A duplicate `require` is not a Go error.** Measured:

| invocation | result |
|---|---|
| `go mod tidy` on a duplicate-require `go.mod` | exit 0, collapses to the higher version, rewrites `go.mod` |
| `go build` (default `-mod=readonly`), no tidy first | exit 1, `go: updates to go.mod needed; to update it: go mod tidy` |
| `go build` with `-mod=mod` | exit 0, silent MVS |

cljgo runs `go mod tidy` whenever `GoRequires` is non-empty
(`pkg/build/build.go:262`), so the **silent** row is the one cljgo hits.

⇒ **ADR 0048 decision 4's hard error must be actively implemented at the
cljgo layer. It is not the default, and today's code already inherits MVS
through the back door — the exact outcome decision 6 flagged as "these
cannot both be true".** Decision 4 is currently contradicted by the
shipping implementation.

### 2.3 Flattening destroys the provenance Go would have kept

A real Go module graph keeps who-asked-for-what (`results/B-module-graph-mvs.txt`):

```
$ go mod graph | grep go-cmp
example.test/consumer     github.com/google/go-cmp@v0.7.0
example.test/depa@v0.0.0  github.com/google/go-cmp@v0.6.0     <- depa's ask survives
example.test/depb@v0.0.0  github.com/google/go-cmp@v0.7.0
```

cljgo's flattened single module cannot (`results/J`, `results/E-conflict.txt`):

```
$ go mod graph
cljgo.gen/main github.com/google/go-cmp@v0.7.0                <- that is all there is
```

Because cljgo deps are **not** Go modules, cljgo hands Go a degenerate
one-node graph. `go mod why` / `go mod graph` can never name the requirer.
**Whatever conflict message cljgo prints, cljgo must produce it from its own
records — the toolchain cannot help after the flattening.**

### 2.4 Leakage is real and by construction

`results/C-flatten.txt` and `results/G-e2e-real-build.txt`: a namespace
`require-go`s `github.com/google/uuid` and links it, while the module was
pinned by a *different* party. In one module there is no scoping mechanism:
**every module any dependency pins is importable from every namespace in the
program.** A diamond (C→A→X, C→B→X) is therefore not a conflict at the Go
layer at all — it is one node, resolved by MVS per §2.2.

The practical hazard is silent capability creep: code compiles against a
module the project never declared, and keeps compiling until that dep drops
it, at which point an unrelated namespace fails to build.

### 2.5 The write-once guard: no bug — a constraint

My brief asked me to confirm a cross-build staleness bug. **There is no such
bug**, and ADR 0048 lines 48–56 are right. Falsifiable test
(`results/I-reresolution.txt`) — pin a real version, then a nonexistent one
in the same project directory:

```
-- build #1: uuid v1.6.0 (real)        -> cljgo build: installed /tmp/s26-rr/rrapp
-- build #2: uuid v9.9.9 (nonexistent) -> error: go get github.com/google/uuid@v9.9.9: exit status 1
-- build #3: back to v1.6.0            -> installed, runs
```

Build #2 fails, so the edited require set **does** reach `go.mod`.
`buildArtifact` allocates `genDir` fresh per artifact (`os.MkdirTemp`,
`pkg/build/build.go:225`) and removes it on success (`:272`), so `go.mod`
never pre-exists on the project path and the guard never fires there.
(Reached independently here and by coordinator correction; recorded rather
than deleted, per ADR 0048's own discipline.)

What the guard actually does is sequence **two writes within one build**:
`SynthGoMod` receives the real requires at `pkg/build/build.go:246`, and
`WriteProgram`'s internal `nil`-requires call (`pkg/emit/program.go:305`) is
a deliberate no-op preserved by the early return. It is load-bearing by
design.

`results/D-never-overwrite.txt` therefore does **not** document a defect. It
documents the guard doing its job: called a second time with a changed set
against an existing `go.mod`, `SynthGoMod` writes nothing and returns `nil`.

**The constraint this imposes on a dependency-aware `go.mod`** — the actual
answer to item 4 — is:

> A merged `go.mod` (consumer requires ∪ every dep's requires) must be
> written **once, fully-formed, into a directory that did not previously
> exist.** Merging must happen *before* the write, in cljgo, not by
> rewriting or appending to a `go.mod` that already exists.

That constraint is already satisfiable with no restructuring: `CompileProgram`
runs at `build.go:220`, **before** `MkdirTemp` (`:225`) and `SynthGoMod`
(`:246`), so transitive namespace discovery completes before `go.mod` is
written (S30). No rewrite-an-existing-`go.mod` design is needed, and the
"distinguish generated from user-edited" migration ADR 0048's Consequences
worries about does not have to be paid.

### 2.6 Discovery without executing dep build code — the crux

The blocker is sharper than "we must not run dep build code". **Only the
consumer's own `./build.cljgo` is ever read** (S30). A dependency's
`go-require`s are not merely unsafe to obtain — they are *invisible*. And
since `build.cljgo` is evaluated code, and ADR 0048 decision 5 forbids
evaluating a dep's build fn at resolve time, they must arrive as **static,
data-only** input emitted per library.

**Do not invent a manifest format for that. S33 already has the answer.** Its lock
prototype writes a per-dep `:impure {:go-require […] :c-link […] :ffi […]}`
block (`spikes/s33-dep-fetch-cache-lock/prototype/resolve.go:148,181`). A
resolver reading `build.lock.edn` recovers every transitive `go-require`
**with its provenance** and never evaluates a `build` fn
(`results/F-lock-discovery.txt`):

```
-- resolve: read build.lock.edn (S33 schema); never evaluate a build fn --
   libhttp  -> github.com/google/go-cmp v0.6.0
   libuuid  -> github.com/google/uuid   v1.6.0
   libuuid  -> github.com/google/go-cmp v0.7.0
```

The three policies, on that same input:

| policy | result | verdict |
|---|---|---|
| **A. naive append** (today) | duplicate path in `go.mod`; `go mod tidy` silently picks v0.7.0 | ✗ silent wrong-version-forever |
| **B. MVS in cljgo** | picks v0.7.0, **and can say so**: `go-cmp: v0.7.0 (from libuuid) upgraded over v0.6.0 (from libhttp)` | viable, but is a resolution algorithm |
| **C. hard error** | names both requirers and both versions, suggests the override | ✓ matches decision 4 |

Policy C's message, verbatim from the prototype:

```
conflicting go-require for github.com/google/go-cmp
  libhttp pins v0.6.0
  libuuid pins v0.7.0
resolve it in build.cljgo with an explicit (go-require ...) override
```

The lock is *sufficient* — but note it only holds because the lock is
written by the **consumer's** resolve step. It does not answer where the
dep's own `:impure` data came from in the first place; that is S32's
question (can ADR 0021's code-first `build.cljgo` emit such data at publish
time). S31 confirms the *consumption* side works; the *production* side is
unresolved.

## 3. Recommendation

### ADR 0048 decision 4 — keep the hard error, but say it is not free

Ratify "explicit pins, hard error on conflict", and add the measured fact
that makes it non-trivial: **a duplicate require is not a Go error.** The
implementation obligation is explicit —

- merge with provenance **before** writing `go.mod`, keyed by module path;
- fail on any path with two versions, naming both requirers (policy C);
- never emit a duplicate path into `go.mod` (today's code does, §2.2);
- do not rely on `go mod tidy` for anything but transitive `go.sum`.

Decision 4 as written is currently **contradicted by the shipping
implementation**; the ADR should record that, per its own "recorded
corrected rather than deleted" discipline.

Add an escape hatch, or the hard error will be unusable in practice: an
explicit `(go-require app "path" "v")` in the consumer's own `build.cljgo`
**overrides** dep-contributed pins and silences the conflict. This is
Go's `replace`/`exclude` role and it is what makes "one bad conflict per
ecosystem-lifetime" survivable.

### ADR 0048 decision 5 — the lock is the answer; adopt S33's schema by reference

Decision 5 holds and is prototyped. Strengthen it to name the artifact:
transitive `go-require`s come from `build.lock.edn`'s per-dep
`:impure :go-require` block (S33's schema), read as data with `pkg/reader`.
Two constraints S31 adds:

1. **Provenance is mandatory, not decorative.** §2.3 proves the Go toolchain
   cannot reconstruct the requirer after flattening. If the lock does not
   record which dep asked, decision 4's error message is unimplementable.
2. Decision 5 is only half-answered. S32 still owns whether a dep can
   *produce* that data without its build fn being run.

### ADR 0048 decision 6 — resolve it, and lead with the divergence

Decision 6 can now be written rather than left open:

- **Go-require merging**: cljgo-layer merge with provenance, hard error,
  consumer override. Explicitly **do not** delegate to `go mod tidy`. This
  resolves the "these cannot both be true" tension in favour of decision 4.
- **Detectability**: yes, and cheaply — `:impure` is a lock field, readable
  before fetching or building. A project flag ("I must stay pure /
  cross-compilable") can reject an impure dep at resolve time. Recommend it.
- **Add a fourth bullet the ADR does not currently have — dual-mode
  parity.** §2.1 is measured, current, and independent of any dependency
  work: an impure `go-require` silently returns `nil` in the interpreter.
  Every impure dependency inherits this. Decision 6 must state that
  **impurity is not adoptable until the interpreter either links the
  project's Go deps (design/05 §1 self-rebuild) or hard-errors on an
  unreachable third-party package.** Returning `nil` is the worst of the
  three options.

### A recommendation this spike RETRACTED

An earlier draft of this verdict recommended that `SynthGoMod` return an
error instead of `nil` when it declines to write. **That is wrong and would
break the build.** `WriteProgram` (`pkg/emit/program.go:305`) *depends* on
the `nil` return to no-op its second call within the same build (§2.5).
Recorded rather than deleted, because it is exactly the kind of "obvious
fix" a future reader would re-propose. ADR 0048's Consequences already say
this correctly: the write-once rule is *a constraint to respect, not a bug
to fix*.

The only defensible tightening is internal to a future dependency-aware
path: whatever merges requires should assert that the target directory is
fresh, rather than relying on the caller's discipline.

## 4. What forces rework if we get this wrong

1. **Shipping `dep` before the interpreter can reach third-party Go.** Every
   impure dep would be a REPL-vs-binary divergence generator. This is a
   release blocker by CLAUDE.md's own words and it is live *today*.
2. **Letting `go mod tidy` do the merging.** Once `go.mod` is the merge
   point, provenance is already destroyed (§2.3) and decision 4 can never be
   implemented without rewriting the whole merge path.
3. **Designing a fresh manifest format for decision 5.** S33's lock schema
   already carries `:impure`. A second format would fork the resolver.
4. **Ratifying decision 4 while the code silently MVS-es.** The ADR and the
   implementation currently disagree; whichever ships first sets the
   de-facto contract.

## 5. Exit criterion met — yes

| clause | met | evidence |
|---|---|---|
| 1. flattening builds and runs | yes | `results/C-flatten.txt`, `G` |
| 2. conflict decided by measurement | yes | `A`, `B`, `E`, `J`; policy C recommended |
| 3. leakage + diamond | yes | §2.4 — leaks by construction; diamond is MVS, not a conflict |
| 4. write-once guard's real constraint | yes — staleness bug **disproven**; constraint stated (§2.5) | `I`, `D` |
| 5. transitive discovery without eval | yes, via S33's lock schema | `F` |

The spike can state which conflict policy ADR 0048 should adopt: **policy C,
hard error with provenance and a consumer override.**

## 6. Follow-on

- **S32** must answer the production side of §2.6 and owns the cgo /
  cross-compilation half of decision 6, untouched here.
- The §2.1 divergence deserves its own conformance case in the dual harness
  once the interpreter's third-party story exists — and arguably its own
  ADR, since it is a live defect independent of dependency resolution.
