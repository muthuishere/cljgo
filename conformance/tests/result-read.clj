;; Reader tagged literals (ADR 0014 D4): #cljgo/ok / #cljgo/err /
;; #cljgo/just read back into the tagged values and round-trip through
;; the printer (pr-str). #cljgo/none reads as the none sentinel.
;; oracle: skip — cljgo Result/Option primitive (no JVM reader tag)
;; harness: eval — a tagged-literal CONSTANT has no AOT emit path in v0 (emitter const table is a later milestone); the printer round-trip is verified compiled via the other result-*.clj files
[#cljgo/ok 5 #cljgo/err :bad #cljgo/just 9 #cljgo/none nil]
;; expect: [#cljgo/ok 5 #cljgo/err :bad #cljgo/just 9 none]
