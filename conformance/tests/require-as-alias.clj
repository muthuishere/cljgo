;; require with a libspec vector honors :as — it creates a namespace alias
;; in the current ns, so the alias-qualified call resolves through it.
;; harness: eval — require/:as are REPL/load-time namespace state
;; oracle: skip — alias state is environmental (verified vs clojure CLI
;;   1.12, 2026-07-15: (require '[clojure.string :as str]) (str/join "-"
;;   [1 2 3]) => "1-2-3")
(require '[clojure.string :as str])
(str/join "-" [1 2 3])
;; expect: "1-2-3"
