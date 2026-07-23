;; Batch A3: the Inst protocol (1.9) over cljgo's #inst type — reader.Inst
;; (pkg/reader/tagged.go, ADR 0050), standing in for java.util.Date.
;; inst-ms* is the protocol method, inst-ms delegates, inst? is
;; satisfies?-based (false for non-inst values and nil).
;; Oracle (clojure 1.12.5): verified 2026-07-23.
[(inst? #inst "2020-01-01")
 (inst? 5)
 (inst? nil)
 (inst? "2020-01-01")
 (inst-ms #inst "2020-01-01")
 (inst-ms* #inst "1970-01-01T00:00:01Z")
 (inst-ms #inst "1970-01-01T00:00:00Z")]
;; expect: [true false false false 1577836800000 1000 0]
