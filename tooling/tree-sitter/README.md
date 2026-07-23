# cljgo tree-sitter queries pack

Per ADR 0017 §2: **adopt, don't fork**. cljgo introduces zero new syntax
(precedence principle, CLAUDE.md), so stock
[tree-sitter-clojure](https://github.com/sogaiu/tree-sitter-clojure) parses
cljgo files unchanged. This directory ships only *extra queries* layered on
top of the standard Clojure ones:

| file | adds |
|---|---|
| `highlights.scm` | `comptime` / `comptime-assert` / `embed-file` / `ffi/deflib` as `@function.builtin`; Result/Option constructors + predicates (`ok` / `err` / `just` / `unwrap` / `ok?` / `err?` / `just?` / `none?` / `result?` / `option?`) as `@function.builtin` and `none` as `@constant.builtin` (ADR 0014); `let?` / `require-go` / `:require-go` / `defroute` / `defroutes` as `@keyword`; core.async — `go` / `go*` / `go-loop` / `thread` / `alt!` / `alt!!` as `@keyword` and the channel, buffer, pipeline, mult/mix/pub families (`<!` / `>!` / `alts!` / `chan` / `timeout` / …, plus `async/`-qualified use) as `@function.builtin` (ADR 0040); bri HTTP methods (`GET` / `POST` / `PUT` / `DELETE` / `PATCH` / `HEAD` / `OPTIONS` / `ANY`) as `@function.builtin` (ADR 0069); reader-conditional selectors `:cljgo` / `:default` as `@keyword` (ADR 0036/0050); `go/` pseudo-ns operators, `.member` / `.-field` / `Ctor.` interop as `@function.method`; `#""` (RE2 in cljgo) as `@string.regexp` |
| `injections.scm` | regex-language injection into `#""` literals |
| `locals.scm` | `let?` treated as a binding scope like `let` |
| `examples/*.cljg` | sample files exercising every query (verified ERROR-free against tree-sitter-clojure `e43eff8`, tree-sitter CLI 0.26) |

Queries use only `#eq?` / `#match?` / negated fields, so they load in
nvim-treesitter, Helix, and Zed without edits.

> **Ahead of the implementation:** `comptime` / `comptime-assert` /
> `embed-file` (ADR 0009) and `ffi/deflib` (ADR 0011/0044) are highlighted but
> not yet implemented — they are still open `openspec/changes/` proposals.
> Highlighting them early is harmless (they are ordinary symbols today), but
> don't read this pack as a statement of what ships.

## File extensions

cljgo loads four extensions (`pkg/eval/libload.go`, ADR 0055 / 0068), all of
them stock Clojure as far as the grammar is concerned:

| ext | note |
|---|---|
| `.cljgo` | preferred long form |
| `.cljg` | short form, used throughout this repo |
| `.clj` | plain Clojure source |
| `.cljc` | cross-platform; cljgo's reader feature is `:cljgo`, never `:clj` |

Map **all four** in your editor — the setup snippets below do. `.clj` and
`.cljc` already map to Clojure everywhere, so in practice only `.cljgo` and
`.cljg` need adding.

## Neovim (nvim-treesitter)

1. Map the filetype (the grammar and base queries are the stock `clojure` ones):

   ```lua
   vim.filetype.add({ extension = { cljg = "clojure", cljgo = "clojure" } })
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
file-types = ["clj", "cljs", "cljc", "cljd", "edn", "boot", "bb", "cljg", "cljgo"]
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
  { "file_types": { "Clojure": ["cljg", "cljgo"] } }
  ```

- Proper: a dev extension that declares `path_suffixes = ["cljg", "cljgo"]` for the
  clojure grammar and vendors these queries under `languages/clojure/`
  (Zed extensions replace rather than extend queries — concatenate stock +
  this pack).

## VS Code

See `../editors/vscode/` — VS Code uses TextMate scopes, not tree-sitter, so
the extension there maps the Clojure grammar instead. Its overlay is kept in
step with `highlights.scm`, and it claims `.cljgo` / `.cljg` (see
`../editors/REGISTRATION.md` for why `.clj` / `.cljc` are deliberately not
claimed).

## Verifying changes to this pack

The grammar is pinned at `e43eff8`, which is still upstream `main`.

```sh
git clone https://github.com/sogaiu/tree-sitter-clojure /tmp/tsclj
cd /tmp/tsclj && git checkout e43eff8 && tree-sitter build

# 1. every example must parse with zero ERROR nodes
for f in <repo>/tooling/tree-sitter/examples/*.cljg; do
  tree-sitter parse "$f" | grep -q ERROR && echo "ERROR: $f"
done

# 2. every query file must load, and capture what it claims
tree-sitter query --captures <repo>/tooling/tree-sitter/highlights.scm \
                             <repo>/tooling/tree-sitter/examples/routes.cljg
```

Run `tree-sitter build` from inside the clone (CLI ≥ 0.22; 0.26.10 used for
the last verification) so the parser resolves without a configured
`parser-directories`.

When adding a query, add or extend an `examples/*.cljg` file that exercises
it and confirm it appears in the `--captures` output — a query that loads is
not the same as a query that matches. The selecting (`#?`) and splicing
(`#?@`) reader conditionals, for instance, are *different* grammar nodes
(`read_cond_lit` / `splicing_read_cond_lit`) and each needs its own pattern.
