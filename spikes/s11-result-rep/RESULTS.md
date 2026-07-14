# S11 — Result/Option representation benchmark

VERDICT: **Variant D wins — two distinct singleton-tag struct types
`okT{v any}` / `errT{v any}` (and `someT`/`none`).** Feeds ADR 0014 D1.

Darwin/arm64, M-series, 10-step railway chain (`and-then` x10), go1.26.3,
medians of 5. Boxed as `any` throughout (the cljgo value model).

| Variant | happy ns/op | happy B/allocs | err ns/op | err B/allocs |
|---|---|---|---|---|
| raw Go error-check (floor) | 3.1 | 0 / 0 | 3.4 | 0 / 0 |
| panic/recover (context) | 53 | 80 / 10 | 128 | 40 / 5 |
| A tagged-ptr `*{tag,val}` | 174 | 344 / 21 | 112 | 208 / 12 |
| B vector `[::ok v]` | **1390** | **6944 / 54** | 898 | 4408 / 33 |
| C struct value | 185 | 344 / 21 | 120 | 208 / 12 |
| **D type-per-tag** | **171** | **256 / 21** | **110** | **152 / 12** |

## Findings
- **D is fastest and lowest-allocation** of every boxed candidate — 256B vs
  344B (A/C) because there's no tag byte and the type IS the tag; ~171ns.
- **B (naked vector) is disqualified**: ~8x slower, 6944B — confirms ADR
  0014's worry about representing Result as a 2-elem vector. Never do this.
- D also gives the cleanest semantics: `ok?`/`err?`/`some?`/`none?` are Go
  **type switches** (no field read); `(ok nil)` vs `none` are distinct types
  so nil-safety (REQUIRED by ADR 0014) is free; Equiv = same type + Equiv on
  `.v`, so `(= (ok 1) (ok 1))` holds; `none` is a single shared sentinel value.
- Printing: distinct types map cleanly to tagged literals `#cljgo/ok`,
  `#cljgo/err`, `#cljgo/just`, `none` (per precedence principle — `just`/`none`,
  not `some`).
- vs panic/recover: D's happy path is ~3x the panic setup but the ERROR path
  is ~1.2x FASTER and railway-composes without unwinding — the right default
  for expected failures; exceptions stay for exceptional ones (ADR 0014).

## Recommendation to D1
Adopt D. Representation: `type okT struct{ v any }`, `type errT struct{ v any }`,
`type justT struct{ v any }`, `var none = noneT{}`. Constructors `ok/err/just`
+ shared `none`. Predicates and `unwrap`/`and-then`/`map-ok` as type switches.
Still ~55x raw Go per step boxed — acceptable for a control-flow value, but the
emitter's typed fast path (ADR 0004 ladder) can unbox known-Result locals later.
