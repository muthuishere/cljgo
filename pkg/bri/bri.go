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
// namespace (bri.html).
type Spec struct {
	Name    string  // "bri.http"
	File    string  // "bri/http.cljg" (bound to *file* while loading)
	Pkg     string  // "brihttp" (the pkg/briaot subpackage genbri emits)
	Source  *string // &core.BriHTTPSource
	install func(def func(name string, fn func(args ...any) any))
}

// Specs returns the bri namespaces in DEPENDENCY-SAFE load order:
// bri.http first (nothing bri depends on it at load time), then the
// namespaces that require it (bri.html) or bri.audit (bri.auth). genbri
// compiles them in this order — each namespace's vars must exist before a
// later one's top-level require resolves — and pkg/briaot's providers are
// registered from it too.
func Specs() []Spec {
	return []Spec{
		{Name: "bri.http", File: "bri/http.cljg", Pkg: "brihttp", Source: &core.BriHTTPSource, install: installHTTPShims},
		{Name: "bri.config", File: "bri/config.cljg", Pkg: "briconfig", Source: &core.BriConfigSource, install: installConfigShims},
		{Name: "bri.audit", File: "bri/audit.cljg", Pkg: "briaudit", Source: &core.BriAuditSource, install: installAuditShims},
		{Name: "bri.html", File: "bri/html.cljg", Pkg: "brihtml", Source: &core.BriHTMLSource, install: nil},
		{Name: "bri.auth", File: "bri/auth.cljg", Pkg: "briauth", Source: &core.BriAuthSource, install: installAuthShims},
	}
}

// InstallShimsInto interns s's Go shims as :private vars in its
// namespace, exactly as the interpreter did before ADR 0071 factored the
// loaders out. Both the interpreter loader (pkg/briloader) and the
// compiled loader (pkg/briaot) call this before evaluating/running the
// namespace's forms, so the private -serve/-jwt-sign/… vars are bound in
// either mode. A namespace with no shims (bri.html) is a no-op.
func InstallShimsInto(s Spec) {
	if s.install == nil {
		return
	}
	ns := lang.FindOrCreateNamespace(lang.NewSymbol(s.Name))
	s.install(func(name string, fn func(args ...any) any) {
		v := ns.Intern(lang.NewSymbol(name))
		v.BindRoot(lang.NewFnFunc(fn))
		v.SetMeta(v.Meta().Assoc(lang.KWPrivate, true).(lang.IPersistentMap))
	})
}
