;; line-seq (fundamentals batch A1): lazy seq of lines from a reader —
;; clojure.core's exact shape (outer when-let, inner lazy-seq: nil at
;; immediate EOF, first line eager). cljgo's rdr is any Go io.Reader
;; (*in* qualifies directly), where the JVM needs a BufferedReader.
;; oracle: skip — the JVM spelling needs (java.io.BufferedReader. *in*);
;; frozen from clojure 1.12.5, 2026-07-23:
;;   (with-in-str "a\nb\nc" (doall (line-seq (java.io.BufferedReader. *in*))))
;;     => ("a" "b" "c")
;;   "" => nil; "a\n" => ("a"); "a\r\nb" => ("a" "b")
[(with-in-str "a\nb\nc" (doall (line-seq *in*)))
 (with-in-str "" (line-seq *in*))
 (with-in-str "a\n" (doall (line-seq *in*)))
 (with-in-str "a\r\nb" (doall (line-seq *in*)))]
;; expect: [("a" "b" "c") nil ("a") ("a" "b")]
