;; cljgo — locals queries (layer on top of tree-sitter-clojure).
;;
;; (let? [name expr …] body) scopes its bindings exactly like let
;; (ADR 0014 §3: railway-style binding form). Same known imperfection as
;; upstream Clojure locals queries: symbols in *value* position of the
;; binding vector are also captured — acceptable for highlighting-grade
;; locals resolution.
;;
;; Helix note: Helix uses @local.definition (no .var suffix); duplicate the
;; second pattern with that capture name if you vendor this file for Helix.

((list_lit
   .
   (sym_lit !namespace name: (sym_name) @_let)
   .
   (vec_lit)) @local.scope
  (#match? @_let "^let\\?$"))

(list_lit
  .
  (sym_lit !namespace name: (sym_name) @_let)
  .
  (vec_lit
    (sym_lit name: (sym_name) @local.definition.var))
  (#match? @_let "^let\\?$"))

((sym_lit name: (sym_name) @local.reference))
