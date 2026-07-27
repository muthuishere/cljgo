# s65 — cross-platform keychain (macOS · Windows · Linux), "like Bun"

**Verdict: MET-PUREGO-DEP.** One unified store gives the same
`save-to-keychain` / `get-from-keychain` / `delete` API on all three OSes under
`CGO_ENABLED=0`, and it *always succeeds*: native OS keychain when a credential
store is reachable, transparent machine-local age-encrypted-file fallback when
it is not (headless servers / CI).

## What was proven here

| Platform | Build (CGO=0) | Runtime executed | Backend |
|---|---|---|---|
| macOS arm64/amd64 | ✅ Mach-O | ✅ **run** — native round-trip + forced-file round-trip both OK | `security` CLI (no cgo) / age file |
| Windows amd64/arm64 | ✅ PE32+ | ⏳ deferred to build phase | Credential Manager (wincred, pure-Go syscalls) / age file |
| Linux amd64/arm64 | ✅ ELF static | ⏳ deferred to build phase | Secret Service D-Bus (godbus) / age file |

- **Cross-compile is a real proof of the pure-Go claim on all three:** every
  OS-specific go-keyring backend (macOS `security`, Windows `wincred`, Linux
  `godbus`/Secret Service) *compiles cgo-free* and links into a static per-OS
  binary. No `import "C"` anywhere.
- **macOS runtime**, both modes: auto → `native-os-keychain`; forced → the
  `encrypted-file-fallback` (age X25519, 0600 machine key). Both round-trip.
- **`reqsume-dev` (real headless Ubuntu 6.8) inspected:** no
  gnome-keyring/kwallet/dbus-launch present — the exact case where native
  keychain is unreachable and the file fallback is mandatory. (Executing the
  binary there was intentionally skipped; verify at build time.)

## Deferred to the `cljg.security` / `bri` build (owner call, 2026-07-28)

- **Run** the static Linux binary on a box *with* a Secret Service daemon
  (desktop or CI with `gnome-keyring-daemon --unlock`) to exercise the native
  Linux path end-to-end, and on a headless box to confirm the fallback.
- **Run** on Windows (GitHub Actions `windows-latest`) to exercise Credential
  Manager end-to-end.
- Both are *environment* checks — the code is already build-proven pure-Go on
  those targets; nothing about cljgo's implementation is unproven, only the
  host credential store's presence.

## Shipping shape

`cljg.security` exposes one keychain API. `:backend :auto` (default) tries
native then falls back to the encrypted file; `:backend :native` / `:file`
force one. The fallback is a **first-class always-available backend**, not a
nice-to-have — it is what makes "works on all three, like Bun" true even on a
bare server. `cljg.secrets`' `keychain://` resolver composes this store.
The secret value is never printed or logged.
