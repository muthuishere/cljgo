;; abs probes -- kept separate from probes.clj: in cljgo `abs` does not
;; resolve at all (compile-time error), so bundling it with the rest of the
;; matrix would abort every probe after it in the same file.

(defn safe [f]
  (try (pr-str (f)) (catch Exception e (str "THREW:" (ex-message e)))))

(defn p [label f] (println (str label " => " (safe f))))

(p "abs -1"           (fn [] (abs -1)))
(p "abs -1.0"         (fn [] (abs -1.0)))
(p "abs -0.0"         (fn [] (abs -0.0)))
(p "abs ##-Inf"       (fn [] (abs ##-Inf)))
(p "abs -123.456M"    (fn [] (abs -123.456M)))
(p "abs -123N"        (fn [] (abs -123N)))
(p "abs MIN"          (fn [] (abs -9223372036854775808)))
(p "abs -1/5"         (fn [] (abs -1/5)))
(p "abs ##NaN"        (fn [] (abs ##NaN)))
(p "abs nil"          (fn [] (abs nil)))
