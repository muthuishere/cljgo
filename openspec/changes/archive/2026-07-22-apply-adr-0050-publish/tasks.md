## 1. Go-interop taint classifier — `pkg/emit/purity.go` (decision 3, from S29)

- [x] 1.1 `pkg/emit/purity.go`: `Taint` + `ClassifyGoInterop` — one pass over `Program.Entry`+`Deps`, walking `CompiledNS.Forms` via the real `eachChild`, switching on the five host ops, first offending `file:line` per namespace
- [x] 1.2 Entry namespace name (`""`) recovered textually (`readNSName`)
- [x] 1.3 Pluggable `Predicate` slot — used for a **second** shipping predicate `RequireGoPredicate` (a bare `(require-go …)` with no member access is taint too; ADR 0050 dec 2/3 name require-go itself as disqualifying — adversarial-review fix)
- [x] 1.4 `WholeLibPure` (OR/first-offender) + `NamespacePure` (lookup); `whole-lib == AND(per-ns)`
- [x] 1.5 Tests: buried require-go caught at leaf `file:line`; pure fixture zero-FP; mixed; invariant; **bare-require-go tainted** (regression)

## 2. `certain-java?` courtesy predicate — `pkg/publish/java.go` (decision 2, from S30)

- [x] 2.1 `pkg/publish/java.go`: `CertainJava`/`CertainJavaFile` (reader-level) — `import`/`new` heads, `java.*`/`javax.*`/`clojure.java.*` + bare-JVM-class table in call-ns position
- [x] 2.2 Zero-FP: never flags bare dot-forms, `instance?`, `catch`, class-ref values; never a gate
- [x] 2.3 Tests: S30 corpus — precision 10/10, zero FP

## 3. `publish clojars` producer + CLI + surface (decisions 1, 3)

- [x] 3.1 `pkg/publish/clojars.go`: compile → `ClassifyGoInterop` → `WholeLibPure`; refuse-before-write naming `file:line`; else copy every `CompiledNS.Path` into `src/<ns>.clj` + `deps.edn` git-coord + pure `cljgo.manifest.edn`. Java allowed
- [x] 3.2 `core/build.cljg` `(lib b spec)` verb (`:kind "lib"` + `:module`); AOT mirror regenerated via `go generate` (byte-identical, parity by construction); `Artifact.Module` + `Plan.LibArtifact`
- [x] 3.3 `cmd/cljgo/main.go` `case "publish"` → `runPublish` (`go`/`clojars`, `-o -name -module -runtime`, usage)
- [x] 3.4 Tests: pure → clojars ok; require-go buried → refused at `file:line`; **bare require-go → refused, no leaked tree** (regression); Java-flavored-but-Go-free → publishes

## 4. `publish go` producer (decisions 1, 2 of the target table)

- [x] 4.1 `pkg/publish/go.go` + `pkg/emit/library.go WriteLibrary`: library-shaped emission (per-ns `Load` packages, `wrappers.go`, `go.mod` with the lib module path, no `main()`); Go-interop allowed
- [x] 4.2 Scope reported honestly (see follow-ups): compiling go-gettable module for pure + stdlib-go-interop libs; exported wrappers exclude `^:private`/`-main`; `Load`-collision reserved (adversarial-review fix); `outDir` created before go/packages chdir (adversarial-review fix)
- [x] 4.3 Tests: pure → `go build ./...` passes; stdlib go-interop → builds; **missing-outDir go-interop → builds** (regression); **public `load` defn → module compiles** (regression)

## 5. Decision 4 — loud per-namespace Java failure (verify + strict hook)

- [x] 5.1 Static Java surfaces hard-error at analysis with `file:line`, never nil — pinned by conformance `java-static-loud-error` / `java-import-loud-error` (eval-only)
- [x] 5.2 Pure sibling of a Java-tainted namespace stays usable — `TestJavaStaticFailsLoudPerNamespace`
- [ ] 5.3 Optional strict resolve-time Java rejection hook in `pkg/deps` — **deferred** (touches lock schema + `checkPurity` across files; ADR marks it optional). The publish-side pure manifest emission the resolve side expects IS wired

## 6. Dual-harness parity + gates

- [x] 6.1 Conformance eval-only Java cases; `publish` output deterministic; classifier/gate covered by unit tests
- [x] 6.2 Full gates green: `go build ./... && go vet ./... && gofmt -l pkg cmd conformance templates && go test ./...` (compiled conformance harness 337s, independently re-run green after the adversarial-review fixes)

## 7. Close-out

- [x] 7.1 No spike code merged verbatim into `pkg/`; S29/S30 reference-only (ADR 0027)
- [x] 7.2 Update ADR 0050 status proposed → accepted (implemented); record ADR 0013 producer-side follow-ups
- [ ] 7.3 `/opsx:archive` this change

## Deferred follow-ups (tracked, not blocking)

- **Decision 5.3** strict resolve-time Java hook — own change (lock-schema surface).
- **`publish go` typed signatures** — wrappers are uniformly `func(args ...any) any`; resolving typed Go signatures from `^long`/hint metadata is deferred (no export dropped, only signatures widened).
- **`publish go` third-party go-require** — the `go get`/tidy wiring `pkg/build` does is not yet in `publish go`; a third-party require currently fails with a raw go/packages message rather than a purpose-built one.
- **`publish clojars` Clojars coordinate / source-jar** — git-coordinate `deps.edn` only (ADR 0050 scoping); a Clojars coordinate step is later.
- **Test nit** — `go_test.go` `^:private` exclusion is exercised only via a dependency ns; add an entry-ns private defn to test that branch directly.
