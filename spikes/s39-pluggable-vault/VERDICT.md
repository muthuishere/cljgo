# Spike S39 verdict — one `Provider` interface + a scheme registry unifies every vault, and it stays CGO-free

Closed 2026-07-23. Feeds the future **bri.vault** battery and exposes the vault
layer seam for sibling spike **s38** (layered config resolver).

**Exit criteria: MET, all five.** One `Provider` interface, three REAL working
providers (env / keychain / age) each proven with a masked store→get round-trip,
four stub cloud providers whose schemes resolve and whose shapes fit the cited
SDK calls, a scheme registry + fallback chain proven to roll keychain-miss→env-hit,
the s38 resolver seam (`AsGetFunc`), and a clean hygiene grep. **The whole thing
builds and runs with `CGO_ENABLED=0`** — the load-bearing question.

Prototype: `prototype/` (self-contained Go module `cljgospike/s39`; never merges
into `pkg/`, ADR 0027 §5). Evidence: `results/e-run.txt` (proof run),
`results/e-static.txt` (static-linkage proof). API sketch: `api-sketch.cljg`.

Reproduce:
```
cd prototype
CGO_ENABLED=0 go build ./... && CGO_ENABLED=0 go vet ./... && gofmt -l .
CGO_ENABLED=0 go build -o /tmp/s39prove ./cmd/prove && /tmp/s39prove
```

---

## 1. The headline finding: the OS keychain ships — go-keyring is genuinely CGO-free

This is the finding that decides whether an OS-keychain provider is even
possible under cljgo's single-binary / `CGO_ENABLED=0` constraint. **It is.**

`github.com/zalando/go-keyring` v0.2.8 does NOT use cgo on any platform:

| OS | how it reaches the store | cgo? |
|---|---|---|
| macOS | execs `/usr/bin/security` (a subprocess) | no |
| Linux | speaks the D-Bus Secret Service protocol via `godbus/dbus` (pure Go over a unix socket) | no |
| Windows | `danieljoos/wincred` → `golang.org/x/sys/windows` syscalls | no |

Proven on macOS (`results/`):
- `CGO_ENABLED=0 go build ./...` compiles the keychain provider clean.
- The built binary carries `build CGO_ENABLED=0` (`go version -m`).
- `otool -L` shows only `libSystem` + `libresolv` — the pure-Go macOS baseline
  (net/DNS + syscalls); **no Security.framework, no cgo runtime**.
- `go list -deps` reports **no package in the build tree has CgoFiles**.
- The string `/usr/bin/security` is embedded in the binary — it shells out, as
  documented.
- A live round-trip ran: `keyring.Set` then `Provider.Get` returned the secret
  (masked `len=25 ***…sh`), and after `keyring.Delete` the next Get correctly
  reported `hit=false`.

**Consequence:** `keychain://` is safe to ship in the single static binary. It
adds three pure-Go transitive deps (`godbus/dbus`, `wincred`, `x/sys`) and, on
macOS, a runtime dependency on `/usr/bin/security` existing on PATH (it always
does). No cgo, no native lib, no build-tag gymnastics.

## 2. The blessed interface

One contract, deliberately tiny — the (value, miss, failure) triad is the whole
insight, because it separates "not here, try the next backend" from "this
backend broke, stop":

```go
type Provider interface {
    Get(ctx context.Context, key string) (secret Secret, ok bool, err error)
    Name() string
}
```

- `ok=false, err=nil` → a **miss**: key absent here → the fallback chain rolls on.
- `err!=nil` → a **backend failure** (network, decrypt, permission): the chain
  **aborts** — we never silently fall past a broken keychain to a weaker source.
- `Secret` is a wrapper type whose `String()`/`GoString()` are **masked**, so
  `%v`/`%s`/`%#v`/log/REPL-echo can't leak it; the raw value leaves only through
  the single audited call `Secret.Reveal()`.

Selection + composition, all the app ever touches:

```go
vault.Open("keychain://myapp/db-password")                 // scheme → Provider
vault.OpenChain("keychain://myapp/db", "env://DB_PASSWORD") // fallback chain
vault.AsGetFunc(provider)  // → func(ctx,key)(string,bool,error) : the s38 seam
```

Providers self-register by scheme in `init()`, so **linking a provider file in =
enabling its scheme**; the app never names a concrete type. Registry is
`map[scheme]Opener`.

## 3. The scheme registry (blessed set)

| scheme | provider | status | backend |
|---|---|---|---|
| `env://KEY` | env | **REAL** | process env — the bootstrap floor / chain tail |
| `keychain://service/account` | keychain | **REAL, CGO-free** | OS keychain via go-keyring |
| `age://path?id=ENV` | age | **REAL** | age-encrypted committable file, identity from env, pure-Go `filippo.io/age` |
| `aws-sm://id?region=` | AWS Secrets Manager | **STUB** | `secretsmanager.GetSecretValue` (aws-sdk-go-v2, pure-Go) |
| `gcp-sm://projects/…/secrets/…/versions/…` | GCP Secret Manager | **STUB** | `AccessSecretVersion` (cloud.google.com/go, pure-Go) |
| `vault://mount/path#field` | HashiCorp Vault | **STUB** | `Secrets.KvV2Read` (vault-client-go, pure-Go) |
| `gh://owner/repo#NAME` | GitHub Actions | **STUB (read-limited)** | `ActionsService` (google/go-github, pure-Go) |

All three real providers proved a **masked** round-trip in one run
(`results/e-run.txt`), and the fallback chain `keychain:miss → env:hit` returned
the env value while an all-miss chain returned a clean `hit=false, err=nil`.

## 4. The two encrypted-file arms — why age, not sops

The "encrypted file store" arm is `filippo.io/age` directly (X25519 +
ChaCha20-Poly1305, **pure Go**): a committed `.age` blob (an encrypted JSON map
of key→secret) decrypts with an identity pulled from env. The demo generated an
identity, sealed a 2-key store to a 278-byte armored blob, committed-style read
it back, and returned `stripe-key` masked (`len=30 ***…ef`) — a missing key
correctly returned `hit=false`. **This gives the sops/age model with zero
external binary and zero cgo.** (`sops://` could be a thin alias later, but it
shells out to the `sops` CLI — an ambient dependency; age-native is the cleaner
default.)

## 5. Hygiene proof

`Mask(v)` renders `len=N ***…xy` (last 2 runes only) for logs/errors/REPL;
`MaskSHA(v)` is the zero-leak `sha256:` prefix. `Secret.String/GoString` route
through `Mask`, so a value cannot reach any output surface by accident — only
via `Reveal()`. Grepping the full proof transcript for every plaintext fixture
(`hunter2`, `swordfish`, `sk_live_FAKE_…`, `fallback-tuna`) returned **zero
matches**; only masked tails and sha prefixes appear. Demo fixtures are obvious
fakes, never real secrets.

## 6. The s38 resolver seam

`vault.AsGetFunc(Provider) → func(ctx,key)(string,bool,error)` collapses the
whole vault stack (single provider or chain) into one lookup layer the layered
config resolver registers alongside env/file/flag layers. The value is Revealed
only at that boundary; the resolver owns hygiene downstream (it must hold the
value in its own Secret-like, not a bare printed field). Proven in E6.

---

## UN-PROVEN risks

1. **Cloud providers are stubs, never dialed.** The `Get` *shape* fits the cited
   SDK method for each (one id in, one string out, a typed NotFound → `ok=false`),
   and all four cloud SDKs are pure-Go so they won't break `CGO_ENABLED=0`. But
   auth wiring, region/endpoint config, IAM/least-privilege, pagination of
   multi-field secrets, and error→(miss vs failure) mapping are unproven. Each
   real dial is its own follow-up.
2. **GitHub secrets are read-limited by GitHub, not by us.** Actions secrets are
   write-only out-of-workflow (`GetRepoSecret` returns metadata only); the value
   is readable only inside a running workflow, where it's really `env://` under
   `${{ secrets.NAME }}`. So `gh://` is honest for *seal/write* and for the
   in-workflow env path, but a general out-of-workflow `Get` cannot return a
   value. Documented in the stub; don't advertise `gh://` as a read source.
3. **Keychain write API shape.** The spike proved read + a test-only Set/Delete.
   A real `bri.vault` write surface (`seal`/`set`) is undesigned — should it live
   on `Provider` (many backends are read-only: age file, cloud-SM often) or a
   separate optional `Writer` interface? Recommend a separate `Writer`.
4. **Keychain on headless Linux/CI.** D-Bus Secret Service needs a running
   secret-service daemon + unlocked keyring; on a bare CI box there is none, so
   `keychain://` will *fail* (err, not miss) there — which is exactly why the
   canonical chain is `keychain:// → env://`. Unproven that the failure is a
   clean err and not a hang; worth a timeout wrapper.
5. **Context/timeout + caching** not exercised. `ctx` is threaded but no provider
   honors deadlines yet; no negative-cache or TTL. Cloud latency makes both
   matter later.
6. **`Reveal()` discipline is a convention, not enforced.** The type masks by
   default, but nothing stops a caller from `Reveal()`-ing into a log. A vet
   lint (`grep Reveal`) or a linter rule is the real guardrail — unbuilt.

## Owner-gated questions

1. **Which providers ship in bri.vault v1?** Recommendation: **env + keychain +
   age** (all REAL, all CGO-free, cover laptop + CI + committed-secrets). Ship
   the cloud four as **stubs behind the registry** so the scheme space is
   reserved and the interface is public, dialing them per-demand.
2. **Does `bri.vault/get` mask by default in the REPL** (return a `Secret` that
   echoes `len=N ***…xy`, forcing `(bri.vault/reveal s)` for plaintext) — or
   return a raw string like clojure.core? Recommendation: **mask by default**;
   it's the whole reason the value can't leak into a transcript, and it fits the
   owner's absolute hygiene policy. Costs one explicit `reveal` call at use.
3. **Write surface?** Do we expose `bri.vault/seal` / `set` at all in v1, or is
   bri.vault read-only (secrets are provisioned out-of-band)? Recommend read-only
   v1 + a separate optional `Writer` interface later (keychain + age + cloud-SM
   can all write; env cannot).
4. **`sops://` alias?** Ship age-native only (no external binary), or also accept
   sops-format files by shelling to the `sops` CLI when present? Recommend
   age-native only for v1 to keep the single-binary promise; revisit if a user
   already standardizes on sops.
5. **Chain config syntax.** `vault: "keychain://…"` (single) vs
   `vault: ["keychain://…" "env://…"]` (chain) — bless the vector form as the
   general case (single-string is the 1-element sugar)? Recommend yes.
