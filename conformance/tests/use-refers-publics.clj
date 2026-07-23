;; use = require + refer: a bare (use 'clojure.set) makes the namespace's
;; publics callable unqualified.
;; oracle (Clojure 1.12.5 CLI, 2026-07-23): (use 'clojure.set) => nil;
;; (union #{1} #{2}) => #{1 2}
(use 'clojure.set)
(union #{1} #{2})
;; expect: #{1 2}
