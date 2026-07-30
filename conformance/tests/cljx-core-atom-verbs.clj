;; cljx.core (ADR 0106): the ergonomic atom verbs are TRANSPARENT ALIASES —
;; each one must leave the atom in exactly the state the swap!/reset! form
;; named in its docstring would. Every pair below runs the sugar on one atom
;; and the raw form on another, then compares: `true` means the alias is
;; honest. Also covers bump!-from-absent (the (fnil inc 0) case), del! on a
;; set (disj) vs a map (dissoc), and clear! preserving the collection type.
;;
;; oracle: n/a — cljx.core is a cljgo namespace with no JVM package, so this
;; file cannot run through the `clojure` CLI verbatim. What it freezes is
;; EQUIVALENCE to clojure.core forms that are themselves oracle-verified, so
;; the assertions are self-checking. REPL-vs-binary parity is enforced by the
;; dual harness.
(require '[cljx.core :as x])
(let [;; add! on a vector == swap! conj
      a1 (atom []) b1 (atom [])
      _ (do (x/add! a1 "one") (x/add! a1 "two")
            (swap! b1 conj "one") (swap! b1 conj "two"))
      ;; add! on a map == swap! assoc
      a2 (atom {}) b2 (atom {})
      _ (do (x/add! a2 :k "v") (swap! b2 assoc :k "v"))
      ;; bump! == swap! inc
      a3 (atom 0) b3 (atom 0)
      _ (do (x/bump! a3) (x/bump! a3) (swap! b3 inc) (swap! b3 inc))
      ;; bump! by key from ABSENT == swap! update (fnil inc 0)
      a4 (atom {}) b4 (atom {})
      _ (do (x/bump! a4 "harsh") (x/bump! a4 "harsh") (x/bump! a4 "shaama")
            (swap! b4 update "harsh" (fnil inc 0))
            (swap! b4 update "harsh" (fnil inc 0))
            (swap! b4 update "shaama" (fnil inc 0)))
      ;; bump! by n
      a5 (atom {}) b5 (atom {})
      _ (do (x/bump! a5 :hits 5) (swap! b5 update :hits (fnil + 0) 5))
      ;; upd! == swap! update
      a6 (atom {:n 2}) b6 (atom {:n 2})
      _ (do (x/upd! a6 :n * 10) (swap! b6 update :n * 10))
      ;; put-in! == swap! assoc-in ; upd-in! == swap! update-in
      a7 (atom {}) b7 (atom {})
      _ (do (x/put-in! a7 [:a :b] 1) (x/upd-in! a7 [:a :b] inc)
            (swap! b7 assoc-in [:a :b] 1) (swap! b7 update-in [:a :b] inc))
      ;; del! on a map == dissoc ; on a set == disj
      a8 (atom {:x 1 :y 2}) b8 (atom {:x 1 :y 2})
      _ (do (x/del! a8 :x) (swap! b8 dissoc :x))
      a9 (atom #{:p :q}) b9 (atom #{:p :q})
      _ (do (x/del! a9 :p) (swap! b9 disj :p))
      ;; toggle! == swap! not
      a10 (atom false) b10 (atom false)
      _ (do (x/toggle! a10) (swap! b10 not))
      ;; clear! == reset! to empty, KEEPING the collection type
      a11 (atom [1 2 3])
      _ (x/clear! a11)
      a12 (atom {:k 1})
      _ (x/clear! a12)]
  [(= @a1 @b1) (= @a2 @b2) (= @a3 @b3) (= @a4 @b4) (= @a5 @b5) (= @a6 @b6)
   (= @a7 @b7) (= @a8 @b8) (= @a9 @b9) (= @a10 @b10)
   @a4
   (and (empty? @a11) (vector? @a11))
   (and (empty? @a12) (map? @a12))])
;; expect: [true true true true true true true true true true {"harsh" 2, "shaama" 1} true true]
