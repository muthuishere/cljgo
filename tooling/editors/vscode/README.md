# cljgo VS Code extension (skeleton)

Minimal filetype extension per ADR 0017 §5: contributes the `cljgo` language
id for `.cljg`, with a one-include TextMate grammar (`source.cljgo` →
`source.clojure`) plus a thin overlay that colors the cljgo builtin forms
(`comptime`, `comptime-assert`, `embed-file`, `ffi/deflib`, `let?`,
`require-go`, `go/*`). Everything else *is* Clojure — precedence principle.

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
{ "files.associations": { "*.cljg": "clojure" } }
```

That loses the distinct `cljgo` language id (needed later for LSP routing)
but gets highlighting today.

## Files

- `package.json` — language + grammar contributions
- `syntaxes/cljgo.tmLanguage.json` — cljgo overlay, then `include source.clojure`
- `language-configuration.json` — lisp brackets/comments/word pattern
- `icons/` — placeholders (see `icons/PLACEHOLDER.md`)
