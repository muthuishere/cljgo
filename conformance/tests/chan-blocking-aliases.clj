;; ADR 0040 #5: the !! blocking / thread-variant names are ALIASES of their
;; parking siblings — real goroutines collapse the JVM's park-vs-thread-pool
;; distinction, so alts!! == alts!, to-chan!! == to-chan!, onto-chan!! ==
;; onto-chan! behaviourally. This freezes the three !! aliases that the
;; other chan-* files reach only through their non-!! siblings (core-async-
;; audit 2026-07 sync pass):
;;   alts!!      — blocking select: [val port] for a ready port; :default
;;                 when nothing is ready.
;;   to-chan!!   — a channel of coll's values that closes after.
;;   onto-chan!! — pumps coll onto ch, closes it, returns a done channel.
;; oracle (JVM core.async 1.6.681, Clojure 1.12.5, fresh 2026-07-22 run):
;;   alts!! ready => [:hi true] · alts!! :default => [:none :default] ·
;;   to-chan!! => [1 2 3] · onto-chan!! => [7 8 9]
;; oracle: skip — needs the core.async dep; frozen from the run above
(require '[clojure.core.async :as async])
(let [c   (async/chan 1)
      _   (async/>!! c :hi)
      rd  (async/alts!! [c])
      dfl (async/alts!! [(async/chan)] :default :none)
      tc  (async/<!! (async/into [] (async/to-chan!! [1 2 3])))
      oc  (async/chan 5)
      _   (async/<!! (async/onto-chan!! oc [7 8 9]))
      ocv (async/<!! (async/into [] oc))]
  [(first rd) (identical? (second rd) c) dfl tc ocv])
;; expect: [:hi true [:none :default] [1 2 3] [7 8 9]]
