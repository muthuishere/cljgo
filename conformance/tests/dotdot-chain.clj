;; `..` — clojure.core's chained member-access macro (core/core.clj), over
;; cljgo's Go-interop member sugar (ADR 0010, design/05 §1): each step's
;; result is the next receiver, left to right. All three member shapes chain:
;; `(m arg...)`, bare-symbol `m` (zero-arg call), and `-Field` access.
;; url.URL.Query() returns url.Values whose .Get reads a query param — a real
;; two-method chain; strings.NewReplacer chains construction into .Replace.
;; JVM semantics verified against the oracle with JVM receivers (clojure
;; 1.12.5, 2026-07-23): (.. "abc" (length)) => 3; (.. "ab" (toUpperCase)
;; (charAt 1)) => \B; bare member (.. "ab" toUpperCase (charAt 0)) => \A;
;; (macroexpand-1 '(.. x (m1) (m2 a))) => (.. (. x (m1)) (m2 a)).
;; oracle: skip — Go interop has no JVM Clojure equivalent (Go stdlib is the oracle)
(require-go '[strings])
(require-go '[net/url])
(def u (url/URL. {:Scheme "https" :Host "x" :RawQuery "a=1&b=2"}))
[(.. u (Query) (Get "a"))
 (.. u (Query) (Get "b"))
 (.. u (String))
 (.. u String)
 (.. u -Host)
 (.. (strings/NewReplacer "a" "1" "b" "2") (Replace "abcab"))]
;; expect: ["1" "2" "https://x?a=1&b=2" "https://x?a=1&b=2" "x" "12c12"]
