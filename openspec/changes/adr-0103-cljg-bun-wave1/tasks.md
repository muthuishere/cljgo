# Tasks — adr-0103-cljg-bun-wave1

Gate for every task (foreground, long timeout):
`CGO_ENABLED=0 go build ./... && go vet ./... && gofmt -l pkg cmd conformance templates core && go test ./... -timeout 1800s -p 1`
Never hand-edit generated briaot files — run `go generate ./pkg/briaot`.

## 1. cljg.socket (stdlib, additive — easiest)

- [x] 1.1 `core/cljg/socket.cljg` (satellite preamble in-ns style) — listen/accept/dial/close for tcp+udp+unix; conn as duplex map composing cljg.stream; TLS via opts.
- [x] 1.2 Go shim `pkg/bri/cljg_socket.go` (installSocketShims) over stdlib net + crypto/tls.
- [x] 1.3 Spec row (LAST in Specs()) + embed in core/cljg.go + `go generate ./pkg/briaot`.
- [x] 1.4 Conformance: loopback TCP echo + UDP round-trip (self-contained, no network), dual harness.

## 2. cljg.net.dns (stdlib, additive)

- [ ] 2.1 `core/cljg/net_dns.cljg` — lookup/reverse/mx/txt/srv/cname/ns.
- [ ] 2.2 Go shim `pkg/bri/cljg_dns.go` (installDNSShims), Resolver{PreferGo:true}.
- [ ] 2.3 Spec row + embed + regenerate.
- [ ] 2.4 Conformance: shape-only tests that don't require network (e.g. localhost lookup + error shape), dual harness.

## 3. cljg.compress (stdlib, additive)

- [ ] 3.1 `core/cljg/compress.cljg` — gzip/deflate/zlib compress+decompress (string + bytes), streaming over cljg.stream.
- [ ] 3.2 Go shim `pkg/bri/cljg_compress.go` (installCompressShims).
- [ ] 3.3 Spec row + embed + regenerate.
- [ ] 3.4 Conformance: round-trip equality for all three codecs, dual harness.

## 4. cljg.security (rename + complete — careful)

- [ ] 4.1 Move `core/bri/auth.cljg` → `core/cljg/security.cljg`; in-ns → 'cljg.security; embed moves core/bri.go → core/cljg.go (CljgSecuritySource).
- [ ] 4.2 Rename spec row IN PLACE (bri.go:95, keep position): OptIn:true, ShimImport pkg/bri/security, install:installSecurityShims (renamed installAuthShims stays in pkg/bri).
- [ ] 4.3 InstallShimsInto runs BOTH s.install AND registry installer (currently either/or) — backward compatible.
- [ ] 4.4 New isolated `pkg/bri/security` pkg: keychain shims (-keychain-set/-get/-del) with s65 unified store (go-keyring native probe → age file fallback; machine key 0600). RegisterInstaller("cljg.security", ...). Never log values.
- [ ] 4.5 Add crypto shims to pkg/bri: -sha256, -hmac-sha256, -secure-random, -uuid, -b64-encode/-decode, -hex-encode/-decode (argon2 + rand-token exist).
- [ ] 4.6 cljg/security.cljg public API: hash-password/check-password, sha256, hmac, random, token, uuid, base64(+decode), hex(+decode), save-to-keychain/get-from-keychain/delete-from-keychain (:backend :auto|:native|:file). JWT sign/verify/issue/guard family unchanged.
- [ ] 4.7 genbri + briloader blank imports for pkg/bri/security; regenerate (old briauth dir goes away → cljgsecurity).
- [ ] 4.8 Rename ALL bri.core.security callers (grep-gate empty over core pkg cmd templates examples conformance docs).
- [ ] 4.9 Extend optin_linking_test: cljgsecurity links go-keyring; always-linked pkgs do NOT.
- [ ] 4.10 Conformance: sha256/hmac known vectors; hash/check-password round-trip (freeze cljgo, argon2 salted); keychain gated behind env var (CI has no keychain), dual harness.

## 5. cljg.http/serve extraction (most delicate — behavior frozen)

- [ ] 5.1 `core/cljg/http.cljg` — serve (port+handler, req map in/resp map out), TLS opts, stop (graceful Shutdown). Go shim carved from pkg/bri/http.go -serve into `pkg/bri/cljg_http.go` (installCljgHTTPShims); bri keeps its json/form/hmac helpers.
- [ ] 5.2 `bri.web.http` requires cljg.http and delegates its serve/listen plumbing to it; routing/middleware/ops/var-deref live-reload logic UNCHANGED.
- [ ] 5.3 Spec row + embed + regenerate (cljg.http BEFORE bri.web.http in Specs() so requires resolve).
- [ ] 5.4 Conformance: new cljg.http serve round-trip test (loopback GET via cljg.net.http); ALL existing bri.web conformance + templates/web stay byte-identical — zero diffs allowed.

## 6. Close-out

- [ ] 6.1 Full gate green on the merged branch; TestGeneratedBriIsUpToDate passes.
- [ ] 6.2 ADR 0103 status → accepted (Wave 1); note bri.ws/bri.grpc deferred.
- [ ] 6.3 Archive this change per openspec flow.
