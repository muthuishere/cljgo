;; Namespaced map literals (#:ns / #:: auto-resolve) — reader Phase 2.
;; Verified vs clojure 1.12.5:
;;   (:foo/a #:foo{:a 1 :b 2}) => 1, (:foo/b ...) => 2,
;;   :_/d strips to :d, already-qualified :bar/c is kept, #::{...}
;;   auto-resolves to the current ns (user).
[(:foo/a #:foo{:a 1 :b 2})
 (:foo/b #:foo{:a 1 :b 2})
 (:bar/c #:foo{:a 1 :bar/c 3})
 (:d #:foo{:_/d 4})
 (:user/a #::{:a 1})]
;; expect: [1 2 3 4 1]
