# ADR 0050 — `publish`: one library, both ecosystems, purity-gated

Date: 2026-07-22 · Status: **proposed** (evidence: spikes S29, S30 — both
closed) · Extends the producer side of **ADR 0013** (every project is a
library); rides the purity axis of **ADR 0048** §6, and depends on the
now-`proposed` **ADR 0049** for its "never silent `nil`" guarantee.

## Context

**Drafted ahead of its evidence, deliberately** (same inverted flow as ADR
0048, per ADR 0027): the falsifiable claims below exist so spikes S29 and S30
have something specific to kill. No decision binds until both close and this
ADR re-issues as `proposed`. If a spike falsifies a claim, the claim changes.

ADR 0013 decided a cljgo project publishes as a Clojure library, a Go library
(`--lib`), a C library, or an executable — but **only the executable exists**;
the producer side is unbuilt. The owner's goal (2026-07-22): cljgo is a citizen
of **both** the Go and Clojure ecosystems — *write pure Clojure once, ship it
to Go developers and to JVM-Clojure developers from one `build.cljgo`, with no
`deps.edn`.*

Two hard constraints, both owner-stated (2026-07-22), settle the shape:

1. **cljgo compiles to Go, not JVM bytecode, and will not grow a JVM backend.**
   So a cljgo library reaches the Clojure ecosystem only as pure Clojure
   **source** that the JVM's own Clojure compiles — never a compiled `.jar`,
   never carrying Java interop.
2. **cljgo does not do Java at all.** Java interop is not a supported host
   operation, in either direction.

**Consume-side interop is deferred, not designed here.** Importing
Clojure-ecosystem libraries is mechanically nearly free (a pure git Clojure dep
is just a source root on ADR 0048 / S25's load path), but almost every real
Clojure library carries Java, so the consumable pure subset is thin. Sequenced
after publish; not foreclosed. This ADR still fixes the **policy** for how a
Java-carrying import must *fail*, because the same purity walk governs it
(decision 4).

## Decision

### 1. `cljgo publish <target>`, declared in `build.cljgo` — no `deps.edn`

One build description, one command. `build.cljgo` stays the single source of
truth (ADR 0021); `publish` produces an artifact and **validates** the library
can actually be that target. No second manifest is introduced, in either
direction.

| `publish <target>` | produces | consumed by | pulled via | purity required | validator gate |
|---|---|---|---|---|---|
| **`go`** | go-gettable Go package (real Go signatures from type hints, `any` otherwise, docstrings→doc comments) | Go developers | `go get <module>` | no — pure *and* `require-go`/ffi | exported surface is Go-expressible, else fail `file:line` |
| **`clojars`** | pure Clojure **source** | JVM-Clojure developers | `deps.edn` `:git/url`+`:sha` (git-coord first; Clojars coord later) | **yes — pure Clojure only** | whole transitive surface pure, else fail `file:line` |
| **`exe`** (default) | standalone native binary | anyone | run it | no | — (ships today) |
| **`c-shared`/`c-archive`** | `.so`/`.a` + header | C / Python / Ruby / … | link it | no | out of scope here (ADR 0013) |

### 2. Purity decides which targets a library qualifies for

| the library uses… | → `go` | → `clojars` |
|---|---|---|
| **pure Clojure only** | yes | **yes** |
| **`require-go` / ffi** (Go-interop) | yes | no — cannot run on the JVM |
| **Java interop** | — cljgo does not do Java — | — |

That is the entire rule. **A pure-Clojure cljgo library is the only artifact
that reaches both worlds** — one `build.cljgo`, `publish go` *and* `publish
clojars`, same source. The moment it uses Go, it is Go-side only, and the
validator says so at publish time with a line number instead of a broken
download.

### 3. Publish validates transitively, fails with `file:line` [S29]

`publish clojars` walks the library's **whole transitive required surface** —
every namespace it requires, recursively — and refuses if *any* reachable form
uses **Go interop** (`require-go`/ffi), naming the offending `file:line`. The
gate is `uses-go-interop?`, **not** "no Java": S30 established that Java interop
runs on the JVM, so it does not disqualify a clojars artifact (see decision 4).
Go interop is precisely the thing that *cannot* run on the JVM, so it is the
whole gate. `publish go` validates the exported surface is Go-expressible.

(A stricter product stance — "a clojars artifact must also run on cljgo,
therefore no Java either" — is available as a future option; it is not adopted
here because the clojars target's contract is "runs on the JVM," which Java
satisfies. Noted, not decided.)

The walk is not new machinery: it is a predicate pass over the existing
ADR-0042 transitive-require traversal `emit.CompileProgram` performs — the
whole-library gate is an OR over the resulting `map[ns]→class`, the
per-namespace gate is a lookup into it. S29 proved this reuse works untouched.

Go-interop is flagged by the mere *presence* of the analyzer's host nodes
(`OpHostRef`/`OpHostCall`/`OpHostMethod`/`OpHostField`/`OpHostNew`, the same
five `pkg/emit/hostfacts.go` already keys on) — no recompilation, no linking.
(`ffi`/`deflib`/`c-link` are not yet AST ops in `pkg/`; the classifier reserves
a **pluggable predicate slot** rather than inventing one — S29 proved N
taint-predicates compose.)

*S29 showed (MET):* a `require-go` buried two levels deep (`core→mid→leaf`) was
caught and cited at `gob/leaf.clj:3` while both pure ancestors passed the
per-namespace gate independently; the pure fixture produced **zero false
positives**; and one traversal yielded both gates, with
`whole-lib == AND(per-ns over all reachable)` verified.

### 4. A Java-tainted import fails LOUD and PER-NAMESPACE — never silent, never a blanket ban [S30]

Purity is a **per-namespace** property, not a per-library one. So when a
(deferred) import contains Java:

- **Do not permanently reject the whole dependency** — its pure namespaces stay
  usable.
- **Hard-error at the point a Java-tainted namespace is required,** with
  `file:line` and "Java interop is unsupported on cljgo's Go host".
- **Never return `nil`/`""`.** That is ADR 0048 §6a's unforgivable
  REPL-vs-binary divergence (ADR 0049). A Java form must fail exactly as loudly
  as an unlinked Go module.
- **Optional strict mode:** a project may opt to reject at *resolve* time any
  dependency whose manifest declares Java taint anywhere, for a portability
  guarantee (mirrors ADR 0048 §6 default-deny).

**Granularity differs by purpose, deliberately:** to *publish* to clojars the
whole transitive surface must be pure (decision 3, whole-library gate); to
merely *use* a namespace in your own build, only *that* namespace need be pure
(this decision). S29 must confirm both fall out of one walk.

**Java detection is a best-effort courtesy, not a correctness gate — S30
settled it.** A sound total `uses-java?` predicate is **not achievable and not
needed.** S30 measured, against a 30-form corpus oracle-checked on Clojure
1.12.5:

- The **static/import Java surfaces** — `(java.util.UUID/randomUUID)`,
  `(System/…)`, `(Math/…)`, `import`, `new`, `clojure.java.*` — already
  **hard-error at analysis with `file:line`**; they are self-identifying.
- The **bare instance dot-form `(.method obj)` is undecidable at analysis
  time.** The identical AST node `(.Replace x)` returns a value on a Go
  `*strings.Replacer` but errors on a string — Go-valid vs Java-miss is decided
  *only* by the runtime receiver, and `parseHostMethod` consults no resolver
  (receiver-type inference is design/05 M4+). An uncalled `(.toUpperCase s)`
  emits no signal at all. So there is a **permanent false-negative floor: all
  bare dot-forms.**

That floor is **safe**, because the failure directions are asymmetric and the
downstream net always catches a missed Java form *loudly*:

- **`build exe` / `publish go`** — the Go compiler rejects an unresolvable
  method at build.
- **`cljgo run`** — ADR 0049 makes the interpreter hard-error (never `nil`).
- **`publish clojars`** — emits pure source; Java runs on the JVM anyway.

Because a **false positive** (wrongly flagging valid Go/pure code) *rejects
good code* while a **false negative** is caught downstream, the Java diagnostic
is **sound-by-construction: certain-only, never guesses the ambiguous dot-form.**
S30's position-aware predicate measured **precision 10/10 — zero false
positives** (recall 10/14, the four misses being exactly the accepted dot-form
tail), correctly leaving `(instance? String x)`, `(catch Exception e)`, and
bare `java.util.UUID` class-ref *values* unflagged.

So the two gates are opposite-polarity and both zero-FP:
- **`publish clojars` → `uses-go-interop?`** (decision 3). Java is *allowed*.
- **Go-host build → `certain-java?`**, a courtesy early diagnostic over the
  self-identifying JVM surfaces only, upgrading a raw compiler error to a named
  one. It is never a gate and never guesses.

## Consequences

- **ADR 0013's producer side gets built** — `go` and `clojars` first;
  `c-shared`/`c-archive` remain its later work.
- **No new resolution machinery.** The validator is the ADR 0048 §6 / S27
  purity walk, reused; the load path (S25) is what a later *import* would ride.
- **Depends on ADR 0049.** Decision 4's "never silent nil" guarantee is only
  true once the interpreter hard-errors on an unlinked/unsupported host call.
  0049 fixes it for the Go case; this extends the same guarantee to Java. **0050
  cannot ship decision 4 before 0049 lands.**
- **`publish clojars` needs a source-jar / coordinate step** (or git-coord
  only, first). Clojars distribution mechanics are a scoping item, not a
  decision here.
- **Out of scope, deliberately:** consuming Maven/Clojars libraries (deferred
  import); a JVM bytecode backend (explicitly never); `c-shared`/`c-archive`
  producer work (ADR 0013).

## Spikes

| spike | question | outcome |
|---|---|---|
| **S29** | Does one transitive walk classify a library's full required surface, catch taint buried 2+ levels deep, and yield both gates? | **MET** — predicate pass over `emit.CompileProgram`, no new walk; taint caught at `gob/leaf.clj:3`; zero false positives; both gates from one map |
| **S30** | Is there a sound low-false-positive `uses-java?` predicate, or is Java indistinguishable from Go interop? | **MET** — no *total* predicate (bare dot-form undecidable), and none needed; the clojars gate is `uses-go-interop?` not "no Java"; the Java diagnostic is certain-only, precision 10/10 |

Both closed with `VERDICT.md` per ADR 0027 §2; this ADR is now `proposed`.
Implementation follows `/opsx:propose`, **after ADR 0049 lands** (decision 4's
"never silent `nil`" is 0049's guarantee).
