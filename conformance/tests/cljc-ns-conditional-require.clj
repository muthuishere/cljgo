;; #? and #?@ inside an ns declaration's (:require ...) — the Reader
;; Conditionals guide's primary use case. conf/condns.cljc requires
;; conf.helper + clojure.string under :cljgo, while its :clj branches
;; name namespaces that don't exist here — they must NOT be attempted
;; (the reader elides them before the ns form is ever analyzed).
;;
;; oracle: skip — :cljgo is cljgo's platform feature (ADR 0036). JVM
;; mirror verified 2026-07-23 (clojure 1.12.5, {:read-cond :allow}):
;; (read-string ... "(ns x (:require #?(:clj [clojure.set :as s]
;; :cljs [missing.ns :as s])))") => (ns x (:require [clojure.set :as
;; s])), and identically for the #?@ wrapped-vector form — the
;; unselected branch vanishes from the ns form entirely.
(require 'conf.condns)
conf.condns/greeting
;; expect: "HI!"
