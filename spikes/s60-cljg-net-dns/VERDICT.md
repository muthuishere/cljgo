# VERDICT — s60-cljg-net-dns (cljg.net.dns)

**Verdict: MET-STDLIB.** All seven record types (A/AAAA, PTR, MX, TXT, SRV,
CNAME, NS) resolve with the Go standard library alone, forced through the
**pure-Go resolver** (`net.Resolver{PreferGo:true}`), building and running
under `CGO_ENABLED=0`. Zero third-party deps.

## Deps

| module | license | pure-Go |
|--------|---------|---------|
| (none — Go stdlib `net` only) | BSD-3-Clause (Go) | yes |

`go list -deps` shows `vendor/golang.org/x/net/dns/dnsmessage` — that is the DNS
wire-format parser **vendored into the Go stdlib itself** (part of the pure-Go
resolver), not an external `require`. `go.mod` has no `require` block.

## Binary

Stripped (`-ldflags='-s -w'`, `CGO_ENABLED=0`): **2,340,914 bytes** (~2.3 MB) at
`/tmp/s60-cljg-net-dns.bin`.

## Real captured run (compiled binary, CGO_ENABLED=0)

```
== cljg.net.dns pure-Go resolver spike ==
resolver.PreferGo = true (pure-Go DNS; no cgo getaddrinfo)

[A/AAAA] example.com
  LookupHost OK   [104.20.23.154 172.66.147.243 2606:4700:83b5:72db:f2fd:308:ef6b:ff98]
  LookupIP   OK   2 IPv4 + 1 IPv6 (first=2606:4700:83b5:72db:f2fd:308:ef6b:ff98)

[PTR] reverse of 8.8.8.8
  LookupAddr OK   [dns.google.]

[MX] gmail.com
  LookupMX   OK   [gmail-smtp-in.l.google.com.(5) alt1.gmail-smtp-in.l.google.com.(10) alt2.gmail-smtp-in.l.google.com.(20) alt3.gmail-smtp-in.l.google.com.(30) alt4.gmail-smtp-in.l.google.com.(40)]

[TXT] google.com
  LookupTXT  OK   15 records; first="cisco-ci-domain-verification=..."

[SRV] _imaps._tcp.gmail.com
  LookupSRV  OK   cname=_imaps._tcp.gmail.com. targets=[imap.gmail.com.:993(p5,w0)]

[CNAME] www.github.com
  LookupCNAME OK   github.com.

[NS] google.com
  LookupNS   OK   [ns1.google.com. ns2.google.com. ns3.google.com. ns4.google.com.]

== done ==
```

## Why CGO=0 is fine (the key point)

The stdlib `net` package has two resolvers: the **cgo resolver** (calls libc
`getaddrinfo`) and the **pure-Go resolver** (speaks DNS on the wire itself using
`golang.org/x/net/dns/dnsmessage`). Under `CGO_ENABLED=0` the cgo path is **not
compiled in at all**, so the pure-Go resolver is the only one available — CGO=0
does not disable DNS, it just guarantees the pure path. Setting `PreferGo:true`
makes that choice explicit and deterministic even in a CGO=1 host build, which
is exactly what a `cljg.net.dns` runtime shim should do so behavior is identical
across build modes and platforms.

## What `net` supports natively vs. what needs miekg/dns

| capability | stdlib `net` | notes |
|------------|--------------|-------|
| A / AAAA (LookupHost, LookupIPAddr) | ✅ | |
| PTR / reverse (LookupAddr) | ✅ | |
| MX (LookupMX) | ✅ | with preference |
| TXT (LookupTXT) | ✅ | |
| SRV (LookupSRV) | ✅ | priority/weight/port/target |
| CNAME (LookupCNAME) | ✅ | |
| NS (LookupNS) | ✅ | |
| custom nameserver / port | ⚠️ | via `Resolver.Dial` hook only (indirect) |
| raw arbitrary types (SOA, CAA, DNSKEY, ANY), DNSSEC validation, EDNS0, zone transfer (AXFR), raw response codes/flags | ❌ | need `github.com/miekg/dns` (pure Go, BSD-3) |

So the entire Bun.dns surface (`lookup`, `resolve`, `resolveMx`, `resolveTxt`,
`resolveSrv`, `resolveNs`, `resolveCname`, `reverse`) maps 1:1 onto stdlib `net`
with zero deps. Only advanced/raw DNS (SOA, CAA, DNSSEC, custom flags) would pull
in `miekg/dns` — still pure Go, CGO=0-clean — and only if cljg later wants it.

## Risks / caveats

- **Network required.** These are live lookups; results vary and an offline host
  yields real errors (handled per-line, does not crash the round-trip).
- **Custom resolver address** (query a specific DNS server:port) is only reachable
  via the `Resolver.Dial` callback in stdlib; if cljg wants a first-class
  "resolve against 1.1.1.1" API, `miekg/dns` is cleaner. Not needed for the
  Bun.dns baseline.
- **No DNSSEC / raw records** in stdlib — expected, out of Bun.dns scope.
- **SRV test hosts drift** (Google Voice SIP record was retired); pinned the run
  to a durable record (`_imaps._tcp.gmail.com`).

## Meaning for cljg

Ship `cljg.net.dns` as a thin corelib wrapper over `net.Resolver{PreferGo:true}`
— zero go.mod additions, CGO=0-clean, cross-platform. Full Bun.dns parity with
stdlib. Defer `miekg/dns` behind an optional `cljg.net.dns.raw` only if raw-type
/ DNSSEC / custom-server queries are ever requested.
