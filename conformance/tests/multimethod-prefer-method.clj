;; prefer-method / prefers (fundamentals audit 2026-07): a dispatch value
;; deriving from TWO method keys is ambiguous ("neither is preferred");
;; prefer-method resolves it, prefers exposes the preference table, and a
;; contradictory preference throws a conflict.
;; oracle (clojure 1.12.5, 2026-07-21):
;;   ambiguity => "Multiple methods in multimethod 'f2' match dispatch
;;     value: :user/c -> ... and neither is preferred" (pair order is
;;     table-iteration order, so the test freezes a substring probe)
;;   (prefer-method f2 ::a ::b) => f2; then (f2 ::c) => :a
;;   (get (prefers f2) ::a) => #{:user/b}
;;   (prefer-method f2 ::b ::a) => "Preference conflict in multimethod
;;     'f2': :user/a is already preferred to :user/b"
(derive ::c ::a)
(derive ::c ::b)
(defmulti f2 (fn [x] x))
(defmethod f2 ::a [_] :a)
(defmethod f2 ::b [_] :b)
(def ambiguous-msg (try (f2 ::c) (catch Exception e (ex-message e))))
(prefer-method f2 ::a ::b)
[(boolean (re-find #"Multiple methods in multimethod 'f2' match dispatch value: :user/c" ambiguous-msg))
 (boolean (re-find #"and neither is preferred" ambiguous-msg))
 (f2 ::c)
 (get (prefers f2) ::a)
 (= f2 (prefer-method f2 ::a ::x))
 (try (prefer-method f2 ::b ::a) (catch Exception e (ex-message e)))]
;; expect: [true true :a #{:user/b} true "Preference conflict in multimethod 'f2': :user/a is already preferred to :user/b"]
