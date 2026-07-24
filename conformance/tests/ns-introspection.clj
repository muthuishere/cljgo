;; harness: standalone — introspects the live namespace registry (all-ns,
;; ns-publics/interns/map/refers over 'user) incl. membership of some-var,
;; which a shared batch binary could pollute; runs as its own binary.
;; Namespace introspection (fundamentals audit 2026-07): ns-name/the-ns/
;; all-ns/ns-publics/ns-interns/ns-map/ns-refers over the live registry.
;; Frozen via membership probes — never by counting a namespace's vars
;; (cljgo's clojure.core surface is a superset-in-progress of the JVM's).
;; oracle (clojure 1.12.5, 2026-07-21): every probe below is true/matches
;; on the JVM (same file, run with `clojure -M`):
;;   (ns-name 'clojure.core) => clojure.core; (ns-name *ns*) => user
;;   (the-ns 'nope.nope) throws "No namespace: nope.nope found"
;;   (some #(= 'clojure.core (ns-name %)) (all-ns)) => true
;;   (contains? (ns-publics 'clojure.core) 'map) => true
;;   after (def some-var 42): publics/interns contain it, refers doesn't,
;;   its var derefs to 42; ^:private is in interns but NOT publics;
;;   'map is in (ns-map 'user) and (ns-refers 'user)
(def some-var 42)
(def ^:private priv-var 1)
[(ns-name 'clojure.core)
 (ns-name *ns*)
 (try (the-ns 'nope.nope) (catch Exception e (ex-message e)))
 (= (the-ns 'user) (the-ns (the-ns 'user)))
 (some (fn [n] (= 'clojure.core (ns-name n))) (all-ns))
 (contains? (ns-publics 'clojure.core) 'map)
 (contains? (ns-publics 'user) 'some-var)
 (contains? (ns-interns 'user) 'some-var)
 (deref (get (ns-publics 'user) 'some-var))
 (contains? (ns-publics 'user) 'priv-var)
 (contains? (ns-interns 'user) 'priv-var)
 (contains? (ns-map 'user) 'map)
 (contains? (ns-refers 'user) 'map)
 (contains? (ns-refers 'user) 'some-var)]
;; expect: [clojure.core user "No namespace: nope.nope found" true true true true true 42 false true true true false]
