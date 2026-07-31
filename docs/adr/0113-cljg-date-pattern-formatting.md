# ADR 0113 — `cljg.date` gets java.time pattern formatting, compiled to ops not to a Go layout

Date: 2026-07-31 · Status: **proposed** — spike **s71** CLOSED
(`spikes/s71-date-patterns/RESULTS.md`, prototype + 4,000-pattern differential
stress test against the JVM oracle).

Extends **ADR 0110**'s `cljg.date`. Governed by **ADR 0101**'s stdlib-gap
program and the precedence principle.

## Context

`cljg.date/format` takes a **Go reference-time layout**:

```clojure
(date/format (date/now) "2006-01-02")   ; a GO layout
```

Its own docstring admits there is no JVM equivalent, warns that a java.time
pattern "would silently mis-format", and sends you to `format-iso` for the
portable case. That is a Go implementation detail leaking through a `cljg.*`
surface — precisely what `cljg.*` exists to prevent, and the reason none of
koine's cljgo branches need `require-go`.

It cost a real consumer a real capability. koine **dropped pattern formatting
entirely** rather than leak Go layouts into portable `.cljc` or hand-translate
and mis-format at the edges: it ships `iso-str`/`parse-iso` and nothing else.
When invited to name an API whose *shape* pushes work over the host boundary,
this is the example they gave.

The gap is ours, so the fix is ours.

### Vocabulary: java.time, and not on taste

koine's reasoning, adopted: **it puts translation risk on exactly one side.** A
`.cljc` library's `:clj` branch becomes `DateTimeFormatter/ofPattern` with *no
translation at all* — the JVM **is** the oracle for the pattern language — and
cljgo, Glojure and let-go each translate against that same oracle. One
reference implementation, N translations, and a conformance check that runs
the same pattern on every host and diffs the string. `strftime` would be more
neutral and would oblige us to hand-write the oracle, which is worse.

`clojure.core` has no date formatting at all, so there is no Clojure oracle to
copy and no precedence-principle conflict: this adds a capability, it does not
rename or reshape one.

### What the stress test proved, and what it destroyed

The design rule: **a pattern language that errors on 20% of inputs is fine;
one that silently mis-formats on 2% is not.**

The prototype's hand-written tests were green. Against 4,000 generated
patterns formatted on both hosts and diffed, the first design — *translate the
java.time pattern into a Go layout string* — **diverged on 1,252 of 4,000
(31%)**. Four silent mis-formats, none reachable from the hand-written suite;
three were token bugs (`H` printing `09` where the JVM prints `9`; unbounded
`E`/`a` runs; and `Locale.ROOT` guidance that was *wrong*, since ROOT collapses
`MMMM`→`Jul` and `EEEE`→`Fri`).

Fixing those took it to 108 and it **stops there**:

```
Format("Mon-at") => "Fri-at"     substitutes
Format("Monat")  => "Monat"      SILENTLY DOES NOT
Format("Jandu")  => "Jandu"      same for the month name
```

Go's `Format` decides whether to substitute a text token **based on the
literal that follows it**. `EEE'at' Z` produced `Monat +0000` against the
JVM's `Friat +0000` — a token that vanished because of its neighbour. No
token-level fix reaches zero, because the translator emits a layout string and
something else re-parses it under rules the translator cannot see.

## Decision

**1. `cljg.date/format-pattern` and `parse-pattern` take a java.time pattern.**
New names; `format`/`parse` keep their Go-layout meaning (see 5).

**2. Compile to an op list. Do NOT emit a Go layout string.** The structural
mistake was re-encoding an already-parsed pattern into a second string
language so `time.Format` could parse it again. A compiled pattern is a small
slice of `(literal | field)` ops, formatted directly, each field independent —
so no op can be swallowed by its neighbour's text.

Measured on the same 4,000-pattern corpus:

| | layout string | **op list** |
|---|---|---|
| divergences | 108 | **0** |
| accepted what the JVM rejects | 6 | **0** |
| patterns refused | 1,128 | **15** |
| format | 71.8 ns · 24 B · 1 alloc | **87.9 ns · 24 B · 1 alloc** |

The refusal row is the one to read: most old refusals were limits of Go's
*layout language*, not its *calendar*. `H` and bare `SSS` are both perfectly
formattable once you stop emitting a layout. **The correct design is also the
far more capable one** — and correctness costs 16 ns at *identical*
allocation, which was the scalability term.

**3. Memoise the compile; do not fold it into the compiler.** Translation is
341 ns and 1,416 B; doing it per call is 6× slower with 60× the garbage — at
bri's measured 78k req/s that is ~112 MB/s of garbage versus ~1.9 MB/s, so
allocation is the term that matters. One memo table consulted by one function
fixes it and lands at 1.08× of the theoretical floor.

Comptime folding of literal patterns buys the remaining **7.6% at identical
allocation** and costs a second code path through the analyzer. Under
*simplicity first, then performance* that does not earn its keep, and the
zero-cost/comptime mandate is not violated: that mandate exists to stop
**runtime wrappers**, and one memo table is not a layer. Revisit only if the
analyzer grows a constant-folding seam for other reasons.

**4. Refuse by name, at compile time, never at format time.** Unrepresentable
tokens (`G` era, `Q` quarter, `D` day-of-year, `w`/`W` week, `F`, `k`, `K`,
`Y` week-year, `u` proleptic year, `z` zone name, `V` zone ID) and malformed
runs are a coded error naming the token. Never a silent drop, never an
approximation.

**5. Locale is `ENGLISH`, and it is stated in the docstring.** Text tokens
(`MMM`/`MMMM`/`EEE`/`EEEE`) match Go's names, which are English-only, so a
`.cljc` author who wants cross-host agreement must pass `Locale.ENGLISH` on
the JVM side. **Not `Locale.ROOT`** — ROOT has no distinct full forms and
yields `Jul`/`Fri` for `MMMM`/`EEEE`. This ADR's first draft said ROOT, which
would have caused a silent divergence on exactly the tokens the advice was
written to protect; the differential corpus caught it.

**6. The existing Go-layout `format`/`parse` are NOT redefined.** Silently
changing what `(date/format inst "2006-01-02")` means would break every
program using it today, and a Go layout is frequently also a valid-looking
java.time pattern, so the breakage would be *silent* — the failure mode this
whole ADR exists to eliminate. They keep working, their docstrings point at
`format-pattern` as the portable choice, and deprecation is a later decision
with its own migration.

## Consequences

- **koine gets pattern formatting back**, and behind a capability flag
  (`:time/pattern-format`) on their side, since let-go may not honour it. Our
  API being right does not oblige every host to have it.
- **Cross-host agreement becomes testable**: same pattern, every host, diff
  the string. The corpus and the oracle script are committed, so any host can
  run it.
- **cljgo carries a java.time pattern subset**, which is new surface to keep
  correct. Bounded by decision 4: everything outside the supported set is a
  refusal, not an approximation, so the surface cannot silently grow wrong.
- Two names now do related things (`format` Go-layout, `format-pattern`
  java.time). Accepted as the price of not breaking working programs;
  decision 6 keeps the ambiguity documented rather than silent.
- The rejected layout-string design is retained in the spike as
  `TestLayoutStringDesignIsUnfixable`, asserting a divergence ceiling, so
  nobody "simplifies" back into it.

## Not in scope

- Locale-aware formatting beyond English. Go's stdlib has no locale data; the
  honest answer is English or a refusal, not a bundled CLDR.
- Time-zone *names* (`z`/`V`). Refused by decision 4 — they do not round-trip.
- Deprecating the Go-layout `format`/`parse` (decision 6).
