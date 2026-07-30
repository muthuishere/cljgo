;; A record/type CONSTRUCTOR must name itself in an arity error, like any other
;; fn — CLAUDE.md's error doctrine calls naming the thing "the #1 win", and the
;; project already freezes it for plain fns (arity-error*.clj). Constructors had
;; no such test, and that gap let a real regression through: the fix for
;; defrecord's defining-namespace bug wrapped the ctor in a (let …) so the type
;; name would resolve at load time, which ALSO broke the def site's fn-name
;; inference — arity errors silently became "passed to: fn". Caught by an
;; adversarial verifier, not by the suite.
;;
;; Both properties must hold AT ONCE, which is what this file pins:
;;   1. the inline protocol impl dispatches for a record defined in ANOTHER
;;      namespace (the original bug), and
;;   2. the constructor is still named in an arity error (the regression).
;; The fix is to hoist the qualified name into its own def rather than a let,
;; keeping the fn directly in the def position.
;;
;; oracle: skip — cljgo's arity-error TEXT is cljgo's own (the JVM says
;; "Wrong number of args (1) passed to: mm.a/eval139/->Person--157", carrying
;; its compiler's mangled fn name). What is frozen here is the cljgo contract:
;; the ctor names ITSELF, qualified by its defining namespace. Dispatch
;; behaviour in part 1 IS JVM-verified in defrecord-defining-namespace.clj.
(ns ctorarity.lib)
(defprotocol Greet (greet [this]))
(defrecord Person [name age]
  Greet
  (greet [this] (str "hi " (:name this))))
(deftype T [x]
  Greet
  (greet [this] (str "T" (.-x this))))

(in-ns 'user)
(clojure.core/refer 'clojure.core)
(require '[ctorarity.lib :as a])

(defn- msg [f]
  (try (f) :no-throw
       (catch Throwable t
         ;; keep only the head: the locus varies per harness
         (let [m (ex-message t)
               i (clojure.string/index-of m " at ")]
           (if i (subs m 0 i) m)))))

[;; 1. the original bug: cross-namespace dispatch
 (a/greet (a/->Person "sreyash" 3))
 (a/greet (a/map->Person {:name "vidya" :age 2}))
 (a/greet (a/->T 7))
 ;; 2. the regression: the ctor names itself, qualified by its defining ns
 (msg #(a/->Person "only-one"))
 (msg #(a/->T))
 (msg #(a/map->Person))]
;; expect: ["hi sreyash" "hi vidya" "T7" "wrong number of args (1) passed to: ctorarity.lib/->Person" "wrong number of args (0) passed to: ctorarity.lib/->T" "wrong number of args (0) passed to: ctorarity.lib/map->Person"]
