# cljgo

Clojure hosted on Go: a compiler (written in Go) that AOT-emits plain Go
source — the ClojureScript model with Go as the JavaScript — plus a
tree-walk evaluator that is the REPL and the macro engine.

## Priorities

1. **Universal interop** — any Go module importable and callable with zero
   bindings; the Go ecosystem is the standard library. C via cgo modules and
   purego FFI.
2. **Full REPL-driven development** — live re-`def`, `defmacro` at the
   prompt, nREPL for CIDER/Calva.
3. **Faithful Clojure principles** — persistent data structures, macros,
   seqs, vars.
4. **High performance in both modes** — a feature, not an option.
5. **cgo builds are first-class** — `CGO_ENABLED=1` projects are supported,
   not tolerated.

## Status

Design phase complete; implementation starting. See `design/00-architecture.md`
for the consolidated architecture, contracts, and the M0–M5 roadmap, and
`design/07-spikes.md` for the de-risking spikes.

```
design/   architecture + component design docs (reader, data structures,
          analyzer/eval, Go emitter, interop/concurrency, spikes)
refs/     (gitignored) reference clones: glojure, cljs2go, let-go
```

Toolchain: Go 1.26.
