;; Oracle probes against REAL core.async on the JVM (Clojure 1.12.5,
;; core.async 1.6.681). Run: bash oracle/run.sh > oracle/transcript.txt
;; Each probe prints a tagged line so the transcript is diffable.
(require '[clojure.core.async :as a
           :refer [chan go thread close! timeout <! >! <!! >!!
                   alts! alts!! put! take! offer! poll!
                   dropping-buffer sliding-buffer promise-chan]])

(defn probe [tag f]
  (print (str tag " => "))
  (flush)
  (try (prn (f))
       (catch Throwable t
         (prn (list :throws (.getName (class t)) (.getMessage t))))))

;; ---- Q5: edge semantics ----
(probe "nil-put->!!" #(>!! (chan 1) nil))
(probe "nil-put-put!" #(put! (chan 1) nil))
(probe "closed-read" #(let [c (chan)] (close! c) (<!! c)))
(probe "closed-read-drains-buffer"
       #(let [c (chan 2)] (>!! c 1) (>!! c 2) (close! c)
              [(<!! c) (<!! c) (<!! c)]))
(probe "closed-put->!!" #(let [c (chan)] (close! c) (>!! c 1)))
(probe "closed-put-put!" #(let [c (chan)] (close! c) (put! c 1)))
(probe "double-close" #(let [c (chan)] (close! c) (close! c)))
(probe "nil-chan-blocks-forever"
       #(let [t (thread (<!! nil))
              [_ p] (alts!! [t (timeout 300)])]
          (if (= p t) :nil-chan-returned :still-blocked-after-300ms)))
(probe "chan-zero" #(chan 0))

;; ---- Q4: park ops outside go ----
(probe "take-outside-go" #(<! (chan 1)))
(probe "put-outside-go" #(>! (chan 1) 1))

;; ---- Q3: timeout ----
(probe "timeout-identical-same-tick"
       #(identical? (timeout 1000) (timeout 1000)))
(probe "timeout-identical-after-gap"
       #(let [t1 (timeout 1000)] (Thread/sleep 50) (identical? t1 (timeout 1000))))
(probe "timeout-closes" #(let [t (timeout 50)] (<!! t)))

;; ---- Q2: alts ----
(probe "alts-default"
       #(let [c (chan)] (first (alts!! [c] :default :none))))
(probe "alts-default-port"
       #(let [c (chan)] (= :default (second (alts!! [c] :default :none)))))
(probe "alts-priority-first-wins"
       #(let [c1 (chan 1) c2 (chan 1)]
          (>!! c1 :a) (>!! c2 :b)
          (first (alts!! [c1 c2] :priority true))))
(probe "alts-write-op"
       #(let [c (chan 1)] (alts!! [[c :v]])))

;; ---- Q1: buffers + transducers ----
(probe "xform-map" #(let [c (chan 3 (map inc))] (>!! c 1) (<!! c)))
(probe "xform-filter-drops"
       #(let [c (chan 3 (filter odd?))]
          (>!! c 2) (>!! c 3) (<!! c)))
(probe "xform-mapcat-expansion"
       #(let [c (chan 2 (mapcat (fn [x] [x x x])))]
          ;; expansion (3 items) exceeds buffer (2): does the put complete?
          (>!! c 1) [(<!! c) (<!! c) (<!! c)]))
(probe "xform-reduced-closes"
       #(let [c (chan 5 (take 2))]
          (>!! c 1) (>!! c 2)
          [(<!! c) (<!! c) (<!! c) (>!! c 3)]))
(probe "xform-unbuffered-chan-throws" #(chan nil (map inc)))
(probe "xform-ex-handler"
       #(let [c (chan 1 (map (fn [x] (/ 1 x))) (fn [_] :handled))]
          (>!! c 0) (>!! c 2) (<!! c)))
(probe "dropping-with-xform"
       #(let [c (chan (dropping-buffer 2) (map inc))]
          (dotimes [i 5] (>!! c i)) (close! c)
          [(<!! c) (<!! c) (<!! c)]))
(probe "sliding-with-xform"
       #(let [c (chan (sliding-buffer 2) (map inc))]
          (dotimes [i 5] (>!! c i)) (close! c)
          [(<!! c) (<!! c) (<!! c)]))

;; ---- go/thread contract ----
(probe "go-result" #(<!! (go 42)))
(probe "go-nil-result" #(<!! (go nil)))
(probe "thread-result" #(<!! (thread 42)))
(probe "go-loop-exists" #(some? (resolve 'clojure.core.async/go-loop)))

;; ---- T1 stragglers ----
(probe "offer-poll"
       #(let [c (chan 1)] [(offer! c 1) (offer! c 2) (poll! c) (poll! c)]))
(probe "promise-chan"
       #(let [p (promise-chan)] (>!! p :v) [(<!! p) (<!! p)]))
(probe "put!-returns-before-taker"
       #(let [c (chan 1)] (put! c :v)))
(probe "take!-callback"
       #(let [c (chan 1) res (promise)]
          (>!! c :v) (take! c (fn [v] (deliver res v))) (deref res 500 :timeout)))

(System/exit 0)
