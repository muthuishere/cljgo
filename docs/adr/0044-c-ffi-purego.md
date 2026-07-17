# ADR 0044 — C FFI via purego: dynamic registration, static AOT, dependency placement
Date: 2026-07-17 · Status: proposed (owner reviews; evidence: spike S21)

## Context

ADR 0011 (spike S7) already decided purego dlopen is the primary FFI door
and cgo the ecosystem door, and sketched `ffi/deflib`'s surface against a
static Go prototype (compile-time-typed func vars bound with
`purego.RegisterLibFunc`). That prototype left two questions open, both
blocking: (1) the INTERPRETER path — `ffi/deflib` is read and evaluated at
runtime, so no Go source ever names the C function's signature; can purego's
registration mechanism be driven from a signature that exists only as
runtime data (Clojure type keywords)? (2) where does the `purego` Go module
dependency live, given the main `cljgo` module's zero-third-party-deps
posture (`go.mod` carries only a build-time tool today)?

Spike S21 (`spikes/s21-c-ffi-purego/`) answers both. It proves dynamic
registration via `reflect.FuncOf` + `reflect.New` + `purego.RegisterFunc`
(no compile-time func type anywhere in the call path), measures its
overhead against the static form and against cgo/pure-Go baselines, sketches
the AOT emission for the identical declaration, and evaluates the three
dependency-placement options against ADR 0028's existing "runtime as
published module" shape.

## Decision

1. **`ffi/deflib` uses TWO registration strategies, chosen by the analyzer,
   both purego underneath:**
   - **Static** (compile-time-typed func var + `purego.RegisterLibFunc`)
     whenever the declaration is comptime-visible — the normal case, since
     `ffi/deflib` is an ordinary top-level form the analyzer already walks
     (ADR 0009). This is what AOT emission always uses (see
     `emit-sketch.go.txt`): one `var` + one `RegisterLibFunc` call per
     declared fn, in a package `init()`.
   - **Dynamic** (`reflect.FuncOf` + `reflect.New` + `purego.RegisterFunc`,
     called through `reflect.Value.Call` with arity/type checks BEFORE the
     call, per S21's `deflib.go`) for the interpreter, and for any
     genuinely runtime-constructed declaration (e.g. an `ffi/fn` one-off
     built from a string at the REPL).
   Both strategies bind to the SAME three purego primitives (`Dlopen`/
   `Dlsym`/`RegisterLibFunc`/`RegisterFunc`) — one ABI-marshaling story,
   tested once (S7 + S21), trusted in both modes (design/00 §5).
2. **Dependency placement: the generated-module approach (S21 VERDICT §7,
   option 3).** `cljgo` itself (the REPL/interpreter binary) takes `purego`
   as an ordinary dependency — it needs `ffi/deflib` to work at the prompt
   regardless of what any given program does. A COMPILED program's own
   `go.mod` (already independent per ADR 0028) gains `purego` only when
   that program actually uses `ffi/`; a plain Clojure program with no FFI
   still builds with zero third-party deps in its own module. No new
   conditional-import machinery is invented — the per-program module
   already exists.
3. **Failure honesty, unchanged from S7, reaffirmed at the dynamic-path
   layer:** missing library, missing symbol, and wrong arity are named,
   positioned errors at declaration or call time — never a raw panic,
   never a partially-registered `Lib`. Variadic C declarations are rejected
   at deflib expansion time (ADR 0011 already decided this; S21 confirms
   the rejection composes with dynamic registration). **Wrong C signature**
   (right symbol, mismatched declared types) is NOT and cannot be caught —
   it is an ABI register-class mismatch, invisible to any marshaling layer
   including cgo's. `ffi/deflib`'s docs carry this as a named limitation
   with a "verify against a known-good call" convention; a future
   `ffi/from-header` (parsing real C headers) is the only structural fix,
   and is explicitly out of this ADR's scope.
4. **Platform claim: full support on darwin/linux/windows × amd64/arm64**
   (purego's own Tier 1, matching cljgo's existing release-binary targets)
   — everything else (BSDs, 386/arm/riscv64/etc., mobile) is "best effort,
   not conformance-tested," and struct-by-value C APIs are "wrap it in
   Go/cgo" outside the amd64/arm64 desktop trio, consistent with design/05's
   existing position.
5. **Performance is disclosed, not gated at this stage:** measured on
   darwin/arm64 (Apple M5 Pro), chained `cos(x)` calls: pure Go 11.8ns,
   cgo 16.3ns, purego static 132ns, purego dynamic 221ns. purego's own
   trampoline overhead (~100-130ns) dominates both registration strategies;
   the dynamic path adds ~90ns of `reflect.Call` marshaling on top. None of
   this is disqualifying for "call a C library function" — design/05
   already tells performance-critical C to go through a cgo-wrapped Go
   package instead of live FFI, and this ADR does not change that guidance.
   A perf budget (ADR 0024) for `ffi/deflib` calls is deferred to the
   implementing OpenSpec change's own tasks, once real conformance tests
   exist to budget against.

## Consequences

- The interpreter/eval side (`pkg/eval`) gains the dynamic-registration
  mechanism from S21's `deflib.go` as its reference implementation; the
  analyzer/emitter side gains the static form from `emit-sketch.go.txt`.
  Both are new surface area post-M3, tracked by `openspec/changes/c-ffi/`.
- `cljgo`'s own `go.mod` gains its first non-tooling third-party dependency
  (`github.com/ebitengine/purego`) the moment `ffi/deflib` lands — a
  deliberate, scoped exception to the zero-deps posture, justified by §2's
  reasoning (the interpreter needs it; emitted programs don't unless they
  ask for it).
- Type-keyword vocabulary (`:string :int32/:int64 :float32/:float64 :ptr
  :ptr!out :cstr!out :callback :rc :void ...`) is the union of S7's and
  S21's tables; owned by `pkg/host`, the single place a keyword becomes a
  concrete Go/reflect type in either registration strategy.
- Not chosen: vendoring purego unconditionally into every emitted binary
  (option 1 — pays the dep tax even for FFI-free programs); a bespoke
  conditional-import build step (option 2 — solves the same problem ADR
  0028's per-program module already solves, at extra complexity).
- Wrong-signature corruption stays a documented risk, not a solved problem;
  reviewers should not expect this ADR to close it.
