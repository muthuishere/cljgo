# Tasks — m3-interop-v0

## 1. Contract (DONE — git 0f29825)
- [x] `OpHostRef`/`OpHostCall` + payload nodes in pkg/ast.
- [x] `ResolveHost` analyzer hook; wire analyzeSymbol (value) + parseInvoke
      (call, `!` strip) to emit the ops. Precedence: Clojure first.
- [x] Green stubs: eval `evalHost`, emit `genHost`. Gates green.

## 2. Interpreted path (pkg/eval) — DONE (git 538dce6)
- [x] `require-go` builtin + per-ns host-alias table (symbol/string paths,
      `:as`, default alias = last path segment).
- [x] `resolveHost` wired into the analyzer; precedence enforced (verified).
- [x] reflect seed registry: strings/strconv/math/fmt members + consts.
- [x] `evalHost`: OpHostRef (fn-as-value/const), OpHostCall with the shaping
      table, arg coercion, nil normalization, int64/float64 widening, `!` throw.
- [x] pkg/eval/host_test.go — 12 tests covering each shaping row. Gates green.

## 3. AOT path (pkg/emit) — DONE (git 3c4c2b1)
- [x] Ported spikes/s2 `facts` into pkg/emit/hostfacts.go (go/packages, batched
      one Load per build, ~43ms warm, only when interop is referenced).
- [x] `genHost`: `import` + direct call + shaping into `any`; rt helpers
      (`NormErr`, `GoError`, `NilNorm`, `ToFloat64`) in pkg/emit/rt.
- [x] go.mod: golang.org/x/tools added; stdlib imports need no require.
- [x] TestInteropCompiled + TestInteropEmittedShape (asserts no reflect).

## 4. Conformance + close-out — DONE (git 71184a3)
- [x] conformance/tests/interop-{strings,numbers,verr,bang-throw}.clj, frozen
      `;; expect:`, oracle-skip (Go stdlib is the oracle).
- [x] Full gates incl. dual-harness (eval + compiled) byte-identical — the
      three dual-mode files PASS both harnesses; example diff identical.
- [x] Memory updated; change archived on M3-v0 ship.

Every task ends with: `go build ./... && go vet ./... && gofmt -l pkg cmd
conformance && go test ./...` green.
