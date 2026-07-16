;; S20 criterion 4: a zero-broker worker queue — real goroutines + a
;; channel, entirely in interpreted cljgo (`cljgo run workers.clj`).
;; THE PERSISTENCE SEAM: every state transition flows through one
;; `record!` fn. Swap the atom for a Postgres insert (jobs table,
;; Oban model) and the queue is durable; the enqueue/worker API does
;; not change. That seam — not the transport — is the framework.

(def journal (atom []))                       ; the seam: db/insert! goes here
(defn record! [event job]
  (swap! journal conj [event (:type job) (:id job)]))

(def jobs (chan 16))
(def done (chan))

(defn enqueue [job]
  (record! :enqueued job)
  (>! jobs job)
  job)

(defn start-worker [handlers]
  (go (loop []
        (let [job (<! jobs)]
          (if job
            (do (record! :picked job)
                ((get handlers (:type job)) job)
                (record! :done job)
                (recur))
            (>! done :drained))))))

(start-worker
  {:email/welcome (fn [job] (println "welcome mail to" (:to job)))
   :report/build  (fn [job] (println "building report" (:id job)))})

(enqueue {:type :email/welcome :id 1 :to "muthu@example.com"})
(enqueue {:type :report/build  :id 2})
(close! jobs)
(<! done)

(println "journal:" @journal)
;; expect: welcome mail to muthu@example.com
;; expect: building report 2
;; expect: journal: [[:enqueued :email/welcome 1] [:enqueued :report/build 2] [:picked :email/welcome 1] [:done :email/welcome 1] [:picked :report/build 2] [:done :report/build 2]]
