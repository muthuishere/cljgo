;; clojure.walk/postwalk-demo + prewalk-demo (core/walk.cljg): print each
;; sub-form in walk order, return the form. Output captured with
;; with-out-str so the traversal ORDER is part of the frozen value.
;; oracle (clojure 1.12.5, 2026-07-21):
;;   (with-out-str (postwalk-demo [1 [2]]))
;;     => "Walked: 1\nWalked: 2\nWalked: [2]\nWalked: [1 [2]]\n"
;;   (with-out-str (prewalk-demo [1 [2]]))
;;     => "Walked: [1 [2]]\nWalked: 1\nWalked: [2]\nWalked: 2\n"
(require '[clojure.walk :as w])
[(with-out-str (w/postwalk-demo [1 [2]]))
 (with-out-str (w/prewalk-demo [1 [2]]))]
;; expect: ["Walked: 1\nWalked: 2\nWalked: [2]\nWalked: [1 [2]]\n" "Walked: [1 [2]]\nWalked: 1\nWalked: [2]\nWalked: 2\n"]
