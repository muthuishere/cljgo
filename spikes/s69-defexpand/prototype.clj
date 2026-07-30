;; s69 — defexpand SEMANTICS prototype (macro-based approximation).
;;
;; This file does NOT touch the analyzer. It approximates ADR 0107's
;; `defexpand` purely in Clojure, as a macro that:
;;   1. binds every argument ONCE, left-to-right, in a single `let`;
;;   2. renames the parameters AND every `let`/`loop`/`fn` local in the body
;;      to fresh names (hygiene by construction, no gensym authored);
;;   3. splices the renamed body at the call site.
;;
;; Its purpose is to settle WHAT defexpand must mean, and to expose the
;; things a macro-based mechanism CANNOT do (see 06-fn-fallback.clj).
;;
;; Deliberate spike-grade limitations (called out in VERDICT.md):
;;   - substitution does not respect `quote` (a quoted symbol equal to a
;;     parameter name would be rewritten);
;;   - local collection handles let/let*/loop/loop*/fn/fn*/when-let/if-let
;;     binding vectors only, and ignores destructuring shapes beyond plain
;;     symbols;
;;   - no recursion guard here — 04-recursion.clj measures what happens
;;     without one.

;; --- code walking -------------------------------------------------------

(defn dx-subst
  "Structure-preserving symbol substitution."
  [smap form]
  (cond
    (symbol? form) (get smap form form)
    (vector? form) (mapv #(dx-subst smap %) form)
    (map? form)    (into {} (map (fn [e] [(dx-subst smap (key e))
                                          (dx-subst smap (val e))]) form))
    (set? form)    (set (map #(dx-subst smap %) form))
    (seq? form)    (apply list (map #(dx-subst smap %) form))
    :else form))

(def dx-binding-forms '#{let let* loop loop* when-let if-let when-some if-some})
(def dx-fn-forms      '#{fn fn*})

(defn dx-locals
  "Every symbol the body binds locally (let/loop/fn params)."
  [form]
  (cond
    (seq? form)
    (let [h (first form)]
      (concat
        (when (and (symbol? h) (dx-binding-forms h) (vector? (second form)))
          (filter symbol? (take-nth 2 (second form))))
        (when (and (symbol? h) (dx-fn-forms h))
          (filter symbol? (flatten (filter vector? (rest form)))))
        (mapcat dx-locals form)))
    (vector? form) (mapcat dx-locals form)
    :else nil))

(defn dx-symbols [form]
  (cond
    (symbol? form) [form]
    (or (seq? form) (vector? form) (set? form)) (mapcat dx-symbols form)
    (map? form) (mapcat (fn [e] (concat (dx-symbols (key e)) (dx-symbols (val e)))) form)
    :else nil))

(defn dx-var-sym
  "ns-qualified symbol naming a var. cljgo vars carry no :name in their meta
  (and core vars carry NO meta at all — see VERDICT incidental findings), so
  read it off the printed form `#=(var ns/name)`."
  [v]
  (let [s (pr-str v)]
    (when (and (> (count s) 8) (= "#=(var " (subs s 0 7)))
      (symbol (subs s 7 (dec (count s)))))))

(defn dx-freemap
  "Resolution IN THE DEFINING NAMESPACE: every FREE symbol of the body (not a
  parameter, not a body local) that resolves there is rewritten to its
  fully-qualified name, so a caller's local of the same name cannot capture it.
  This is the second half of hygiene — referential transparency. Done at
  EXPANSION time (not definition time) so forward references still resolve."
  [def-ns argv body]
  (let [bound (set (concat argv (dx-locals body)))]
    (into {} (keep (fn [s]
                     (when-not (or (bound s) (namespace s))
                       (when-let [v (ns-resolve def-ns s)]
                         (when-let [q (dx-var-sym v)]
                           [s q]))))
                   (distinct (dx-symbols body))))))

(defn dx-simple?
  "R1-elision test: an argument form that is effect-free AND yields the same
  value however many times it is evaluated, so binding it to a temporary is
  pure overhead. Literals and bare symbols (locals) qualify."
  [form]
  (or (symbol? form) (keyword? form) (string? form) (number? form)
      (nil? form) (true? form) (false? form) (char? form)))

(defn dx-expand
  "The expansion engine: argv/body/def-ns captured at definition time, argvals
  are the caller's *unevaluated* argument forms. Returns the spliced form.

  Four rules, in order:
    R1  bind every argument once, left to right, in one outer let;
    R1' ELIDE that binding for arguments that are already simple (dx-simple?)
        — otherwise the temporary costs more than the call it replaced;
    R2  rename every local the body introduces;
    R3  qualify every remaining free symbol against the DEFINING namespace."
  ([argv body argvals] (dx-expand (ns-name *ns*) argv body argvals true true))
  ([def-ns argv body argvals rename-locals?]
   (dx-expand def-ns argv body argvals rename-locals? true))
  ([def-ns argv body argvals rename-locals? elide?]
   (let [;; R2 first: rename the body's own locals.
         locals  (if rename-locals? (distinct (dx-locals body)) ())
         lsmap   (zipmap locals (map #(gensym (str (name %) "__")) locals))
         renamed (dx-subst lsmap body)
         ;; R3 next, while the parameters still have their AUTHORED names —
         ;; qualifying after substitution would let a caller symbol that
         ;; happens to match a body free name suppress its qualification.
         qbody   (dx-subst (dx-freemap def-ns (vec (concat argv (vals lsmap))) renamed)
                           renamed)
         ;; R1 (+ R1' elision) last.
         pairs   (map (fn [p v] (if (and elide? (dx-simple? v))
                                  [p v nil]
                                  (let [g (gensym (str (name p) "__"))] [p g [g v]])))
                      argv argvals)
         smap    (into {} (map (fn [t] [(nth t 0) (nth t 1)]) pairs))
         binds   (vec (mapcat #(nth % 2) pairs))
         newbody (dx-subst smap qbody)]
     (if (seq binds)
       (concat (list 'clojure.core/let binds) newbody)
       (cons 'do newbody)))))

;; --- the surface form ---------------------------------------------------

(defmacro defexpand
  "Prototype of ADR 0107 defexpand: written like a defn, expanded at every
  direct call site with arguments bound once, left to right."
  [nm & decl]
  (let [doc  (when (string? (first decl)) (first decl))
        decl (if doc (rest decl) decl)
        argv (first decl)
        body (rest decl)]
    (when (some #{'&} argv)
      (throw (ex-info (str "defexpand: variadic parameters are not supported: " nm)
                      {:name nm :params argv})))
    (let [def-ns (ns-name *ns*)]
      `(defmacro ~nm ~@(when doc [doc]) [~@argv]
         (dx-expand '~def-ns '~argv '~body (list ~@argv) true true)))))

(defmacro defexpand-no-elide
  "defexpand with R1 applied unconditionally (no R1' elision) — the shape
  08-cost-of-once-only.clj measures against."
  [nm argv & body]
  `(defmacro ~nm [~@argv]
     (dx-expand '~(ns-name *ns*) '~argv '~body (list ~@argv) true false)))

(defmacro defexpand-unqualified
  "defexpand with R1+R2 but WITHOUT R3 (free-symbol qualification) — kept only
  to demonstrate the capture that R3 prevents (02-hygiene.clj, case D)."
  [nm argv & body]
  `(defmacro ~nm [~@argv]
     (let [gs# (mapv (fn [p#] (gensym (str (name p#) "__"))) '~argv)
           ls# (distinct (dx-locals '~body))
           sm# (merge (zipmap ls# (map (fn [l#] (gensym (str (name l#) "__"))) ls#))
                      (zipmap '~argv gs#))]
       (concat (list 'clojure.core/let (vec (interleave gs# (list ~@argv))))
               (dx-subst sm# '~body)))))

(defmacro defexpand-no-rename
  "defexpand with R1+R3 but WITHOUT R2 (body-local renaming) — used to test
  whether R2 is load-bearing once R1 and R3 are in place."
  [nm argv & body]
  `(defmacro ~nm [~@argv]
     (dx-expand '~(ns-name *ns*) '~argv '~body (list ~@argv) false false)))

(defmacro defexpand-no-rename-elide
  "R1' + R3 but NOT R2 — shows that once R1 is elided, body-local renaming
  becomes load-bearing again (the caller's symbol is spliced in bare)."
  [nm argv & body]
  `(defmacro ~nm [~@argv]
     (dx-expand '~(ns-name *ns*) '~argv '~body (list ~@argv) false true)))

;; --- the ADR-0066-shaped GUARDED variant --------------------------------
;;
;; ADR 0066 seals the core arithmetic vars and elides the per-call guard until
;; a process-global dirty flag trips; once anything redefines a sealed var the
;; guarded path comes back. This is the same shape, per-defexpand: snapshot the
;; pristine fn at definition, and emit
;;
;;   (let [p1 ARG1 ...] (if (identical? f pristine) INLINE-BODY (f p1 ...)))
;;
;; so a live redefinition (with-redefs / alter-var-root) IS seen at inlined
;; sites. In this prototype the fn must carry a DIFFERENT name from the macro
;; (see 06-fn-fallback.clj — that is exactly the limitation the real analyzer
;; implementation removes).

(defmacro defexpand-guarded [nm argv & body]
  (let [fname (symbol (str nm "-fn"))
        pname (symbol (str "dx-pristine-" nm))
        dns   (ns-name *ns*)]
    `(do
       (defn ~fname ~argv ~@body)
       (def ~pname ~fname)
       (defmacro ~nm [~@argv]
         (let [inline# (dx-expand '~dns '~argv '~body (list ~@argv) true false)
               gs#     (vec (second inline#))
               names#  (vec (take-nth 2 gs#))]
           (list 'clojure.core/let gs#
                 (list 'if (list 'clojure.core/identical?
                                 '~(symbol (str dns) (str fname))
                                 '~(symbol (str dns) (str pname)))
                       (cons 'do (drop 2 inline#))
                       (cons '~(symbol (str dns) (str fname)) names#)))))
       (var ~fname))))

;; --- the NAIVE version, for contrast ------------------------------------

(defmacro defexpand-naive
  "What a hand-written defmacro does: textual substitution, no once-only
  binding, no renaming. Kept so the failures are demonstrable, not asserted."
  [nm argv & body]
  `(defmacro ~nm [~@argv]
     (dx-subst (zipmap '~argv (list ~@argv)) '~(cons 'do body))))

(println ";; s69 defexpand prototype loaded")
