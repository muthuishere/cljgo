;; update-keys / update-vals (clojure 1.11+, fundamentals audit 2026-07):
;; transform every key / every value of a map, preserving metadata.
;; oracle (clojure 1.12.5, 2026-07-21):
;;   (update-keys {:a 1 :b 2} name) => {"a" 1, "b" 2}
;;   (update-keys {} name) => {}
;;   (meta (update-keys (with-meta {:a 1} {:m 1}) name)) => {:m 1}
;;   (update-vals {:a 1 :b 2} inc) => {:a 2, :b 3}
;;   (update-vals {} inc) => {}
;;   (meta (update-vals (with-meta {:a 1} {:m 1}) inc)) => {:m 1}
[(update-keys {:a 1 :b 2} name)
 (update-keys {} name)
 (meta (update-keys (with-meta {:a 1} {:m 1}) name))
 (update-vals {:a 1 :b 2} inc)
 (update-vals {} inc)
 (meta (update-vals (with-meta {:a 1} {:m 1}) inc))]
;; expect: [{"a" 1, "b" 2} {} {:m 1} {:a 2, :b 3} {} {:m 1}]
