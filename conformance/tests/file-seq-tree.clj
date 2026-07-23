;; file-seq (fundamentals batch A1): lazy depth-first tree of the
;; directory and everything under it, root first — clojure.core's
;; (tree-seq isDirectory listFiles dir) over the Go host. cljgo
;; represents files as PATH STRINGS (the representation slurp/spit take;
;; cljgo has no java.io.File), children "/"-joined in os.ReadDir's sorted
;; order on every OS. A non-directory path is its own one-element tree.
;; The fixture tree is committed at conformance/fixtures/fileseq-tree/
;; (every harness runs with cwd = the conformance/ dir).
;; oracle: skip — the JVM returns java.io.File objects (a LazySeq, root
;; first, directories included, listFiles order); cljgo's path-string
;; representation is the documented deviation, semantics frozen from the
;; JVM shape (clojure 1.12.5, 2026-07-23:
;; (class (file-seq (java.io.File. "."))) => clojure.lang.LazySeq).
[(file-seq "fixtures/fileseq-tree")
 (file-seq "fixtures/fileseq-tree/b.txt")]
;; expect: [("fixtures/fileseq-tree" "fixtures/fileseq-tree/b.txt" "fixtures/fileseq-tree/sub" "fixtures/fileseq-tree/sub/one.txt") ("fixtures/fileseq-tree/b.txt")]
