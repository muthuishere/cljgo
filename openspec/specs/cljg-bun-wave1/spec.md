# cljg-bun-wave1 Specification

## Purpose
TBD - created by archiving change adr-0103-cljg-bun-wave1. Update Purpose after archive.
## Requirements
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

### Requirement: cljg.date formats and parses java.time patterns

`cljg.date` MUST provide `format-pattern` and `parse-pattern` accepting a
**java.time** pattern string, so a portable `.cljc` library can use one pattern
vocabulary across hosts with its `:clj` branch calling
`DateTimeFormatter/ofPattern` untranslated.

A compiled pattern MUST be represented as a sequence of independent literal
and field operations. It MUST NOT be represented as a Go reference-time layout
string, because Go's `time.Format` substitutes a text token conditionally on
the literal that follows it, which makes an adjacent literal able to silently
suppress a field.

Output MUST be byte-identical to `java.time` for every pattern accepted, under
`Locale.ENGLISH`.

#### Scenario: a text token adjacent to a literal still formats

- GIVEN the pattern `EEE'at' Z`
- WHEN an instant is formatted with it
- THEN the output matches java.time's, with the weekday substituted
- AND the weekday is NOT emitted as the literal text of the pattern token

#### Scenario: the fraction needs no separator

- GIVEN the patterns `HH:mm:ss.SSS`, `HH:mm:ss,SSS` and `SSS`
- WHEN an instant is formatted with each
- THEN each output matches java.time's, including the bare fraction

### Requirement: an unrepresentable pattern is refused by name, at compile time

Compiling a pattern MUST fail with a registered diagnostic naming the offending
token when the token cannot be represented exactly — era, quarter, day-of-year,
week-of-year, week-of-month, day-of-week-in-month, clock-hour variants,
week-based year, proleptic year, zone name, zone id — or when a token's run
length is invalid.

The failure MUST occur when the pattern is compiled, never when an instant is
formatted, and a token MUST NEVER be silently dropped or approximated.
A pattern `java.time` itself rejects MUST NOT be accepted.

#### Scenario: an unsupported token names itself

- GIVEN the pattern `QQ yyyy`
- WHEN it is compiled
- THEN compilation fails with a registered code and the message names `QQ`
- AND no instant is formatted

#### Scenario: cljgo does not accept what the JVM rejects

- GIVEN a pattern `java.time` rejects, such as `EEEEEEE` or `aa`
- WHEN it is compiled
- THEN compilation fails

### Requirement: pattern compilation is memoised, never per call

Formatting with the same pattern repeatedly MUST NOT recompile it. Per-call
compilation costs 6× the time and 60× the allocation of the compiled path,
which is the term that matters on a server path.

#### Scenario: repeated formatting does not recompile

- GIVEN a pattern formatted many times
- WHEN the same pattern string is used
- THEN the compiled form is reused
- AND concurrent use from many goroutines is race-free

