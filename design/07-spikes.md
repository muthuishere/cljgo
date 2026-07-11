# 07 — Spikes: riskiest assumptions to prove before/while building

Each spike is a throwaway prototype (days, not weeks) that validates a bet the
architecture makes. Ordered by how much of the design collapses if the bet is
wrong. Spike code lives in `spikes/<id>-<slug>/`, never in `pkg/`.

Context for all of them: we emit **real `.go` source files** (text →
`go/format.Source` → `go build`). Go has no stable public IR — the gc
compiler's SSA is internal and changes every release — so `.go` source *is*
the intermediate representation, exactly as JS is for ClojureScript. This is
settled (doc 04), not a spike; several spikes below exist to prove the
consequences of it.

## S1 — Emitter thin slice (validates docs 03+04 contract end-to-end)
Hand-construct the AST for `(def fact (fn* [n] (if (< n 2) 1 (* n (fact (- n 1))))))`
+ `(println (fact 10))` (no reader/analyzer needed), run it through a minimal
flattening emitter → `.go` → `go build` → run.
**Proves:** statement flattening without IIFEs, `Load()` model, `lang.Fn` +
`switch len(args)` calling convention, `format.Source` gate, and that emitted
code links `pkg/lang` cleanly.
**Also measure:** `go build` latency on the generated module (dev-loop speed).
**Kills if wrong:** the whole emission strategy → would force rethink before M2.

## S2 — go/packages direct interop call (validates priority #1)
Given `(:require-go [github.com/gorilla/websocket :as ws])`-equivalent input,
load type facts via `golang.org/x/tools/go/packages`, resolve `ws.Dial`'s
signature, emit a direct call with arg coercion and `[v err]` shaping.
**Proves:** zero-binding third-party interop with non-reflective calls.
**Also measure:** go/packages load latency per package (known to be seconds
cold) → informs the signature-cache design in pkg/host.
**Kills if wrong:** the "Go toolchain is our classpath" thesis.

## S3 — REPL deps self-rebuild UX (validates interop in interpreted mode)
Prototype the flow: add module to deps file → `go get` → generate a
registry file (genpkg-style go/types walk) → rebuild project-local binary →
`syscall.Exec` self-replace, REPL state reloaded.
**Proves:** "add a dep, one command, import it live at the REPL."
**Also measure:** wall-clock for the whole cycle on a real module — if it's
>15s the UX story needs work (pre-warmed builds? incremental?).

## S4 — Vendor-prune Glojure pkg/lang (validates doc 02's ownership plan)
Copy `pkg/lang` in, delete interpreter glue (`builtins.go`, `class.go`,
reflect FnFuncs), drop `go4.org/intern`/`hashstructure`/`pcastools`, swap
keyword interning onto `unique.Handle`, compile standalone under go 1.26
with zero external deps, run its tests.
**Proves:** the vendored runtime is genuinely severable and ours.
**Also:** verify `unique.Handle` preserves the `k1 == k2` identity contract
including across separately-compiled emitted packages (§4.4).

## S5 — recur/loop emission edge cases (the flattening claim under stress)
Emit and run: nested `loop*`s with shadowing, closure capturing a loop
local (must not see later iterations' rebinding), `recur` under `when`/`and`
expansion, loop in non-tail expression position (loop-as-expression needs a
temp var), 100k-iteration constant-stack check.
**Proves:** goto/continue + temp-var simultaneous rebinding is correct where
cljs2go's IIFE approach would have been trivially correct.

## S6 — Var-indirection cost in emitted code
Benchmark fib/factorial three ways: raw Go, emitted-with-var-deref-per-call,
emitted-with-direct-link. Quantifies the price of REPL-liveness-by-default
(§4.2) and whether the opt-in performance ladder is a "later" or an "M3".

## S7 — purego FFI on darwin/arm64 + linux
`dlopen` libsqlite3, bind 3 functions via purego, call them — from a plain
Go program first, then sketch the `ffi/deflib` shape.
**Proves:** the C-ecosystem story works without cgo, including on macOS ARM.

## S8 — Syntax-quote conformance harness (cheap, high value)
Script: run N syntax-quote inputs through real JVM Clojure (`clojure -M -e`)
capturing expansion output, diff against our reader's expansion (once built).
Build the harness now, before the reader — it becomes reader CI.
**Proves:** the trickiest reader feature is testable against ground truth,
not our intuition. Same harness pattern later drives the dual-mode
conformance suite.

## S9 — Upstream core.clj reuse census
Parse `clojure-master/src/clj/clojure/core.clj` (7600+ lines), classify top-level
forms: (a) loads as-is with our special forms, (b) needs host-interop rewrite
(Java calls), (c) JVM-only (unreachable). Gives a real number for "reuse
Clojure as it is" — how much of core we port vs inherit, informing M1/M5
scoping. (Glojure's own patched core.clj is the comparison baseline.)

## S10 — Dynamic alts! mechanism
Static `alt!` → Go `select` is clean; *dynamic* `alts!` (runtime channel
list) needs `reflect.Select`. Prototype it, measure overhead, confirm
semantics (default/timeout/priority) match core.async's contract.

## Sequencing vs roadmap
S1, S4, S8 before starting M0 (they de-risk the foundations).
S2, S3 before committing M3's design. S5, S6 alongside M2. S7, S10 before
their post-M3 milestones. S9 anytime — it's analysis, not code.
