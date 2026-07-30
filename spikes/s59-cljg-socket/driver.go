// Spike s59-cljg-socket: prove cljg.socket (Bun.listen analog) in pure Go stdlib (net).
//
// Round-trips exercised:
//  1. TCP  — listen 127.0.0.1:0, accept + echo server; client dials, writes, reads back.
//  2. UDP  — ListenPacket, WriteTo (sendto), ReadFrom (readfrom) — datagram round-trip.
//  3. Unix — socketpair via a listening unix domain socket (Bun `unix:` analog).
//  4. Stream shape — a net.Conn IS an io.ReadWriteCloser, so it wraps directly as
//     cljg.stream (ADR 0101): we io.Copy through one and bufio-scan lines from it.
//
// Everything below is stdlib only: net, io, bufio, crypto/tls, crypto/{rand,rsa,x509}.
package main

import (
	"bufio"
	"crypto/rand"
	"crypto/rsa"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"fmt"
	"io"
	"math/big"
	"net"
	"os"
	"path/filepath"
	"sync"
	"time"
)

func must(err error) {
	if err != nil {
		panic(err)
	}
}

// tcpEcho demonstrates a TCP listener that accepts and echoes, and a client round-trip.
func tcpEcho() {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	must(err)
	defer ln.Close()
	addr := ln.Addr().String()

	go func() {
		conn, err := ln.Accept()
		if err != nil {
			return
		}
		defer conn.Close()
		// conn is io.ReadWriteCloser — echo the stream through io.Copy.
		io.Copy(conn, conn)
	}()

	conn, err := net.Dial("tcp", addr)
	must(err)
	defer conn.Close()
	msg := "hello over tcp\n"
	_, err = conn.Write([]byte(msg))
	must(err)
	got, err := bufio.NewReader(conn).ReadString('\n')
	must(err)
	fmt.Printf("[tcp ]  listen %s  -> sent %q  got %q\n", addr, msg, got)
}

// udpDatagram demonstrates ListenPacket + WriteTo/ReadFrom.
func udpDatagram() {
	// server socket
	srv, err := net.ListenPacket("udp", "127.0.0.1:0")
	must(err)
	defer srv.Close()
	srvAddr := srv.LocalAddr()

	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		buf := make([]byte, 1500)
		srv.SetReadDeadline(time.Now().Add(2 * time.Second))
		n, from, err := srv.ReadFrom(buf) // readfrom
		if err != nil {
			return
		}
		// echo back to sender
		srv.WriteTo(buf[:n], from) // sendto
	}()

	cli, err := net.ListenPacket("udp", "127.0.0.1:0")
	must(err)
	defer cli.Close()
	payload := []byte("hello over udp")
	_, err = cli.WriteTo(payload, srvAddr) // sendto
	must(err)

	buf := make([]byte, 1500)
	cli.SetReadDeadline(time.Now().Add(2 * time.Second))
	n, from, err := cli.ReadFrom(buf) // readfrom
	must(err)
	fmt.Printf("[udp ]  listen %s  -> sent %q  got %q from %s\n",
		srvAddr, string(payload), string(buf[:n]), from)
	wg.Wait()
}

// unixSocket demonstrates a unix domain socket (Bun `unix:` path listener).
func unixSocket() {
	dir, err := os.MkdirTemp("", "cljgsock")
	must(err)
	defer os.RemoveAll(dir)
	sockPath := filepath.Join(dir, "echo.sock")

	ln, err := net.Listen("unix", sockPath)
	must(err)
	defer ln.Close()

	go func() {
		conn, err := ln.Accept()
		if err != nil {
			return
		}
		defer conn.Close()
		io.Copy(conn, conn)
	}()

	conn, err := net.Dial("unix", sockPath)
	must(err)
	defer conn.Close()
	msg := "hello over unix\n"
	_, err = conn.Write([]byte(msg))
	must(err)
	got, err := bufio.NewReader(conn).ReadString('\n')
	must(err)
	fmt.Printf("[unix]  listen %s  -> sent %q  got %q\n", sockPath, msg, got)
}

// streamShape proves a net.Conn satisfies io.Reader/io.Writer/io.Closer so it
// wraps directly as cljg.stream (ADR 0101). We assign it into the interface
// types and drive it purely through the stream API.
func streamShape() {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	must(err)
	defer ln.Close()

	go func() {
		conn, err := ln.Accept()
		if err != nil {
			return
		}
		defer conn.Close()
		// treat inbound conn as a pure io.Writer sink of two lines
		var w io.Writer = conn
		io.WriteString(w, "line-one\n")
		io.WriteString(w, "line-two\n")
	}()

	conn, err := net.Dial("tcp", ln.Addr().String())
	must(err)
	defer conn.Close()

	// Compile-time proof: net.Conn IS an io.ReadWriteCloser.
	var rwc io.ReadWriteCloser = conn
	var _ io.Reader = rwc
	var _ io.Writer = rwc

	sc := bufio.NewScanner(rwc) // consume the conn as a plain stream
	var lines []string
	for sc.Scan() && len(lines) < 2 {
		lines = append(lines, sc.Text())
	}
	fmt.Printf("[strm]  net.Conn as io.ReadWriteCloser, scanned lines: %v\n", lines)
}

// tlsSocket demonstrates a TLS socket (Bun `tls:` option) with a self-signed
// in-memory cert — crypto/tls wraps a net.Conn and is itself a net.Conn.
func tlsSocket() {
	// generate a self-signed cert entirely in memory
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	must(err)
	tmpl := x509.Certificate{
		SerialNumber: big.NewInt(1),
		Subject:      pkix.Name{CommonName: "localhost"},
		NotBefore:    time.Now().Add(-time.Hour),
		NotAfter:     time.Now().Add(time.Hour),
		DNSNames:     []string{"localhost"},
		IPAddresses:  []net.IP{net.IPv4(127, 0, 0, 1)},
	}
	der, err := x509.CreateCertificate(rand.Reader, &tmpl, &tmpl, &key.PublicKey, key)
	must(err)
	cert := tls.Certificate{Certificate: [][]byte{der}, PrivateKey: key}

	srvCfg := &tls.Config{Certificates: []tls.Certificate{cert}}
	ln, err := tls.Listen("tcp", "127.0.0.1:0", srvCfg)
	must(err)
	defer ln.Close()

	go func() {
		conn, err := ln.Accept()
		if err != nil {
			return
		}
		defer conn.Close()
		io.Copy(conn, conn) // TLS conn is still an io.ReadWriteCloser
	}()

	// client trusts our self-signed cert
	pool := x509.NewCertPool()
	c, _ := x509.ParseCertificate(der)
	pool.AddCert(c)
	conn, err := tls.Dial("tcp", ln.Addr().String(), &tls.Config{RootCAs: pool, ServerName: "localhost"})
	must(err)
	defer conn.Close()
	msg := "hello over tls\n"
	_, err = conn.Write([]byte(msg))
	must(err)
	got, err := bufio.NewReader(conn).ReadString('\n')
	must(err)
	fmt.Printf("[tls ]  listen %s  -> sent %q  got %q\n", ln.Addr().String(), msg, got)
}

func main() {
	fmt.Println("== cljg.socket feasibility round-trips (pure Go stdlib net) ==")
	tcpEcho()
	udpDatagram()
	unixSocket()
	streamShape()
	tlsSocket()
	fmt.Println("== all socket round-trips completed ==")
}
