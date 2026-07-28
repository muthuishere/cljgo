;; clojure.core.match (ADR 0097): the ::not-found sentinel a map pattern hands
;; to a key's VALUE pattern when the key is absent.
;;
;; The key-union fix (core-match-map-key-union.clj) modelled a map row as
;; "every key it names must be present". That is right only for BARE wildcard
;; values. core.match binds each map sub-occurrence to
;; (val-at m k ::not-found) and then, in `wrap-values`, wraps ONLY a
;; wildcard-pattern value in a MapKeyPattern — whose test is
;; (not= ocr ::not-found). Any real pattern (literal, nested map, vector, or a
;; :guard) is applied DIRECTLY to that sub-occurrence, so on a missing key the
;; pattern sees :clojure.core.match/not-found:
;;   - a literal simply fails to equal it        => the row falls through
;;   - a nested map/vector fails its type test   => the row falls through
;;   - a :guard RUNS on the keyword and throws   => IllegalArgumentException
;; The existence tests are also evaluated LAST, after every guard in the row,
;; so a guard on ANY key fires before a sibling key's presence test.
;;
;; oracle (real `clojure` CLI, Clojure 1.12.5 + org.clojure/core.match 1.1.0,
;; run 2026-07-28 with
;;   clojure -Sdeps '{:deps {org.clojure/core.match {:mvn/version "1.1.0"}}}'):
;;   ;; guarded value, key may be absent  (a)
;;   (a {})           => throws IllegalArgumentException
;;                       "Argument must be an integer: :clojure.core.match/not-found"
;;   (a {:b 9})       => throws, same message
;;   (a {:a 2})       => [:even 2] · (a {:a 3}) => [:odd 3]
;;   (a {:a 3 :b 9})  => [:b 9]    · (a {:a 2 :b 9}) => [:even 2]
;;   ;; guarded row falls through to a later row on a PRESENT key  (k)
;;   (k {}) => :none · (k {:a "hi"}) => [:s "hi"] · (k {:a 5}) => [:other 5]
;;   ;; guard on one key, presence on another — guard wins the ordering  (g/h)
;;   (g {}) => throws · (g {:b 3}) => :none · (g {:a 1}) => throws
;;   (g {:a 1 :b 2}) => [:hit 1 2] · (g {:a 1 :b 3}) => :none
;;   (h {}) => throws · (h {:b 3}) => throws · (h {:a 2}) => :none
;;   (h {:a 2 :b 1}) => [:hit 2 1] · (h {:a 3 :b 1}) => :none
;;   ;; guard on the WHOLE map  (f)
;;   (f {}) => :none · (f {:a 1}) => [:one 1]
;;   (f {:a 1 :b 2}) => [:b 2] · (f {:b 2}) => [:b 2]
;;   ;; nested maps: outer sentinel is not a map; inner guard still throws  (l)
;;   (l {}) => :none · (l {:c 1}) => [:c 1] · (l {:a {}}) => throws
;;   (l {:a {:b 2}}) => [:hit 2] · (l {:a {:b 3}}) => :none · (l {:a 7}) => :none
;;   ;; literal / nested value on an absent key just fails  (b/d)
;;   (b {}) => :none · (b {:b 2}) => :hasb · (b {:a 1}) => :one · (b {:a nil}) => :none
;;   (d {}) => :none · (d {:c 5}) => [:c 5] · (d {:a {:b 7}}) => [:nested 7]
;;   (d {:a 3}) => :none · (d {:a {} :c 5}) => [:c 5]
;;   ;; the sentinel stored AS a value reads as absent  (i)
;;   (i {}) => :none · (i {:a :clojure.core.match/not-found}) => :none
;;   (i {:a nil}) => [:a nil]
;; oracle: skip — needs the org.clojure/core.match dep; frozen from that run
(require '[clojure.core.match :refer [match]])

(defn safe [f] (try (f) (catch Throwable e (ex-message e))))

;; guarded value on a key that may be absent
(defn a [m]
  (safe #(match [m]
           [{:a (x :guard even?)}] [:even x]
           [{:b y}]                [:b y]
           [{:a z}]                [:odd z]
           :else                   :none)))

;; a guarded row falling through to a later row on the SAME key
(defn k [m]
  (safe #(match [m]
           [{:a (x :guard string?)}] [:s x]
           [{:a y}]                  [:other y]
           :else                     :none)))

;; presence on :a, guard on :b — the guard runs first
(defn g [m] (safe #(match [m] [{:a x :b (y :guard even?)}] [:hit x y] :else :none)))
;; guard on :a, presence on :b — the guard still runs first
(defn h [m] (safe #(match [m] [{:a (x :guard even?) :b y}] [:hit x y] :else :none)))

;; :guard on the whole map (unaffected — the map itself is never ::not-found)
(defn f [m]
  (safe #(match [m]
           [({:a x} :guard (fn [v] (= 1 (count v))))] [:one x]
           [{:b y}]                                   [:b y]
           :else                                      :none)))

;; nested map with a guard inside
(defn l [m]
  (safe #(match [m]
           [{:a {:b (x :guard even?)}}] [:hit x]
           [{:c y}]                     [:c y]
           :else                        :none)))

;; literal and nested-map values on an absent key
(defn b [m] (safe #(match [m] [{:a 1}] :one [{:b _}] :hasb :else :none)))
(defn d [m] (safe #(match [m] [{:a {:b x}}] [:nested x] [{:c y}] [:c y] :else :none)))

;; the sentinel stored as a real value
(defn i [m] (safe #(match [m] [{:a x}] [:a x] :else :none)))

[(a {}) (a {:b 9}) (a {:a 2}) (a {:a 3}) (a {:a 3 :b 9}) (a {:a 2 :b 9})
 (k {}) (k {:a "hi"}) (k {:a 5})
 (g {}) (g {:b 3}) (g {:a 1}) (g {:a 1 :b 2}) (g {:a 1 :b 3})
 (h {}) (h {:b 3}) (h {:a 2}) (h {:a 2 :b 1}) (h {:a 3 :b 1})
 (f {}) (f {:a 1}) (f {:a 1 :b 2}) (f {:b 2})
 (l {}) (l {:c 1}) (l {:a {}}) (l {:a {:b 2}}) (l {:a {:b 3}}) (l {:a 7})
 (b {}) (b {:b 2}) (b {:a 1}) (b {:a nil})
 (d {}) (d {:c 5}) (d {:a {:b 7}}) (d {:a 3}) (d {:a {} :c 5})
 (i {}) (i {:a :clojure.core.match/not-found}) (i {:a nil})]
;; expect: ["Argument must be an integer: :clojure.core.match/not-found" "Argument must be an integer: :clojure.core.match/not-found" [:even 2] [:odd 3] [:b 9] [:even 2] :none [:s "hi"] [:other 5] "Argument must be an integer: :clojure.core.match/not-found" :none "Argument must be an integer: :clojure.core.match/not-found" [:hit 1 2] :none "Argument must be an integer: :clojure.core.match/not-found" "Argument must be an integer: :clojure.core.match/not-found" :none [:hit 2 1] :none :none [:one 1] [:b 2] [:b 2] :none [:c 1] "Argument must be an integer: :clojure.core.match/not-found" [:hit 2] :none :none :none :hasb :one :none :none [:c 5] [:nested 7] :none [:c 5] :none :none [:a nil]]
