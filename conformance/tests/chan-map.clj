;; ADR 0040 T2 + core-async-audit 2026-07: (map f chs) / (map f chs buf-or-n)
;; combines N channels — each round takes one value from every input and
;; delivers (apply f vals), closing as soon as ANY input closes. NOT
;; deprecated (unlike the arrow map< / map>); interns ONLY in
;; clojure.core.async, so clojure.core/map is untouched — the precedence
;; principle: async's `map` is a different var reached as
;; clojure.core.async/map, shadowing nothing in core (JVM core.async does
;; the same via :refer-clojure :exclude [map …]). Uneven inputs stop at the
;; shortest; an empty chs vector closes the output immediately.
;; oracle (JVM core.async 1.6.681, Clojure 1.12.5, fresh 2026-07-22 run):
;;   (map + [1 2 3]/[10 20 30])    => [11 22 33]
;;   (map vector [1 2]/[10 20 30]) => [[1 10] [2 20]]  (stops at shortest)
;;   (map - [5 6]/[1 2] 3)         => [4 4]            (buf-or-n arg)
;;   (map + [])                    => []               (empty closes)
;; oracle: skip — needs the core.async dep; frozen from the run above
(require '[clojure.core.async :as async])
[(async/<!! (async/into [] (async/map + [(async/to-chan! [1 2 3]) (async/to-chan! [10 20 30])])))
 (async/<!! (async/into [] (async/map vector [(async/to-chan! [1 2]) (async/to-chan! [10 20 30])])))
 (async/<!! (async/into [] (async/map - [(async/to-chan! [5 6]) (async/to-chan! [1 2])] 3)))
 (async/<!! (async/into [] (async/map + [])))]
;; expect: [[11 22 33] [[1 10] [2 20]] [4 4] []]
