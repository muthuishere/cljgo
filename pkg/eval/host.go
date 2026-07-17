package eval

import (
	"fmt"
	"reflect"
	"strings"

	"github.com/muthuishere/cljgo/pkg/ast"
	"github.com/muthuishere/cljgo/pkg/corelib"
	"github.com/muthuishere/cljgo/pkg/lang"
)

// evalHost evaluates Go interop nodes (OpHostRef / OpHostCall) in the
// interpreter (ADR 0010, design/05 §1–§2). M3-v0: a reflect-backed seed
// registry (strings/strconv/math/fmt), require-go alias resolution, and
// the shared shaping table ([v err] vectors, `!` throw, nil/number
// normalization). Both consumers — this reflect path and the AOT emitter
// via go/types — MUST apply the identical shaping so behavior is
// dual-mode-identical (design/00 §1.4, the unforgivable divergence).
func (e *Evaluator) evalHost(n *ast.Node, s *Scope) (any, error) {
	switch n.Op {
	case ast.OpHostRef:
		r := n.Sub.(*ast.HostRefNode)
		rv, ok := corelib.LookupHostMember(r.Pkg, r.Member)
		if !ok {
			if isThirdPartyGoPath(r.Pkg) {
				return nil, nil // AOT-only member (ADR 0021 B2): compile-time no-op
			}
			return nil, fmt.Errorf("unable to resolve Go member: %s.%s", r.Pkg, r.Member)
		}
		if rv.Kind() == reflect.Func {
			// Fn-as-value: wrap as a native IFn cljgo can apply. Used in
			// value position it shapes with Throw=false (no `!` sugar on a
			// bare ref), so a multi-return fn yields the [v err]/[v ok]
			// vector exactly as a direct call would.
			name := r.Pkg + "." + r.Member
			frv := rv
			return corelib.NewNativeFn(name, func(args ...any) any {
				return corelib.CallHostFn(name, frv, args, false)
			}), nil
		}
		// Const/var value (e.g. math/Pi): number/nil normalized so the
		// printer renders 3.141592653589793, not float64(...).
		return corelib.NormalizeResult(rv), nil

	case ast.OpHostCall:
		c := n.Sub.(*ast.HostCallNode)
		rv, ok := corelib.LookupHostMember(c.Pkg, c.Member)
		argVals := make([]any, len(c.Args))
		for i, an := range c.Args {
			v, err := e.Eval(an, s)
			if err != nil {
				return nil, err
			}
			argVals[i] = v
		}
		if !ok {
			if isThirdPartyGoPath(c.Pkg) {
				// AOT-only member (ADR 0021 B2): args are evaluated for their
				// side effects, but the unlinked call is a compile-time no-op —
				// the emitted binary makes the real, non-reflective call.
				return nil, nil
			}
			return nil, fmt.Errorf("unable to resolve Go member: %s.%s", c.Pkg, c.Member)
		}
		if rv.Kind() != reflect.Func {
			return nil, fmt.Errorf("Go member is not callable: %s.%s", c.Pkg, c.Member)
		}
		// corelib.CallHostFn panics on coercion failure or a thrown (`!`) error —
		// recovered into an error at the IFn boundary / top level, matching
		// how builtins.go signals runtime failures.
		return corelib.CallHostFn(c.Pkg+"."+c.Member, rv, argVals, c.Throw), nil

	case ast.OpHostMethod:
		m := n.Sub.(*ast.HostMethodNode)
		recv, err := e.Eval(m.Recv, s)
		if err != nil {
			return nil, err
		}
		argVals := make([]any, len(m.Args))
		for i, an := range m.Args {
			v, aerr := e.Eval(an, s)
			if aerr != nil {
				return nil, aerr
			}
			argVals[i] = v
		}
		// The receiver's type is only known at runtime (v0), so BOTH modes
		// reflect through CallGoMethod — the AOT emitter reaches the very
		// same function via rt.CallMethod, guaranteeing byte-identity.
		return corelib.CallGoMethod(recv, m.Method, m.Throw, argVals), nil

	case ast.OpHostField:
		f := n.Sub.(*ast.HostFieldNode)
		recv, err := e.Eval(f.Recv, s)
		if err != nil {
			return nil, err
		}
		// Reflective in both modes (v0); the AOT emitter reaches the same
		// GoFieldGet via rt.FieldGet.
		return corelib.GoFieldGet(recv, f.Field), nil

	case ast.OpHostNew:
		nw := n.Sub.(*ast.HostNewNode)
		if nw.Zero {
			return corelib.NewGoStruct(nw.Pkg, nw.Type), nil
		}
		var fields any
		if nw.Fields != nil {
			v, err := e.Eval(nw.Fields, s)
			if err != nil {
				return nil, err
			}
			fields = v
		}
		// Reflective in both modes (v0); the AOT emitter reaches the same
		// MakeGoStruct via rt.MakeStruct.
		return corelib.MakeGoStruct(nw.Pkg, nw.Type, fields), nil

	default:
		return nil, fmt.Errorf("evalHost: unexpected op %v", n.Op)
	}
}

// resolveHost is the analyzer's Go-interop hook (ResolveHost). It resolves
// a namespaced symbol whose namespace is a `:require-go` alias in the
// current ns to (import-path, member). The precedence principle (CLAUDE.md)
// is non-negotiable: Clojure is first-class, so a namespace that resolves
// as a Clojure namespace OR a Clojure ns-alias in the current ns wins and
// this returns ok=false. Membership is gated on the seed registry so that
// (a) an unknown member falls through to Clojure's resolution error rather
// than a host miss, and (b) the analyzer's `!`-suffix retry works: for
// `sc/Atoi!` the full name misses the registry (ok=false) and the analyzer
// retries the `!`-stripped `sc/Atoi`, which hits.
func (e *Evaluator) resolveHost(sym *lang.Symbol) (pkg, member string, ok bool) {
	if !sym.HasNamespace() || sym.Namespace() == "" {
		return "", "", false
	}
	nsName := sym.Namespace()
	nsSym := lang.NewSymbol(nsName)
	// Precedence: Clojure alias / namespace always wins.
	if e.CurrentNS().LookupAlias(nsSym) != nil {
		return "", "", false
	}
	if lang.FindNamespace(nsSym) != nil {
		return "", "", false
	}
	aliases := e.hostAliases[e.CurrentNS().Name().Name()]
	path, found := aliases[nsName]
	if !found {
		return "", "", false
	}
	if _, inReg := corelib.LookupHostMember(path, sym.Name()); inReg {
		return path, sym.Name(), true
	}
	// Third-party modules (a domain-dotted import path, declared via a
	// build.cljgo `go-require`, ADR 0021 B2) are NOT in the reflect seed
	// registry. Resolve any member as host anyway: the AOT emitter validates
	// and links it from go/packages type facts (zero hand-written bindings),
	// and the interpreter's compile-time pass no-ops the unlinked call
	// (evalHost). A trailing `!` yields to the analyzer's bang-retry.
	if isThirdPartyGoPath(path) && !strings.HasSuffix(sym.Name(), "!") {
		return path, sym.Name(), true
	}
	return "", "", false
}

// isThirdPartyGoPath reports whether an import path is a third-party module
// (its first `/`-segment is a domain, i.e. contains a dot) rather than a Go
// stdlib package (strings, net/url, …). Third-party members resolve through
// go/packages type facts, not the reflect seed registry.
func isThirdPartyGoPath(path string) bool {
	first := path
	if i := strings.IndexByte(path, '/'); i >= 0 {
		first = path[:i]
	}
	return strings.Contains(first, ".")
}

// registerRequireGo backs the `require-go` builtin: it records an
// alias→import-path mapping scoped to the current namespace (ADR 0010,
// design/05 §1). Each libspec is a vector: a path (a symbol — one segment,
// no slash — or a string that may contain slashes), then optional
// `:as alias`. The default alias is the last `/`-segment of the path.
func (e *Evaluator) registerRequireGo(specs []any) {
	for _, spec := range specs {
		path, alias := parseRequireGoSpec(spec)
		nsName := e.CurrentNS().Name().Name()
		if e.hostAliases[nsName] == nil {
			e.hostAliases[nsName] = map[string]string{}
		}
		e.hostAliases[nsName][alias] = path
	}
}

func parseRequireGoSpec(spec any) (path, alias string) {
	// A bare symbol/string libspec (no options).
	switch s := spec.(type) {
	case *lang.Symbol:
		path = hostPathOf(s)
		return path, defaultHostAlias(path)
	case string:
		return s, defaultHostAlias(s)
	}
	vec, ok := spec.(lang.IPersistentVector)
	if !ok || vec.Count() == 0 {
		panic(fmt.Errorf("require-go expects a libspec vector, got: %s", lang.PrintString(spec)))
	}
	switch p := vec.Nth(0).(type) {
	case *lang.Symbol:
		path = hostPathOf(p)
	case string:
		path = p
	default:
		panic(fmt.Errorf("require-go path must be a symbol or string, got: %s", lang.PrintString(vec.Nth(0))))
	}
	alias = defaultHostAlias(path)
	kwAs := lang.NewKeyword("as")
	for i := 1; i < vec.Count(); i++ {
		if lang.Equiv(vec.Nth(i), kwAs) {
			if i+1 >= vec.Count() {
				panic(fmt.Errorf("require-go :as requires an alias, in libspec: %s", lang.PrintString(vec)))
			}
			as, ok := vec.Nth(i + 1).(*lang.Symbol)
			if !ok {
				panic(fmt.Errorf("require-go :as alias must be a symbol, got: %s", lang.PrintString(vec.Nth(i+1))))
			}
			alias = as.Name()
			i++
		}
	}
	return path, alias
}

// hostPathOf reconstructs a path from a symbol libspec. A single-segment
// name (strings, strconv) is the common case; a namespaced symbol is
// reassembled (avoid it — slashed paths belong in a string).
func hostPathOf(s *lang.Symbol) string {
	if s.HasNamespace() && s.Namespace() != "" {
		return s.Namespace() + "/" + s.Name()
	}
	return s.Name()
}

func defaultHostAlias(path string) string {
	if i := strings.LastIndex(path, "/"); i >= 0 {
		return path[i+1:]
	}
	return path
}

// resolveHostType is the analyzer's Go-type-resolution hook
// (ResolveHostType). It mirrors resolveHost's precedence exactly — Clojure
// namespaces/aliases win — but resolves the symbol's name against the type
// registry rather than the member registry (ADR 0010, design/05 §1).
func (e *Evaluator) resolveHostType(sym *lang.Symbol) (pkg, typeName string, ok bool) {
	if !sym.HasNamespace() || sym.Namespace() == "" {
		return "", "", false
	}
	nsName := sym.Namespace()
	nsSym := lang.NewSymbol(nsName)
	if e.CurrentNS().LookupAlias(nsSym) != nil {
		return "", "", false
	}
	if lang.FindNamespace(nsSym) != nil {
		return "", "", false
	}
	aliases := e.hostAliases[e.CurrentNS().Name().Name()]
	path, found := aliases[nsName]
	if !found {
		return "", "", false
	}
	if _, inReg := corelib.LookupHostType(path, sym.Name()); !inReg {
		return "", "", false
	}
	return path, sym.Name(), true
}
