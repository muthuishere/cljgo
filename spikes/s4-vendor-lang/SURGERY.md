# S4 surgery log — severing Glojure pkg/lang

Source: `refs/glojure` @ local checkout (glojure `pkg/lang` + the `internal/`
packages it needs), copied 2026-07-11. License: EPL-1.0, repo-level
`LICENSE.md` (the upstream files carry no per-file headers); preserved here as
`LICENSE-glojure.md`. `internal/persistent/vector` keeps its own upstream
`LICENSE` (elvish port).

Every change below is marked `cljgo S4 surgery:` in the code.

## Copied

| what | into | LOC |
|---|---|---|
| `pkg/lang/*.go` (91 files) | `lang/` | 15,818 |
| `internal/murmur3` | `internal/murmur3` | (part of 1,441) |
| `internal/seq` (murmur3's only dep) | `internal/seq` | " |
| `internal/persistent/vector` (+testdata, LICENSE) | `internal/persistent/vector` | " |
| `internal/goid` (dynamic-var goroutine registry) | `internal/goid` | " |

Total copied: 17,259 LOC. NOT copied: `pkg/pkgmap` (interpreter host-class
registry — severed instead, see below).

## Deleted files (interpreter glue) — 560 LOC

1. **`builtins.go` (332)** — `GoAppend/GoMake/GoSlice/GoChanOf/...`: the
   tree-walking interpreter's reflect-based Go-interop builtins. Our emitter
   calls Go directly; none of this belongs in the runtime.
2. **`builtins_test.go` (69)** — its tests.
3. **`class.go` (45)** — JVM-style `*Class` wrapper around `reflect.Type` for
   pkgmap host classes (`java.lang.Math` etc.). Interpreter-only concept.
4. **`environment.go` (114)** — the `Environment` interface (PushScope, Eval,
   ResolveFile, load paths...), `GlobalEnv`, and `Import` via pkgmap. This IS
   the interpreter's coupling point; nothing in a compiled runtime needs it.

## Edited files

- **`namespace.go`** — dropped `pkgmap` import:
  - `seedHostClassImports` → no-op (no host-class registry to seed).
  - `Namespace.Import` → inlined the split-on-last-dot logic that
    `pkgmap.SplitExport` performed.
  - `GlobalEnv.Stderr()` → `os.Stderr` (GlobalEnv died with environment.go).
- **`multifn.go`** — deleted `registerWellKnownMethods` +
  `IsAutoRegisteredMethod` and the constructor call: they seeded a
  print-method for `*Class` (deleted) and existed for the AOT-codegen
  serializer, i.e. compiler glue, not runtime.
- **`keyword.go`** — replaced `go4.org/intern` with stdlib **`unique`**
  (Go 1.23+): `kw *intern.Value` → `kw unique.Handle[string]`,
  `intern.GetByString(s)` → `unique.Make(s)`, `k.kw.Get().(string)` →
  `k.kw.Value()`. `Keyword` stays a comparable 2-word struct; the `hash`
  field is a pure function of the name so `==` identity is preserved. Bonus
  over go4.org: unique's canonical strings are weakly held (GC-reclaimable),
  and it's maintained stdlib.
- **`hashes.go`** — dropped both external hash deps:
  - `mitchellh/hashstructure` (`hashString`) → Java `String.hashCode`
    analog over UTF-16 units (this is what JVM Clojure hasheq feeds to
    murmur3, so it's a parity step, not just a swap).
  - `bitbucket.org/pcastools/hash` (`hashNumber`) → local `mix64to32`
    (murmur3 fmix64 finalizer folded to 32 bits): `hashInt64`,
    `hashUint64`, `hashFloat64`, `hashByteSlice` (fnv32a). float32 widens
    exactly to float64 before hashing so equal floats agree. int64/uint64/
    big.Int of equal magnitude funnel through the same function, keeping
    `Equiv(a,b) ⇒ hash(a)==hash(b)`.
- **`bigint.go` / `bigdecimal.go`** — pcastools calls → the local helpers
  above (same functions hashes.go uses, so BigInt/int64 stay hash-consistent).
- **`persistenthashmap_test.go`** — rewritten on stdlib `testing` only
  (was `stretchr/testify`, the last external dep; assertions ported 1:1).
- **`vector_test.go`** — deleted four `TestGoSliceString*` tests (they tested
  builtins.go's `GoSlice`, deleted above).
- **all files** — import path rewrite
  `github.com/glojurelang/glojure/internal/...` → `cljgo-spike-s4/internal/...`.

## Added (spike verification, 381 LOC)

- `lang/s4_defects_test.go` — pins the two known defects (Equiv≡Equals alias,
  no HAMT transients) so their removal is detectable.
- `lang/s4_smoke_test.go` — smoke tests + benchmarks (vector 10k, HAMT 10k,
  lazy-seq 10k, atom CAS, keyword intern/compare).
- `identity/{pkga,pkgb,identity_test.go}` — §4.4 identity contract: two
  separately compiled packages hoisting the same keyword literals to
  package-level vars, compared with Go `==`, plus 200-goroutine interning.

## Kept deliberately (candidates for later passes, not glue)

- reflect-based fallbacks in `apply.go`, `equal.go`, `hashes.go`, `type.go`,
  `struct.go`, `gomapseq.go`, `slices.go` — the "any Go value is a Clojure
  value" slow paths. Doc 02 calls these acceptable escape hatches; they are
  self-contained (no interpreter imports) and cutting them is a semantics
  decision for the real fork, not severing work.
- `internal/goid` dynamic-var registry — hacky but sanctioned by doc 02 §3.3
  ("v0: goid registry behind PushThreadBindings API").
- `AllKeywords()` registry in keyword.go — trivial, used by REPL completion.
- `agent.go`, `ref.go` (STM stubs), `multifn.go`, sorted maps, ratio/bigdec —
  all standalone, all part of the runtime surface we want.
