;; Multi-host .cljc namespace whose ns declaration selects its :require
;; clauses with reader conditionals — the Reader Conditionals guide's
;; headline use case (ADR 0036/0068). The :clj branches name namespaces
;; that DO NOT EXIST here; loading succeeds only because cljgo never
;; attempts an unselected branch. Lives under tests/conf/ — outside the
;; harness glob — loaded only via require.
(ns conf.condns
  (:require #?(:cljgo [conf.helper :as h]
               :clj [not.a.real.jvm.ns :as h])
            #?@(:cljgo [[clojure.string :as cstr]]
                :clj [[another.missing.jvm.ns :as cstr]])))

(def greeting (h/exclaim (cstr/upper-case "hi")))
