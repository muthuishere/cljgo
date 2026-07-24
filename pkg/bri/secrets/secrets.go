// Package secrets is the ISOLATED Go half of bri.core.secrets — the pluggable
// secret store (ADR 0086, realizing ADR 0060 / spike S39). It is a SEPARATE
// package from pkg/bri on purpose: the OS-keychain client (zalando/go-keyring
// and its D-Bus / wincred / x/sys transports) is a dependency that must NOT
// link into a bri binary that never touches a secret store. pkg/bri never
// imports this package; only the generated pkg/briaot/brisecrets sub-package
// and the interpreter's briloader (blank import) do, so the linker keeps the
// keychain client exactly when — and only when — an app requires
// bri.core.secrets (ADR 0074/0076 opt-in linking).
//
// go-keyring is PURE GO on every platform (macOS execs /usr/bin/security;
// Linux speaks the D-Bus Secret Service protocol; Windows calls wincred via
// x/sys), so a bri.core.secrets app still AOT-compiles to a CGO_ENABLED=0
// static binary and cross-compiles (S39-proven).
//
// This package holds ONLY the fetch/store host shims (scalar strings in, raw
// value or nil out). All policy — masking, the reveal seam, the fallback
// chain — lives in portable Clojure (core/bri/secrets.cljg), so a secret's
// value is masked at every print surface and only an explicit `reveal`
// unwraps it (ADR 0086).
package secrets

import (
	"errors"
	"fmt"
	"net/url"
	"os"

	"github.com/zalando/go-keyring"

	"github.com/muthuishere/cljgo/pkg/bri"
)

// init wires bri.core.secrets's shim installer into pkg/bri's registry. It runs
// only when this package is linked (the app requires bri.core.secrets), so a
// non-secrets binary never carries the keychain client (ADR 0074).
func init() { bri.RegisterInstaller("bri.core.secrets", installShims) }

// installShims interns bri.core.secrets's private Go primitives. The chain,
// masking and reveal policy are Clojure (secrets.cljg); these three are the
// scalar fetch/store boundary.
func installShims(def func(name string, fn func(args ...any) any)) {
	// -secret-get1 (uri key) -> raw string, or nil on a miss. A real backend
	// failure panics (it becomes a cljgo error, aborting the chain — never a
	// silent fall-through to a weaker source).
	def("-secret-get1", func(args ...any) any {
		uri, key := twoStr("-secret-get1", args)
		v, ok, err := fetch(uri, key)
		if err != nil {
			panic(err)
		}
		if !ok {
			return nil
		}
		return v
	})
	// -secret-set1 (uri key value) -> nil. Only a writable scheme (keychain)
	// accepts it; env:// is read-only and panics.
	def("-secret-set1", func(args ...any) any {
		uri, key, val := threeStr("-secret-set1", args)
		if err := store(uri, key, val); err != nil {
			panic(err)
		}
		return nil
	})
	// -secret-del1 (uri key) -> nil.
	def("-secret-del1", func(args ...any) any {
		uri, key := twoStr("-secret-del1", args)
		if err := remove(uri, key); err != nil {
			panic(err)
		}
		return nil
	})
}

// --- the two schemes (env + keychain; the registry grows age/cloud later) ---

func fetch(uri, key string) (string, bool, error) {
	scheme, host, path, err := parseURI(uri)
	if err != nil {
		return "", false, err
	}
	switch scheme {
	case "env":
		v, ok := os.LookupEnv(firstNonEmpty(host, path, key))
		return v, ok, nil
	case "keychain":
		v, err := keyring.Get(host, firstNonEmpty(key, path))
		if errors.Is(err, keyring.ErrNotFound) {
			return "", false, nil // a miss, not a failure → drives the chain
		}
		if err != nil {
			return "", false, fmt.Errorf("bri.core.secrets: keychain %q failed: %w", host, err)
		}
		return v, true, nil
	default:
		return "", false, unknownScheme(scheme, uri)
	}
}

func store(uri, key, val string) error {
	scheme, host, path, err := parseURI(uri)
	if err != nil {
		return err
	}
	switch scheme {
	case "keychain":
		return keyring.Set(host, firstNonEmpty(key, path), val)
	case "env":
		return fmt.Errorf("bri.core.secrets: env:// is read-only; cannot set %q", uri)
	default:
		return unknownScheme(scheme, uri)
	}
}

func remove(uri, key string) error {
	scheme, host, path, err := parseURI(uri)
	if err != nil {
		return err
	}
	switch scheme {
	case "keychain":
		err := keyring.Delete(host, firstNonEmpty(key, path))
		if errors.Is(err, keyring.ErrNotFound) {
			return nil
		}
		return err
	case "env":
		return fmt.Errorf("bri.core.secrets: env:// is read-only; cannot delete %q", uri)
	default:
		return unknownScheme(scheme, uri)
	}
}

// --- helpers ----------------------------------------------------------------

func parseURI(uri string) (scheme, host, path string, err error) {
	u, e := url.Parse(uri)
	if e != nil {
		return "", "", "", fmt.Errorf("bri.core.secrets: bad URI %q: %w", uri, e)
	}
	if u.Scheme == "" {
		return "", "", "", fmt.Errorf("bri.core.secrets: URI %q has no scheme (want env://KEY or keychain://service/account)", uri)
	}
	return u.Scheme, u.Host, trimSlash(u.Path), nil
}

func unknownScheme(scheme, uri string) error {
	return fmt.Errorf("bri.core.secrets: unknown scheme %q in %q (have env, keychain)", scheme, uri)
}

func firstNonEmpty(vs ...string) string {
	for _, v := range vs {
		if v != "" {
			return v
		}
	}
	return ""
}

func trimSlash(s string) string {
	for len(s) > 0 && s[0] == '/' {
		s = s[1:]
	}
	return s
}

func asStr(v any) string {
	if v == nil {
		return ""
	}
	if s, ok := v.(string); ok {
		return s
	}
	panic(fmt.Errorf("bri.core.secrets: expected a string, got %T", v))
}

func twoStr(name string, args []any) (string, string) {
	if len(args) != 2 {
		panic(fmt.Errorf("%s expects 2 args, got %d", name, len(args)))
	}
	return asStr(args[0]), asStr(args[1])
}

func threeStr(name string, args []any) (string, string, string) {
	if len(args) != 3 {
		panic(fmt.Errorf("%s expects 3 args, got %d", name, len(args)))
	}
	return asStr(args[0]), asStr(args[1]), asStr(args[2])
}
