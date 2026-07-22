# Spike S25 verdict — one resolver change genuinely serves both legs

Closed 2026-07-21. Recommendation feeds **ADR 0048**.

**Exit criterion: MET.** An 8-line addition to `ResolveLibPath` — the only
resolver in the codebase — made a namespace whose source lives entirely
outside the consuming project's tree resolve under `cljgo run` AND under
`cljgo build`, with **byte-identical stdout** (`cmp` exit 0) and both
foreign namespaces emitted as Go packages. The emitter needed **zero**
changes. The patch is frozen as `prototype.patch` and was reverted; gates
are green on the clean tree.

## 1. The central de-risk claim — VERIFIED

The brief's claim was that `pkg/emit/module.go` has no independent
resolver, so one change serves both legs. Confirmed by construction and
by measurement:

`emit.CompileProgram` (`module.go:83-103`) installs `moduleCompiler.load`
as `ev.LibLoader` and then *evaluates* the entry through the interpreter.
`pkg/eval/loadLibFile` (`libload.go:37-51`) calls `ResolveLibPath`, and
hands the **already-resolved path** to whatever `LibLoader` is installed.
The emitter therefore consumes a path it never computed. Patching
`ResolveLibPath` alone moved both legs simultaneously — I did not touch
`pkg/emit` at any point.

### Baseline (before the patch), both legs fail identically

```
$ cljgo run fixture/app/src/main.clj
entry start
error: could not locate namespace a.core (no registered provider, and no a/core.clj/.cljg relative to the requiring file)

$ cljgo build -o /tmp/x.bin fixture/app/src/main.clj
entry start
error: could not locate namespace a.core (no registered provider, and no a/core.clj/.cljg relative to the requiring file)
```

(The `entry start` line under `build` is itself the proof that the
emitter evaluates require forms through the interpreter.)

### After the patch, with `CLJGO_PATH=<abs>/fixture/libsrc`

```
run exit=0
build exit=0
bin exit=0
--- run.out ---
entry start
before a.util
loading a.util
after a.util
entry after require
a.core sees 42
--- cmp ---
BYTE-IDENTICAL
--- packages ---
a/core/core.go
a/util/util.go
main.go
```

Both foreign namespaces (`a.core` and its own transitive `a.util`) were
emitted as real Go packages, and load-time side-effect **order** is
preserved across the legs — ADR 0042 §2's registry-triggered loading
carries over to foreign roots unchanged. Raw captures in `results/`.

## 2. The `*file*` trap — resolution is correctly dep-relative

The brief predicted a trap: when a dep loads from a foreign root, does
its *own* sibling require resolve from ITS root or from the consumer's?
The fixture bakes in a decoy — `fixture/app/src/a/util.clj` (in the
CONSUMER's root, printing `DECOY … LOADED`, with `twice` = `* -999`)
competing with `fixture/libsrc/a/util.clj` (`twice` = `* 2`).

**No trap.** Output shows `loading a.util` and `a.core sees 42`, not the
decoy's `-20979`. The emitted `a/util/util.go` binds
`lang.VarFile` to `…/fixture/libsrc/a/util.clj`.

The reason is structural and worth stating in the ADR: `evalLibFile`
(`libload.go:104-115`) pushes `VarFile` to the dep's own path before
evaluating it, and `ResolveLibPath` derives its roots from `dir(*file*)`.
So each file resolves from its own root **by construction**. The decoy is
genuinely reachable — requiring `a.util` directly from the entry loads it
and prints `-20979` — it simply never wins for `a.core`.

**Consequence for ADR 0048:** the load path must be appended to, never
replace, the requiring-file-relative roots. Relative-first is what keeps
a dep's internal structure self-consistent.

## 3. Roots and precedence (as measured)

`corelib.loadLib` (`require.go:158-185`) fixes the outer order, and it is
NOT what the brief assumed:

1. **Registered provider** — consulted FIRST, unconditionally
   (`require.go:164`), before any existence check.
2. **Namespace already present** — embedded core, or previously loaded.
   If present, `ResolveLibPath` is **never called**.
3. **File resolution** via `ResolveLibPath`: requiring-ns-stripped root →
   `dir(*file*)` → *(proposed)* load-path roots, left to right.

So the load path sits at the very bottom. Measured consequence: **a load
path cannot shadow an embedded namespace.** A root containing
`clojure/string.clj` that prints `HIJACKED` was ignored:

```
$ CLJGO_PATH=amb/shadow cljgo run amb/app/shadow.clj
OK
```

**This diverges from JVM Clojure**, oracled against the real CLI
(1.12.5, `-Sdeps :paths`):

```
$ clojure -Sdeps '{:paths ["amb/shadow"]}' -M -e "(require (quote clojure.string)) (println :loaded)"
HIJACKED clojure.string
:loaded
```

The JVM lets a classpath root shadow `clojure.string`; cljgo cannot,
because embedded namespaces exist before any require runs. I regard
cljgo's behavior as *safer* but ADR 0048 must state it deliberately
rather than inherit it by accident — it is a real, permanent divergence
from classpath semantics.

## 4. Ambiguity — first-root-wins, and it matches the JVM exactly

Same namespace `z.core` in two roots:

```
$ CLJGO_PATH=amb/root1:amb/root2 → Z FROM ROOT1
$ CLJGO_PATH=amb/root2:amb/root1 → Z FROM ROOT2
```

JVM oracle, same fixture:

```
$ clojure -Sdeps '{:paths ["amb/root1" "amb/root2"]}' → Z FROM ROOT1
$ clojure -Sdeps '{:paths ["amb/root2" "amb/root1"]}' → Z FROM ROOT2
```

Silent, order-dependent, first-wins — identical to the classpath. This is
JVM-faithful, so it is defensible; but it is also the classpath's
best-known failure mode. Recommend ADR 0048 keep first-wins semantics
(precedence principle: don't differ from Clojure) while adding a
**diagnostic** — a `--warn-on-shadow` style check that reports when a
namespace is resolvable from more than one root. Diagnosing is not
changing semantics.

## 5. `$CLJGO_PATH` — a footgun; use it only as an escape hatch

The prototype used the env var purely as the cheapest way to inject roots.
As a product surface it is the wrong default:

- **Irreproducible builds.** `cljgo build` bakes foreign source into the
  binary. With roots from the environment, the same command on two
  machines silently produces different binaries. That is a worse failure
  than the ambiguity in §4, because it is invisible in the repo.
- It cannot express versions, and there is nothing to review in a PR.

Recommend: **roots are declared in `build.cljgo`** (checked in,
reviewable, versionable) as the primary mechanism. Keep an env override
only if it is explicitly a developer escape hatch, and — critically —
consider having `cljgo build` **refuse** or at minimum warn loudly when
env-supplied roots contribute source to a binary. `cljgo run` may honor
it freely; the run leg is not an artifact.

## 6. The go.mod constraint (coordinator's reframed item)

I verified the coordinator's correction before relying on it, and it is
accurate: `pkg/build/build.go:225` allocates `genDir` fresh per artifact
via `os.MkdirTemp`, removed on success at `build.go:272`. `SynthGoMod` is
called with the real requires at `build.go:246`, *before* `WriteProgram`;
`WriteProgram`'s internal nil-requires call (`program.go:305`) is the
deliberate no-op preserved by the `program.go:329` early return. The
guard sequences two writes within one build. No cross-build staleness
defect exists.

Assessing the reframed question — can a merged go.mod be written once,
fully-formed, into a fresh directory? — **yes, the ordering already
supports it.** `emit.CompileProgram` runs at `build.go:220`, *before*
`MkdirTemp` (225) and before `SynthGoMod` (246). Transitive dep discovery
therefore already completes before the single go.mod write. Merging a
dep's requires needs no rewrite-existing-go.mod design and forces no
generated-vs-user-edited distinction.

The blocker is elsewhere, and S25 puts it in sharp relief: **only the
consumer's `./build.cljgo` is ever read** (`build.go:31`
`BuildFileName`, `LoadPlan` on the project dir). Nothing reads a
dependency's build file — grep confirms no reader outside the project
path. So a foreign-root library's `go-require`s are invisible today, and
a load path is exactly what makes that gap reachable: the root you
resolved `a.core` from is also where its `build.cljgo` would sit.

Since `build.cljgo` is *evaluated* code, reading a dep's build file at
resolve time is arbitrary code execution during dependency resolution.
Recommend ADR 0048 require a **static, data-only** manifest for deps (or
read only a declared data key without invoking artifacts/steps), so
transitive `go-require` discovery never executes a dep's build fn.

## 7. What ADR 0048 decision 2 should say

> **Decision 2 — Namespace resolution gains an ordered load path, appended
> to the requiring-file-relative roots; it does not replace them.**
>
> Roots are consulted in order: (a) the requiring file's own root
> (`dir(*file*)` minus the requiring ns's directory suffix, then
> `dir(*file*)`), then (b) declared load-path roots, left to right.
> Relative-first is load-bearing, not cosmetic: because `evalLibFile`
> rebinds `*file*` to each loaded file, every file resolves its own
> requires from its own root, which is what lets a foreign library keep
> its internal structure and prevents a consumer-local namespace from
> hijacking a dependency's sibling (S25 §2, measured).
>
> A namespace already present (embedded core) or served by a registered
> provider wins over any load-path root; load-path roots cannot shadow
> `clojure.*`. This is a deliberate, permanent divergence from JVM
> classpath semantics, which does allow such shadowing (S25 §3, oracled
> against Clojure 1.12.5).
>
> Among load-path roots, first-wins, silently — matching classpath
> semantics exactly (S25 §4, oracled). A shadowing *diagnostic* is
> in-scope; changing the resolution order is not.
>
> Roots are declared in `build.cljgo`, not the environment. Any
> environment override is a run-leg developer escape hatch and must not
> silently contribute source to a `cljgo build` artifact (S25 §5).
>
> This decision requires **no change to `pkg/emit`**. The module compiler
> consumes paths resolved by `pkg/eval.ResolveLibPath` and has no
> resolver of its own, so the interpreter and AOT legs move together by
> construction — verified end-to-end, byte-identical output (S25 §1).

## Verdict: **yes — high confidence, and cheaper than expected.**

The de-risk claim holds: there is exactly one resolver, and both legs go
through it. The predicted `*file*` trap is already handled correctly by
ADR 0042's existing design. The genuine open risks are governance, not
mechanism — env-supplied roots poisoning build reproducibility (§5) and
transitive `go-require` discovery requiring a non-executable dep manifest
(§6).

### Caveat found incidentally (pre-existing, NOT caused by this change)

`*file*` diverges between the legs. Control fixture with no load path at
all, entirely in-tree:

```
$ cljgo run control/src/entry.clj      → *file*: control/src/entry.clj
$ ./ctl.bin                            → *file*: NO_SOURCE_FILE
```

The entry namespace's `*file*` is `NO_SOURCE_FILE` in an AOT binary while
the interpreter reports the real path. Dependency namespaces are fine
(their `Load()` pushes the real path). This is a REPL-vs-binary
divergence of the ADR 0002/0007 class and is out of scope for S25, but it
should be filed separately — a program that reads `*file*` at top level
behaves differently compiled. It also means `require` from an entry
namespace is unresolvable inside a binary (`ResolveLibPath` returns ""
on `NO_SOURCE_FILE`), which is masked today only because AOT binaries
serve requires from the provider registry.

## Files

- `README.md` — question + exit criterion, written before any code.
- `prototype.patch` — the 8-line `ResolveLibPath` diff, **reverted from
  the working tree after measurement** per ADR 0027. Gates verified green
  after revert (`go build ./... && go vet ./... && gofmt -l … && go test ./...`).
- `fixture/` — out-of-tree lib (`libsrc/a/`), consumer (`app/src/`), and
  the decoy `app/src/a/util.clj` that proves dep-relative resolution.
- `control/` — in-tree control proving the `*file*` divergence is pre-existing.
- `amb/` — ambiguity + embedded-shadowing fixtures (used for both the
  cljgo runs and the JVM Clojure oracle).
- `results/run.out`, `results/bin.out` — the byte-identical captures.

No `go.mod` — the prototype patched the worktree in place and was
reverted; nothing here builds standalone.
