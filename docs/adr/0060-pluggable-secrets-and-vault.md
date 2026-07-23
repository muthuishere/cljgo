# ADR 0060 — Pluggable secrets & vault (bri.vault)
Date: 2026-07-23 · Status: accepted (owner-directed, 2026-07-23; evidence: spike
S39) · Provides the **vault layer of ADR 0059**. Honors the owner's absolute
secret-hygiene policy.

## Context

The owner wants "any vault — any cloud or GitHub or even env," behind one
interface, backend chosen by config, with the hard constraint that a secret's
**value never enters logs, the REPL echo, chat, or any non-secret file**. Spike
S39 built it and proved it stays `CGO_ENABLED=0`.

## Decision

1. **One `Provider` interface**, selected by **URI scheme** from config; return
   triad `(Secret, ok, err)` — `ok=false` = miss / try next in chain, `err` =
   backend broke / abort. A chain therefore **never silently degrades** past a
   broken backend to a weaker source.
2. **v1 ships three REAL providers, all cgo-free:**
   - `env://KEY` — process env, the bootstrap floor.
   - `keychain://service/account` — OS keychain via `zalando/go-keyring` (execs
     `/usr/bin/security` on macOS, D-Bus Secret Service in pure Go on Linux,
     `x/sys` syscalls on Windows). **Verified static on macOS arm64** (`otool
     -L`: no Security.framework; zero CgoFiles); the Linux/Windows paths are
     cgo-free by inspection but unrun in CI (see Un-proven).
   - `age://path?id=ENV` — `filippo.io/age` (pure-Go X25519+ChaCha20); a
     committed encrypted blob decrypts with an env-held identity. The sops/age
     model with **no external binary**.
   Cloud backends `aws-sm://`, `gcp-sm://`, `vault://`, `gh://` are **reserved
   interface-shaped stubs** (their pure-Go SDKs won't break the static build);
   `gh://` is write/seal + the in-workflow `env://` path only — GitHub Actions
   secrets are write-only out-of-workflow.
3. **Registry** (scheme→opener, self-registering) + **`OpenChain`** fallback
   (`keychain` miss → `env` hit). Integrates as **ADR 0059 layer 5** via a
   `Get`-shaped hook.
4. **`Secret` is a masked type:** `String()`/`GoString()` render `len`+suffix
   (`***…ab`) so `%v`/logs/REPL can't leak it; plaintext exits only via an
   audited `Reveal()`. REPL echo masks by default. Writes (`seal`/`set`) are a
   separate `Writer` interface.

## Evidence (S39)

`CGO_ENABLED=0` static build confirmed (`otool -L`, zero CgoFiles);
env/keychain/age round-trips proven with masked readback; **grep of the full run
transcript for any plaintext fixture = zero matches.**

## Consequences

- Secrets get one pluggable surface that honors the hygiene rule **structurally**
  — the value cannot stringify by accident.
- Each cloud provider is one PR behind the already-proven interface.
- Un-proven: real cloud dials; keychain on Linux/Windows CI; `age` key
  management/rotation.
- Not chosen: a `Secret` that stringifies plainly; cgo keychain libraries;
  env-only.

**Constraint-filter #4 commitment (ADR 0056):** provider round-trips and the
masking invariant land with dual-harness conformance (a `.clj` test proving a
`Secret` never renders plaintext, interpreted AND AOT-compiled) and cross-OS
static-build checks in CI (the Linux/Windows keychain paths above). No perf
budget — a vault read is a one-shot config-time call, not a hot path.
