// Package vault — S39 spike: one pluggable secrets interface behind a URI scheme.
//
// The whole design is one contract (Provider) + a registry that maps a URI
// scheme to a Provider constructor + a Chain combinator for fallback. Nothing
// in the app hardcodes a backend; it holds a URI string from config and calls
// Open(uri).Get(ctx).
package vault

import (
	"context"
	"fmt"
	"net/url"
	"sort"
	"strings"
)

// Secret is a fetched secret value. It is a distinct type (not a bare string)
// so callers can't accidentally %v it into a log: String() masks. Get the raw
// bytes only via Reveal(), which is the one auditable call site.
type Secret struct {
	val string
}

func NewSecret(v string) Secret { return Secret{val: v} }

// Reveal returns the raw value. This is the ONLY place a value leaves the type;
// grep for .Reveal() to audit every consumer.
func (s Secret) Reveal() string { return s.val }

// String is the masked form — safe for logs, %v, error strings, REPL echo.
func (s Secret) String() string { return Mask(s.val) }

// GoString makes %#v masked too, so a struct dump can't leak it.
func (s Secret) GoString() string { return "vault.Secret(" + Mask(s.val) + ")" }

// Provider is THE contract. Every backend — env, keychain, encrypted file,
// AWS, GCP, Vault, GitHub — implements exactly this.
type Provider interface {
	// Get resolves key within this provider. ok=false means "not found here"
	// (a normal miss, drives the fallback chain); err means the backend
	// itself failed (network, decrypt, permission) and should NOT be masked
	// by a fallback.
	Get(ctx context.Context, key string) (secret Secret, ok bool, err error)
	// Name is a stable identifier for diagnostics ("env", "keychain:myapp").
	Name() string
}

// Opener constructs a Provider from a parsed URI. Registered per scheme.
type Opener func(u *url.URL) (Provider, error)

var registry = map[string]Opener{}

// Register wires a scheme (without "://") to an Opener. Called from init() in
// each provider file, so linking a provider file in = enabling its scheme.
func Register(scheme string, o Opener) { registry[scheme] = o }

// Schemes lists registered schemes (sorted) — used in not-found diagnostics.
func Schemes() []string {
	out := make([]string, 0, len(registry))
	for s := range registry {
		out = append(out, s)
	}
	sort.Strings(out)
	return out
}

// Open selects a Provider from a config URI like "keychain://myapp/db-password".
// The app calls this; it never names a concrete provider type.
func Open(uri string) (Provider, error) {
	u, err := url.Parse(uri)
	if err != nil {
		return nil, fmt.Errorf("vault: bad URI %q: %w", uri, err)
	}
	if u.Scheme == "" {
		return nil, fmt.Errorf("vault: URI %q has no scheme (want e.g. env://KEY, keychain://svc/acct)", uri)
	}
	o, found := registry[u.Scheme]
	if !found {
		return nil, fmt.Errorf("vault: unknown scheme %q in %q; registered: %s",
			u.Scheme, uri, strings.Join(Schemes(), ", "))
	}
	return o(u)
}

// Chain tries providers in order, returning the first hit. A per-provider err
// (backend failure) aborts the chain — we do not silently fall through a real
// failure to a weaker source. A plain miss (ok=false) rolls to the next.
type chain struct{ ps []Provider }

func Chain(ps ...Provider) Provider { return chain{ps: ps} }

func (c chain) Name() string {
	names := make([]string, len(c.ps))
	for i, p := range c.ps {
		names[i] = p.Name()
	}
	return "chain[" + strings.Join(names, "→") + "]"
}

func (c chain) Get(ctx context.Context, key string) (Secret, bool, error) {
	for _, p := range c.ps {
		s, ok, err := p.Get(ctx, key)
		if err != nil {
			return Secret{}, false, fmt.Errorf("vault: provider %s failed for key %q: %w", p.Name(), key, err)
		}
		if ok {
			return s, true, nil
		}
	}
	return Secret{}, false, nil
}

// OpenChain parses several config URIs into one fallback chain, e.g.
//
//	OpenChain("keychain://myapp/db", "env://DB_PASSWORD")
func OpenChain(uris ...string) (Provider, error) {
	ps := make([]Provider, 0, len(uris))
	for _, u := range uris {
		p, err := Open(u)
		if err != nil {
			return nil, err
		}
		ps = append(ps, p)
	}
	return Chain(ps...), nil
}

// GetFunc is the s38 resolver seam: the whole vault stack collapsed to a
// Get-shaped hook the layered config resolver can register as one lookup layer.
type GetFunc func(ctx context.Context, key string) (value string, ok bool, err error)

// AsGetFunc adapts any Provider (single or Chain) to the resolver seam. It
// Reveals the value at the boundary — the resolver owns hygiene past this line
// (it must store the value in a Secret-like of its own, not a bare field).
func AsGetFunc(p Provider) GetFunc {
	return func(ctx context.Context, key string) (string, bool, error) {
		s, ok, err := p.Get(ctx, key)
		if err != nil || !ok {
			return "", ok, err
		}
		return s.Reveal(), true, nil
	}
}
