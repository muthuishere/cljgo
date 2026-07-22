;; The DEPENDENCY library namespace. This is the stand-in for a published
;; cljgo library that uses FFI: it reaches for purego, the exact module
;; ADR 0044 says a program's go.mod gains "only when that program uses ffi/".
;; Here the program does not — its dependency does.
(ns depffi.core)
(require-go '["github.com/ebitengine/purego" :as purego])
(defn describe []
  (str "dep is impure; purego RTLD_NOW=" purego/RTLD_NOW))
