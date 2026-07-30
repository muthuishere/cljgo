// Spike s60-cljg-net-dns: prove cljg.net.dns (Bun.dns analog) in PURE Go.
//
// Every lookup below goes through net.Resolver with PreferGo:true, which forces
// the built-in pure-Go DNS resolver (never the cgo/libc getaddrinfo path). Under
// CGO_ENABLED=0 the cgo resolver is not even compiled in, so the pure-Go resolver
// is the only one available regardless. We prove both here.
package main

import (
	"context"
	"fmt"
	"net"
	"sort"
	"time"
)

// Force the pure-Go resolver. This is the exact knob a cljg.net.dns runtime shim
// would set so behavior is identical whether the host binary is built CGO=1 or CGO=0.
var goResolver = &net.Resolver{PreferGo: true}

func line(label, got string, err error) {
	if err != nil {
		fmt.Printf("  %-10s ERR  %v\n", label, err)
		return
	}
	fmt.Printf("  %-10s OK   %s\n", label, got)
}

func main() {
	ctx, cancel := context.WithTimeout(context.Background(), 8*time.Second)
	defer cancel()

	fmt.Println("== cljg.net.dns pure-Go resolver spike ==")
	fmt.Printf("resolver.PreferGo = %v (pure-Go DNS; no cgo getaddrinfo)\n\n", goResolver.PreferGo)

	// --- A / AAAA: LookupHost + LookupIPAddr on a real host ---
	fmt.Println("[A/AAAA] example.com")
	if addrs, err := goResolver.LookupHost(ctx, "example.com"); err != nil {
		line("LookupHost", "", err)
	} else {
		sort.Strings(addrs)
		line("LookupHost", fmt.Sprint(addrs), nil)
	}
	if ips, err := goResolver.LookupIPAddr(ctx, "example.com"); err != nil {
		line("LookupIP", "", err)
	} else {
		var v4, v6 int
		var first string
		for _, ip := range ips {
			if ip.IP.To4() != nil {
				v4++
			} else {
				v6++
			}
			if first == "" {
				first = ip.IP.String()
			}
		}
		line("LookupIP", fmt.Sprintf("%d IPv4 + %d IPv6 (first=%s)", v4, v6, first), nil)
	}

	// --- Reverse (PTR) ---
	fmt.Println("\n[PTR] reverse of 8.8.8.8")
	if names, err := goResolver.LookupAddr(ctx, "8.8.8.8"); err != nil {
		line("LookupAddr", "", err)
	} else {
		line("LookupAddr", fmt.Sprint(names), nil)
	}

	// --- MX ---
	fmt.Println("\n[MX] gmail.com")
	if mxs, err := goResolver.LookupMX(ctx, "gmail.com"); err != nil {
		line("LookupMX", "", err)
	} else {
		var s []string
		for _, mx := range mxs {
			s = append(s, fmt.Sprintf("%s(%d)", mx.Host, mx.Pref))
		}
		line("LookupMX", fmt.Sprint(s), nil)
	}

	// --- TXT ---
	fmt.Println("\n[TXT] google.com")
	if txts, err := goResolver.LookupTXT(ctx, "google.com"); err != nil {
		line("LookupTXT", "", err)
	} else {
		got := fmt.Sprintf("%d records; first=%q", len(txts), firstOr(txts, ""))
		line("LookupTXT", got, nil)
	}

	// --- SRV ---
	fmt.Println("\n[SRV] _imaps._tcp.gmail.com")
	if cname, srvs, err := goResolver.LookupSRV(ctx, "imaps", "tcp", "gmail.com"); err != nil {
		line("LookupSRV", "", err)
	} else {
		var s []string
		for _, srv := range srvs {
			s = append(s, fmt.Sprintf("%s:%d(p%d,w%d)", srv.Target, srv.Port, srv.Priority, srv.Weight))
		}
		line("LookupSRV", fmt.Sprintf("cname=%s targets=%v", cname, s), nil)
	}

	// --- CNAME ---
	fmt.Println("\n[CNAME] www.github.com")
	if cname, err := goResolver.LookupCNAME(ctx, "www.github.com"); err != nil {
		line("LookupCNAME", "", err)
	} else {
		line("LookupCNAME", cname, nil)
	}

	// --- NS ---
	fmt.Println("\n[NS] google.com")
	if nss, err := goResolver.LookupNS(ctx, "google.com"); err != nil {
		line("LookupNS", "", err)
	} else {
		var s []string
		for _, ns := range nss {
			s = append(s, ns.Host)
		}
		sort.Strings(s)
		line("LookupNS", fmt.Sprint(s), nil)
	}

	fmt.Println("\n== done ==")
}

func firstOr(s []string, def string) string {
	if len(s) == 0 {
		return def
	}
	return s[0]
}
