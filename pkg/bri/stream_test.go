// stream_test.go — cljg.stream / cljg.process / cljg.net.http :as :stream
// behavior through the interpreter (ADR 0101, spike s56). No JVM oracle; these
// drive the streaming spawn against this test binary re-invoked as the child
// (TestHelperProcess in io_test.go, guarded by GO_WANT_HELPER_PROCESS — portable,
// no external binary) and the streaming HTTP body against a local httptest
// server (no network — CI-safe). cljgo host behavior with no JVM analog, so the
// frozen expectations are cljgo's OWN output.
package bri_test

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// helperCmd builds a Clojure vector literal that invokes this test binary as
// the portable helper subprocess (mode + args after "--").
func helperCmd(parts ...string) string {
	self := filepath.ToSlash(os.Args[0])
	var b strings.Builder
	b.WriteString("[")
	for _, p := range append([]string{self, "-test.run=TestHelperProcess", "--"}, parts...) {
		fmt.Fprintf(&b, " %q", p)
	}
	b.WriteString("]")
	return b.String()
}

const helperEnv = `{:env {"GO_WANT_HELPER_PROCESS" "1"}}`

func TestCljgProcessSpawnEchoLine(t *testing.T) {
	d := newDriver(t)
	eval(t, d, `(require '[cljg.process :as proc] '[cljg.stream :as st])`)

	// echo a line through :in → :out: write a line into the child's stdin,
	// close it (EOF → cat drains and exits), read the line back from stdout.
	got := evalString(t, d, `
    (let [p (proc/spawn `+helperCmd("cat")+` `+helperEnv+`)]
      (st/write (:in p) "hello world\n")
      (st/close (:in p))
      (let [line (st/read-line (:out p))]
        ((:wait p))
        line))`)
	if got != "hello world" {
		t.Errorf("spawn cat echo-a-line = %q, want %q", got, "hello world")
	}

	// :wait returns the child's exit code (a normal value, non-throwing).
	if code := eval(t, d, `
    (let [p (proc/spawn `+helperCmd("cat")+` `+helperEnv+`)]
      (st/close (:in p))
      (st/read-all (:out p))
      ((:wait p)))`); code != int64(0) {
		t.Errorf("spawn cat :wait = %v, want 0", code)
	}
}

func TestCljgProcessSpawnBidirectionalStream(t *testing.T) {
	d := newDriver(t)
	eval(t, d, `(require '[cljg.process :as proc] '[cljg.stream :as st])`)

	// bidirectional line streaming WITHOUT closing stdin: the child echoes each
	// line UPPER-CASED and flushed immediately, so read-line sees it live.
	got := evalString(t, d, `
    (let [p (proc/spawn `+helperCmd("upcaselines")+` `+helperEnv+`)]
      (st/write-line (:in p) "abc")
      (let [a (st/read-line (:out p))]
        (st/write-line (:in p) "def")
        (let [b (st/read-line (:out p))]
          (st/close (:in p))
          ((:wait p))
          (str a "|" b))))`)
	if got != "ABC|DEF" {
		t.Errorf("bidirectional stream = %q, want %q", got, "ABC|DEF")
	}
}

func TestCljgProcessSpawnKill(t *testing.T) {
	d := newDriver(t)
	eval(t, d, `(require '[cljg.process :as proc] '[cljg.stream :as st])`)

	// :kill force-terminates a long-running child; :wait then returns a
	// non-zero exit (killed), which is a normal value, not a throw.
	code := eval(t, d, `
    (let [p (proc/spawn `+helperCmd("sleep", "60000")+` `+helperEnv+`)]
      ((:kill p))
      ((:wait p)))`)
	if code == int64(0) {
		t.Errorf("killed process :wait = %v, want non-zero", code)
	}
}

func TestCljgStreamReduce(t *testing.T) {
	d := newDriver(t)
	eval(t, d, `(require '[cljg.process :as proc] '[cljg.stream :as st])`)

	// the readable handle IS reducible: reduce over (st/lines out) counts lines,
	// and the handle itself is Seqable (reduce over chunks counts bytes).
	// count mode emits "line-1".."line-5\n" → 5 lines, 5*"line-N\n".
	nlines := eval(t, d, `
    (let [p (proc/spawn `+helperCmd("count", "5")+` `+helperEnv+`)]
      (st/close (:in p))
      (let [n (reduce (fn [n _] (inc n)) 0 (st/lines (:out p)))]
        ((:wait p))
        n))`)
	if nlines != int64(5) {
		t.Errorf("reduce over st/lines = %v, want 5", nlines)
	}

	// into a vector of the lines (transducer/into path over the lazy seq).
	body := evalString(t, d, `
    (let [p (proc/spawn `+helperCmd("count", "3")+` `+helperEnv+`)]
      (st/close (:in p))
      (let [v (into [] (st/lines (:out p)))]
        ((:wait p))
        (clojure.string/join "," v)))`)
	if body != "line-1,line-2,line-3" {
		t.Errorf("into [] st/lines = %q, want line-1,line-2,line-3", body)
	}

	// the handle itself is Seqable → reduce over byte chunks (count total bytes).
	// "line-1\n".."line-4\n" = 4*7 = 28 bytes.
	nbytes := eval(t, d, `
    (let [p (proc/spawn `+helperCmd("count", "4")+` `+helperEnv+`)]
      (st/close (:in p))
      (let [n (reduce (fn [n c] (+ n (count c))) 0 (:out p))]
        ((:wait p))
        n))`)
	if nbytes != int64(28) {
		t.Errorf("reduce byte-count over readable handle = %v, want 28", nbytes)
	}

	// take short-circuits the lazy stream (constant-memory / early stop).
	first2 := evalString(t, d, `
    (let [p (proc/spawn `+helperCmd("count", "100")+` `+helperEnv+`)]
      (st/close (:in p))
      (let [v (into [] (take 2) (st/lines (:out p)))]
        ((:kill p))
        (clojure.string/join "," v)))`)
	if first2 != "line-1,line-2" {
		t.Errorf("take 2 over st/lines = %q, want line-1,line-2", first2)
	}
}

func TestCljgNetHTTPStream(t *testing.T) {
	d := newDriver(t)

	mux := http.NewServeMux()
	mux.HandleFunc("/lines", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(200)
		for i := 1; i <= 4; i++ {
			fmt.Fprintf(w, "row-%d\n", i)
		}
	})
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)

	eval(t, d, `(require '[cljg.net.http :as hc] '[cljg.stream :as st])`)

	// {:as :stream} → :body is a cljg.stream readable handle; reduce over its
	// lines, then close it.
	joined := evalString(t, d, `
    (let [resp (hc/request {:method :get :url "`+srv.URL+`/lines" :as :stream})
          body (:body resp)
          v    (into [] (st/lines body))]
      (st/close body)
      (str (:status resp) "|" (clojure.string/join "," v)))`)
	if joined != "200|row-1,row-2,row-3,row-4" {
		t.Errorf("http :as :stream lines = %q, want 200|row-1,row-2,row-3,row-4", joined)
	}

	// the default (no :as) stays a buffered string body (unchanged behavior).
	if got := evalString(t, d, `(:body (hc/get "`+srv.URL+`/lines"))`); got != "row-1\nrow-2\nrow-3\nrow-4\n" {
		t.Errorf("default buffered body = %q", got)
	}
}
