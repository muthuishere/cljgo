;; with-open (fundamentals batch 1): binds resources left-to-right, runs
;; the body, closes every resource on the way out — throw included — in
;; nested finally blocks. Structure and semantics oracle-verified on the
;; JVM (clojure 1.12.5): (macroexpand-1 '(with-open [a x b y] body)) =>
;; (let [a x] (try (with-open [b y] body) (finally (. a close))));
;; close order over two resources => [:b :a]; a throwing body still
;; closes; (with-open [] :nothing) => :nothing.
;; oracle: skip — the closeable here is a cljgo channel (-close-resource:
;; io.Closer or channel; JVM with-open reflects on .close instead).
;; A read on a buffered-empty OPEN channel would block forever, so the
;; nil reads below PROVE each channel was closed.
(def c1 (chan 1))
(def c2 (chan 1))
(def v (with-open [a c1 b c2] (>! a 1) (<! a)))
(def c3 (chan 1))
(def thrown (try (with-open [x c3] (throw (ex-info "boom" {})))
                 (catch Exception _e :thrown)))
[v (<! c2) (<! c1) thrown (<! c3) (with-open [] :nothing)]
;; expect: [1 nil nil :thrown nil :nothing]
