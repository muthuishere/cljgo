package vault

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
)

// Mask renders a secret safe for any output surface (logs, chat, REPL echo,
// error strings). It NEVER returns the value. Two forms:
//   - short/empty → fixed placeholder, so length of tiny secrets doesn't leak.
//   - otherwise   → "len=N ***…xy" where xy is only the last 2 runes.
//
// The last-2 tail is a deliberate, minimal disambiguator (tell two secrets
// apart in a log) at the cost of ~1 byte of entropy — acceptable per owner
// hygiene policy; drop to MaskSHA when even that is too much.
func Mask(v string) string {
	n := len(v)
	if n == 0 {
		return "***(empty)"
	}
	if n < 4 {
		return fmt.Sprintf("len=%d ***", n)
	}
	return fmt.Sprintf("len=%d ***…%s", n, v[n-2:])
}

// MaskSHA is the zero-leak form: a sha256 prefix, no plaintext bytes at all.
// Use when a secret must be identified across runs without any tail leak.
func MaskSHA(v string) string {
	sum := sha256.Sum256([]byte(v))
	return "sha256:" + hex.EncodeToString(sum[:])[:12]
}
