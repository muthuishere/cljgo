# .cljg upstream registration checklist (ADR 0017 §2)

Order matters: Linguist first (other projects cite it as the authority),
then the grammar repo, then editor file-type maps. Track each PR here.

## 1. GitHub Linguist — `.cljg` as a Clojure extension

- [ ] PR to [github-linguist/linguist](https://github.com/github-linguist/linguist)
- Edit `lib/linguist/languages.yml`, Clojure entry — add `".cljg"` to
  `extensions` (keep the list sorted):

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
    - ".cljs"
    - ".cljs.hl"
    - ".cljscm"
    - ".cljx"
    - ".hic"
    ...
  ```

- Linguist requirements: add `samples/Clojure/*.cljg` (use
  `tooling/tree-sitter/examples/interop.cljg` — real-world shaped), and show
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
         "cljs"
       ]
     }
   ]
  ```

- PR text: cite ADR 0017's guarantee (cljgo adds zero new syntax; the grammar
  parses `.cljg` unchanged — precedence principle) and attach a parse run of
  `tooling/tree-sitter/examples/` showing zero ERROR nodes. Note `sogaiu` is
  conservative about scope — this is metadata-only, no grammar change.

## 3. Editor file-type maps

- [ ] **Neovim** — PR to [neovim/neovim](https://github.com/neovim/neovim)
  `runtime/lua/vim/filetype.lua`: add `cljg = "clojure"` in the extension
  table (near `clj`/`cljs`/`cljc`). Until merged, users use
  `vim.filetype.add` (see `../tree-sitter/README.md`).
- [ ] **Helix** — PR to [helix-editor/helix](https://github.com/helix-editor/helix)
  `languages.toml`: add `"cljg"` to the clojure `file-types` list.
- [ ] **Zed** — PR to the clojure extension repo
  ([zed-extensions/clojure](https://github.com/zed-extensions/clojure)):
  add `"cljg"` to `path_suffixes` in `extension.toml` / language config.
- [ ] **Emacs clojure-mode** — PR to
  [clojure-emacs/clojure-mode](https://github.com/clojure-emacs/clojure-mode):
  `auto-mode-alist` entry for `\.cljg\'` (and same for `clojure-ts-mode`).
- [ ] **Vim (classic)** — `runtime/filetype.vim` clojure pattern, via the
  vim/vim runtime update flow.

## 4. Follow-ups tied to milestones (not yet)

- [ ] clojure-lsp docs: note that after Linguist/filetype registration it
  works statically on `.cljg` (ADR 0017 §3) — file an issue only if their
  source-detection needs the extension whitelisted.
- [ ] VS Code Marketplace publish of `vscode/` — blocked on M3 (`cljgo lsp`).
- [ ] nvim-treesitter queries upstreaming (optional): the cljgo patterns in
  `../tree-sitter/highlights.scm` are cljgo-specific; they stay in this repo
  and in dotfiles `after/queries`, not upstream.

## Sequencing note

Do **not** open the Linguist PR until public `.cljg` code exists (the
examples/ + core/ rename to `.cljg` lands with M2 per ADR 0017). Grammar and
editor PRs can go earlier — they are metadata-only and uncontroversial.
