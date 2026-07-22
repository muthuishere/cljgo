# ADR 0051 — JVM-compatible hashing + the clojure.core hash surface
Date: 2026-07-22 · Status: accepted

## Context

cljgo's `lang.HashEq` (pkg/lang/hashes.go) is what hash-maps and hash-sets
use to bucket their keys, and it is what a `hash` function would have to return.
It was *internally consistent* (equal values hashed equal) but computed a
**different algorithm** from JVM Clojure's `clojure.lang.Util.hasheq`. Measured
against real Clojure 1.12.5 on 2026-07-22:

- **Matched** already: string (`String.hashCode` fed through `Murmur3.hashInt`),
  nil, boolean.
- **Diverged**: integers (`(hash 1)` gave -2137753648 vs JVM 1392991556), doubles,
  keywords (`(hash :a)` gave -1757876548 vs -2123407586), symbols, and — because
  their leaves diverged — every vector / list / map / set.

Root causes were all in the **leaf** hashes: integers went through a `fmix64`
fold (`mix64to32`) instead of `Murmur3.hashLong`; doubles folded the same way
instead of `Double.hashCode`; keywords/symbols used arbitrary XOR masks
(`keywordHashMask` / `symbolHashMask`) instead of `Symbol.hasheq`. The collection
combiners were **already correct** (`murmur3.HashOrdered` / `HashUnordered` /
`MixCollHash` over element hasheqs) — they just fed on wrong leaves.

Consequently the five clojure.core hash fns — `hash`, `hash-ordered-coll`,
`hash-unordered-coll`, `mix-collection-hash`, `hash-combine` — were **missing**:
exposing `hash` over the old HashEq would have shipped wrong values into user code
and broken portability (a serialized hash, a `hash`-keyed cache, cross-checking a
value against JVM output would all disagree).

## Decision

1. **`lang.HashEq` matches JVM Clojure 1.12.5's `Util.hasheq` byte-for-byte.**
   The leaf hashes are corrected to Clojure's real algorithm:
   - **Long / integer categories** → `Murmur3.hashLong(v)` (`hashInt64`). All
     signed and unsigned Go integer widths funnel through this so Equiv-equal
     magnitudes still hash equal.
   - **Double** → `Double.hashCode`, i.e. `(int)(bits ^ (bits >>> 32))` over the
     IEEE-754 bit pattern, with `0.0`/`-0.0` → 0 (`hashFloat64`).
   - **Symbol** → `hashCombine(Murmur3.hashUnencodedChars(name), hash(ns))` where a
     missing namespace contributes 0 (`Symbol.hasheq`); a new
     `murmur3.HashUnencodedChars` implements Clojure's char-pair Murmur3.
   - **Keyword** → `Symbol.hasheq + 0x9e3779b9` (`Keyword.hasheq`).
   - **Ratio** → `numerator.hashCode() ^ denominator.hashCode()` over the two
     BigIntegers (`Ratio.hasheq`); a new `javaBigIntegerHashCode` reproduces
     `java.math.BigInteger.hashCode`.
   - **BigInt** → `Murmur3.hashLong` when it fits a Long, else
     `BigInteger.hashCode` (`Numbers.hasheq`).
   - **char** already returned the code point (= `Character.hashCode`); **string /
     nil / boolean** were already correct; **collections** were already correct
     and now inherit correct leaves.

   Verified: a broad oracle table (integers incl. 0 / negative / MaxLong, doubles
   incl. ±0.0 / negatives, many keywords/symbols with and without namespaces,
   strings, char, nested and mixed vectors / lists / maps / sets, ratios, a
   20-digit BigInt) matches Clojure 1.12.5 exactly.

2. **The five hash fns are exposed in clojure.core** (pkg/corelib):
   `hash` = `int(int32(HashEq x))`; `hash-ordered-coll` / `hash-unordered-coll`
   over `Murmur3.hashOrdered` / `hashUnordered`; `mix-collection-hash` =
   `Murmur3.mixCollHash`; `hash-combine` = `Util.hashCombine`. Each has a
   dual-harness conformance test with `;; expect:` values oracled from JVM 1.12.5.

## Consequences

- **Re-bucketing is expected and safe.** Changing HashEq re-buckets every
  hash-map and hash-set. The guardrail is the **equal ⇒ equal-hash invariant**,
  not stable bucket layout: the full conformance suite and every pkg/lang
  hash-map/set test stay green, proving equal values still collide and collection
  internals are intact.
- **Portability restored.** `hash` now returns real Clojure values, so a cljgo
  hash agrees with a JVM Clojure hash for the same value — the point of exposing
  it at all.
- **`hashCode` (the `Hasher.Hash()` path) is left as-is.** This ADR corrects the
  **hasheq** path only, which is what `hash` and collection bucketing use. The
  separate `.Hash()` method (keyword/symbol `hashCode`-style masks) is unrelated
  to `hash` and out of scope; it remains internally consistent.
- **No type proved infeasible.** Every type in the oracle table matches exactly.
  The only documented edge is a Go-interop `uint64` above `MaxInt64`, which wraps
  on the int64 cast — a value the JVM would represent as a BigInt and which the
  cljgo reader never produces; equal-hash within cljgo is preserved because all
  integer widths share the one path.
