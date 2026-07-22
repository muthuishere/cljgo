## 1. Go-interop taint classifier — `pkg/emit/purity.go` (decision 3, from S29)

- [ ] 1.1 New `pkg/emit/purity.go` in `package emit`: `type Taint struct { Class, NS, Path string; Line int; Detail string }` and `func ClassifyGoInterop(p *Program) map[string]*Taint` — one pass over `Program.Entry`+`Deps`, walking each `CompiledNS.Forms` via the existing `eachChild` (`emit.go:313`), switching on the five host ops `OpHostRef/OpHostCall/OpHostMethod/OpHostField/OpHostNew`, recording the first offending `file:line` per namespace
- [ ] 1.2 Recover the entry namespace's name textually (its `CompiledNS.Name` is `""`) so it is addressable in the taint map/gate output
- [ ] 1.3 Reserve a pluggable `type Predicate func(*CompiledNS) *Taint` slot (S29 proved N predicates compose) so `ffi`/`c-link` taint can be added without touching the traversal
- [ ] 1.4 Expose `WholeLibPure(map) (bool, *Taint)` (OR / first-offender) and `NamespacePure(map, ns) bool` (lookup); assert `whole-lib == AND(per-ns over all reachable)`
- [ ] 1.5 Tests (port S29 fixtures): buried `require-go` (`core→mid→leaf`) caught + cited at leaf `file:line` while pure ancestors pass per-ns; pure fixture zero false positives; mixed fixture; whole-lib == AND(per-ns)

## 2. `certain-java?` courtesy predicate — `pkg/publish` (decision 2, from S30)

- [ ] 2.1 New `pkg/publish` package: `func CertainJava(forms []any) []Diag` (reader-level, from S30 `javaSyntactic`) — flags `import`/`new` heads, `java.*`/`javax.*` + `clojure.java.*` in **call-namespace** position, and the bare-JVM-class table (`System Math Thread Integer …`) in call-ns position; each with `file:line`
- [ ] 2.2 Zero-FP guarantee: MUST NOT flag bare dot-forms `(.method obj)`, `(instance? String x)`, `(catch Exception e)`, or bare class-ref values; it is never a gate
- [ ] 2.3 Tests: port S30's 30-form corpus labels — precision must be 10/10 (zero FP); the accepted misses (bare dot-forms) stay unflagged

## 3. `publish clojars` producer + CLI + surface (decisions 1, 3)

- [ ] 3.1 `pkg/publish`: `PublishClojars(entrySrc, outDir, opts)` — `emit.CompileProgram` → `emit.ClassifyGoInterop` → if any NS tainted, FAIL naming the `file:line` ("uses Go interop, cannot run on the JVM"); else copy every `CompiledNS.Path` into a source-tree layout and write a git-coordinate `deps.edn` stub. Java is allowed (not a gate)
- [ ] 3.2 `core/build.cljg`: add a publish/lib target verb mirroring `(exe …)` that stamps `Artifact.Kind` (`"clojars"`/`"go"`); regenerate the AOT mirror `pkg/coreaot/cljgobuild/cljgobuild.go` via `go generate` (parity by construction); add minimal `Plan`/`Artifact` fields for a library module path
- [ ] 3.3 `cmd/cljgo/main.go`: `case "publish"` → `runPublish(args)` dispatching `go`/`clojars` (+ usage/`--help`)
- [ ] 3.4 Tests: a pure library publishes to clojars (source tree + deps.edn present); a `require-go` library is refused at the offending `file:line`; a Java-using-but-pure-of-Go library still publishes

## 4. `publish go` producer (decisions 1, 2 of the target table)

- [ ] 4.1 `pkg/publish`: `PublishGo(entrySrc, outDir, opts)` — `CompileProgram` → validate the exported surface is Go-expressible (reuse `hostfacts.go` signature resolution; fail `file:line` on an inexpressible export) → emit a go-gettable **library** package: per-namespace layout (from `WriteProgram`), library-shaped (named packages, exported Go wrappers for exported defns, a `go.mod` with the library module path, no `main()`). Go-interop is allowed
- [ ] 4.2 Scope honestly: support pure + go-interop libraries for the common export shapes `hostfacts.go` already resolves; `log()`/report which export shapes are deferred rather than silently dropping them
- [ ] 4.3 Tests: a pure library `publish go` produces a package that `go build`s; a go-interop library publishes; the emitted signatures match type hints (`any` where absent)

## 5. Decision 4 — loud per-namespace Java failure (verify + strict hook)

- [ ] 5.1 Verify the existing analyzer errors for static Java (`(System/…)`, `(Math/…)`, `import`, `new`, `java.*`, `clojure.java.*`) fire with `file:line` and never `nil` (S30 measured they do); add a conformance case pinning it
- [ ] 5.2 Pure siblings of a Java-tainted namespace stay usable (per-namespace granularity); cover with a test
- [ ] 5.3 Optional strict resolve-time rejection: a dependency manifest declaring Java taint is default-denied unless acknowledged (reuse the ADR 0048 `pkg/deps` `checkPurity` shape); wire the publish-time manifest emission the resolve side (`pkg/deps/manifest.go:6`) already expects

## 6. Dual-harness parity + gates

- [ ] 6.1 Conformance: the taint classifier and Java-failure cases run in the dual harness where applicable; `publish` output is deterministic
- [ ] 6.2 Full gates green: `go build ./... && go vet ./... && gofmt -l pkg cmd conformance templates && go test ./...`

## 7. Close-out

- [ ] 7.1 Verify no spike code merged verbatim into `pkg/`; S29/S30 stay reference-only (ADR 0027)
- [ ] 7.2 Update ADR 0050 status proposed → accepted (implemented); record ADR 0013 producer-side follow-ups (`c-shared`/`c-archive`) and the deferred import/Clojars-coordinate scoping
- [ ] 7.3 `/opsx:archive` this change
