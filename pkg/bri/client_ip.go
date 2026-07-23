// client_ip.go — the ONE blessed client-IP resolver bri.http/client-ip
// leans on (ADR 0069 abuse-protection §). RemoteAddr is the socket peer
// — unspoofable, but it is the PROXY's address behind an LB/CDN. We
// honor X-Forwarded-For / X-Real-IP ONLY when the immediate peer is a
// configured trusted proxy (APP_HTTP__TRUSTED_PROXIES CIDRs), taking the
// right-most hop that is NOT itself a trusted proxy. Trusting XFF from
// an untrusted peer is the classic ban-evasion / IP-spoofing bypass, so
// we never do it.
package bri

import (
	"net"
	"net/netip"
	"strings"

	"github.com/muthuishere/cljgo/pkg/lang"
)

// clientIP resolves the effective client IP. trusted is a cljgo vector
// of CIDR strings (may be nil/empty → never trust XFF).
func clientIP(remoteAddr, xff, xreal string, trusted any) string {
	peer := hostOnly(remoteAddr)
	cidrs := parseCIDRs(trusted)
	// No trusted proxies configured, or the peer isn't one → the socket
	// peer is the only trustworthy identity.
	if len(cidrs) == 0 || !ipInAny(peer, cidrs) {
		return peer
	}
	// Peer is a trusted proxy: walk XFF right-to-left (right = closest
	// hop) and return the first address that is NOT a trusted proxy — the
	// real client as seen entering our trusted edge.
	if xff != "" {
		hops := strings.Split(xff, ",")
		for i := len(hops) - 1; i >= 0; i-- {
			ip := strings.TrimSpace(hops[i])
			if ip == "" {
				continue
			}
			if !ipInAny(ip, cidrs) {
				return ip
			}
		}
	}
	if xreal != "" {
		return strings.TrimSpace(xreal)
	}
	return peer
}

func hostOnly(remoteAddr string) string {
	if remoteAddr == "" {
		return ""
	}
	if host, _, err := net.SplitHostPort(remoteAddr); err == nil {
		return host
	}
	return remoteAddr
}

func parseCIDRs(trusted any) []netip.Prefix {
	var out []netip.Prefix
	for s := lang.Seq(trusted); s != nil; s = lang.Next(s) {
		str, ok := lang.First(s).(string)
		if !ok || strings.TrimSpace(str) == "" {
			continue
		}
		str = strings.TrimSpace(str)
		if p, err := netip.ParsePrefix(str); err == nil {
			out = append(out, p)
			continue
		}
		// bare IP → /32 or /128
		if a, err := netip.ParseAddr(str); err == nil {
			out = append(out, netip.PrefixFrom(a, a.BitLen()))
		}
	}
	return out
}

func ipInAny(ip string, cidrs []netip.Prefix) bool {
	a, err := netip.ParseAddr(ip)
	if err != nil {
		return false
	}
	for _, p := range cidrs {
		if p.Contains(a) {
			return true
		}
	}
	return false
}
