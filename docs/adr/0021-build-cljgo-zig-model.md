# ADR 0021 — `build.cljgo`: a Zig-style build system, not native `deps.edn`
Date: 2026-07-15 · Status: proposed (owner-directed 2026-07-15; design → OpenSpec)

## Context
Priority #1 is universal Go/C interop; the remaining piece is *third-party*
modules (design/05 M2). The obvious path was a Clojure-style `deps.edn`
manifest + a `go get`/regen/self-rebuild flow (Glojure's model). Owner decision
(2026-07-15): **we do NOT want `deps.edn` natively.** Instead cljgo adopts
**Zig's build model** — a `build.zig` analog written in cljgo itself: build is
*code that constructs a build graph*, not a static dependency manifest. This
also composes with the Zig-inspired `comptime` of ADR 0009 (build.cljgo is
itself evaluated at compile time and may run comptime codegen), and it is the
mechanism by which Go modules, C libraries (ADR 0011), library artifacts
(ADR 0013), and tests (ADR 0012) are all declared.

## Decision
1. **`build.cljgo` at the project root is a cljgo program** defining a `build`
   fn `(defn build [b] ...)`. `cljgo build` (no file arg) evaluates it with a
   builder `b`, producing a build graph (a DAG of steps), then executes the
   requested step(s). `cljgo build <file.clj>` (ADR 0001 form) stays as the
   fast single-file path; `build.cljgo` is the project path.
2. **Builder API mirrors `std.Build`** (names are cljgo-idiomatic, settle in
   design): artifacts `(exe b {...})` / `(lib b {:kind :go-archive|:c-shared|
   :c-archive})` (ADR 0013); dependency declaration on an artifact —
   `(go-require art "github.com/gorilla/websocket" "v1.5.3")` (**this replaces
   `deps.edn` for Go deps** — the build fetches via `go get` and pins the
   emitted `go.mod`), `(c-link art {:pkg-config "sqlite3"})` (cgo, ADR 0011),
   `(ffi art {...})` (purego); steps `(test-step b {:paths [...] :both true})`
   (ADR 0012), `(install b art)`, `(run b art)`; configuration
   `(option b :name :type :default)` (Zig `-D` build options),
   cross-compile `:target`/`:optimize` on artifacts.
3. **cljgo package deps** (other cljgo libs) are declared in `build.cljgo` too
   (`(dep b "name" {:git ... | :path ...})`) — a Zig `build.zig.zon` analog,
   but expressed in the same file. No separate resolver manifest is native.
4. **comptime is a first-class build citizen** (ADR 0009): a
   `(comptime-step b ...)` runs cljgo at build time to generate `.cljg`/`.go`
   or embed assets (`embed-file`) before emission; build.cljgo evaluation IS a
   comptime context, so the build description gets the full language.
5. **The Go toolchain is the backend, hidden.** Users never write `go.mod` by
   hand; the build graph emits it (module requires from `go-require`, C flags
   from `c-link`), runs `go mod tidy` + `go build`. `CGO_ENABLED=1` is set when
   any `c-link` is present (priority #5).

## Consequences
- Third-party Go modules ship via `build.cljgo`, not `deps.edn` — declarative
  where Zig is declarative, imperative where a build genuinely needs code
  (conditional deps, generated sources, matrix targets).
- One file describes the whole project: exe/lib artifacts, Go + C deps, FFI,
  tests, comptime codegen, install/run — the Zig ergonomic, on the Go backend.
- Supersedes design/05's M2 `deps.edn` interop path **for the AOT product**;
  the interpreted REPL still needs Go deps linked into a project-local binary
  (design/05 §1 self-rebuild) — that rebuild is now driven by `build.cljgo`'s
  `go-require` set, not a `deps.edn`. The self-exec mechanism is unchanged.
- Requires: a builder/graph runtime, a `build.cljgo` loader, go.mod synthesis
  from the graph, and step execution — scoped in design/08 + an OpenSpec change
  (`/opsx:propose build-system`). Bootstrapping note: `cljgo build` must load
  and run `build.cljgo` through the interpreter before any emission exists, so
  the builder API is interpreted-first (AOT of build.cljgo itself is later).
- Differentiator: no other Clojure has a Zig-style build system, and none makes
  the Go+C ecosystem reachable through one code-defined build graph.
