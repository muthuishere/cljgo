;; Reader conditionals (#? selecting, #?@ splicing) — reader Phase 2.
;; cljgo's platform feature is :cljgo (never :clj); :default is the
;; always-match fallback. cljgo processes reader conditionals in normal
;; file/REPL reading (no :read-cond opt-in).
;;
;; oracle: skip — the :cljgo feature is cljgo's own platform. JVM Clojure
;; always injects its :clj feature instead, so this exact selection is not
;; reproducible on the `clojure` CLI. Verified by analogy: the JVM oracle
;; run with :features #{:clj} on `#?(:clj ...)` mirrors cljgo's :cljgo
;; (feature present => its value; feature absent => :default or elision).
;; The no-match SELECTING case (ADR 0036, oracle-verified 2026-07-16):
;; (read-string {:read-cond :allow :features #{:clj}} "[1 #?(:cljs 2) 3]")
;; => [1 3] — a conditional with no matching feature and no :default
;; reads as nothing at all, like a #_ discard.
[#?(:cljgo :yes :default :no)
 #?(:clj :j :default :d)
 [1 #?@(:cljgo [2 3]) 4]
 [1 #?@(:clj [2 3]) 4]
 [1 #?(:cljs 2) 3]]
;; expect: [:yes :d [1 2 3 4] [1 4] [1 3]]
