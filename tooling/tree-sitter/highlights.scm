;; cljgo — extra highlight queries layered ON TOP of tree-sitter-clojure's
;; standard Clojure highlights (ADR 0017 §2: adopt, don't fork).
;;
;; cljgo introduces ZERO new syntax (precedence principle) — every cljgo
;; addition is an ordinary form, so these queries only re-color well-known
;; head symbols; the grammar is stock tree-sitter-clojure.
;;
;; Portability: only #eq? / #match? / negated fields are used — supported by
;; nvim-treesitter, Helix, and Zed alike. Capture names follow the modern
;; nvim-treesitter set (@function.builtin, @keyword, @function.method,
;; @string.regexp), which Helix and Zed themes also resolve.

;; ---------------------------------------------------------------------------
;; Compile-time value forms (ADR 0009): (comptime …), (comptime-assert …),
;; (embed-file "path")
;; ---------------------------------------------------------------------------
(list_lit
  .
  (sym_lit
    !namespace
    name: (sym_name) @function.builtin
    (#match? @function.builtin "^(comptime|comptime-assert|embed-file)$")))

;; ---------------------------------------------------------------------------
;; Binding / require forms: (let? […] …) railway binding (ADR 0014),
;; (require-go …) at the REPL (ADR 0010)
;; ---------------------------------------------------------------------------
(list_lit
  .
  (sym_lit
    !namespace
    name: (sym_name) @keyword
    (#match? @keyword "^(let\\?|require-go)$")))

;; :require-go clause inside (ns …) (ADR 0010 §1)
(kwd_lit
  name: (kwd_name) @keyword
  (#eq? @keyword "require-go"))

;; ---------------------------------------------------------------------------
;; FFI forms (ADR 0011): (ffi/deflib lib "libname" (fn …) …)
;; ---------------------------------------------------------------------------
(list_lit
  .
  (sym_lit
    namespace: (sym_ns) @_ffi_ns
    name: (sym_name) @function.builtin
    (#eq? @_ffi_ns "ffi")
    (#match? @function.builtin "^(deflib)$")))

;; ---------------------------------------------------------------------------
;; Go interop (ADR 0010)
;; ---------------------------------------------------------------------------

;; go/ reserved pseudo-namespace operators: go/new, go/slice-of, go/instantiate…
(sym_lit
  namespace: (sym_ns) @_go_ns
  name: (sym_name) @function.method
  (#eq? @_go_ns "go"))

;; Member access / method call dot forms: (.Do client req), field (.-Timeout c)
((sym_lit
   !namespace
   name: (sym_name) @function.method)
  (#match? @function.method "^\\.-?[A-Za-z_]"))

;; Constructor forms: (http/Client. {…}), (Buffer. n) — trailing dot
((sym_lit
   name: (sym_name) @function.method)
  (#match? @function.method "^[A-Za-z_].*\\.$"))

;; ---------------------------------------------------------------------------
;; #"" regex literals are RE2 in cljgo (not java.util.regex)
;; ---------------------------------------------------------------------------
(regex_lit) @string.regexp
