;; extend-type extends one type to protocols; here a deftype gets an inline
;; impl and a built-in (Long) is extended after the fact. cljgo's built-in
;; designator is Long (JVM oracle: java.lang.Long — same behavior).
;; Verified vs Clojure CLI 1.12.5: => ["woof" "num 5"]
;; expect: ["woof" "num 5"]
(defprotocol Speak (speak [this]))

(deftype Dog []
  Speak
  (speak [this] "woof"))

(extend-type Long
  Speak
  (speak [this] (str "num " this)))

[(speak (->Dog)) (speak 5)]
