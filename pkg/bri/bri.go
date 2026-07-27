// Package bri is the Go half of the bri application framework (ADR
// 0041, ADR 0069, ADR 0071, openspec app-framework T1): the net/http
// adapter and the small host primitives (JSON, HMAC, argon2/JWT, env,
// files) the bri.* namespaces lean on. The Clojure half lives in
// core/bri/*.cljg, embedded via the core package.
//
// This package is PURE Go — it must NOT import pkg/eval, so it links
// cleanly into an AOT-compiled binary (ADR 0071 decision 2). It exposes:
//
//   - the shim installers (installHTTPShims/…), gathered per namespace
//     by Specs();
//   - InstallShimsInto, which interns a namespace's Go shims as :private
//     vars (the interning the interpreter did in-line before ADR 0071
//     factored the loaders out);
//   - Specs(), the ordered descriptor list both loaders drive.
//
// Two loaders drive these specs:
//   - pkg/briloader (interpreter): reads + evaluates the embedded source
//     through an evaluator, registered lazily via the lib-provider
//     registry — the `cljgo dev` / REPL path (ADR 0041). Nothing loads at
//     boot; nothing is scanned (the app requires bri, bri never the app).
//   - pkg/briaot (compiled): links AOT-compiled Go packages generated
//     from the same sources (cmd/genbri), registering a provider per
//     namespace that installs the shims then runs the compiled Load() —
//     the `cljgo build` path (ADR 0071).
//
// The single Go shim implementation both modes share is what makes the
// dual-harness parity structural rather than hoped-for (ADR 0071 dec 6).
package bri

import (
	"github.com/muthuishere/cljgo/core"
	"github.com/muthuishere/cljgo/pkg/lang"
)

// Spec is one bri namespace: its embedded source, the source file it
// binds *file* to while loading, the Go package genbri emits it into
// (pkg/briaot/<Pkg>), and the Go shims interned (as :private vars)
// before the source evaluates. install is nil for a pure-Clojure
// namespace (bri.web.html) or for an OptIn namespace whose shims live in an
// isolated package that registers itself via RegisterInstaller (bri.core.telemetry).
// A namespace may carry BOTH (cljg.security, ADR 0103): cheap stdlib shims
// here plus a heavy opt-in trio via the registry — InstallShimsInto runs both.
type Spec struct {
	Name    string  // "bri.web.http"
	File    string  // "bri/http.cljg" (bound to *file* while loading)
	Pkg     string  // "brihttp" (the pkg/briaot subpackage genbri emits)
	Source  *string // &core.BriHTTPSource
	install func(def func(name string, fn func(args ...any) any))

	// OptIn marks a namespace whose Go shims pull a HEAVY dependency
	// (bri.core.telemetry → the OpenTelemetry SDK) that must NOT link into a bri
	// binary that does not require it (ADR 0074). An OptIn namespace is
	// EXCLUDED from the always-linked umbrella pkg/briaot: its Go shims
	// live in an isolated package (ShimImport) that registers its
	// installer via RegisterInstaller, its compiled sub-package
	// self-registers its lib provider, and the emitter blank-imports that
	// sub-package ONLY when the app requires the namespace. install is nil
	// (the installer arrives through the registry when ShimImport is
	// linked, so pkg/bri never references the heavy dependency).
	OptIn bool

	// ShimImport is the Go import path of the isolated package holding an
	// OptIn namespace's Go shims (e.g. github.com/muthuishere/cljgo/pkg/bri/otel).
	// genbri blank-imports it into the generated sub-package so its init()
	// registers the installer (RegisterInstaller) before Load runs. Empty
	// for the always-linked namespaces (their installers are in pkg/bri).
	ShimImport string
}

// installers holds the shim installer for an OptIn namespace, registered
// by its isolated shim package's init() (RegisterInstaller). It stays empty
// — and the heavy dependency stays UNLINKED — in any binary that does not
// import that package (ADR 0074: opt-in means zero cost when unused).
var installers = map[string]func(def func(name string, fn func(args ...any) any)){}

// RegisterInstaller records an OptIn namespace's Go-shim installer. The
// isolated shim package (e.g. pkg/bri/otel) calls this from its init(), so
// the installer is present exactly when that package is linked — and absent
// (the heavy dependency dropped by the linker) otherwise.
func RegisterInstaller(name string, install func(def func(name string, fn func(args ...any) any))) {
	installers[name] = install
}

// Specs returns the bri namespaces in DEPENDENCY-SAFE load order:
// bri.web.http first (nothing bri depends on it at load time), then the
// namespaces that require it (bri.web.html) or bri.core.audit (cljg.security). genbri
// compiles them in this order — each namespace's vars must exist before a
// later one's top-level require resolves — and pkg/briaot's providers are
// registered from it too.
func Specs() []Spec {
	return []Spec{
		// cljg.http — the raw HTTP server primitive (ADR 0103 wave 1), carved
		// out of bri.web.http. Registered FIRST (not last like other new
		// namespaces): genbri compiles specs strictly in this order with a
		// panicking LibLoader, so bri.web.http's top-level
		// (require '[cljg.http]) can only resolve against an EARLIER row. The
		// resulting deterministic gensym renumbering across the later
		// generated briaot files is accepted (one-time cascade).
		{Name: "cljg.http", File: "cljg/http.cljg", Pkg: "cljghttp", Source: &core.CljgHTTPSource, install: installCljgHTTPShims},
		{Name: "bri.web.http", File: "bri/http.cljg", Pkg: "brihttp", Source: &core.BriHTTPSource, install: installHTTPShims},
		{Name: "bri.core.config", File: "bri/config.cljg", Pkg: "briconfig", Source: &core.BriConfigSource, install: installConfigShims},
		{Name: "bri.core.audit", File: "bri/audit.cljg", Pkg: "briaudit", Source: &core.BriAuditSource, install: installAuditShims},
		{Name: "bri.web.html", File: "bri/html.cljg", Pkg: "brihtml", Source: &core.BriHTMLSource, install: nil},
		// cljg.security (ADR 0103, renamed IN PLACE from bri.core.security — it
		// still requires bri.web.http + bri.core.audit above). Its crypto/JWT
		// shims stay in pkg/bri (install), but its keychain trio pulls the
		// OS-keyring client + age, so it is ALSO OptIn: the isolated
		// pkg/bri/security (ShimImport) registers the keychain installer via
		// RegisterInstaller when linked, and InstallShimsInto runs BOTH.
		{Name: "cljg.security", File: "cljg/security.cljg", Pkg: "cljgsecurity", Source: &core.CljgSecuritySource, install: installSecurityShims, OptIn: true, ShimImport: "github.com/muthuishere/cljgo/pkg/bri/security"},
		// cljg.data.cast is OPT-IN (ADR 0076): its shims pull the SQLite + pgx drivers
		// (~7 MB), which must not link into an app that never touches a
		// database. Like bri.core.telemetry it is excluded from the umbrella pkg/briaot;
		// its shims live in the isolated pkg/bri/db (ShimImport), which registers
		// its installer via RegisterInstaller when linked. Re-homed to
		// cljg.data.cast by ADR 0102 (framework-agnostic mechanism).
		{Name: "cljg.data.cast", File: "cljg/data_cast.cljg", Pkg: "cljgdatacast", Source: &core.CljgDataCastSource, install: nil, OptIn: true, ShimImport: "github.com/muthuishere/cljgo/pkg/bri/db"},
		// bri.core.telemetry is OPT-IN (ADR 0074): its shims pull the OpenTelemetry SDK,
		// which must not link into a bri app that does not require tracing. It
		// is excluded from the umbrella pkg/briaot; its shims live in the
		// isolated pkg/bri/otel (ShimImport), which registers its installer via
		// RegisterInstaller when linked.
		{Name: "bri.core.telemetry", File: "bri/otel.cljg", Pkg: "briotel", Source: &core.BriOtelSource, install: nil, OptIn: true, ShimImport: "github.com/muthuishere/cljgo/pkg/bri/otel"},
		// bri.cli.validate stays PURE CLOJURE (composable validators, no host
		// deps). bri.cli carries a THIN terminal primitive (installCLIShims,
		// cli.go): -tty?/-read-input/-read-secret for the interactive half of
		// the unified parameter model (increment 2, ADR 0078 / s47 VERDICT).
		// It stays NON-OptIn — after the Charm rejection (s47) the interactive
		// layer is just golang.org/x/term, pure-Go and tiny, so there is no
		// heavy dep to isolate; the prompt POLICY is portable Clojure.
		{Name: "bri.cli.validate", File: "bri/cli_validate.cljg", Pkg: "briclivalidate", Source: &core.BriCLIValidateSource, install: nil},
		{Name: "bri.cli", File: "bri/cli.cljg", Pkg: "bricli", Source: &core.BriCLISource, install: installCLIShims},
		// cljg.secrets is OPT-IN (ADR 0086/0074): its shims pull the OS-keychain
		// client (go-keyring + D-Bus/wincred transports), which must not link
		// into an app that never touches a secret store. Its shims live in
		// the isolated pkg/bri/secrets (ShimImport), registering their installer
		// via RegisterInstaller when linked. Re-homed to cljg.secrets by
		// ADR 0102 (framework-agnostic mechanism).
		{Name: "cljg.secrets", File: "cljg/secrets.cljg", Pkg: "cljgsecrets", Source: &core.CljgSecretsSource, install: nil, OptIn: true, ShimImport: "github.com/muthuishere/cljgo/pkg/bri/secrets"},
		// bri.cli.auth is PURE CLOJURE (no Go shims) but transitively requires
		// cljg.secrets — so an app that requires it links the (opt-in)
		// keychain client, while a CLI that never uses auth does not (the
		// transitive opt-in fire in emit/module.go).
		{Name: "bri.cli.auth", File: "bri/cli_auth.cljg", Pkg: "bricliauth", Source: &core.BriCLIAuthSource, install: nil},
		// cljg.net.http is the first cljg.* stdlib namespace (ADR 0087) — the
		// outbound HTTP client. It rides this same name-generic registry (the
		// pkg/bri package name is a legacy of bri being the first tenant). Its
		// Go half (net_http.go) is one request shim over pure-Go net/http, so
		// it is a normal non-OptIn namespace (net/http is stdlib, no dep).
		{Name: "cljg.net.http", File: "cljg/net_http.cljg", Pkg: "cljgnethttp", Source: &core.CljgNetHTTPSource, install: installNetHTTPShims},
		// cljg.os — OS primitives (ADR 0088), this increment the cron scheduler.
		// Pure-Go over stdlib time (os_cron.go), non-OptIn.
		{Name: "cljg.os", File: "cljg/os.cljg", Pkg: "cljgos", Source: &core.CljgOSSource, install: installOSShims},
		// cljg.io — filesystem primitives (ADR 0089), this increment the
		// structural file/path/directory ops clojure.core lacks. Pure-Go over
		// stdlib os + path/filepath (io_fs.go), non-OptIn.
		{Name: "cljg.io", File: "cljg/io.cljg", Pkg: "cljgio", Source: &core.CljgIOSource, install: installIOShims},
		// bri.web.openapi — an OpenAPI-driven typed HTTP client (ADR 0090). Pure
		// Clojure over cljg.net.http (no Go shims, like bri.web.html), so it is
		// registered AFTER cljg.net.http — its top-level require must resolve
		// against already-loaded vars. Non-OptIn (net/http is stdlib).
		{Name: "bri.web.openapi", File: "bri/openapi.cljg", Pkg: "briopenapi", Source: &core.BriOpenAPISource, install: nil},
		// bri.cli.api — an OpenAPI client with automatic login (ADR 0091,
		// realizing ADR 0080). Pure composition of bri.web.openapi + bri.cli.auth
		// + bri.cli (no Go shims), so it is registered AFTER all three — its
		// top-level requires must resolve. Its own namespace (not bri.cli) so the
		// transitive keychain link is opt-in.
		{Name: "bri.cli.api", File: "bri/cli_api.cljg", Pkg: "bricliapi", Source: &core.BriCLIAPISource, install: nil},
		// cljg.cache — the fundamental in-process cache (ADR 0093, re-homed to
		// cljg.* by ADR 0102): a TTL map with singleflight behind the `Cache`
		// protocol. Pure Clojure (install: nil, like bri.web.html) over atoms +
		// promise + cljg.os/now, so it is registered AFTER cljg.os (its
		// (require [cljg.os]) for the clock must resolve). No Go shim, no
		// dependency — links free, inert until called. Placed LAST (nothing
		// requires it) so it does not shift the gensym numbering of the
		// namespaces genbri emits before it.
		{Name: "cljg.cache", File: "cljg/cache.cljg", Pkg: "cljgcache", Source: &core.CljgCacheSource, install: nil},
		// cljg.jobs — the fundamental in-process job queue (ADR 0094, re-homed to
		// cljg.* by ADR 0102): a core.async worker pool behind the `Queue`
		// protocol. Pure Clojure (install: nil) over clojure.core.async (a core
		// namespace, always resolvable) + an atom. No Go shim, no dependency. Also
		// placed at the end (nothing requires it) to keep the gensym numbering of
		// earlier emitted namespaces stable.
		{Name: "cljg.jobs", File: "cljg/jobs.cljg", Pkg: "cljgjobs", Source: &core.CljgJobsSource, install: nil},
		// --- org.clojure contrib, natively (ADR 0097) — all LAST, so they do not
		// shift the gensym numbering of the namespaces genbri emits before them;
		// all LAZY + opt-in, NOT boot sources: a binary that never requires one
		// pays zero bytes (ADR 0097 mandate B). A clojure.*-named namespace rides
		// this same name-generic registry (cljg.io already proves a non-bri name
		// does). ---
		// clojure.tools.cli — GNU-style option parser, pure Clojure over
		// clojure.string; arg parsing is one-shot, no Go host needed.
		{Name: "clojure.tools.cli", File: "tools_cli.cljg", Pkg: "toolscli", Source: &core.ClojureToolsCLISource, install: nil},
		// clojure.data.csv — reader/writer, pure Clojure over clojure.string/escape.
		{Name: "clojure.data.csv", File: "data_csv.cljg", Pkg: "cljgdatacsv", Source: &core.CljDataCSVSource, install: nil},
		// clojure.data.json — Go host JSON codec (-json-read/-json-write in the
		// isolated pkg/bri/cljson) under a thin Clojure option layer; OptIn so the
		// codec links only when required (mandate A: fast path is Go).
		{Name: "clojure.data.json", File: "clojure/data_json.cljg", Pkg: "cljjson", Source: &core.DataJSONSource, install: nil, OptIn: true, ShimImport: "github.com/muthuishere/cljgo/pkg/bri/cljson"},
		// clojure.core.match — match/matchv/matchm/match-let over a real Maranget
		// decision-tree compiler (the Go host primitive -match-compile, match.go);
		// macroexpand-time only, so a compiled binary links zero match runtime.
		{Name: "clojure.core.match", File: "match.cljg", Pkg: "corematch", Source: &core.CoreMatchSource, install: installMatchShims},
		// --- cljg.* native stdlib gaps (ADR 0101) — lazy + opt-in, Go-host over
		// stdlib os/time; non-OptIn (stdlib pulls no heavy dep). ---
		// cljg.system — env (getenv/environ), exit, args over stdlib os.
		{Name: "cljg.system", File: "cljg/system.cljg", Pkg: "cljgsystem", Source: &core.CljgSystemSource, install: installSystemShims},
		// cljg.date — public monotonic nano-time + wall-clock now/since over stdlib time.
		{Name: "cljg.date", File: "cljg/date.cljg", Pkg: "cljgdate", Source: &core.CljgDateSource, install: installDateShims},
		// cljg.stream — the reducible readable/writable stream handle (ADR 0101,
		// spike s56): a Seqable over a Go io.Reader (reduce/into/transduce) + a
		// writable over io.Writer. Reused by cljg.process pipes and cljg.net.http.
		{Name: "cljg.stream", File: "cljg/stream.cljg", Pkg: "cljgstream", Source: &core.CljgStreamSource, install: installStreamShims},
		// cljg.process — streaming subprocess: spawn -> {:in :out :err :wait :kill}
		// over exec.Cmd pipes wrapped as cljg.stream handles.
		{Name: "cljg.process", File: "cljg/process.cljg", Pkg: "cljgprocess", Source: &core.CljgProcessSource, install: installProcSpawnShims},
	}
}

// InstallShimsInto interns s's Go shims as :private vars in its
// namespace, exactly as the interpreter did before ADR 0071 factored the
// loaders out. Both the interpreter loader (pkg/briloader) and the
// compiled loader (pkg/briaot) call this before evaluating/running the
// namespace's forms, so the private -serve/-jwt-sign/… vars are bound in
// either mode. A namespace with no shims (bri.web.html) is a no-op.
func InstallShimsInto(s Spec) {
	// A namespace may have a direct installer (s.install, shims in pkg/bri),
	// a registry installer set by its isolated shim package's init() when that
	// package is linked (ADR 0074), or — cljg.security (ADR 0103) — BOTH: its
	// crypto/JWT shims are stdlib-cheap and stay in pkg/bri while its keychain
	// trio rides the opt-in registry. Run every installer present; a
	// pure-Clojure namespace (bri.web.html) has neither — a no-op.
	direct, registered := s.install, installers[s.Name]
	if direct == nil && registered == nil {
		return
	}
	ns := lang.FindOrCreateNamespace(lang.NewSymbol(s.Name))
	def := func(name string, fn func(args ...any) any) {
		v := ns.Intern(lang.NewSymbol(name))
		v.BindRoot(lang.NewFnFunc(fn))
		v.SetMeta(v.Meta().Assoc(lang.KWPrivate, true).(lang.IPersistentMap))
	}
	if direct != nil {
		direct(def)
	}
	if registered != nil {
		registered(def)
	}
}
