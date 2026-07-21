;; clojure.core/read-string (fundamentals audit 2026-07): the GENERAL
;; reader (distinct from clojure.edn/read-string) — reads ONE form, resolves
;; ::auto keywords against *ns*, throws on bare EOF unless the opts arity
;; supplies :eof. cljgo note: #= eval-on-read does not exist in cljgo's
;; reader (it throws), the safe subset of the JVM's *read-eval* default.
;; oracle (clojure 1.12.5, 2026-07-21):
;;   (read-string "(+ 1 2)") => (+ 1 2)
;;   (read-string "{:a 1} ignored") => {:a 1}
;;   (read-string "::a") => :user/a  (in ns user)
;;   (read-string "\"hi\"") => "hi"
;;   (double? (read-string "1.5")) => true
;;   (try (read-string "") (catch Exception e (ex-message e))) => "EOF while reading"
;;   (read-string {:eof :done} "") => :done
[(read-string "(+ 1 2)")
 (read-string "{:a 1} ignored")
 (read-string "::a")
 (read-string "\"hi\"")
 (double? (read-string "1.5"))
 (try (read-string "") (catch Exception e (ex-message e)))
 (read-string {:eof :done} "")]
;; expect: [(+ 1 2) {:a 1} :user/a "hi" true "EOF while reading" :done]
