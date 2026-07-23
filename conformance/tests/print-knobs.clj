;; Printer knobs *print-level* / *print-meta* / *print-namespace-maps*
;; (fundamentals batch A2). All values are runtime-constructed (with-meta /
;; keyword literals), never reader-metadata-carrying quoted forms, so the
;; frozen output is host-independent.
;; oracle (clojure 1.12.5, `clojure -M` 2026-07-23):
;;   (binding [*print-level* 2] (pr-str [1 [2 [3 [4]]]])) => "[1 [2 #]]"
;;   (binding [*print-level* 2] (pr-str {:a {:b {:c 1}}})) => "{:a {:b #}}"
;;   (binding [*print-level* 0] (pr-str [1 2])) => "#"
;;   (binding [*print-level* 2] (pr-str '(1 (2 (3))))) => "(1 (2 #))"
;;   (binding [*print-level* 1] (pr-str #{[1]})) => "#{#}"
;;   (binding [*print-level* 1 *print-length* 2] (pr-str [1 [2] 3 4])) => "[1 # ...]"
;;   (binding [*print-meta* true] (pr-str (with-meta [1] {:a 1}))) => "^{:a 1} [1]"
;;   (binding [*print-meta* true] (print-str (with-meta [1] {:a 1}))) => "[1]"
;;   (binding [*print-meta* true] (pr-str {:k (with-meta [1] {:a 1})})) => "{:k ^{:a 1} [1]}"
;;   (binding [*print-meta* true] (pr-str (with-meta (symbol "x") {:tag (symbol "Long")}))) => "^Long x"
;;   (pr-str (with-meta (symbol "x") {})) => "x"  (empty meta never prints)
;;   (pr-str {:foo/a 1 :foo/b 2}) => "#:foo{:a 1, :b 2}"  (*print-namespace-maps* is true under clojure.main)
;;   (pr-str {:foo/a 1 :bar/b 2}) => "{:foo/a 1, :bar/b 2}"
;;   (pr-str {:foo/a 1 :b 2}) => "{:foo/a 1, :b 2}"
;;   (pr-str {(symbol "foo/a") 1}) => "#:foo{a 1}"
;;   (binding [*print-namespace-maps* false] (pr-str {:foo/a 1})) => "{:foo/a 1}"
;;   (pr-str {:foo/a {:bar/b 1}}) => "#:foo{:a #:bar{:b 1}}"
;;   defaults: *print-level* => nil, *print-meta* => false, *print-namespace-maps* => true
[(binding [*print-level* 2] (pr-str [1 [2 [3 [4]]]]))
 (binding [*print-level* 2] (pr-str {:a {:b {:c 1}}}))
 (binding [*print-level* 0] (pr-str [1 2]))
 (binding [*print-level* 2] (pr-str '(1 (2 (3)))))
 (binding [*print-level* 1] (pr-str #{[1]}))
 (binding [*print-level* 1 *print-length* 2] (pr-str [1 [2] 3 4]))
 (binding [*print-meta* true] (pr-str (with-meta [1] {:a 1})))
 (binding [*print-meta* true] (print-str (with-meta [1] {:a 1})))
 (binding [*print-meta* true] (pr-str {:k (with-meta [1] {:a 1})}))
 (binding [*print-meta* true] (pr-str (with-meta (symbol "x") {:tag (symbol "Long")})))
 (pr-str (with-meta (symbol "x") {}))
 (pr-str {:foo/a 1 :foo/b 2})
 (pr-str {:foo/a 1 :bar/b 2})
 (pr-str {:foo/a 1 :b 2})
 (pr-str {(symbol "foo/a") 1})
 (binding [*print-namespace-maps* false] (pr-str {:foo/a 1}))
 (pr-str {:foo/a {:bar/b 1}})
 *print-level*
 *print-meta*
 *print-namespace-maps*]
;; expect: ["[1 [2 #]]" "{:a {:b #}}" "#" "(1 (2 #))" "#{#}" "[1 # ...]" "^{:a 1} [1]" "[1]" "{:k ^{:a 1} [1]}" "^Long x" "x" "#:foo{:a 1, :b 2}" "{:foo/a 1, :bar/b 2}" "{:foo/a 1, :b 2}" "#:foo{a 1}" "{:foo/a 1}" "#:foo{:a #:bar{:b 1}}" nil false true]
