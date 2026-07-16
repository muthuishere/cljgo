;; A bare \r ends a `;` line comment, same as \n (pkg/reader/reader.go's
;; skipLine — previously only stopped at \n, so a lone \r line ending
;; swallowed the next form into the comment too). Shared by clojure.core's
;; reader and clojure.edn/read-string alike (not edn-specific). oracle
;; (clojure 1.12.5): (read-string ";foo\r3\n5") => 3 (the comment ends at
;; \r; 3 is the next form, 5 is left unread).
(require '[clojure.edn :as edn])
(edn/read-string ";foo\r3\n5")
;; expect: 3
