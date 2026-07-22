;; ADR 0040 tier T3: when the pipeline transform THROWS, the optional
;; ex-handler (the 6th arg) receives the exception and its return replaces
;; the poisoned value; an ex-handler returning nil (and the DEFAULT handler,
;; when none is supplied — the JVM logs+drops, cljgo drops silently) drops
;; the value and the pipeline keeps running. Here x=2 throws; the rest map
;; to (* x 10).
;; oracle (JVM core.async 1.6.681, Clojure 1.12.5, fresh 2026-07-22 run):
;;   ex-handler (fn [e] :handled) => [10 :handled 30]
;;   ex-handler (fn [e] nil)      => [10 30]
;;   (default handler, none)      => [10 30]   (value dropped, error logged)
;; oracle: skip — needs the core.async dep; frozen from the run above
(require '[clojure.core.async :as async])
[(async/<!! (async/into [] (let [to (async/chan 100)]
   (async/pipeline 3 to (map (fn [x] (if (= x 2) (throw (ex-info "boom" {})) (* x 10))))
                   (async/to-chan! [1 2 3]) true (fn [e] :handled)) to)))
 (async/<!! (async/into [] (let [to (async/chan 100)]
   (async/pipeline 3 to (map (fn [x] (if (= x 2) (throw (ex-info "boom" {})) (* x 10))))
                   (async/to-chan! [1 2 3]) true (fn [e] nil)) to)))
 (async/<!! (async/into [] (let [to (async/chan 100)]
   (async/pipeline 3 to (map (fn [x] (if (= x 2) (throw (ex-info "boom" {})) (* x 10))))
                   (async/to-chan! [1 2 3])) to)))]
;; expect: [[10 :handled 30] [10 30] [10 30]]
