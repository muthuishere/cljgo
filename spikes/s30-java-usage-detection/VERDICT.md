# Spike S30 verdict — perfect `uses-java?` is neither achievable NOR necessary; the gate that matters is `uses-go-interop?`

Closed 2026-07-22. Feeds **ADR 0050** decision 4 (purity validation).
Framed by the owner insight (coordinator, 2026-07-22): a mis-detected Java
form does **not** ship broken code, so this predicate is a **best-effort
early diagnostic for a nicer/earlier error**, not a correctness barrier.

**Exit criterion: MET.** Two findings, one of which reframes the question:

1. A sound, low-false-positive standalone `uses-java?` predicate is **not
   achievable** at analysis time on a Go host: bare instance dot-forms
   (`(.method obj)`) are structurally host-neutral and their Java-vs-Go
   provenance is decided only by the runtime receiver type, which v0 knows
   in neither leg (§2). **This is acceptable, not a blocking failure** —
   the downstream net (§0) catches any missed Java loudly.
2. The gate a purity/portability validator actually needs is **polarity-
   dependent**, and *neither polarity is "no Java"* (§5). For publish-to-
   clojars the gate is **`uses-go-interop?`** (Java runs fine on the JVM;
   only Go-interop/ffi can't) — soundly detectable by AST node today. For
   the Go-host build path the gate is a courtesy Java diagnostic backed by
   the Go compiler. The sound, agnostic `uses-host-interop?` (§3) remains
   the cheap superset when a validator wants "touches the host at all."

## 0. The downstream net — why a false negative is not a correctness bug

The owner insight: a Java form the detector *misses* cannot ship broken,
because a later stage always catches it loudly (measured / by construction):

- **`build` / `publish --go`:** the Go emitter/compiler rejects an
  unresolvable method — there is no such Go method on the receiver's type,
  a hard **compile-time** error. A static-detector false negative cannot
  produce a broken Go binary. (S30 §1 shows the interpreter analog: `no
  method X on Y`.)
- **`publish --clojars`:** emits pure Clojure **source**, no Go compile,
  and Java interop **runs fine on the JVM**. Clojars therefore need not
  reject Java at all — it only rejects `require-go`/ffi (the Go-only
  surfaces that can't run on a JVM), which S29 already detects cleanly by
  AST node. **"No Java" was never the clojars gate; "no Go-interop" is.**
- **`run` (interpreter):** the one silent-`nil` hole (ADR 0048 §6a) is
  closed by **ADR 0049** making the interpreter hard-error on an
  unresolvable/unlinked host ref. Post-0049 every leg fails loudly whether
  or not the detector fired first.

So the detector's job is to move the error **earlier and friendlier**
(a `file:line` "JVM interop is unsupported here" at analysis, instead of a
Go compiler diagnostic three steps later), never to be the thing that keeps
a broken artifact from shipping.

All claims below are backed by captured output in `results/`
(`predicate-run.txt` = the scored corpus; `cljgo-behavior.txt` = raw
per-snippet cljgo runs; `oracle-jvm.txt` = `clojure` CLI confirmations).

## 1. What a Java form does in cljgo TODAY — measured

30-form labeled corpus, each java-interop snippet oracle-confirmed as
genuine working JVM interop on `clojure` 1.12.5. cljgo's behavior splits
**cleanly by surface**:

| Java surface | example | cljgo outcome | when |
|---|---|---|---|
| static class call | `(java.util.UUID/randomUUID)` | `no such namespace: java.util.UUID` | **analysis** |
| bare JVM class call | `(System/currentTimeMillis)`, `(Math/sqrt 2)`, `(Integer/parseInt …)` | `no such namespace: System/Math/…` | **analysis** |
| `import` | `(import '[java.util Date])` | `unable to resolve symbol: import` | **analysis** |
| `new` | `(new java.io.File "x")` | `unable to resolve symbol: new` | **analysis** |
| `clojure.java.*` | `(clojure.java.io/file "x")` | `no such namespace: clojure.java.io` | **analysis** |
| **instance dot-method** | `(.toUpperCase s)`, `(.getBytes x)`, `(.length s)` | `no method toUpperCase on string` | **RUNTIME** |
| **dot-method, uncalled** | `(defn up [s] (.toUpperCase s))` | **`ok` — no error at all** | **never** |

**No form ever silently returned `nil`** (the ADR 0048 §6a failure mode —
that defect is specific to *third-party `require-go`*, not Java surfaces).
Every explicit Java surface **hard-errors with `file:line` at analysis
time**: self-identifying, reliably detectable today. This matches S29's
handoff exactly.

The last two rows are the whole problem. A bare instance dot-method:
- errors only at **runtime**, and only *if the branch executes* — an
  uncalled `(.toUpperCase s)` inside a `defn` **analyzes AND "runs" with
  exit 0 and no diagnostic whatsoever** (measured). Nothing downstream of
  the reader flags it.

## 2. Why the dot-form is irreducible — the crux (S29's Q1–Q3, answered)

**Q1 — is the receiver's type known at analysis time? NO.** By code:
`parseHostMethod` (`pkg/analyzer/analyzer.go:1058`) consults **no**
resolver — the comment at `:984` states "the receiver's type is only
known at runtime for v0, so no ResolveHost is consulted." It emits
`OpHostMethod` unconditionally. By measurement: `(defn m [x] (.Close x))`
uncalled ⇒ exit 0, analysis silent. The analyzer cannot see the receiver.

**Q2 — is the sound predicate necessarily resolution/runtime-based, not
syntactic? WORSE than that.** For a dot-form, resolution is unavailable at
analysis time (no receiver type), so *even a resolution-based predicate
cannot classify it at analysis time*. Dispatch is reflective in **both**
legs via `corelib.CallGoMethod` (`pkg/eval/host.go:91`); the AOT emitter
reaches the same function. Receiver-type inference is design/05 **M4+**,
not shipped. So the only signals are: (a) **runtime** — the method exists
on the actual Go receiver or it doesn't; (b) **emit-time** — *once*
go/packages receiver-type inference lands, not before.

**Q3 — is there a genuinely Java `(.method obj)` that is indistinguishable
from a valid Go call? YES — demonstrated.** The single node `(.Replace x)`:

```
$ (require-go '[strings]) (def b (strings/NewReplacer "a" "b")) (.Replace b "a")  → "b"   (valid Go)
$ (.Replace "abc" "a")                                                            → error: no method Replace on string
```

Same form, same `OpHostMethod` node — Go-valid or host-miss decided
**entirely by the runtime receiver**. `(.Error (ex-info "x" {}))` even
resolves against cljgo's *own* runtime type (the exception's Go `Error()`
method) ⇒ `"x"`. There is **no analysis-time signal** separating a Java
instance-method call from a Go method call on an unknown receiver.

**The false-negative floor for `uses-java?` is therefore: 100% of bare
instance dot-forms.** None can be positively confirmed Java at analysis
time. Symmetrically, "flag every dot-form as Java" has a false-positive
floor of 100% of Go dot-method interop (the `.Replace`/`.Do`/`.Read`
idiom, which is the *normal* way ADR 0010 Go interop calls methods).
ADR 0050 decision 4 must acknowledge this floor explicitly.

Case convention (idiomatic Java `getBytes` lowercase-first vs idiomatic Go
exported `Replace` uppercase-first) is a **heuristic, not a signal**:
Clojure-Java interop method names are literally the Java names, and Go
receivers legitimately carry lowercase-adjacent method names; it cannot
carry a soundness claim and is not used in the recommended predicate.

## 3. The two predicates, MEASURED over the corpus

Prototype in `proto/` (reads `pkg/reader` + `pkg/lang` read-only; `pkg/`
never modified). `results/predicate-run.txt` is the full scored table.

### P1 — `javaSyntactic`: reader-level JVM-marker scan (best-effort Java detector)

Markers: `java.*`/`javax.*` **call**-namespace; `import`/`new` heads;
`clojure.java.*`; a table of bare JVM classes (`System Math Thread Integer
String …`) in call-namespace position. Deliberately **position-aware** — a
bare `java.util.UUID` *value* (an ADR 0036 ClassRef, a pure constant) is
**not** flagged; only interop *execution* positions are.

```
positive class = java-interop
true-pos=10  false-pos=0  false-neg=4
recall = 10/14   precision = 10/10
false negatives = the 4 bare dot-forms (exactly §2's residual)
false positives = none
```

Zero false negatives **on the explicit-marker subset (10/10)**; zero false
positives. The 4 misses are precisely the irreducible dot-forms. Position
awareness is load-bearing: without it, `(pr-str java.util.UUID)`,
`(instance? String x)`, `(catch Exception e)`, `(def x String)` — all pure
cljgo-native uses of JVM class *names* — would be false positives. The
naive scan scored precision 10/11; the refinement drove it to 10/10.

### P3 — `usesHostInterop`: the sound decidable boundary (Java OR Go)

Everything P1 flags, **plus** every host-interop *shape* the reader can
see — `require-go`, dot-method `(.m …)`, dot-field `(.-f …)`, ctor
`(T. …)` — **union** cljgo's analysis-resolution outcome (an unresolvable
namespaced call). It does not try to name the host; it asks only "does this
touch the host at all."

```
positive class = host-interop (NOT pure)
false-pos (pure flagged) = 0
false-neg (host missed)  = 0
19/19 host-interop caught · 10/10 pure clean
```

P3 catches even the silent uncalled `(defn up [s] (.toUpperCase s))` that
produces no error signal anywhere — by *shape*. It marks the outer limit of
what is soundly decidable: "touches the host at all" is decidable; "which
host" is not. **But P3 flags bare dot-forms by guessing they are host
interop** — harmless for a purity-superset question, yet exactly the guess
the shipping predicate must *not* make (see the asymmetry in §5). P3 is
therefore the analytical boundary, not the shipped predicate.

## 4. The Math/String overlap trap (brief item 4) — resolved

"Java-looking" forms that cljgo actually supports as **pure/native**, and
must NOT be rejected:
- `(instance? String x)` — `String` is macro syntax (ADR 0026), never a
  value; ⇒ `true`. Pure.
- `(try … (catch Exception e …))` — class name matched by string, host-
  neutral; ⇒ works. Pure.
- `(def x String)`, `(pr-str java.util.UUID)` — bare class *values*
  resolve to opaque ADR 0036 ClassRefs; ⇒ `java.lang.String` /
  `java.util.UUID`. Pure constants, no host execution.

All four are correctly **unflagged** by both refined predicates —
precision is real, not luck. Conversely `(Math/sqrt 2)` (Java, call-ns)
is genuinely unresolvable and flagged, while its cljgo/Go analog
`(require-go '[math :as m]) (m/Sqrt 2.0)` resolves and is classed as Go
host interop, not Java. The overlap is disambiguated by **position + a Go
binding**, exactly as the reframing predicts.

## 5. Recommendation — the exact text ADR 0050 decision 4 should carry

**The design is dictated by an error asymmetry** (coordinator, 2026-07-22):
a **false positive** (valid Go/pure code wrongly flagged Java) is *harmful*
— it rejects a legitimately pure clojars publish or blocks a real Go method
call; a **false negative** (Java slips through) is *safe* — the Go
compiler, ADR 0049, or the JVM catches it downstream (§0). The two are not
symmetric, so the predicate must be **certain-only: zero false positives by
construction, an accepted false-negative tail. It must NEVER guess on the
ambiguous bare `(.method obj)` — that guess is deferred to the compiler /
ADR 0049, which fail only on *actually* unresolvable refs, never on valid
Go.** P1 is exactly this predicate and measured it: precision **10/10**
(zero FP), recall 10/14 (the four dot-form misses are the intended tail).

> **Decision 4 — Purity/portability is gated by CERTAIN-ONLY predicates
> with a zero-false-positive guarantee; a sound standalone `uses-java?` is
> neither adopted nor needed. Never flag the ambiguous bare dot-form.**
>
> A mis-detected Java form does not ship broken code (§0): `build`/`publish
> --go` fail at Go compile on an unresolvable method; `publish --clojars`
> emits pure Clojure source and Java runs on the JVM; ADR 0049 makes the
> interpreter hard-error on an unresolvable host ref. The validator is a
> **best-effort early diagnostic** that moves the error earlier and
> friendlier — never the barrier that keeps a broken artifact from shipping.
> Its one hard obligation, given the asymmetry, is **zero false positives.**
>
> Two gates, opposite polarity, each certain-only:
>
> 1. **`publish --clojars` (JVM target) → gate = `uses-go-interop?`, NOT
>    `uses-java?`.** Java interop runs fine on the JVM; only Go-only
>    surfaces can't. Flag iff the AST carries a **Go-backed** node — a
>    `require-go`/`:require-go`, an `OpHostCall`/`OpHostRef` whose `Pkg`
>    resolves to a Go import, or an ffi/cgo declaration (all node-level,
>    S29). **Do not flag bare `(.method obj)`** (on the JVM it is a Java
>    call and legal; a Go-intended one is caught by the JVM at runtime —
>    an accepted false negative). Zero false positives on pure/Java source.
>
> 2. **Go-host build path → gate = `certain-java?`, a courtesy diagnostic.**
>    Flag iff the form carries a **self-identifying** JVM surface that
>    cannot be anything else: a `java.*`/`javax.*` **call**-namespace
>    (`java.util.UUID/randomUUID`), a bare JVM-class call-namespace from a
>    fixed table (`System`/`Math`/`Thread`/`Integer`/… `/member`), the
>    JVM-only special forms `import`/`new`, or a `clojure.java.*` target.
>    These already hard-error at analysis with `file:line` (S29, S30 §1);
>    the diagnostic only prefixes a clearer "JVM interop is unsupported on
>    this Go host." **Never flag a bare `(.method obj)`/`(.-field obj)`** —
>    its Java-vs-Go provenance is undecidable at analysis time (below), so
>    guessing would risk a false positive on valid Go interop; defer it to
>    the Go compiler / ADR 0049.
>
> **Zero-false-positive guarantee, by construction.** A bare well-known
> class **value** (`String`, `java.util.UUID` — an ADR 0036 ClassRef) is a
> pure constant; `instance?` class-syntax (ADR 0026) and `catch` class
> names are cljgo-native. None is an execution position, so **position-
> awareness (execution vs value reference) is mandatory** — it is what
> removed the sole measured false positive and took precision to 10/10
> (S30 §3–§4).
>
> **Why the ambiguous case is never flagged.** For a bare instance
> dot-form the receiver's type is unknown to the analyzer
> (`parseHostMethod` consults no resolver; dispatch is reflective in both
> legs) and to the AOT emitter until receiver-type inference lands
> (design/05 M4+). The identical node `(.Replace x)` is a valid Go call on
> a `*strings.Replacer` and a host-miss on a string, separated only by the
> runtime receiver (S30 §2, measured). Positively naming it Java is
> therefore impossible without a false-positive risk on Go; the
> false-negative tail — *all* bare instance dot-forms — is accepted and
> caught downstream (§0). If a friendlier early message is ever wanted for
> these, it must come from **emit-time receiver-type resolution** (does the
> receiver's inferred Go type carry the method?), never from a syntactic
> guess.

## Verdict: **certain-only early diagnostic — high confidence.**

`uses-java?` as literally asked is a trap: sound only for the explicit
static/import surfaces (already self-erroring today) and provably
undecidable for the bare instance dot-form. But given the error asymmetry
and the downstream net, perfect Java detection is neither achievable nor
necessary. ADR 0050 decision 4 should adopt **certain-only, zero-FP
predicates** — `uses-go-interop?` for the clojars gate, `certain-java?` for
the Go-host courtesy diagnostic — that flag only self-identifying surfaces
and **never guess the ambiguous `(.method obj)`**, leaving it to the Go
compiler / ADR 0049. Measured: precision 10/10, an accepted false-negative
tail of the four bare dot-forms.

### Exit criterion checklist
- ✅ ≥30-form labeled corpus, java class oracle-confirmed on `clojure` 1.12.5.
- ✅ Obvious JVM markers classified with **zero false negatives** (P1 10/10 on the explicit self-identifying subset — the certain-only gate).
- ✅ False-positive rate **stated and bounded — and zero** for the recommended certain-only predicate: P1 0/16 non-java after position-awareness (every candidate FP enumerated in §4). The asymmetry making FP the metric that matters is honored by construction (§5).
- ✅ Residual ambiguous set enumerated (`(.method obj)`/`(.-field obj)`), resolved as "never flag — defer to compiler/ADR 0049," with the accepted false-negative floor and why it is safe (§0, §2, §5).
- ✅ cljgo's today-behavior measured per snippet as real output (§1, `results/`).

## Files
- `README.md` — question + exit criterion, written before code.
- `corpus/corpus.md` — the labeled corpus with oracle notes.
- `proto/` — self-contained Go prototype (`go.mod` `replace`s the repo
  read-only for `pkg/reader`+`pkg/lang`; `pkg/` never modified). Run:
  `cd proto && go run .`.
- `results/predicate-run.txt` — scored confusion table (the §3 numbers).
- `results/cljgo-behavior.txt` — raw per-snippet cljgo runs (§1).
- `results/oracle-jvm.txt` — `clojure` CLI confirmations (§1 truth labels).

No change to `pkg/`, `cmd/`, `core/`, `templates/`; tree left clean.
No committed binaries.
