;; recur cannot cross a try boundary — an analysis-time error, as on JVM
;; Clojure. Oracle (clojure 1.12.5): Syntax error compiling recur ...
;; "Cannot recur across try".
(loop [i 0]
  (try
    (recur (inc i))
    (catch Exception e nil)))
;; harness: eval — expects an error: analysis error; v0 has no compiled error-output contract
;; expect-error: recur across try
