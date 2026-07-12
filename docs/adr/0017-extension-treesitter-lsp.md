# ADR 0017 — File extension .cljg; editor support via tree-sitter + LSP everywhere
Date: 2026-07-12 · Status: accepted (extension now; LSP/grammar via OpenSpec, staged M2→M5)

## Context
The Clojure family convention: .cljs (Script), .cljd (Dart), .cljc (common) —
platform dialects get their own extension so tooling and humans can tell
targets apart, while .cljc shares code across them. Editors need three
layers: syntax (tree-sitter), intelligence (LSP), interactivity (nREPL).
Crucial fact from the precedence principle (CLAUDE.md): cljgo introduces ZERO
new syntax — every addition (comptime, let?, ffi/deflib) is an ordinary form
— so existing Clojure grammars and structural tooling parse cljgo unchanged.

## Decision
1. **Extension: `.cljg`** (family pattern). Loader resolution order:
   .cljg → .cljc → .clj (a lib can ship all three; most specific wins).
   Reader-conditional feature key stays **`:cljgo`** (reads clearly:
   `#?(:cljgo (go/import …) :clj (import …))`); `.cljc` files with :cljgo
   branches are the cross-platform vehicle. cmd/cljgo accepts all three
   extensions everywhere today.
2. **Tree-sitter: adopt, don't fork.** tree-sitter-clojure parses cljgo
   as-is; we ship a `tooling/tree-sitter/` queries pack (highlights for
   comptime/ffi/interop forms as builtin-face, injections for #"" RE2) and
   upstream PRs registering .cljg with tree-sitter-clojure, GitHub Linguist
   (syntax: Clojure), and editors' file-type maps. No grammar of our own to
   maintain — the precedence principle guarantees this stays true.
3. **LSP: `cljgo lsp` as a thin adapter over what we already have** —
   ADR 0015's structured diagnostics/introspection (check → publishDiagnostics
   with fixes[] → codeActions; get_symbols → completion/hover/definition;
   get_ast → documentSymbol/selectionRange) plus the conformance-grade
   analyzer. Static-only mode works without a REPL; when a REPL session is
   live (ADR 0016 id), the LSP attaches for evaluated-state accuracy.
   clojure-lsp compatibility documented (it works statically on .cljg after
   Linguist/filetype registration; ours adds cljgo-specific smarts).
4. **nREPL (design/03 §7c, M5)** stays the interactive channel — CIDER/Calva
   connect to `cljgo repl` sessions; LSP and nREPL are complementary, both
   adapters over the same compiler internals (no parallel bookkeeping,
   ADR 0015).
5. Ship order: extension + filetype/queries pack (now, cheap) → structured
   diagnostics --json (M2, per ADR 0015) → `cljgo lsp` static core (M3) →
   nREPL (M5) → LSP live-attach (post-M5). A minimal VS Code extension
   (filetype, icon, LSP client pointing at `cljgo lsp`) lands with M3.

## Consequences
"Works across anywhere we give them": any tree-sitter editor (VS Code, nvim,
Helix, Zed) highlights .cljg from the queries pack + upstream registration;
any LSP editor gets diagnostics-with-fixes from the same registry the CLI
uses; CIDER/Calva users keep their muscle memory via nREPL. We own zero
grammars and one thin LSP adapter — the compiler's data model (ADR 0015) is
the single source for every editor feature. Rename note: examples/ and core/
migrate to .cljg as part of the M2 emitter change landing.
