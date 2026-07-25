// os_test.go — cljg.os scheduler behavior through the interpreter (ADR 0088).
// The cron next-fire math is covered white-box in os_cron_test.go; here the
// portable scheduler LOOP is driven with a FAKE clock (the -now-millis /
// -sleep-millis shims are stubbed so "sleeping" just advances time), so a
// per-minute job fires :max-ticks times instantly with no real waiting.
package bri_test

import (
	"testing"
)

func TestCljgOsScheduler(t *testing.T) {
	d := newDriver(t)
	eval(t, d, `(require '[cljg.os :as cos])`)

	// cron-next through the interpreter is deterministic future-of-`from`
	if got := eval(t, d, `(let [n (cos/cron-next "* * * * *" 0)] (> n 0))`); got != true {
		t.Errorf("cron-next should return a positive future ms, got %v", got)
	}

	// install a fake clock, then run a per-minute job bounded to 3 ticks
	hits := evalString(t, d, `
	  (clojure.core/in-ns 'cljg.os)
	  (def clock (atom 0))
	  (defn -now-millis [] @clock)
	  (defn -sleep-millis [ms] (swap! clock + ms) nil)   ; "sleeping" advances the fake clock
	  (clojure.core/in-ns 'user)
	  (require '[cljg.os :as cos])
	  (let [hits (atom 0)
	        j (cos/job "* * * * *" (fn [] (swap! hits inc)))]
	    (cos/run [j] {:max-ticks 3})
	    (str @hits))`)
	if hits != "3" {
		t.Errorf("scheduler fired %s times, want 3 (max-ticks)", hits)
	}

	// a job whose fn throws must not kill the loop (other ticks still run)
	survived := evalString(t, d, `
	  (let [ok (atom 0)
	        bad (cos/job "* * * * *" (fn [] (throw (ex-info "boom" {}))) {:name "bad"})
	        good (cos/job "* * * * *" (fn [] (swap! ok inc)))]
	    (with-out-str (binding [*err* *out*]        ; swallow the warning into a discarded string
	      (cos/run [bad good] {:max-ticks 2})))
	    (str @ok))`)
	if survived != "2" {
		t.Errorf("a throwing job killed the scheduler; good job ran %s times, want 2", survived)
	}
}
