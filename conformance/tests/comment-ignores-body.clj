;; comment (design/08 batch E, ADR 0022): a real clojure.core macro (not
;; special — (special-symbol? 'comment) is false), always nil, body
;; never evaluated (so it can hold anything, including a throwing form).
;; oracle (clojure 1.12.5): confirmed interactively via `clojure -e`.
[(comment)
 (comment 1)
 (comment (throw (ex-info "should never run" {})))
 (special-symbol? 'comment)]
;; expect: [nil nil nil false]
