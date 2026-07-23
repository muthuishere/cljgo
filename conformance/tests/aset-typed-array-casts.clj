;; The typed-array micro-API (tail wave, 2026-07-23): aset-<type> (aset
;; + the checked element cast, RETURNING the original val — the JVM
;; def-aset contract) and the array casts booleans/bytes/chars/doubles/
;; floats/ints/longs/shorts (no-op on a matching array, nil for nil,
;; ClassCastException otherwise).
;; DEVIATIONS (documented, not frozen): cljgo's int-array and long-array
;; are both []int64 (ADR 0025 — one fixnum representation), so
;; (ints (long-array ..)) succeeds here where the JVM throws [J-vs-[I;
;; bytes likewise accepts Go-native []byte; (identical? a (ints a)) is
;; false here (Go slice headers never compare identical) though the SAME
;; array is returned — mutation via the returned value proves it below.
;; oracle (clojure 1.12.5, 2026-07-23, scratch tailwave/o2.clj+o3.clj):
;;   (let [a (int-array 3)] [(aset-int a 0 7) (vec a)]) => [7 [7 0 0]]
;;   (let [a (int-array [0 0])] (aset-int a 0 1.5) (vec a)) => [1 0]
;;     (the cast truncates, the RETURN is the original 1.5)
;;   (let [a (double-array 2)] (aset-double a 1 2.5) (vec a)) => [0.0 2.5]
;;   (let [a (boolean-array 2)] (aset-boolean a 0 true) (vec a)) => [true false]
;;   (let [a (char-array 2)] (aset-char a 0 \z) (first a)) => \z
;;   (let [a (byte-array 2)] (aset-byte a 0 7) (vec a)) => [7 0]
;;   (let [a (long-array 2)] (aset-long a 0 9) (vec a)) => [9 0]
;;   (let [a (short-array 2)] (aset-short a 0 3) (vec a)) => [3 0]
;;   (let [a (float-array 2)] (aset-float a 0 1.5) (vec a)) => [1.5 0.0]
;;   (vec (ints (int-array [1 2]))) => [1 2]
;;   (vec (longs (long-array [1 2]))) => [1 2]
;;   (vec (doubles (double-array [1.5]))) => [1.5]
;;   (booleans nil) => nil
;;   (let [a (int-array [5])] (aset-int (ints a) 0 9) (vec a)) => [9] —
;;     the cast returns the same array
[(let [a (int-array 3)] [(aset-int a 0 7) (vec a)])
 (let [a (int-array [0 0])] [(aset-int a 0 1.5) (vec a)])
 (let [a (double-array 2)] (aset-double a 1 2.5) (vec a))
 (let [a (boolean-array 2)] (aset-boolean a 0 true) (vec a))
 (let [a (char-array 2)] (aset-char a 0 \z) (first a))
 (let [a (byte-array 2)] (aset-byte a 0 7) (vec a))
 (let [a (long-array 2)] (aset-long a 0 9) (vec a))
 (let [a (short-array 2)] (aset-short a 0 3) (vec a))
 (let [a (float-array 2)] (aset-float a 0 1.5) (vec a))
 (vec (ints (int-array [1 2])))
 (vec (longs (long-array [1 2])))
 (vec (doubles (double-array [1.5])))
 (booleans nil)
 (let [a (int-array [5])] (aset-int (ints a) 0 9) (vec a))]
;; expect: [[7 [7 0 0]] [1.5 [1 0]] [0.0 2.5] [true false] \z [7 0] [9 0] [3 0] [1.5 0.0] [1 2] [1 2] [1.5] nil [9]]
