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
;; Result / Option track (ADR 0014). Per the precedence principle these are
;; NEW names chosen to avoid shadowing clojure.core (`some` stays Clojure's,
;; hence `just`/`none`). Constructors, predicates and the unwrapper are
;; ordinary core vars (pkg/corelib/builtins.go).
;; ---------------------------------------------------------------------------
(list_lit
  .
  (sym_lit
    !namespace
    name: (sym_name) @function.builtin
    (#match? @function.builtin "^(ok|err|just|unwrap|ok\\?|err\\?|just\\?|none\\?|result\\?|option\\?)$")))

;; `none` is a value, not a call, so it is matched as a bare symbol. Caveat:
;; a local binding named `none` is captured too — acceptable, and shadowing a
;; core var is discouraged by the precedence principle anyway.
((sym_lit
   !namespace
   name: (sym_name) @constant.builtin)
  (#eq? @constant.builtin "none"))

;; ---------------------------------------------------------------------------
;; Reader conditionals (ADR 0036 / 0050). cljgo is its OWN platform: its
;; reader feature is `:cljgo`, never `:clj` (pkg/reader/readcond.go) —
;; `:default` is the always-matching fallback. Highlighting the branch
;; selectors makes it obvious which arm cljgo takes in a `.cljc` file.
;; ---------------------------------------------------------------------------
;; Selecting form #?(…) and the splicing form #?@(…) are separate nodes in
;; tree-sitter-clojure, so both need a pattern.
(read_cond_lit
  (kwd_lit
    name: (kwd_name) @keyword
    (#match? @keyword "^(cljgo|default)$")))

(splicing_read_cond_lit
  (kwd_lit
    name: (kwd_name) @keyword
    (#match? @keyword "^(cljgo|default)$")))

;; ---------------------------------------------------------------------------
;; core.async on Go channels (ADR 0040). `go` bodies are real goroutines (no
;; IOC transform), and the M4-v0 surface is refer'd into clojure.core, so
;; these read as builtins in ordinary code.
;;
;; `go` / `go-loop` and the alt macros are control flow -> @keyword.
;; ---------------------------------------------------------------------------
(list_lit
  .
  (sym_lit
    !namespace
    name: (sym_name) @keyword
    (#match? @keyword "^(go|go\\*|go-loop|thread|alt!|alt!!)$")))

;; Channel ops, buffers, and the pipeline/mult/mix/pub families.
;;
;; DELIBERATELY EXCLUDED: map, merge, reduce, take, into, transduce. Those
;; are clojure.core.async names that shadow clojure.core, and the stock
;; Clojure queries already color them — matching here would fight upstream.
(list_lit
  .
  (sym_lit
    !namespace
    name: (sym_name) @function.builtin
    (#match? @function.builtin "^(<!|<!!|>!|>!!|alts!|alts!!|put!|take!|poll!|offer!|close!|chan|timeout|thread-call|promise-chan|buffer|dropping-buffer|sliding-buffer|unblocking-buffer\\?|pipe|pipeline|pipeline-async|pipeline-blocking|split|mult|tap|untap|untap-all|mix|admix|unmix|unmix-all|solo-mode|toggle|pub|sub|unsub|unsub-all|to-chan|to-chan!|to-chan!!|onto-chan|onto-chan!|onto-chan!!)$")))

;; Namespaced use of the same surface: (async/<! c), (a/chan 8), (async/go …)
(sym_lit
  namespace: (sym_ns) @_async_ns
  name: (sym_name) @function.builtin
  (#match? @_async_ns "^(async|a|clojure\\.core\\.async)$"))

;; ---------------------------------------------------------------------------
;; bri routing (ADR 0069): Compojure-style routes. `defroute`/`defroutes` are
;; macros; the all-caps method names are ordinary fns in head position
;; (core/bri/http.cljg).
;; ---------------------------------------------------------------------------
(list_lit
  .
  (sym_lit
    !namespace
    name: (sym_name) @keyword
    (#match? @keyword "^defroutes?$")))

(list_lit
  .
  (sym_lit
    name: (sym_name) @function.builtin
    (#match? @function.builtin "^(GET|POST|PUT|DELETE|PATCH|HEAD|OPTIONS|ANY)$")))

;; ---------------------------------------------------------------------------
;; #"" regex literals are RE2 in cljgo (not java.util.regex)
;; ---------------------------------------------------------------------------
(regex_lit) @string.regexp
