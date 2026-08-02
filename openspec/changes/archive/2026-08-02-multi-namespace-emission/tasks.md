# Tasks — multi-namespace-emission

## 1. pkg/eval: require seams

- [x] 1.1 Thread `*Evaluator` through `requireSpec`/`loadLib`; add the
  lib-provider registry (`RegisterLibProvider`, mutex-guarded) and the
  requiring-file-relative resolver (`resolveLibPath`, ADR 0042 §4); default
  interpreter loader (read + `EvalForm` under a pushed `*ns*`/`*file*`
  frame, namespace-exists check after); compile/interp cycle detection with
  the Clojure-style error. `Evaluator.LibLoader` seam for the module
  compiler. Gates green.

## 2. pkg/emit: module compiler + multi-package emission

- [x] 2.1 `module.go`: `CompiledNS`, `Program`, `CompileProgram(path)` —
  capture loader, per-ns forms + require edges, dependency order;
  `CompileReader` installs the refusing loader. Gates green.
- [x] 2.2 Refactor `program.go` into a parameterized `emitPackage`
  (package name, ns registration, dep blank-imports, main-ness, shared
  host facts); `EmitMain` keeps signature AND byte-identical single-file
  output; `WriteProgram(dir, prog, opts)` delegates to `WriteModule` when
  there are no dependency namespaces. `rt.RegisterLib` re-export. Gates
  green.
- [x] 2.3 Wire `emit.Build` and `pkg/build.buildArtifact` through
  `CompileProgram`/`WriteProgram`. Gates green.

## 3. Prove it

- [x] 3.1 `pkg/emit/module_test.go`: 3-namespace program (entry requires
  multi.util requires multi.data; cross-ns var refs; a macro from
  multi.data used in the entry) compiled to a binary whose stdout is
  byte-identical to the interpreted run; cycle-detection test; single-file
  delegation test. Gates green.
- [x] 3.2 Conformance: switch `compiledOutput` to
  `CompileProgram`/`WriteProgram`; add ≥2 multi-ns tests (dep files under
  `tests/conf/`, outside the harness glob) with oracle-verified
  `;; expect:` — both harnesses green, suite baseline (~238/242) not
  regressed. Gates green.

## 4. Docs

- [x] 4.1 ADR 0042 committed; design/04 §1/§7 annotated with EDIT NOTEs
  pointing at ADR 0042 (registry-triggered loading supersedes the
  top-of-Load sketch). Gates green.
