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

;; let? — railway binding (ADR 0014 D5; cljgo extension, no JVM oracle).
;; Bindings evaluate left to right: a value satisfying err?/none? short-
;; circuits the WHOLE form to that value; an ok/just value binds its
;; UNWRAPPED payload; a plain value binds unchanged. A core macro over the
;; Result/Option builtins — no analyzer/emitter change, so both modes get
;; it identically. (M1 deviation: simple-symbol bindings only, matching
;; core `let`; destructuring-after-unwrap arrives with `let` destructuring.)
(defn -let?-expand [bindings body]
  (if (seq bindings)
    (let [name (first bindings)
          expr (second bindings)
          more (next (next bindings))]
      `(let [v# ~expr]
         (if (or (err? v#) (none? v#))
           v#
           (let [~name (if (or (ok? v#) (just? v#)) (unwrap v#) v#)]
             ~(-let?-expand more body)))))
    (cons 'do body)))

(defmacro let? [bindings & body]
  (-let?-expand bindings body))

;; ===========================================================================
;; Destructuring (design/03 §5) — faithful port of clojure.core/destructure.
;; Sequential [a b & rest :as all] via nth/nthnext; associative
;; {:keys/:strs/:syms [..]}, {local key}, :or defaults, :as, arbitrarily
;; nested. PURE macro expansion: let / loop / fn / defn route their
;; binding+param vectors through `destructure`, expanding to plain
;; let*/fn* over simple symbols — no analyzer/emitter change, so REPL and
;; AOT get it byte-identically. Behavior oracle-verified against JVM
;; Clojure 1.12.5 (conformance/tests/destructure-*.clj).
;; Precedence-safe: `destructure`, `nth`, `conj`, ... are real clojure.core
;; names; nothing here shadows or renames Clojure (CLAUDE.md precedence).

;; membership by = over a seq (stands in for `(some #{x} coll)`).
(defn -mem? [coll x]
  (if (seq coll)
    (if (= (first coll) x) true (-mem? (next coll) x))
    false))

;; append [sym (keyfn sym)] pairs for each sym in a :keys/:strs/:syms vec.
(defn -map-add [acc syms keyfn]
  (loop [ss (seq syms) acc acc]
    (if ss
      (recur (next ss) (conj acc (vector (first ss) (keyfn (first ss)))))
      acc)))

;; expand a map-binding map into a seq of [binding-form key-expr] pairs,
;; resolving :keys/:strs/:syms and dropping :as/:or.
(defn -map-entries [b]
  (loop [ks (seq (keys b)) acc []]
    (if ks
      (let [k (first ks)
            acc2 (cond
                   (= k :as)   acc
                   (= k :or)   acc
                   (= k :keys) (-map-add acc (get b k) (fn [s] (keyword (name s))))
                   (= k :strs) (-map-add acc (get b k) (fn [s] (name s)))
                   (= k :syms) (-map-add acc (get b k) (fn [s] (list 'quote s)))
                   :else       (conj acc (vector k (get b k))))]
        (recur (next ks) acc2))
      (seq acc))))

;; -pb: process one binding-form b against value-expr v, appending simple
;; [sym expr] pairs to the accumulator vector bvec. Self-recursive for
;; nested forms (vector-in-vector, map-in-vector, ...).
(defn -pb [bvec b v]
  (cond
    (symbol? b) (conj bvec b v)

    (vector? b)
    (let [gvec (gensym "vec__")
          gseq (gensym "seq__")
          gfirst (gensym "first__")
          has-rest (-mem? b '&)]
      (loop [ret (let [r (conj bvec gvec v)]
                   (if has-rest
                     (conj r gseq (list 'clojure.core/seq gvec))
                     r))
             n 0
             bs (seq b)
             seen-rest? false]
        (if bs
          (let [firstb (first bs)]
            (cond
              (= firstb '&)
              (recur (-pb ret (second bs) gseq) n (nnext bs) true)

              (= firstb :as)
              (-pb ret (second bs) gvec)

              :else
              (if seen-rest?
                (-illegal-argument "Unsupported binding form, only :as can follow & parameter")
                (recur (-pb (if has-rest
                              (conj ret gfirst (list 'clojure.core/first gseq)
                                    gseq (list 'clojure.core/next gseq))
                              ret)
                            firstb
                            (if has-rest gfirst (list 'clojure.core/nth gvec n nil)))
                       (inc n) (next bs) seen-rest?))))
          ret)))

    (map? b)
    (let [gmap (gensym "map__")
          defaults (get b :or)]
      (loop [ret (let [r (conj bvec gmap v)]
                   (if (get b :as) (conj r (get b :as) gmap) r))
             bes (-map-entries b)]
        (if (seq bes)
          (let [entry (first bes)
                bb (first entry)
                bk (second entry)
                is-id (ident? bb)
                local (if is-id (symbol nil (name bb)) bb)
                bv (if (and is-id (contains? defaults local))
                     (list 'clojure.core/get gmap bk (get defaults local))
                     (list 'clojure.core/get gmap bk))]
            (recur (if is-id (conj ret local bv) (-pb ret bb bv))
                   (next bes)))
          ret)))

    :else (-illegal-argument (str "Unsupported binding form: " b))))

;; partition the flat binding vector into a seq of [form init] pairs.
(defn -pairs [coll]
  (loop [c (seq coll) acc []]
    (if c
      (recur (nnext c) (conj acc (vector (first c) (second c))))
      (seq acc))))

(defn -all-simple? [pairs]
  (loop [ps pairs]
    (if (seq ps)
      (if (symbol? (first (first ps))) (recur (next ps)) false)
      true)))

;; destructure: [binding-vector] -> a plain let*-ready binding vector of
;; only simple symbols. Returns the input unchanged when already simple.
(defn destructure [bindings]
  (let [pairs (-pairs bindings)]
    (if (-all-simple? pairs)
      bindings
      (loop [ret [] ps pairs]
        (if (seq ps)
          (recur (-pb ret (first (first ps)) (second (first ps))) (next ps))
          ret)))))

;; fn/defn param destructuring: maybe-destructured turns a param vector
;; with destructuring forms into (simple-params (let [..] body)).
(defn -all-symbols? [coll]
  (loop [c (seq coll)]
    (if c
      (if (symbol? (first c)) (recur (next c)) false)
      true)))

(defn -maybe-destructured [params body]
  (if (-all-symbols? params)
    (cons params body)
    (loop [ps (seq params)
           new-params []
           lets []]
      (if ps
        (if (symbol? (first ps))
          (recur (next ps) (conj new-params (first ps)) lets)
          (let [gp (gensym "p__")]
            (recur (next ps) (conj new-params gp)
                   (conj (conj lets (first ps)) gp))))
        (list new-params
              (cons 'clojure.core/let (cons lets body)))))))

(defn -fn-method [sig]
  (-maybe-destructured (first sig) (next sig)))

;; --- Supersede the M1 thin pass-throughs: destructuring-aware let / loop /
;;     fn / defn. Each expands to plain let*/fn* over simple symbols. ---

;; oracle: (let [[a b] x] ...) destructures per clojure.core/destructure.
(defmacro let [bindings & body]
  (cons 'let* (cons (destructure bindings) body)))

;; helper (plain fn — the loop macro can't use `loop` while redefining it):
;; splits a destructured loop binding vector into [outer-let loop*-bindings
;; inner-redestructure-let], per clojure.core/loop.
(defn -loop-parts [bindings]
  (loop [bs (seq bindings)
         bfs []       ; outer let: g v (and g v form g for destructured)
         loopbs []    ; loop* bindings: g g ...
         innerbs []]  ; inner re-destructure: form g ...
    (if bs
      (let [b (first bs)
            v (second bs)
            g (if (symbol? b) b (gensym))]
        (recur (nnext bs)
               (if (symbol? b)
                 (conj bfs g v)
                 (conj (conj (conj (conj bfs g) v) b) g))
               (conj (conj loopbs g) g)
               (conj (conj innerbs b) g)))
      (vector bfs loopbs innerbs))))

;; oracle: loop with destructured bindings wraps loop* over gensyms and a
;; re-destructuring let (clojure.core/loop). recur targets the gensyms.
(defmacro loop [bindings & body]
  (let [db (destructure bindings)]
    (if (= db bindings)
      (cons 'loop* (cons bindings body))
      (let [parts (-loop-parts bindings)
            bfs (nth parts 0)
            loopbs (nth parts 1)
            innerbs (nth parts 2)]
        (list 'clojure.core/let bfs
              (list 'loop* loopbs
                    (cons 'clojure.core/let (cons innerbs body))))))))

;; oracle: (fn name? [params] body) | (fn name? ([params] body)+), each
;; param vector destructured via maybe-destructured (clojure.core/fn).
(defmacro fn [& sigs]
  (let [nm (if (symbol? (first sigs)) (first sigs) nil)
        sigs (if nm (next sigs) sigs)
        sigs (if (vector? (first sigs)) (list sigs) sigs)]
    (loop [ss (seq sigs) methods []]
      (if ss
        (recur (next ss) (conj methods (-fn-method (first ss))))
        (if nm
          (cons 'fn* (cons nm (seq methods)))
          (cons 'fn* (seq methods)))))))

;; oracle: (defn f [x] x) => (def f (fn ([x] x))) — fn now destructures.
(defmacro defn [name & fdecl]
  (if (string? (first fdecl))
    (list 'def name (first fdecl) (cons 'clojure.core/fn (next fdecl)))
    (list 'def name (cons 'clojure.core/fn fdecl))))
