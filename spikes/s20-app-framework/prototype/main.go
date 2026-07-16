// S20 prototype — proves criteria 1–3 against the REAL cljgo runtime:
//
//  1. LIVE HANDLERS: handlers live behind cljgo vars, derefed per request;
//     re-`def` through the evaluator changes the next response, no restart.
//  2. ROUTES AS DATA: a plain Clojure vector of [pattern var] pairs is
//     walked by this adapter and mounted on Go 1.22+ ServeMux patterns —
//     method match + {name} path params come from the stdlib, no router.
//  3. CONFIG: an EDN file read by the real pkg/reader, overlaid by
//     APP_-prefixed env vars, yielding one plain Clojure map with
//     env > file > default precedence.
//
// This file IS the sketch of the framework's http adapter (keel.http's
// Go half). Throwaway per ADR 0027 — it never merges into pkg/.
package main

import (
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"time"

	"github.com/muthuishere/cljgo/pkg/lang"
	"github.com/muthuishere/cljgo/pkg/reader"
	"github.com/muthuishere/cljgo/pkg/repl"
)

var (
	kwStatus = lang.NewKeyword("status")
	kwBody   = lang.NewKeyword("body")
	kwMethod = lang.NewKeyword("method")
	kwPath   = lang.NewKeyword("path")
	kwParams = lang.NewKeyword("params")
)

func main() {
	failures := 0
	fail := func(name, why string) {
		failures++
		fmt.Printf("FAIL %-28s %s\n", name, why)
	}
	pass := func(name, detail string) {
		fmt.Printf("PASS %-28s %s\n", name, detail)
	}

	// --- boot the real evaluator, load app.clj -------------------------
	d := repl.New(nil, os.Stdout, os.Stderr)
	f, err := os.Open("app.clj")
	if err != nil {
		fmt.Println("cannot open app.clj:", err)
		os.Exit(1)
	}
	if _, err := d.EvalReader(f, "app.clj"); err != nil {
		fmt.Println("eval app.clj:", err)
		os.Exit(1)
	}
	f.Close()

	// --- criterion 2: walk routes-as-data, mount on ServeMux -----------
	userNS := lang.FindNamespace(lang.NewSymbol("user"))
	routesVar := userNS.FindInternedVar(lang.NewSymbol("routes"))
	if routesVar == nil {
		fmt.Println("no #'user/routes")
		os.Exit(1)
	}
	mux := http.NewServeMux()
	nroutes := 0
	for s := lang.Seq(routesVar.Deref()); s != nil; s = lang.Next(s) {
		route := lang.First(s)
		pattern, _ := lang.First(route).(string)
		hv, ok := lang.Get(route, int64(1)).(*lang.Var)
		if !ok {
			fmt.Println("route handler is not a var:", pattern)
			os.Exit(1)
		}
		mux.HandleFunc(pattern, adapt(pattern, hv))
		nroutes++
	}
	pass("routes-as-data", fmt.Sprintf("%d data routes mounted on stdlib ServeMux (method match + {params}, no router engine)", nroutes))

	// --- serve for real -------------------------------------------------
	srv := httptest.NewServer(mux)
	defer srv.Close()

	get := func(path string) (int, string) {
		resp, err := http.Get(srv.URL + path)
		if err != nil {
			return 0, "GET error: " + err.Error()
		}
		defer resp.Body.Close()
		b, _ := io.ReadAll(resp.Body)
		return resp.StatusCode, string(b)
	}

	// criterion 2 continued: path param + method matching via stdlib.
	if code, body := get("/hello/muthu"); code == 200 && body == "hello, muthu (v1)" {
		pass("stdlib-routing", fmt.Sprintf("GET /hello/muthu -> %d %q ({name} bound by ServeMux)", code, body))
	} else {
		fail("stdlib-routing", fmt.Sprintf("got %d %q", code, body))
	}

	// --- criterion 1: live re-def through the evaluator -----------------
	if _, err := d.EvalString(
		`(defn hello [req] {:status 200 :body (str "hello, " (:name (:params req)) " (v2)")})`,
		"live-redef"); err != nil {
		fail("live-redef", "re-def eval failed: "+err.Error())
	} else if code, body := get("/hello/muthu"); code == 200 && body == "hello, muthu (v2)" {
		pass("live-redef", fmt.Sprintf("same server, next request -> %q — var deref per request, zero restart", body))
	} else {
		fail("live-redef", fmt.Sprintf("after re-def got %d %q", code, body))
	}

	// --- liveness overhead: var-deref handler vs native Go handler ------
	native := func(w http.ResponseWriter, r *http.Request) {
		io.WriteString(w, "hello, "+r.PathValue("name")+" (v2)")
	}
	const N = 20000
	rec := func(h http.HandlerFunc) time.Duration {
		req := httptest.NewRequest("GET", "/hello/muthu", nil)
		req.SetPathValue("name", "muthu")
		start := time.Now()
		for i := 0; i < N; i++ {
			h(httptest.NewRecorder(), req)
		}
		return time.Since(start)
	}
	helloVar := userNS.FindInternedVar(lang.NewSymbol("hello"))
	live := rec(adapt("GET /hello/{name}", helloVar))
	raw := rec(native)
	fmt.Printf("INFO %-28s live-var handler %v/req vs native Go %v/req (x%.1f) over %d calls\n",
		"liveness-overhead", live/N, raw/N, float64(live)/float64(raw), N)

	// --- criterion 3: config = EDN file + env overlay, one plain map ----
	os.Setenv("APP_PORT", "9090")           // env beats file
	os.Setenv("APP_DB_HOST", "db.internal") // env beats file, nested key
	cfg, err := loadConfig("conf.edn")      // file beats the default below
	if err != nil {
		fail("config", err.Error())
	} else {
		port := lang.GetDefault(cfg, lang.NewKeyword("port"), int64(3000))
		dbHost := lang.Get(lang.Get(cfg, lang.NewKeyword("db")), lang.NewKeyword("host"))
		dbName := lang.Get(lang.Get(cfg, lang.NewKeyword("db")), lang.NewKeyword("name"))
		if port == int64(9090) && dbHost == "db.internal" && dbName == "app" {
			pass("config", fmt.Sprintf("env > file > default held: port=%v (env), db.host=%v (env), db.name=%v (file)", port, dbHost, dbName))
		} else {
			fail("config", fmt.Sprintf("precedence broken: port=%v db.host=%v db.name=%v", port, dbHost, dbName))
		}
	}

	if failures > 0 {
		os.Exit(1)
	}
}

// adapt is the whole framework adapter: request map in, response map out
// (the Ring contract), handler DEREFED FROM ITS VAR ON EVERY REQUEST.
func adapt(pattern string, hv *lang.Var) http.HandlerFunc {
	params := paramNames(pattern)
	return func(w http.ResponseWriter, r *http.Request) {
		kvs := []any{}
		for _, p := range params {
			kvs = append(kvs, lang.NewKeyword(p), r.PathValue(p))
		}
		req := lang.NewMap(
			kwMethod, r.Method,
			kwPath, r.URL.Path,
			kwParams, lang.NewMap(kvs...),
		)
		res := lang.Apply(hv.Deref(), []any{req}) // <- the liveness line
		status, _ := lang.Get(res, kwStatus).(int64)
		if status == 0 {
			status = 200
		}
		w.WriteHeader(int(status))
		if body, ok := lang.Get(res, kwBody).(string); ok {
			io.WriteString(w, body)
		}
	}
}

// paramNames extracts {name} segments from a ServeMux pattern.
func paramNames(pattern string) []string {
	var names []string
	for _, seg := range strings.Split(pattern, "/") {
		if strings.HasPrefix(seg, "{") && strings.HasSuffix(seg, "}") {
			names = append(names, strings.Trim(seg, "{}"))
		}
	}
	return names
}

// loadConfig reads an EDN file with the REAL cljgo reader and overlays
// APP_-prefixed env vars (APP_DB_HOST -> [:db :host]). Values stay
// strings from env except all-digit ones, which parse as ints — the
// framework would do typed coercion against a declared schema instead.
func loadConfig(path string) (any, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	r := reader.New(strings.NewReader(readAll(f)), reader.WithFilename(path))
	cfg, err := r.ReadOne()
	if err != nil {
		return nil, err
	}
	for _, e := range os.Environ() {
		k, v, _ := strings.Cut(e, "=")
		if !strings.HasPrefix(k, "APP_") {
			continue
		}
		var keys []any
		for _, part := range strings.Split(strings.TrimPrefix(k, "APP_"), "_") {
			keys = append(keys, lang.NewKeyword(strings.ToLower(part)))
		}
		cfg = assocIn(cfg, keys, coerce(v))
	}
	return cfg, nil
}

func assocIn(m any, keys []any, v any) any {
	if len(keys) == 1 {
		return lang.Assoc(m, keys[0], v)
	}
	inner := lang.Get(m, keys[0])
	if inner == nil {
		inner = lang.NewMap()
	}
	return lang.Assoc(m, keys[0], assocIn(inner, keys[1:], v))
}

func coerce(s string) any {
	allDigits := s != ""
	for _, c := range s {
		if c < '0' || c > '9' {
			allDigits = false
			break
		}
	}
	if allDigits {
		var n int64
		fmt.Sscanf(s, "%d", &n)
		return n
	}
	return s
}

func readAll(r io.Reader) string {
	b, _ := io.ReadAll(r)
	return string(b)
}
