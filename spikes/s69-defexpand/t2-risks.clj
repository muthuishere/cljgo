;; s69 t2 — the risks the ADR asks the spike to settle.
(defexpand add! [a x] (swap! a conj x))

;; RISK 1: with-redefs. Direct call sites are ALREADY expanded, so a
;; redefinition is invisible to them; the HOF/value path DOES see it.
(def a (atom []))
(defn indirect [f] (f a :via-hof))
(with-redefs [add! (fn [a x] (swap! a conj :REDEFFED))]
  (add! a :direct)          ; expanded at analysis time -> NOT redefined
  (indirect add!))          ; value path -> redefined
(println "1. with-redefs:" @a)

;; RISK 2: recursion — a self-referential body.
(println "2. recursion: see t2-recursion.clj (separate file, it aborts)")

;; RISK 3: arity mismatch at a direct call site.
(println "3. wrong arity:"
         (try (eval '(add! (atom []) 1 2 3)) (catch Throwable e (str "ERR: " (.Error e)))))

;; RISK 4: variadic defexpand
(println "4. variadic:"
         (try (eval '(do (defexpand vv [a & xs] (swap! a into xs)) :defined))
              (catch Throwable e (str "ERR: " (.Error e)))))

;; RISK 5: defexpand referring to a private/other-ns var in the body,
;; called from a DIFFERENT namespace (does qualification hold?).
(ns other.ns)
(clojure.core/refer 'clojure.core)
(def b (atom []))
(println "5. cross-ns:" (do (user/add! b :x) @b))
