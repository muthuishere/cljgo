# ADR 0104 — `require-go` reaches the Go standard library

Date: 2026-07-27 · Status: **proposed**

Extends the Go-interop surface of **ADR 0010** and the hostfacts path of
**ADR 0033**; constrained by the binary-size mandate of **ADR 0023** and the
link-set discipline of **ADR 0046**. Generator shape follows **ADR 0071**'s
`genbri` / ADR 0046's `gencore` precedent.

## Context

`require-go` today reaches five stdlib packages: `strings`, `strconv`, `math`,
`fmt`, `net/url` — and not even all of each (`strings` exposes 9 of ~100
functions). The registry is hand-written `reflect.ValueOf(strings.ToUpper)`
entries in `pkg/corelib/host.go:33-80`.

Everything else fails:

```clojure
(require-go '[os])          ;=> returns cleanly, exit 0, interns NOTHING
                            ;   the later os/Getenv -> "no such namespace: os"
(require-go '[net/http])    ;=> returns cleanly, interns NOTHING
                            ;   the later http/Get -> "no such namespace: http"
(require-go '[nope/nope])   ;=> returns cleanly too — even a package that
                            ;   does not exist at all is silently accepted
```

Both modes fail — `cljgo run` **and** `cljgo build` (measured 2026-07-27,
installed and in-repo binaries).

> **Correction (S56, 2026-07-30).** An earlier draft of this section reported
> `(require-go '[os])` as erroring at the form with `no such namespace: os`.
> Re-measured: it does **not**. It returns cleanly with exit 0 and interns
> nothing, exactly like `net/http` — the error surfaces later at the *call
> site*. So `os` was never a special case; it is one more instance of the
> decision-3 silent-no-op bug, which is also why the bug is worth fixing.

### The driver

[koine](https://github.com/muthuishere/koine) is a portability library giving
one Clojure API across JVM Clojure, cljgo, Glojure and let-go. cljgo is a
tier-1 host there, and it is the **only** one of the four that cannot:

- read an environment variable at all (`cljg.os` is cron/service only, no
  `System/getenv`, `require-go '[os]` fails);
- run a long-lived subprocess with piped stdin/stdout (`cljg.io/exec` is
  run-to-completion), which blocks MCP **stdio** transport;
- stream an HTTP response (`pkg/bri/net_http.go` does `io.ReadAll` then closes
  the body before Clojure sees it);
- read a monotonic clock (`-nano-time` exists and is `defPrivate`).

Each of those is a `cljg.*` shim we would otherwise hand-write. Go's stdlib
already has `os.Getenv`, `exec.Cmd.StdinPipe`, `resp.Body` and `time.Since`.
**One interop change removes four shims** and makes cljgo a first-class Go host
rather than a special case.

### The key discovery — most of this already exists

`resolveHost` (`pkg/eval/host.go:160-170`) already resolves **arbitrary**
members of third-party packages with *zero* hand-written bindings:

> Third-party modules … are NOT in the reflect seed registry. Resolve any
> member as host anyway: the AOT emitter validates and links it from
> go/packages type facts.

Stdlib packages are excluded for exactly one reason: `isThirdPartyGoPath`
requires a dot in the first path segment, and `os` / `net/http` have none. The
capability is present and shipping; the gate simply does not admit stdlib.

**Therefore AOT and interpreted are different-sized problems**, and conflating
them is what made this look large:

| mode | needs | cost |
|---|---|---|
| AOT (`cljgo build`) | admit stdlib paths to the existing third-party route | **small** — emitter and go/packages already do the work |
| interpreted (`cljgo run`) | real `reflect.Value`s | a generated registry |

## Decision

### 1. AOT: stdlib resolves exactly as third-party already does

`resolveHost` admits a stdlib import path declared via `require-go` on the same
terms as a domain-dotted one — the AOT emitter validates the member against
go/packages type facts and links it. `isThirdPartyGoPath` stops being the
gate; the gate becomes "declared in a `require-go` in this ns".

The `!`-suffix bang-retry is preserved: a trailing `!` still yields to the
analyzer so `sc/Atoi!` keeps working.

**Binary size (ADR 0023) is unaffected.** AOT emits direct Go calls and links
only referenced symbols, so a program that never calls `os.Getenv` never links
it. This is strictly better than the reflect registry, which links everything
it names.

### 2. Interpreted: a generated reflect registry, opt-in per package

`cljgo run` needs actual `reflect.Value`s, which Go cannot produce without
naming each symbol in source. So generate them.

- **`cmd/genhost`** — walks a declared package list with `go/packages` (already
  a dependency), emits `reflect.ValueOf(pkg.Sym)` for each exported func/var/
  const and `reflect.TypeOf` for each exported type.
- Output is **checked in**, with a **drift test** that re-runs the generator
  into a temp dir and fails on divergence — the `gencore`/`genbri` contract
  (ADR 0046, ADR 0071), so committed output can never disagree with its source.
- The default set linked into the `cljgo` CLI is deliberately generous —
  `os`, `os/exec`, `time`, `net/http`, `io`, `bufio`, `bytes`, `path/filepath`,
  `encoding/json`, `sort`, `regexp`, `errors`, `context`, `sync` — plus the
  existing five. The CLI is already ~40 MB; this cost lands there and **not** in
  user binaries, which are AOT and covered by decision 1.
- A project may extend the interpreted set via `build.cljgo` (ADR 0021), which
  regenerates locally.

### 3. An unresolvable `require-go` is a LOUD error

`(require-go '[net/http])` currently **returns cleanly and interns nothing**,
so the failure only appears later as `no such namespace: http` at the call
site. Worse, a clean return reads as a successful capability probe — it cost a
koine contributor real time.

`require-go` MUST fail at the `require-go` form, naming the package and the
reason (unknown package · not in the interpreted registry · no go/packages
facts), with a `help:` pointer per ADR 0015/0048.

### 4. Types and multi-returns keep their existing contracts

No new calling convention. `(T,error)` → `[v err]` (ADR 0005), `!` unwrap-or-
throw, member sugar `(.Method r)` / `(.-Field r)` (ADR 0010) all apply
unchanged to the newly reachable packages. `hostTypeRegistry` is generated by
the same pass so `(go/new os.File)` and struct constructors work.

## Consequences

- **Four planned `cljg.*` shims disappear** before being written: env,
  streaming subprocess, streaming HTTP response, monotonic clock. Each becomes
  ordinary Clojure over the Go stdlib.
- **koine's cljgo branch stops throwing named errors** with no koine change —
  the seam absorbs it and consumers never see it. This is the design working.
- cljgo becomes a **first-class Go host**: a portability library can target Go's
  stdlib as the contract rather than cljgo's curated namespaces, which also
  future-proofs it against the next Go-hosted Clojure.
- **Dual-mode parity risk, and it is the real one.** AOT resolves through
  go/packages; interpreted resolves through the generated registry. A package in
  one and not the other is a REPL-vs-binary divergence — the unforgivable
  failure mode (ADR 0007). Mitigation: the generated set is the *declared* set,
  and a conformance test asserts both paths resolve the same package list.
  A package reachable in AOT but absent from the interpreted registry MUST be a
  loud error under decision 3, never a silent no-op.
- `cljg.*` namespaces are **not** deprecated. They stay as the ergonomic,
  Clojure-shaped API (`sh`, `slurp`-alikes); raw `require-go` is the escape
  hatch beneath them. ADR 0085's taxonomy is unchanged.
- Generated code grows the repo. Bounded by the declared list, and the drift
  test keeps it honest.

### Spikes S56–S58 (2026-07-30) — decision 1 IMPLEMENTED and all four blockers CLOSED

Decision 1 was not merely re-argued, it was **built and run**. A worktree off
`dd0c314` with the gate changed from `isThirdPartyGoPath(path)` to "declared via
`require-go`" produces working AOT binaries against the Go stdlib:

| spike | what was built | result |
|---|---|---|
| **S56** | `os.Getenv` in a `cljgo build` binary | ✅ 6.7 MB binary prints the real env var; unset yields `""` (the Go-vs-JVM empty/nil trap koine already normalizes) |
| **S57** | `exec.Command` + `StdinPipe`/`StdoutPipe`, long-lived child, **two** stdin→stdout round trips | ✅ 6.8 MB binary — **this is MCP stdio transport working** |
| **S58** | `resp.Body` read line-by-line through `bufio.NewReader`, plus `time.Now`/`Since`/`Sleep` | ✅ reads the first two lines only, body still open — genuinely streamed, not `io.ReadAll`; monotonic elapsed verified ≥ 10 ms |

Blockers **1, 2, 3 and 4 are therefore closed** — env, streaming subprocess,
streaming HTTP response, monotonic clock — by a change to **one file**. Structs,
methods, fields and interfaces (`io.Reader`) all resolve through go/packages with
zero hand-written bindings, so decision 2's "hostile signature" worry does **not**
apply to the AOT path.

Dual-mode parity was checked explicitly and **holds**: `cljgo run` fails loudly
(`go module os is not linked into the interpreter (accessing member Getenv) …
build it (cljgo build)`) while `cljgo build` produces a working binary. Clojure
precedence still wins, and the `!` bang-retry (`sc/Atoi!`) still works.

#### Two findings that the ADR did not anticipate

**A. `isThirdPartyGoPath` has 3 call sites, but the fix needs 5.** `OpHostMethod`
and `OpHostField` (`pkg/eval/host.go:80`, `:99`) have **no `HostUnlinkedTolerant`
branch at all**. During the AOT discovery pass an unlinked host call returns
`nil`, so *any* method on a host-returned value — `(.StdinPipe cmd)`,
`(.-Body resp)` — dies at build time with `cannot call method .StdinPipe on nil`.
The original uuid spike never hit this because it only chained a plain function
whose result was printed. Both ops need the same `recv == nil &&
e.HostUnlinkedTolerant → nil, nil` guard. Without it decision 1 closes blocker 1
and **none** of blockers 2–4, since every one of those is method-shaped.

**B. The discovery pass runs user Clojure with `nil` for every host result.**
This is by design (args run for side effects) but it means a nil-intolerant pure
function applied to a host value breaks the *build*, not just the run:
`(clojure.string/trim (.ReadString! rdr 10))` fails with `trim expects a string,
got: nil`. This is a real constraint on how koine's wrappers must be written —
host results have to reach a nil-tolerant path, or the discovery pass needs a
typed zero value rather than `nil`. **Worth its own decision before koine's cljgo
branch is written**; it is the most likely source of future "works in `run`,
fails in `build`" reports.

**C. Minor coercion gap.** `lang.Char` does not coerce to Go `byte`:
`(.ReadString! rdr (char 10))` → `cannot coerce lang.Char to Go Byte`. Passing
the integer `10` works. Cheap to fix in `CallHostFn`.

Regression gate on the patched tree: `go build ./...`, `go vet ./...` and
`gofmt -l pkg cmd core` all clean; `pkg/eval` and `pkg/corelib` suites pass.

### Spike (2026-07-27) — decision 1's assumption is PROVEN, and it surfaced a bug

A real third-party module was built end-to-end (`github.com/google/uuid v1.6.0`,
pinned with `go-require` in `build.cljgo`, no hand-written bindings anywhere):

```
$ ./tpdemo        # AOT binary, 6.8 MB
uuid: c6ffa7e5-3149-469f-98a3-f24fbd62a1e1
```

So the zero-bindings go/packages path works, and decision 1 really is the small
change it claims to be.

**Correction to an earlier draft of this ADR.** That draft reported a
REPL-vs-binary divergence, because the same program printed an empty string
under `cljgo build`'s interpreter pass. That was a misreading: the empty result
is `e.HostUnlinkedTolerant` behaving exactly as designed — during the AOT
discovery pass, arguments run for their side effects and the unlinked host call
is a deliberate compile-time no-op, since the emitted binary makes the real
call (`pkg/eval/host.go:26-31, 61-68`). Outside that pass the same code returns
`unlinkedGoError` (ADR 0053 decision 2), which is what `cljgo run` actually
does. There is **no divergence**: `cljgo run` errors loudly, `cljgo build`
produces a working binary. No bug here.

What decision 3 still covers is narrower and real: `(require-go '[net/http])`
itself **returns cleanly and interns nothing**, so the failure surfaces later at
the call site rather than at the form that caused it.

## Every issue found while porting koine (2026-07-27)

koine was built against cljgo as a tier-1 host alongside JVM Clojure, Glojure
and let-go. Porting it surfaced the following. Each row says whether **this**
ADR fixes it. Everything was measured against cljgo 0.1.0-dev (installed *and*
in-repo binaries); nothing here is inferred from documentation.

| # | issue | evidence | severity | fixed by 0096? |
|---|---|---|---|---|
| 1 | **No environment-variable access at all.** `cljg.os` is cron/service only; no `System/getenv`; `(getenv …)` unresolvable; `require-go '[os]` fails | probed all four routes | **blocker** — kills `${ENV_VAR}` expansion, hence all remote-MCP auth | **yes** — `os.Getenv` |
| 2 | **No streaming subprocess.** `cljg.io/exec` takes `:in` as a *string* and returns after exit | `core/cljg/io.cljg:155` | **blocker** — kills MCP stdio transport | **yes** — `exec.Cmd.StdinPipe` |
| 3 | **No streaming HTTP response.** The only shim ends in `io.ReadAll` + `defer resp.Body.Close()`; namespace exposes no other entry | `pkg/bri/net_http.go` | **blocker** — kills streaming LLM responses | **yes** — `resp.Body` |
| 4 | **No *documented* monotonic clock or sleep.** ~~unresolvable~~ **corrected (S56):** `-nano-time` and `-sleep-ms` are `defPrivate` but ARE reachable fully-qualified — `(clojure.core/-nano-time)` returns, `(clojure.core/-sleep-ms 1)` sleeps. So this is a *contract* gap (no public name, no guarantee), not a capability gap | `pkg/corelib/macro_support_builtins.go:6`; re-probed 2026-07-30 | **minor** (was: major) — a private var is not an API, but the capability is there | **yes** — `time.Since` / `time.Sleep` |
| 5 | **`require-go` returns cleanly and interns nothing.** `(require-go '[net/http])` succeeds; the later `http/Get` fails `no such namespace: http` | measured | major — a clean return reads as a successful capability probe; cost real debugging time | **yes** — decision 3 |
| 6 | **core.async aliases are referred into the `user` ns only.** `<!!` `timeout` `chan` `go` resolve in a script but fail at **compile time** inside any `(ns …)` file | `pkg/corelib/chan_builtins.go:632`, applied by `InitUserNS` | major — a working script is a misleading probe for library code | **no** — separate |
| 7 | ~~**Inconsistent private visibility.**~~ **RETRACTED (S56)** — the claim was backwards. Privates in `clojure.core` ARE reachable fully-qualified too (`clojure.core/-nano-time` returns a value). Visibility is *consistent*: privates are hidden from bare resolution and reachable when fully qualified, in both namespaces. There is no inconsistency to fix | re-probed 2026-07-30 | — | n/a — not a bug |
| 8 | **`(str e)` on an exception prints `#object[*lang.ExceptionInfo]`** | measured | minor — a test asserting on error *text* silently passes. `ex-message` works | **no** — separate |
| 9 | **No process exit code.** No `System/exit` equivalent found | measured | minor — but every CLI needs one | partially — `os.Exit` |
| 10 | **`cljg.io` lacks binary `read-bytes`/`write-bytes`.** ~~also mkdir-p, rename, stat, temp-dir~~ **corrected (S56):** those four all exist already — `mkdirs`, `move!` (= rename), `stat`/`size`/`modified`, `temp-dir`/`temp-file` are in `ns-publics 'cljg.io`. Only the binary read/write pair is genuinely missing | `ns-publics 'cljg.io`, re-probed 2026-07-30 | **minor** (was: major) — crash-safe write is already possible today via `move!` | **yes** — `os` |
| 11 | Cannot consume from Clojars | — | — | **already addressed by ADR 0095** (proposed same day, S50/S51 closed MET) |

Nine of eleven are closed by this ADR, seven of them purely because Go's stdlib
already has the primitive. **After the S56 re-measurement three rows shrank** —
one retracted outright (7), two downgraded from major to minor (4, 10) — so the
honest count of *blockers* is **four** (1, 2, 3 and, for koine's purposes, 5),
and all four are now proven closed by the S56–S58 spikes below. That ratio is the argument for doing interop
properly rather than writing `cljg.*` shims one at a time.

**Not cljgo's bug, recorded for context:** `file-seq` takes a string path on
cljgo and a `java.io.File` on the JVM. Both are defensible; the divergence is
absorbed by koine's seam. Listed only so it is not rediscovered.

## Effort

Measured against the code, not estimated in the abstract.

| decision | scope | size | confidence |
|---|---|---|---|
| **1 — AOT stdlib** | ~~3 call sites~~ **5** — `isThirdPartyGoPath` at `pkg/eval/host.go` :26, :61, :168, **plus** the missing `HostUnlinkedTolerant` guards on `OpHostMethod` (:80) and `OpHostField` (:99), see finding A | **DONE — implemented and proven in S56–S58** (~15 lines, one file) | **certain** — working AOT binaries for all four blockers |
| **3 — loud `require-go`** | `unlinkedGoError` already exists; only the form-level check is new | **~2 hours** | high |
| **2 — interpreted registry** | `cmd/genhost`; precedent `genbri` = 235 lines, `gencore` = 147 | **3–5 days** | **medium** |

Decision 2 carries the only real unknown, and it is not the generator. The
interpreter must **reflect-call arbitrary Go signatures**. Today's seed registry
is all simple shapes (`string → string`); `net/http` brings interfaces, structs,
func-typed parameters and channels. Whether the existing reflect call path
handles those is unverified, and it is exactly the kind of unknown that turns
three days into ten. **Spike one hostile signature before committing to it.**

## Sequencing

1. **Decisions 1 + 3 together** — under a day, one file, three call sites.
2. **Re-run koine's `./run-conformance.sh`.** If cljgo's named errors turn into
   passes, all four blockers (issues 1–4) are closed **with no koine change at
   all** — the seam absorbs it and consumers never see it.
3. **Then reassess decision 2.** It buys only the `cljgo run` dev loop: if
   programs are compiled, decision 1 alone is sufficient. It costs 6–10× more
   than steps 1–2 combined, so it should be scheduled on its own merits rather
   than carried along.
4. Issues 6–8 are independent of this ADR and can go whenever.

## Open questions

1. Should the interpreted default set be smaller? It is a CLI-size decision, not
   a user-binary one, so generosity looks cheap — but `net/http` pulls a large
   transitive graph into the CLI.
2. `unsafe`, `syscall`, `plugin` and `os/signal` — admit or refuse? Refusing
   keeps `CGO_ENABLED=0` and `cljgo dist` guarantees (ADR 0077) obviously
   intact.
