;; core.clj — cljgo's clojure.core bootstrap (M1, eval v2; design/00 §6,
;; design/03 §4/§8-v2). Loaded at startup into the clojure.core
;; namespace by pkg/eval, AFTER the Go builtins (list, cons, first,
;; next, concat, apply, not, ...) and the hand-built `defmacro` are
;; interned — every fn used below at expansion time is a Go builtin or
;; defined earlier in this file.
;;
;; Every macro's expansion/behavior is oracle-verified against JVM
;; Clojure 1.12.5 (macroexpand-1 unless noted); the oracle output is
;; cited on each form. Deliberate M1 deviations are marked DEVIATION.

;; fn / let / loop pass straight through to the starred specials.
;; DEVIATION: destructuring is deferred (design/03 §5 makes it core's
;; job; M1 ships simple-symbol bindings only) — on JVM Clojure these
;; three also expand destructuring; the pass-through matches the
;; no-destructuring case:
;;   oracle: (macroexpand-1 '(let [x 1] x))    => (let* [x 1] x)
;;   oracle: (macroexpand-1 '(loop [x 1] x))   => (loop* [x 1] x)
;;   oracle: (macroexpand-1 '(fn [x] x))       => (fn* ([x] x))
;;     (ours yields (fn* [x] x); fn* normalizes the single-method
;;     shorthand to the same methods, design/03 §5)
(defmacro fn [& decl] (cons 'fn* decl))
(defmacro let [bindings & body] (cons 'let* (cons bindings body)))
(defmacro loop [bindings & body] (cons 'loop* (cons bindings body)))

;; oracle: (macroexpand-1 '(defn f [x] x)) => (def f (clojure.core/fn ([x] x)))
;; (ours emits fn* directly — same result once fn passes through; the
;; docstring lands in def's 3-arg doc slot, as JVM def does. DEVIATION:
;; no attr-map / :arglists metadata yet.)
(defmacro defn [name & fdecl]
  (if (string? (first fdecl))
    (list 'def name (first fdecl) (cons 'fn* (next fdecl)))
    (list 'def name (cons 'fn* fdecl))))

;; oracle: (macroexpand-1 '(when a b c)) => (if a (do b c))
(defmacro when [test & body]
  (list 'if test (cons 'do body)))

;; oracle: (macroexpand-1 '(when-not a b c)) => (if a nil (do b c))
(defmacro when-not [test & body]
  (list 'if test nil (cons 'do body)))

;; oracle: (macroexpand-1 '(if-not a b))   => (clojure.core/if-not a b nil)
;; oracle: (macroexpand-1 '(if-not a b c)) => (if (clojure.core/not a) b c)
(defmacro if-not
  ([test then] `(if-not ~test ~then nil))
  ([test then else] `(if (not ~test) ~then ~else)))

;; oracle: (macroexpand-1 '(and))     => true
;; oracle: (macroexpand-1 '(and a))   => a
;; oracle: (macroexpand-1 '(and a b)) =>
;;   (clojure.core/let [and__5600__auto__ a]
;;     (if and__5600__auto__ (clojure.core/and b) and__5600__auto__))
(defmacro and
  ([] true)
  ([x] x)
  ([x & next]
   `(let [and# ~x]
      (if and# (and ~@next) and#))))

;; oracle: (macroexpand-1 '(or))      => nil
;; oracle: (macroexpand-1 '(or a))    => a
;; oracle: (macroexpand-1 '(or a b))  =>
;;   (clojure.core/let [or__5602__auto__ a]
;;     (if or__5602__auto__ or__5602__auto__ (clojure.core/or b)))
(defmacro or
  ([] nil)
  ([x] x)
  ([x & next]
   `(let [or# ~x]
      (if or# or# (or ~@next)))))

;; oracle: (macroexpand-1 '(-> 5 (+ 3) (* 2))) => (* (+ 5 3) 2)
;; oracle: (macroexpand-1 '(-> x inc))         => (inc x)
;; oracle: (-> 5 (+ 3) (* 2))                  => 16
(defmacro -> [x & forms]
  (loop [x x, forms forms]
    (if forms
      (let [form (first forms)
            threaded (if (seq? form)
                       (with-meta `(~(first form) ~x ~@(next form)) (meta form))
                       (list form x))]
        (recur threaded (next forms)))
      x)))

;; oracle: (macroexpand-1 '(->> 5 (- 20) (* 2))) => (* 2 (- 20 5))
;; oracle: (->> 5 (- 20))                        => 15
(defmacro ->> [x & forms]
  (loop [x x, forms forms]
    (if forms
      (let [form (first forms)
            threaded (if (seq? form)
                       (with-meta `(~(first form) ~@(next form) ~x) (meta form))
                       (list form x))]
        (recur threaded (next forms)))
      x)))

;; oracle: (macroexpand-1 '(cond a 1 b 2)) => (if a 1 (clojure.core/cond b 2))
;; oracle: (macroexpand-1 '(cond))         => nil
;; oracle: (macroexpand-1 '(cond a))       => Syntax error macroexpanding cond
;;   ("cond requires an even number of forms" — thrown WHILE expanding,
;;    as on JVM; -illegal-argument is our stand-in for `throw` until v3)
(defmacro cond [& clauses]
  (when clauses
    (list 'if (first clauses)
          (if (next clauses)
            (second clauses)
            (-illegal-argument "cond requires an even number of forms"))
          (cons 'clojure.core/cond (next (next clauses))))))
