;; cljg.io/read-bytes + write-bytes and cljg.stream/of-file + to-file (ADR 0110
;; ask 1): the route between a FILE and BYTES. slurp/spit are the text route and
;; are lossy for non-UTF-8 content on BOTH hosts, so this freezes the byte one:
;; a round-trip through 0xFF/0x00 survives byte-for-byte, the elements are
;; SIGNED like the JVM's byte[], {:append true} appends, a byte-array from
;; clojure.core/byte-array writes as-is, and the ONE stream abstraction now
;; opens a file (lines / reduce over of-file, write-line into to-file).
;;
;; oracle: partial. cljg.io / cljg.stream are cljgo host namespaces with no JVM
;; package, so this file cannot run through the `clojure` CLI verbatim. The two
;; semantics that DO have a JVM equivalent were verified against it at authoring
;; time (clojure 1.12.5.1645, 2026-07-30):
;;   (java.nio.file.Files/write p (byte-array [104 101 -1 0]) …)
;;   (vec (java.nio.file.Files/readAllBytes p))  => [104 101 -1 0]
;;   (class (java.nio.file.Files/readAllBytes p)) => byte/1   ; signed bytes
;; i.e. a 0xFF byte reads back as -1, not 255 — which is what this file pins.
;; The rest (append, of-file/to-file) is cljgo's own surface, frozen against
;; cljgo's output; REPL-vs-binary parity is enforced by the dual harness.
(require '[cljg.io :as io])
(require '[cljg.stream :as st])
(let [p  (io/temp-file "cljg-bytes-" ".bin")
      bs (byte-array [104 101 108 108 111 -1 0])
      n  (io/write-bytes p bs)
      rt (io/read-bytes p)]
  (io/write-bytes p (byte-array [33]) {:append true})
  (let [q (io/temp-file "cljg-bytes-" ".txt")
        w (st/to-file q)]
    (st/write-line w "alpha")
    (st/write-line w "beta")
    (st/close w)
    (let [out [n
               (vec rt)
               (bytes? rt)
               (= (vec bs) (vec rt))
               (vec (io/read-bytes p))
               (vec (st/lines (st/of-file q)))
               ;; the handle IS reducible: reduce walks it a CHUNK at a time
               ;; (not a line at a time), so this totals the raw bytes, 11.
               (reduce (fn [acc chunk] (+ acc (count chunk))) 0 (st/of-file q))
               (st/read-all (st/of-file q))
               (try (io/read-bytes (io/path (io/temp-dir) "absent.bin"))
                    (catch Throwable e :threw))]]
      (io/delete! p)
      (io/delete! q)
      out)))
;; expect: [7 [104 101 108 108 111 -1 0] true true [104 101 108 108 111 -1 0 33] ["alpha" "beta"] 11 "alpha\nbeta\n" :threw]
