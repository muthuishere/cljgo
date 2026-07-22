;; clojure.core/hash over scalars, matching JVM Clojure 1.12.5's
;; clojure.lang.Util.hasheq byte-for-byte (ADR 0051): integers via
;; Murmur3.hashLong, doubles via Double.hashCode, keyword = symbol +
;; 0x9e3779b9, symbol via Murmur3.hashUnencodedChars, string via
;; String.hashCode, char = code point, booleans/nil as the JVM.
;; Oracle: clojure 1.12.5 (hash x) for each value.
[(hash 1) (hash -1) (hash 0) (hash 42) (hash -100) (hash 9223372036854775807)
 (hash 1.0) (hash -1.5) (hash 3.14) (hash 0.0)
 (hash :a) (hash :foo) (hash :foo/bar) (hash :ns/x)
 (hash 'foo) (hash 'foo/bar)
 (hash "hello") (hash "") (hash \a)
 (hash true) (hash false) (hash nil)]
;; expect: [1392991556 1651860712 0 1871679806 16143486 -2106506049 1072693248 -1074266112 300063655 0 -2123407586 1268894036 -1386151538 2099359606 -1385541733 254379989 1715862179 0 97 1231 1237 0]
