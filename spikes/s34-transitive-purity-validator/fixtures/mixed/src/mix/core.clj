(ns mix.core (:require [mix.pureside :as p] [mix.goside :as g]))
(defn run [] [(p/twice 3) (g/up "hey")])
