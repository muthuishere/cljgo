# cljgo editor + tooling configuration

Everything an editor or an indexer needs to understand cljgo source, driven
from **one file**: [`definers.json`](definers.json).

## Why one file

Clojure is homoiconic — a definition has **no node type of its own**.
`(defn fetch-user [] …)` is an ordinary list whose *head symbol* (`defn`)
carries the meaning and whose *second element* (`fetch-user`) is the name.

So every tool that wants to find a definition needs the same list of head
symbols: editors to highlight and indent them, ctx-optimize to emit them as
graph nodes. Four hand-kept copies drift. `definers.json` is the source; a
generator writes the rest.

```
tooling/definers.json ──► tooling/tree-sitter/generated/definers.scm      nvim · Helix · Zed
                     ──► tooling/editors/emacs/generated/definers.el      Emacs
                     ──► tooling/editors/vscode/generated/definers.json   VS Code
                     ──► .ctxoptimize/grammars/cljgo.json                 ctx-optimize
```

## Add a defining form

Say your app defines `(defjob nightly-sync …)`. Add one entry to
`definers.json`:

```json
{ "head": "defjob", "kind": "function", "indent": "defun" }
```

`kind` is the ctx-optimize node kind — one of `function`, `variable`, `macro`,
`class`, `interface`, `module`, `test`. `indent` is the clojure-mode indent
spec (`defun` for anything shaped like `defn`).

Then regenerate:

```sh
go run ./tooling/gen-editors            # rewrite all four outputs
go run ./tooling/gen-editors -check     # CI: fail if any output is stale
```

Never edit a file under a `generated/` directory — the next run overwrites it.

---

## Editor setup

cljgo loads four extensions (`pkg/eval/libload.go`, ADR 0055/0068) and **adds
zero new syntax** (ADR 0017 §2), so every editor uses the *stock Clojure*
grammar. `.clj` and `.cljc` already map to Clojure everywhere; only `.cljgo`
and `.cljg` need adding.

### Neovim

```sh
mkdir -p ~/.config/nvim/after/queries/clojure
{ echo ';; extends'; cat tooling/tree-sitter/generated/definers.scm; } \
  > ~/.config/nvim/after/queries/clojure/highlights.scm
{ echo ';; extends'; cat tooling/tree-sitter/highlights.scm; } \
  >> ~/.config/nvim/after/queries/clojure/highlights.scm
for q in injections locals; do
  { echo ';; extends'; cat tooling/tree-sitter/$q.scm; } \
    > ~/.config/nvim/after/queries/clojure/$q.scm
done
```

Then in your config:

```lua
vim.filetype.add({ extension = { cljg = "clojure", cljgo = "clojure" } })
```

Verify: `:checkhealth nvim-treesitter` shows clojure installed, then open a
`.cljg` file and run `:InspectTree`.

### Emacs

```sh
mkdir -p ~/.emacs.d/lisp
cp tooling/editors/emacs/cljgo.el \
   tooling/editors/emacs/generated/definers.el ~/.emacs.d/lisp/
```

```elisp
(add-to-list 'load-path "~/.emacs.d/lisp")
(require 'cljgo)
(require 'definers)
(cljgo--apply-definer-indents)
(add-to-list 'auto-mode-alist '("\\.cljgo?\\'" . clojure-mode))
```

### VS Code

Install the bundled extension (the generated file is wired as a TextMate
injection in `package.json`):

```sh
cd tooling/editors/vscode
npx @vscode/vsce package
code --install-extension cljgo-0.0.1.vsix
```

Or, without the extension, just map the file types in `settings.json`:

```json
{ "files.associations": { "*.cljg": "clojure", "*.cljgo": "clojure" } }
```

VS Code uses TextMate scopes, not tree-sitter — see
[`editors/REGISTRATION.md`](editors/REGISTRATION.md) for why `.clj` / `.cljc`
are deliberately not claimed.

### Helix

```toml
# ~/.config/helix/languages.toml
[[language]]
name = "clojure"
file-types = ["clj", "cljs", "cljc", "cljd", "edn", "boot", "bb", "cljg", "cljgo"]
```

Helix replaces rather than extends queries — copy the stock clojure queries into
`~/.config/helix/runtime/queries/clojure/` and append this repo's, cljgo
patterns first. In `locals.scm`, rename `@local.definition.var` →
`@local.definition`.

### Zed

```json
// .zed/settings.json
{ "file_types": { "Clojure": ["cljg", "cljgo"] } }
```

---

## ctx-optimize

```sh
ctx-optimize languages add https://github.com/sogaiu/tree-sitter-clojure
cp ~/ctxoptimize/grammars/clojure.wasm .ctxoptimize/grammars/cljgo.wasm
go run ./tooling/gen-editors      # writes .ctxoptimize/grammars/cljgo.json
ctx-optimize up
ctx-optimize query "fetch-user"
```

The pack is **repo-local** (`.ctxoptimize/grammars/`), so cljgo's definers ship
with cljgo — no registry entry and no ctx-optimize release needed to add your
own. Commit `cljgo.json`; the `.wasm` is gitignored.

> **Requires `decl_rules` support in ctx-optimize.** A pack keyed on node types
> (`decls`) cannot express a homoiconic declaration; mapping `list_lit →
> function` yields nodes all named `defn` while the real names never appear.
> See [`ctx-optimize/README.md`](ctx-optimize/README.md).

## Verifying a change

```sh
go run ./tooling/gen-editors -check                    # outputs in sync
git clone https://github.com/sogaiu/tree-sitter-clojure /tmp/tsclj
cd /tmp/tsclj && git checkout e43eff8 && tree-sitter build && cd -

for f in tooling/tree-sitter/examples/*.cljg; do        # zero ERROR nodes
  (cd /tmp/tsclj && tree-sitter parse "$OLDPWD/$f") | grep -q ERROR && echo "ERROR: $f"
done

(cd /tmp/tsclj && tree-sitter query --captures \        # queries actually match
  "$OLDPWD/tooling/tree-sitter/generated/definers.scm" \
  "$OLDPWD/tooling/tree-sitter/examples/cli.cljg")
```

A query that *loads* is not a query that *matches* — when you add a definer,
add or extend an `examples/*.cljg` that exercises it and confirm it appears in
the `--captures` output.
