;; --- cljx.core prototype (ADR 0106) ---------------------------------
(defn add!
  "Append to a collection atom. (add! a x) => (swap! a conj x).
   On a map atom: (add! a k v) => (swap! a assoc k v)."
  ([a x] (swap! a conj x))
  ([a k v] (swap! a assoc k v))
  ([a k v & kvs] (apply swap! a assoc k v kvs)))

(defn del!
  "Remove from a collection atom. Map => dissoc, set => disj."
  [a k] (swap! a (fn [c] (if (set? c) (disj c k) (dissoc c k)))))

(defn bump!
  "Increment. (bump! a) => (swap! a inc).
   (bump! a k) => (swap! a update k (fnil inc 0)) — counters, in one word.
   (bump! a k n) => bump by n."
  ([a] (swap! a inc))
  ([a k] (swap! a update k (fnil inc 0)))
  ([a k n] (swap! a update k (fnil + 0) n)))

(defn upd!    [a k f & args] (apply swap! a update k f args))
(defn put-in! [a path v]     (swap! a assoc-in path v))
(defn upd-in! [a path f & a2] (apply swap! a update-in path f a2))
(defn clear!  [a]            (reset! a (empty @a)))
(defn toggle! [a]            (swap! a not))

(defn dbg
  "Print and RETURN the value — drops into any pipeline."
  ([x] (println "dbg:" (pr-str x)) x)
  ([label x] (println (str label ":") (pr-str x)) x))

;; --- before / after ---------------------------------------------------
(println "=== the todo list ===")
(def todo (atom []))
(add! todo "buy milk")
(add! todo "write docs")
(println @todo)

(println "\n=== counters (the vote tally) ===")
(def votes (atom {}))
(bump! votes "harsh")
(bump! votes "shaama")
(bump! votes "harsh")
(println @votes)

(println "\n=== a plain counter ===")
(def hits (atom 0))
(bump! hits) (bump! hits) (bump! hits)
(println @hits)

(println "\n=== maps and nesting ===")
(def user (atom {:name "Sreyash"}))
(add! user :city "Chennai")
(put-in! user [:prefs :theme] "dark")
(upd! user :name clojure.string/upper-case)
(println @user)
(del! user :city)
(println @user)

(println "\n=== flags and clearing ===")
(def debug? (atom false))
(toggle! debug?)
(println @debug?)
(clear! todo)
(println @todo)

(println "\n=== dbg drops into a pipeline ===")
(def total (->> [120 45 80]
                (dbg "prices")
                (filter #(> % 50))
                (dbg "over 50")
                (reduce +)))
(println "total:" total)
