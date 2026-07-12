;; cond over recursive expansion. Oracle (clojure 1.12.5):
;; [:neg :zero :pos], and (cond) => nil.
(defn classify [n]
  (cond
    (< n 0) :neg
    (= n 0) :zero
    :else :pos))
[(classify -1) (classify 0) (classify 5) (cond)]
;; expect: [:neg :zero :pos nil]
