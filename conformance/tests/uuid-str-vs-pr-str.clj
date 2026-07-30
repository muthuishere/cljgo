;; UUID: `str` is bare, printing is tagged (ADR 0110 ask 2). java.util.UUID's
;; toString has no reader tag, so (str u) / (format "%s" u) / string
;; concatenation give the bare 36-char form — this is what goes on a wire as a
;; JSON-RPC id or a correlation id. print / pr / prn / print-method all emit
;; the round-trippable #uuid tag, readably or not. cljgo used to give the
;; tagged form for BOTH (its UUID stringer was the tagged text), silently
;; corrupting every id built with `str`.
;; oracle (Clojure 1.12.5, `clojure -M`, 2026-07-30):
;;   (count (str (random-uuid)))                  => 36
;;   (count (pr-str (random-uuid)))               => 44
;;   (str #uuid "550e8400-e29b-41d4-a716-446655440000")
;;     => "550e8400-e29b-41d4-a716-446655440000"
;;   (pr-str #uuid "550e8400-...")  => "#uuid \"550e8400-...\""
;;   (with-out-str (print #uuid "550e8400-..."))  => "#uuid \"550e8400-...\""
;;     (print keeps the tag on the JVM — only str is bare)
;;   (str "id=" #uuid "550e8400-...") => "id=550e8400-e29b-41d4-a716-446655440000"
;;   (= u (clojure.edn/read-string (pr-str u)))  => true   ; round-trip
(require '[clojure.edn :as edn])
(def u #uuid "550e8400-e29b-41d4-a716-446655440000")
[(str u)
 (count (str u))
 (pr-str u)
 (count (pr-str u))
 (with-out-str (print u))
 (with-out-str (pr u))
 (str "id=" u)
 (count (str (random-uuid)))
 (count (pr-str (random-uuid)))
 (= u (edn/read-string (pr-str u)))
 (uuid? (edn/read-string (pr-str u)))]
;; expect: ["550e8400-e29b-41d4-a716-446655440000" 36 "#uuid \"550e8400-e29b-41d4-a716-446655440000\"" 44 "#uuid \"550e8400-e29b-41d4-a716-446655440000\"" "#uuid \"550e8400-e29b-41d4-a716-446655440000\"" "id=550e8400-e29b-41d4-a716-446655440000" 36 44 true true]
