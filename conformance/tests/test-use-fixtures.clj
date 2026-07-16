;; clojure.test use-fixtures (ADR 0022 batch/harness-misc): :once wraps the
;; whole namespace run, :each wraps every test — order for 2 deftests is
;; [:once-before :each-before :each-after :each-before :each-after :once-after].
;; harness: eval — clojure.test is interpreted (ADR 0012); no compiled harness
;; oracle: skip — cljgo's clojure.test bootstrap slice; the fixture ORDER
;; below was verified against JVM Clojure 1.12.5 clojure.test
;; (use-fixtures/join-fixtures/test-vars), same log, same summary counts.
(clojure.core/require 'clojure.test)
(clojure.core/refer 'clojure.test)
(def log (atom []))
(use-fixtures :once (fn [f] (swap! log conj :once-before) (f) (swap! log conj :once-after)))
(use-fixtures :each (fn [f] (swap! log conj :each-before) (f) (swap! log conj :each-after)))
(deftest fx-t1 (is (= 1 1)))
(deftest fx-t2 (is (= 2 2)))
(let [s (run-tests 'user)]
  [@log (:test s) (:pass s) (:fail s) (:error s)])
;; expect: [[:once-before :each-before :each-after :each-before :each-after :once-after] 2 2 0 0]
