;; num / type (batch/error-files): `num` is a checked cast to Number —
;; `(Number) x` in the real implementation — so nil casts trivially (a null
;; reference is assignable to any reference type) instead of throwing.
;; `type` is new: cljgo has no java.lang.Class objects (design/05), so this
;; is a v0 stand-in (reflect.TypeOf, comparable via `=`'s existing `a == b`
;; fast path) — enough for self-consistency checks like "a no-op numeric
;; coercion doesn't change representation" without a full class/`:type`-meta
;; substrate (that's its own future ADR).
;; oracle (clojure 1.12.5): (num nil) => nil.
[(num nil)
 (= (type nil) (type (num nil)))
 (= (type 1) (type (num 1)))
 (= (type 1.0) (type (num 1.0)))
 (not= (type 1) (type 1.0))]
;; expect: [nil true true true true]
