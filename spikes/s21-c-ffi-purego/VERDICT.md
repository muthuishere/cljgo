VERDICT: PASS (with one clean corollary) — the interpreter's runtime-typed
`ffi/deflib` binding is buildable on purego via `reflect.FuncOf` +
`reflect.New` + `purego.RegisterFunc`, calling through `reflect.Value.Call`.
REPL-liveness (re-declare in the same process, get a different answer, no
restart) is demonstrated. AOT does not need this dynamic machinery at all —
comptime knowledge of the C signature lets it use the cheaper static form
S7 already proved. Failure modes are honest for everything except wrong C
*signatures*, which is architecturally uncatchable by any FFI mechanism
(cgo included) and must stay a documentation/tooling problem, not a runtime
guarantee.

Run it: `cd spikes/s21-c-ffi-purego && CGO_ENABLED=0 go build -o s21 . && ./s21`
(darwin/arm64, go1.26.3, purego v0.10.1)

## 1. Dynamic registration — the actual risk this spike retires

S7 proved purego marshaling patterns (strings, pointer out-params, callbacks,
return codes) from a Go program where every binding was a **compile-time**
`var f func(...)T; purego.RegisterLibFunc(&f, lib, name)`. That is not
available to an interpreter: `(ffi/deflib sqlite "libsqlite3" (version []
:string) ...)` is data at eval time, and no Go source names `func() string`
anywhere for it to bind to.

`deflib.go` proves the missing piece: build the func's `reflect.Type` from
declared type keywords (`reflect.FuncOf`), `reflect.New` an addressable func
value of that type, and hand `fnPtr.Interface()` — which satisfies purego's
`fptr interface{}` contract exactly as well as a real `*func(...)T` would —
to `purego.RegisterFunc`. purego does not know or care that the func type
was built at runtime; it inspects the `reflect.Type` either way. This is the
mechanism `ffi/deflib`'s expansion/eval must use in interpreted mode.

Demonstrated (`main.go`):
- no-arg → scalar: `posix/getpid` (libSystem/libc) → int32, nonzero.
- scalar-arg → scalar: `libm/cos`, `libm/sqrt` (float64 → float64).
- pointer/buffer arg: `libz/crc32` over a Go `[]byte`'s address, passed as a
  raw `uintptr` (the same S7-proven `:ptr` marshaling pattern — this spike
  does not re-invent buffer marshaling, it proves it composes with dynamic
  registration).

## 2. REPL-liveness

`live.go`'s `demoRedeclareLive`: "turn 1" declares `m1/cos` bound to C
`cos`, calls it (`=> 1`, i.e. `cos(0)`); "turn 2" re-declares the SAME
Clojure name in the SAME process, this time bound to C `sin`, calls it
again (`=> 0`, i.e. `sin(0)`) — no process restart, no rebuild, between the
two. This is the headline claim (design/05: "declare a C function at the
prompt, call it immediately, no rebuild") and it holds.

## 3. AOT path — sketch, not executed

`emit-sketch.go.txt` (uncompiled) shows the emitted form for the same
declaration: package-level `var libm_cos func(float64) float64` +
`purego.RegisterLibFunc` in `init()`. This is IDENTICAL to S7's static
prototype and to this spike's own `BenchmarkPuregoStatic` — because AOT's
compile step (comptime/analyzer, ADR 0009) knows the C signature before
`go build` ever runs, it never needs `reflect.FuncOf`. Both interpreted and
AOT funnel through the same three purego primitives (`Dlopen`/`Dlsym`/
`RegisterLibFunc`), so there is exactly one ABI-marshaling story to get
right, tested once, trusted in both modes (design/00 §5's non-negotiable).
**Recommendation: `ffi/deflib` compiles to the static form whenever the
analyzer can see the declaration at comptime (the normal case — it's a
top-level form), and falls back to the dynamic form only for genuinely
runtime-constructed declarations** (e.g. `ffi/fn` one-offs built from a
string at the REPL, or a declaration assembled by a macro from
non-literal data). This spike did not implement that analyzer-side
decision — it only establishes that both code paths exist and produce
the same observable behavior, which is what the recommendation rests on.

## 4. Failure honesty

| Failure | When | What happens |
|---|---|---|
| Missing library | declaration (`Declare`/dlopen) | named error, e.g. `ffi/deflib nope: dlopen "..." failed: ... (declaration-time failure, no functions bound)` — no function gets bound, nothing is left half-registered |
| Missing symbol | declaration (`Dlsym`) | named error identifying both the C symbol and the Clojure name, e.g. `ffi/deflib libm2: symbol "..." (fn not-a-real-fn) not found` |
| Wrong arity | call | caught BEFORE `reflect.Call`, named error: `ffi: lib/cos expects 1 arg(s), got 2` — never a raw reflect panic |
| Variadic declaration | declaration (deflib expansion) | rejected outright, citing ADR 0011/S7's finding, telling the author to wrap it in cgo/Go instead |
| **Wrong signature** (right symbol, wrong declared types) | call | **NOT caught.** `libm4/cos_mis_typed` declared `cos` as `(:int32)->:int32` instead of `(:float64)->:float64`: no panic, no error, returns `0` (silently wrong; real `cos(0)=1`). This is a hardware-ABI-register-class mismatch (int vs float register file) — no marshaling layer, cgo included, can validate that the DECLARED type matches the LIBRARY's real type without either a header (C has none machine-checkable at this boundary) or a manifest. **Verdict: this stays a documentation/testing-discipline problem, not a runtime guarantee** — `ffi/deflib` should ship worked examples + a "verify against a known-good call first" convention, and where a library ships versioned headers, a future `ffi/from-header` (out of scope here) could close this gap by parsing them. |

Declaration-time failures never leave the `Lib` half-usable: `Declare`
returns `(nil, err)` on the FIRST bad decl, registering nothing.

## 5. Platform matrix (purego's own guarantees, from its README)

| Tier | OS/arch | Notes for cljgo |
|---|---|---|
| 1 (primary) | darwin amd64/arm64, linux amd64/arm64, windows amd64/arm64 | cljgo can claim full `ffi/deflib` support here; this spike ran darwin/arm64 |
| 1, cgo-gated | iOS amd64/arm64, Android amd64/arm64 | require `CGO_ENABLED=1` even for purego itself on these — cljgo's "no C toolchain" claim narrows to desktop/server targets only |
| 2 (best-effort) | freebsd/netbsd amd64/arm64, linux 386/arm/loong64/ppc64le/riscv64/s390x, windows 386/arm | document as "may work, not covered by conformance" |
| Struct-by-value | unsupported on 386/arm/riscv64; callback-only restricted on windows 386/arm; loong64/ppc64le/arm64-windows support struct args but not in callbacks | `ffi/deflib`'s docs should say struct-by-value C APIs are a "wrap it in Go/cgo" case on anything but the amd64/arm64 desktop trio, consistent with design/05's existing "struct-by-value: wrap in Go" position |

**cljgo's honest claim**: `ffi/deflib` is fully supported on darwin, linux,
and windows, amd64 and arm64 — the same tier cljgo already targets for
release binaries (design/00 roadmap) — with everything else "best effort,
not conformance-tested".

## 6. Per-call overhead (darwin/arm64, Apple M5 Pro, go1.26.3)

Same call — `cos(0.5)`, chained (`x = cos(x)`) to defeat any silly constant
folding — measured four ways, `go test -bench=. -benchtime=2s`:

| path | ns/op | vs pure Go |
|---|---|---|
| pure Go (`math.Cos`) | 11.83 | 1.0x (floor) |
| cgo (`C.cos`) | 16.33 | 1.4x |
| purego static (S7-style, compile-time func var — the AOT path) | 132.2 | 11.2x |
| purego dynamic (this spike's `reflect.FuncOf` mechanism — the REPL path) | 221.2 | 18.7x |

Reading these numbers: cgo's per-call cost is negligible (a few ns of Go/C
transition). purego's dlsym-bound call costs roughly 100-130ns regardless of
static/dynamic — that is purego's own trampoline/marshaling overhead, not
something `ffi/deflib` can shave further at the static tier. The dynamic
(interpreted) path adds ~90ns on top of static for `reflect.Call`'s argument
boxing/type-checking — proportionally large (1.7x static) but small in
absolute terms next to interpreter dispatch overhead cljgo already pays for
every Clojure form. **None of these numbers are disqualifying**: even at
221ns/call, `ffi/deflib` calls in a hot loop cost about the same order of
magnitude as one Go map lookup — fine for "call a C library function",
wrong tool for "replace a tight numeric kernel" (which is exactly what
design/05 already says: wrap performance-critical C in a Go package via cgo
and import it normally, don't lean on live FFI for that).

## 7. Dependency-policy recommendation (the "zero-deps" question)

CLAUDE.md's zero-deps root module currently has no third-party deps
(`go.mod` shows only `golang.org/x/tools`, itself a dev/build-time tool).
Three options for where `purego` lives once `ffi/deflib` ships:

1. **Vendor purego into the main module.** Keeps `cljgo` a single `go.mod`
   with no user-visible new dependency, but drags dlopen/reflect-heavy code
   into every binary whether or not the user ever writes `ffi/deflib`, and
   commits cljgo to tracking purego's own upstream (beta, per its own
   README) inside our tree.
2. **Require purego only when `ffi/deflib` is used** (lazy/optional module,
   loaded via Go's build-constraint or a plugin-style separate package
   that `cljgo build` only imports when the emitted program actually uses
   `ffi/`). Keeps the zero-deps promise for programs that never touch FFI,
   at the cost of a more complex build/emit story (conditional imports,
   `go.mod` surgery at build time).
3. **Generated-module approach** (this spike's own structure is a preview
   of it): a program's `cljgo build` output is its OWN Go module (already
   true per ADR 0028 "runtime as published module") — `ffi/deflib` usage
   simply adds `github.com/ebitengine/purego` to THAT module's `go.mod`,
   never to cljgo's own. The interpreter (for REPL use) carries purego as
   an ordinary dependency of `cljgo` itself (option 1, but scoped to the
   REPL/dev binary only, not to every emitted program).

**Recommendation: option 3.** It matches the existing ADR 0028 shape exactly
(emitted programs already are independent modules with their own deps), so
it costs nothing new architecturally: the interpreter/REPL binary (`cljgo`
itself) takes `purego` as a normal dependency — it needs it to support
`ffi/deflib` at the prompt regardless — while a compiled program's `go.mod`
only gains `purego` if that specific program actually calls `ffi/deflib`.
This keeps "a plain Clojure program with no FFI" building with zero
third-party deps in its OWN module (option 2's goal) without inventing a
new conditional-import mechanism (option 2's cost) — the mechanism
(separate go.mod per emitted program) already exists.

## 8. `ffi/deflib` surface recommendation

Confirmed unchanged from S7/design/05's sketch, with the dynamic-vs-static
registration strategy now settled (§3 above) and the type-keyword table
extended by this spike's `Kind`/`goType` (S21) union with S7's original
table:

```clojure
(ffi/deflib libm "libm.dylib"                  ; resolved per-OS by the runtime
  (cos  "cos"  [:double] :double)
  (sqrt "sqrt" [:double] :double))

(libm/cos 0.0)                                  ; => 1.0, REPL-live, no rebuild

;; wrong arity, wrong signature, missing symbol/lib -> see §4's table
;; variadic C fns -> rejected at expansion, per ADR 0011
```

No changes to the `[v err]`/`:rc`/`:ptr!out`/`:callback` conventions S7
already settled — this spike only adds the registration-strategy answer
(dynamic when needed, static when comptime-knowable) and the dependency
placement answer (§7) that S7 left open.
