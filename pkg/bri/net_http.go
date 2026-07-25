// net_http.go — the Go half of cljg.net.http, the outbound HTTP client (ADR
// 0087). One request shim over the stdlib net/http (pure Go, so CGO_ENABLED=0
// + cljgo dist hold) plus JSON encode/decode reused from bri's JSON shaping;
// all method sugar, body encoding, query strings, and retry are portable
// Clojure (core/cljg/net_http.cljg). Interned as :private vars into
// cljg.net.http (a non-OptIn namespace: net/http is stdlib, no dep to isolate).
//
// cljg.net.http is the first cljg.* stdlib namespace; it rides the same
// name-generic embedded-namespace registry as bri (the pkg/bri package name is
// a legacy of bri being the first tenant — ADR 0087 §1).
package bri

import (
	"fmt"
	"io"
	"net/http"
	neturl "net/url"
	"strings"
	"time"

	"github.com/muthuishere/cljgo/pkg/lang"
)

// installNetHTTPShims interns cljg.net.http's private Go primitives.
func installNetHTTPShims(def func(name string, fn func(args ...any) any)) {
	def("-http-do", httpDoShim)
	def("-json-encode", func(args ...any) any { return jsonEncode(one("-json-encode", args)) })
	def("-json-decode", func(args ...any) any { return jsonDecode(one("-json-decode", args)) })
	def("-url-encode", func(args ...any) any { return neturl.QueryEscape(asString(one("-url-encode", args))) })
}

// httpDoShim performs ONE request: (method url headers body timeout-ms) ->
// {:status int :headers {name value} :body string}. A transport failure
// (connection, timeout, DNS) panics → a cljgo error the Clojure retry loop
// sees; an HTTP status (incl. 5xx) is a normal response, not a panic.
func httpDoShim(args ...any) any {
	if len(args) != 5 {
		panic(fmt.Errorf("-http-do expects 5 args (method url headers body timeout-ms), got %d", len(args)))
	}
	method := strings.ToUpper(asString(args[0]))
	url := asString(args[1])

	var body io.Reader
	if s, ok := args[3].(string); ok {
		body = strings.NewReader(s)
	}
	req, err := http.NewRequest(method, url, body)
	if err != nil {
		panic(fmt.Errorf("cljg.net.http: bad request %s %q: %w", method, url, err))
	}
	if hm, ok := args[2].(lang.IPersistentMap); ok {
		for s := lang.Seq(hm); s != nil; s = lang.Next(s) {
			e := lang.First(s)
			req.Header.Set(asString(lang.First(e)), asString(lang.Get(e, int64(1))))
		}
	}

	client := &http.Client{Timeout: time.Duration(asInt(args[4])) * time.Millisecond}
	resp, err := client.Do(req)
	if err != nil {
		panic(fmt.Errorf("cljg.net.http: %s %q: %w", method, url, err))
	}
	defer resp.Body.Close()
	b, err := io.ReadAll(resp.Body)
	if err != nil {
		panic(fmt.Errorf("cljg.net.http: reading %s %q: %w", method, url, err))
	}

	hkvs := make([]any, 0, len(resp.Header)*2)
	for k := range resp.Header {
		hkvs = append(hkvs, k, resp.Header.Get(k)) // first value per header
	}
	return lang.NewMap(
		lang.NewKeyword("status"), int64(resp.StatusCode),
		lang.NewKeyword("headers"), lang.NewMap(hkvs...),
		lang.NewKeyword("body"), string(b),
	)
}
