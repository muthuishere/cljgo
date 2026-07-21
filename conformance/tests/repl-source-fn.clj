;; clojure.repl/source-fn + source — a DOCUMENTED DEVIATION, frozen so the
;; gap stays visible instead of drifting quietly.
;;
;; On the JVM, source-fn re-reads a var's :file/:line metadata off the
;; classpath and hands back its source text; (source-fn 'clojure.core/when)
;; => a string, verified on clojure 1.12.5 (2026-07-21). cljgo retains no
;; source text — core.clj is embedded and compiled, and vars carry no
;; :file/:line pointing at a readable classpath entry — so source-fn
;; returns nil for EVERY var, exactly as the JVM does for a var it cannot
;; locate. That "nil when the source is not findable" branch IS upstream
;; behavior; what deviates is how often we take it (always).
;;
;; Frozen deliberately: when source retention lands, this file fails and
;; forces the update, rather than the gap going unnoticed.
;; oracle: skip — the JVM answers `true` for the first form (source text
;; found); cljgo answers `false`. The nil-for-unknown-var contract below is
;; oracle-identical.
(require '[clojure.repl :as r])
[(some? (r/source-fn 'clojure.core/when))
 (nil? (r/source-fn 'no.such/var))]
;; expect: [false true]
