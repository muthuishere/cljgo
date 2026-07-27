// Package security is the ISOLATED keychain half of cljg.security (ADR 0103,
// spike s65). It is a SEPARATE package from pkg/bri on purpose: the OS-keychain
// client (zalando/go-keyring + its D-Bus / wincred / x/sys transports) and the
// age encrypted-file fallback are dependencies that must NOT link into a
// binary that never touches the keychain. pkg/bri never imports this package;
// only the generated pkg/briaot/cljgsecurity sub-package (blank-imported by
// the emitter when the app requires cljg.security), cmd/genbri, and the
// interpreter's briloader do — so the linker keeps the keyring client exactly
// when, and only when, an app requires cljg.security (ADR 0074 opt-in linking).
//
// The store is the s65 UNIFIED contract — the same save/get/delete API on
// every platform, never failing just because a desktop keyring daemon is
// absent:
//
//	:native — the OS credential store (macOS Keychain / Windows Credential
//	          Manager / Linux Secret Service), probed before use;
//	:file   — a machine-local age-encrypted file store (X25519 identity,
//	          0600, under the user config dir — headless servers, CI);
//	:auto   — native when the probe succeeds, else the file fallback.
//
// go-keyring is PURE GO on every platform and age is pure Go, so a
// cljg.security app still AOT-compiles to a CGO_ENABLED=0 static binary.
// Secret VALUES never appear in any error, log, or print path here.
package security

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"sync"

	"filippo.io/age"
	"github.com/zalando/go-keyring"

	"github.com/muthuishere/cljgo/pkg/bri"
)

// init wires cljg.security's keychain installer into pkg/bri's registry. It
// runs only when this package is linked (the app requires cljg.security), so
// a binary without it never carries the keyring client (ADR 0074). The
// crypto/JWT shims are installed separately by pkg/bri's own
// installSecurityShims — InstallShimsInto runs BOTH.
func init() { bri.RegisterInstaller("cljg.security", installKeychainShims) }

// installKeychainShims interns the -keychain-set/-get/-del trio. Each takes a
// trailing backend argument ("auto" | "native" | "file") from the Clojure
// opts map; set/del return the backend name that served the call, get returns
// the value or nil when absent (both backends).
func installKeychainShims(def func(name string, fn func(args ...any) any)) {
	def("-keychain-set", func(args ...any) any {
		service, account, value, backend := fourStr("-keychain-set", args)
		used, err := storeSet(service, account, value, backend)
		if err != nil {
			panic(fmt.Errorf("cljg.security/save-to-keychain: %s backend failed for service %q: %w", used, service, err))
		}
		return string(used)
	})
	def("-keychain-get", func(args ...any) any {
		service, account, backend := threeStr("-keychain-get", args)
		v, ok, used, err := storeGet(service, account, backend)
		if err != nil {
			panic(fmt.Errorf("cljg.security/get-from-keychain: %s backend failed for service %q: %w", used, service, err))
		}
		if !ok {
			return nil
		}
		return v
	})
	def("-keychain-del", func(args ...any) any {
		service, account, backend := threeStr("-keychain-del", args)
		used, err := storeDel(service, account, backend)
		if err != nil {
			panic(fmt.Errorf("cljg.security/delete-from-keychain: %s backend failed for service %q: %w", used, service, err))
		}
		return string(used)
	})
}

// Backend names the store that actually served an operation.
type Backend string

const (
	BackendNative Backend = "native"
	BackendFile   Backend = "file"
)

// nativeProbe caches the native-keychain reachability probe for the process:
// one Set/Get/Delete round-trip against the OS store. A failed probe (no
// Secret Service daemon on a headless Linux box, locked keychain, …) means
// "use the fallback" — never an error to the caller (s65).
var nativeProbe = sync.OnceValue(func() bool {
	const probe = "cljgo-native-probe"
	if err := keyring.Set(probe, probe, "1"); err != nil {
		return false
	}
	_, err := keyring.Get(probe, probe)
	_ = keyring.Delete(probe, probe)
	return err == nil
})

// pick resolves the caller's backend choice ("auto"/"native"/"file", from the
// Clojure {:backend …} opt) to the concrete store.
func pick(backend string) (Backend, error) {
	switch backend {
	case "", "auto":
		if nativeProbe() {
			return BackendNative, nil
		}
		return BackendFile, nil
	case "native":
		return BackendNative, nil
	case "file":
		return BackendFile, nil
	default:
		return "", fmt.Errorf("cljg.security: unknown keychain backend %q (expected :auto, :native, or :file)", backend)
	}
}

func storeSet(service, account, value, backend string) (Backend, error) {
	b, err := pick(backend)
	if err != nil {
		return b, err
	}
	switch b {
	case BackendNative:
		return b, keyring.Set(service, account, value)
	default:
		return b, fileSet(service, account, value)
	}
}

func storeGet(service, account, backend string) (string, bool, Backend, error) {
	b, err := pick(backend)
	if err != nil {
		return "", false, b, err
	}
	switch b {
	case BackendNative:
		v, err := keyring.Get(service, account)
		if errors.Is(err, keyring.ErrNotFound) {
			return "", false, b, nil // a miss, not a failure — nil to the caller
		}
		return v, err == nil, b, err
	default:
		return fileGetBackend(service, account, b)
	}
}

func storeDel(service, account, backend string) (Backend, error) {
	b, err := pick(backend)
	if err != nil {
		return b, err
	}
	switch b {
	case BackendNative:
		err := keyring.Delete(service, account)
		if errors.Is(err, keyring.ErrNotFound) {
			return b, nil // deleting an absent key is a no-op, like Bun
		}
		return b, err
	default:
		err := os.Remove(blobPath(service, account))
		if err != nil && os.IsNotExist(err) {
			return b, nil
		}
		return b, err
	}
}

// --- encrypted-file fallback (pure Go, age X25519, machine-local key) --------
//
// A per-machine age identity is generated once, stored 0600 under the user
// config dir (~/.config/cljgo/keystore/ on Linux, the platform equivalent
// elsewhere — NOT tmp); secrets are encrypted to it. No passphrase prompt, no
// daemon — works on any headless box (s65).

// storeDir is overridable for tests (security_test.go points it at t.TempDir()
// so the round-trip never touches the real user keystore).
var storeDir = func() (string, error) {
	base, err := os.UserConfigDir()
	if err != nil {
		return "", fmt.Errorf("cljg.security: cannot resolve the user config dir for the file keystore: %w", err)
	}
	return filepath.Join(base, "cljgo", "keystore"), nil
}

func ensureDir() (string, error) {
	dir, err := storeDir()
	if err != nil {
		return "", err
	}
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return "", fmt.Errorf("cljg.security: cannot create the file keystore at %s: %w", dir, err)
	}
	return dir, nil
}

func identity() (*age.X25519Identity, error) {
	dir, err := ensureDir()
	if err != nil {
		return nil, err
	}
	keyPath := filepath.Join(dir, "machine.age-key")
	if b, err := os.ReadFile(keyPath); err == nil {
		return age.ParseX25519Identity(strings.TrimSpace(string(b)))
	}
	id, err := age.GenerateX25519Identity()
	if err != nil {
		return nil, err
	}
	if err := os.WriteFile(keyPath, []byte(id.String()), 0o600); err != nil {
		return nil, err
	}
	return id, nil
}

func blobPath(service, account string) string {
	dir, err := storeDir()
	if err != nil {
		// ensureDir/identity surface this first on every write path; reads
		// against an unresolvable dir report not-found via os.Open below.
		dir = "."
	}
	safe := strings.NewReplacer("/", "_", " ", "_", ":", "_").Replace(service + "__" + account)
	return filepath.Join(dir, safe+".age")
}

func fileSet(service, account, secret string) error {
	id, err := identity()
	if err != nil {
		return err
	}
	var buf bytes.Buffer
	w, err := age.Encrypt(&buf, id.Recipient())
	if err != nil {
		return err
	}
	if _, err := io.WriteString(w, secret); err != nil {
		return err
	}
	if err := w.Close(); err != nil {
		return err
	}
	return os.WriteFile(blobPath(service, account), buf.Bytes(), 0o600)
}

func fileGetBackend(service, account string, b Backend) (string, bool, Backend, error) {
	ct, err := os.ReadFile(blobPath(service, account))
	if err != nil {
		if os.IsNotExist(err) {
			return "", false, b, nil // a miss, not a failure — nil to the caller
		}
		return "", false, b, err
	}
	id, err := identity()
	if err != nil {
		return "", false, b, err
	}
	r, err := age.Decrypt(bytes.NewReader(ct), id)
	if err != nil {
		return "", false, b, err
	}
	pt, err := io.ReadAll(r)
	if err != nil {
		return "", false, b, err
	}
	return string(pt), true, b, nil
}

// --- arg helpers -------------------------------------------------------------

func asStr(name string, v any) string {
	if v == nil {
		return ""
	}
	if s, ok := v.(string); ok {
		return s
	}
	panic(fmt.Errorf("%s: expected a string, got %T", name, v))
}

func threeStr(name string, args []any) (string, string, string) {
	if len(args) != 3 {
		panic(fmt.Errorf("wrong number of args (%d) passed to: %s (expects 3: [service account backend])", len(args), name))
	}
	return asStr(name, args[0]), asStr(name, args[1]), asStr(name, args[2])
}

func fourStr(name string, args []any) (string, string, string, string) {
	if len(args) != 4 {
		panic(fmt.Errorf("wrong number of args (%d) passed to: %s (expects 4: [service account value backend])", len(args), name))
	}
	return asStr(name, args[0]), asStr(name, args[1]), asStr(name, args[2]), asStr(name, args[3])
}
