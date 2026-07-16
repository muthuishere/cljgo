;; A deftype/defrecord that DECLARES a protocol with no method forms
;; still satisfies it (ADR 0039; the -type-marker builtin registers the
;; declaration). Previously cljgo's macros dropped a method-less protocol
;; entirely, so satisfies? answered false.
;; oracle: this EXACT file evaluated against clojure 1.12.5 (clojure CLI,
;; 2026-07-17) prints the same vector.
(defprotocol Marker)
(defprotocol Other)
(defrecord MRec [] Marker)
(deftype MTyp [] Marker)
[(satisfies? Marker (->MRec))
 (satisfies? Marker (->MTyp))
 (satisfies? Other (->MRec))
 (satisfies? Other (->MTyp))]
;; expect: [true true false false]
