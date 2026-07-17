# Tasks — c-ffi

Sequenced T1 → T3 (ADR 0044 / S21). Every task ends with the gates
(`go build ./... && go vet ./... && gofmt -l pkg cmd conformance && go test
./...`) green; every semantic task adds conformance `.clj` files, comment-
citing either `spikes/s7-purego-ffi/RESULTS.md` or `spikes/s21-c-ffi-purego/
VERDICT.md` for the purego-mechanical behaviors (there is no JVM oracle for
FFI forms).

## 1. T1 — the type-keyword table and the interpreter path

- [ ] 1.1 **`pkg/host` type-keyword table**: the union of S7's and S21's
  `Kind`s → concrete Go/reflect types, one function each direction
  (keyword→reflect.Type for dynamic registration, keyword→Go source type
  name for emission). Unit tests per keyword. Gates green.
- [ ] 1.2 **`ffi/deflib` interpreted form**: reader/analyzer special-form
  recognition; eval-time expansion calls a `Declare`-shaped function
  (S21's `deflib.go` is the reference implementation) — dlopen, then per
  decl: reject variadic (named error citing ADR 0011), dlsym, build
  `reflect.FuncOf`/`reflect.New`/`purego.RegisterFunc`, bind the result as
  a Clojure fn value on a new namespace's vars. Conformance: successful
  declare+call (no-arg, scalar-arg, pointer/buffer-arg per S21 §1);
  missing-lib, missing-symbol, wrong-arity each as a named, positioned
  error (not a raw panic); variadic declaration rejected at expansion.
  Gates green.
- [ ] 1.3 **REPL-liveness conformance**: re-declare the same `ffi/deflib`
  form (or a var) to a different C symbol in one running REPL session,
  assert the second call observes the new binding with no restart (S21
  `live.go`'s claim, now via the real interpreter path, not the spike's
  simulation). Gates green.
- [ ] 1.4 **`ffi/fn`**: one-off dynamic binding, same underlying mechanism
  as 1.2 minus the namespace-of-many-fns wrapper. Conformance: declare and
  call inline. Gates green.
- [ ] 1.5 **`ffi/callback`**: `purego.NewCallback` wrapper, cached by fn
  identity (S7's ≤2000-callbacks-per-process ceiling documented, cache
  prevents re-minting on every call). Conformance: a C callback into a
  Clojure fn (reuse S7's sqlite3_exec pattern or an equivalent qsort-style
  libc callback). Gates green.

## 2. T2 — the AOT path

- [ ] 2.1 **Static `ffi/deflib` emission**: analyzer emits, per declared
  fn, a package-level `var lib_fnname func(ArgTypes...) RetType` plus a
  `purego.RegisterLibFunc` call in the namespace's Go package `init()`
  (per `emit-sketch.go.txt`) whenever the declaration is comptime-visible.
  Conformance: dual-harness (REPL + AOT) parity test — same `ffi/deflib`
  form, same conformance `.clj` file, both harnesses assert the identical
  output (ADR 0007's non-negotiable, now covering FFI).
- [ ] 2.2 **`cmd/cljgo`'s `go.mod` gains `purego`** as an ordinary
  dependency (documented as the first non-tooling dependency, per ADR
  0044 §2); a compiled program's generated `go.mod` gains `purego` ONLY
  when its source uses `ffi/` (build/emit step checks for `ffi/deflib`
  usage before adding the require). Conformance: an FFI-free program's
  emitted `go.mod` has no purego entry; an FFI-using program's does.
  Gates green.

## 3. T3 — documentation and disclosure

- [ ] 3.1 **Wrong-signature limitation documented**: `ffi/deflib` reference
  doc states plainly that a declared type mismatched against the real C
  signature is undetectable (ABI register-class corruption, not an
  error), with S21's `cos`-declared-as-`:int32` example as the worked
  case, and the "verify against a known-good call first" convention.
  No code task — a docs gate.
- [ ] 3.2 **Platform matrix documented**: darwin/linux/windows ×
  amd64/arm64 as fully supported; everything else best-effort, citing
  purego's own README tiers (ADR 0044 §4). No code task — a docs gate.
- [ ] 3.3 **Perf disclosure**: publish S21's benchmark table (pure Go
  11.8ns, cgo 16.3ns, purego static 132ns, purego dynamic 221ns per call,
  darwin/arm64) in the `ffi/deflib` doc as a baseline, explicitly NOT a
  CI-gated budget (ADR 0024 gating deferred — see Non-goals). Gates green.

## 4. Wrap-up

- [ ] 4.1 design/05 §1.2/§C-FFI updated to cite ADR 0044 + S21's numbers;
  S7 and S21 spikes remain frozen. `openspec archive c-ffi` after owner
  sign-off. Gates green.
