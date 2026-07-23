# cljgo VS Code extension (skeleton)

Minimal filetype extension per ADR 0017 §5: contributes the `cljgo` language
id for `.cljgo` / `.cljg`, with a one-include TextMate grammar
(`source.cljgo` → `source.clojure`) plus a thin overlay that colors the cljgo
builtin forms. Everything else *is* Clojure — precedence principle.

The overlay is kept in step with `../../tree-sitter/highlights.scm` and covers:

| ADR | forms |
|---|---|
| 0009 / 0011 | `comptime`, `comptime-assert`, `embed-file`, `ffi/deflib` *(ahead of implementation — still open openspec proposals)* |
| 0010 | `require-go`, `go/*` pseudo-namespace |
| 0014 | `let?`; `ok` / `err` / `just` / `unwrap` + predicates; `none` |
| 0036 / 0050 | `:cljgo` / `:default` reader-conditional selectors |
| 0040 | `go` / `go*` / `go-loop` / `thread` / `alt!` / `alt!!` and the channel, buffer, pipeline, mult/mix/pub surface |
| 0069 | `defroute` / `defroutes` and the HTTP methods |

`map` / `merge` / `reduce` / `take` / `into` / `transduce` are **deliberately
excluded** — they are `clojure.core.async` names that shadow `clojure.core`,
and the Clojure grammar already colors them.

## File extensions

cljgo loads four (`pkg/eval/libload.go`, ADR 0055/0068): `.cljgo` `.cljg`
`.clj` `.cljc`. This extension claims **only `.cljgo` and `.cljg`**.

`.clj` and `.cljc` are deliberately *not* claimed: they already map to the
`clojure` language id, and re-claiming them here would take plain Clojure
files away from Calva and the built-in Clojure grammar. Open a `.clj` file in
a cljgo project and it behaves exactly as it always has.

**Not published.** This is a local skeleton; the real extension (adding an
LSP client that launches `cljgo lsp`) lands with M3.

## Try it locally

```sh
cd tooling/editors/vscode
code --install-extension .   # or: symlink into ~/.vscode/extensions/cljgo-0.0.1
```

Simplest dev loop: open this folder in VS Code and hit `F5`
(Run Extension) — no packaging needed. To package: `npx @vscode/vsce package`
(do **not** publish).

Prerequisite for highlighting: a Clojure extension that provides the
`source.clojure` TextMate grammar (Calva or the built-in `clojure` extension
ship one; VS Code includes Clojure syntax out of the box).

## Zero-install alternative

Without any extension, users can add to `settings.json`:

```json
{
  "files.associations": {
    "*.cljgo": "clojure",
    "*.cljg": "clojure"
  }
}
```

That loses the distinct `cljgo` language id (needed later for LSP routing)
but gets highlighting today.

## Files

- `package.json` — language + grammar contributions
- `syntaxes/cljgo.tmLanguage.json` — cljgo overlay, then `include source.clojure`
- `language-configuration.json` — lisp brackets/comments/word pattern
- `icons/` — placeholders (see `icons/PLACEHOLDER.md`)
