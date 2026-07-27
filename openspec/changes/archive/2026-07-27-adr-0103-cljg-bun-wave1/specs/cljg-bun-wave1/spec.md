## ADDED Requirements

### Requirement: cljg.* Bun-complete Wave-1 namespaces are lazy pure-Go mechanism

cljgo MUST ship `cljg.http` (serve), `cljg.socket`, `cljg.net.dns`,
`cljg.compress`, and `cljg.security` as `cljg.*` mechanism-tier namespaces
under the ADR 0085 taxonomy and the ADR 0103 line (transport/connection/crypto
primitive → cljg). Each MUST:

1. register in `bri.Specs()` (NOT `core.BootSources()`), lazy-load on first
   `require` in the interpreter, and AOT-link via the generated briaot twin —
   a binary that never requires it pays zero bytes;
2. build and run under `CGO_ENABLED=0` with pure-Go dependencies only
   (spike-proven s57–s65);
3. put hot paths in Go host shims (`pkg/bri/*.go`) under a thin Clojure API
   (ADR 0097 mandate A — no degraded secondary version);
4. carry dual-harness conformance (`conformance/tests/*.clj`, Eval + Compiled)
   with frozen `;; expect:` output — self-contained (loopback/no external
   network), REPL-vs-binary divergence is a release blocker.

#### Scenario: unused wave namespace costs nothing

- GIVEN a cljgo program that never requires any Wave-1 namespace
- WHEN it is AOT-compiled
- THEN the binary contains no Wave-1 shim symbols and no keychain/crypto deps
  beyond what core already links

### Requirement: cljg.http/serve is the raw server primitive; bri.web keeps its behavior

`cljg.http/serve` MUST expose the raw HTTP server (port + handler fn taking a
request map and returning a response map, TLS options, graceful stop) extracted
from `bri.web.http`. `bri.web.http` MUST rebuild on top of `cljg.http` with
byte-identical observable behavior: all pre-existing bri.web conformance tests
and `templates/web` output MUST pass unchanged.

#### Scenario: raw serve round-trip

- GIVEN `(cljg.http/serve {:port 0 :handler f})`
- WHEN a client GETs the bound address
- THEN `f`'s response map (status/headers/body) is returned and `stop`
  gracefully shuts the server down

### Requirement: cljg.security owns password, crypto and keychain primitives

`cljg.security` (renamed from `bri.core.security`, all callers updated, zero
stale references) MUST provide: argon2id `hash-password`/`check-password`
(names MUST NOT shadow the namespace's existing JWT `verify` — precedence
principle), `sha256`, `hmac`, secure `random`/`token`, `uuid`, base64/hex, and
`save-to-keychain`/`get-from-keychain`/`delete-from-keychain` backed by the s65
unified store: native OS keychain (go-keyring) when reachable, machine-local
age-encrypted-file fallback otherwise, selectable via `:backend
:auto|:native|:file`. Keychain dependencies MUST live in an isolated opt-in
package so always-linked packages stay keychain-free (CI-guarded). Secret
values MUST never be printed or logged by any code path.

#### Scenario: keychain works on a headless box

- GIVEN a machine with no OS credential store daemon
- WHEN `save-to-keychain` then `get-from-keychain` run with `:backend :auto`
- THEN the round-trip succeeds via the encrypted-file fallback and reports
  which backend served it
