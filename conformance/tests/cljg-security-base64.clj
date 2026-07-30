;; cljg.security/base64-encode + base64-decode-bytes (ADR 0110 ask 5): standard
;; (RFC 4648, padded) base64 over BOTH strings and byte-arrays — base64 without
;; a byte route only solves half, so this pins the byte half too: encoding a
;; byte-array, decoding back to signed bytes, and the round-trip through a
;; payload that is NOT valid UTF-8 (which the string-returning `base64-decode`
;; cannot carry).
;;
;; oracle: clojure 1.12.5.1645, 2026-07-30 (java.util.Base64, the JVM's own):
;;   (.encodeToString (java.util.Base64/getEncoder) (.getBytes "hello")) => "aGVsbG8="
;;   (.encodeToString (java.util.Base64/getEncoder) (.getBytes ""))      => ""
;;   (.encodeToString (java.util.Base64/getEncoder) (byte-array [0 1 2 123])) => "AAECew=="
;;   (String. (.decode (java.util.Base64/getDecoder) "aGVsbG8="))        => "hello"
;;   (vec (.decode (java.util.Base64/getDecoder) "AAECew=="))            => [0 1 2 123]
;;   (vec (.decode (java.util.Base64/getDecoder) "AAECgA=="))            => [0 1 2 -128]
;; Decoded bytes are SIGNED on the JVM (byte[]) and are here too. Invalid input
;; returning nil rather than throwing is cljgo's own pre-existing convention
;; for this family (base64-decode/hex-decode), not a JVM claim.
(require '[cljg.security :as sec])
[(sec/base64-encode "hello")
 (sec/base64-encode "")
 (sec/base64-encode (byte-array [0 1 2 123]))
 (sec/base64-decode "aGVsbG8=")
 (vec (sec/base64-decode-bytes "AAECew=="))
 (vec (sec/base64-decode-bytes "AAECgA=="))
 (bytes? (sec/base64-decode-bytes "aGVsbG8="))
 (= (vec (byte-array [0 1 2 -128]))
    (vec (sec/base64-decode-bytes (sec/base64-encode (byte-array [0 1 2 -128])))))
 (sec/base64-decode-bytes "!!not base64!!")
 (sec/base64 "hello")]
;; expect: ["aGVsbG8=" "" "AAECew==" "hello" [0 1 2 123] [0 1 2 -128] true true nil "aGVsbG8="]
