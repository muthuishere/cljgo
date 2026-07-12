## Context

ADR 0009 adds value-level compile-time evaluation next to unchanged Clojure
macros. The evaluator that runs macros at AOT time (design/00 §2) also runs
comptime bodies; pkg/emit then embeds the resulting value as a literal in the
namespace `Load()` (design/04 §1). In the interpreter, comptime is just
evaluation. The four things ADR 0009 left to this design round: form names,
the embeddability rules table, build-cache invalidation for file-reading
comptime, and REPL semantics.

## Goals / Non-Goals

**Goals:**
- Settle names, embeddability, caching, and REPL semantics (ADR 0009 §Consequences).
- Zero divergence between interpreted and compiled results (design/03 §7d).
- Embedded values cost nothing at runtime beyond a constant in `Load()`.

**Non-Goals:**
- Any change to defmacro. Comptime types/generics. `#=` read-eval. Laziness
  redesign (interaction handled by the realization rule below, nothing more).

## Decisions

### D1 — Form names (settled)
`comptime`, `comptime-assert`, `embed-file` — exactly the ADR 0009 working
names. Alternatives considered: `const` (collides with Go mental model and a
likely future typed-const story), `eval-when` (CL baggage, order semantics we
don't offer), `$` sigils (un-Clojure). All three are special forms resolved in
the analyzer, not macros, so the emitter can see them structurally.
`embed-file` returns a string by default; `(embed-file "p" :bytes)` returns a
byte array (prints as a vector of ints in readable form).

### D2 — Embeddability rules table (settled; verify each row against the real `clojure` CLI printer before freezing conformance expectations)

| Value class | Embeddable? | Notes |
|---|---|---|
| nil, booleans | yes | |
| integers (fixed + big), doubles, ratios | yes | NaN/±Inf rejected (not readable) |
| strings, chars | yes | |
| keywords, symbols | yes | interned at Load() (design/00 §4.4) |
| lists, vectors, maps, sets — recursively embeddable elements | yes | emitted as literal constructors, sorted deterministically |
| regex patterns | yes | `#"..."` is readable; re-compiled at Load() |
| lazy seqs / seqs | realize, then check | realized fully at compile time; the *realized list* embeds. Non-terminating realization = build hang; documented, `comptime` docs say "force finite" |
| metadata on embeddable values | yes | carried through |
| fns, macros, vars | **error** | positioned compile error E-band per ADR 0015 |
| Go handles, channels, atoms/refs, ports, evaluator internals | **error** | same |
| tagged/custom types without a readable print | **error** | same |

The checker walks the value once (cycle detection via identity set —
persistent data can't cycle, Go values could).

### D3 — Build-cache invalidation (settled)
Comptime bodies run under the compile-time evaluator with an **I/O recorder**:
`embed-file`, `slurp`, and any pkg/lang file-open route through one runtime
hook that records absolute paths. Recorded paths (content hashes) join the
namespace's build-cache key next to source hashes. Non-file impurity (env
vars, network, time) cannot be tracked honestly: reading env vars records the
var name+value into the key; anything else (network, randomness, clock) is
**documented as non-cache-safe** and `cljgo build --no-comptime-cache` forces
re-evaluation. Alternative considered: require declared inputs
(`^:comptime/deps`) — rejected, silently-wrong builds when the declaration
lies; recording is honest by construction for the file case.

### D4 — REPL semantics: inline eval (settled)
In interpreted mode `(comptime body)` evaluates `body` inline, once, at the
point the form is evaluated — semantically `(do body)` plus the embeddability
check (the check runs in BOTH modes so the REPL rejects exactly what the
build rejects; skipping it interpreted would be a 7d divergence).
`comptime-assert` throws (build-fail analog) on false. `embed-file` reads at
eval time. Re-evaluating a form re-runs its comptime — matching "compile time
= eval time" (ADR 0009 §2). Cross-package contract per config rule: consumers
touched are pkg/analyzer, pkg/eval, pkg/emit, cmd/cljgo (cache key) — no
pkg/lang value-model change.

## Risks / Trade-offs

- [Non-terminating comptime hangs builds] → document; future `--comptime-timeout`.
- [Interpreted check makes REPL stricter than plain eval] → intentional (7d).
- [Env-var recording leaks values into cache keys] → hash, don't store raw.
- [Emitter literal printer drifts from reader] → conformance round-trips every table row through both modes.

## Open Questions

- Byte-array embed representation in emitted Go (string const + conversion vs `[]byte{...}`) — perf-spike at implementation, budget per ADR 0004.
