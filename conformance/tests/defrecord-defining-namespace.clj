;; A defrecord's type is named for the namespace that DEFINES it, not for
;; whatever namespace happens to be current when its ->Ctor / map->Ctor is
;; later CALLED. That distinction is what makes inline protocol impls
;; dispatch for a record defined in a required library namespace, and what
;; keeps two same-short-named records in different namespaces apart.
;; oracle: this EXACT file evaluated against clojure 1.12.5 (clojure CLI
;; 1.12.5.1645, 2026-07-30) prints the same vector.
(in-ns 'recns.a)
(clojure.core/refer 'clojure.core)
(defprotocol Greet (greet [this]))
(defrecord Person [name]
  Greet
  (greet [this] (str "hello " (:name this))))
(defn make [n] (->Person n))
(defrecord Boxed [v])

(in-ns 'recns.b)
(clojure.core/refer 'clojure.core)
(defrecord Person [name])
(defn make [n] (->Person n))

(in-ns 'recns.c)
(clojure.core/refer 'clojure.core)
(defn use-a [n] (recns.a/greet (recns.a/make n)))

(in-ns 'user)
(clojure.core/refer 'clojure.core)
(extend-protocol recns.a/Greet
  recns.a.Boxed
  (greet [this] (str "boxed " (:v this))))

[(recns.a/greet (recns.a/make "sreyash"))
 (recns.a/greet (recns.a/->Person "direct"))
 (recns.a/greet (recns.a/map->Person {:name "mapped"}))
 (recns.c/use-a "third")
 (recns.a/greet (recns.a/->Boxed 7))
 (pr-str (recns.a/make "x"))
 (pr-str (recns.b/make "x"))
 (= (recns.a/make "a") (recns.b/make "a"))
 (= (recns.a/make "a") (recns.a/map->Person {:name "a"}))
 (map? (recns.a/make "a"))
 (:name (recns.b/->Person "bee"))]
;; expect: ["hello sreyash" "hello direct" "hello mapped" "hello third" "boxed 7" "#recns.a.Person{:name \"x\"}" "#recns.b.Person{:name \"x\"}" false true true "bee"]
