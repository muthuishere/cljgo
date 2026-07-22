# Spike S36 verdict — the unlinked-vs-nil distinction is reliably detectable; the fix is one mode-aware predicate

Closed 2026-07-22. Validates **ADR 0049 decision 2**. Host: darwin/arm64,
go1.26.3, cljgo built from this worktree at `specs/toolkit`.

**VERDICT: PASS. Exit criterion MET on all four clauses.** The interpreter
can distinguish an unlinked third-party `require-go` member from a
legitimate `nil` **with zero ambiguity**, because the two are produced at
different points in the evaluator and one of them is already an explicit,
labelled branch in the code. The fix is ~40 lines, mode-aware so it does
not break `cljgo build`, and yields a message naming module + member +
file. ADR 0049 moves to `proposed`.

All claims below are backed by real captured output under `results/`
(`baseline-*.out` pre-fix, `fixed-*.out` lazy prototype, `eager-*.out`
eager variant, `SUMMARY.txt` side-by-side). The prototype is frozen as
`prototype.patch`; `pkg/` was reverted and the tree left clean.

---

## 1. What the interpreter knows at member-access time — the signal is CLEAN, not murky

The nil is not the result of a partial registration or a reflect miss deep
in a call. It is an **explicit, deliberate `return nil, nil`** at two sites
in `pkg/eval/host.go` (pre-fix lines 27 and 62), reached by exactly one
predicate:

```go
rv, ok := corelib.LookupHostMember(r.Pkg, r.Member)   // seed reflect registry
if !ok {
    if isThirdPartyGoPath(r.Pkg) {   // first path segment contains a "." (a domain)
        return nil, nil              // <-- THE BUG: silent nil for an unlinked module
    }
    return nil, fmt.Errorf("unable to resolve Go member: %s.%s", ...)  // stdlib miss already errors
}
```

So at member-access time the interpreter knows, precisely:

- **`LookupHostMember` HIT** → a linked symbol (stdlib seed set:
  strings/strconv/math/fmt/net/url, or a cljgo-own registered type). Real
  value returned. This is case **(c)** — never reaches the nil branch.
- **miss + `isThirdPartyGoPath` false** → a stdlib package member that
  isn't in the v0 seed set. **Already a hard error** today
  (`unable to resolve Go member`). Not our target, no regression.
- **miss + `isThirdPartyGoPath` true** → a member of a domain-dotted
  third-party import path that is definitionally **not linked** into the
  interpreter binary (only stdlib + cljgo-own are in the registry; S31
  §2.1, S32 §1.3). This is the **only** path that produces the silent nil,
  and it is case **(a)**.

`isThirdPartyGoPath` (host.go:172) is a pure syntactic test on the import
path — `github.com/...` / `golang.org/...` have a dot in the first segment;
`strings` / `net/url` do not. It requires no registry state and cannot be
fooled by a missing dependency, because the classification is of the
*declared path*, not of any fetched artifact.

**The crux for false-positives:** a genuinely-`nil` Clojure value — case
**(b)** — is produced by a *completely different* evaluator op
(`OpConst`, a var deref, `(get {} :x)`, a nil-returning fn, `(when false
…)`). It never enters `evalHost` at all. There is therefore **no code path
on which a real Clojure nil and an unlinked-member nil are the same
value** — they are minted in different functions. The distinction is not
heuristic; it is structural.

---

## 2. The detection predicate — LAZY (at member access), measured

`prototype.patch` replaces each silent `return nil, nil` with:

```go
if e.HostUnlinkedTolerant { return nil, nil }        // AOT discovery pass (see §4)
return nil, e.unlinkedGoError(pkg, member)           // run/REPL: hard error
```

### 2.1 MEASURED — all three cases, `cljgo run` (interpreter leg)

Same three fixtures, before vs after (`baseline-*.out` vs `fixed-*.out`):

| fixture | before fix | after fix (lazy) |
|---|---|---|
| **(c)** `c-stdlib.clj` (strings/strconv/math) | `ToUpper: HELLO …` EXIT 0 | **identical**, EXIT 0 — no false error |
| **(b)** `b-real-nil.clj` (`nil`, `(get {} :x)`, nil fn, `(when false …)`) | four `nil`s, EXIT 0 | **identical**, EXIT 0 — no false error |
| **(a)** `a-thirdparty.clj` (`ws/CloseNormalClosure`, OpHostRef) | `close-normal code: nil` **EXIT 0** | **EXIT 1**, hard error (below) |
| **(a)** `a2-thirdparty-call.clj` (`(ws/FormatCloseMessage …)`, OpHostCall) | `call result: nil` **EXIT 0** | **EXIT 1**, hard error naming `FormatCloseMessage` |

The exact after-fix error (real output, `fixed-a-thirdparty.out`):

```
error: go module github.com/gorilla/websocket is not linked into the
interpreter (accessing member CloseNormalClosure) (at .../a-thirdparty.clj);
build it (cljgo build), or use the self-rebuild flow (design/05 §1)
```

Both the value-position ref (`OpHostRef`) and the call (`OpHostCall`) fire
— the two independent nil sites are both closed.

### 2.2 Lazy vs eager — both work; lazy wins

The **eager** variant (error inside `registerRequireGo`, at `require-go`
time — `eager-*.out`) also passes: stdlib unaffected, third-party errors,
`cljgo build` still succeeds. Difference, measured:

| | LAZY (at member access) | EAGER (at require-go) |
|---|---|---|
| message names | module **+ member + file** | **module only** |
| fires | at the exact divergence point | as soon as `require-go` runs |
| a program that `require-go`s a third-party module but never derefs a member on the interpreter path | runs fine | **errors anyway** |
| REPL ergonomics | can `(require-go …)` to set up, errors only on real use | cannot even declare a third-party module at the REPL |

**Recommend LAZY.** It gives the strictly better message (the member is
the actionable detail), fires only at the true point of divergence, and
does not punish a legitimate `require-go` that the interpreter never
actually exercises. Eager's only edge — failing faster — is marginal
because the member access is almost always the next form.

---

## 3. False-positive guards — enumerated, and the one ambiguous case (which does NOT exist here)

- **stdlib require-go** (`strings`, `os`, `net/url`): `isThirdPartyGoPath`
  is false → the third-party branch is never taken → a hit returns the
  value, a miss errors as it already did. **No false error.** (MEASURED,
  §2.1 case c.)
- **cljgo-own / registered symbols**: present in `hostRegistry` /
  `hostTypeRegistry` → `LookupHostMember` HIT → never reaches the nil
  branch. **No false error.**
- **genuine Clojure nil**: minted by other ops, never enters `evalHost`.
  **No false error.** (MEASURED, §2.1 case b.)

**The one theoretically-ambiguous case, and why it isn't ambiguous here:**
could a *linked* third-party member legitimately return `nil`, and would
the fix wrongly error on it? No — if the member were linked,
`LookupHostMember` would HIT and the value would flow through
`CallHostFn`/`NormalizeResult`, which can return a legitimate normalized
`nil` **without ever reaching the error branch**. The error branch is
reached *only* on a registry miss for a domain-dotted path — i.e. only when
the member is provably absent from the interpreter. Linked-returns-nil and
unlinked-is-absent are distinguished by `ok`, not by the nil itself. There
is no residual ambiguity to resolve.

(One honest edge, noted not fixed: a *macro* whose expansion evaluates a
third-party member at compile time would, under the tolerant build pass,
still see nil — but that is compile-time evaluation of an unavailable
value, out of scope for member-access parity and equally broken today.)

---

## 4. The real subtlety: the fix MUST be mode-aware, or it breaks `cljgo build`

`cljgo build` discovers namespaces by **evaluating** every top-level form
through the interpreter (`pkg/emit/compile.go` `compileStream → evalNode →
ev.Eval`; confirmed — the build prints `close-normal code: nil` during its
interpreted pass). A blanket hard error at member access would therefore
make **every `cljgo build` of a third-party program fail**, since the build
legitimately hits the same unlinked access before emitting a binary that
*does* link the module.

The prototype adds one boolean, `Evaluator.HostUnlinkedTolerant`
(default **false** = `run`/REPL → error; the emitter sets it **true** in
`CompileProgram` and `CompileReader`). The build's discovery pass tolerates
(no-op nil, as today); only the standalone interpreter errors.

### 4.1 MEASURED — build parity guard (exit criterion clause 4)

```
$ cljgo build            # examples/build-websocket, third-party gorilla/websocket
cljgo build: installed .../wsclient          EXIT 0
$ ./wsclient
gorilla/websocket close-normal code: 1000    EXIT 0   (the REAL linked value)
```

So after the fix:

```
                    BEFORE                         AFTER
cljgo run  (interp) close-normal code: nil  →  error: not linked (EXIT 1)
./wsclient (AOT)    close-normal code: 1000     close-normal code: 1000 (EXIT 0)
```

The silent nil-vs-1000 divergence is gone. The interpreter honestly reports
"this capability is not available here"; the binary produces the real
value. ADR 0049's invariant — *never silently a different value* — holds.

The mode signal already exists structurally (only the emitter installs a
capturing `LibLoader`); the explicit boolean makes the intent legible and
is the natural home for the same flag the eventual self-rebuild flow
(design/05 §1) will clear once it links the project's Go deps.

---

## 5. Note for ADR 0049 decision 4 (the dual-harness parity gate)

The parity gate as drafted asserts *"identical output OR identical error."*
For **this** divergence that is not the post-fix reality: the interpreter
leg **errors** while the AOT leg **succeeds with a value** (§4.1) — by
design, because the interpreter genuinely cannot link the module. The gate
must therefore accept a **third** outcome for a declared capability gap:
*"the interpreter leg hard-errors naming the unavailable capability AND the
AOT leg succeeds."* Encoded for the conformance case: assert that
`cljgo run` on the third-party fixture exits non-zero with the
`not linked into the interpreter` message, and the AOT binary exits 0 with
the real value. Without this third branch, the gate would flag the correct
fix as a failure.

---

## 6. Exact text ADR 0049 decision 2 should carry

> Access to a member of a `require-go`'d third-party package that is not
> linked into the interpreter is detected by a single predicate at
> member-access time — a miss in the reflect seed registry for a
> domain-dotted import path (`pkg/eval/host.go`: `!LookupHostMember(...)` &&
> `isThirdPartyGoPath(path)`). That predicate is unambiguous: a linked
> symbol hits the registry (returns its value, nil or otherwise); a stdlib
> miss already hard-errors; a genuine Clojure nil is produced by other
> evaluator ops and never reaches this path. On the match, the interpreter
> raises a hard error naming the module path and member —
> `"go module <path> is not linked into the interpreter (accessing member
> <M>) (at <file>); build it (cljgo build), or use the self-rebuild flow
> (design/05 §1)"` — instead of returning nil (S36, MEASURED). The check is
> **lazy** (at first member access, not at `require-go`), which names the
> member and does not reject a `require-go` the interpreter never
> exercises. It is **mode-aware**: the AOT emitter's namespace-discovery
> pass (`Evaluator.HostUnlinkedTolerant = true`) continues to no-op the
> unlinked access, because the emitted binary links the module and produces
> the real value — so the fix restores parity without breaking `cljgo
> build`. The self-rebuild flow (design/05 §1) later upgrades the error to
> a working call by clearing that flag once the project's Go deps are
> linked; the invariant holds throughout (link, or error — never a silent
> wrong value).

---

## 7. Exit criterion met — yes

| clause | met | evidence |
|---|---|---|
| 1. (a) third-party member → clear error naming module+member, non-zero exit | **yes** | `fixed-a-thirdparty.out`, `fixed-a2-thirdparty-call.out` (EXIT 1) |
| 2. (c) stdlib-only → no false error, identical output | **yes** | `baseline-c-stdlib.out` == `fixed-c-stdlib.out` (EXIT 0) |
| 3. (b) genuine Clojure nil → no false error | **yes** | `baseline-b-real-nil.out` == `fixed-b-real-nil.out` (EXIT 0) |
| 4. `cljgo build` of the same third-party program still succeeds | **yes** | `fixed-build-websocket.out` (EXIT 0) + binary prints 1000 |

Plus: `go build ./... && go vet && gofmt` clean, and `go test ./pkg/eval/...
./pkg/emit/...` **green with the prototype applied** — no existing
conformance regressed.

## 8. Reproducing

```
cd <repo root>
go build -o /tmp/cljgo-s31 ./cmd/cljgo                      # baseline (worktree)
git apply spikes/s36-unlinked-goref-detection/prototype.patch
go build -o /tmp/cljgo-s31 ./cmd/cljgo                      # patched
F=spikes/s36-unlinked-goref-detection/fixtures
/tmp/cljgo-s31 run $F/c-stdlib.clj          # EXIT 0, values
/tmp/cljgo-s31 run $F/b-real-nil.clj        # EXIT 0, nils
/tmp/cljgo-s31 run $F/a-thirdparty.clj      # EXIT 1, "not linked" naming CloseNormalClosure
/tmp/cljgo-s31 run $F/a2-thirdparty-call.clj# EXIT 1, "not linked" naming FormatCloseMessage
(cd examples/build-websocket && /tmp/cljgo-s31 build && ./wsclient && rm -f wsclient)  # EXIT 0, 1000
git checkout pkg/eval/eval.go pkg/eval/host.go pkg/emit/module.go pkg/emit/compile.go  # revert
```

Built binaries are deliberately not committed (CLAUDE.md).
