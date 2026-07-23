;; clojure.core/read-string's reader-conditional opts protocol (Reader
;; Conditionals guide + ADR 0050). JVM parity: a bare read-string
;; REFUSES conditionals ("Conditional read not allowed", R1011);
;; {:read-cond :allow} selects (:cljgo/:default on this platform);
;; {:features #{...}} ADDS selectable features on top of the
;; always-present platform feature (the JVM keeps :clj alongside
;; explicit :features — cljgo mirrors with :cljgo); {:read-cond
;; :preserve} reads the conditional as ONE ReaderConditional data value,
;; tagged literals inside preserved as tagged-literal values, and
;; top-level #?@ allowed (one value, nothing to splice); :allow still
;; rejects top-level #?@; an unrecognized :read-cond behaves as
;; not-allowed.
;;
;; oracle: skip — branch keys use :cljgo (ADR 0036), which the JVM
;; elides. JVM mirror verified 2026-07-23 (clojure 1.12.5), :clj for
;; :cljgo throughout:
;;   (read-string "#?(:clj 1)") throws "Conditional read not allowed"
;;   (read-string {:read-cond :allow} "[1 #?(:cljgo 2) 3]") => [1 3]
;;   (read-string {:read-cond :allow :features #{:cljs}} "#?(:cljs 1 :clj 2)") => 1
;;   (read-string {:read-cond :allow :features #{:cljs}} "#?(:clj 2)") => 2
;;   (read-string {:read-cond :allow :features #{:cljr}} "#?(:cljr 1 :default 9)") => 1
;;   (pr-str (read-string {:read-cond :preserve} "#?(:clj foo/bar :cljs bar/foo)"))
;;     => "#?(:clj foo/bar :cljs bar/foo)"; reader-conditional? true,
;;     :form (:clj foo/bar :cljs bar/foo), :splicing? false
;;   (pr-str (read-string {:read-cond :preserve} "[1 #?@(:clj [2 3]) 4]"))
;;     => "[1 #?@(:clj [2 3]) 4]"
;;   (pr-str (read-string {:read-cond :preserve} "#?@(:clj [1 2])")) => "#?@(:clj [1 2])"
;;   (map tagged-literal? (:form (read-string {:read-cond :preserve}
;;     "#?(:clj #inst \"2020-01-01T00:00:00Z\")"))) => (false true)
;;   (read-string {:read-cond :allow} "#?@(:clj [1 2])") throws
;;     "Reader conditional splicing not allowed at the top level."
;;   (read-string {:read-cond :bogus} "#?(:clj 1)") throws "Conditional read not allowed"
(require '[clojure.string :as str])
(let [msg (fn [f] (try (f) (catch Exception e (ex-message e))))]
  [(str/includes? (msg #(read-string "#?(:cljgo 1)")) "Conditional read not allowed")
   (read-string {:read-cond :allow} "#?(:cljgo 1)")
   (read-string {:read-cond :allow} "[1 #?(:cljs 2) 3]")
   (read-string {:read-cond :allow :features #{:clj}} "#?(:clj 1 :cljgo 2)")
   (read-string {:read-cond :allow :features #{:cljs}} "#?(:cljgo 2)")
   (read-string {:read-cond :allow :features #{:cljr}} "#?(:cljr 1 :default 9)")
   (pr-str (read-string {:read-cond :preserve} "#?(:clj foo/bar :cljs bar/foo)"))
   (let [v (read-string {:read-cond :preserve} "#?(:clj foo)")]
     [(reader-conditional? v) (:form v) (:splicing? v)])
   (pr-str (read-string {:read-cond :preserve} "[1 #?@(:clj [2 3]) 4]"))
   (pr-str (read-string {:read-cond :preserve} "#?@(:clj [1 2])"))
   (map tagged-literal? (:form (read-string {:read-cond :preserve}
                                            "#?(:clj #inst \"2020-01-01T00:00:00Z\")")))
   (str/includes? (msg #(read-string {:read-cond :allow} "#?@(:clj [1 2])"))
                  "Reader conditional splicing not allowed at the top level.")
   (str/includes? (msg #(read-string {:read-cond :bogus} "#?(:cljgo 1)"))
                  "Conditional read not allowed")])
;; expect: [true 1 [1 3] 1 2 1 "#?(:clj foo/bar :cljs bar/foo)" [true (:clj foo) false] "[1 #?@(:clj [2 3]) 4]" "#?@(:clj [1 2])" (false true) true true]
