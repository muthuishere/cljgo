;; clojure.edn/read with options (core/edn.cljg): :readers overrides/adds
;; tag readers and :default handles unknown tags, exactly as read-string's
;; opts arity does — same edn-strict reader, stream-fed.
;; oracle: skip — Go interop constructs the stream; the JVM equivalent
;;   (java.io.PushbackReader over a StringReader, clojure 1.12.5,
;;   2026-07-21) returns the same values: [:foo 42], [:unknown bar 7].
(require-go '[strings])
(require '[clojure.edn :as edn])
[(edn/read {:readers {'foo (fn [v] [:foo v])}} (strings/NewReader "#foo 42"))
 (edn/read {:default (fn [tag v] [:unknown tag v])} (strings/NewReader "#bar 7"))]
;; expect: [[:foo 42] [:unknown bar 7]]
