;; A reader conditional supplying the exception class in a catch clause
;; (Reader Conditionals guide: cross-host try/catch). The conditional is
;; resolved by the READER, before the analyzer ever sees the catch form.
;;
;; oracle: skip — :cljgo is cljgo's platform feature (ADR 0036). JVM
;; mirror verified 2026-07-23 (clojure 1.12.5, .cljc file):
;; (try (throw (ex-info "boom" {})) (catch #?(:clj Exception :cljs
;; :default) e :caught)) => :caught.
(try
  (throw (ex-info "boom" {}))
  (catch #?(:cljgo Exception :clj Exception) e
    [:caught (ex-message e)]))
;; expect: [:caught "boom"]
