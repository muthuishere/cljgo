;; Same rule as defrecord: a deftype's type is named for the namespace that
;; DEFINES it, so its inline protocol impls dispatch when the ->Ctor is called
;; from another namespace, and two same-short-named types stay distinct.
;; oracle: this EXACT file evaluated against clojure 1.12.5 (clojure CLI
;; 1.12.5.1645, 2026-07-30) prints the same vector.
(in-ns 'dtns.a)
(clojure.core/refer 'clojure.core)
(defprotocol P (m [this]))
(deftype T [x] P (m [this] (str "a:" x)))
(defn mk [v] (->T v))

(in-ns 'dtns.b)
(clojure.core/refer 'clojure.core)
(deftype T [x])
(defn mk [v] (->T v))

(in-ns 'user)
(clojure.core/refer 'clojure.core)
[(dtns.a/m (dtns.a/mk 3))
 (dtns.a/m (dtns.a/->T 4))
 (satisfies? dtns.a/P (dtns.a/mk 1))
 (satisfies? dtns.a/P (dtns.b/mk 1))
 (.-x (dtns.b/mk 9))]
;; expect: ["a:3" "a:4" true false 9]
