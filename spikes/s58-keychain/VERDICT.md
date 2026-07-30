# Spike s58-keychain — VERDICT

**Capability:** `cljg.security` keychain / `cljg.secrets` `keychain://` resolver —
`saveinkeychain` + `retrivefromkeychain` (OS keychain write+read).

**Verdict: MET-PUREGO-DEP** (macOS + Windows) — **with a NEEDS-DAEMON caveat on
headless Linux**. Ship keychain **plus** a pure-Go encrypted-file fallback.

- CGO-free: **YES**. `CGO_ENABLED=0 go build -ldflags='-s -w'` succeeded.
- Ran clean: **YES**. Real Set→Get→Delete round-trip against **this macOS login
  keychain**, and a pure-Go age encrypted-file round-trip.
- Stripped binary: **2,550,290 bytes** (`/tmp/s58-keychain.bin`, includes the age
  fallback + x/crypto).

## Deps (all pure Go, no C files, permissive)

| module | version | license | pure Go | role |
|---|---|---|---|---|
| github.com/zalando/go-keyring | v0.2.6 | MIT | yes | OS keychain façade |
| github.com/godbus/dbus/v5 | v5.1.0 | BSD-2 | yes | Linux Secret Service (indirect) |
| github.com/danieljoos/wincred | v1.2.2 | MIT | yes | Windows credential store (indirect) |
| al.essio.dev/pkg/shellescape | v1.5.1 | MIT | yes | quotes args to `security` (indirect) |
| filippo.io/age | v1.2.1 | BSD-3 | yes | encrypted-file fallback |
| golang.org/x/crypto | v0.24.0 | BSD-3 | yes | scrypt/chacha for age (indirect) |

`otool -L` on the stripped binary shows only `libSystem.B.dylib` + `libresolv`
(what every pure-Go net binary links) — **no cgo, no Security.framework**. The
binary embeds the literal string `/usr/bin/security`: on macOS go-keyring
**shells out to `/usr/bin/security`**, it does not link the Keychain C API.

## Per-OS reality under CGO_ENABLED=0

| OS | mechanism | headless/CI status |
|---|---|---|
| **macOS** | shells out to `/usr/bin/security` (add-generic-password / find / delete) | **works** — proven here. Needs an unlocked login keychain (interactive session or `security unlock-keychain`). No cgo. |
| **Windows** | `wincred` = pure-Go syscalls to `advapi32` Credential Manager | **works** — no daemon, no cgo (not runtime-tested here). |
| **Linux** | D-Bus Secret Service (`godbus`, pure Go) → gnome-keyring / KWallet | **NEEDS-DAEMON** — requires a running secret-service provider + a D-Bus session bus. On a headless server / CI with no session keyring, `Set`/`Get` return an error (`org.freedesktop.DBus.Error.ServiceUnknown`). This is the real risk. |

## Fallback assessment (headless Linux)

`filippo.io/age` scrypt (passphrase) round-trip proven pure-Go under CGO=0 in the
same binary: encrypt→file (205 bytes) →decrypt→exact match. **Recommendation:
ship keychain + encrypted-file fallback.** On macOS/Windows use the native store;
on Linux try Secret Service, and when no daemon answers, fall back to an
age-encrypted file under e.g. `~/.config/cljgo/secrets.age` (passphrase from env
or prompt). nacl/secretbox (`x/crypto`) is an equally-pure lighter alternative if
we don't want the age envelope.

## Real captured run output

```
=== s58-keychain round-trip ===
runtime: darwin/arm64  CGO note: this binary built with CGO_ENABLED=0

[A] OS keychain (go-keyring)
  saveinkeychain  service="cljgo-spike-s58" user="keychain-test-key" -> OK
  retrivefromkeychain             -> "s3cr3t-value-1785178679"
  round-trip match: true
  cleanup: deleted keychain entry

[B] encrypted-file fallback (age scrypt, pure Go)
  encrypted -> secret.age (205 bytes on disk)
  decrypted                       -> "s3cr3t-value-1785178679"
  round-trip match: true

=== done ===
```

## Risks / honest caveats

- **Linux headless is the trap.** No secret-service daemon = keychain calls fail.
  The file fallback is mandatory for CI / servers, not optional.
- macOS depends on `/usr/bin/security` existing on PATH (always true on stock
  macOS) and an unlocked keychain; locked keychain / SSH-without-login-session
  can prompt or fail.
- go-keyring has a documented ~3000-byte secret size limit on some backends
  (macOS chunks internally). Fine for tokens/keys, not for large blobs.
- Adds 5 modules to `go.mod` (all MIT/BSD, all pure Go). Binary grows ~2.5 MB
  mostly from x/crypto + age; keychain-only (drop age) would be smaller.
- Not runtime-tested on Windows/Linux here — macOS is the only OS actually
  exercised. Cross-OS claims are from source inspection (pure-Go syscalls /
  D-Bus), not execution.
```
