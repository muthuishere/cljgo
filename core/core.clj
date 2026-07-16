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

;; comment: ignores its body entirely, always expanding to nil — a real
;; clojure.core MACRO (not a special form; (special-symbol? 'comment) is
;; false on the JVM), previously simply missing from cljgo (design/08
;; batch E, ADR 0022). Since the body is never analyzed as code, it can
;; hold anything, including forms cljgo can't parse.
;; oracle: (comment (this is not valid clojure +++)) => nil
(defmacro comment [& body] nil)

;; when-first: bindings is [x coll] — calls (seq coll) exactly ONCE, binds
;; x to its first element, and runs body (implicit do); nil if coll is
;; empty (design/08 batch E, ADR 0022). A direct port of clojure.core's own
;; when-first onto when-let.
;; oracle: (when-first [x [0 1 2]] x) => 0; (when-first [x []] x) => nil;
;; (when-first [x nil] x) => nil; body has an implicit do.
(defmacro when-first [bindings & body]
  (when-not (vector? bindings)
    (throw (ex-info "when-first requires a vector for its binding" {:bindings bindings})))
  (when-not (= 2 (count bindings))
    (throw (ex-info "when-first requires exactly 2 forms in binding vector" {:bindings bindings})))
  (let [[x xs] bindings]
    `(when-let [xs# (seq ~xs)]
       (let [~x (first xs#)]
         ~@body))))

;; with-out-str: runs body with *out* bound to a fresh in-memory writer
;; (-string-writer / -string-writer-str, builtins.go) and returns
;; everything printed as a string (design/08 batch E, ADR 0022). An empty
;; body captures "". `binding` is forced bare with ~'binding: it is a
;; cljgo special form (not in the reader's syntax-quote special list,
;; which only knows the standard Clojure specials, per test.cljg's
;; `testing` macro) — syntax-quote would otherwise qualify it to
;; clojure.core/binding, which now resolves (the resolve-vs-special-form
;; fix, builtins.go) to a placeholder macro Var that panics if actually
;; invoked, instead of taking the special-form path.
;; oracle: (with-out-str (print "a") (print "b")) => "ab";
;; (with-out-str) => ""
(defmacro with-out-str
  [& body]
  `(let [s# (-string-writer)]
     (~'binding [*out* s#]
       ~@body)
     (-string-writer-str s#)))

;; future: runs body in a new goroutine, conveying the calling goroutine's
;; dynamic-var bindings (future-call/lang.AgentSubmit, design/08 batch E,
;; ADR 0022); @/deref blocks for the result, realized? reports completion.
;; oracle: @(future (+ 1 2)) => 3
(defmacro future [& body]
  (list 'clojure.core/future-call (list* 'fn* [] body)))

;; bound-fn* / bound-fn: wrap f (or a fn literal) so that when INVOKED —
;; possibly on another goroutine (future, go, thread) — it re-establishes
;; the dynamic-var bindings captured at WRAP time, not whatever happens to
;; be bound on the calling goroutine (design/08 batch E, ADR 0022). A
;; direct port of clojure.core's own bound-fn*/bound-fn onto
;; get/push/pop-thread-bindings (var_builtins.go).
;; oracle: (binding [*x* :v] (let [f (bound-fn [] *x*)] (binding [*x* :other] (f)))) => :v
(defn bound-fn*
  [f]
  (let [bindings (get-thread-bindings)]
    (fn [& args]
      (push-thread-bindings bindings)
      (try
        (apply f args)
        (finally (pop-thread-bindings))))))

(defmacro bound-fn [& fntail]
  `(bound-fn* (fn ~@fntail)))

;; --- print-str family : format to a string instead of *out* --------------
;; print/println/pr/prn are Go builtins (builtins.go) that write through
;; *out*; these *-str fns capture the same output via with-out-str instead
;; of printing it — the real clojure.core definitions, unchanged (design/08
;; batch E, ADR 0022).
;; oracle: (print-str "a" 1) => "a 1"; (println-str "a" 1) => "a 1\n";
;; (prn-str "a" 1) => "\"a\" 1\n"; (print-str) => ""
(defn print-str
  [& xs]
  (with-out-str (clojure.core/apply print xs)))

(defn println-str
  [& xs]
  (with-out-str (clojure.core/apply println xs)))

(defn prn-str
  [& xs]
  (with-out-str (clojure.core/apply prn xs)))

;; --- Core higher-order fns ------------------------------------------------

;; -all-seqs : (seq c) for every c in cs as a seq, or nil if any is empty —
;; the termination test for map's 4+-arity (stops at the shortest coll).
(defn -all-seqs [cs]
  (loop [cs (seq cs) acc []]
    (if cs
      (let [s (seq (first cs))]
        (when s (recur (next cs) (conj acc s))))
      (seq acc))))

;; oracle: (map inc [1 2 3]) => (2 3 4); (map + [1 2 3] [10 20 30]) => (11 22 33)
;; oracle: (map + [1 2] [10 20] [100 200]) => (111 222)
;; oracle: (into [] (map inc) [1 2 3]) => [2 3 4]  -- 1-arity is the transducer form (ADR 0022 Batch 4)
(defn map
  ([f]
   (fn [rf]
     (fn
       ([] (rf))
       ([result] (rf result))
       ([result input] (rf result (f input))))))
  ([f coll]
   (lazy-seq
    (when-let [s (seq coll)]
      (cons (f (first s)) (map f (rest s))))))
  ([f c1 c2]
   (lazy-seq
    (let [s1 (seq c1) s2 (seq c2)]
      (when (and s1 s2)
        (cons (f (first s1) (first s2))
              (map f (rest s1) (rest s2)))))))
  ([f c1 c2 c3 & colls]
   (let [step (fn step [cs]
                (lazy-seq
                 (when-let [ss (-all-seqs cs)]
                   (cons (apply f (map first ss))
                         (step (map rest ss))))))]
     (step (list* c1 c2 c3 colls)))))

;; oracle: (filter even? (range 10)) => (0 2 4 6 8)
;; oracle: (into [] (filter even?) (range 10)) => [0 2 4 6 8]
(defn filter
  ([pred]
   (fn [rf]
     (fn
       ([] (rf))
       ([result] (rf result))
       ([result input] (if (pred input) (rf result input) result)))))
  ([pred coll]
   (lazy-seq
    (when-let [s (seq coll)]
      (let [x (first s)]
        (if (pred x)
          (cons x (filter pred (rest s)))
          (filter pred (rest s))))))))

;; oracle: (remove even? (range 10)) => (1 3 5 7 9)
;; oracle: (into [] (remove even?) (range 10)) => [1 3 5 7 9]
(defn remove
  ([pred] (filter (fn [x] (not (pred x)))))
  ([pred coll] (filter (fn [x] (not (pred x))) coll)))

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

;; unreduced/ensure-reduced : the reduced-box helpers transducers need
;; (design/08 §5 Batch 4). `reduced`/`reduced?` are host builtins already.
;; oracle: (unreduced (reduced 5)) => 5; (unreduced 5) => 5
(defn unreduced [x] (if (reduced? x) (deref x) x))

;; oracle: (reduced? (ensure-reduced 5)) => true; (reduced? (ensure-reduced (reduced 5))) => true
(defn ensure-reduced [x] (if (reduced? x) x (reduced x)))

;; oracle: (keep #(when (even? %) %) (range 6)) => (0 2 4)
;; oracle: (into [] (keep #(when (even? %) %)) (range 6)) => [0 2 4]
(defn keep
  ([f]
   (fn [rf]
     (fn
       ([] (rf))
       ([result] (rf result))
       ([result input]
        (let [v (f input)]
          (if (nil? v) result (rf result v)))))))
  ([f coll]
   (lazy-seq
    (when-let [s (seq coll)]
      (let [x (f (first s))]
        (if (nil? x)
          (keep f (rest s))
          (cons x (keep f (rest s)))))))))

;; oracle: (map-indexed vector [:a :b :c]) => ([0 :a] [1 :b] [2 :c])
;; oracle: (into [] (map-indexed vector) [:a :b :c]) => [[0 :a] [1 :b] [2 :c]]
(defn -map-indexed-step [f i coll]
  (lazy-seq
   (when-let [s (seq coll)]
     (cons (f i (first s)) (-map-indexed-step f (inc i) (rest s))))))

(defn map-indexed
  ([f]
   (fn [rf]
     (let [iv (atom -1)]
       (fn
         ([] (rf))
         ([result] (rf result))
         ([result input] (rf result (f (swap! iv inc) input)))))))
  ([f coll] (-map-indexed-step f 0 coll)))

;; oracle: (keep-indexed (fn [i x] (when (even? i) x)) [:a :b :c :d]) => (:a :c)
;; oracle: (into [] (keep-indexed (fn [i x] (when (even? i) x))) [:a :b :c :d]) => [:a :c]
(defn -keep-indexed-step [f i coll]
  (lazy-seq
   (when-let [s (seq coll)]
     (let [x (f i (first s))]
       (if (nil? x)
         (-keep-indexed-step f (inc i) (rest s))
         (cons x (-keep-indexed-step f (inc i) (rest s))))))))

(defn keep-indexed
  ([f]
   (fn [rf]
     (let [iv (atom -1)]
       (fn
         ([] (rf))
         ([result] (rf result))
         ([result input]
          (let [i (swap! iv inc)
                v (f i input)]
            (if (nil? v) result (rf result v))))))))
  ([f coll] (-keep-indexed-step f 0 coll)))

;; identity/comp are hoisted here (ahead of their "Function combinators"
;; section further down) because `cat`/`mapcat`'s transducer forms need
;; `comp` at analysis time; the rest of the combinators (partial, complement,
;; fnil, juxt) don't depend on this and stay in their original section.
;; oracle: (identity 7) => 7
(defn identity [x] x)

;; oracle: ((comp inc inc) 5) => 7
(defn comp
  ([] identity)
  ([f] f)
  ([f g] (fn [& args] (f (apply g args))))
  ([f g & fs] (reduce comp (list* f g fs))))

;; -preserving-reduced : wraps rf so a `reduced` returned by an INNER reduce
;; (cat's per-input reduce) re-wraps as `reduced` again, so the OUTER reduce
;; also stops instead of unwrapping once and continuing (design/08 §5 Batch 4).
;; oracle: (reduced? ((-preserving-reduced (fn [_ _] (reduced :x))) nil 1)) => true
(defn -preserving-reduced [rf]
  (fn [result input]
    (let [ret (rf result input)]
      (if (reduced? ret) (reduced ret) ret))))

;; cat : a transducer (not a fn of args — a value) that concatenates each
;; input (itself a collection) into the reduction, e.g. (into [] cat [[1 2] [3]]).
;; oracle: (into [] cat [[1 2] [3 4]]) => [1 2 3 4]
(def cat
  (fn [rf]
    (let [rrf (-preserving-reduced rf)]
      (fn
        ([] (rf))
        ([result] (rf result))
        ([result input] (reduce rrf result input))))))

;; -concat-seqs : lazily concatenate a (possibly infinite) seq OF seqs.
;; JVM mapcat spells this (apply concat xs) — lazy there because apply
;; realizes only concat's fixed arity; cljgo's apply forces the whole
;; last-arg seq (ToSlice), which would hang on infinite input, so the
;; lazy flatten is explicit.
(defn -concat-seqs [colls]
  (lazy-seq
   (when-let [s (seq colls)]
     (concat (first s) (-concat-seqs (rest s))))))

;; oracle: (mapcat (fn [x] [x x]) [1 2 3]) => (1 1 2 2 3 3)
;; oracle: (into [] (mapcat (fn [x] [x x])) [1 2 3]) => [1 1 2 2 3 3]
;; oracle: (take 5 (mapcat (fn [x] (repeat 2 x)) (range))) => (0 0 1 1 2)
(defn mapcat
  ([f] (comp (map f) cat))
  ([f & colls]
   (let [xs (apply map f colls)]
     ;; JVM mapcat is eager in its first few elements (apply realizes
     ;; concat's fixed arity at call time) — e.g. (mapcat identity 5)
     ;; throws immediately. seq once here to match.
     (seq xs)
     (-concat-seqs xs))))

;; oracle: (mapv inc [1 2 3]) => [2 3 4]
(defn mapv
  ([f coll] (vec (map f coll)))
  ([f c1 c2] (vec (map f c1 c2))))

;; oracle: (filterv even? (range 10)) => [0 2 4 6 8]
(defn filterv [pred coll] (vec (filter pred coll)))

;; oracle: (run! println [1 2]) prints, returns nil.
;; oracle: (let [calls (atom 0)] (run! (fn [_] (swap! calls inc) (reduced :done)) (range 2)) @calls) => 1
(defn run! [proc coll]
  ;; The reducing fn must return proc's result (not an unconditional nil):
  ;; when proc returns a (reduced x), reduce needs to SEE that Reduced to
  ;; short-circuit — discarding it here (as an earlier version did) hides
  ;; early termination and makes run! always walk the whole collection.
  (reduce (fn [_ x] (proc x)) nil coll)
  nil)

;; --- Take / drop ----------------------------------------------------------

;; oracle: (take 3 (range)) => (0 1 2); (take 3 (range 10)) => (0 1 2)
;; oracle: (into [] (take 3) (range)) => [0 1 2]
(defn take
  ([n]
   (fn [rf]
     (let [nv (atom n)]
       (fn
         ([] (rf))
         ([result] (rf result))
         ([result input]
          (let [n @nv
                nn (swap! nv dec)
                result (if (pos? n) (rf result input) result)]
            (if (not (pos? nn)) (ensure-reduced result) result)))))))
  ([n coll]
   (lazy-seq
    (when (pos? n)
      (when-let [s (seq coll)]
        (cons (first s) (take (dec n) (rest s))))))))

;; oracle: (drop 2 [1 2 3 4]) => (3 4)
;; oracle: (into [] (drop 2) [1 2 3 4 5]) => [3 4 5]
(defn drop
  ([n]
   (fn [rf]
     (let [nv (atom n)]
       (fn
         ([] (rf))
         ([result] (rf result))
         ([result input]
          (let [n @nv]
            (swap! nv dec)
            (if (pos? n) result (rf result input))))))))
  ([n coll]
   ;; oracle: (= () (drop 5 nil)) => true — same lazy-seq-vs-bare-nil equiv
   ;; gotcha as drop-while above: an eager nil return here would make
   ;; (= () (drop n coll)) false whenever coll is exhausted.
   (lazy-seq
    (loop [n n s (seq coll)]
      (if (and (pos? n) s)
        (recur (dec n) (next s))
        s)))))

;; nthrest: like drop, but returns coll itself (not (seq coll)) for n <= 0,
;; and () rather than nil once the seq is exhausted — the () vs nil
;; distinction real Clojure's `=` respects ((= () nil) is false). A
;; straight port of clojure.core/nthrest, minus the IDrop fast path
;; (design/08 batch E, ADR 0022; cljgo has no IDrop-implementing colls yet).
;; oracle: (nthrest (range 0 10) 3) => (3 4 5 6 7 8 9);
;; (nthrest [0 1 2 3 4 5] 3) => [3 4 5]; (nthrest (range 0 10) 10) => ();
;; (nthrest (range 3) -1) => (0 1 2) (n < 1 => coll unchanged);
;; (nthrest nil 0) => nil; (nthrest nil 100) => ()
(defn nthrest
  [coll n]
  (if (pos? n)
    (or (loop [n n xs coll]
          (if-let [xs (and (pos? n) (seq xs))]
            (recur (dec n) (rest xs))
            (seq xs)))
        ())
    coll))

;; oracle: (take-while #(< % 3) (range 10)) => (0 1 2)
;; oracle: (into [] (take-while even?) [2 4 6 1 8]) => [2 4 6]
(defn take-while
  ([pred]
   (fn [rf]
     (fn
       ([] (rf))
       ([result] (rf result))
       ([result input] (if (pred input) (rf result input) (reduced result))))))
  ([pred coll]
   (lazy-seq
    (when-let [s (seq coll)]
      (when (pred (first s))
        (cons (first s) (take-while pred (rest s))))))))

;; oracle: (drop-while #(< % 3) (range 10)) => (3 4 5 6 7 8 9)
;; oracle: (into [] (drop-while even?) [2 4 6 1 8]) => [1 8]
(defn drop-while
  ([pred]
   (fn [rf]
     (let [dv (atom true)]
       (fn
         ([] (rf))
         ([result] (rf result))
         ([result input]
          (if (and @dv (pred input))
            result
            (do (reset! dv false) (rf result input))))))))
  ([pred coll]
   ;; oracle: (= () (drop-while nil? nil)) => true — real Clojure wraps this in
   ;; lazy-seq, so the empty case returns a LazySeq that seqs to nil, which
   ;; equiv's true against '() (Sequential-with-nil-seq, not bare nil: bare
   ;; nil is NOT = '() — that's a classic gotcha). An eager nil return would
   ;; make (= () (drop-while ...)) false on empty input.
   (lazy-seq
    (loop [s (seq coll)]
      (if (and s (pred (first s)))
        (recur (next s))
        s)))))

;; oracle: (take-nth 2 (range 10)) => (0 2 4 6 8)
;; oracle: (into [] (take-nth 2) (range 10)) => [0 2 4 6 8]; negative n acts
;; like positive in the transducer; n=0 throws (divide by zero), as JVM.
(defn take-nth
  ([n]
   (fn [rf]
     (let [iv (atom -1)]
       (fn
         ([] (rf))
         ([result] (rf result))
         ([result input]
          (let [i (swap! iv inc)]
            (if (zero? (rem i n))
              (rf result input)
              result)))))))
  ([n coll]
   (lazy-seq
    (when-let [s (seq coll)]
      (cons (first s) (take-nth n (drop n s)))))))

;; oracle: (every? even? [2 4 6]) => true
(defn every? [pred coll]
  (loop [s (seq coll)]
    (if s
      (if (pred (first s)) (recur (next s)) false)
      true)))

;; oracle: (partition 2 (range 6)) => ((0 1) (2 3) (4 5))
;; oracle: (partition 3 1 [:a :a :a] nil) => (())  -- wait: pad exhausted mid-way
;;         yields one short partition; (partition 4 4 [:a] (range 10)) pads
;;         the final partial group from `pad`.
(defn partition
  ([n coll] (partition n n coll))
  ([n step coll]
   (lazy-seq
    (when-let [s (seq coll)]
      (let [p (take n s)]
        (when (= n (count p))
          (cons p (partition n step (nthrest s step))))))))
  ([n step pad coll]
   (lazy-seq
    (when-let [s (seq coll)]
      (let [p (take n s)]
        (if (= n (count p))
          (cons p (partition n step pad (nthrest s step)))
          (list (take n (concat p pad)))))))))

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

;; into (2-arity, plus the 3-arity xform form) is defined in transducers.cljg
;; (loaded after this file), since the xform arity needs `transduce`.

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
;; oracle: (into [] (distinct) [1 1 2 3 3 2]) => [1 2 3]
(defn -distinct-step [xs seen]
  (lazy-seq
   (loop [xs xs seen seen]
     (when-let [s (seq xs)]
       (let [f (first s)]
         (if (contains? seen f)
           (recur (rest s) seen)
           (cons f (-distinct-step (rest s) (conj seen f)))))))))

(defn distinct
  ([]
   (fn [rf]
     (let [seen (atom #{})]
       (fn
         ([] (rf))
         ([result] (rf result))
         ([result input]
          (if (contains? @seen input)
            result
            (do (swap! seen conj input) (rf result input))))))))
  ([coll] (-distinct-step coll #{})))

;; oracle: (interpose 0 [1 2 3]) => (1 0 2 0 3)
;; oracle: (into [] (interpose 0) [1 2 3]) => [1 0 2 0 3]
(defn interpose
  ([sep]
   (fn [rf]
     (let [started (atom false)]
       (fn
         ([] (rf))
         ([result] (rf result))
         ([result input]
          (if @started
            (let [sepr (rf result sep)]
              (if (reduced? sepr) sepr (rf sepr input)))
            (do (reset! started true) (rf result input))))))))
  ([sep coll] (drop 1 (mapcat (fn [x] (list sep x)) coll))))

;; oracle: (interleave [1 2 3] [:a :b :c]) => (1 :a 2 :b 3 :c)
;; oracle: (apply interleave [[1 2 3 4 5] ["a" "b" "c"] "12"]) => (1 \a1 2 \b2)
;;         -- stops at the shortest of any number of colls; (interleave) => ();
;;         (interleave c1) => (seq c1).
(defn interleave
  ([] ())
  ([c1] (lazy-seq c1))
  ([c1 c2]
   (lazy-seq
    (let [s1 (seq c1) s2 (seq c2)]
      (when (and s1 s2)
        (cons (first s1)
              (cons (first s2)
                    (interleave (rest s1) (rest s2))))))))
  ([c1 c2 & colls]
   (lazy-seq
    (let [ss (map seq (conj colls c2 c1))]
      (when (every? identity ss)
        (concat (map first ss) (apply interleave (map rest ss))))))))

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
;; oracle: (merge :foo) => :foo -- a single non-map arg (0 or 1 total args)
;; passes through unchanged; real Clojure's reduce1 with no explicit init
;; just returns the sole element without ever inspecting it.
;; oracle: (merge '(1 2 3) 1) => (1 1 2 3) -- the no-init reduce seeds from
;; the first element verbatim, so a non-map first arg just gets conj'd onto.
;;
;; Shape matches real Clojure's `(reduce1 #(conj (or %1 {}) %2) maps)` —
;; 2-arg reduce over the rest args, first element as the seed. An earlier
;; version deliberately dodged this form and always seeded from a fresh {}:
;; conj onto a map fetched out of an existing set (exactly what
;; clojure.set/join does) produced a result carrying the source's stale
;; cached hash, corrupting hash-addressed lookups of any collection the
;; result was stored in. The root cause (Map.clone keeping the cached
;; hash/hasheq across a content-changing Assoc) is fixed in
;; pkg/lang/persistentarraymap.go — see PROVENANCE.md "Stale hash cache on
;; array-map assoc" — so the faithful form is safe again.
;; (`every?` instead of reduce1's `(some identity ...)` guard: `some` is
;; defined later in this file; the two are equivalent for a nil check.)
(defn merge [& maps]
  (when-not (every? nil? maps)
    (reduce (fn [a b] (conj (or a {}) b)) maps)))

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
;; The 3-arity uses `get` with a fresh sentinel (not `contains?`): a
;; non-associative intermediate value (e.g. a keyword mid-path) must yield
;; not-found rather than throw (contains? on a non-collection errors).
(defn get-in
  ([m ks] (reduce get m ks))
  ([m ks not-found]
   (let [sentinel (atom nil)]
     (loop [m m ks (seq ks)]
       (if ks
         (let [v (get m (first ks) sentinel)]
           (if (identical? sentinel v)
             not-found
             (recur v (next ks))))
         m)))))

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
;; Non-integers throw exactly as JVM clojure.core (ADR 0029 cluster D) —
;; oracle 1.12.5: (even? 1.5) => THREW "Argument must be an integer: 1.5";
;; ##Inf strs as "Infinity", nil as "" (so the message ends with the space).
(defn even? [n]
  (if (integer? n)
    (zero? (rem n 2))
    (-illegal-argument (str "Argument must be an integer: " n))))
(defn odd? [n] (not (even? n)))

;; oracle: (mod 7 3) => 1; (mod -7 3) => 2
(defn mod [num div]
  (let [m (rem num div)]
    (if (if (zero? m) true (= (pos? num) (pos? div)))
      m
      (+ m div))))

;; --- Function combinators -------------------------------------------------
;; (identity/comp are hoisted earlier — see the note above `-preserving-reduced`.)

;; oracle: ((constantly 42) 1 2 3) => 42
(defn constantly [x] (fn [& _] x))

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
  ([f a b] (fn [x y & args] (apply f (if (nil? x) a x) (if (nil? y) b y) args)))
  ([f a b c]
   (fn [x y z & args]
     (apply f (if (nil? x) a x) (if (nil? y) b y) (if (nil? z) c z) args))))

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
;; oracle: (some-fn) => ArityException (real Clojure's some-fn has no 0-arg
;; arity — [p] is the minimum); (some-fn even?) 2 => true
(defn some-fn [p & preds]
  (let [preds (cons p preds)]
    (fn [& args]
      (loop [ps (seq preds)]
        (if ps
          (let [r (loop [as (seq args)]
                    (if as
                      (or ((first ps) (first as)) (recur (next as)))
                      false))]
            (if r r (recur (next ps))))
          false)))))

;; --- Predicates / reducers ------------------------------------------------

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
;; oracle: (min-key identity ##-Inf 1 ##NaN) => ##-Inf;
;; (min-key identity ##-Inf ##NaN 1) => ##NaN — NOT the naive fold with `<`
;; (which would land on ##NaN in the first case too): real Clojure's
;; 3+-arity walks the rest with `<=`, not `<` (clojure.repl/source
;; min-key), so once NaN's `<=` comparisons all fail, the loop KEEPS the
;; running winner instead of falling through to the new element.
(defn min-key
  ([k x] x)
  ([k x y] (if (< (k x) (k y)) x y))
  ([k x y & more]
   (let [kx (k x) ky (k y)]
     (loop [v (if (< kx ky) x y) kv (if (< kx ky) kx ky) more more]
       (if more
         (let [w (first more) kw (k w)]
           (if (<= kw kv)
             (recur w kw (next more))
             (recur v kv (next more))))
         v)))))

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

;; --- assert : honors *assert*, throws on a falsy expr (ADR 0022 batch/
;; harness-misc) --------------------------------------------------------------
;; oracle (Clojure 1.12.5): (macroexpand-1 '(assert (= 1 1))) =>
;;   (clojure.core/when-not (= 1 1) (throw (new java.lang.AssertionError ...)))
;; cljgo has no java.lang.AssertionError (no JVM class hierarchy, design/05),
;; so the throw is an ex-info instead — still caught by any Throwable/
;; Exception catch (pkg/eval/ex_builtins.go CatchMatches), which is all the
;; suite's own `assert` usages need. *assert* defaults true, exactly
;; clojure.core's compile-time elision knob: (binding [*assert* false] ...)
;; does NOT suppress an already-compiled assert (v0 has no separate compile
;; step to gate), but a nil-bound var at THIS expansion (the common load-time
;; case) still elides the check, matching the common usage.
(def ^:dynamic *assert* true)

(defmacro assert
  ([x]
   (when *assert*
     `(when-not ~x
        (throw (ex-info (str "Assert failed: " (pr-str '~x)) {})))))
  ([x message]
   (when *assert*
     `(when-not ~x
        (throw (ex-info (str "Assert failed: " ~message "\n" (pr-str '~x)) {}))))))

;; --- delay / force / delay? : lazy, memoized single-value promise -----------
;; pkg/lang already vendors a Delay type (IDeref + IPending, delay.go); this
;; is just the clojure.core surface over it (-make-delay/force/delay? are Go
;; builtins, misc_builtins.go). oracle: (force (delay (+ 1 2))) => 3;
;; (delay? (delay 1)) => true; the body runs at most once (memoized).
(defmacro delay [& body]
  (list 'clojure.core/-make-delay (list* 'fn* [] body)))

;; --- instance? : class-position-as-syntax (ADR 0026) -------------------------
;; cljgo has no java.lang.Class objects (design/05: host interop is Go
;; structs, not a JVM class hierarchy). A literal class symbol is therefore
;; NEVER evaluated — it is matched by NAME, exactly how `catch`'s class
;; symbol already works (CatchMatches, pkg/eval/ex_builtins.go) — via the
;; -instance-of-name? designator table (misc_builtins.go): built-in
;; designators (String/Long/Double/...), cljgo's host wrapper types
;; (Atom/Delay/Var/Namespace/UUID/BigInt/BigDecimal/...), and any
;; deftype/defrecord type name (resolved to its *TypeMarker*, same identity
;; check as -instance?/satisfies?). A non-symbol first argument (already a
;; value, e.g. bound through a local) is evaluated normally and checked the
;; same TypeMarker way. DEVIATION: since the class position is syntax, a
;; literal class symbol only works in DIRECT call position — (instance?
;; String x) works; (partial instance? String) does not (String is not a
;; value cljgo can hand around). oracle (Clojure 1.12.5): (instance? String
;; "x") => true; (instance? Long 1) => true.
(defmacro instance? [c x]
  (if (symbol? c)
    (list 'clojure.core/-instance-of-name? (str c) x)
    (list 'clojure.core/-instance? c x)))

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
