;; Structs (deprecated on the JVM, still shipped) — create-struct /
;; defstruct / struct / struct-map / accessor, implemented over plain
;; array-maps (tail wave, 2026-07-23). Construction, basis-key order,
;; extras-after-basis order, positional defaults, accessor and the
;; too-many-args error all match the oracle byte-for-byte.
;; DEVIATIONS (documented): the JVM basis is an opaque
;; PersistentStructMap$Def (cljgo: the plain key vector) and its maps a
;; PersistentStructMap that throws "Can't remove struct key" on
;; (dissoc m :name) — cljgo's are ordinary array-maps, so that dissoc
;; succeeds; neither shape is frozen here.
;; oracle (clojure 1.12.5, 2026-07-23):
;;   (defstruct person :name :age)
;;   (struct person "amy" 3) => {:name "amy", :age 3}
;;   (struct person "amy") => {:name "amy", :age nil}
;;   (struct-map person :age 5 :name "bo" :extra 1)
;;     => {:name "bo", :age 5, :extra 1}
;;   (keys (struct-map person :extra 1 :age 2)) => (:name :age :extra)
;;   ((accessor person :name) (struct person "z" 9)) => "z"
;;   (assoc (struct person "a" 1) :x 9) => {:name "a", :age 1, :x 9}
;;   (into {} (struct person "a" 1)) => {:name "a", :age 1}
;;   (= (struct person "a" 1) {:name "a" :age 1}) => true
;;   (struct person 1 2 3) throws "Too many arguments to struct constructor"
(defstruct person :name :age)
[(struct person "amy" 3)
 (struct person "amy")
 (struct-map person :age 5 :name "bo" :extra 1)
 (vec (keys (struct-map person :extra 1 :age 2)))
 ((accessor person :name) (struct person "z" 9))
 (assoc (struct person "a" 1) :x 9)
 (into {} (struct person "a" 1))
 (= (struct person "a" 1) {:name "a" :age 1})
 (try (struct person 1 2 3) (catch Exception e (ex-message e)))
 (struct-map person)]
;; expect: [{:name "amy", :age 3} {:name "amy", :age nil} {:name "bo", :age 5, :extra 1} [:name :age :extra] "z" {:name "a", :age 1, :x 9} {:name "a", :age 1} true "Too many arguments to struct constructor" {:name nil, :age nil}]
