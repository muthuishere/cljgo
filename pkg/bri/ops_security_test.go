package bri_test

import (
	"fmt"
	"testing"
)

// ADR 0126: /metrics and CORS fail closed. The conformance file freezes the
// env-INDEPENDENT postures; these are the ones that turn on environment
// (BRI_DEV, APP_METRICS_TOKEN), which a conformance test must not depend on.

const opsProbe = `
(require '[bri.web.http :as http])
(let [rts [["GET /x" (fn [_] {:status 200 :body "ok"})]]]
  (:status (http/request rts {:method "GET" :path "/metrics" %s}
                         {:middleware []})))`

// metricsStatus boots a driver then applies dev, because newDriver clears
// BRI_DEV itself — setting it before the call is silently undone.
func metricsStatus(t *testing.T, headers string, dev bool) int64 {
	t.Helper()
	d := newDriver(t)
	if dev {
		t.Setenv("BRI_DEV", "1")
	}
	v := eval(t, d, fmt.Sprintf(opsProbe, headers))
	n, ok := v.(int64)
	if !ok {
		t.Fatalf("expected an int status, got %T (%v)", v, v)
	}
	return n
}

// With no token and not in dev, /metrics must not be mounted at all —
// the whole point of ADR 0126. A 200 here means the endpoint went back to
// being open to the internet.
func TestMetricsNotMountedWhenUnconfigured(t *testing.T) {
	t.Setenv("APP_METRICS_TOKEN", "")
	if got := metricsStatus(t, "", false); got != 404 {
		t.Errorf("unconfigured /metrics = %d, want 404 (route must not be mounted)", got)
	}
}

// In dev it stays open: a local scrape is the normal case, and forcing a
// token here just teaches everyone to set :public permanently.
func TestMetricsOpenInDev(t *testing.T) {
	t.Setenv("APP_METRICS_TOKEN", "")
	if got := metricsStatus(t, "", true); got != 200 {
		t.Errorf("dev /metrics = %d, want 200", got)
	}
}

// With a token configured: only the right bearer gets in, and a miss is a
// 404 rather than a 401 so a scan cannot confirm the endpoint exists.
func TestMetricsBearerToken(t *testing.T) {
	for _, tc := range []struct {
		name, headers string
		want          int64
	}{
		{"no header", "", 404},
		{"wrong token", `:headers {"authorization" "Bearer nope"}`, 404},
		{"not a bearer", `:headers {"authorization" "s3cret"}`, 404},
		{"right token", `:headers {"authorization" "Bearer s3cret"}`, 200},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Setenv("APP_METRICS_TOKEN", "s3cret")
			if got := metricsStatus(t, tc.headers, false); got != tc.want {
				t.Errorf("/metrics (%s) = %d, want %d", tc.name, got, tc.want)
			}
		})
	}
}

const corsProbe = `
(require '[bri.web.http :as http])
(let [rts [["GET /x" (fn [_] {:status 200 :body "ok"})]]]
  (str (get (:headers (http/request rts {:method "GET" :path "/x"
                                         :headers {"origin" "https://evil.example"}}
                                    {:middleware [(http/cors {})] :ops false}))
            "access-control-allow-origin")))`

// Unconfigured CORS outside dev emits no header at all; in dev it stays
// permissive. "" is how the probe renders a nil header.
func TestCORSFailsClosedOutsideDev(t *testing.T) {
	t.Setenv("APP_HTTP__CORS_ORIGINS", "")
	d := newDriver(t)
	if got := evalString(t, d, corsProbe); got != "" {
		t.Errorf("unconfigured prod CORS emitted %q, want no header", got)
	}
}

func TestCORSPermissiveInDev(t *testing.T) {
	t.Setenv("APP_HTTP__CORS_ORIGINS", "")
	d := newDriver(t) // clears BRI_DEV, so set dev AFTER it
	t.Setenv("BRI_DEV", "1")
	if got := evalString(t, d, corsProbe); got != "*" {
		t.Errorf("dev CORS = %q, want \"*\"", got)
	}
}
