;; remove-all-methods (fundamentals audit 2026-07): empties the method
;; table — even :default goes — and returns the multifn.
;; oracle (clojure 1.12.5, 2026-07-21):
;;   before: (h :x) => 1; after: (methods h) => {} and (h :x) throws
;;   "No method in multimethod 'h' for dispatch value: :x";
;;   (= h (remove-all-methods h)) => true
(defmulti h identity)
(defmethod h :x [_] 1)
(defmethod h :default [_] :dflt)
(def before (h :x))
(def same (= h (remove-all-methods h)))
[before same (methods h)
 (try (h :x) (catch Exception e (ex-message e)))]
;; expect: [1 true {} "No method in multimethod 'h' for dispatch value: :x"]
