(ns gob.leaf)
(require-go '[strconv :as sc])
(defn bump [n] (sc/Itoa (inc n)))
