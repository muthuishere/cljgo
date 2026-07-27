;; clojure.data.json write-str escape options (ADR 0097): :escape-unicode false
;; emits raw UTF-8 for non-ASCII, and :escape-slash false leaves '/' bare.
;; oracle (org.clojure/data.json 2.5.1, verified 2026-07-27):
;;   (json/write-str "a/b héllo" :escape-unicode false :escape-slash false)
;;   => "a/b héllo"
;; oracle: skip — external contrib dep, not on the default oracle classpath;
;; verified manually vs data.json 2.5.1.
(require '[clojure.data.json :as json])
(json/write-str "a/b héllo" :escape-unicode false :escape-slash false)
;; expect: "\"a/b héllo\""
