VERDICT: PASS — zero-binding direct interop works end-to-end; go/packages latency is a solvable caching problem (warm ~50–90ms per Load, cold 1.3–2.5s once per module, and a gcexportdata-based cache cuts warm loads to ~1ms/pkg).

# S2 — go/packages direct interop (spike results)

Machine: darwin/arm64 (M-series), go1.26.3. Module: `spikes/s2-gopackages-interop`.
Targets: `github.com/google/uuid` v1.6.0, `github.com/gorilla/websocket` v1.5.3, stdlib `os` / `net/http`.

## What was proven

1. **Signature resolution from type facts** (`facts/facts.go`, `cmd/sigdump`).
   One `packages.Load` with `NeedName|NeedTypes` gives `types.Scope().Lookup(name)`
   → `*types.Func` → full signature: params, results, variadic. Trailing-error
   detection is **by type** (`types.Identical(last, types.Universe.Lookup("error").Type())`),
   comma-ok by trailing `*types.Basic` bool — exactly doc 05 §2's contract.
   Resolved correctly: `uuid.NewRandom → (UUID, error)`, `os.Open → (*os.File, error)`,
   `os.LookupEnv → (string, bool)`, `http.NewRequest → (*Request, error)`,
   3-result shapes too (`websocket.NewClient → (*Conn, *Response, error)` → `[a b err]`).
   Whole-package enumeration (registry/completion path) works: uuid 31 exported
   fns (14 trailing-error), os 63 (40), net/http 45 (16).

2. **Generation of direct, non-reflective calls** (`cmd/gen` → `gen-out/main.go`).
   The emitter prototype consumes only the `Sig` facts (result count +
   trailing-error/bool flags) — no hardcoded knowledge of callees. It generates,
   `format.Source`-gates, builds, and runs Go that:
   - plain form: `t0, t1 := uuid.NewRandom(); t2 := []any{t0, normErr(t1)}` — the
     `[v err]` 2-element vector, err slot nil-normalized;
   - `!` form: `if t4 != nil { panic(goError(t4)) }` — unwrap or throw, caught by
     `recover` (try/catch stand-in);
   - comma-ok `[v ok]`, and `errors.Is(err, os.ErrNotExist)` composes on the err
     value out of the vector (Go errors-are-values idiom preserved).
   `grep -c reflect gen-out/main.go` → **0**. Output verified by running the binary.

3. **Third-party = stdlib.** uuid and gorilla/websocket resolve through the same
   path as `os`/`net/http` with zero extra machinery, provided the module is in
   the spike's `go.mod` (the deps.edn → go.mod pinning flow supplies this).
   Note: `go mod tidy` prunes deps nothing imports yet — the real `cljgo deps
   sync` must pin `:go-deps` explicitly (e.g. `go get`-managed requires or a
   generated blank-import file, as this spike does in `deps.go`).

## Measurements: packages.Load wall-clock

`cmd/bench`, fresh process per row unless noted. "Cold" = empty `GOCACHE`
(export data must be compiled); "warm" = populated build cache.

### Mode `NeedName|NeedTypes` (what the emitter needs)

| packages loaded | cold (fresh GOCACHE) | warm (fresh process) |
|---|---|---|
| `os` | 1303 ms | 49–51 ms |
| `net/http` | 2459 ms | 78–80 ms |
| `github.com/google/uuid` | 1541 ms | 63–67 ms |
| `github.com/gorilla/websocket` | 2463 ms | 80–82 ms |
| **all 4 in ONE Load call** | **2506 ms** | **87–95 ms** |

Second Load in the same process ≈ same as warm fresh-process (58–83 ms):
**go/packages caches nothing in-process; every Load shells out to `go list`.**

### LoadMode flags that matter (warm)

| mode | os | net/http | uuid | websocket | note |
|---|---|---|---|---|---|
| `NeedName` only | 19 ms | 24 ms | 21 ms | 25 ms | pure `go list` floor |
| `NeedName\|NeedTypes` | 50 ms | 79 ms | 64 ms | 81 ms | **use this** — types from export data |
| `+NeedImports\|NeedDeps` | 130 ms | 235 ms | 155 ms | 238 ms | 2.5–3× — never request the deps graph |
| `NeedName\|NeedExportFile` | 44 ms | 66 ms | 51 ms | 64 ms | path only, decode deferred to us |
| `+NeedSyntax\|NeedTypesInfo\|NeedDeps` | 192–265 ms | 319–326 ms | 204–266 ms | 322–365 ms | full source typecheck — 4× warm |

Findings:
- **`NeedTypes` WITHOUT `NeedDeps` is the sweet spot.** Dependency types arrive
  transitively through export data and are fully usable (websocket's signature
  references `net.Conn`, `*url.URL`, `http.Header` — all resolved) without
  materializing dep `packages.Package` objects.
- **Batch every pattern into one Load.** 4 packages ≈ the cost of 1 (the ~20 ms
  `go list` process start + module graph walk dominates; per-package marginal
  cost is small). The compiler should collect all `:require-go` targets of a
  build and load them in a single call.
- Cold cost is `go list` compiling export data into GOCACHE for the target and
  its dep closure — paid **once per (module version, toolchain)**, then it's the
  OS's/go's cache forever. Oddly, cold *source* mode is cheaper cold (websocket
  415 ms — no export-data compile) but strictly worse warm and much heavier.

### Cache prototype (`cmd/cachetest`)

| step | time |
|---|---|
| `go list` (`NeedName\|NeedExportFile`, all 4 pkgs, warm) | 62 ms |
| **`gcexportdata.Read` decode of all 4 export files — no subprocess** | **3.4 ms total (~0.8 ms/pkg)** |
| JSON round-trip of serialized `Sig` facts (uuid, 31 sigs, 7.4 KB) | 40 µs |

Decoded export data yields the identical `types.Package` — same lookups, same
signatures. Export file sizes: uuid 362 KB, websocket 1.3 MB, os 2.4 MB,
net/http 10.1 MB (they live in GOCACHE; don't copy them around).

## Cache design recommendation for pkg/host

Three layers, in order of consultation:

1. **In-process map** `pkgPath → *types.Package` (+ derived `Sig` side tables),
   `weak`-held per doc 00 §3.1. Mandatory: go/packages has no in-process cache,
   so a REPL that re-`require-go`s or a compiler that touches N namespaces
   would otherwise pay 50–90 ms per call.
2. **On-disk export-file index** (the real win): a small manifest
   `(goVersion, moduleVersion|stdlib, pkgPath) → exportFilePath + file size/mtime`.
   Warm start: stat-validate, then `gcexportdata.Read` straight off GOCACHE —
   **~1 ms/pkg, no subprocess**. Any miss/stale entry → one *batched*
   `packages.Load(NeedName|NeedExportFile)` for all missing packages (~60–90 ms)
   and reindex. GOCACHE eviction is handled by the stat-check + fallback; no
   need to copy multi-MB export files into our own store.
3. **Serialized `Sig` JSON is NOT the cache currency** for the emitter — it
   loses methods, struct fields, embedded interfaces, and generics that
   `.Method`/`.-Field`/`go/instantiate` emission needs. Keep full
   `types.Package` (via export data) as truth; serialize only tiny derived
   hint tables if the REPL wants completion before first decode.

The cold 1.3–2.5 s belongs to `cljgo deps sync` / first `require-go` of a new
module — surface it there ("compiling type info for …"), never on the hot
compile/REPL path. With layer 2, steady-state interop resolution is ~1 ms/pkg
+ one 60–90 ms batched `go list` per toolchain/deps change.

## Files

- `facts/facts.go` — loader + signature resolver (trailing-error by type)
- `cmd/sigdump` — step 2: resolve/enumerate signatures for uuid, net/http, os
- `cmd/gen` — step 3: generate direct calls with `[v err]` / `!` shaping → `gen-out/main.go` (build+run verified)
- `cmd/bench` — step 4: Load latency matrix (mode × package × cold/warm)
- `cmd/cachetest` — export-data decode path that motivates the cache design
