// keel_test.go — the T0 exit criterion, end to end through the REAL
// binary (openspec app-framework tasks 0.1/0.2; S20 VERDICT: "the
// terminal transcript below is the T0 exit criterion"):
//
//	cljgo new demo && cd demo && cljgo dev
//
// boots a styled page on a real port with an nREPL attached; the
// generated test passes via `cljgo test`; and a handler re-def over
// the nREPL WIRE changes the live response (the S15-style wire proof,
// through the shipped adapter instead of a spike bridge).
package main

import (
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"
	"testing"
	"time"

	"github.com/muthuishere/cljgo/pkg/emit"
)

func buildCljgo(t *testing.T) string {
	t.Helper()
	bin := filepath.Join(t.TempDir(), "cljgo"+emit.ExeSuffix)
	build := exec.Command("go", "build", "-o", bin, "github.com/muthuishere/cljgo/cmd/cljgo")
	build.Dir = ".." // module root (this package sits at cmd/cljgo)
	if out, err := build.CombinedOutput(); err != nil {
		t.Fatalf("go build: %v\n%s", err, out)
	}
	return bin
}

func TestKeelNewDevTest(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping binary build in -short mode")
	}
	bin := buildCljgo(t)
	work := t.TempDir()

	// --- cljgo new demo -------------------------------------------------
	newCmd := exec.Command(bin, "new", "demo")
	newCmd.Dir = work
	if out, err := newCmd.CombinedOutput(); err != nil {
		t.Fatalf("cljgo new: %v\n%s", err, out)
	}
	app := filepath.Join(work, "demo")
	for _, f := range []string{
		"src/app/main.cljg", "conf.edn", "conf.schema.edn",
		"public/app.css", "test/app/main_test.cljg", "build.cljgo", ".gitignore",
	} {
		if _, err := os.Stat(filepath.Join(app, f)); err != nil {
			t.Fatalf("generated app missing %s: %v", f, err)
		}
	}

	// --- cljgo test (the generated test passes) ---------------------------
	testCmd := exec.Command(bin, "test")
	testCmd.Dir = app
	if out, err := testCmd.CombinedOutput(); err != nil {
		t.Fatalf("cljgo test: %v\n%s", err, out)
	}

	// --- cljgo dev: styled page + nREPL + LIVE re-def over the wire -------
	// A fixed free port keeps the curl target deterministic.
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	port := ln.Addr().(*net.TCPAddr).Port
	ln.Close()

	dev := exec.Command(bin, "dev")
	dev.Dir = app
	dev.Env = append(os.Environ(), fmt.Sprintf("APP_PORT=%d", port))
	var devOut strings.Builder
	dev.Stdout = &devOut
	dev.Stderr = &devOut
	if err := dev.Start(); err != nil {
		t.Fatalf("cljgo dev: %v", err)
	}
	defer func() {
		dev.Process.Signal(syscall.SIGTERM)
		done := make(chan struct{})
		go func() { dev.Wait(); close(done) }()
		select {
		case <-done:
		case <-time.After(15 * time.Second):
			dev.Process.Kill()
			<-done
		}
		if t.Failed() {
			t.Logf("cljgo dev output:\n%s", devOut.String())
		}
	}()

	get := func(path string) (int, string) {
		var lastErr error
		for i := 0; i < 100; i++ { // the interpreter boots in well under 10s
			resp, err := http.Get(fmt.Sprintf("http://127.0.0.1:%d%s", port, path))
			if err != nil {
				lastErr = err
				time.Sleep(100 * time.Millisecond)
				continue
			}
			defer resp.Body.Close()
			b, _ := io.ReadAll(resp.Body)
			return resp.StatusCode, string(b)
		}
		t.Fatalf("GET %s never came up: %v", path, lastErr)
		return 0, ""
	}

	if code, body := get("/"); code != 200 || !strings.Contains(body, "alive.") ||
		!strings.Contains(body, `href="/static/app.css"`) {
		t.Fatalf("landing page: %d %q", code, body)
	}
	if code, body := get("/static/app.css"); code != 200 || !strings.Contains(body, "--bg") {
		t.Fatalf("stylesheet: %d %q", code, body)
	}
	if code, body := get("/health"); code != 200 || body != `{"ok":true}` {
		t.Fatalf("health: %d %q", code, body)
	}

	// The wire proof: re-def app.main/home over the nREPL socket, next
	// request observes the new definition — no restart.
	portFile := filepath.Join(app, ".nrepl-port")
	var nreplPort string
	for i := 0; i < 100; i++ {
		if b, err := os.ReadFile(portFile); err == nil {
			nreplPort = strings.TrimSpace(string(b))
			break
		}
		time.Sleep(100 * time.Millisecond)
	}
	if nreplPort == "" {
		t.Fatal("no .nrepl-port written by cljgo dev")
	}
	nreplEval(t, nreplPort,
		`(in-ns 'app.main) (defn home [_req] (keel.http/ok (keel.html/page [:h1 "redefined live"])))`)
	if code, body := get("/"); code != 200 || !strings.Contains(body, "redefined live") {
		t.Fatalf("after nREPL re-def: %d %q — the live-var story is broken", code, body)
	}
}

// nreplEval sends one eval op over the bencode wire and waits for done
// (the minimal client shape of pkg/nrepl's own tests).
func nreplEval(t *testing.T, port, code string) {
	t.Helper()
	conn, err := net.DialTimeout("tcp", "127.0.0.1:"+port, 5*time.Second)
	if err != nil {
		t.Fatalf("nrepl dial: %v", err)
	}
	defer conn.Close()
	conn.SetDeadline(time.Now().Add(20 * time.Second))
	msg := fmt.Sprintf("d2:id1:14:code%d:%s2:op4:evale", len(code), code)
	if _, err := io.WriteString(conn, msg); err != nil {
		t.Fatalf("nrepl send: %v", err)
	}
	buf := make([]byte, 1<<16)
	var got strings.Builder
	for {
		n, err := conn.Read(buf)
		got.Write(buf[:n])
		if strings.Contains(got.String(), "done") {
			return
		}
		if err != nil {
			t.Fatalf("nrepl read: %v (got %q)", err, got.String())
		}
	}
}
