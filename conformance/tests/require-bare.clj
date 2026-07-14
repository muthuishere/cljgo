;; require of a bare namespace symbol still works — it loads (asserts) the
;; embedded namespace without aliasing or referring; fully-qualified access
;; resolves.
;; harness: eval — require is REPL/load-time namespace state
;; oracle: skip — namespace-load state is environmental (verified vs clojure
;;   CLI 1.12, 2026-07-15: (require 'clojure.string) (clojure.string/blank?
;;   "") => true)
(require 'clojure.string)
(clojure.string/blank? "")
;; expect: true
