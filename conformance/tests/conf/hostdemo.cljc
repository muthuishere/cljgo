;; Multi-host .cljc dependency namespace (ADR 0068): each Clojure host
;; reads its own reader-conditional branch (ADR 0036). Lives under
;; tests/conf/ — outside the harness glob — loaded only via require.
(ns conf.hostdemo)

(def platform
  #?(:clj "jvm" :cljs "cljs" :cljr "clr" :cljgo "cljgo" :default "other"))

(def fallback
  #?(:clj "jvm-only" :default "fallback"))

(defn tag [s] (str s "@" platform))
