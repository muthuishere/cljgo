# s71 — java.time patterns for cljg.date: results

**Question.** `cljg.date/format` takes a **Go reference-time layout**
(`"2006-01-02"`). Its own docstring admits there is no JVM equivalent and
sends you to `format-iso` for the portable case. That is a Go implementation
detail leaking through a `cljg.*` surface — exactly what `cljg.*` exists to
prevent — and it cost a real consumer a real capability: koine dropped pattern
formatting entirely rather than leak Go layouts into portable `.cljc` or
hand-translate and silently mis-format.

What does a java.time-shaped replacement cost, and where does the translation
belong?

**Measured 2026-07-31**, darwin/arm64, Apple M5 Pro, go1.26.
Prototype + tests: this directory (`pattern.go`, `pattern_test.go`).
Oracle: real `clojure` CLI + `java.time.format.DateTimeFormatter`.

## Performance — where the translation belongs

Formatting one instant with `"yyyy-MM-dd HH:mm:ss.SSS"`:

| strategy | ns/op | B/op | allocs/op | vs ideal |
|---|---|---|---|---|
| translate on every call | 421.6 | 1440 | 16 | **6.0×** |
| translate once, memoised by pattern | 76.3 | 24 | 1 | 1.08× |
| layout already a constant (comptime) | 70.5 | 24 | 1 | 1.00× |
| *(translation step alone)* | 341.2 | 1416 | 15 | — |

Three readings, and the second is the one that decides the design:

1. **Naive per-call translation is not viable.** 6× slower, and it allocates
   **1440 B against 24 B** — 60× the garbage. In a bri handler stamping a
   timestamp per request at the measured ~78k req/s, that is ~112 MB/s of
   garbage versus ~1.9 MB/s. The scalability problem here is allocation
   pressure, not nanoseconds.

2. **A runtime cache captures 92% of the benefit.** 76.3 ns against the
   comptime ideal's 70.5 — a 5.8 ns / 7.6% gap, and identical allocation
   (1 alloc, 24 B, which is the output string itself and irreducible).

3. Therefore: **memoise, and stop there.** This is a correction of this
   document's first conclusion, which proposed comptime translation for
   literal patterns *plus* a runtime cache for computed ones. Under
   *simplicity first, then performance* (CLAUDE.md, owner 2026-07-31) that
   does not survive its own numbers: comptime buys **5.8 ns, 7.6%**, with
   identical allocation, and its price is a second code path through the
   analyzer. A measured 8% does not earn a second code path — the operational
   test is "would you keep this if it were the same speed", and the answer
   here is no.

   The zero-cost/comptime mandate is not violated by this, because the
   mandate exists to stop us wrapping things in *runtime machinery*. One
   memo table consulted by one function is not a layer, and it already lands
   at 1.08× of the theoretical floor. If the analyzer later grows a
   constant-folding seam for other reasons, folding a literal pattern through
   the *same* `Compile` function is then nearly free and can be revisited —
   but it must not be built as new machinery for 8%.

   What the measurement genuinely settles is the thing that IS worth acting
   on: **never translate per call.** That is the 6×/60× decision, and it is
   one line of caching, not an architecture.

## Stress test: 4,000 patterns differentially against the JVM

The hand-written tests above were green. They were also worthless as evidence,
for the reason the `..000` bug already demonstrated: they assert the cases
their author thought of. So the prototype was stress-tested against the oracle
— 4,000 generated patterns, formatted on both hosts, strings diffed
(`fuzzgen.go` + `oracle.clj` + `differential_test.go`).

**The layout-string design failed on 1,252 of 4,000 (31%).** Four defects, all
silent mis-formats, none reachable from the hand-written suite:

| # | defect | effect |
|---|---|---|
| 1 | `H` mapped to Go's `15` | JVM prints `9`, cljgo printed `09` — a wrong hour. The prototype had a *comment* admitting Go has no unpadded 24-hour form, and shipped anyway. |
| 2 | `E`/`a` run lengths unbounded | accepted `EEEEEEE` and `aa`, which java.time itself rejects |
| 3 | **`Locale.ROOT` guidance was wrong** | ROOT collapses `MMMM`→`Jul` and `EEEE`→`Fri`; only `ENGLISH`/`US` give full names. The advice written to protect the text tokens would have caused a silent divergence on exactly those tokens. |
| 4 | **token/literal adjacency** | see below — the one that killed the design |

Fixing 1–3 took divergence from 1,252 to **108**. It stops there, and defect 4
is why.

### The defect that invalidated the approach

```
ref.Format("Mon-at") => "Fri-at"     substitutes
ref.Format("Monat")  => "Monat"      SILENTLY DOES NOT
ref.Format("Jandu")  => "Jandu"      same for the month name
```

**Go's `Format` decides whether to substitute a text token based on the
literal that FOLLOWS it.** So `EEE'at' Z` — an ordinary pattern — produced
`Monat +0000` where the JVM gives `Friat +0000`. Not a wrong token: a token
that *vanished because of the literal next to it*.

No amount of token-level fixing reaches zero, because the translator emits a
layout string and something else re-parses it under rules the translator
cannot see.

### The repair is a deletion, not an adjacency rule

The mistake was structural: **we had already parsed the pattern, then
re-encoded it into a second string language so Go could parse it again.**
Deleting that second encoding deletes the whole bug class. `direct.go`
compiles to a small op list and formats each field independently.

| | layout string | direct op list |
|---|---|---|
| divergences / 4,000 | **108** | **0** |
| accepted what the JVM rejects | 6 | **0** |
| patterns refused | 1,128 | **15** |
| format | 71.8 ns · 24 B · 1 alloc | 87.9 ns · 24 B · 1 alloc |

Three things to read here, and the third is the point:

1. **Zero divergence.** The design rule is met, and now it is *evidenced*
   rather than asserted.
2. **It refuses 15 patterns instead of 1,128.** Most old refusals were limits
   of Go's *layout language*, not its *calendar* — `H` and bare `SSS` are both
   fine once you stop emitting a layout string. The correct design is also the
   far more capable one.
3. **Correctness costs 16 ns and zero allocation.** Identical B/op and
   allocs/op — and allocation was the scalability term, per the table above.
   My prediction that removing Go's per-call layout re-scan would make it
   *faster* was wrong; it is 22% slower on time. Reported as measured.

The rejected design is retained as `TestLayoutStringDesignIsUnfixable`, which
asserts a divergence ceiling rather than passing — evidence for the ADR, and a
guard against anyone "simplifying" back to it.

## The first hazard (found before the stress test)

The design rule, from the exchange with koine: *a pattern language that errors
on 20% of inputs is fine; one that silently mis-formats on 2% is not.*

Writing the prototype found one violation of that rule that a token-at-a-time
translator gets wrong by construction. Verified against the JVM oracle:

```
"HH:mm:ss.SSS" => 09:04:05.123     the dot is a LITERAL, fraction is 3 digits
"HH:mm:ss,SSS" => 09:04:05,123     so is the comma
"SSS"          => 123              bare digits, no separator at all
```

**Go ties the separator to the fraction.** Its layout spells `.000` or `,000`
as one token, and it has **no spelling at all for a bare fraction**. Translate
`S` on its own to `000` and let the literal `.` through separately, and
`"yyyy-MM-dd HH:mm:ss.SSS"` — the single most common pattern anyone writes —
compiles to `2006-01-02 15:04:05..000` and formats as `09:04:05..123`. A
double dot. Silent, and in the 2% that the rule forbids.

So `S` consumes the separator the preceding literal emitted, and **refuses**
when there is none, because bare `SSS` is genuinely inexpressible in Go.

The second oracle check corrected the prototype's *test*, not its code:
java.time's `X`/`XX`/`XXX` all render a zero offset as `Z`, which is exactly
what Go's `Z07`/`Z0700`/`Z07:00` do. The implementation was right and the
expectation was wrong — worth recording, because "verify against the oracle"
caught it in both directions.

## The refusal set

Refused by name at pattern-compile time, never at format time, never dropped:

| token | why |
|---|---|
| `G` era · `Q` quarter · `D` day-of-year · `w`/`W` week · `F` · `k` · `K` | no Go layout equivalent exists |
| `Y` | week-based year is not the calendar year — use `yyyy` |
| `u` | proleptic year differs from `yyyy` only before year 1 — pick one, reject the other |
| `z` zone name · `V` zone ID | do not round-trip; use `XXX` or `Z` |
| malformed runs (`yyy`, `MMMMM`, `ddd`, `HHH`) | ambiguous width |
| bare `SSS` | inexpressible in Go (above) |

**Locale is stated, not discovered.** `MMM`/`MMMM`/`EEE`/`EEEE` are supported
as **Locale.ROOT only**. On a JVM with a non-English default locale,
`ofPattern("MMM")` yields `juil.` where Go yields `Jul`, and nothing errors —
so a caller wanting cross-host agreement must force `Locale.ROOT` on the JVM
side, which is a thing to do knowingly rather than discover in production.

## What this settles

- Vocabulary is **java.time**, on koine's reasoning rather than taste: it puts
  translation risk on one side. A `.cljc` library's `:clj` branch becomes
  `DateTimeFormatter/ofPattern` with **no translation at all** — the JVM IS
  the oracle for the pattern language — and every Go-hosted Clojure translates
  against that same oracle. `strftime` would be more neutral and would oblige
  us to hand-write the oracle, which is worse.
- **Memoise the translation, and stop there.** One map, consulted by one
  function, at 1.08× of the theoretical floor. Comptime folding buys 7.6%
  and costs a second code path, which under *simplicity first* it does not
  earn; revisit only if the analyzer grows a constant-folding seam anyway.
- Never translate per call: 6× slower and 60× the allocation. That is the
  decision worth making, and it is a cache, not an architecture.
- The existing Go-layout `format`/`parse` are not silently redefined — see
  ADR 0113 for the migration, which must respect the precedence principle.

→ **ADR 0113.**
