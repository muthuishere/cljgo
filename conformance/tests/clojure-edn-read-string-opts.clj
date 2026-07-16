;; clojure.edn/read-string's 2-arg opts form: :eof / :default / :readers
;; (ADR 0022 batch/harness-misc, core/edn.cljg + pkg/eval/misc_builtins.go's
;; -edn-read-string-opts — clojure-test-suite edn_test/read_string.cljc
;; "Reading with Options"). oracle (clojure 1.12.5):
;;   (edn/read-string {:eof :END} "") => :END
;;   (edn/read-string {} " ") throws (no :eof => EOF is an error)
;;   (edn/read-string {:default (fn [_tag v] [:unknown v])} "#foo 42") => [:unknown 42]
;;   (edn/read-string {:default (fn [_tag v] [:unknown v])}
;;                    "#uuid \"550e8400-e29b-41d4-a716-446655440000\"")
;;     => #uuid "550e8400-e29b-41d4-a716-446655440000" — a KNOWN tag still
;;     uses its built-in reader; :default is only the last resort.
;;   (edn/read-string {:readers {'uuid (constantly :override)}}
;;                    "#uuid \"550e8400-e29b-41d4-a716-446655440000\"") => :override
;;     — :readers overrides even a BUILT-IN tag.
;;   (edn/read-string {:readers {'my/a (fn [x] [:a x]) 'my/b (fn [x] [:b x])}}
;;                    "#my/a #my/b 42") => [:a [:b 42]]
(require '[clojure.edn :as edn])
[(edn/read-string {:eof :END} "")
 (try (edn/read-string {} " ") :no-throw (catch Throwable e :threw))
 (edn/read-string {:default (fn [_tag v] [:unknown v])} "#foo 42")
 (edn/read-string {:default (fn [_tag v] [:unknown v])}
                   "#uuid \"550e8400-e29b-41d4-a716-446655440000\"")
 (edn/read-string {:readers {'uuid (constantly :override)}}
                   "#uuid \"550e8400-e29b-41d4-a716-446655440000\"")
 (edn/read-string {:readers {'my/a (fn [x] [:a x]) 'my/b (fn [x] [:b x])}}
                   "#my/a #my/b 42")]
;; expect: [:END :threw [:unknown 42] #uuid "550e8400-e29b-41d4-a716-446655440000" :override [:a [:b 42]]]
