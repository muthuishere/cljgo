# ADR 0103 Wave 1 — Merge Plan

Base branch: `feat/clojure-contrib-tier1` (tip `d4105b5`; wave branches forked at `beba8eb`).
Each namespace was built on its own `wave1/<label>` branch off that base. They share the base
but each independently (a) appended one `bri.Specs()` row in `pkg/bri/bri.go` and (b) regenerated
`pkg/briaot/*`. Those two touch-points **conflict on merge** — the merge is not a clean fast-forward.

## Merge sequence (do this, in order)

1. Merge the four `wave1/*` branches into an integration branch off `feat/clojure-contrib-tier1`.
2. On every merge, take **both** sides of `pkg/bri/bri.go` — the Spec rows are additive and must
   all survive. For `cljg.security` this is a **rename-in-place** of the existing row, not an
   append (see below).
3. Take **both** sides of `core/cljg.go` — each ns added one `//go:embed` + `var …Source`.
4. **Discard all per-branch `pkg/briaot/` regen.** After the source-level merge is resolved, run
   the regenerator **once**:
   ```
   go generate ./pkg/briaot
   ```
   This is the single source of truth for `pkg/briaot/briaot.go` + the per-ns twin dirs. Never
   hand-merge generated files.
5. Run the full gate once, at the end (below).

Ordering note: appended Spec rows are order-sensitive for gensym stability in earlier-emitted
namespaces. Each branch placed its row **LAST** except `cljg.security`, which is a rename in place
at its existing position (5). Preserve that: security stays at position 5; compress / socket /
net-dns append after the existing tail, in any stable order among themselves.

---

## Status at a glance

| ns              | branch            | green | committed?                | notes |
|-----------------|-------------------|-------|---------------------------|-------|
| cljg.compress   | `wave1/compress`  | YES   | YES — pushed, commit `d452e4a` | ready to merge as-is |
| cljg.socket     | `wave1/socket`    | NO    | NO — worktree deleted mid-run, work lost | re-implement |
| cljg.net.dns    | `wave1/net-dns`   | NO    | NO — worktree deleted, source saved to scratchpad | reconstruct from artifacts |
| cljg.security   | `wave1/security`  | NO    | NO — worktree deleted before any edit | build from plan; RENAME + owner question |

Only **cljg.compress** actually landed on its branch. The other three lost their isolation
worktrees mid-task (external deletion) and have **zero commits** on their branches — they must be
(re)built in fresh worktrees off the base before this plan can complete.

---

## 1. cljg.compress — READY (green, committed)

Branch `wave1/compress`, commit `d452e4a`. gzip/gunzip, deflate/inflate, zlib-compress/
zlib-decompress; String-or-byte-array -> canonical signed byte-array (`[]int8`). Non-OptIn,
Spec row LAST, stdlib only (`compress/gzip` + `compress/flate` + `compress/zlib`).

NEW files:
- `core/cljg/compress.cljg`
- `pkg/bri/compress.go` (`installCompressShims`)
- `conformance/tests/cljg-compress-roundtrip.clj`

MODIFIED:
- `core/cljg.go` — `+ CljgCompressSource` embed
- `pkg/bri/bri.go` — Spec row (append LAST):
  ```go
  {Name: "cljg.compress", File: "cljg/compress.cljg", Pkg: "cljgcompress", Source: &core.CljgCompressSource, install: installCompressShims},
  ```

GENERATED (regen, do not hand-merge): `pkg/briaot/cljgcompress/cljgcompress.go`,
`pkg/briaot/briaot.go`.

Callers: none.

Deferred (ADR 0103 latitude): zstd (non-stdlib dep), streaming variants (cljg.stream has no public
byte-backed source constructor yet).

---

## 2. cljg.socket — RE-IMPLEMENT (work lost, no commits)

Branch `wave1/socket` (empty). TCP/UDP sockets over stdlib `net`; builds duplex connection maps
`{:in <writable> :out <readable> :addr :-conn}` on the EXISTING cljg.stream Readable/Writable
wrappers (ADR 0101 — one stream abstraction). Non-OptIn, Spec row LAST, stdlib only.

API: `listen` (net.Listen -> opaque `*socketListener` handle), `accept`/`dial` (duplex conn map;
dial defaults tcp, `{:proto "udp"}` = connected UDP), `close`, `local-addr`.

NEW files to recreate:
- `core/cljg/socket.cljg`
- `pkg/bri/socket.go` (`installSocketShims`: `-socket-listen/-accept/-dial/-close/-local-addr`;
  `socketConnMap` wraps net.Conn as cljg.stream duplex with a nil stream-closer, conn owns teardown)
- `conformance/tests/cljg-socket-echo.clj` (loopback TCP echo, oracle: skip, frozen
  `;; expect: ["ping" true true]`)

MODIFIED:
- `core/cljg.go` — `+ CljgSocketSource` embed
- `pkg/bri/bri.go` — Spec row (append LAST):
  ```go
  {Name: "cljg.socket", File: "cljg/socket.cljg", Pkg: "cljgsocket", Source: &core.CljgSocketSource, install: installSocketShims},
  ```

GENERATED: `pkg/briaot/cljgsocket/cljgsocket.go` (via the single regen).

Callers: none.

Re-verify after rebuild: (1) `future` + deref work in the **compiled** conformance harness — the
echo server uses `(future (accept …))`; it passed interpreted but the compiled leg never ran.
(2) No goroutine/fd leak when only one side of the duplex conn is closed.

---

## 3. cljg.net.dns — RECONSTRUCT from scratchpad (green build, no commit)

Branch `wave1/net-dns` (empty). Bun.dns analog over stdlib `net.LookupHost/LookupIP/LookupMX/
LookupTXT/LookupCNAME/LookupAddr`. Non-OptIn, Spec row LAST, stdlib only. Build passed once
(`CGO_ENABLED=0 go build ./...`) before the worktree was deleted; regen never ran.

API: `resolve` (host -> vector of IP strings), `lookup` (1/2-arity: `:ip/:a/:aaaa` -> maps or IP
strings, `:mx` -> `{:host :pref}`, `:txt` -> strings, `:cname` -> string), `reverse` (IP -> vector
of names). `resolve` and `reverse` collide with clojure.core -> the `refer` **excludes both**
(precedence principle, mirrors net_http excluding `get`).

**All finished source is saved** at
`…/01c95a46-…/scratchpad/net-dns-artifacts/` — `net_dns.go`, `net_dns.cljg`,
`cljg-net-dns-localhost.clj`, and `RECONSTRUCT.md` (exact edits). Drop the 4 files in place and
apply the two edits:

NEW files:
- `core/cljg/net_dns.cljg`
- `pkg/bri/net_dns.go` (`installDNSShims`)
- `conformance/tests/cljg-net-dns-localhost.clj` (loopback invariants only — `127.0.0.1` ∈ resolve
  and ∈ lookup `:a`; `reverse` returns a vector without freezing the /etc/hosts-variable PTR value;
  oracle: skip)

MODIFIED:
- `core/cljg.go` — `+ CljgNetDNSSource` embed (after `CljgProcessSource`)
- `pkg/bri/bri.go` — Spec row (append LAST):
  ```go
  {Name: "cljg.net.dns", File: "cljg/net_dns.cljg", Pkg: "cljgnetdns", Source: &core.CljgNetDNSSource, install: installDNSShims},
  ```

GENERATED: `pkg/briaot/cljgnetdns/cljgnetdns.go` (via the single regen). `TestGeneratedBriIsUpToDate`
fails until this runs.

Callers: none.

---

## 4. cljg.security — BUILD FROM PLAN (RENAME + owner question + CI keychain)

Branch `wave1/security` (empty). This is the **hardest** merge: a **RENAME**
`bri.core.security` (file `bri/auth.cljg`) -> `cljg.security` (ADR 0102 style), PLUS it must become
**OPT-IN** because it gains keychain access, and keychain (`zalando/go-keyring` + godbus) is
**forbidden** in the always-linked `pkg/bri` by `TestSecretsIsOptIn`.

Verified fact (base `beba8eb`): the namespace is **already** `bri.core.security` — `core/bri/
auth.cljg` opens `(in-ns 'bri.core.security)`. The string `bri.auth` only matches the **file path**
`bri/auth.cljg` + comments, not a namespace. Spec row is at `pkg/bri/bri.go:95`.

### Rename steps
1. `core/bri/auth.cljg` -> `core/cljg/security.cljg`; change `(in-ns 'bri.core.security)` ->
   `(in-ns 'cljg.security)`. Requires (`bri.web.http`, `bri.core.audit`, `clojure.string`) unchanged.
2. Move the embed OUT of `core/bri.go` into `core/cljg.go` as `CljgSecuritySource` with
   `//go:embed cljg/security.cljg`; delete `BriAuthSource`.
3. **Spec row rename-in-place** at `bri.go:95` (keep position 5 — its requires bri.web.http +
   bri.core.audit are rows 1 & 3):
   ```go
   {Name: "cljg.security", File: "cljg/security.cljg", Pkg: "cljgsecurity", Source: &core.CljgSecuritySource, install: installSecurityShims, OptIn: true, ShimImport: "github.com/muthuishere/cljgo/pkg/bri/security"},
   ```
4. `InstallShimsInto` (`bri.go:176`) currently runs **either** `s.install` **or**
   `installers[s.Name]`. Make it run **both** so cljg.security can carry package-bri crypto shims
   AND isolated registry keychain shims. Backward-compatible (existing namespaces have exactly one
   non-nil).
5. Keep the crypto shims in `pkg/bri` (rename `auth.go` -> `security.go`, `installAuthShims` ->
   `installSecurityShims`). They use only `crypto/*` + `x/crypto` (no go-keyring), so `pkg/bri`
   stays go-keyring-free and `TestSecretsIsOptIn` keeps passing. Add crypto shims: `-sha256`,
   `-hmac-sha256`, `-rand-bytes`/`-secure-random`, `-uuid` (v4), `-b64-encode/-decode`,
   `-hex-encode/-decode`. (`-argon2-hash/-verify`, `-rand-token` already exist.)
6. **NEW isolated opt-in package** `pkg/bri/security/security.go` (`package security`): imports
   `zalando/go-keyring` + `pkg/bri`; `init(){ bri.RegisterInstaller("cljg.security", installKeychainShims) }`;
   shims `-keychain-set` (keyring.Set), `-keychain-get` (keyring.Get, nil on ErrNotFound),
   `-keychain-del` (keyring.Delete, nil on ErrNotFound). **Never log the value.**
7. `cmd/genbri/main.go` + `pkg/briloader.go`: add blank import `_ …/pkg/bri/security` (next to
   otel / db / secrets) so keychain shims exist at compile time.
8. Regen: `go generate ./pkg/briaot` creates `pkg/briaot/cljgsecurity` (opt-in) and **REMOVES**
   `pkg/briaot/briauth` — **delete the old `briauth` dir**.
9. Extend `TestSecretsIsOptIn` (or add `TestSecurityKeychainIsOptIn`): assert
   `pkg/briaot/cljgsecurity` links go-keyring while alwaysLinked (`pkg/briaot`, `pkg/bri`,
   `pkg/briaot/brihttp`) still do NOT.

### Callers to update (grep-gate)
After the rename, grep and rewrite `bri.core.security` -> `cljg.security` across:
```
grep -rn 'bri.core.security' core pkg cmd templates examples conformance docs
```
`templates/web` likely requires it; docs mention it. **Grep-gate must be empty** for both
`bri.core.security` and the file path `bri/auth.cljg` before commit.

### Conformance
`core/cljg/security.cljg` roundtrips: hash/verify-password (oracle skip — argon2 non-deterministic
salt; freeze cljgo behavior w/ rationale), sha256 + hmac against known vectors, keychain
save->get->del roundtrip **guarded** (CI has no keychain — skip when e.g. `CLJG_KEYCHAIN_TEST`
env is unset, or inject a test store).

### ⚠ OPEN QUESTION FOR OWNER (blocks final API naming)
The password op the task calls `verify` **collides with the existing JWT `verify`** already in this
namespace. Precedence principle: do not shadow. **Recommendation:** keep password ops as
`hash-password`/`check-password` (existing names) or `hash`/`verify-password` — do NOT rename the
JWT `verify`. Needs an owner call before the public API freezes.

---

## Full gate (run ONCE after the merged tree + single regen)

```
CGO_ENABLED=0 go build ./... \
 && go vet ./... \
 && gofmt -l pkg cmd conformance templates core \
 && go generate ./pkg/briaot && git diff --exit-code pkg/briaot \
 && go test ./pkg/briaot/... ./pkg/coreaot/... ./pkg/bri/... ./conformance/... \
      -timeout 1800s -p 1 \
      -run 'Security|Keychain|Socket|NetDNS|Compress|Generated|OptIn|NoInterpreter|UpToDate'
```

Also confirm `pkg/bri/auth_test.go` + `secrets_test.go` still pass (they reference the crypto shims
being renamed). `gofmt -l` must be empty; `git diff --exit-code pkg/briaot` proves the twins match
the single regen.

### Known pre-existing (NOT wave-1 regressions)
The full conformance suite has pre-existing `clojure.test` `thrown?`/`thrown-with-msg?` failures
(`user/t-thrown`, `user/t-msg`) on base `feat/clojure-contrib-tier1`. Unrelated to any wave-1 ns
(all four are purely additive). Do not block the merge on them; do not attribute to this wave.
