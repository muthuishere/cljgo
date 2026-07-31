;; ADR 0114 — cljg.io/cljg.process errors read as Clojure, with the Go detail
;; in ex-data. This freezes the FIRST slice: slurp, cljg.io/read-bytes,
;; cljg.io/delete!, cljg.io/mkdirs. Each throws an ex-info whose message
;; names the operation and states the failure once (no doubled op/path, no
;; syscall name), while :op / :reason / :go/error carry the structured data
;; a caller can branch on. :reason is a small, stable, closed keyword set —
;; :not-found / :directory-not-empty / :not-a-directory here — verified
;; against Go's own errno classification (io/fs sentinels + ENOTDIR/ENOTEMPTY).
;;
;; oracle: partial, like cljg-io-bytes.clj — cljg.io/mkdirs and the :reason
;; keyword set are cljgo-native (Clojure/JVM has no mkdirs-throws and no
;; exception-independent reason keyword; the JVM types by EXCEPTION CLASS,
;; which ADR 0114 explicitly declines to add — see "Not asking for" in
;; issue #174). What IS oracle-verified (clojure 1.12.5.1645, 2026-07-31,
;; `clojure -e`):
;;   (slurp "missing.txt") => throws java.io.FileNotFoundException,
;;     message "missing.txt (No such file or directory)", (ex-data e) => nil
;;   (java.nio.file.Files/delete (Paths/get "a-non-empty-dir")) throws
;;     java.nio.file.DirectoryNotEmptyException whose message is just the path
;; i.e. the JVM never puts :reason/:go/error in ex-data for these, and its
;; message repeats the path rather than naming the operation — cljgo's
;; ex-info shape is a deliberate ADR-0114 addition, not a JVM match.
(require '[cljg.io :as io])

(defn probe [thunk]
  (try (thunk) nil
       (catch Throwable e
         {:msg (ex-message e)
          :op (:op (ex-data e))
          :reason (:reason (ex-data e))
          :go-error? (string? (:go/error (ex-data e)))})))

(def d (io/temp-dir))
(def missing (io/path d "no-such-174.txt"))
(def slurp-r (probe #(slurp missing)))
(def read-bytes-r (probe #(io/read-bytes missing)))

(io/mkdirs (io/path d "full"))
(spit (io/path d "full" "f.txt") "x")
(def delete-r (probe #(io/delete! (io/path d "full"))))

(def afile (io/path d "afile.txt"))
(spit afile "x")
(def mkdirs-r (probe #(io/mkdirs (io/path afile "sub"))))

(io/delete! afile)
(io/delete-tree! (io/path d "full"))
(io/delete-tree! d)

[slurp-r read-bytes-r delete-r mkdirs-r]
;; expect: [{:msg "slurp: no such file or directory", :op :fs/read, :reason :not-found, :go-error? true} {:msg "cljg.io/read-bytes: no such file or directory", :op :fs/read, :reason :not-found, :go-error? true} {:msg "cljg.io/delete!: directory is not empty", :op :fs/delete, :reason :directory-not-empty, :go-error? true} {:msg "cljg.io/mkdirs: not a directory", :op :fs/mkdir, :reason :not-a-directory, :go-error? true}]
