;; iterator-seq / enumeration-seq over the Go host's iterator shapes
;; (tail wave, 2026-07-23; pkg/corelib/compat_builtins.go). Receiver set
;; (documented — the cljgo-truthful analogue): (a) any Go value with a
;; HasNext() bool / Next() any method pair (iterator-seq) or
;; HasMoreElements()/NextElement() (enumeration-seq, falling back to
;; HasNext/Next), (b) a core.async channel — the seq takes until it
;; closes — and (c) a raw Go channel. nil in => nil out.
;; JVM shape cited (clojure 1.12.5, scratch tailwave/o3.clj):
;;   (iterator-seq (.iterator [1 2 3])) => (1 2 3); empty iterator => nil
;;   (enumeration-seq (java.util.Collections/enumeration [1 2])) => (1 2)
;; oracle: skip — the receivers are Go-host iterator shapes; there is no
;; java.util.Iterator/Enumeration on a Go host (JVM shape cited above)
(def c1 (chan 5))
(>!! c1 1) (>!! c1 2) (>!! c1 3)
(close! c1)
(def c2 (chan 2))
(close! c2)
(def c3 (chan 2))
(>!! c3 :a) (>!! c3 :b)
(close! c3)
[(iterator-seq c1)
 (iterator-seq c2)
 (enumeration-seq c3)
 (iterator-seq nil)
 (enumeration-seq nil)]
;; expect: [(1 2 3) nil (:a :b) nil nil]
