;; clojure.data.json (spike s53, ADR 0096) — native port of org.clojure/
;; data.json 2.5.1. Each behavior below is frozen against the REAL JVM library
;; (Clojure 1.12.5 + data.json 2.5.1), captured with:
;;   clojure -Sdeps '{:deps {org.clojure/data.json {:mvn/version "2.5.1"}}}' \
;;     -M -e "(require '[clojure.data.json :as j]) (prn <form>)"
;; and re-verified against the cljgo port by inlining draft-data_json.cljg
;; ahead of these forms (load-file is not yet implemented in cljgo; the real
;; integration loads the satellite via the Go lib provider — see VERDICT.md).
;;
;; oracle: fresh 2026-07-27 run of the commands above; outputs copied verbatim.
(require '[clojure.data.json :as j])

;; --- read: object + nested array, string keys, mixed scalar types ----------
;; oracle: (j/read-str "{\"a\":1,\"b\":[2,3.5,true,null]}")
(j/read-str "{\"a\":1,\"b\":[2,3.5,true,null]}")
;; expect: {"a" 1, "b" [2 3.5 true nil]}

;; --- read: :key-fn keyword keywordizes object keys -------------------------
;; oracle: (j/read-str "{\"a\": {\"b\": [1, {\"c\": \"d\"}]}}" :key-fn keyword)
(j/read-str "{\"a\": {\"b\": [1, {\"c\": \"d\"}]}}" :key-fn keyword)
;; expect: {:a {:b [1 {:c "d"}]}}

;; --- read: string escapes (\n and \uXXXX) ----------------------------------
;; oracle: (j/read-str "\"hi\\nthere\\u0041\"")
(j/read-str "\"hi\\nthere\\u0041\"")
;; expect: "hi\nthereA"

;; --- read: number literals — negative exponent, BigInt promotion, :bigdec --
;; oracle: [(j/read-str "-12.5e2") (j/read-str "123456789012345678901234567890") (j/read-str "1.5" :bigdec true)]
[(j/read-str "-12.5e2")
 (j/read-str "123456789012345678901234567890")
 (j/read-str "1.5" :bigdec true)]
;; expect: [-1250.0 123456789012345678901234567890N 1.5M]

;; --- read: :eof-error? false returns :eof-value on empty input --------------
;; oracle: (j/read-str "" :eof-error? false :eof-value :none)
(j/read-str "" :eof-error? false :eof-value :none)
;; expect: :none

;; --- read: :value-fn transforms values by (already key-fn'd) key ------------
;; oracle: (j/read-str "{\"a\":1,\"b\":2}" :value-fn (fn [k v] (if (= k "b") (* v 10) v)))
(j/read-str "{\"a\":1,\"b\":2}" :value-fn (fn [k v] (if (= k "b") (* v 10) v)))
;; expect: {"a" 1, "b" 20}

;; --- read: error strings are byte-identical to the JVM ---------------------
;; oracle: [(msg {bad}) (msg [1,]) (msg "") (msg nul)]  (each via .getMessage)
[(try (j/read-str "{bad}") (catch Throwable e (ex-message e)))
 (try (j/read-str "[1,]")  (catch Throwable e (ex-message e)))
 (try (j/read-str "")      (catch Throwable e (ex-message e)))
 (try (j/read-str "nul")   (catch Throwable e (ex-message e)))]
;; expect: ["JSON error (non-string key in object), found `b`, expected `\"`" "JSON error (unexpected character): ]" "JSON error (end-of-file)" "JSON error (expected null)"]

;; --- write: keyword keys via default key-fn, nested vector, scalar mix -----
;; oracle: (j/write-str {:a 1 :b [2 3.5 true nil]})
(j/write-str {:a 1 :b [2 3.5 true nil]})
;; expect: "{\"a\":1,\"b\":[2,3.5,true,null]}"

;; --- write: unicode escaping — default on, :escape-unicode false off -------
;; oracle: [(j/write-str {"name" "josé"}) (j/write-str {"name" "josé"} :escape-unicode false)]
[(j/write-str {"name" "josé"})
 (j/write-str {"name" "josé"} :escape-unicode false)]
;; expect: ["{\"name\":\"jos\\u00e9\"}" "{\"name\":\"josé\"}"]

;; --- write: slash escaping — default on, :escape-slash false off, \t escape -
;; oracle: [(j/write-str "a/b") (j/write-str "a/b" :escape-slash false) (j/write-str "tab\there")]
[(j/write-str "a/b")
 (j/write-str "a/b" :escape-slash false)
 (j/write-str "tab\there")]
;; expect: ["\"a\\/b\"" "\"a/b\"" "\"tab\\there\""]

;; --- write: :indent true, nested map/array -------------------------------
;; oracle: (j/write-str [1 [2 3] {"k" "v"}] :indent true)
(j/write-str [1 [2 3] {"k" "v"}] :indent true)
;; expect: "[\n  1,\n  [\n    2,\n    3\n  ],\n  {\n    \"k\": \"v\"\n  }\n]"

;; --- write: :key-fn transforms keys; ratio serializes as its double --------
;; oracle: [(j/write-str {:z 1 :a 2} :key-fn (fn [k] (str "k_" (name k)))) (j/write-str 1/3)]
[(j/write-str {:z 1 :a 2} :key-fn (fn [k] (str "k_" (name k))))
 (j/write-str 1/3)]
;; expect: ["{\"k_z\":1,\"k_a\":2}" "0.3333333333333333"]

;; --- write: non-string map keys are stringified by default key-fn ----------
;; oracle: (j/write-str {1 "a" 2 "b"})
(j/write-str {1 "a" 2 "b"})
;; expect: "{\"1\":\"a\",\"2\":\"b\"}"

;; --- write: infinite Double is rejected with the oracle message ------------
;; oracle: (try (j/write-str (/ 1.0 0.0)) (catch Throwable e (.getMessage e)))
(try (j/write-str (/ 1.0 0.0)) (catch Throwable e (ex-message e)))
;; expect: "JSON error: cannot write infinite Double"

;; --- deprecated string-based APIs still work -------------------------------
;; oracle: [(j/json-str {:a 1}) (j/read-json "{\"a\":1}") (j/read-json "{\"a\":1}" false)]
[(j/json-str {:a 1})
 (j/read-json "{\"a\":1}")
 (j/read-json "{\"a\":1}" false)]
;; expect: ["{\"a\":1}" {:a 1} {"a" 1}]
