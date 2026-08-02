package bri_test

import (
	"strings"
	"testing"

	"github.com/muthuishere/cljgo/pkg/diag"
)

// ADR 0121. ADR 0120 fixed the INSTANCE (`:timeout` vs `:timeout-ms` in
// cljg.net.http); these tests are the CLASS. Every one of them passes
// vacuously on the old code — the misspelled call simply succeeds — which is
// the whole defect: an ignored option has no symptom, so the assertion has to
// be "the call FAILS", never "the option is honoured".

// TestUnknownOptIsRefused sweeps the cljg.* opts maps that now declare a
// closed key set. One misspelling per namespace, because the mechanism is
// shared: if it works for one it works for all, and what this really pins is
// that the check is WIRED UP in each of them — including the pure-Clojure
// namespaces (cljg.cache, cljg.jobs) that have no Go half to hang it on.
func TestUnknownOptIsRefused(t *testing.T) {
	for _, c := range []struct{ name, prelude, form, wantFn string }{
		{"net.http", `(require '[cljg.net.http :as ochttp])`,
			`(ochttp/get "http://127.0.0.1:1/x" {:time-out 5})`, "cljg.net.http/request"},
		{"net.http/request", `(require '[cljg.net.http :as ochttp])`,
			`(ochttp/request {:url "http://127.0.0.1:1/x" :timeoutms 5})`, "cljg.net.http/request"},
		{"io/exec", `(require '[cljg.io :as ocio])`,
			`(ocio/exec ["echo" "hi"] {:timeout 50})`, "cljg.io/exec"},
		{"io/write-bytes", `(require '[cljg.io :as ocio])`,
			`(ocio/write-bytes "/dev/null" "x" {:appendd true})`, "cljg.io/write-bytes"},
		{"process/spawn", `(require '[cljg.process :as ocproc])`,
			`(ocproc/spawn ["echo" "hi"] {:timeout-ms 50})`, "cljg.process/spawn"},
		{"socket/dial", `(require '[cljg.socket :as ocsock])`,
			`(ocsock/dial {:port 1 :timeout 5})`, "cljg.socket/dial"},
		{"socket/listen", `(require '[cljg.socket :as ocsock])`,
			`(ocsock/listen {:port 0 :hosts "127.0.0.1"})`, "cljg.socket/listen"},
		{"http/serve", `(require '[cljg.http :as ocsrv])`,
			`(ocsrv/serve {:port 0 :handler (fn [_] {:status 200}) :tsl {}})`, "cljg.http/serve"},
		{"compress/gzip", `(require '[cljg.compress :as occomp])`,
			`(occomp/gzip "hello" {:levl 9})`, "cljg.compress/gzip"},
		{"cache/local", `(require '[cljg.cache :as occache])`,
			`(occache/local {:ttls 10})`, "cljg.cache/local"},
		{"jobs/local", `(require '[cljg.jobs :as ocjobs])`,
			`(ocjobs/local {} {:worker 2})`, "cljg.jobs/local"},
		{"os/run", `(require '[cljg.os :as ocos])`,
			`(ocos/run [] {:max-tick 1})`, "cljg.os/run"},
		{"security/sign", `(require '[cljg.security :as ocsec])`,
			`(ocsec/sign {:sub "u"} {:exp-second 60})`, "cljg.security/sign"},
		{"openapi/client", `(require '[bri.web.openapi :as ocoa])`,
			`(ocoa/client {:servers [{:url "http://x"}] :paths {}} {:timeoutms 5})`,
			"bri.web.openapi/client"},
	} {
		t.Run(c.name, func(t *testing.T) {
			d := newDriver(t)
			eval(t, d, c.prelude)
			// evalErr fails the test when the form returns normally — which
			// is exactly what the pre-ADR-0121 code does.
			got := evalErr(t, d, c.form)
			if !strings.Contains(got, c.wantFn+": unknown option") {
				t.Fatalf("error = %q, want it to name %s and the unknown option", got, c.wantFn)
			}
			if !strings.Contains(got, "(known: ") {
				t.Fatalf("error = %q, want the known option set listed", got)
			}
		})
	}
}

// TestUnknownOptCarriesCodeAndDidYouMean is the diagnostics half: the error
// must arrive as a registered code with a did-you-mean Fix, in the form
// `cljgo explain` and the --json envelope can consume — not as prose.
func TestUnknownOptCarriesCodeAndDidYouMean(t *testing.T) {
	d := newDriver(t)
	eval(t, d, `(require '[cljg.net.http :as ochttp])`)
	_, err := d.EvalString(`(ochttp/get "http://127.0.0.1:1/x" {:timeoutms 5})`, "opts_check_test")
	if err == nil {
		t.Fatal("a misspelled option returned normally — it was silently defaulted")
	}
	dg := diag.FromError(err)
	if dg.ErrorCode != "G5027" {
		t.Errorf("code = %q, want G5027", dg.ErrorCode)
	}
	if len(dg.Fixes) != 1 || dg.Fixes[0].Replacement != ":timeout-ms" {
		t.Errorf("fixes = %+v, want one did-you-mean :timeout-ms", dg.Fixes)
	}
	if rendered := diag.Render(dg); !strings.Contains(rendered, "did you mean :timeout-ms?") ||
		!strings.Contains(rendered, "cljgo explain G5027") {
		t.Errorf("rendered:\n%s\nwant a did-you-mean help line and the explain pointer", rendered)
	}
}

// TestKnownOptsStillPass guards the other direction — the check must not
// reject the documented options, including `:timeout`, the alias ADR 0120
// promised to keep accepting indefinitely. A closed key set that closed on
// the wrong keys would be a worse defect than the one it replaced.
func TestKnownOptsStillPass(t *testing.T) {
	d := newDriver(t)
	eval(t, d, `(require '[cljg.compress :as occomp] '[cljg.cache :as occache] '[cljg.io :as ocio])`)
	for _, form := range []string{
		`(occomp/gunzip (occomp/gzip "hello" {:level 9}) {:as :string})`,
		`(occache/local {:ttl 5})`,
		`(occache/local)`,
		`(ocio/exec ["echo" "hi"] {:timeout-ms 5000 :dir "."})`,
		// nil opts is not a map and must not be diagnosed as one
		`(occomp/gzip "hello" nil)`,
	} {
		eval(t, d, form)
	}
	// Both timeout spellings survive on cljg.net.http (ADR 0120).
	eval(t, d, `(require '[cljg.net.http :as ochttp])`)
	for _, form := range []string{
		`(try (ochttp/get "http://127.0.0.1:1/x" {:timeout 5})    (catch Throwable e (ex-message e)))`,
		`(try (ochttp/get "http://127.0.0.1:1/x" {:timeout-ms 5}) (catch Throwable e (ex-message e)))`,
	} {
		got, ok := eval(t, d, form).(string)
		if ok && strings.Contains(got, "unknown option") {
			t.Errorf("%s: %s — an accepted spelling was refused", form, got)
		}
	}
}

// TestOpenOptsMapsStayOpen records the deliberate exemption (ADR 0121): a
// map that MERGES into the value it returns is a record the caller extends,
// not an argument list, so closing it would break a documented idiom. If a
// later change closes these, this test says so out loud.
func TestOpenOptsMapsStayOpen(t *testing.T) {
	d := newDriver(t)
	eval(t, d, `(require '[cljg.os :as ocos])`)
	got := eval(t, d, `(:tenant (ocos/job "* * * * *" (fn []) {:name "j" :tenant "acme"}))`)
	if got != "acme" {
		t.Fatalf("cljg.os/job dropped a caller key: %v — the job map is open by design", got)
	}
}
