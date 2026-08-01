// notruncate_test.go — the assertion cljgo did not have: a subprocess's
// output arrives COMPLETE and the call returns PROMPTLY.
//
// Two bugs shipped this cycle because nothing checked either property. Both
// were os/exec's documented hazard — "Wait will close the pipe after seeing
// the command exit … it is thus incorrect to call Wait before all reads from
// the pipe have completed" — and both raced a reaping goroutine against the
// reader:
//
//   - cljg.io/exec returned an EMPTY :out for a command that had written to
//     stdout. Silent: a normal-looking map, exit 0, wrong contents.
//   - cljg.process/spawn's readable handle failed with "file already closed"
//     mid-stream.
//
// Neither reproduced locally. Both passed every run on this machine and went
// red only on slower CI, which is precisely what a race does — and it is why
// the existing tests missed them: they assert on output small enough to fit
// in a pipe buffer and complete in one read, where the race window never
// opens.
//
// So these tests use output far larger than a pipe buffer (64 KiB on Linux,
// 16 KiB typical on macOS), forcing many reads and keeping the reader busy
// while the child exits — the window where a Wait-side close truncates. Each
// asserts the EXACT expected byte count, because "some output" is exactly the
// assertion that let a truncation through, and each is wall-clock bounded so
// a hang is a failure rather than a timeout of the whole suite.
//
// What these tests DO and DO NOT prove, measured rather than assumed. Reverting
// io_proc.go to cmd.StdoutPipe/StderrPipe — the exact defect — and re-running
// on this machine: they PASS, three times over. The race simply does not open
// on a fast reader. Adding a 1 ms sleep before each drain read, which is what a
// slower machine amounts to, they FAIL immediately and precisely:
//
//	exec large stdout = [167936 0], want [208894 0] — output was truncated
//
// So the assertion is right and it does catch the defect, but only where the
// race actually fires. On fast hardware it will sit green over a genuinely
// broken implementation. That is not a flaw to paper over — it is the same
// property that let both bugs ship, and it means CI on slower runners, not this
// gate, is what actually guards these. Treat a green here as necessary and
// never sufficient.
package bri_test

import (
	"fmt"
	"strings"
	"testing"
	"time"
)

// bigCount is chosen so the child's stdout (~10 bytes/line) far exceeds any
// platform's pipe buffer, guaranteeing the reader must loop.
const bigCount = 20000

// wantBigBytes is the exact size of `count bigCount` output: "line-N\n" for
// N in 1..bigCount.
func wantBigBytes() int {
	n := 0
	for i := 1; i <= bigCount; i++ {
		n += len(fmt.Sprintf("line-%d\n", i))
	}
	return n
}

// withinBudget fails if fn has not returned by budget. The budget is
// deliberately loose — this catches a HANG or a blocked-on-EOF wait, not a
// slow machine.
func withinBudget(t *testing.T, budget time.Duration, what string, fn func()) {
	t.Helper()
	done := make(chan struct{})
	go func() { defer close(done); fn() }()
	select {
	case <-done:
	case <-time.After(budget):
		t.Fatalf("%s did not return within %s", what, budget)
	}
}

// TestExecOutputIsNeverTruncated is the regression guard for the cljg.io/exec
// truncation: a command whose stdout dwarfs the pipe buffer must come back
// whole, with exit 0, promptly.
func TestExecOutputIsNeverTruncated(t *testing.T) {
	d := newDriver(t)
	eval(t, d, `(require '[cljg.io :as io])`)

	want := wantBigBytes()
	withinBudget(t, 60*time.Second, "cljg.io/exec over a large stdout", func() {
		got := eval(t, d, `
    (let [r (io/exec `+helperCmd("count", fmt.Sprint(bigCount))+` `+helperEnv+`)]
      [(count (:out r)) (:exit r)])`)
		v := fmt.Sprint(got)
		if v != fmt.Sprintf("[%d 0]", want) {
			t.Errorf("exec large stdout = %s, want [%d 0] — output was truncated", v, want)
		}
	})
}

// TestExecStdinRoundTripIsNeverTruncated is the same guard on the :in path,
// which is how the truncation was actually noticed: `exec cat` with :in came
// back empty on CI while passing every local run.
func TestExecStdinRoundTripIsNeverTruncated(t *testing.T) {
	d := newDriver(t)
	eval(t, d, `(require '[cljg.io :as io])`)

	// A payload well past any pipe buffer, echoed back by `cat`.
	const lines = 20000
	payload := strings.Repeat("abcdefghijklmnopqrstuvwxyz\n", lines)
	want := len(payload)

	withinBudget(t, 60*time.Second, "cljg.io/exec cat with a large :in", func() {
		got := eval(t, d, `
    (let [r (io/exec `+helperCmd("cat")+` {:env {"GO_WANT_HELPER_PROCESS" "1"}
                                            :in (clojure.string/join (repeat `+fmt.Sprint(lines)+` "abcdefghijklmnopqrstuvwxyz\n"))})]
      [(count (:out r)) (:exit r)])`)
		v := fmt.Sprint(got)
		if v != fmt.Sprintf("[%d 0]", want) {
			t.Errorf("exec cat large :in = %s, want [%d 0] — round trip was truncated", v, want)
		}
	})
}

// TestSpawnStreamIsNeverTruncated is the guard for cljg.process/spawn: the
// reaper calls cmd.Wait() from the moment Start returns, so a caller still
// draining the handle must not have its read end closed underneath it. Reads
// every line and asserts the exact count.
func TestSpawnStreamIsNeverTruncated(t *testing.T) {
	d := newDriver(t)
	eval(t, d, `(require '[cljg.process :as proc] '[cljg.stream :as st])`)

	withinBudget(t, 60*time.Second, "cljg.process/spawn over a large stdout", func() {
		got := eval(t, d, `
    (let [p (proc/spawn `+helperCmd("count", fmt.Sprint(bigCount))+` `+helperEnv+`)]
      (st/close (:in p))
      (let [n (reduce (fn [n _] (inc n)) 0 (st/lines (:out p)))]
        ((:wait p))
        n))`)
		if got != int64(bigCount) {
			t.Errorf("spawn st/lines counted %v, want %d — the stream was truncated", got, bigCount)
		}
	})
}

// TestSpawnExitsPromptlyAfterOutput pins the OTHER half: a child that has
// written everything and exited must be observable as finished without the
// caller blocking. This is the property `:exit-code` exists to answer, and
// the one a closed stdout alone cannot express.
func TestSpawnExitsPromptlyAfterOutput(t *testing.T) {
	d := newDriver(t)
	eval(t, d, `(require '[cljg.process :as proc] '[cljg.stream :as st] '[cljg.system :as sys])`)

	withinBudget(t, 60*time.Second, "spawn drain + exit observation", func() {
		got := evalString(t, d, `
    (let [p (proc/spawn `+helperCmd("count", fmt.Sprint(bigCount))+` `+helperEnv+`)]
      (st/close (:in p))
      (let [body (st/read-all (:out p))]
        ;; Poll for the exit WITHOUT calling :wait, bounded so a regression
        ;; fails fast instead of hanging.
        (loop [n 0]
          (cond
            ((:exit-code p))    (pr-str [(count body) ((:exit-code p)) ((:alive? p))])
            (> n 500)           "never-exited"
            :else (do (sys/sleep 10) (recur (inc n)))))))`)
		want := fmt.Sprintf("[%d 0 false]", wantBigBytes())
		if got != want {
			t.Errorf("spawn drain+exit = %s, want %s", got, want)
		}
	})
}
