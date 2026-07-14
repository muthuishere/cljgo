;; cljgo — injection queries (layer on top of tree-sitter-clojure).
;;
;; #"" literals compile to RE2 in cljgo; inject the regex grammar so the
;; pattern body gets its own highlighting where a regex parser is installed.
;;
;; NOTE: regex_lit has no separate content child — the capture spans the whole
;; literal including the #"…" delimiters. nvim-treesitter honors #offset! to
;; trim them; editors that don't understand #offset! (Helix, Zed) simply
;; inject over the full span, which is still correct enough visually.

((regex_lit) @injection.content
  (#offset! @injection.content 0 2 0 -1)
  (#set! injection.language "regex"))

;; (comment …) blocks stay Clojure — no injection needed (grammar parses them).
