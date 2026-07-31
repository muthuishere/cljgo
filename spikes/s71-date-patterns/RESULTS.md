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

3. Therefore: **comptime translation for literal patterns, runtime cache for
   computed ones, and no cliff between them.** The zero-cost mandate says a
   pattern LITERAL is known at compile time and must be translated then,
   emitting the Go layout as a constant — that is free and it is right. But
   the measurement says a program that builds its pattern at runtime lands
   within 8% rather than falling off a 6× cliff. We can honour the mandate
   without making the dynamic case a trap.

## Correctness — the hazard that would have shipped silently

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
- Translate **at compile time for literals** (free, per the zero-cost
  mandate), **cache at runtime for computed patterns** (within 8%, no cliff).
- Never translate per call: 6× slower and 60× the allocation.
- The existing Go-layout `format`/`parse` are not silently redefined — see
  ADR 0113 for the migration, which must respect the precedence principle.

→ **ADR 0113.**
