# Tasks — m3-interop-v0

## 1. Contract (DONE — git 0f29825)
- [x] `OpHostRef`/`OpHostCall` + payload nodes in pkg/ast.
- [x] `ResolveHost` analyzer hook; wire analyzeSymbol (value) + parseInvoke
      (call, `!` strip) to emit the ops. Precedence: Clojure first.
- [x] Green stubs: eval `evalHost`, emit `genHost`. Gates green.

## 2. Interpreted path (pkg/eval)
- [ ] `require-go` builtin + per-ns host-alias table (symbol/string paths,
      `:as`, default alias = last path segment).
- [ ] `resolveHost` wired into the analyzer; precedence enforced.
- [ ] reflect seed registry: strings/strconv/math/fmt members + consts.
- [ ] `evalHost`: OpHostRef (fn-as-value/const), OpHostCall with the shaping
      table, arg coercion, nil normalization, int64/float64 widening, `!` throw.
- [ ] pkg/eval/host_test.go covering each shaping row. Gates green.

## 3. AOT path (pkg/emit)
- [ ] Port spikes/s2 `facts` (go/packages signature resolution) into pkg/emit.
- [ ] `genHost`: emit `import` + direct call + shaping into `any`; rt helpers
      (`NormErr`, nil-norm, widening) in pkg/emit/rt.
- [ ] go.mod pinning path (stdlib = no require); golang.org/x/tools dep added.
- [ ] emit unit test: compile+run a snippet, assert shaping. Gates green.

## 4. Conformance + close-out
- [ ] conformance/tests/interop-*.clj (single-return, int coercion, `[v err]`
      happy+error, `!` happy+throw, const read, precedence) with frozen
      `;; expect:` cited against the Go stdlib oracle.
- [ ] Full gates incl. dual-harness (eval + compiled) byte-identical.
- [ ] Update memory; archive this change when M3-v0 ships.

Every task ends with: `go build ./... && go vet ./... && gofmt -l pkg cmd
conformance && go test ./...` green.
