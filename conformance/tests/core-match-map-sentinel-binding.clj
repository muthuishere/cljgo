;; clojure.core.match — the ::not-found sentinel must NEVER escape into user
;; code, even though the matcher applies :guards to it.
;;
;; core.match binds each map sub-occurrence with (get m k ::not-found) so an
;; absent key is distinguishable from a nil value, and applies value patterns
;; and :guards to THAT. When a guard on an absent key THROWS (even?, odd?,
;; string?) the sentinel is never observed — that case is frozen in
;; core-match-map-not-found.clj. But when a guard SUCCEEDS on the sentinel
;; (keyword?, some?, identity, or a guard that tests for the sentinel itself),
;; the row matches and the action body runs — and there the JVM binds nil, NOT
;; the sentinel. The guard and the binding see DIFFERENT values.
;;
;; The decisive probe is case A, which asks the action body both questions at
;; once: keyword? passed (so the guard genuinely received ::not-found), yet
;; inside the body x is nil and is NOT the sentinel.
;;
;; This was found by an adversarial verifier after the key-union fix: the
;; first implementation bound one gensym for both roles, so the sentinel
;; leaked into every successful-guard-on-absent-key action.
;;
;; oracle: skip — needs the core.match dep. Verified 2026-07-28 against the
;; real CLI with
;;   clojure -Sdeps '{:deps {org.clojure/core.match {:mvn/version "1.1.0"}}}'
;; which printed, byte-identical to the frozen value below:
;;   [[:kw nil true false] [:kw :k false false] [:some nil] [:nb nil] [:sent nil] :threw]
(require '[clojure.core.match :refer [match]])

;; A — guard succeeds on an absent key; the body inspects what it got.
(defn a [m]
  (match [m]
    [{:a (x :guard keyword?)}] [:kw x (nil? x) (= x :clojure.core.match/not-found)]
    :else :none))

;; B — a succeeding guard that is not about keywords at all.
(defn b [m]
  (match [m]
    [{:a (x :guard some?)}] [:some x]
    :else :none))

;; C — the same, one level down: the sentinel must not escape a nested map.
(defn c [m]
  (match [m]
    [{:a {:b (x :guard keyword?)}}] [:nb x]
    :else :none))

;; D — a guard that explicitly tests FOR the sentinel: it passes (the guard
;; really does see it) and the binding is still nil.
(defn d [m]
  (match [m]
    [{:a (x :guard #(= % :clojure.core.match/not-found))}] [:sent x]
    :else :none))

;; E — the throwing case still throws: nil-ing the BINDING must not soften
;; what the GUARD sees (this is core-match-map-not-found.clj's contract).
(defn e [m]
  (match [m]
    [{:a (x :guard even?)}] [:even x]
    [{:b y}]                [:b y]
    :else :none))

[(a {}) (a {:a :k}) (b {}) (c {:a {}}) (d {}) (try (e {}) (catch Throwable t :threw))]
;; expect: [[:kw nil true false] [:kw :k false false] [:some nil] [:nb nil] [:sent nil] :threw]
