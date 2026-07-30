;; Functions carry metadata. On the JVM every fn extends AFunction, which
;; implements IObj, so with-meta on a fn returns the fn WITH its map; meta
;; replaces (never nests) and with-meta always returns a NEW value.
;; Before ADR 0105 (spike s66) cljgo diverged in BOTH legs: the interpreter
;; threw "value of type *eval.evalFn can't have metadata" and a compiled
;; binary silently DROPPED the map (lang.FnFuncN/NamedFnN had a no-op
;; WithMeta). Both now box the closure in lang.MetaFn.
;; oracle (clojure 1.12.5, 2026-07-28):
;;   $ clojure -M -e '(let [f (with-meta (fn [] 1) {:tag :mock})] [(f) (meta f)])'
;;   [1 {:tag :mock}]
;;   (meta (fn [] 1)) => nil
;;   (fn? (with-meta (fn [] 1) {:a 1})) => true
;;   (meta (with-meta (with-meta f {:a 1}) {:b 2})) => {:b 2}
;;   (meta (with-meta (with-meta f {:a 1}) nil)) => nil
;;   (identical? f (with-meta f {:a 1})) => false
;;   (meta (vary-meta (with-meta f {:a 1}) assoc :b 2)) => {:a 1, :b 2}
(let [f (with-meta (fn [] 1) {:tag :mock})
      g (with-meta (fn [] 1) {:a 1})]
  [(f) (meta f)
   (meta (fn [] 1))
   (fn? f)
   (ifn? f)
   (meta (with-meta g {:b 2}))
   (meta (with-meta g nil))
   (identical? g (with-meta g {:a 1}))
   (meta (vary-meta g assoc :b 2))])
;; expect: [1 {:tag :mock} nil true true {:b 2} nil false {:a 1, :b 2}]
