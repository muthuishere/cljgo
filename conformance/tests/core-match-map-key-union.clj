;; clojure.core.match (ADR 0097): map patterns across rows with DIFFERENT key
;; sets. Map patterns are not mutually exclusive constructors — {:kind :click}
;; and {:kind :click :button "left"} both match the same map — so the Maranget
;; column must dispatch on ONE map constructor over the union of the column's
;; keys, re-checking each row's own keys with `contains?`. Compiling one
;; constructor per key set made every arm after the first dead (cljgo returned
;; "unknown" where the JVM returns "other click").
;;
;; Key presence, not (get m k), is what the JVM tests: {:a _} matches
;; {:a nil} but not {}.
;;
;; oracle (real `clojure` CLI, Clojure 1.12.5 + org.clojure/core.match 1.1.0,
;; run 2026-07-28 with
;;   clojure -Sdeps '{:deps {org.clojure/core.match {:mvn/version "1.1.0"}}}'):
;;   (f {:kind :click :button "left"})  => "left click"
;;   (f {:kind :click :button "right"}) => "other click"
;;   (f {:kind :click})                 => "other click"
;;   (f {:kind :scroll})                => "unknown"
;;   (g {:a 1}) => :has-a · (g {:a nil}) => :has-a · (g {}) => :no
;;   (h {:a 1 :b 2}) => [:both 1 2] · (h {:a 1}) => [:only-a 1] · (h {}) => :none
;;   (k {:t 1 :u 2 :v 3}) => :tuv · (k {:t 1 :u 2}) => :tu · (k {:u 2}) => :u
;;   (k {:t 1}) => :t · (k {:t 1 :v 3}) => :t · (k {:z 9}) => :other
;;   (n {:a {:b 1}}) => :ab1 · (n {:a {:b 2}}) => :ab · (n {:a {}}) => :no
;;   (q {:x 1} {:y 2}) => :xy · (q {:x 1} {:y 3}) => :x · (q {:x 2} {:y 2}) => :o
;; oracle: skip — needs the org.clojure/core.match dep; frozen from that run
(require '[clojure.core.match :refer [match]])

(defn f [e]
  (match [e]
    [{:kind :click :button "left"}] "left click"
    [{:kind :click}]                "other click"
    :else                           "unknown"))

(defn g [m] (match [m] [{:a _}] :has-a :else :no))

(defn h [m]
  (match [m]
    [{:a x :b y}] [:both x y]
    [{:a x}]      [:only-a x]
    :else         :none))

(defn k [m]
  (match [m]
    [{:t 1 :u 2 :v 3}] :tuv
    [{:t 1 :u 2}]      :tu
    [{:u 2}]           :u
    [{:t 1}]           :t
    :else              :other))

(defn n [m] (match [m] [{:a {:b 1}}] :ab1 [{:a {:b _}}] :ab :else :no))

(defn q [a b] (match [a b] [{:x 1} {:y 2}] :xy [{:x 1} _] :x :else :o))

[(f {:kind :click :button "left"})
 (f {:kind :click :button "right"})
 (f {:kind :click})
 (f {:kind :scroll})
 (g {:a 1}) (g {:a nil}) (g {}) (g {:b 1})
 (h {:a 1 :b 2}) (h {:a 1}) (h {:b 2}) (h {})
 (k {:t 1 :u 2 :v 3}) (k {:t 1 :u 2}) (k {:u 2}) (k {:t 1}) (k {:t 1 :v 3}) (k {:z 9})
 (n {:a {:b 1}}) (n {:a {:b 2}}) (n {:a {}}) (n {})
 (q {:x 1} {:y 2}) (q {:x 1} {:y 3}) (q {:x 2} {:y 2})]
;; expect: ["left click" "other click" "other click" "unknown" :has-a :has-a :no :no [:both 1 2] [:only-a 1] :none :none :tuv :tu :u :t :t :other :ab1 :ab :no :no :xy :x :o]
