package vault

import (
	"context"
	"errors"
	"net/url"

	"github.com/zalando/go-keyring"
)

// keychainProvider talks to the OS secret store via zalando/go-keyring.
// Scheme: keychain://service/account
//
// CRITICAL for cljgo's single-binary constraint: go-keyring is PURE GO. On
// macOS it shells out to /usr/bin/security (exec, not cgo); on Linux it speaks
// the D-Bus Secret Service wire protocol over a unix socket in pure Go; on
// Windows it calls wincred via golang.org/x/sys. So this provider builds and
// runs with CGO_ENABLED=0 — proven in results/e2.
type keychainProvider struct {
	service string
	account string // optional default; caller key overrides
}

func init() {
	Register("keychain", func(u *url.URL) (Provider, error) {
		return keychainProvider{service: u.Host, account: trimSlash(u.Path)}, nil
	})
}

func (p keychainProvider) Name() string { return "keychain:" + p.service }

func (p keychainProvider) Get(ctx context.Context, key string) (Secret, bool, error) {
	acct := p.account
	if key != "" {
		acct = key // caller's key wins if provided
	}
	v, err := keyring.Get(p.service, acct)
	if errors.Is(err, keyring.ErrNotFound) {
		return Secret{}, false, nil // a miss, not a failure → drives fallback
	}
	if err != nil {
		return Secret{}, false, err // real backend failure
	}
	return NewSecret(v), true, nil
}

// setForTest / delForTest — spike-only helpers to prove a round-trip. A real
// bri.vault would expose Set behind a separate write API, not on Provider.
func (p keychainProvider) setForTest(account, value string) error {
	return keyring.Set(p.service, account, value)
}
func (p keychainProvider) delForTest(account string) error {
	return keyring.Delete(p.service, account)
}
