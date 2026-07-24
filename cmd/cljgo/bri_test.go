// bri_test.go — the T0 exit criterion, end to end through the REAL
// binary (openspec app-framework tasks 0.1/0.2; S20 VERDICT: "the
// terminal transcript below is the T0 exit criterion"):
//
//	cljgo new demo && cd demo && cljgo dev
//
// boots a styled page on a real port with an nREPL attached; the
// generated test passes via `cljgo test`; and a handler re-def over
// the nREPL WIRE changes the live response (the S15-style wire proof,
// through the shipped adapter instead of a spike bridge).
//
// It is ALSO the proof that templates/web — the real files `cljgo new
// --template web` generates from — compiles and runs: every gate run
// generates it, runs its test, boots it, and curls the page, so a
// template cannot rot without turning this red.
//
// TestNewTemplatesRun does the same for the other two built-ins (ADR
// 0047): `cljgo new` (lib, the default) and `--template cli` are
// generated, tested, and — for cli — compiled and EXECUTED, so all
// three shipped templates are run by CI. Fast guards:
// templates_test.go.
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

// repoRoot is the module root — the emitter needs it (CLJGO_SRC) to
// resolve the generated go.mod's replace when a build runs from a temp
// dir outside the repo.
func repoRoot(t *testing.T) string {
	t.Helper()
	abs, err := filepath.Abs(filepath.Join("..", "..")) // this package sits at cmd/cljgo
	if err != nil {
		t.Fatal(err)
	}
	return abs
}

// TestNewTemplatesRun is the anti-rot gate for the two non-web
// built-ins. `cljgo new` with no --template must hand a library author
// a LIBRARY (ADR 0047), and every shipped template's generated project
// must pass its own test — plus, for cli, actually compile and print
// what its README claims.
func TestNewTemplatesRun(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping binary build in -short mode")
	}
	bin := buildCljgo(t)

	// --- lib: the DEFAULT. `cljgo new demo`, no --template. -------------
	t.Run("lib", func(t *testing.T) {
		work := t.TempDir()
		if out, err := runIn(work, bin, "new", "demo"); err != nil {
			t.Fatalf("cljgo new: %v\n%s", err, out)
		}
		app := filepath.Join(work, "demo")
		for _, f := range []string{
			"src/demo/core.cljg", "test/demo/core_test.cljg",
			"build.cljgo", "README.md", ".gitignore",
		} {
			if _, err := os.Stat(filepath.Join(app, f)); err != nil {
				t.Fatalf("generated library missing %s: %v", f, err)
			}
		}
		// The layering itself: no server was handed to a library author.
		if _, err := os.Stat(filepath.Join(app, "conf.edn")); err == nil {
			t.Error("`cljgo new` generated conf.edn — the default is a library, not a web app")
		}
		if out, err := runIn(app, bin, "test"); err != nil {
			t.Fatalf("cljgo test: %v\n%s", err, out)
		}
		// A library declares no artifacts; `cljgo build` says so and does
		// not pretend to fail.
		if out, err := runIn(app, bin, "build"); err != nil {
			t.Fatalf("cljgo build in a library: %v\n%s", err, out)
		} else if !strings.Contains(out, "nothing to build") {
			t.Errorf("cljgo build in a library said: %q", out)
		}
	})

	// --- cli: generated, tested, COMPILED, and run ----------------------
	t.Run("cli", func(t *testing.T) {
		work := t.TempDir()
		if out, err := runIn(work, bin, "new", "--template", "cli", "demo"); err != nil {
			t.Fatalf("cljgo new --template cli: %v\n%s", err, out)
		}
		app := filepath.Join(work, "demo")
		if out, err := runIn(app, bin, "test"); err != nil {
			t.Fatalf("cljgo test: %v\n%s", err, out)
		}

		// The tool's whole pitch is the binary. Build it and run it.
		build := exec.Command(bin, "build")
		build.Dir = app
		build.Env = append(os.Environ(), "CLJGO_SRC="+repoRoot(t))
		if out, err := build.CombinedOutput(); err != nil {
			t.Fatalf("cljgo build: %v\n%s", err, out)
		}
		out, err := exec.Command(filepath.Join(app, "demo"+emit.ExeSuffix), "ada", "alan").CombinedOutput()
		if err != nil {
			t.Fatalf("running the built binary: %v\n%s", err, out)
		}
		if strings.TrimSpace(string(out)) != "Hello, ada, alan!" {
			t.Fatalf("the built tool printed %q — the cli template's own README is wrong", out)
		}
	})
}

// runIn runs bin with args in dir and returns the combined output.
func runIn(dir, bin string, args ...string) (string, error) {
	cmd := exec.Command(bin, args...)
	cmd.Dir = dir
	out, err := cmd.CombinedOutput()
	return string(out), err
}

// The shipped examples/web-api project (a JWT-secured JSON notes API) is
// REAL source, and it stays that way: every gate run compiles the binary
// and runs the example's own in-process suite through it. The example is
// the thing people copy to get a web API, so a rot in it — a renamed bri
// fn, a broken guard, a dropped reverse-route — turns this red instead of
// greeting the next reader.
func TestExampleWebApiSuite(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping binary build in -short mode")
	}
	bin := buildCljgo(t)
	dir, err := filepath.Abs(filepath.Join("..", "..", "examples", "web-api"))
	if err != nil {
		t.Fatal(err)
	}
	if out, err := runIn(dir, bin, "test"); err != nil {
		t.Fatalf("cljgo test (examples/web-api): %v\n%s", err, out)
	}
}

// The shipped examples/notes-db project (a persistent notes CRUD on
// bri.db, ADR 0072) is REAL source too: every gate run runs its
// in-process suite through the built binary. It is the thing people copy
// to get a database-backed API, so a rot in bri.db — a renamed verb, a
// broken migration, a lost snake→kebab mapping — turns this red. The
// dual-mode (interpreted vs compiled) proof lives in TestBriDBParity.
func TestExampleNotesDBSuite(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping binary build in -short mode")
	}
	bin := buildCljgo(t)
	dir, err := filepath.Abs(filepath.Join("..", "..", "examples", "notes-db"))
	if err != nil {
		t.Fatal(err)
	}
	if out, err := runIn(dir, bin, "test"); err != nil {
		t.Fatalf("cljgo test (examples/notes-db): %v\n%s", err, out)
	}
}

func TestBriNewDevTest(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping binary build in -short mode")
	}
	bin := buildCljgo(t)
	work := t.TempDir()

	// --- cljgo new --template web demo ----------------------------------
	newCmd := exec.Command(bin, "new", "--template", "web", "demo")
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
		`(in-ns 'app.main) (defn home [_req] (bri.http/ok (bri.html/page [:h1 "redefined live"])))`)
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
