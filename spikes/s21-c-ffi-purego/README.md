# S21 — C FFI via purego: the `ffi/deflib` surface, for real

Owner mandate (design/05 §C-FFI, priority-5): "any ffi can be imported and
used directly", live at the REPL, no C toolchain. S7 (docs/adr/0011) already
proved purego can call real C (sqlite3) from a **static**, compile-time-typed
Go program. That is not the same claim cljgo needs: the interpreter reads
`(ffi/deflib sqlite "libsqlite3" (version [] :string) ...)` at eval time —
there is no Go source declaring `func() string` anywhere. This spike answers
whether purego's registration mechanism (`RegisterFunc`/`RegisterLibFunc`,
which take a pointer to a *compile-time* Go func variable) can be driven from
a **runtime-constructed** signature, which is the actual hard problem behind
"REPL-live FFI in a dynamically-typed host".

This spike is design-with-prototypes (ADR 0027): the riskiest technical claim
(dynamic registration) is demonstrated in code; the rest (AOT sketch,
platform matrix) is documented from purego's own guarantees plus S7's
findings, not re-proven where S7 already proved it.

## The one question

Can `ffi/deflib`'s declaration form become a purego binding **without a
Go func type existing in source**, so the interpreter can dlopen/register/call
a C function purely from Clojure-level type keywords — and does the AOT path
still get to use the cheaper static form for the same declaration?

## Exit criteria (written before any code)

1. **Dynamic registration demonstrated**: build a `reflect.Type` for a C
   signature from type keywords (`:string :int32 :float64 :ptr ...`),
   `reflect.New` a func of that type, hand it to `purego.RegisterFunc`, and
   call it via `reflect.Value.Call` — no compile-time Go func signature
   anywhere in the call path. Cover: no-arg→scalar, scalar-arg→scalar,
   pointer/buffer-arg.
2. **REPL-liveness demonstrated**: register a binding, call it, re-declare
   the SAME Clojure name to a DIFFERENT C symbol in the same running
   process, call it again — prove the second call answers differently with
   no restart.
3. **AOT path sketched** (not executed — no compiler integration in this
   spike): show what `cljgo build` would emit for the identical
   `ffi/deflib` form, and argue why it can use the cheaper static path.
4. **Failure honesty measured**: missing lib, missing symbol, wrong arity —
   must fail at declaration/call time with a positioned, named error, never
   silently. Wrong SIGNATURE (right symbol, wrong types) must be shown to
   NOT be catchable this way — document it as an unfixable-by-us class of
   bug, matching S7's finding.
5. **Platform matrix documented**: purego's own supported OS/arch tiers,
   cited from its README, translated into what cljgo can claim to support.
6. **Per-call overhead measured**: benchmark the same libm `cos(x)` call via
   (a) pure Go stdlib, (b) purego static (S7-style), (c) purego dynamic
   (this spike's mechanism), (d) cgo — four numbers, one table.

## Run it

```
cd spikes/s21-c-ffi-purego
CGO_ENABLED=0 go build -o s21 . && ./s21   # demo (binary is gitignored, not committed)
CGO_ENABLED=0 go test -bench=. -benchtime=2s -run=^$ .          # purego benchmarks
cd cgobench && CGO_ENABLED=1 go test -bench=. -benchtime=2s -run=^$ .  # cgo benchmark
```

## Files

- `typemap.go` — the `Kind` vocabulary (deflib type keywords) → `reflect.Type`.
- `deflib.go` — `Declare()`/`BoundFn.Call()`: the dynamic-registration
  mechanism this spike exists to prove; every failure path is a named error,
  not a panic (except the truly unrecoverable "wrong C signature" case,
  which is architecturally uncatchable — see VERDICT.md).
- `main.go`, `live.go` — the demo: happy paths, failure paths, REPL-liveness.
- `emit-sketch.go.txt` — uncompiled sketch of the AOT emission for the same
  declaration.
- `bench_test.go` — pure-Go / purego-static / purego-dynamic benchmarks.
- `cgobench/` — separate module (own go.mod, needs `CGO_ENABLED=1`) with the
  cgo baseline for the same call, kept out of the main module so this
  spike's own build stays cgo-free by default.
- `VERDICT.md` — the verdict, filed after the code above ran.

See `docs/adr/0044-c-ffi-purego.md` for the decision this spike feeds and
`openspec/changes/c-ffi/` for the resulting proposal.
