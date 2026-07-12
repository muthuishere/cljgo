# ADR 0001 — Emit Go source files, not an intermediate representation
Date: 2026-07-11 · Status: accepted

## Context
Go has no stable public IR: the gc compiler's SSA is internal and changes per
release; there is no JVM-bytecode equivalent. Alternatives: build go/ast in
memory (still must print to source), or target LLVM (abandons the Go toolchain
and the interop thesis).

## Decision
The compiler AOT-emits plain `.go` source text, validated by go/format.Source,
built by the standard `go build`. `.go` IS the intermediate form, as JS is for
ClojureScript. Emission is text via fmt, not go/ast construction (what both
working precedents — Glojure gen, cljs2go — do).

## Consequences
Output is debuggable (delve), readable, gopls-compatible; cross-compilation
and third-party imports come free. Spike S1 validated end-to-end (65ms warm
rebuild, 2.3ms binary startup).
