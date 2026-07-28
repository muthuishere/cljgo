;; A metadata-carrying fn is still fully a fn: every arity of a multi-arity
;; fn dispatches through the wrapper, apply works, and it composes as a
;; higher-order argument (ADR 0105; cljgo boxes the closure in lang.MetaFn,
;; which delegates Invoke/ApplyTo — so none of this may change).
;; oracle (clojure 1.12.5, 2026-07-28):
;;   (defn base ([] :zero) ([x] [:one x]) ([x & r] [:var x r]))
;;   (let [g (with-meta base {:tag :wrapped})]
;;     [(g) (g 5) (g 1 2 3) (meta g)])
;;   => [:zero [:one 5] [:var 1 (2 3)] {:tag :wrapped}]
;;   ((with-meta (fn [x] (inc x)) {:a 1}) 41) => 42
;;   (apply (with-meta (fn [a b] (+ a b)) {:a 1}) [2 3]) => 5
;;   (map (with-meta (fn [x] (* 2 x)) {:a 1}) [1 2 3]) => (2 4 6)
(defn base ([] :zero) ([x] [:one x]) ([x & r] [:var x r]))
(let [g (with-meta base {:tag :wrapped})]
  [(g) (g 5) (g 1 2 3) (meta g)
   ((with-meta (fn [x] (inc x)) {:a 1}) 41)
   (apply (with-meta (fn [a b] (+ a b)) {:a 1}) [2 3])
   (map (with-meta (fn [x] (* 2 x)) {:a 1}) [1 2 3])])
;; expect: [:zero [:one 5] [:var 1 (2 3)] {:tag :wrapped} 42 5 (2 4 6)]
