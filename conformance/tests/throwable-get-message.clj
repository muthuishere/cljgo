;; java.lang.Throwable's accessor trio works on the WHOLE host-error family —
;; .getMessage / .getLocalizedMessage / .getCause — not just ex-info. This is
;; the first thing a JVM-trained user writes inside a catch block, and it must
;; agree with ex-message / ex-cause by construction.
;; Oracle (real clojure CLI 1.12.5, `clojure -M o6.clj`, 2026-07-28), one line
;; per element below:
;;   Divide by zero / "/ by zero" / boom / boom / inner / nil / true / true
;; expect: ["Divide by zero" "/ by zero" "boom" "boom" "inner" nil true true]
[(try (/ 1 0) (catch Throwable t (.getMessage t)))
 (try (quot 1 0) (catch Throwable t (.getMessage t)))
 (try (throw (ex-info "boom" {:a 1})) (catch Throwable t (.getMessage t)))
 (try (throw (ex-info "boom" {:a 1})) (catch Throwable t (.getLocalizedMessage t)))
 (try (throw (ex-info "outer" {} (ex-info "inner" {})))
      (catch Throwable t (.getMessage (.getCause t))))
 (try (throw (ex-info "outer" {})) (catch Throwable t (.getCause t)))
 (try (throw (ex-info "boom" {:a 1}))
      (catch Throwable t (= (.getMessage t) (ex-message t))))
 (try (/ 1 0) (catch Throwable t (= (.getMessage t) (ex-message t))))]
