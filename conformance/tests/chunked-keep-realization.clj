;; Chunk-aware keep (ADR 0063 follow-up) realizes a CHUNK at a time over a
;; chunked source, matching JVM Clojure exactly — not element-at-a-time (the old
;; core.clj lazy-seq behavior) and not whole-range. LongRange chunks in blocks of
;; 32, so pulling the first element of (keep f (range 1000)) realizes the whole
;; first chunk = 32. An UNCHUNKED source (iterate) still realizes only as many as
;; needed to yield the first kept value: keep #(when (odd? %) %) over 0,1,2,...
;; realizes 2 (0 -> nil dropped, 1 -> kept) — the fallback path is untouched and
;; stays element-at-a-time in both cljgo and the JVM.
;;
;; This is the companion contract to chunked-map-filter-realization.clj: it
;; freezes keep's chunked realization so a regression to element-wise (would read
;; 2 on the chunked leg) or whole-range (would read 1000) is caught. The move to
;; 32 is the deliberate JVM-parity decision of the ADR 0063 follow-up (native
;; chunk-aware keep alongside map/filter).
;; Oracle (clojure 1.12.5, verified 2026-07-23): [32 2]
[(let [c (atom 0)] (first (keep (fn [x] (swap! c inc) (when (odd? x) x)) (range 1000))) @c)
 (let [c (atom 0)] (first (keep (fn [x] (swap! c inc) (when (odd? x) x)) (iterate inc 0))) @c)]
;; expect: [32 2]
