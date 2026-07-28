;; ADR 0064 cross-var direct calls: a compiled call to a top-level defn of
;; the same compilation unit invokes the def's published typed handle
;; directly (no var deref, no lang.ApplyN dispatch) while the var's ADR
;; 0066 seal bit is armed — and MUST fall back to the var path the instant
;; anything mutates the root, so redefinition liveness is unchanged.
;; Shapes covered: a forward reference (declare g, f calls it before g's
;; def is emitted), a plain same-unit call (callh -> h), with-redefs of the
;; callee seen through that call site (and restored after), the callee used
;; as a VALUE (twice g), mutual recursion through a declare (ping/pong),
;; and alter-var-root — the other root writer — seen through the same site.
;; No JVM divergence here: h is an ordinary var with no :inline, so the JVM
;; also observes both redefinitions at the call site.
;; oracle: clojure 1.12.5 CLI, 2026-07-27 -> [[30 7] [-3 8] 7 200 :done [10 12]]
(declare g)
(defn f [x] (g x))
(defn g [x] (* x 10))
(defn h [a b] (+ a b))
(defn callh [a b] (h a b))
(defn twice [fx v] (fx (fx v)))
(declare pong)
(defn ping [n] (if (zero? n) :done (pong (dec n))))
(defn pong [n] (ping (dec n)))
(def r1 [(f 3) (callh 2 5)])
(def r2 (with-redefs [h (fn [a b] (- a b))] [(callh 2 5) (h 9 1)]))
(def r3 (callh 2 5))
(def r4 (twice g 2))
(def r5 (ping 8))
(alter-var-root #'h (fn [_] (fn [a b] (* a b))))
(def r6 [(callh 2 5) (h 3 4)])
[r1 r2 r3 r4 r5 r6]
;; expect: [[30 7] [-3 8] 7 200 :done [10 12]]
