(ns pure.core (:require [pure.util :as u]))
(defn greet [n] (u/shout (str "hi " n)))
(def answer 42)
