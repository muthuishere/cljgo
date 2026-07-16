;; Second oracle pass: (1) probes after the pass-1 deadlock (the pass-1
;; xform-ex-handler probe put twice into a 1-buffer before taking — a
;; probe bug, not a core.async finding); (2) a sharper nil-channel probe
;; (pass 1 couldn't distinguish "returned" from "threw" inside thread).
(require '[clojure.core.async :as a
           :refer [chan go thread close! timeout <! >! <!! >!!
                   alts! alts!! put! take! offer! poll! promise-chan]])

(defn probe [tag f]
  (print (str tag " => "))
  (flush)
  (try (prn (f))
       (catch Throwable t
         (prn (list :throws (.getName (class t)) (.getMessage t))))))

(probe "xform-ex-handler"
       #(let [c (chan 1 (map (fn [x] (/ 1 x))) (fn [_] :handled))]
          (>!! c 0) (<!! c)))
(probe "xform-ex-handler-nil-return-skips"
       #(let [c (chan 1 (map (fn [x] (/ 1 x))) (fn [_] nil))]
          (>!! c 0) (>!! c 2) (<!! c)))
(probe "xform-no-ex-handler-throws-where"
       #(let [c (chan 1 (map (fn [x] (/ 1 x))))]
          (try (>!! c 0) :put-returned
               (catch Throwable t (list :put-threw (.getName (class t)))))))

;; nil channel: does (<!! nil) RETURN, THROW, or BLOCK?
(probe "nil-chan-take"
       #(let [t (thread (try [:returned (<!! nil)]
                             (catch Throwable e [:threw (.getName (class e))])))
              [v p] (alts!! [t (timeout 300)])]
          (if (= p t) v :blocked-300ms)))
(probe "nil-chan-put"
       #(let [t (thread (try [:returned (>!! nil 1)]
                             (catch Throwable e [:threw (.getName (class e))])))
              [v p] (alts!! [t (timeout 300)])]
          (if (= p t) v :blocked-300ms)))

(probe "dropping-with-xform"
       #(let [c (chan (a/dropping-buffer 2) (map inc))]
          (dotimes [i 5] (>!! c i)) (close! c)
          [(<!! c) (<!! c) (<!! c)]))
(probe "sliding-with-xform"
       #(let [c (chan (a/sliding-buffer 2) (map inc))]
          (dotimes [i 5] (>!! c i)) (close! c)
          [(<!! c) (<!! c) (<!! c)]))

(probe "go-result" #(<!! (go 42)))
(probe "go-nil-result" #(<!! (go nil)))
(probe "thread-result" #(<!! (thread 42)))
(probe "go-loop-exists" #(some? (resolve 'clojure.core.async/go-loop)))

(probe "offer-poll"
       #(let [c (chan 1)] [(offer! c 1) (offer! c 2) (poll! c) (poll! c)]))
(probe "offer-on-unbuffered-no-taker" #(offer! (chan) 1))
(probe "promise-chan"
       #(let [p (promise-chan)] (>!! p :v) [(<!! p) (<!! p)]))
(probe "promise-chan-put-after-first"
       #(let [p (promise-chan)] (>!! p :a) (>!! p :b) [(<!! p) (<!! p)]))
(probe "put!-returns-before-taker"
       #(let [c (chan 1)] (put! c :v)))
(probe "take!-callback"
       #(let [c (chan 1) res (promise)]
          (>!! c :v) (take! c (fn [v] (deliver res v))) (deref res 500 :timeout)))

;; timeout write behavior (is a timeout chan a normal chan?)
(probe "timeout-put" #(>!! (timeout 5000) :v))

;; alts! inside go with :priority + :default combined
(probe "alts-priority-and-default"
       #(let [c (chan)] (first (alts!! [c] :default :dflt :priority true))))

(System/exit 0)
