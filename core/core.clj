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

;; ===========================================================================
;; Sequence & collection library (clojure.core). Standard Clojure — every fn
;; matches JVM Clojure 1.12.5 exactly (no renames; CLAUDE.md precedence
;; principle). LAZINESS: map/filter/take/… are lazy, built on the `lazy-seq`
;; macro over the `lazy-seq*` host primitive (pkg/eval/coll_builtins.go), which
;; wraps a lang.LazySeq — the faithful Clojure model. Producers range/repeat/
;; iterate/cycle are native lang seqs (also lazy). Runtime primitives that need
;; host support (lazy-seq*, the producers, sort/sort-by/dissoc/vec/vals,
;; reduced, <=/>=/quot/rem/max/min, zero?/pos?/neg?/nil?/some?/true?/false?)
;; live in coll_builtins.go; everything else is defined here in Clojure over
;; first/next/seq/cons/reduce. Behavior oracle-verified against the `clojure`
;; CLI (conformance/tests/seq-*.clj, coll-*.clj).

;; list* : prepend leading args onto a trailing seq (clojure.core).
(defn list*
  ([args] (seq args))
  ([a args] (cons a args))
  ([a b args] (cons a (cons b args)))
  ([a b c args] (cons a (cons b (cons c args))))
  ([a b c d & more] (cons a (cons b (cons c (cons d (apply list* more)))))))

;; lazy-seq : wrap a body as a lazily-realized seq. Expands to a 0-arg thunk
;; handed to the lazy-seq* host primitive.
;; oracle: (macroexpand-1 '(lazy-seq a)) => (clojure.core/lazy-seq* (fn* [] a))-shape
(defmacro lazy-seq [& body]
  (list 'lazy-seq* (cons 'fn* (cons [] body))))

;; if-let / when-let : bind + branch on the truthiness of a single value.
;; oracle: (if-let [x v] a b) tests v, binds x=v in the taken branch.
(defmacro if-let
  ([bindings then] `(if-let ~bindings ~then nil))
  ([bindings then else]
   (let [form (first bindings) tst (second bindings)]
     `(let [temp# ~tst]
        (if temp# (let [~form temp#] ~then) ~else)))))

;; oracle: (macroexpand '(when-let [x (range 5) y (range 5)])) throws
;; IllegalArgumentException ("when-let requires exactly 1 binding form") —
;; real Clojure's when-let accepts exactly one binding pair, same as if-let.
(defmacro when-let [bindings & body]
  (when-not (= 2 (count bindings))
    (throw (ex-info "when-let requires exactly 1 binding form" {:bindings bindings})))
  (let [form (first bindings) tst (second bindings)]
    `(let [temp# ~tst]
       (if temp# (let [~form temp#] ~@body) nil))))

;; --- Core higher-order fns ------------------------------------------------

;; oracle: (map inc [1 2 3]) => (2 3 4); (map + [1 2 3] [10 20 30]) => (11 22 33)
(defn map
  ([f coll]
   (lazy-seq
    (when-let [s (seq coll)]
      (cons (f (first s)) (map f (rest s))))))
  ([f c1 c2]
   (lazy-seq
    (let [s1 (seq c1) s2 (seq c2)]
      (when (and s1 s2)
        (cons (f (first s1) (first s2))
              (map f (rest s1) (rest s2))))))))

;; oracle: (filter even? (range 10)) => (0 2 4 6 8)
(defn filter [pred coll]
  (lazy-seq
   (when-let [s (seq coll)]
     (let [x (first s)]
       (if (pred x)
         (cons x (filter pred (rest s)))
         (filter pred (rest s)))))))

;; oracle: (remove even? (range 10)) => (1 3 5 7 9)
(defn remove [pred coll]
  (filter (fn [x] (not (pred x))) coll))

;; oracle: (reduce + 0 (range 1 11)) => 55; (reduce + (range 1 11)) => 55.
;; Honors the `reduced` short-circuit box.
(defn reduce
  ([f coll]
   (let [s (seq coll)]
     (if s (reduce f (first s) (rest s)) (f))))
  ([f val coll]
   (loop [acc val s (seq coll)]
     (if s
       (let [ret (f acc (first s))]
         (if (reduced? ret) (deref ret) (recur ret (next s))))
       acc))))

;; oracle: (reduce-kv (fn [a k v] (+ a v)) 0 {:a 1 :b 2 :c 3}) => 6
(defn reduce-kv [f init coll]
  (reduce (fn [acc k] (f acc k (get coll k))) init (keys coll)))

;; oracle: (keep #(when (even? %) %) (range 6)) => (0 2 4)
(defn keep [f coll]
  (lazy-seq
   (when-let [s (seq coll)]
     (let [x (f (first s))]
       (if (nil? x)
         (keep f (rest s))
         (cons x (keep f (rest s))))))))

;; oracle: (mapcat (fn [x] [x x]) [1 2 3]) => (1 1 2 2 3 3)
(defn mapcat [f & colls]
  (apply concat (apply map f colls)))

;; oracle: (mapv inc [1 2 3]) => [2 3 4]
(defn mapv
  ([f coll] (vec (map f coll)))
  ([f c1 c2] (vec (map f c1 c2))))

;; oracle: (filterv even? (range 10)) => [0 2 4 6 8]
(defn filterv [pred coll] (vec (filter pred coll)))

;; oracle: (run! println [1 2]) prints, returns nil.
(defn run! [proc coll]
  (reduce (fn [_ x] (proc x) nil) nil coll)
  nil)

;; --- Take / drop ----------------------------------------------------------

;; oracle: (take 3 (range)) => (0 1 2); (take 3 (range 10)) => (0 1 2)
(defn take [n coll]
  (lazy-seq
   (when (pos? n)
     (when-let [s (seq coll)]
       (cons (first s) (take (dec n) (rest s)))))))

;; oracle: (drop 2 [1 2 3 4]) => (3 4)
(defn drop [n coll]
  (loop [n n s (seq coll)]
    (if (and (pos? n) s)
      (recur (dec n) (next s))
      s)))

;; oracle: (take-while #(< % 3) (range 10)) => (0 1 2)
(defn take-while [pred coll]
  (lazy-seq
   (when-let [s (seq coll)]
     (when (pred (first s))
       (cons (first s) (take-while pred (rest s)))))))

;; oracle: (drop-while #(< % 3) (range 10)) => (3 4 5 6 7 8 9)
(defn drop-while [pred coll]
  (loop [s (seq coll)]
    (if (and s (pred (first s)))
      (recur (next s))
      s)))

;; oracle: (take-nth 2 (range 10)) => (0 2 4 6 8)
(defn take-nth [n coll]
  (lazy-seq
   (when-let [s (seq coll)]
     (cons (first s) (take-nth n (drop n s))))))

;; oracle: (partition 2 (range 6)) => ((0 1) (2 3) (4 5))
(defn partition [n coll]
  (lazy-seq
   (let [s (seq coll)]
     (when s
       (let [p (take n s)]
         (when (= n (count p))
           (cons p (partition n (drop n s)))))))))

;; oracle: (partition-all 2 (range 5)) => ((0 1) (2 3) (4))
(defn partition-all [n coll]
  (lazy-seq
   (let [s (seq coll)]
     (when s
       (cons (take n s) (partition-all n (drop n s)))))))

;; oracle: (split-at 2 [1 2 3 4 5]) => [(1 2) (3 4 5)]
(defn split-at [n coll]
  [(take n coll) (drop n coll)])

;; --- Producers (core.clj half; the lazy natives are host builtins) --------

;; oracle: (take 3 (repeatedly (fn [] 1))) => (1 1 1)
(defn repeatedly
  ([f] (lazy-seq (cons (f) (repeatedly f))))
  ([n f] (take n (repeatedly f))))

;; --- Collection ops -------------------------------------------------------

;; oracle: (into [] (range 3)) => [0 1 2]; (into {} [[:a 1] [:b 2]]) => {:a 1, :b 2}
(defn into [to from]
  (reduce conj to from))

;; oracle: (reverse [1 2 3]) => (3 2 1)
(defn reverse [coll]
  (reduce conj () coll))

;; sequential? : true for lists/seqs/vectors (used by flatten). Approximates
;; clojure.core/sequential? over the collections cljgo currently ships.
(defn sequential? [x]
  (if (vector? x) true (seq? x)))

;; oracle: (flatten [1 [2 [3 [4]]]]) => (1 2 3 4)
(defn flatten [x]
  (lazy-seq
   (when-let [s (seq x)]
     (let [f (first s)]
       (if (sequential? f)
         (concat (flatten f) (flatten (rest s)))
         (cons f (flatten (rest s))))))))

;; oracle: (distinct [1 1 2 3 3 3 4]) => (1 2 3 4)
(defn -distinct-step [xs seen]
  (lazy-seq
   (loop [xs xs seen seen]
     (when-let [s (seq xs)]
       (let [f (first s)]
         (if (contains? seen f)
           (recur (rest s) seen)
           (cons f (-distinct-step (rest s) (conj seen f)))))))))

(defn distinct [coll] (-distinct-step coll #{}))

;; oracle: (interpose 0 [1 2 3]) => (1 0 2 0 3)
(defn interpose [sep coll]
  (drop 1 (mapcat (fn [x] (list sep x)) coll)))

;; oracle: (interleave [1 2 3] [:a :b :c]) => (1 :a 2 :b 3 :c)
(defn interleave [c1 c2]
  (lazy-seq
   (let [s1 (seq c1) s2 (seq c2)]
     (when (and s1 s2)
       (cons (first s1)
             (cons (first s2)
                   (interleave (rest s1) (rest s2))))))))

;; oracle: (frequencies [1 1 2 3 3 3]) => {1 2, 2 1, 3 3}
(defn frequencies [coll]
  (reduce (fn [m x] (assoc m x (inc (get m x 0)))) {} coll))

;; oracle: (group-by even? (range 6)) => {true [0 2 4], false [1 3 5]}
(defn group-by [f coll]
  (reduce (fn [ret x]
            (let [k (f x)]
              (assoc ret k (conj (get ret k []) x))))
          {} coll))

;; oracle: (zipmap [:a :b :c] [1 2 3]) => {:a 1, :b 2, :c 3}
(defn zipmap [keys vals]
  (loop [m {} ks (seq keys) vs (seq vals)]
    (if (and ks vs)
      (recur (assoc m (first ks) (first vs)) (next ks) (next vs))
      m)))

;; oracle: (merge {:a 1} {:b 2} {:a 3}) => {:a 3, :b 2}
(defn merge [& maps]
  (reduce (fn [a b]
            (if (nil? b) a
                (reduce (fn [m k] (assoc m k (get b k))) (if (nil? a) {} a) (keys b))))
          nil maps))

;; oracle: (merge-with + {:a 1 :b 2} {:a 10}) => {:a 11, :b 2}
(defn merge-with [f & maps]
  (reduce (fn [a b]
            (if (nil? b) a
                (reduce (fn [m k]
                          (let [v (get b k)]
                            (if (contains? m k)
                              (assoc m k (f (get m k) v))
                              (assoc m k v))))
                        (if (nil? a) {} a) (keys b))))
          nil maps))

;; oracle: (select-keys {:a 1 :b 2 :c 3} [:a :c]) => {:a 1, :c 3}
(defn select-keys [m ks]
  (reduce (fn [acc k] (if (contains? m k) (assoc acc k (get m k)) acc)) {} ks))

;; oracle: (get-in {:a {:b 5}} [:a :b]) => 5; (get-in m ks nf) with a missing
;; key returns nf.
(defn get-in
  ([m ks] (reduce get m ks))
  ([m ks not-found]
   (loop [m m ks (seq ks)]
     (if ks
       (if (contains? m (first ks))
         (recur (get m (first ks)) (next ks))
         not-found)
       m))))

;; oracle: (assoc-in {:a {:b 1}} [:a :c] 9) => {:a {:b 1, :c 9}}
(defn assoc-in [m [k & ks] v]
  (if ks
    (assoc m k (assoc-in (get m k) ks v))
    (assoc m k v)))

;; oracle: (update {:a 1} :a inc) => {:a 2}
(defn update
  ([m k f] (assoc m k (f (get m k))))
  ([m k f x] (assoc m k (f (get m k) x)))
  ([m k f x y] (assoc m k (f (get m k) x y)))
  ([m k f x y & more] (assoc m k (apply f (get m k) x y more))))

;; oracle: (update-in {:a {:b 1}} [:a :b] inc) => {:a {:b 2}}
(defn update-in [m [k & ks] f & args]
  (if ks
    (assoc m k (apply update-in (get m k) ks f args))
    (assoc m k (apply f (get m k) args))))

;; oracle: (empty? []) => true; (empty? [1]) => false
(defn empty? [coll] (not (seq coll)))

;; oracle: (not-empty [1 2]) => [1 2]; (not-empty []) => nil
(defn not-empty [coll] (if (seq coll) coll nil))

;; --- Numeric predicates & mod (tower primitives are host builtins) --------

;; oracle: (even? 4) => true; (odd? 3) => true
(defn even? [n] (zero? (rem n 2)))
(defn odd? [n] (not (even? n)))

;; oracle: (mod 7 3) => 1; (mod -7 3) => 2
(defn mod [num div]
  (let [m (rem num div)]
    (if (if (zero? m) true (= (pos? num) (pos? div)))
      m
      (+ m div))))

;; --- Function combinators -------------------------------------------------

;; oracle: (identity 7) => 7
(defn identity [x] x)

;; oracle: ((constantly 42) 1 2 3) => 42
(defn constantly [x] (fn [& _] x))

;; oracle: ((comp inc inc) 5) => 7
(defn comp
  ([] identity)
  ([f] f)
  ([f g] (fn [& args] (f (apply g args))))
  ([f g & fs] (reduce comp (list* f g fs))))

;; oracle: ((partial + 10) 5) => 15
(defn partial
  ([f] f)
  ([f a] (fn [& args] (apply f a args)))
  ([f a b] (fn [& args] (apply f a b args)))
  ([f a b c] (fn [& args] (apply f a b c args)))
  ([f a b c & more] (fn [& args] (apply f a b c (concat more args)))))

;; oracle: ((complement even?) 3) => true
(defn complement [f]
  (fn [& args] (not (apply f args))))

;; oracle: ((fnil inc 0) nil) => 1
(defn fnil
  ([f a] (fn [x & args] (apply f (if (nil? x) a x) args)))
  ([f a b] (fn [x y & args] (apply f (if (nil? x) a x) (if (nil? y) b y) args))))

;; oracle: ((juxt inc dec) 5) => [6 4]
(defn juxt
  ([f] (fn [& args] [(apply f args)]))
  ([f g] (fn [& args] [(apply f args) (apply g args)]))
  ([f g h] (fn [& args] [(apply f args) (apply g args) (apply h args)]))
  ([f g h & fs]
   (fn [& args]
     (reduce (fn [v p] (conj v (apply p args))) [] (list* f g h fs)))))

;; oracle: ((every-pred even? pos?) 4) => true
(defn every-pred [& preds]
  (fn [& args]
    (loop [ps (seq preds)]
      (if ps
        (if (loop [as (seq args)]
              (if as
                (if ((first ps) (first as)) (recur (next as)) false)
                true))
          (recur (next ps))
          false)
        true))))

;; oracle: ((some-fn even? neg?) 3) => false
(defn some-fn [& preds]
  (fn [& args]
    (loop [ps (seq preds)]
      (if ps
        (let [r (loop [as (seq args)]
                  (if as
                    (or ((first ps) (first as)) (recur (next as)))
                    false))]
          (if r r (recur (next ps))))
        false))))

;; --- Predicates / reducers ------------------------------------------------

;; oracle: (every? even? [2 4 6]) => true
(defn every? [pred coll]
  (loop [s (seq coll)]
    (if s
      (if (pred (first s)) (recur (next s)) false)
      true)))

(defn not-every? [pred coll] (not (every? pred coll)))

;; oracle: (some even? [1 3 4]) => true. clojure.core seq predicate — NOT the
;; Result-track `just`/`none` (CLAUDE.md precedence: `some` stays Clojure's).
(defn some [pred coll]
  (loop [s (seq coll)]
    (when s
      (or (pred (first s)) (recur (next s))))))

(defn not-any? [pred coll] (not (some pred coll)))

;; oracle: (max-key count "a" "ccc" "bb") => "ccc"
(defn max-key
  ([k x] x)
  ([k x y] (if (> (k x) (k y)) x y))
  ([k x y & more]
   (reduce (fn [a b] (if (> (k a) (k b)) a b)) (if (> (k x) (k y)) x y) more)))

;; oracle: (min-key count "a" "ccc" "bb") => "a"
(defn min-key
  ([k x] x)
  ([k x y] (if (< (k x) (k y)) x y))
  ([k x y & more]
   (reduce (fn [a b] (if (< (k a) (k b)) a b)) (if (< (k x) (k y)) x y) more)))

;; --- Iteration macros -----------------------------------------------------

;; oracle: (dotimes [i 3] ...) runs the body for i=0,1,2, returns nil.
(defmacro dotimes [bindings & body]
  (let [i (first bindings) n (second bindings)]
    `(let [n# ~n]
       (loop [~i 0]
         (if (< ~i n#)
           (do ~@body (recur (inc ~i)))
           nil)))))

;; oracle: (doseq [x coll] ...) runs the body per element, returns nil.
;; Multiple binding pairs nest; modifiers (:when/:let/:while) are not yet
;; supported (v0). TODO: modifiers.
(defmacro doseq [bindings & body]
  (if (empty? bindings)
    `(do ~@body nil)
    (let [x (first bindings) coll (second bindings) more (nnext bindings)]
      `(loop [s# (seq ~coll)]
         (if s#
           (do (let [~x (first s#)]
                 (doseq [~@more] ~@body))
               (recur (next s#)))
           nil)))))

;; for — simplified list comprehension (v0): one or more [binding coll] pairs,
;; no modifiers. Expands to nested map/mapcat, so the result is a lazy seq.
;; oracle: (for [x (range 3)] (* x x)) => (0 1 4)
;; TODO: :when / :let / :while modifiers.
(defn -for-expand [pairs body]
  (if (seq pairs)
    (let [x (first (first pairs))
          coll (second (first pairs))
          more (rest pairs)]
      (if (seq more)
        (list 'clojure.core/mapcat (list 'clojure.core/fn (vector x) (-for-expand more body)) coll)
        (list 'clojure.core/map (list 'clojure.core/fn (vector x) body) coll)))
    body))

(defmacro for [bindings body]
  (-for-expand (-pairs bindings) body))

;; ===========================================================================
;; Control-flow macros (clojure.core) — conditional threading, constant/
;; predicate dispatch, some/nil-aware binding, and side-effecting iteration.
;; Standard Clojure; behavior oracle-verified against JVM Clojure 1.12.5
;; (conformance/tests/macro-*.clj). No renames; nothing here shadows or
;; changes clojure.core semantics (CLAUDE.md precedence principle).
;;
;; v0 deviations (semantics-identical, only the emitted shape differs):
;;   - `case` expands to a sequential `=` comparison chain (cond-style), not
;;     the JVM's O(1) constant-hash jump table — an optimization only; the
;;     analyzer has no `case*` special yet (design/00). Test constants are
;;     unevaluated; a list `(a b c)` in test position matches any member.
;;   - `letfn` is deliberately NOT provided: it needs a `letfn*` special to
;;     give the local fns mutually-recursive scope, which the analyzer/eval
;;     does not implement (only referenced in comments). A let-over-atoms
;;     emulation cannot preserve plain cross-call syntax, so shipping it
;;     would be broken; skipped cleanly until letfn* lands (see report).

;; --- Conditional threading: cond-> / cond->> ------------------------------
;; Thread the (once-evaluated) initial value through a step ONLY when its
;; paired test is truthy; each step and the init are evaluated once.
;; oracle: (cond-> 1 true inc false (* 100) true (* 2)) => 4
;; oracle: (cond->> 1 true inc true (- 10))             => 8
(defn -cond-thread [arrow g clauses]
  (if (seq clauses)
    (let [test (first clauses)
          step (second clauses)
          g2 (gensym "cond__")]
      `(let [~g2 (if ~test (~arrow ~g ~step) ~g)]
         ~(-cond-thread arrow g2 (nnext clauses))))
    g))

(defmacro cond-> [expr & clauses]
  (let [g (gensym "cond__")]
    `(let [~g ~expr] ~(-cond-thread '-> g (seq clauses)))))

(defmacro cond->> [expr & clauses]
  (let [g (gensym "cond__")]
    `(let [~g ~expr] ~(-cond-thread '->> g (seq clauses)))))

;; --- Nil-short-circuiting threading: some-> / some->> ---------------------
;; Thread while non-nil; the first nil short-circuits the whole form to nil.
;; oracle: (some-> {:a {:b 5}} :a :b inc)            => 6
;; oracle: (some-> nil :a)                            => nil
;; oracle: (some->> [1 2 3] (map inc) (reduce +))     => 9
(defn -some-thread [arrow g forms]
  (if (seq forms)
    (let [form (first forms)
          g2 (gensym "some__")]
      `(let [~g2 (if (nil? ~g) nil (~arrow ~g ~form))]
         ~(-some-thread arrow g2 (rest forms))))
    g))

(defmacro some-> [expr & forms]
  (let [g (gensym "some__")]
    `(let [~g ~expr] ~(-some-thread '-> g (seq forms)))))

(defmacro some->> [expr & forms]
  (let [g (gensym "some__")]
    `(let [~g ~expr] ~(-some-thread '->> g (seq forms)))))

;; --- as-> : thread into a named binding at any position -------------------
;; oracle: (as-> 5 x (+ x 1) (* x 2)) => 12
(defn -as-steps [name forms]
  (if (seq forms)
    `(let [~name ~(first forms)] ~(-as-steps name (rest forms)))
    name))

(defmacro as-> [expr name & forms]
  `(let [~name ~expr] ~(-as-steps name (seq forms))))

;; --- if-some / when-some : bind + branch on non-nil (some?) ----------------
;; Unlike if-let/when-let (truthiness), these test only for nil, so a bound
;; `false` still takes the "some" branch.
;; oracle: (macroexpand-1 '(if-some [x v] a b)) =>
;;   (clojure.core/let [temp__auto__ v]
;;     (if (clojure.core/nil? temp__auto__) b
;;       (clojure.core/let [x temp__auto__] a)))
;; oracle: (if-some [x (get {:a 1} :a)] x :none) => 1; (when-some [x false] :got) => :got
(defmacro if-some
  ([bindings then] `(if-some ~bindings ~then nil))
  ([bindings then else]
   (let [form (first bindings) tst (second bindings)]
     `(let [temp# ~tst]
        (if (nil? temp#) ~else (let [~form temp#] ~then))))))

(defmacro when-some [bindings & body]
  (let [form (first bindings) tst (second bindings)]
    `(let [temp# ~tst]
       (if (nil? temp#) nil (let [~form temp#] ~@body)))))

;; --- condp : dispatch by a binary predicate --------------------------------
;; (condp pred expr t1 r1 t2 r2 ... default?) — returns r for the first
;; clause where (pred t expr) is truthy; the ternary `t :>> f` form calls
;; (f (pred t expr)). No match + no default throws (runtime).
;; oracle: (condp = 3 1 :a 3 :c :none)                    => :c
;; oracle: (condp = 2 1 :a 2 :>> (fn [x] [:got x]) :none) => [:got true]
(defn -condp-emit [pred expr clauses]
  (let [n (if (= :>> (second clauses)) 3 2)
        clause (take n clauses)
        more (drop n clauses)
        c (count clause)]
    (cond
      (= 0 c) `(clojure.core/-illegal-argument (str "No matching clause: " ~expr))
      (= 1 c) (first clause)
      (= 2 c) `(if (~pred ~(first clause) ~expr)
                 ~(second clause)
                 ~(-condp-emit pred expr more))
      :else `(if-let [p# (~pred ~(first clause) ~expr)]
               (~(nth clause 2) p#)
               ~(-condp-emit pred expr more)))))

(defmacro condp [pred expr & clauses]
  (let [gpred (gensym "pred__") gexpr (gensym "expr__")]
    `(let [~gpred ~pred ~gexpr ~expr]
       ~(-condp-emit gpred gexpr (seq clauses)))))

;; --- case : constant dispatch (v0 = sequential = comparison) --------------
;; Test constants are unevaluated literals; a list matches any of its members.
;; A trailing odd clause is the default; no match + no default throws.
;; oracle: (case 2 1 :one 2 :two :default)     => :two
;; oracle: (case 9 1 :one :default)            => :default
;; oracle: (case :b :a 1 (:b :c) 2 :d)         => 2
(defn -case-test [ge const]
  (if (seq? const)
    (cons 'clojure.core/or (map (fn [c] `(= ~ge (quote ~c))) const))
    `(= ~ge (quote ~const))))

(defn -case-emit [ge clauses]
  (if (seq clauses)
    (if (next clauses)
      `(if ~(-case-test ge (first clauses))
         ~(second clauses)
         ~(-case-emit ge (nnext clauses)))
      (first clauses))
    `(clojure.core/-illegal-argument (str "No matching clause: " ~ge))))

(defmacro case [e & clauses]
  (let [ge (gensym "case__")]
    `(let [~ge ~e] ~(-case-emit ge (seq clauses)))))

;; --- doto : side-effect a value, then return it ---------------------------
;; oracle: (doto (atom 0) (reset! 5)) => the atom (deref => 5)
(defmacro doto [x & forms]
  (let [g (gensym "doto__")]
    `(let [~g ~x]
       ~@(map (fn [f]
                (if (seq? f)
                  `(~(first f) ~g ~@(next f))
                  `(~f ~g)))
              forms)
       ~g)))

;; --- while : loop for side effects while test is truthy -------------------
;; oracle: (while false 1) => nil
(defmacro while [test & body]
  `(loop []
     (when ~test
       ~@body
       (recur))))

;; --- dorun / doall : force a lazy seq -------------------------------------
;; oracle: (dorun (map identity [1 2 3])) => nil (walks for effect)
;; oracle: (doall (map inc [1 2 3]))      => (2 3 4) (forces AND returns)
(defn dorun [coll]
  (loop [s (seq coll)]
    (if s (recur (next s)) nil)))

(defn doall [coll]
  (dorun coll)
  coll)

;; --- defmulti / defmethod : multimethods (the value-dispatch polymorphism
;; mechanism; the type-dispatch one is defprotocol) ------------------------
;; The runtime MultiFn value + registry live in pkg/eval/multimethod_builtins.go
;; (mirrors protocols.go); these macros are the surface over the private
;; -defmulti/-defmethod builtins. A MultiFn implements IFn, so the var a
;; defmulti binds is directly callable.
;;
;; (defmulti area :shape)             ; dispatch fn can be a keyword/fn/etc.
;; (defmethod area :circle [s] ...)   ; register an impl for a dispatch value
;; (defmethod area :default [s] ...)  ; :default is the fallback
;; oracle (Clojure 1.12.5): see conformance/tests/multimethod-*.clj
(defmacro defmulti [mm-name dispatch-fn]
  (list 'def mm-name
        (list '-defmulti (name mm-name) dispatch-fn)))

(defmacro defmethod [mm-name dispatch-val params & body]
  (list '-defmethod mm-name dispatch-val
        (list* 'fn params body)))

;; --- ns : namespace declaration (minimal; jank clojure-test-suite harness,
;; ADR 0022) --------------------------------------------------------------
;; Expands to: switch to the namespace (in-ns), refer clojure.core, then one
;; require per :require libspec. Reader conditionals in libspecs are already
;; resolved by the reader before this macro sees the clauses (a #?(:clj …)
;; clause the reader elides never reaches here). Clauses other than :require
;; (:import / :use / :refer-clojure / :gen-class …) are currently ignored —
;; the suite gates its only :import behind #?(:clj …), which cljgo elides.
;; oracle (Clojure 1.12.5): (ns foo (:require [clojure.string :as s]))
;;   makes s/upper-case resolve in foo.
(defmacro ns [nsname & clauses]
  (let [requires (mapcat
                  (fn [clause]
                    (when (and (seq? clause) (= :require (first clause)))
                      (map (fn [spec]
                             (list 'clojure.core/require (list 'quote spec)))
                           (rest clause))))
                  clauses)]
    (cons 'do
          (cons (list 'clojure.core/in-ns (list 'quote nsname))
                (cons (list 'clojure.core/refer (list 'quote 'clojure.core))
                      requires)))))
