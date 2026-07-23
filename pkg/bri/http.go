// http.go — bri.http's Go half: the routes→ServeMux adapter (grown
// from spike S20's prototype/main.go), the http.Server with production
// timeouts + SIGTERM drain, the in-process test client, and the small
// host primitives (JSON, form decode, HMAC, tokens, base64, clock,
// env) interned as :private vars into the bri.http namespace.
//
// The Ring contract at this boundary (design/05 shaping discipline —
// one conversion, both directions, no reflection surprises):
//
//	request map:  :request-method (keyword) :uri :query-string
//	              :headers {lowercase-string string} :body (string)
//	              :params {:name "string"} (ServeMux {name} segments)
//	              :query-params {:name "string"} :remote-addr
//	response map: :status (int) :headers {string string}
//	              :body (string)
package bri

import (
	"context"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"os/signal"
	"sort"
	"strings"
	"syscall"
	"time"

	"github.com/muthuishere/cljgo/pkg/lang"
)

var (
	kwStatus        = lang.NewKeyword("status")
	kwHeaders       = lang.NewKeyword("headers")
	kwBody          = lang.NewKeyword("body")
	kwRequestMethod = lang.NewKeyword("request-method")
	kwURI           = lang.NewKeyword("uri")
	kwQueryString   = lang.NewKeyword("query-string")
	kwParams        = lang.NewKeyword("params")
	kwQueryParams   = lang.NewKeyword("query-params")
	kwRemoteAddr    = lang.NewKeyword("remote-addr")
	kwPort          = lang.NewKeyword("port")
	kwDrain         = lang.NewKeyword("drain")
	kwBlock         = lang.NewKeyword("block?")
	kwStop          = lang.NewKeyword("stop")
	kwMethod        = lang.NewKeyword("method")
	kwPath          = lang.NewKeyword("path")
	kwDirMarker     = lang.NewKeyword("bri.http/dir")
	kwPathParams    = lang.NewKeyword("path-params")
	kwRoutePattern  = lang.NewKeyword("bri.http/route")
)

// installHTTPShims interns bri.http's private Go primitives.
func installHTTPShims(def func(name string, fn func(args ...any) any)) {
	def("-serve", serveShim)
	def("-request", requestShim)
	def("-json-encode", func(args ...any) any { return jsonEncode(one("-json-encode", args)) })
	def("-json-decode", func(args ...any) any { return jsonDecode(one("-json-decode", args)) })
	def("-form-decode", func(args ...any) any { return formDecode(one("-form-decode", args)) })
	def("-hmac-sign", func(args ...any) any {
		if len(args) != 2 {
			panic(fmt.Errorf("wrong number of args (%d) passed to: -hmac-sign", len(args)))
		}
		return hmacSign(asString(args[0]), asString(args[1]))
	})
	def("-rand-token", func(args ...any) any { return randToken() })
	def("-const-eq", func(args ...any) any {
		if len(args) != 2 {
			panic(fmt.Errorf("wrong number of args (%d) passed to: -const-eq", len(args)))
		}
		a, aok := args[0].(string)
		b, bok := args[1].(string)
		return aok && bok && subtle.ConstantTimeCompare([]byte(a), []byte(b)) == 1
	})
	def("-b64-encode", func(args ...any) any {
		return base64.RawURLEncoding.EncodeToString([]byte(asString(one("-b64-encode", args))))
	})
	def("-b64-decode", func(args ...any) any {
		b, err := base64.RawURLEncoding.DecodeString(asString(one("-b64-decode", args)))
		if err != nil {
			panic(fmt.Errorf("-b64-decode: %w", err))
		}
		return string(b)
	})
	def("-now-millis", func(args ...any) any { return time.Now().UnixMilli() })
	def("-getenv", getenvShim)
	def("-result-payload", func(args ...any) any { return lang.ResultPayload(one("-result-payload", args)) })
	// observability: metrics registry (observability.go)
	def("-metrics-observe", func(args ...any) any {
		if len(args) != 3 {
			panic(fmt.Errorf("wrong number of args (%d) passed to: -metrics-observe", len(args)))
		}
		metricsObserve(asString(args[0]), asInt(args[1]), asFloat(args[2]))
		return nil
	})
	def("-metrics-render", func(args ...any) any { return metricsRender() })
	def("-metrics-reset", func(args ...any) any { metricsReset(); return nil })
	// client identity: proxy-aware IP resolver (client-ip.go)
	def("-client-ip", func(args ...any) any {
		if len(args) != 4 {
			panic(fmt.Errorf("wrong number of args (%d) passed to: -client-ip", len(args)))
		}
		return clientIP(asString(args[0]), asString(args[1]), asString(args[2]), args[3])
	})
	// reverse routing (path-for / url-for)
	def("-url-encode", func(args ...any) any { return url.QueryEscape(asString(one("-url-encode", args))) })
	def("-path-escape", func(args ...any) any { return url.PathEscape(asString(one("-path-escape", args))) })
}

// nowMillis is the shared clock the observability shims read.
func nowMillis() int64 { return time.Now().UnixMilli() }

// installConfigShims interns bri.config's private Go primitives.
func installConfigShims(def func(name string, fn func(args ...any) any)) {
	def("-read-file", func(args ...any) any {
		b, err := os.ReadFile(asString(one("-read-file", args)))
		if err != nil {
			return nil
		}
		return string(b)
	})
	def("-env-pairs", func(args ...any) any {
		env := os.Environ()
		sort.Strings(env) // deterministic overlay order
		pairs := make([]any, 0, len(env))
		for _, e := range env {
			k, v, _ := strings.Cut(e, "=")
			pairs = append(pairs, lang.NewVector(k, v))
		}
		return lang.NewVectorOwning(pairs)
	})
	def("-getenv", getenvShim)
}

func getenvShim(args ...any) any {
	v, ok := os.LookupEnv(asString(one("-getenv", args)))
	if !ok {
		return nil
	}
	return v
}

func one(name string, args []any) any {
	if len(args) != 1 {
		panic(fmt.Errorf("wrong number of args (%d) passed to: %s", len(args), name))
	}
	return args[0]
}

func asString(v any) string {
	s, ok := v.(string)
	if !ok {
		panic(fmt.Errorf("expected a string, got: %s", lang.PrintString(v)))
	}
	return s
}

// --- mux construction -------------------------------------------------------

// buildMux mounts the [pattern handler] pairs bri.http/mount compiled
// (handlers are fully-wrapped IFns; {:bri.http/dir path} markers become
// stdlib file servers) onto a Go 1.22+ ServeMux — the stdlib does the
// routing (method match, {name} params); we build no router.
func buildMux(mounted any) *http.ServeMux {
	mux := http.NewServeMux()
	for s := lang.Seq(mounted); s != nil; s = lang.Next(s) {
		route := lang.First(s)
		pattern := asString(lang.First(route))
		h := lang.Get(route, int64(1))
		if dir := lang.Get(h, kwDirMarker); dir != nil {
			prefix := pathOfPattern(pattern)
			mux.Handle(pattern, http.StripPrefix(strings.TrimSuffix(prefix, "/"),
				http.FileServer(http.Dir(asString(dir)))))
			continue
		}
		ifn, ok := h.(lang.IFn)
		if !ok {
			panic(fmt.Errorf("bri.http: route %q handler is not callable: %s", pattern, lang.PrintString(h)))
		}
		mux.HandleFunc(pattern, adapt(pattern, ifn))
	}
	return mux
}

// pathOfPattern returns the path part of a "METHOD /path" ServeMux
// pattern (or the whole pattern when no method prefix is present).
func pathOfPattern(pattern string) string {
	if i := strings.Index(pattern, " "); i >= 0 {
		return pattern[i+1:]
	}
	return pattern
}

// paramNames extracts {name} segments from a ServeMux pattern ({$} is
// the end-anchor, not a param; {path...} binds as "path").
func paramNames(pattern string) []string {
	var names []string
	for _, seg := range strings.Split(pathOfPattern(pattern), "/") {
		if strings.HasPrefix(seg, "{") && strings.HasSuffix(seg, "}") {
			name := strings.Trim(seg, "{}")
			if name == "$" {
				continue
			}
			names = append(names, strings.TrimSuffix(name, "..."))
		}
	}
	return names
}

// adapt is the request/response boundary: request map in, response map
// out (the Ring contract). The handler arrives fully wrapped — var
// deref (the liveness line) and middleware happen in bri.http/mount's
// Clojure closures; this side only converts and writes.
func adapt(pattern string, ifn lang.IFn) http.HandlerFunc {
	names := paramNames(pattern)
	return func(w http.ResponseWriter, r *http.Request) {
		defer func() {
			// The :recover middleware catches handler errors; this last-resort
			// recover covers a custom stack that removed it (and adapter bugs)
			// so one request can never kill the server loop silently.
			if rec := recover(); rec != nil {
				fmt.Fprintf(os.Stderr, "bri.http: unrecovered handler panic on %s %s: %v\n",
					r.Method, r.URL.Path, rec)
				w.WriteHeader(http.StatusInternalServerError)
				io.WriteString(w, "internal error")
			}
		}()
		res := lang.Apply(ifn, []any{requestMap(r, names, pattern)})
		writeResponse(w, res)
	}
}

func requestMap(r *http.Request, paramNames []string, pattern string) any {
	params := []any{}
	for _, p := range paramNames {
		params = append(params, lang.NewKeyword(p), r.PathValue(p))
	}
	headers := []any{}
	for k, vs := range r.Header {
		headers = append(headers, strings.ToLower(k), strings.Join(vs, ", "))
	}
	query := []any{}
	for k, vs := range r.URL.Query() {
		if len(vs) > 0 {
			query = append(query, lang.NewKeyword(k), vs[0])
		}
	}
	var body string
	if r.Body != nil {
		b, _ := io.ReadAll(io.LimitReader(r.Body, 10<<20)) // 10 MiB request cap
		body = string(b)
	}
	pm := lang.NewMap(params...)
	return lang.NewMap(
		kwRequestMethod, lang.NewKeyword(strings.ToLower(r.Method)),
		kwURI, r.URL.Path,
		kwQueryString, r.URL.RawQuery,
		kwHeaders, lang.NewMap(headers...),
		kwParams, pm,
		kwPathParams, pm, // Compojure-ish alias; :params kept for back-compat
		kwQueryParams, lang.NewMap(query...),
		kwBody, body,
		kwRemoteAddr, r.RemoteAddr,
		kwRoutePattern, pattern, // low-cardinality label for metrics/logging
	)
}

func writeResponse(w http.ResponseWriter, res any) {
	status := 200
	switch v := lang.Get(res, kwStatus).(type) {
	case int64:
		status = int(v)
	case int:
		status = v
	}
	for s := lang.Seq(lang.Get(res, kwHeaders)); s != nil; s = lang.Next(s) {
		entry := lang.First(s)
		k := lang.First(entry)
		v := lang.Get(entry, int64(1))
		name, nok := k.(string)
		val, vok := v.(string)
		if nok && vok {
			w.Header().Set(name, val)
		}
	}
	w.WriteHeader(status)
	switch b := lang.Get(res, kwBody).(type) {
	case string:
		io.WriteString(w, b)
	case []byte:
		w.Write(b)
	case nil:
	default:
		io.WriteString(w, lang.ToString(b))
	}
}

// --- serve -------------------------------------------------------------------

// serveShim is bri.http/-serve: mount, listen, and (by default) block
// until SIGTERM/SIGINT, then drain — in-flight requests get a deadline,
// then each handle in :drain is invoked (shutdown wiring is ON THE
// PAGE, spec: no ambient shutdown registry). Production timeouts are
// DEFAULT ON. :block? false returns {:port N :stop (fn)} for tests and
// REPL sessions.
func serveShim(args ...any) any {
	if len(args) != 2 {
		panic(fmt.Errorf("wrong number of args (%d) passed to: -serve", len(args)))
	}
	mounted, opts := args[0], args[1]
	mux := buildMux(mounted)

	port := 0
	switch v := lang.Get(opts, kwPort).(type) {
	case int64:
		port = int(v)
	case int:
		port = v
	case nil:
		panic(fmt.Errorf("bri.http/serve: no :port in opts"))
	default:
		panic(fmt.Errorf("bri.http/serve: :port must be an int, got: %s", lang.PrintString(v)))
	}

	ln, err := net.Listen("tcp", fmt.Sprintf(":%d", port))
	if err != nil {
		panic(fmt.Errorf("bri.http/serve: %w", err))
	}
	actual := ln.Addr().(*net.TCPAddr).Port

	srv := &http.Server{
		Handler: mux,
		// Production timeouts default ON (ADR 0041: the safe stack is
		// what you didn't type).
		ReadHeaderTimeout: 10 * time.Second,
		ReadTimeout:       30 * time.Second,
		WriteTimeout:      30 * time.Second,
		IdleTimeout:       60 * time.Second,
	}

	drain := func() {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		_ = srv.Shutdown(ctx) // in-flight requests finish (deadline), listener closes
		for s := lang.Seq(lang.Get(opts, kwDrain)); s != nil; s = lang.Next(s) {
			if h, ok := lang.First(s).(lang.IFn); ok {
				h.Invoke()
			}
		}
	}

	fmt.Printf("bri: listening on http://localhost:%d\n", actual)

	if block, ok := lang.Get(opts, kwBlock).(bool); ok && !block {
		go func() { _ = srv.Serve(ln) }()
		return lang.NewMap(
			kwPort, int64(actual),
			kwStop, lang.NewFnFunc(func(args ...any) any { drain(); return nil }),
		)
	}

	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, os.Interrupt, syscall.SIGTERM)
	errCh := make(chan error, 1)
	go func() { errCh <- srv.Serve(ln) }()
	select {
	case <-sigCh:
		signal.Stop(sigCh)
		fmt.Println("bri: shutting down (draining in-flight requests, then :drain handles)")
		drain()
	case err := <-errCh:
		if err != nil && err != http.ErrServerClosed {
			panic(fmt.Errorf("bri.http/serve: %w", err))
		}
	}
	return nil
}

// requestShim is bri.http/-request: the in-process test client — the
// same mount path as serve, no socket, an httptest recorder.
func requestShim(args ...any) any {
	if len(args) != 2 {
		panic(fmt.Errorf("wrong number of args (%d) passed to: -request", len(args)))
	}
	mounted, reqMap := args[0], args[1]
	mux := buildMux(mounted)

	method := "GET"
	if m, ok := lang.Get(reqMap, kwMethod).(string); ok {
		method = strings.ToUpper(m)
	}
	path := "/"
	if p, ok := lang.Get(reqMap, kwPath).(string); ok {
		path = p
	}
	var body io.Reader
	if b, ok := lang.Get(reqMap, kwBody).(string); ok {
		body = strings.NewReader(b)
	}
	req := httptest.NewRequest(method, path, body)
	if ra, ok := lang.Get(reqMap, kwRemoteAddr).(string); ok && ra != "" {
		req.RemoteAddr = ra // let tests drive client-ip / rate-limit / auto-ban keying
	}
	for s := lang.Seq(lang.Get(reqMap, kwHeaders)); s != nil; s = lang.Next(s) {
		entry := lang.First(s)
		k, kok := lang.First(entry).(string)
		v, vok := lang.Get(entry, int64(1)).(string)
		if kok && vok {
			req.Header.Set(k, v)
		}
	}
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)

	headers := []any{}
	for k, vs := range rec.Result().Header {
		headers = append(headers, strings.ToLower(k), strings.Join(vs, ", "))
	}
	return lang.NewMap(
		kwStatus, int64(rec.Code),
		kwHeaders, lang.NewMap(headers...),
		kwBody, rec.Body.String(),
	)
}

// --- JSON / form -------------------------------------------------------------

// jsonEncode renders cljgo data as JSON: map keys and keyword values by
// name, vectors/seqs as arrays — the one shaping, both directions.
func jsonEncode(v any) string {
	b, err := json.Marshal(toJSONValue(v))
	if err != nil {
		panic(fmt.Errorf("-json-encode: %w", err))
	}
	return string(b)
}

func toJSONValue(v any) any {
	switch t := v.(type) {
	case nil, bool, string, int64, int, float64:
		return t
	case lang.Keyword:
		return keywordName(t)
	case *lang.Symbol:
		return t.FullName()
	}
	if m, ok := v.(lang.IPersistentMap); ok {
		out := map[string]any{}
		for s := lang.Seq(m); s != nil; s = lang.Next(s) {
			entry := lang.First(s)
			k := lang.First(entry)
			val := lang.Get(entry, int64(1))
			var key string
			switch kt := k.(type) {
			case lang.Keyword:
				key = keywordName(kt)
			case string:
				key = kt
			default:
				key = lang.ToString(kt)
			}
			out[key] = toJSONValue(val)
		}
		return out
	}
	if s := lang.Seq(v); s != nil || lang.Count(v) == 0 {
		out := []any{}
		for ; s != nil; s = lang.Next(s) {
			out = append(out, toJSONValue(lang.First(s)))
		}
		return out
	}
	return lang.ToString(v)
}

// jsonDecode parses JSON into cljgo data: object keys become keywords,
// integral numbers become int64 (JVM parity: longs for whole numbers).
func jsonDecode(v any) any {
	s, ok := v.(string)
	if !ok {
		panic(fmt.Errorf("-json-decode expects a string, got: %s", lang.PrintString(v)))
	}
	var parsed any
	dec := json.NewDecoder(strings.NewReader(s))
	dec.UseNumber()
	if err := dec.Decode(&parsed); err != nil {
		panic(fmt.Errorf("-json-decode: %w", err))
	}
	return fromJSONValue(parsed)
}

func fromJSONValue(v any) any {
	switch t := v.(type) {
	case map[string]any:
		kvs := make([]any, 0, len(t)*2)
		for k, val := range t {
			kvs = append(kvs, lang.NewKeyword(k), fromJSONValue(val))
		}
		return lang.NewMap(kvs...)
	case []any:
		vals := make([]any, len(t))
		for i, val := range t {
			vals[i] = fromJSONValue(val)
		}
		return lang.NewVectorOwning(vals)
	case json.Number:
		if n, err := t.Int64(); err == nil {
			return n
		}
		f, _ := t.Float64()
		return f
	default:
		return v
	}
}

// formDecode parses an application/x-www-form-urlencoded body into a
// map of keyword → string (first value per key).
func formDecode(v any) any {
	s, _ := v.(string)
	values, err := url.ParseQuery(s)
	if err != nil {
		panic(fmt.Errorf("-form-decode: %w", err))
	}
	kvs := []any{}
	for k, vs := range values {
		if len(vs) > 0 {
			kvs = append(kvs, lang.NewKeyword(k), vs[0])
		}
	}
	return lang.NewMap(kvs...)
}

// --- crypto helpers ----------------------------------------------------------

// keywordName is a keyword's full name without the leading colon
// (":ns/kw" → "ns/kw") — the JSON key/value spelling.
func keywordName(k lang.Keyword) string {
	return strings.TrimPrefix(k.String(), ":")
}

func hmacSign(key, payload string) string {
	mac := hmac.New(sha256.New, []byte(key))
	mac.Write([]byte(payload))
	return hex.EncodeToString(mac.Sum(nil))
}

func randToken() string {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		panic(fmt.Errorf("-rand-token: %w", err))
	}
	return hex.EncodeToString(b)
}
