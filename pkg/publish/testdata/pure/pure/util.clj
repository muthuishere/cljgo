(ns pure.util)
(defn shout [s] (clojure.string/upper-case s))
(defn ^:private helper [x] (inc x))
