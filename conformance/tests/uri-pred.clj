;; Batch A3: uri? — true for cljgo's host URI type, net/url's URL
;; ((url/Parse! ...) returns *url.URL — pkg/corelib/host.go's seed
;; registry), the honest Go-host stand-in for the JVM's java.net.URI.
;; oracle: skip — url/Parse! is Go host interop the JVM cannot run; the
;; JVM shape was verified separately 2026-07-23 (clojure 1.12.5):
;; [(uri? (java.net.URI. "http://example.com")) (uri? "http://example.com") (uri? nil)]
;; => [true false false]
(require-go '[net/url])
[(uri? (url/Parse! "http://example.com"))
 (uri? "http://example.com")
 (uri? nil)
 (uri? 5)]
;; expect: [true false false false]
