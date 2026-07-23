# cljgo upstream registration checklist (ADR 0017 §2)

Order matters: Linguist first (other projects cite it as the authority),
then the grammar repo, then editor file-type maps. Track each PR here.

## Which extensions to register

cljgo loads four (`pkg/eval/libload.go`, ADR 0055/0068):

| ext | register upstream? |
|---|---|
| `.cljgo` | **yes** — the preferred long form |
| `.cljg` | **yes** — the short form used throughout this repo |
| `.clj` | no — already Clojure everywhere |
| `.cljc` | no — already Clojure everywhere |

Every PR below adds **both** `.cljgo` and `.cljg`. Never claim `.clj` or
`.cljc` for a cljgo-specific language id: they already resolve to Clojure,
and re-claiming them takes plain Clojure files away from the tooling that
owns them (Calva, clojure-mode, clojure-lsp).

## 1. GitHub Linguist — `.cljgo` / `.cljg` as Clojure extensions

- [ ] PR to [github-linguist/linguist](https://github.com/github-linguist/linguist)
- Edit `lib/linguist/languages.yml`, Clojure entry — add `".cljgo"` and
  `".cljg"` to `extensions` (keep the list sorted):

  ```yaml
  Clojure:
    type: programming
    ace_mode: clojure
    codemirror_mode: clojure
    codemirror_mime_type: text/x-clojure
    extensions:
    - ".clj"
    - ".boot"
    - ".cl2"
    - ".cljc"
    - ".cljg"        # <- new: cljgo (Clojure hosted on Go)
    - ".cljgo"       # <- new: cljgo, long form
    - ".cljs"
    - ".cljs.hl"
    - ".cljscm"
    - ".cljx"
    - ".hic"
    ...
  ```

- Linguist requirements: add `samples/Clojure/` samples for both (use
  `tooling/tree-sitter/examples/interop.cljg` and `async.cljg` — real-world
  shaped), and show
  **in-the-wild usage** (Linguist wants ~200 unique repos or clear evidence
  of adoption for a *new language*; for a new *extension on an existing
  language* the bar is lower but they still ask for public usage — land
  cljgo's own repos/examples on GitHub first, link them in the PR).
- Run `script/add-sample` + `bundle exec rake test` per their CONTRIBUTING.

## 2. tree-sitter-clojure — file-types

- [ ] PR to [sogaiu/tree-sitter-clojure](https://github.com/sogaiu/tree-sitter-clojure)
- One-line change in `package.json` (grammar metadata; upstream still uses the
  old layout as of `e43eff8` — if a `tree-sitter.json` has appeared, edit its
  `grammars[0].file-types` instead):

  ```diff
   "tree-sitter": [
     {
       "scope": "source.clojure",
       "file-types": [
         "bb",
         "clj",
         "cljc",
+        "cljg",
+        "cljgo",
         "cljs"
       ]
     }
   ]
  ```

- PR text: cite ADR 0017's guarantee (cljgo adds zero new syntax; the grammar
  parses cljgo sources unchanged — precedence principle) and attach a parse run of
  `tooling/tree-sitter/examples/` showing zero ERROR nodes. Note `sogaiu` is
  conservative about scope — this is metadata-only, no grammar change.

## 3. Editor file-type maps

- [ ] **Neovim** — PR to [neovim/neovim](https://github.com/neovim/neovim)
  `runtime/lua/vim/filetype.lua`: add `cljg = "clojure"` and
  `cljgo = "clojure"` in the extension table (near `clj`/`cljs`/`cljc`).
  Until merged, users use `vim.filetype.add` (see
  `../tree-sitter/README.md`).
- [ ] **Helix** — PR to [helix-editor/helix](https://github.com/helix-editor/helix)
  `languages.toml`: add `"cljg"` and `"cljgo"` to the clojure `file-types`
  list.
- [ ] **Zed** — PR to the clojure extension repo
  ([zed-extensions/clojure](https://github.com/zed-extensions/clojure)):
  add `"cljg"` and `"cljgo"` to `path_suffixes` in `extension.toml` /
  language config.
- [ ] **Emacs clojure-mode** — PR to
  [clojure-emacs/clojure-mode](https://github.com/clojure-emacs/clojure-mode):
  `auto-mode-alist` entries for `\.cljgo\'` and `\.cljg\'` (and the same
  for `clojure-ts-mode`). Note `\.cljg\'` does **not** match `.cljgo` —
  `\'` anchors end-of-string — so both entries are required. Shipped
  locally in `emacs/cljgo.el`.
- [ ] **Vim (classic)** — `runtime/filetype.vim` clojure pattern, via the
  vim/vim runtime update flow.

## 4. Follow-ups tied to milestones (not yet)

- [ ] clojure-lsp docs: note that after Linguist/filetype registration it
  works statically on `.cljgo` / `.cljg` (ADR 0017 §3) — file an issue only if their
  source-detection needs the extension whitelisted.
- [ ] VS Code Marketplace publish of `vscode/` — blocked on M3 (`cljgo lsp`).
- [ ] nvim-treesitter queries upstreaming (optional): the cljgo patterns in
  `../tree-sitter/highlights.scm` are cljgo-specific; they stay in this repo
  and in dotfiles `after/queries`, not upstream.

## Sequencing note

Do **not** open the Linguist PR until public cljgo code exists (the
examples/ + core/ rename to `.cljg` lands with M2 per ADR 0017). Grammar and
editor PRs can go earlier — they are metadata-only and uncontroversial.

## Local packs (already shipping, no upstream needed)

- `vscode/` — claims `.cljgo` / `.cljg` for the `cljgo` language id.
- `emacs/cljgo.el` — `auto-mode-alist` for both, plus indent rules.
- `../tree-sitter/` — the queries pack, with per-editor setup snippets.
