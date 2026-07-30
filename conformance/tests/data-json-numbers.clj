;; clojure.data.json number reading (ADR 0097): an integer literal beyond the
;; long range promotes to bigint (N); a decimal literal is a double by default,
;; or a bigdec (M) under :bigdec true. The Go codec mirrors data.json's tower.
;; oracle (org.clojure/data.json 2.5.1, verified 2026-07-27):
;;   (json/read-str "12345678901234567890") => 12345678901234567890N
;;   (json/read-str "3.14" :bigdec true)     => 3.14M
;;   (json/read-str "3.5")                    => 3.5
;; oracle: skip — external contrib dep, not on the default oracle classpath;
;; verified manually vs data.json 2.5.1.
(require '[clojure.data.json :as json])
[(json/read-str "12345678901234567890")
 (json/read-str "3.14" :bigdec true)
 (json/read-str "3.5")]
;; expect: [12345678901234567890N 3.14M 3.5]
