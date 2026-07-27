;; cljg.security digests + codecs (ADR 0103): sha256 and hmac against the
;; STANDARD test vectors — sha256("abc") is the FIPS 180-2 vector, and the
;; HMAC-SHA256 of "The quick brown fox jumps over the lazy dog" with key "key"
;; is the canonical RFC-cited example (both reproduced with `shasum -a 256` /
;; `openssl dgst -sha256 -hmac key`, 2026-07-28) — plus base64/hex round-trips
;; and the nil-on-invalid decode contract.
;;
;; oracle: skip — cljg.security is a cljgo host namespace with no JVM package,
;; so this file cannot run through the `clojure` CLI verbatim. The digest
;; values are the published standard vectors (verified against shasum/openssl
;; at authoring time); REPL-vs-binary parity is enforced by the dual harness.
(require '[cljg.security :as sec])
[(sec/sha256 "abc")
 (sec/hmac "key" "The quick brown fox jumps over the lazy dog")
 (sec/base64 "hello, cljgo")
 (sec/base64-decode "aGVsbG8sIGNsamdv")
 (sec/base64-decode "!!!not-base64!!!")
 (sec/hex "abc")
 (sec/hex-decode "616263")
 (sec/hex-decode "zz")]
;; expect: ["ba7816bf8f01cfea414140de5dae2223b00361a396177a9cb410ff61f20015ad" "f7bc83f430538424b13298e6aa6fb143ef4d59a14946175997479dbc2d1a3cd8" "aGVsbG8sIGNsamdv" "hello, cljgo" nil "616263" "abc" nil]
