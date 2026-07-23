;; requiring-resolve (fundamentals batch A1): resolve a QUALIFIED symbol,
;; requiring its namespace first when it does not resolve yet; nil when
;; the namespace loads but the var is absent; an unqualified symbol
;; throws "Not a qualified symbol: <sym>" (core/core.clj — clojure.core's
;; shape with the qualified check inlined).
;; oracle (clojure 1.12.5, 2026-07-23): [true nil "Not a qualified symbol: trim"]
[(some? (requiring-resolve 'clojure.string/trim))
 (requiring-resolve 'clojure.string/nope-not-here)
 (try (requiring-resolve 'trim) (catch Exception e (ex-message e)))]
;; expect: [true nil "Not a qualified symbol: trim"]
