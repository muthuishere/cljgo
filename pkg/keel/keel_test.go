// keel_test.go — the T1 behavior suite for keel.http / keel.html /
// keel.config (openspec app-framework tasks 1.2–1.7). These behaviors
// have NO JVM oracle (keel does not exist in Clojure 1.12.5), so they
// live here as Go tests against the real interpreter rather than in
// conformance/tests (whose files are oracle-verified; CLAUDE.md).
// The scenarios mirror the spec's own: security is what you didn't
// type, the tutorial curl still works, routing without a router, live
// handlers, bad path param is a 400, page from data, forms carry the
// token, the funnel does not launder type confusion, misconfigured
// deploys must not boot, 2 a.m. debugging.
package keel_test

import (
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/muthuishere/cljgo/pkg/repl"
)

// newDriver boots a fresh driver (repl.New registers keel's lib
// providers) with a clean keel-relevant environment.
func newDriver(t *testing.T) *repl.Driver {
	t.Helper()
	t.Setenv("KEEL_DEV", "")
	return repl.New(nil, io.Discard.(io.Writer), os.Stderr)
}

func eval(t *testing.T, d *repl.Driver, code string) any {
	t.Helper()
	v, err := d.EvalString(code, "keel_test")
	if err != nil {
		t.Fatalf("eval %q: %v", code, err)
	}
	return v
}

func evalString(t *testing.T, d *repl.Driver, code string) string {
	t.Helper()
	s, ok := eval(t, d, code).(string)
	if !ok {
		t.Fatalf("eval %q: expected a string result", code)
	}
	return s
}

// --- keel.http: routing, params, funnel, negotiation ------------------------

const testClientPrelude = `
(require '[keel.http :as http] '[keel.html :as html])
(defn show [req] {:status 200 :body (str "user " (http/param! req :id :int))})
(def routes [["GET /users/{id}" #'show]])
`

// Scenario: routing without a router — {name} binds via ServeMux, a
// wrong method 405s from the stdlib matcher.
func TestRoutingWithoutARouter(t *testing.T) {
	d := newDriver(t)
	eval(t, d, testClientPrelude)
	body := evalString(t, d, `(:body (http/request routes {:method "GET" :path "/users/7"}))`)
	if body != "user 7" {
		t.Fatalf("GET /users/7 body = %q, want \"user 7\"", body)
	}
	if st := eval(t, d, `(:status (http/request routes {:method "POST" :path "/users/7"}))`); st != int64(405) {
		t.Fatalf("POST /users/7 status = %v, want 405", st)
	}
}

// Scenario: bad path param is a 400, not a stack trace — no
// error-handling code in the handler.
func TestBadParamIsA400(t *testing.T) {
	d := newDriver(t)
	eval(t, d, testClientPrelude)
	if st := eval(t, d, `(:status (http/request routes {:method "GET" :path "/users/abc"}))`); st != int64(400) {
		t.Fatalf("GET /users/abc status = %v, want 400 (the funnel's :http/bad-param row)", st)
	}
}

// Scenario: the funnel does not launder type confusion — a bare Result
// response is a loud 500, never a quietly derived status.
func TestBareResultIsALoud500(t *testing.T) {
	d := newDriver(t)
	eval(t, d, `
(require '[keel.http :as http])
(defn bare [_] (ok {:status 200 :body "x"}))
(def routes [["GET /b" #'bare]])
`)
	if st := eval(t, d, `(:status (http/request routes {:method "GET" :path "/b"}))`); st != int64(500) {
		t.Fatalf("bare Result response status = %v, want 500", st)
	}
}

// Scenario: the railway crosses on a visible bridge — http/render maps
// (ok resp) to the response and (err e) through the table.
func TestRenderBridge(t *testing.T) {
	d := newDriver(t)
	eval(t, d, `
(require '[keel.http :as http])
(defn good [_] (http/render (ok {:status 201 :body "made"})))
(defn bad  [_] (http/render (err {:keel/error :db/not-found})))
(def routes [["GET /good" #'good] ["GET /bad" #'bad]])
`)
	if st := eval(t, d, `(:status (http/request routes {:method "GET" :path "/good"}))`); st != int64(201) {
		t.Fatalf("(render (ok ...)) status = %v, want 201", st)
	}
	if st := eval(t, d, `(:status (http/request routes {:method "GET" :path "/bad"}))`); st != int64(404) {
		t.Fatalf("(render (err {:keel/error :db/not-found})) status = %v, want 404", st)
	}
}

// The recover table is overridable data.
func TestRecoverErrorMapOverride(t *testing.T) {
	d := newDriver(t)
	eval(t, d, `
(require '[keel.http :as http])
(defn boom [_] (throw (ex-info "teapot" {:keel/error :app/teapot})))
(def routes [["GET /t" #'boom]])
(def stack (into [(http/recover {:error-map {:app/teapot 418}})]
                 (http/without (http/defaults) :recover)))
`)
	if st := eval(t, d, `(:status (http/request routes {:method "GET" :path "/t"} {:middleware stack}))`); st != int64(418) {
		t.Fatalf("overridden funnel status = %v, want 418", st)
	}
}

// JSON negotiation: a JSON request body parses into :json; a data
// response body encodes as JSON with the content type.
func TestJSONNegotiation(t *testing.T) {
	d := newDriver(t)
	eval(t, d, `
(require '[keel.http :as http])
(defn echo [req] {:status 200 :body {:got (:a (:json req))}})
(def routes [["POST /e" #'echo]])
(def res (http/request routes {:method "POST" :path "/e"
                               :headers {"content-type" "application/json"}
                               :body "{\"a\":41}"}))
`)
	if body := evalString(t, d, `(:body res)`); body != `{"got":41}` {
		t.Fatalf("json echo body = %q", body)
	}
	if ct := evalString(t, d, `(get (:headers res) "content-type")`); ct != "application/json" {
		t.Fatalf("json echo content-type = %q", ct)
	}
}

// --- security: sessions + CSRF ------------------------------------------------

// Scenario: security is what you didn't type + the tutorial curl still
// works + forms carry the token — the full CSRF posture, zero
// middleware code in the app.
func TestCSRFPosture(t *testing.T) {
	d := newDriver(t)
	eval(t, d, `
(require '[keel.http :as http] '[keel.html :as html])
(defn login [_] (http/start-session {:status 200 :body "in"} {:user "m"}))
(defn page-h [_] (http/ok (html/page (html/form {:post "/save"} [:button "go"]))))
(defn save [_] {:status 200 :body "saved"})
(def routes [["POST /login" #'login] ["GET /f" #'page-h] ["POST /save" #'save]])
(def r1 (http/request routes {:method "POST" :path "/login"}))
(def cookie (get (:headers r1) "set-cookie"))
`)
	// Session-bearing mutating POST without a token: rejected.
	if st := eval(t, d, `(:status (http/request routes {:method "POST" :path "/save" :headers {"cookie" cookie}}))`); st != int64(403) {
		t.Fatalf("session-bearing POST without CSRF token = %v, want 403", st)
	}
	// The form mints the token.
	body := evalString(t, d, `
(def r2 (http/request routes {:method "GET" :path "/f" :headers {"cookie" cookie}}))
(:body r2)`)
	m := regexp.MustCompile(`name="__csrf" value="([0-9a-f]+)"`).FindStringSubmatch(body)
	if m == nil {
		t.Fatalf("form did not mint a CSRF token: %s", body)
	}
	// The browser POST with the minted token passes.
	if st := eval(t, d, fmt.Sprintf(`(:status (http/request routes
      {:method "POST" :path "/save"
       :headers {"cookie" cookie "content-type" "application/x-www-form-urlencoded"}
       :body "__csrf=%s"}))`, m[1])); st != int64(200) {
		t.Fatalf("form POST with token = %v, want 200", st)
	}
	// The documented API posture: a sessionless JSON POST passes CSRF.
	if st := eval(t, d, `(:status (http/request routes {:method "POST" :path "/save"
      :headers {"content-type" "application/json"} :body "{}"}))`); st != int64(200) {
		t.Fatalf("sessionless JSON POST = %v, want 200 (nothing to forge without a session)", st)
	}
	// A tampered cookie reads as no session (and therefore passes as sessionless).
	if st := eval(t, d, `(:status (http/request routes {:method "POST" :path "/save"
      :headers {"cookie" "keel-session=forged.deadbeef"}}))`); st != int64(200) {
		t.Fatalf("tampered-cookie POST = %v, want 200 (unsigned cookie = no session)", st)
	}
}

// --- live handlers over a REAL server -------------------------------------------

// Scenario: live handlers — the S20 claim made real through the shipped
// adapter: #'var handlers deref per request, a re-def through the
// evaluator changes the next response on the same running server.
func TestLiveRedefOnARunningServer(t *testing.T) {
	d := newDriver(t)
	eval(t, d, `
(require '[keel.http :as http])
(defn hello [req] {:status 200 :body (str "hello, " (:name (:params req)) " (v1)")})
(def routes [["GET /hello/{name}" #'hello]])
(def srv (http/serve routes {:port 0 :block? false}))
`)
	port := eval(t, d, `(:port srv)`).(int64)
	defer eval(t, d, `((:stop srv))`)

	get := func() string {
		resp, err := http.Get(fmt.Sprintf("http://127.0.0.1:%d/hello/muthu", port))
		if err != nil {
			t.Fatalf("GET: %v", err)
		}
		defer resp.Body.Close()
		b, _ := io.ReadAll(resp.Body)
		return string(b)
	}
	if got := get(); got != "hello, muthu (v1)" {
		t.Fatalf("v1 body = %q", got)
	}
	eval(t, d, `(defn hello [req] {:status 200 :body (str "hello, " (:name (:params req)) " (v2)")})`)
	if got := get(); got != "hello, muthu (v2)" {
		t.Fatalf("after re-def body = %q — the liveness line is broken", got)
	}
}

// Static files through (http/dir ...) on a real server.
func TestStaticDir(t *testing.T) {
	d := newDriver(t)
	pub := t.TempDir()
	if err := os.WriteFile(pub+"/app.css", []byte("body{color:red}"), 0o644); err != nil {
		t.Fatal(err)
	}
	eval(t, d, fmt.Sprintf(`
(require '[keel.http :as http])
(def routes [["GET /static/" (http/dir %q)]])
(def srv (http/serve routes {:port 0 :block? false}))`, pub))
	port := eval(t, d, `(:port srv)`).(int64)
	defer eval(t, d, `((:stop srv))`)
	resp, err := http.Get(fmt.Sprintf("http://127.0.0.1:%d/static/app.css", port))
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	b, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != 200 || string(b) != "body{color:red}" {
		t.Fatalf("static file: %d %q", resp.StatusCode, b)
	}
}

// :drain handles run after shutdown (visible shutdown wiring).
func TestStopDrainsHandles(t *testing.T) {
	d := newDriver(t)
	eval(t, d, `
(require '[keel.http :as http])
(def drained (atom false))
(defn h [_] {:status 200 :body "ok"})
(def srv (http/serve [["GET /" #'h]] {:port 0 :block? false
                                      :drain [(fn [] (reset! drained true))]}))
((:stop srv))
`)
	if v := eval(t, d, `@drained`); v != true {
		t.Fatalf("drain handle did not run on stop: %v", v)
	}
}

// --- keel.html -----------------------------------------------------------------

// Scenario: page from data — escaped by construction; the opt-out is
// explicit and ugly.
func TestHTMLEscapingByConstruction(t *testing.T) {
	d := newDriver(t)
	eval(t, d, `(require '[keel.html :as html])`)
	if got := evalString(t, d, `(html/render [:p "<script>alert(1)</script>"])`); got != "<p>&lt;script&gt;alert(1)&lt;/script&gt;</p>" {
		t.Fatalf("escaping broken: %q", got)
	}
	if got := evalString(t, d, `(html/render [:p (html/unsafe-raw-html "<b>x</b>")])`); got != "<p><b>x</b></p>" {
		t.Fatalf("unsafe-raw-html broken: %q", got)
	}
	if got := evalString(t, d, `(html/render [:p.hint#top "y"])`); got != `<p id="top" class="hint">y</p>` &&
		got != `<p class="hint" id="top">y</p>` {
		t.Fatalf("tag sugar broken: %q", got)
	}
	page := evalString(t, d, `(html/page {:title "t"} [:h1 "hi"])`)
	for _, want := range []string{"<!doctype html>", "<title>t</title>", "<h1>hi</h1>", `href="/static/app.css"`} {
		if !strings.Contains(page, want) {
			t.Fatalf("page missing %q: %s", want, page)
		}
	}
}

// --- keel.config -----------------------------------------------------------------

// Two layers into one plain map: env > profile > file > schema default —
// and `cljgo config`'s explain names the winning layer.
func TestConfigLayers(t *testing.T) {
	d := newDriver(t)
	dir := t.TempDir()
	must(t, os.WriteFile(dir+"/conf.edn", []byte(`{:port 3000
 :db {:host "localhost" :pool-size 5}
 :profiles {:test {:db {:host "test-db"}}}}`), 0o644))
	must(t, os.WriteFile(dir+"/conf.schema.edn", []byte(`{[:port] {:type :int :required true}
 [:greeting] {:type :string :default "hi"}}`), 0o644))
	cwd, _ := os.Getwd()
	must(t, os.Chdir(dir))
	defer os.Chdir(cwd)
	t.Setenv("APP_PROFILE", "test")
	t.Setenv("APP_DB__POOL_SIZE", "9")

	eval(t, d, `(require '[keel.config :as config]) (def cfg (config/load!))`)
	checks := map[string]any{
		`(:port cfg)`:                   int64(3000), // file
		`(get-in cfg [:db :host])`:      "test-db",   // profile overlay
		`(get-in cfg [:db :pool-size])`: int64(9),    // env, __ nesting + _ word join
		`(:greeting cfg)`:               "hi",        // schema default
	}
	for code, want := range checks {
		if got := eval(t, d, code); got != want {
			t.Errorf("%s = %v, want %v", code, got, want)
		}
	}
	explain := evalString(t, d, `(config/explain)`)
	for _, want := range []string{"<- env", "<- file", "<- profile", "<- default"} {
		if !strings.Contains(explain, want) {
			t.Errorf("explain missing %q:\n%s", want, explain)
		}
	}
}

// Scenario: misconfigured deploy must not boot — a schema-required key
// absent from file and env throws before anything starts.
func TestConfigSchemaRefusesToBoot(t *testing.T) {
	d := newDriver(t)
	dir := t.TempDir()
	must(t, os.WriteFile(dir+"/conf.edn", []byte(`{}`), 0o644))
	must(t, os.WriteFile(dir+"/conf.schema.edn", []byte(`{[:port] {:type :int :required true}}`), 0o644))
	cwd, _ := os.Getwd()
	must(t, os.Chdir(dir))
	defer os.Chdir(cwd)

	eval(t, d, `(require '[keel.config :as config])`)
	_, err := d.EvalString(`(config/load!)`, "keel_test")
	if err == nil || !strings.Contains(err.Error(), "[:port]") {
		t.Fatalf("load! with a missing required key: err = %v, want a diagnostic naming [:port]", err)
	}
}

func must(t *testing.T, err error) {
	t.Helper()
	if err != nil {
		t.Fatal(err)
	}
}

// --- perf seam (task 1.7) -----------------------------------------------------

// TestInterpretedHandlerOverhead is the T1 perf budget's CI seam: the
// interpreted var-deref handler vs a native Go handler over the same
// in-process client path. S20 measured 1.6–1.7×; the spec budget is
// ≤ 2× — but wall-clock ratios of two tiny measurements jitter on
// shared runners (same rationale as pkg/emit's gate), so the default
// ceiling is 6× locally and CLJGO_KEEL_PERF_MAX overrides per host.
// The gate exists to catch an adapter regression (per-request
// re-mounting, accidental reflection), which shows up as 10×+.
func TestInterpretedHandlerOverhead(t *testing.T) {
	if testing.Short() {
		t.Skip("perf seam skipped under -short")
	}
	max := 6.0
	if s := os.Getenv("CLJGO_KEEL_PERF_MAX"); s != "" {
		if _, err := fmt.Sscanf(s, "%f", &max); err != nil {
			t.Fatalf("CLJGO_KEEL_PERF_MAX=%q is not a number", s)
		}
	}

	d := newDriver(t)
	eval(t, d, `
(require '[keel.http :as http])
(defn hello [req] {:status 200 :body (str "hello, " (:name (:params req)))})
(def routes [["GET /hello/{name}" #'hello]])
(def srv (http/serve routes {:port 0 :block? false :middleware []}))
`)
	port := eval(t, d, `(:port srv)`).(int64)
	defer eval(t, d, `((:stop srv))`)

	native := http.NewServeMux()
	native.HandleFunc("GET /hello/{name}", func(w http.ResponseWriter, r *http.Request) {
		io.WriteString(w, "hello, "+r.PathValue("name"))
	})
	nativeSrv := &http.Server{Handler: native}
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	go nativeSrv.Serve(ln)
	defer nativeSrv.Close()

	const n = 300
	bench := func(url string) time.Duration {
		// warm-up
		for i := 0; i < 20; i++ {
			resp, err := http.Get(url)
			if err != nil {
				t.Fatal(err)
			}
			io.Copy(io.Discard, resp.Body)
			resp.Body.Close()
		}
		start := time.Now()
		for i := 0; i < n; i++ {
			resp, err := http.Get(url)
			if err != nil {
				t.Fatal(err)
			}
			io.Copy(io.Discard, resp.Body)
			resp.Body.Close()
		}
		return time.Since(start)
	}
	live := bench(fmt.Sprintf("http://127.0.0.1:%d/hello/muthu", port))
	raw := bench(fmt.Sprintf("http://%s/hello/muthu", ln.Addr()))
	ratio := float64(live) / float64(raw)
	t.Logf("interpreted %v/req vs native %v/req = %.2fx (ceiling %.1fx)", live/n, raw/n, ratio, max)
	if ratio > max {
		t.Fatalf("interpreted handler overhead %.2fx exceeds the %.1fx ceiling (CLJGO_KEEL_PERF_MAX)", ratio, max)
	}
}
