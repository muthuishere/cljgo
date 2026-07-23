;; The tail-wave odds and ends (2026-07-23): replicate, test, *repl*,
;; unquote/unquote-splicing placeholder vars, await1, print-ctor,
;; definline, ->Eduction.
;; DEVIATIONS (documented, not frozen): definline stores :inline metadata
;; but no call-site inlining happens (performance-only); ->Eduction
;; returns the seq view eduction already is (the JVM's prints as
;; #object[clojure.core.Eduction ...]); *repl* is bound true only in the
;; interactive REPL/nREPL session frame — here (script context) it reads
;; its false root, exactly like `clojure -M script.clj`.
;; oracle (clojure 1.12.5, 2026-07-23, scratch tailwave/o1.clj+o3.clj):
;;   (replicate 3 :x) => (:x :x :x)
;;   (defn tf [] 1) (alter-meta! #'tf assoc :test (fn [] :ran))
;;   (test #'tf) => :ok; (test #'tg) => :no-test;
;;   (test #'clojure.core/map) => :no-test
;;   *repl* => false (script context)
;;   [(bound? #'unquote) (bound? #'unquote-splicing)] => [false false]
;;   (let [a (agent 0)] (send a inc) (await1 a) @a) => 1
;;   (with-out-str (print-ctor 5 (fn [o w] (.write w "5")) *out*))
;;     => "#=(java.lang.Long. 5)"
;;   (definline dsqr [x] `(* ~x ~x)) (dsqr 5) => 25; (fn? dsqr) => true;
;;   (map dsqr [1 2 3]) => (1 4 9)
;;   (vec (->Eduction (map inc) [1 2 3])) => [2 3 4]
;;   (reduce + 0 (->Eduction (map inc) [1 2 3])) => 9
(defn tf [] 1)
(alter-meta! #'tf assoc :test (fn [] :ran))
(defn tg [] 1)
(definline dsqr [x] `(* ~x ~x))
[(replicate 3 :x)
 (test #'tf)
 (test #'tg)
 (test #'clojure.core/map)
 *repl*
 [(bound? #'unquote) (bound? #'unquote-splicing)]
 (let [a (agent 0)] (send a inc) (await1 a) @a)
 (with-out-str (print-ctor 5 (fn [o w] (.write w "5")) *out*))
 (dsqr 5)
 (fn? dsqr)
 (vec (map dsqr [1 2 3]))
 (vec (->Eduction (map inc) [1 2 3]))
 (reduce + 0 (->Eduction (map inc) [1 2 3]))]
;; expect: [(:x :x :x) :ok :no-test :no-test false [false false] 1 "#=(java.lang.Long. 5)" 25 true [1 4 9] [2 3 4] 9]
