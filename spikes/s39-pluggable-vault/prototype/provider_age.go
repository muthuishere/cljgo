package vault

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/url"
	"os"

	"filippo.io/age"
)

// ageProvider decrypts a repo-committable ciphertext file with an age identity
// taken from the environment. Scheme: age://path?id=ENV_VAR_NAME
//
//   - The .age blob (an age-encrypted JSON map of key→secret) is safe to commit:
//     it's useless without the identity, which lives only in env / a keychain.
//   - filippo.io/age is PURE GO (X25519 + ChaCha20-Poly1305) → CGO_ENABLED=0.
//   - The identity env var name is configurable; default S39_AGE_IDENTITY.
//
// This is the "encrypted file store" arm — the sops/age model without the sops
// binary. Proven in results/e3.
type ageProvider struct {
	path  string
	idEnv string
}

func init() {
	Register("age", func(u *url.URL) (Provider, error) {
		// age://relative/path  → Host="relative", Path="/path"  → "relative/path"
		// age:///absolute/path → Host="",         Path="/absolute/path" → keep absolute
		var p string
		if u.Host != "" {
			p = u.Host + u.Path
		} else {
			p = u.Path
		}
		idEnv := u.Query().Get("id")
		if idEnv == "" {
			idEnv = "S39_AGE_IDENTITY"
		}
		return ageProvider{path: p, idEnv: idEnv}, nil
	})
}

func (p ageProvider) Name() string { return "age:" + p.path }

func (p ageProvider) Get(ctx context.Context, key string) (Secret, bool, error) {
	idStr, ok := os.LookupEnv(p.idEnv)
	if !ok {
		return Secret{}, false, fmt.Errorf("age identity env %q not set", p.idEnv)
	}
	id, err := age.ParseX25519Identity(idStr)
	if err != nil {
		return Secret{}, false, fmt.Errorf("age: bad identity in %q: %w", p.idEnv, err)
	}
	ct, err := os.ReadFile(p.path)
	if err != nil {
		return Secret{}, false, fmt.Errorf("age: read %q: %w", p.path, err)
	}
	r, err := age.Decrypt(bytes.NewReader(ct), id)
	if err != nil {
		return Secret{}, false, fmt.Errorf("age: decrypt %q: %w", p.path, err)
	}
	pt, err := io.ReadAll(r)
	if err != nil {
		return Secret{}, false, fmt.Errorf("age: read plaintext: %w", err)
	}
	var m map[string]string
	if err := json.Unmarshal(pt, &m); err != nil {
		return Secret{}, false, fmt.Errorf("age: store is not a JSON map: %w", err)
	}
	v, found := m[key]
	if !found {
		return Secret{}, false, nil // key not in this store → fallback
	}
	return NewSecret(v), true, nil
}

// EncryptStore is a spike helper: build an age blob from a map, given a
// recipient (public key). Writes ciphertext to w. A real bri.vault would ship
// this behind `bri.vault/seal`.
func EncryptStore(w io.Writer, recipientPub string, m map[string]string) error {
	rcpt, err := age.ParseX25519Recipient(recipientPub)
	if err != nil {
		return fmt.Errorf("age: bad recipient: %w", err)
	}
	pt, err := json.Marshal(m)
	if err != nil {
		return err
	}
	aw, err := age.Encrypt(w, rcpt)
	if err != nil {
		return err
	}
	if _, err := aw.Write(pt); err != nil {
		return err
	}
	return aw.Close()
}
