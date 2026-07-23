;; Chunk-aware map/filter (ADR 0063) realize a CHUNK at a time over a chunked
;; source, matching JVM Clojure exactly — not element-at-a-time (the old
;; unchunked behavior) and not whole-range (an uncapped chunk). LongRange chunks
;; in blocks of 32, so pulling the first element of (map f (range 1000)) realizes
;; the whole first chunk = 32; through a filter it is still 32 (the map chunk is
;; realized to feed the filter). An UNCHUNKED source (iterate) still realizes
;; exactly 1 — the fallback path is untouched, so it stays element-at-a-time in
;; both cljgo and the JVM (guarded separately by
;; lazy-map-filter-no-over-realization.clj).
;;
;; This is the companion contract to that probe: it freezes the chunked count so
;; a regression to element-wise (would read 1) or whole-range (would read 1000)
;; is caught. The move to 32 is the deliberate JVM-parity decision of ADR 0063.
;; Oracle (clojure 1.12.5, verified 2026-07-23): [32 1 32]
[(let [c (atom 0)] (first (map (fn [x] (swap! c inc) x) (range 1000))) @c)
 (let [c (atom 0)] (first (map (fn [x] (swap! c inc) x) (iterate inc 0))) @c)
 (let [c (atom 0)] (first (filter odd? (map (fn [x] (swap! c inc) x) (range 1000)))) @c)]
;; expect: [32 1 32]
