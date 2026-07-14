;;; cljgo.el --- .cljg filetype for Emacs -*- lexical-binding: t; -*-

;; cljgo adds zero new syntax (ADR 0017), so clojure-mode handles .cljg
;; unchanged. Drop this in your init.el (or load this file):

(add-to-list 'auto-mode-alist '("\\.cljg\\'" . clojure-mode))

;; Emacs 29+ tree-sitter users (clojure-ts-mode) instead:
;;   (add-to-list 'auto-mode-alist '("\\.cljg\\'" . clojure-ts-mode))

;; Optional: teach CIDER that .cljg is Clojure so `cider-jack-in` /
;; `cider-connect` (to a future `cljgo repl` nREPL, ADR 0017 §4) treat the
;; buffer as a Clojure source buffer — clojure-mode derivation already
;; covers this; nothing else is required.

;; Optional niceties: indent the cljgo builtin forms like their Clojure
;; analogues.
(with-eval-after-load 'clojure-mode
  (put-clojure-indent 'comptime 0)
  (put-clojure-indent 'comptime-assert 1)
  (put-clojure-indent 'let? 1)
  (put-clojure-indent 'ffi/deflib 2))

(provide 'cljgo)
;;; cljgo.el ends here
