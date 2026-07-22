# ADR 0049 — Dual-mode host-resolution parity: never silently diverge, always hard-error

Date: 2026-07-22 · Status: **accepted** — implemented (commit `b0e591a`;
OpenSpec change `apply-adr-0049-host-parity`, archived). Diagnosis evidence:
spikes S30, S31, S32; fix-validation: spike S36 — all closed. Enforces **ADR 0007**
(JVM-oracle dual harness) and **ADR 0002** (dual-mode, one analyzer) as an
executable invariant. Gates **ADR 0048** and **ADR 0050**.

## Context

The dependency-resolution spike round (S30–S33) was not sent to find bugs. It
found two — both **live on `main` today**, both the failure mode CLAUDE.md
calls *unforgivable*: the interpreter (`cljgo run`, the REPL) and the compiled
binary (`cljgo build`) silently produce **different results for the same
program**, each exiting `0`.

**Divergence 1 — a third-party `go-require` returns `nil` interpreted, the real
value compiled.** Measured *independently* by S31 and S32:

```
$ cljgo run src/main.cljg   →  uuid: nil          (S31)   RTLD_NOW=      (S32)
$ ./app                     →  uuid: 3d91365f-…   (S31)   RTLD_NOW=2     (S32)
```

Both exit `0`. It also corrupts a boolean (interpreted `false`, compiled
`true`). The interpreter cannot reach an **unlinked** third-party Go package —
only the stdlib and cljgo's own dependencies are in the reflect registry — and
**returns `nil`/`""` instead of erroring**. Stdlib `require-go` is consistent
in both legs; the divergence is specific to *third-party* modules. It fires
during **every `cljgo build`**, because the emitter discovers namespaces by
evaluating require forms through the interpreter, and it reproduces against the
repo's own `examples/build-websocket`. Dependencies (ADR 0048) do not introduce
it; they *multiply* it, because a consumer inherits an impure dependency's
`require-go` without ever typing one.

**Divergence 2 — entry-namespace `*file*` and `require`.** Measured by S30 with
an in-tree control fixture: an entry namespace's `*file*` reads
`NO_SOURCE_FILE` in an AOT binary but the real path under the interpreter, and
entry-namespace `require` is therefore unresolvable inside a binary — masked
today only by the provider registry happening to satisfy the common cases.

Both are ADR 0002/0007-class. Neither was caught by CI, because the
`clojure.core`-mediated dual-harness gate that ADR 0007 envisions does not
enforce host-resolution parity. That gap is the real defect; the two
divergences are its symptoms.

## Decision

### 1. The invariant: dual-mode host-resolution parity

**Any reference the two legs would resolve differently must resolve to a hard
error in the leg that cannot satisfy it — never silently to `nil`, `""`,
`false`, or a no-op.** A program that runs under one leg and produces a
different value (rather than an error) under the other is a **release
blocker**, not a known limitation.

This restates ADR 0007 as an *enforceable* rule: divergence is allowed to
manifest only as *"this operation is not available here,"* never as a wrong
answer. A capability the interpreter lacks (linking arbitrary third-party Go)
is honest as an error and dishonest as a `nil`.

### 2. Fix — third-party `go-require` (divergence 1) [S36]

Access to a member of a `require-go`'d third-party package that is **not linked
into the interpreter** must **hard-error** — `"go module <path> is not linked
into the interpreter; build it, or use the self-rebuild flow (design/05 §1)"` —
never return `nil`.

The *capability* fix (actually making third-party Go usable at the REPL) is the
design/05 self-rebuild / self-exec flow, driven by `build.cljgo`'s `go-require`
set (ADR 0021); when that lands, the reference resolves and the error does not
fire. Until then, **error, not `nil`.** The invariant is satisfied either way —
by linking, or by erroring — but never by a silent wrong value.

*S36 showed (PASS) — the distinction is structural, not a heuristic.* The silent
`nil` is an explicit `return nil, nil` at two sites in `pkg/eval/host.go` (`:27`,
`:62`), reached by exactly one predicate: a miss in the reflect-seed registry
(`corelib.LookupHostMember`) for a domain-dotted import path
(`isThirdPartyGoPath`). The three cases separate cleanly and with **zero
measured false positives**:

- **unlinked third-party** → the *only* path reaching the `nil` branch (registry
  miss + domain-dotted path = definitionally not linked) → hard-error.
- **stdlib / cljgo-own** → registry *hit* → real value; a stdlib *miss* already
  hard-errors today.
- **a legitimately-`nil` Clojure value** → minted by other ops (`OpConst`, var
  deref, `(get {} :x)`) and **never enters `evalHost`**. A *linked* member that
  returns `nil` is distinguished by the registry `ok` flag, not by the `nil`.

**Lazy (at member access) over eager (at require-go):** lazy names module +
member + `file`, fires at the exact divergence point, and does not reject a
`require-go` the interpreter never exercises.

**The one subtlety, and why the fix is a mode flag, not a blanket error:**
`cljgo build`'s namespace-discovery pass evaluates these same member-access
forms *through the interpreter*, so an unconditional error would break every
third-party build. The fix is one boolean — `Evaluator.HostUnlinkedTolerant`
(default `false`: `run`/REPL error; the emitter sets `true`: tolerate). Measured
with the prototype patch (`spikes/s36-.../prototype.patch`): after the fix
`cljgo run` errors (exit 1) while the AOT binary still prints the real value
(exit 0) — the silent `nil`-vs-value divergence is gone, and
`go test ./pkg/eval/... ./pkg/emit/...` stays green. Recommended message:
`go module <path> is not linked into the interpreter (accessing member <M>) (at <file>); build it (cljgo build), or use the self-rebuild flow (design/05 §1)`.

### 3. Fix — entry-namespace `*file*` and `require` (divergence 2)

- **`*file*`** in an AOT binary binds to the entry namespace's **logical source
  path** (the relative namespace path, e.g. `main.cljg`), matching the
  interpreter's semantics and the JVM's compiled-`*file*` convention — never
  `NO_SOURCE_FILE`. (Absolute paths do not survive into a shipped binary and
  are not the target; consistency of *semantics* is.)
- **`require`** of a namespace **not compiled into the binary** must
  **hard-error** — `"namespace <ns> was not compiled into this binary"` —
  rather than silently no-op behind the provider-registry mask. A static binary
  legitimately cannot load un-compiled source at runtime; that is fine as an
  error and a divergence as a no-op.

### 4. Enforcement: a dual-harness parity gate

Each divergence lands with a **dual-harness conformance case** (ADR 0007): the
same program run interpreted and AOT-compiled. **The accepted outcomes are
three, not two** — S36 showed a two-outcome gate ("identical output *or*
identical error") would flag *this very fix* as a failure, because the correct
fix makes `cljgo run` error while the AOT binary succeeds. The parity assertion
is therefore:

1. **identical output**, or
2. **identical error**, or
3. **the interpreter hard-errors naming an unavailable host capability *and* the
   AOT leg succeeds** — an *honest* divergence (the interpreter genuinely cannot
   link third-party Go; the binary can), explicitly permitted.

What remains forbidden is the fourth quadrant: **different non-error values**, or
**one leg silently succeeding with `nil`/`""`/`false`/a no-op while the other
produces a real value.** That is the only failure the gate must catch, and it is
exactly what shipped on `main`. This closes the CI-gate gap named in Context and
keeps ADR 0048 and 0050's cross-leg guarantees honest.

## Consequences

- **Unblocks ADR 0048 and 0050.** ADR 0048 §6a and ADR 0050 decision 4 both
  assert "never silent `nil`" for impure/Java references; that guarantee is
  *this ADR's* invariant. Neither can ship its purity policy until §2 lands —
  0049 is on their critical path, ahead of `/opsx:propose` on either.
- **A capability becomes an honest error before it becomes a feature.** The REPL
  will *say* it cannot use a third-party Go module rather than pretending the
  call returned `nil`. The self-rebuild capability (design/05) upgrades the
  error to a working call later, without changing the invariant.
- **The dual-harness parity gate is reusable.** Once it exists, every future
  host-interop addition (ffi, more Go surface, the Java-detection of ADR 0050)
  inherits a divergence check for free.
- **Scope note:** this ADR fixes *host-resolution* parity. It does not attempt
  general REPL-vs-binary equivalence (timing, ordering, GC) — only that a
  resolvable-vs-unresolvable reference never silently yields a different value.

## Spike

| spike | question | outcome |
|---|---|---|
| **S36** | Can the interpreter reliably distinguish an unlinked third-party `require-go` member (→ hard error) from a legitimately-`nil` symbol, without false-erroring on stdlib / cljgo-own symbols? | **PASS** — distinction is structural (`host.go:27/:62`, registry `ok`), zero false positives; fix is the `HostUnlinkedTolerant` mode flag; and it forced the §4 gate to add a third accepted outcome |

Diagnosis was closed evidence (S30/S31/S32 VERDICTs); S36 validated the *fix's*
detection mechanism and froze a working prototype patch
(`spikes/s36-unlinked-goref-detection/prototype.patch`). This ADR is now
`proposed`. Implementation (spike code never merges — ADR 0027) follows via
`/opsx:propose`, ahead of ADR 0048/0050's own spec stages since it gates them.
