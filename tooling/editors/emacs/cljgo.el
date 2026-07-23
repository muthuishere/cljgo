;;; cljgo.el --- cljgo filetypes for Emacs -*- lexical-binding: t; -*-

;; cljgo adds zero new syntax (ADR 0017), so clojure-mode handles cljgo
;; sources unchanged. Drop this in your init.el (or load this file):

;; cljgo loads four extensions (pkg/eval/libload.go, ADR 0055/0068):
;; .cljgo .cljg .clj .cljc. Only the first two need registering here —
;; clojure-mode already claims .clj and .cljc, and re-claiming them would
;; fight it for plain Clojure buffers.
(add-to-list 'auto-mode-alist '("\\.cljgo\\'" . clojure-mode))
(add-to-list 'auto-mode-alist '("\\.cljg\\'" . clojure-mode))

;; Emacs 29+ tree-sitter users (clojure-ts-mode) instead:
;;   (add-to-list 'auto-mode-alist '("\\.cljgo\\'" . clojure-ts-mode))
;;   (add-to-list 'auto-mode-alist '("\\.cljg\\'"  . clojure-ts-mode))

;; Optional: teach CIDER that cljgo files are Clojure so `cider-jack-in` /
;; `cider-connect` (to a future `cljgo repl` nREPL, ADR 0017 §4) treat the
;; buffer as a Clojure source buffer — clojure-mode derivation already
;; covers this; nothing else is required.

;; Optional niceties: indent the cljgo builtin forms like their Clojure
;; analogues. Kept in step with tooling/tree-sitter/highlights.scm.
(with-eval-after-load 'clojure-mode
  ;; Compile-time forms (ADR 0009) — not yet implemented, still an open
  ;; openspec proposal; harmless to indent early.
  (put-clojure-indent 'comptime 0)
  (put-clojure-indent 'comptime-assert 1)
  (put-clojure-indent 'ffi/deflib 2)

  ;; Railway binding (ADR 0014) — indents like `let`.
  (put-clojure-indent 'let? 1)

  ;; core.async on Go channels (ADR 0040). `go` bodies are real goroutines.
  ;; clojure-mode already indents go/go-loop/alt! when core.async is in
  ;; play, but cljgo refers this surface into clojure.core, so set it
  ;; unconditionally.
  (put-clojure-indent 'go 0)
  (put-clojure-indent 'go* 0)
  (put-clojure-indent 'go-loop 1)
  (put-clojure-indent 'thread 0)
  (put-clojure-indent 'alt! 0)
  (put-clojure-indent 'alt!! 0)

  ;; bri routing (ADR 0069) — Compojure-style, same indent as Compojure.
  (put-clojure-indent 'defroutes 'defun)
  (put-clojure-indent 'defroute 'defun)
  (put-clojure-indent 'context 2)
  (put-clojure-indent 'GET 2)
  (put-clojure-indent 'POST 2)
  (put-clojure-indent 'PUT 2)
  (put-clojure-indent 'DELETE 2)
  (put-clojure-indent 'PATCH 2)
  (put-clojure-indent 'HEAD 2)
  (put-clojure-indent 'OPTIONS 2)
  (put-clojure-indent 'ANY 2))

(provide 'cljgo)
;;; cljgo.el ends here
