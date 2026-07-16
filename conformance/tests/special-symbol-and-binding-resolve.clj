;; special-symbol? (design/08 batch E, ADR 0022): a static membership
;; check against the JVM's Compiler/specials, independent of which of
;; these cljgo's own analyzer implements. The `binding` resolve-vs-
;; special-form fix (ADR 0022 batch E, ratified 2026-07-16): cljgo's
;; analyzer treats `binding` as special (for its push/pop-thread-bindings
;; machinery), but the JVM does not — special-symbol? must say false and
;; resolve must still find a Var, without changing (binding ...)'s
;; evaluation path.
;; oracle (clojure 1.12.5): (special-symbol? 'quote) => true;
;; (special-symbol? 'binding) => false; (special-symbol? 'a-symbol) =>
;; false; (special-symbol? "not a symbol") => false;
;; (resolve 'binding) => a Var (not nil).
[[(special-symbol? 'quote)
  (special-symbol? 'def)
  (special-symbol? 'case*)
  (special-symbol? '&)
  (special-symbol? 'binding)
  (special-symbol? 'a-symbol)
  (special-symbol? "not a symbol")
  (var? (resolve 'binding))
  (binding [*out* *out*] :still-works)]]
;; expect: [[true true true true false false false true :still-works]]
