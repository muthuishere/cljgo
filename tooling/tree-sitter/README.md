# cljgo tree-sitter queries pack

Per ADR 0017 §2: **adopt, don't fork**. cljgo introduces zero new syntax
(precedence principle, CLAUDE.md), so stock
[tree-sitter-clojure](https://github.com/sogaiu/tree-sitter-clojure) parses
`.cljg` files unchanged. This directory ships only *extra queries* layered on
top of the standard Clojure ones:

| file | adds |
|---|---|
| `highlights.scm` | `comptime` / `comptime-assert` / `embed-file` / `ffi/deflib` as `@function.builtin`; `let?` / `require-go` / `:require-go` as `@keyword`; `go/` pseudo-ns operators, `.member` / `.-field` / `Ctor.` interop as `@function.method`; `#""` (RE2 in cljgo) as `@string.regexp` |
| `injections.scm` | regex-language injection into `#""` literals |
| `locals.scm` | `let?` treated as a binding scope like `let` |
| `examples/*.cljg` | sample files exercising every query (verified ERROR-free against tree-sitter-clojure `e43eff8`, tree-sitter CLI 0.26) |

Queries use only `#eq?` / `#match?` / negated fields, so they load in
nvim-treesitter, Helix, and Zed without edits.

## Neovim (nvim-treesitter)

1. Map the filetype (the grammar and base queries are the stock `clojure` ones):

   ```lua
   vim.filetype.add({ extension = { cljg = "clojure" } })
   ```

2. Layer these queries via `after/queries` in your config
   (`~/.config/nvim/after/queries/clojure/`). Copy or symlink each file and
   keep the extension semantics additive by starting the file with:

   ```scheme
   ;; extends
   ```

   e.g. `after/queries/clojure/highlights.scm` = `;; extends` + the contents
   of `highlights.scm` here. Same for `injections.scm` and `locals.scm`.

3. `:checkhealth nvim-treesitter` should show clojure installed; open an
   `examples/*.cljg` file and `:InspectTree` / `:Inspect` to confirm captures.

## Helix

In `~/.config/helix/languages.toml`, extend the built-in clojure entry:

```toml
[[language]]
name = "clojure"
file-types = ["clj", "cljs", "cljc", "cljd", "edn", "boot", "bb", "cljg"]
```

Helix loads queries from `runtime/queries/clojure/`. To layer these, copy the
stock clojure queries into `~/.config/helix/runtime/queries/clojure/` and
append this pack's patterns (Helix uses first-match-wins, so put the cljgo
patterns *before* the generic symbol patterns). In `locals.scm`, rename
`@local.definition.var` → `@local.definition` for Helix.

## Zed

Zed's Clojure support is the `zed-clojure` extension (same grammar). Two
options:

- Quick: per-project file association in `.zed/settings.json`:

  ```json
  { "file_types": { "Clojure": ["cljg"] } }
  ```

- Proper: a dev extension that declares `path_suffixes = ["cljg"]` for the
  clojure grammar and vendors these queries under `languages/clojure/`
  (Zed extensions replace rather than extend queries — concatenate stock +
  this pack).

## VS Code

See `../editors/vscode/` — VS Code uses TextMate scopes, not tree-sitter, so
the extension there maps `.cljg` to the Clojure grammar instead.

## Verifying changes to this pack

```sh
git clone https://github.com/sogaiu/tree-sitter-clojure /tmp/tsclj
cd /tmp/tsclj
tree-sitter parse  <repo>/tooling/tree-sitter/examples/*.cljg   # expect zero ERROR nodes
tree-sitter query  <repo>/tooling/tree-sitter/highlights.scm \
                   <repo>/tooling/tree-sitter/examples/interop.cljg
```

(Requires the tree-sitter CLI ≥ 0.22 with a configured parser directory:
`tree-sitter init-config`, then add the clone's parent dir to
`parser-directories`.)
