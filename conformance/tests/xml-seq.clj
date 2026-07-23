;; Batch A3: xml-seq — tree-seq over xml/parse-shaped element maps
;; ({:tag .. :attrs .. :content [..]}); strings are leaves.
;; Oracle (clojure 1.12.5): verified 2026-07-23.
(def x {:tag :a :attrs nil
        :content [{:tag :b :attrs nil :content ["hi"]}
                  {:tag :c :attrs nil :content nil}]})
[(map (fn [n] (if (string? n) n (:tag n))) (xml-seq x))
 (xml-seq "plain")
 (count (xml-seq x))]
;; expect: [(:a :b "hi" :c) ("plain") 4]
