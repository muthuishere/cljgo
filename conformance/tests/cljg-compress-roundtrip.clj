;; cljg.compress (ADR 0103 wave 1, spike s61): gzip / deflate / zlib
;; round-trips over Go's stdlib compress/*. Freezes ROUND-TRIP EQUALITY and
;; decompressed CONTENT only — never raw compressed bytes (gzip headers embed
;; an OS byte and mtime, so compressed output is not stable across platforms).
;; Covers: whole-value round-trip for all three codecs (bytes and {:as
;; :string}), an explicit :level, the empty string, the streaming
;; decompress-on-read wrappers composing with cljg.stream (read-all over a
;; gunzip-stream of a byte-array source), and that corrupt input / a bad
;; :level throw instead of returning garbage.
;;
;; oracle: n/a — cljg.compress is a cljgo host namespace with no JVM package,
;; so this file cannot run through the `clojure` CLI verbatim. The codec
;; semantics are RFC 1952/1951/1950 as implemented by Go's compress/gzip,
;; compress/flate and compress/zlib (round-trip identity proven in spike
;; s61-cljg-compress). Frozen against cljgo's own output; REPL-vs-binary
;; parity is enforced by the dual harness.
(require '[cljg.compress :as z])
(require '[cljg.stream :as st])
(let [s "the quick brown fox jumps over the lazy dog — cljg.compress 0123456789"]
  [(= s (z/gunzip (z/gzip s) {:as :string}))
   (= s (z/inflate (z/deflate s) {:as :string}))
   (= s (z/zlib-decompress (z/zlib-compress s) {:as :string}))
   (= s (z/gunzip (z/gzip s {:level 9}) {:as :string}))
   (= "" (z/gunzip (z/gzip "") {:as :string}))
   (= s (st/read-all (z/gunzip-stream (z/gzip s))))
   (= s (st/read-all (z/inflate-stream (z/deflate s))))
   (= s (st/read-all (z/zlib-decompress-stream (z/zlib-compress s))))
   (try (z/gunzip "not gzip data") (catch Throwable e :threw))
   (try (z/gzip s {:level 42}) (catch Throwable e :threw))])
;; expect: [true true true true true true true true :threw :threw]
