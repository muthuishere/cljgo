# s75 — should bri's request map build :headers/:query-params/:body lazily?

Question (owner): bri's per-request cost tracks WHAT THE CLIENT SENT, not
what the handler READS (s72/s73's finding). Does making `:headers`,
`:query-params`, `:body` lazy earn its complexity, versus cheaper eager
fixes?

Machine: darwin/arm64, Apple M5 Pro, this session only. Per CLAUDE.md's own
rule, absolute ns are **not** compared against s72/s73's numbers (different
session/machine) — only ratios *within* this run's tables are used. All
benchmarks go through `requestMap`/`adapt`'s real code path
(`pkg/bri/http_lazy_spike_test.go`, prototype only, not committed), same
`httptest` double the correctness tests use. Benchmarks that read a body
create a fresh `*http.Request` per iteration (`bodyReq(n)`, inside the timed
loop, for both the eager and lazy-untouched variants) because
`http.Request.Body` is single-read; that per-iteration `strings.Repeat`
allocation is included in **every** body row equally, so it cancels in the
deltas but inflates the absolute B/op at n=256 KiB (documented, not hidden).

## 1. The floor — headers/query-params/body never materialised at all

`requestMapFloor`: same base fields (method, uri, query-string, path params,
remote-addr, route) as production, `:headers`/`:query-params`/`:body`
omitted outright. Upper bound on any laziness win, not a proposal.

| headers n | ns/op | B/op | allocs/op |
|---|---|---|---|
| 0  | 522 | 832 | 14 |
| 2  | 456 | 832 | 14 |
| 8  | 430 | 832 | 14 |
| 20 | 409 | 832 | 14 |
| 50 | 382 | 832 | 14 |

Flat — as expected, since the field is never touched. (The mild downward
drift is noise, not signal.)

## 2. Eager (production `requestMap`, this run)

| headers n | ns/op | B/op | allocs/op |
|---|---|---|---|
| 0  | 1896  | 3632  | 62  |
| 2  | 2095  | 3888  | 71  |
| 8  | 4166  | 6352  | 133 |
| 20 | 9983  | 13982 | 247 |
| 50 | 24256 | 38686 | 583 |

| query n | ns/op | B/op | allocs/op |
|---|---|---|---|
| 0  | 2018  | 3632  | 62  |
| 5  | 3229  | 4776  | 93  |
| 20 | 12834 | 16688 | 268 |

| body | ns/op | B/op | allocs/op |
|---|---|---|---|
| 0        | 3820   | 8783    | 73 |
| 16 KiB   | 20492  | 79400   | 90 |
| 256 KiB  | 182258 | 1163405 | 97 |

Confirms s72/s73's shape at this machine's own scale: headers/query cost is
superlinear-looking in the 8→50 range (≈10–11 allocs per header/param, as
already established) and body cost is ≈4.4× the payload at 256 KiB in this
run (includes the shared per-iteration `strings.Repeat` allocation noted
above, so the framework's true multiplier is a bit lower than 4.4×, still
super-linear vs 3.4× previously measured).

## 3. The lazy mechanism

**One mechanism**, `lazyRequestMap` (`pkg/bri/http_lazy_spike_test.go`): a
custom type implementing the full `lang.IPersistentMap` + `IHashEq` +
`Hasher` + `IFn` + `IMeta` surface. Base fields (method/uri/query-string/
path-params/remote-addr/route) are built eagerly in the constructor — cheap
and fixed-cost. `:headers`/`:query-params`/`:body` are each behind their own
`sync.Once`, built only when `ValAt` is asked for that specific key. Any
whole-map operation (`Seq`, `Count`, `Assoc`, `Without`, `Equiv`, `Cons`,
`Empty`, hashing, `pr-str`'s dispatch, `Meta`) forces **all three** fields
via `full()` and delegates to a real `lang.Map` built exactly like
`requestMap` today, then never rebuilds it.

**Structural finding, not a preference:** `Counted` (required by
`IPersistentMap`) embeds an unexported marker method `xxx_counted()` scoped
to package `lang`. No type outside `pkg/lang` can implement it directly —
`grep`-verified, nothing outside `pkg/lang` implements it anywhere in this
repo. The prototype only compiles by embedding a throwaway `*lang.Map` to
promote that marker method and overriding every method actually used. That
is a real, load-bearing constraint on where this mechanism could live: it
cannot cleanly live in `pkg/bri` as ordinary code — either it moves into
`pkg/lang` (extending the vendored Glojure runtime, its own EPL/PROVENANCE
discipline) or it stays in `pkg/bri` leaning on an undocumented Go
promoted-unexported-method trick nothing else in the codebase relies on.

### Untouched (handler ignores the field, matching the s72 benchmark handler)

| headers n | ns/op | B/op | allocs/op |
|---|---|---|---|
| 0  | 506 | 1040 | 15 |
| 2  | 494 | 1040 | 15 |
| 8  | 490 | 1040 | 15 |
| 20 | 499 | 1040 | 15 |
| 50 | 485 | 1040 | 15 |

| query n | ns/op | B/op | allocs/op |
|---|---|---|---|
| 0  | 458 | 1040 | 15 |
| 5  | 521 | 1056 | 16 |
| 20 | 421 | 1056 | 16 |

| body | ns/op | B/op | allocs/op |
|---|---|---|---|
| 0       | 1241  | 6188   | 26 |
| 16 KiB  | 3231   | 22583  | 27 |
| 256 KiB | 19237  | 268416 | 27 |

Matches the floor almost exactly (+~208 B / +1 alloc for the wrapper
struct itself) — at n=50 headers, **50× less time, 37× less memory** than
eager. At 256 KiB body, the 268416 B is almost entirely the shared
`strings.Repeat` test-fixture allocation (see exclusions above), not
framework cost — the real marginal cost of an untouched body is ~0.

### Touched (handler reads exactly one field — the realistic case)

| headers n (only :headers read) | ns/op | B/op | allocs/op |
|---|---|---|---|
| 0  | 518   | 1056  | 16  |
| 2  | 769   | 1312  | 25  |
| 8  | 2707  | 3776  | 87  |
| 20 | 7610  | 11147 | 201 |
| 50 | 22830 | 35955 | 537 |

| query n (only :query-params read) | ns/op | B/op | allocs/op |
|---|---|---|---|
| 0  | 527   | 1056  | 16  |
| 5  | 1833  | 2200  | 47  |
| 20 | 10403 | 14111 | 222 |

At n=50, touching only `:headers` costs 22830 ns / 537 allocs — **within
6% of eager's 24256 ns / 583 allocs**, because eager was also only building
headers+empty-query+empty-body at that scale; the two converge as soon as
the field that's actually expensive is the one being read. **Laziness saves
nothing on what is read — only on what is skipped.**

## 4. Semantic divergence check

Tested directly against `pkg/lang`'s real `Get`/`Equiv`/`PrintString`
functions — the same functions the interpreter and every builtin call
(`pkg/bri/http_lazy_spike_test.go`, `TestLazyRequestMapSemantics`,
`TestLazyRequestMapPrintStrUnforced`, `TestLazyRequestMapAssocOnUnforced`):

- `count` — matches after forcing. **No divergence.**
- `=` (`Equiv`) between an eager map and an untouched lazy map — matches
  (forces the lazy side internally). **No divergence.**
- `pr-str` called on the **unforced** lazy value directly (no explicit
  force first) — byte-identical to the eager map's `pr-str`. **No
  divergence.**
- `dissoc`/`assoc` round-trip (`Without`/`Assoc`) on an unforced lazy map —
  correct, forces once, key removal/insertion both visible after. **No
  divergence.**
- `seq`/`keys` over an unforced lazy map — all lazily-built keys present
  and correct. **No divergence.**
- destructuring-equivalent (`{:keys [headers]}` ≈ individual `ValAt` calls)
  — confirmed that touching `:headers` alone does **not** force
  `:query-params` or `:body` (`lazy3.queryVal`/`lazy3.bodyVal` stay zero
  after only `ValAt(kwHeaders)`). This is a feature, not a divergence: a
  handler that destructures only `:headers` genuinely only pays for
  `:headers`.
- Concrete-type assertions (`.(*lang.Map)`) that would have silently broken
  on a non-`*lang.Map` value: none found anywhere in `pkg/bri` (the only
  interop boundary bri's handlers cross) — grep-verified.

**Zero semantic divergences found in this prototype.** The real price is
structural (§3), not behavioral: a 17-method delegate type, the
`xxx_counted()` embedding workaround, and — going forward — every new Ring
field would need updating in **three** places (base-map construction, the
`ValAt` switch, and `full()`'s aggregation) instead of one flat key-value
list. Forgetting the third is a silent bug (dissoc/seq/pr-str/count would
omit the field while direct `ValAt` would still see it) that this snapshot
happens not to exhibit only because I was the one adding every field.

## 5. The cheap eager body fix — pre-size from ContentLength, no laziness

One-line-scale change: when `r.ContentLength` is known and ≤10 MiB,
`io.ReadFull` into a pre-sized `[]byte(ContentLength)` instead of letting
`io.ReadAll` grow-by-doubling; still one `string(buf)` copy (unavoidable
without changing the Ring `:body` contract from string to something
byte-reusable, which is out of scope here).

| body | eager (today) ns/op, B/op, allocs | presized ns/op, B/op, allocs | Δ |
|---|---|---|---|
| 1 KiB   | 4038 / 13050 / 81   | 4085 / 11873 / 77  | ~flat time, 9% less memory, 4 fewer allocs |
| 16 KiB  | 15967 / 79400 / 90  | 13344 / 57981 / 77 | 16% faster, 27% less memory |
| 256 KiB | 170395 / 1163408 / 97 | 59866 / 795398 / 77 | **2.8× faster, 32% less memory** |

No new type, no cache, no forcing semantics to get right — just a smarter
read. This is the "correctness would have wanted this anyway" case the
doctrine calls out as free: fewer reallocations is strictly better with no
behavioral change, verified against the identical `requestMap` output
shape.

## Verdict

**The cheap eager fixes get real, unconditional wins. Laziness does not
earn its keep as a general mechanism — take the body fix, skip the lazy
request map.**

Applying the operational test:

- **Would you keep it at the same speed?** No. The lazy map's only argument
  is the number, and the number only shows up when a handler touches
  *nothing* — the s72 benchmark handler is written that way specifically
  to isolate framework cost, not because it's the common shape of a real
  API route. Realistic handlers read at least one header (auth, content
  negotiation) or the body; §3's "touched" table shows that as soon as the
  field actually read is the expensive one, cost converges to within 6% of
  eager. The 50×/37× win is real but conditional on a request shape that's
  the exception, not the rule.
- **How much does it buy, and at what price?** Up to 50× time / 37×
  memory, *when* it fires — a number that would ordinarily clear "6× with
  60× the allocation" territory. But the price is not free either: a
  17-method delegate type that cannot be implemented outside `pkg/lang`
  without an undocumented embedding trick, and a maintenance hazard where
  every future Ring field needs three synchronized edit sites instead of
  one. That is exactly the "second code path with a forcing/caching story"
  the doctrine refuses by default, and here it would also cross a package
  boundary the rest of the codebase doesn't.
- **Is it independently justified?** The body fix is — fewer allocations
  is something correctness wants regardless of the laziness question, and
  it needed zero new moving parts. The lazy map is not: its entire
  justification is the benchmark number, and only for the narrow untouched
  case.
- **Count the moving parts.** `requestMap` today is one ~55-line function
  with one exit. `lazyRequestMap` is a new type, 17 delegate methods, three
  `sync.Once` fields, a `full()` aggregator, and a cross-package embedding
  workaround. That is strictly more to hold in your head for a win that
  only shows up off the realistic path.

**Recommendation:** ship the presized-body read (§5) into `requestMap` —
independently justified, no mechanism added. Do **not** adopt the lazy
request map. If production telemetry later shows a large share of routes
generically never touch `:headers`/`:query-params`/`:body` at all (not
proven or measured here — s72's handler is synthetic), that would be the
trigger to revisit, and even then the `pkg/lang`-placement question (§3)
would need its own ADR before writing any of this for real.
