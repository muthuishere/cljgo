;; cljx.test spy / stub / with-spies (ADR 0105): `spy` wraps a var's CURRENT
;; implementation so calls are recorded AND forwarded; `stub` replaces it;
;; `with-spies` installs either for the duration of its body and restores every
;; var on the way out — including when the body throws (it rides with-redefs,
;; whose unwind spike s66 proved identical interpreted and compiled).
;;
;; oracle: skip — cljx.test is a cljgo namespace (ADR 0105's cljx.* developer
;; experience tier) with no JVM analog to freeze byte-for-byte. Frozen against
;; cljgo's own output; REPL-vs-binary parity is enforced by the dual harness.
(require '[cljx.test :as x])

(defn fetch-user [id] {:id id :name "REAL"})
(defn greet [id] (str "Hello, " (:name (fetch-user id)) "!"))

;; spy: records AND forwards to the real fn
(def spy-count (atom nil))
(def spy-args (atom nil))
(def spied-greeting
  (x/with-spies [s (x/spy #'fetch-user)]
    (let [g (greet 7)]
      (greet 8)
      (reset! spy-count (x/call-count s))
      (reset! spy-args (x/calls s))
      g)))
(def restored-after-spy (:name (fetch-user 1)))

;; stub: replaces the implementation, and still records
(def stub-count (atom nil))
(def stubbed-greeting
  (x/with-spies [db (x/stub #'fetch-user (fn [_] {:name "Asha"}))]
    (let [g (greet 1)]
      (reset! stub-count (x/call-count db))
      g)))
(def restored-after-stub (:name (fetch-user 1)))

;; the var is restored even when the body THROWS
(def threw
  (try
    (x/with-spies [db (x/stub #'fetch-user (fn [_] {:name "Never"}))]
      (throw (ex-info "boom" {})))
    (catch Throwable e (ex-message e))))
(def restored-after-throw (:name (fetch-user 1)))

[spied-greeting
 (deref spy-count)
 (deref spy-args)
 restored-after-spy
 stubbed-greeting
 (deref stub-count)
 restored-after-stub
 threw
 restored-after-throw]
;; expect: ["Hello, REAL!" 2 [[7] [8]] "REAL" "Hello, Asha!" 1 "REAL" "boom" "REAL"]
