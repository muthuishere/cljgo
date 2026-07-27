// cljg_dns.go — the Go half of cljg.net.dns (ADR 0103 wave 1, spike s60): the
// Bun.dns analog. Seven record types (A/AAAA, PTR, MX, TXT, SRV, CNAME, NS)
// over the stdlib pure-Go resolver: net.Resolver{PreferGo:true} forces the
// built-in wire-format DNS client (never the cgo/libc getaddrinfo path), so
// behavior is identical whether the host binary is CGO=1 or CGO=0, on every
// platform (s60 VERDICT). Zero dependencies — stdlib net only — so this is a
// normal non-OptIn namespace. The ergonomic API lives in portable Clojure
// (core/cljg/net_dns.cljg); results are DATA (vectors of strings/maps,
// hostnames without the DNS trailing dot).
//
// Errors are informative ex-info values, never raw Go panics: the message
// names the query kind and the host, and ex-data carries
// {:type :cljg.net.dns/error :query <kw> :host <name>} so callers dispatch on
// shape without parsing prose (the underlying resolver error rides as the
// cause).
//
// cljg.net.dns rides the same name-generic embedded-namespace registry as bri
// and the other cljg.* namespaces (the pkg/bri package name is a legacy of
// bri being the first tenant — ADR 0087 §1).
package bri

import (
	"context"
	"fmt"
	"net"
	"sort"
	"strings"
	"time"

	"github.com/muthuishere/cljgo/pkg/lang"
)

// dnsResolver is the pure-Go stdlib resolver (s60): PreferGo pins the
// wire-format DNS client explicitly even in a CGO=1 host build, so lookups
// behave identically across build modes and platforms.
var dnsResolver = &net.Resolver{PreferGo: true}

// dnsTimeout bounds every lookup; a stuck nameserver becomes an error, not a
// hang.
const dnsTimeout = 10 * time.Second

// installDNSShims interns cljg.net.dns's private Go lookup primitives.
func installDNSShims(def func(name string, fn func(args ...any) any)) {
	// -dns-lookup host -> sorted vector of IP strings (A + AAAA).
	def("-dns-lookup", func(args ...any) any {
		host := asString(one("-dns-lookup", args))
		ctx, cancel := dnsCtx()
		defer cancel()
		addrs, err := dnsResolver.LookupHost(ctx, host)
		if err != nil {
			panic(dnsError("lookup", host, err))
		}
		sort.Strings(addrs)
		return dnsStringVector(addrs)
	})

	// -dns-reverse ip -> sorted vector of hostnames (PTR), trailing dot
	// trimmed.
	def("-dns-reverse", func(args ...any) any {
		ip := asString(one("-dns-reverse", args))
		ctx, cancel := dnsCtx()
		defer cancel()
		names, err := dnsResolver.LookupAddr(ctx, ip)
		if err != nil {
			panic(dnsError("reverse", ip, err))
		}
		for i, n := range names {
			names[i] = dnsTrimDot(n)
		}
		sort.Strings(names)
		return dnsStringVector(names)
	})

	// -dns-mx domain -> vector of {:host string :preference int} maps,
	// sorted by preference (LookupMX's order).
	def("-dns-mx", func(args ...any) any {
		domain := asString(one("-dns-mx", args))
		ctx, cancel := dnsCtx()
		defer cancel()
		mxs, err := dnsResolver.LookupMX(ctx, domain)
		if err != nil {
			panic(dnsError("mx", domain, err))
		}
		out := make([]any, len(mxs))
		for i, mx := range mxs {
			out[i] = lang.NewMap(
				lang.NewKeyword("host"), dnsTrimDot(mx.Host),
				lang.NewKeyword("preference"), int64(mx.Pref),
			)
		}
		return lang.NewVectorOwning(out)
	})

	// -dns-txt domain -> vector of TXT record strings.
	def("-dns-txt", func(args ...any) any {
		domain := asString(one("-dns-txt", args))
		ctx, cancel := dnsCtx()
		defer cancel()
		txts, err := dnsResolver.LookupTXT(ctx, domain)
		if err != nil {
			panic(dnsError("txt", domain, err))
		}
		return dnsStringVector(txts)
	})

	// -dns-srv service proto domain -> vector of {:target :port :priority
	// :weight} maps in the resolver's RFC 2782 order.
	def("-dns-srv", func(args ...any) any {
		if len(args) != 3 {
			panic(fmt.Errorf("-dns-srv expects 3 args (service proto domain), got %d", len(args)))
		}
		service, proto, domain := asString(args[0]), asString(args[1]), asString(args[2])
		name := "_" + service + "._" + proto + "." + domain
		ctx, cancel := dnsCtx()
		defer cancel()
		_, srvs, err := dnsResolver.LookupSRV(ctx, service, proto, domain)
		if err != nil {
			panic(dnsError("srv", name, err))
		}
		out := make([]any, len(srvs))
		for i, srv := range srvs {
			out[i] = lang.NewMap(
				lang.NewKeyword("target"), dnsTrimDot(srv.Target),
				lang.NewKeyword("port"), int64(srv.Port),
				lang.NewKeyword("priority"), int64(srv.Priority),
				lang.NewKeyword("weight"), int64(srv.Weight),
			)
		}
		return lang.NewVectorOwning(out)
	})

	// -dns-cname domain -> the canonical name string, trailing dot trimmed.
	def("-dns-cname", func(args ...any) any {
		domain := asString(one("-dns-cname", args))
		ctx, cancel := dnsCtx()
		defer cancel()
		cname, err := dnsResolver.LookupCNAME(ctx, domain)
		if err != nil {
			panic(dnsError("cname", domain, err))
		}
		return dnsTrimDot(cname)
	})

	// -dns-ns domain -> sorted vector of nameserver hostnames.
	def("-dns-ns", func(args ...any) any {
		domain := asString(one("-dns-ns", args))
		ctx, cancel := dnsCtx()
		defer cancel()
		nss, err := dnsResolver.LookupNS(ctx, domain)
		if err != nil {
			panic(dnsError("ns", domain, err))
		}
		names := make([]string, len(nss))
		for i, ns := range nss {
			names[i] = dnsTrimDot(ns.Host)
		}
		sort.Strings(names)
		return dnsStringVector(names)
	})
}

// dnsCtx bounds one lookup with the shared timeout.
func dnsCtx() (context.Context, context.CancelFunc) {
	return context.WithTimeout(context.Background(), dnsTimeout)
}

// dnsError builds the informative ex-info a failed lookup throws: the message
// names the query kind and the host (error doctrine — name the thing), and
// ex-data carries {:type :cljg.net.dns/error :query <kw> :host <name>} so
// callers dispatch on the shape, with the resolver error as the cause.
func dnsError(query, host string, err error) error {
	return lang.NewExceptionInfoWithCause(
		fmt.Sprintf("cljg.net.dns: %s lookup for %q failed", query, host),
		lang.NewMap(
			lang.NewKeyword("type"), lang.NewKeyword("cljg.net.dns/error"),
			lang.NewKeyword("query"), lang.NewKeyword(query),
			lang.NewKeyword("host"), host,
		),
		err,
	)
}

// dnsStringVector wraps a []string as an immutable vector of strings.
func dnsStringVector(ss []string) any {
	out := make([]any, len(ss))
	for i, s := range ss {
		out[i] = s
	}
	return lang.NewVectorOwning(out)
}

// dnsTrimDot strips the DNS trailing dot ("dns.google." -> "dns.google") so
// results read as ordinary hostnames.
func dnsTrimDot(s string) string {
	return strings.TrimSuffix(s, ".")
}
