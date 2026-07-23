;; Reader conditionals (#? selecting, #?@ splicing) — reader Phase 2,
;; systematically re-verified against the official Reader Conditionals
;; guide (clojure.org/guides/reader_conditionals), 2026-07-23.
;; cljgo's platform feature is :cljgo (ADR 0036's ratified feature set);
;; :default is the always-match fallback; the other hosts' features
;; :clj/:cljs/:cljr do NOT match — host-specific by design. cljgo
;; processes reader conditionals in normal file/REPL reading regardless
;; of source extension (no .cljc-only gate — deliberate divergence from
;; the JVM, ADR 0068 addendum; this very file is .clj and would be a
;; "Conditional read not allowed" reader error on the JVM).
;;
;; oracle: skip — the :cljgo feature is cljgo's own platform. JVM Clojure
;; always injects its :clj feature instead, so this exact selection is not
;; reproducible on the `clojure` CLI. Verified by analogy: the JVM oracle
;; run with :features #{:clj} on `#?(:clj ...)` mirrors cljgo's :cljgo
;; (feature present => its value; feature absent => :default or elision).
;; The no-match SELECTING case (ADR 0036, oracle-verified 2026-07-16):
;; (read-string {:read-cond :allow :features #{:clj}} "[1 #?(:cljs 2) 3]")
;; => [1 3] — a conditional with no matching feature and no :default
;; reads as NOTHING at all (not nil), like a #_ discard.
;; The guide's splicing shapes, oracle-verified 2026-07-23 with :clj on
;; JVM 1.12.5 ({:read-cond :allow}):
;;   (list #?@(:clj [5 6 7 8])) => (5 6 7 8)
;;   [#?@(:clj [])] => []   [#?@(:clj [:a])] => [:a]
;;   [#?@(:clj [:a :b])] => [:a :b]
[#?(:cljgo :yes :default :no)
 #?(:clj :j :default :d)
 #?(:cljs :s :default :d)
 #?(:cljr :r :default :d)
 [1 #?@(:cljgo [2 3]) 4]
 [1 #?@(:clj [2 3]) 4]
 [1 #?(:cljs 2) 3]
 [1 #?(:clj 2) 3]
 (list #?@(:cljgo [5 6 7 8]))
 [#?@(:cljgo [])]
 [#?@(:cljgo [:a])]
 [#?@(:cljgo [:a :b])]]
;; expect: [:yes :d :d :d [1 2 3 4] [1 4] [1 3] [1 3] (5 6 7 8) [] [:a] [:a :b]]
