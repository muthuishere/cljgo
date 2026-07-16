;; realized? on a lazy-seq must flip true once it's been forced (e.g. by
;; `first`). Root cause of a prior regression: LazySeq.IsRealized checked
;; (IsNil s.fn) where s.fn is a `func() interface{}` — Go's classic "typed
;; nil" trap: a nil func boxed into an `any` is != the untyped nil literal,
;; so a generic IsNil that only special-cased reflect.Ptr always answered
;; false for it, and (realized? some-lazy-seq) was permanently false even
;; after forcing. IsNil must also check Func (and Chan/Map/Slice/...).
;; Regression: clojure-test-suite core_test/lazy_seq.cljc (jank suite, ADR
;; 0022).
;; Oracle (clojure 1.12.5): [false true]
(let [s (lazy-seq (cons 1 nil))]
  [(realized? s) (do (first s) (realized? s))])
;; expect: [false true]
