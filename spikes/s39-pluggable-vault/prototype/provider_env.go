package vault

import (
	"context"
	"net/url"
	"os"
)

// envProvider reads from process environment. Scheme: env://KEY
// The bootstrap floor — always available, no external dependency, the natural
// tail of most fallback chains (CI/containers inject secrets as env vars).
type envProvider struct{ key string }

func init() {
	Register("env", func(u *url.URL) (Provider, error) {
		// env://DB_PASSWORD  → Host="DB_PASSWORD"; env:///DB → Path.
		key := u.Host
		if key == "" {
			key = trimSlash(u.Path)
		}
		return envProvider{key: key}, nil
	})
}

func (p envProvider) Name() string { return "env" }

func (p envProvider) Get(ctx context.Context, key string) (Secret, bool, error) {
	// A URI-embedded key (env://DB_PASSWORD) pins the name; otherwise the
	// caller's key is the env var name.
	name := p.key
	if name == "" {
		name = key
	}
	v, ok := os.LookupEnv(name)
	if !ok {
		return Secret{}, false, nil
	}
	return NewSecret(v), true, nil
}

func trimSlash(s string) string {
	for len(s) > 0 && s[0] == '/' {
		s = s[1:]
	}
	return s
}
