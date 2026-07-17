;; map / filter must not realize the source seq one element AHEAD of what the
;; consumer asked for. The tail of each lazy node is Clojure's `rest` (ISeq
;; More — hands back the UNREALIZED remainder), never `next` (= seq(more),
;; which forces the following element to decide nil-ness).
;;
;; Invisible on a vector; observable the moment the source element is
;; side-effecting, blocking, or expensive — e.g. (map f (repeatedly read-line))
;; would consume a line nobody asked for. Caught by this exact probe when
;; reduce/map/filter/mapv/comp went native Go (ADR 0045): the Go port used
;; s.Next() in the tail position and every count below read 2 instead of 1,
;; while the core.clj defn it replaced — and the JVM — read 1.
;;
;; Third case pins the 2-coll zip arity (map2Seq), which had the same defect.
;; Oracle (clojure 1.12.5): [1 1 1]
[(let [c (atom 0) src (repeatedly (fn [] (swap! c inc) 1))] (first (map inc src)) @c)
 (let [c (atom 0) src (repeatedly (fn [] (swap! c inc) 1))] (first (filter odd? src)) @c)
 (let [c (atom 0) src (repeatedly (fn [] (swap! c inc) 1))] (first (map + src src)) @c)]
;; expect: [1 1 1]
