;; with-redefs must stay live through the ADR 0067 second-table unboxed
;; paths. Those emissions never deref the operator var — `(quot x 10)` is a
;; raw rt.IQuot, `(even? x)` is a raw `x%2 == 0` — so liveness rests
;; entirely on the ops joining the sealed core set (rt.SealedCoreNames):
;; redefining one trips lang.CoreArithDirty, every typed region's
;; `!rt.CoreDirty()` entry guard fails, and the boxed body reads the
;; redefined var per call. pkg/emit's TestUnboxedOpsAreSealed enforces that
;; every open-coded op is in that set; this file proves the behavior end to
;; end through the specialized/lifted (digit-sum, half, biggest, mask) and
;; loop-carrier shapes, with pristine calls before and after to prove the
;; restore.
;; NOTE deliberate JVM divergence (ADR 0066 §context, same as
;; numeric-le-ge-redefs-unboxed.clj): JVM 1.12.5 :inline arithmetic does not
;; see these redefs at compiled call sites at all. cljgo is strictly MORE
;; live (ADR 0004); eval and compiled must match byte-identically.
;; oracle: cljgo eval harness (JVM divergence documented above).
(defn digit-sum [n] (loop [x n s 0] (if (< x 1) s (recur (quot x 10) (+ s (rem x 10))))))
(defn half [n] (if (even? n) (quot n 2) n))
(defn biggest [a b] (max a b))
(defn mask [a b] (bit-and a b))

[(digit-sum 987654)
 (half 10) (half 7)
 (biggest 3 9)
 (mask 12 10)
 (with-redefs [quot (fn [a b] 0)] (digit-sum 987654))
 (with-redefs [even? (fn [n] false)] (half 10))
 (with-redefs [max (fn [a b] (min a b))] (biggest 3 9))
 (with-redefs [bit-and (fn [a b] (bit-or a b))] (mask 12 10))
 (digit-sum 987654)
 (half 10)
 (biggest 3 9)
 (mask 12 10)]
;; expect: [39 5 7 9 8 4 10 3 14 39 5 9 8]
