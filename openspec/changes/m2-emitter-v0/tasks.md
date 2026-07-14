# m2-emitter-v0 — tasks

Every task ends with the gates: `go build ./... && go vet ./... &&
gofmt -l pkg cmd conformance && go test ./...` green.

- [x] 1. `pkg/emit` core: munging (`munge.go` + `MUNGING.md`), Generator
  (flattener over all current ops: Const/Vector/Map/Set/Var/Local/Do/If/
  Def/Let/Fn/FnMethod/Invoke/Quote/Loop/Recur/TheVar/SetBang/DynBind),
  hoisted interns, S5 recur/capture machinery, format.Source gate.
- [x] 2. Program assembly + build driver: `Load()`/`main()` emission,
  go.mod generation (replace → runtime dir resolution), `go build` runner,
  compile-time evaluation driver (`CompileFile`).
- [x] 3. `pkg/emit` tests: per-op compile-and-run coverage (S1/S5 case
  ports: factorial, loop 100k, closure-over-loop-local 0 1 2, loop-binding
  init capture, swap rebinding, shadowing, fn-level recur), plus a
  factorial benchmark vs handwritten Go (M2 ~10× budget, ADR 0004).
- [x] 4. `cmd/cljgo build` verb + `examples/hello/core.clj`; measure
  startup < 50 ms.
- [x] 5. Conformance dual harness: compiled runner (byte-identical vs
  eval output), `;; harness: eval` markers with reasons on files that
  need them; README update.
- [x] 6. `ORACLE=1` mode against the real `clojure` CLI (1.12.5), with
  `;; oracle: skip` support; run it once and record results.
- [x] 7. Full gates + M2 exit demo (`cljgo build examples/hello/core.clj
  && ./hello`, factorial conformance file passing BOTH harnesses).
