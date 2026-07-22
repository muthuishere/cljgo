# Spike S39 — one pluggable vault interface: any backend behind a URI scheme

Opened 2026-07-23. Feeds the future **bri.vault** battery and the layered
config resolver (sibling spike **s38** owns the resolver; S39 owns the
*vault layer* it plugs the secret-bearing sources into).

## Context

The owner wants secrets to come from "any vault — any cloud or GitHub or
even env", with the app **never hardcoding a backend**. That is a classic
strategy-behind-an-interface problem: one `Provider` contract, many
implementations, selected at runtime by a **URI scheme in config**
(`vault: "keychain://myapp/db-password"`), plus a **fallback chain** so a
missing secret in one backend rolls to the next (keychain → env is the
canonical pair: real secret on a dev laptop, env var in CI/containers).

Two cljgo constraints dominate the design and are the reason this is a spike
and not a straight build:

- **Single static binary, `CGO_ENABLED=0`, pure-Go only.** A vault provider
  that needs cgo or a native lib (the usual keychain FFI story) would break
  the single-binary promise. The keychain provider is the load-bearing
  unknown: does `github.com/zalando/go-keyring` work with cgo **off** on
  macOS (it claims to shell out to `/usr/bin/security`)? If yes, the OS
  keychain ships. If no, it can't, and we say so.
- **Batteries live under `bri.*`, never shadow clojure.core.** The Clojure
  surface is `bri.vault`, e.g. `(bri.vault/get "keychain://myapp/db")`.

## The one question

**Can one `Provider` interface, selected by URI scheme, unify env + OS
keychain + an encrypted file + (stubbed) cloud secret managers — while
staying `CGO_ENABLED=0` pure-Go — and what is the blessed interface + scheme
registry?**

## Exit criteria (written before any code, per ADR 0027)

Met iff all hold, each backed by captured real output in `results/`:

1. **One interface, ≥3 REAL working providers**, each proven with a
   store→get round-trip whose readback is **MASKED** (never the value):
   - `env://KEY` — process env. The always-works bootstrap floor.
   - `keychain://service/account` — OS keychain via `zalando/go-keyring`,
     built and run with **`CGO_ENABLED=0`**. A green round-trip here is the
     critical single-binary finding; a build/run failure is an equally
     valid (negative) finding and must be reported honestly.
   - `age://path` (encrypted file) — a repo-committable ciphertext blob that
     decrypts with an age identity taken **from env**, using pure-Go
     `filippo.io/age`. Masked readback.

2. **A stub cloud provider** (interface-only, not dialed) for ≥1 of AWS
   Secrets Manager / GCP Secret Manager / HashiCorp Vault / GitHub Actions:
   its scheme (`aws-sm://`, `gh://`, `vault://`) resolves to the right
   provider through the registry, and the `Provider` shape is shown to FIT
   the real SDK call (cite the exact method). No real cloud creds required.

3. **Scheme→provider registry + fallback chain.** `Open(uri)` dispatches on
   scheme; `Chain(a,b,…)` tries in order and returns the first hit. Prove
   keychain-miss → env-hit rolls over, and that an all-miss chain returns a
   legible not-found (naming the key + the tried schemes).

4. **Integration seam for s38's resolver.** Expose a `Get`-shaped hook
   (`func(ctx, key) (string, bool, error)`) so the layered config resolver
   can treat the whole vault stack as one lookup layer. Show the adapter.

5. **Hygiene proof.** `grep` the spike's own captured output/logs and
   confirm **no secret value** appears — only masked forms
   (`len=… ***…xx` or a sha256 prefix). Document the masking helper.

## Non-goals

- Not building s38's resolver (only the seam).
- Not dialing real cloud APIs (stub + cited SDK method is enough).
- Not deciding which providers ship in bri.vault v1 — that's an owner gate
  the VERDICT raises.

## Layout

- `prototype/` — self-contained Go module `cljgospike/s39` (throwaway, never
  merges into `pkg/`, ADR 0027 §5).
- `results/` — captured transcripts (`e1`…`e6`).
- `VERDICT.md` — verdict, evidence, blessed form, un-proven risks, gates.
