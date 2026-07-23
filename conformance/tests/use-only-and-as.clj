;; use with refer filters: :only restricts what gets referred, :as still
;; adds the alias (so other publics stay reachable qualified), and a name
;; outside :only is NOT referred.
;; oracle (Clojure 1.12.5 CLI, 2026-07-23, fresh REPL):
;; (use '[clojure.string :only [upper-case] :as ss])
;; [(upper-case "ab") (ss/lower-case "AB") (nil? (resolve 'blank?))]
;; => ["AB" "ab" true]
(use '[clojure.string :only [upper-case] :as ss])
[(upper-case "ab") (ss/lower-case "AB") (nil? (resolve 'blank?))]
;; expect: ["AB" "ab" true]
