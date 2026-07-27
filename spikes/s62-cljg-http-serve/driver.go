// Spike s62-cljg-http-serve: prove cljg.http/serve (Bun.serve analog) in pure Go.
//
// Round-trips exercised:
//  1. Plain HTTP/1.1 server on 127.0.0.1:0, real client GET, assert body.
//  2. HTTPS with an in-process self-signed cert (crypto/tls + crypto/x509),
//     real client GET over TLS, assert body + assert negotiated proto == h2.
//  3. h2c (HTTP/2 cleartext) via golang.org/x/net/http2/h2c, real prior-knowledge
//     h2 client GET, assert body + assert proto == HTTP/2.0.
//  4. Graceful shutdown (Server.Shutdown) — assert in-flight request completes
//     and the listener stops accepting.
package main

import (
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"fmt"
	"io"
	"math/big"
	"net"
	"net/http"
	"os"
	"sync"
	"time"

	"golang.org/x/net/http2"
	"golang.org/x/net/http2/h2c"
)

func die(msg string, err error) {
	fmt.Printf("FAIL: %s: %v\n", msg, err)
	os.Exit(1)
}

func assertEq(label, got, want string) {
	if got != want {
		die(fmt.Sprintf("%s mismatch: got %q want %q", label, got, want), fmt.Errorf("assertion failed"))
	}
	fmt.Printf("  OK  %s == %q\n", label, got)
}

// selfSignedCert generates a self-signed cert entirely in-process, pure Go.
func selfSignedCert() tls.Certificate {
	priv, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		die("ecdsa keygen", err)
	}
	tmpl := x509.Certificate{
		SerialNumber: big.NewInt(1),
		Subject:      pkix.Name{CommonName: "cljgo-spike"},
		NotBefore:    time.Now().Add(-time.Hour),
		NotAfter:     time.Now().Add(time.Hour),
		KeyUsage:     x509.KeyUsageDigitalSignature | x509.KeyUsageCertSign,
		ExtKeyUsage:  []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
		IPAddresses:  []net.IP{net.ParseIP("127.0.0.1")},
		DNSNames:     []string{"localhost"},
	}
	der, err := x509.CreateCertificate(rand.Reader, &tmpl, &tmpl, &priv.PublicKey, priv)
	if err != nil {
		die("x509 create cert", err)
	}
	return tls.Certificate{Certificate: [][]byte{der}, PrivateKey: priv}
}

// clientTrusting builds a client that trusts the given leaf cert (self-signed).
func clientTrusting(cert tls.Certificate, tlsCfg *tls.Config) *x509.CertPool {
	pool := x509.NewCertPool()
	leaf, err := x509.ParseCertificate(cert.Certificate[0])
	if err != nil {
		die("parse leaf", err)
	}
	pool.AddCert(leaf)
	return pool
}

func main() {
	fmt.Println("== s62 cljg.http/serve feasibility ==")

	// ---------- 1. Plain HTTP/1.1 ----------
	fmt.Println("[1] plain HTTP/1.1 server + client GET")
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprintf(w, "hello %s over %s", r.URL.Path[1:], r.Proto)
	})
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		die("listen", err)
	}
	srv := &http.Server{Handler: handler}
	go srv.Serve(ln)
	addr := ln.Addr().String()

	resp, err := http.Get("http://" + addr + "/world")
	if err != nil {
		die("http get", err)
	}
	body, _ := io.ReadAll(resp.Body)
	resp.Body.Close()
	fmt.Printf("  body: %q  proto: %s\n", string(body), resp.Proto)
	assertEq("http1 body", string(body), "hello world over HTTP/1.1")

	// ---------- 4a. Graceful shutdown of the plain server ----------
	fmt.Println("[4] graceful shutdown of HTTP/1.1 server")
	// fire an in-flight request against a slow handler via a second server.
	var wg sync.WaitGroup
	slowLn, _ := net.Listen("tcp", "127.0.0.1:0")
	slowSrv := &http.Server{Handler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(150 * time.Millisecond)
		fmt.Fprint(w, "drained-cleanly")
	})}
	go slowSrv.Serve(slowLn)
	slowAddr := slowLn.Addr().String()
	var inflightBody string
	wg.Add(1)
	go func() {
		defer wg.Done()
		r, e := http.Get("http://" + slowAddr + "/slow")
		if e != nil {
			die("inflight get", e)
		}
		b, _ := io.ReadAll(r.Body)
		r.Body.Close()
		inflightBody = string(b)
	}()
	time.Sleep(30 * time.Millisecond) // ensure request is in flight
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	if err := slowSrv.Shutdown(ctx); err != nil {
		die("shutdown", err)
	}
	cancel()
	wg.Wait()
	assertEq("in-flight request drained", inflightBody, "drained-cleanly")
	// listener should now refuse new connections.
	if _, e := http.Get("http://" + slowAddr + "/after"); e == nil {
		die("post-shutdown accept", fmt.Errorf("server still accepting after Shutdown"))
	}
	fmt.Println("  OK  server refuses new connections post-Shutdown")

	// shut down the plain server too.
	c2, cancel2 := context.WithTimeout(context.Background(), time.Second)
	srv.Shutdown(c2)
	cancel2()

	// ---------- 2. HTTPS + self-signed + HTTP/2 auto-negotiation ----------
	fmt.Println("[2] HTTPS (self-signed, in-process) + HTTP/2 auto-negotiation")
	cert := selfSignedCert()
	tlsLn, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		die("tls listen", err)
	}
	tlsSrv := &http.Server{
		Handler:   handler,
		TLSConfig: &tls.Config{Certificates: []tls.Certificate{cert}},
	}
	// ServeTLS with empty cert/key files uses TLSConfig.Certificates.
	// net/http auto-configures HTTP/2 (ALPN h2) when serving TLS.
	go tlsSrv.ServeTLS(tlsLn, "", "")
	tlsAddr := tlsLn.Addr().String()

	pool := clientTrusting(cert, nil)
	h2Client := &http.Client{
		Transport: &http.Transport{
			TLSClientConfig: &tls.Config{RootCAs: pool, ServerName: "127.0.0.1"},
			ForceAttemptHTTP2: true,
		},
	}
	r2, err := h2Client.Get("https://" + tlsAddr + "/tls")
	if err != nil {
		die("https get", err)
	}
	b2, _ := io.ReadAll(r2.Body)
	r2.Body.Close()
	fmt.Printf("  body: %q  proto: %s  tls-version: 0x%x\n", string(b2), r2.Proto, r2.TLS.Version)
	assertEq("https body", string(b2), "hello tls over HTTP/2.0")
	assertEq("negotiated proto (ALPN h2)", r2.Proto, "HTTP/2.0")

	c3, cancel3 := context.WithTimeout(context.Background(), time.Second)
	tlsSrv.Shutdown(c3)
	cancel3()

	// ---------- 3. h2c (HTTP/2 cleartext) via x/net/http2/h2c ----------
	fmt.Println("[3] h2c (HTTP/2 cleartext) via golang.org/x/net/http2/h2c")
	h2s := &http2.Server{}
	h2cLn, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		die("h2c listen", err)
	}
	h2cSrv := &http.Server{
		Handler: h2c.NewHandler(handler, h2s),
	}
	go h2cSrv.Serve(h2cLn)
	h2cAddr := h2cLn.Addr().String()

	// prior-knowledge h2c client: allow HTTP/2 over plaintext TCP.
	h2cClient := &http.Client{
		Transport: &http2.Transport{
			AllowHTTP: true,
			DialTLSContext: func(ctx context.Context, network, addr string, cfg *tls.Config) (net.Conn, error) {
				var d net.Dialer
				return d.DialContext(ctx, network, addr)
			},
		},
	}
	r3, err := h2cClient.Get("http://" + h2cAddr + "/cleartext")
	if err != nil {
		die("h2c get", err)
	}
	b3, _ := io.ReadAll(r3.Body)
	r3.Body.Close()
	fmt.Printf("  body: %q  proto: %s\n", string(b3), r3.Proto)
	assertEq("h2c body", string(b3), "hello cleartext over HTTP/2.0")
	assertEq("h2c negotiated proto", r3.Proto, "HTTP/2.0")

	c4, cancel4 := context.WithTimeout(context.Background(), time.Second)
	h2cSrv.Shutdown(c4)
	cancel4()

	fmt.Println("== ALL ROUND-TRIPS PASSED ==")
}
