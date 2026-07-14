;; require with :refer interns the named public vars into the current ns,
;; so the unqualified name resolves.
;; harness: eval — require/:refer are REPL/load-time namespace state
;; oracle: skip — refer state is environmental (verified vs clojure CLI
;;   1.12, 2026-07-15: (require '[clojure.string :refer [upper-case]])
;;   (upper-case "hi") => "HI")
(require '[clojure.string :refer [upper-case]])
(upper-case "hi")
;; expect: "HI"
