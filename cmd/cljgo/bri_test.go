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

// TestMigrateCLI drives `cljgo migrate` end to end (app-framework task 2.3).
// Runs against the zero-install SQLite default (ADR 0057), so it is hermetic and
// cross-platform. Two independent apps keep the assertions robust against the
// second-granularity version scheme (two migrations minted in the same wall-clock
// second would share a version — a property of the timestamp naming, not tested here).
func TestMigrateCLI(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping binary build in -short mode")
	}
	bin := buildCljgo(t)

	// --- app 1: the apply flow, over a real generated migration ----------------
	work := t.TempDir()
	if out, err := runIn(work, bin, "new", "--template", "web", "demo"); err != nil {
		t.Fatalf("cljgo new: %v\n%s", err, out)
	}
	app := filepath.Join(work, "demo")
	if out, err := runIn(app, bin, "generate", "resource", "Note", "title:string", "body:text"); err != nil {
		t.Fatalf("cljgo generate resource: %v\n%s", err, out)
	}
	// status before applying: the create_notes migration is pending
	if out, err := runIn(app, bin, "migrate", "status"); err != nil || !strings.Contains(out, "pending (1)") {
		t.Fatalf("migrate status (pre): err=%v out=%q, want pending (1)", err, out)
	}
	// up applies exactly one new migration
	if out, err := runIn(app, bin, "migrate", "up"); err != nil || !strings.Contains(out, "applied 1 new") {
		t.Fatalf("migrate up: err=%v out=%q, want applied 1 new", err, out)
	}
	// idempotent: a second up applies nothing
	if out, err := runIn(app, bin, "migrate", "up"); err != nil || !strings.Contains(out, "applied 0 new") {
		t.Fatalf("migrate up (2nd): err=%v out=%q, want applied 0 new", err, out)
	}
	// status after: nothing pending
	if out, err := runIn(app, bin, "migrate", "status"); err != nil || !strings.Contains(out, "pending (0)") {
		t.Fatalf("migrate status (post): err=%v out=%q, want pending (0)", err, out)
	}

	// --- app 2: `migrate new` writes a stub that then shows pending -------------
	work2 := t.TempDir()
	if out, err := runIn(work2, bin, "new", "--template", "web", "demo2"); err != nil {
		t.Fatalf("cljgo new (2): %v\n%s", err, out)
	}
	app2 := filepath.Join(work2, "demo2")
	if out, err := runIn(app2, bin, "migrate", "new", "create widgets"); err != nil ||
		!strings.Contains(out, "create_widgets.sql") {
		t.Fatalf("migrate new: err=%v out=%q, want a create_widgets.sql path", err, out)
	}
	if out, err := runIn(app2, bin, "migrate", "status"); err != nil || !strings.Contains(out, "pending (1)") {
		t.Fatalf("migrate status (after new): err=%v out=%q, want pending (1)", err, out)
	}
}

// The one-person-framework promise, end to end through the REAL binary
// (ADR 0073, reconciled with bri.core.data ADR 0072): `cljgo new --template web`,
// then `cljgo generate resource Note …`, then `cljgo test` is GREEN — a
// working, authenticated, DB-backed CRUD scaffolded and passing its own
// suite against a fresh in-memory database, with zero hand-editing.
func TestGenerateResourceRunsGreen(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping binary build in -short mode")
	}
	bin := buildCljgo(t)
	work := t.TempDir()
	if out, err := runIn(work, bin, "new", "--template", "web", "demo"); err != nil {
		t.Fatalf("cljgo new: %v\n%s", err, out)
	}
	app := filepath.Join(work, "demo")
	if out, err := runIn(app, bin, "generate", "resource", "Note", "title:string", "body:text"); err != nil {
		t.Fatalf("cljgo generate resource: %v\n%s", err, out)
	}
	// The generated CRUD suite runs against a fresh in-memory bri.core.data.
	if out, err := runIn(app, bin, "test"); err != nil {
		t.Fatalf("cljgo test on the generated resource was not green: %v\n%s", err, out)
	}
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
// bri.core.data, ADR 0072) is REAL source too: every gate run runs its
// in-process suite through the built binary. It is the thing people copy
// to get a database-backed API, so a rot in bri.core.data — a renamed verb, a
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

// TestGenerateResourceSuite scaffolds a web app, generates a resource, and runs
// the generated `cljgo test` end to end — the green CI gate task 5.1 (ADR 0073)
// asked for once bri.core.data's API froze. TestGenerateResource (generate_test.go)
// validates the emitted source + splice; this proves the generated CRUD suite
// (db/query|one|insert!|exec! over bri.core.data) actually PASSES under the binary.
func TestGenerateResourceSuite(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping binary build in -short mode")
	}
	bin := buildCljgo(t)
	work := t.TempDir()

	if out, err := runIn(work, bin, "new", "--template", "web", "demo"); err != nil {
		t.Fatalf("cljgo new: %v\n%s", err, out)
	}
	app := filepath.Join(work, "demo")

	if out, err := runIn(app, bin, "generate", "resource", "Note", "title:string", "body:text"); err != nil {
		t.Fatalf("cljgo generate resource: %v\n%s", err, out)
	}
	if _, err := os.Stat(filepath.Join(app, "test", "app", "notes_test.cljg")); err != nil {
		t.Fatalf("generated resource test missing: %v", err)
	}
	// the generated resource's own suite must pass under `cljgo test`
	if out, err := runIn(app, bin, "test"); err != nil {
		t.Fatalf("cljgo test (generated resource suite): %v\n%s", err, out)
	}
}

// TestDeployMigrateArm proves the deploy story (app-framework task 2.5): after
// `cljgo generate resource`, the generated -main gains a `migrate` arm, so the
// COMPILED app binary runs `./app migrate` to apply pending migrations before
// serving (`./app migrate && ./app`). Runs against the zero-install SQLite
// default, hermetic and cross-platform.
func TestDeployMigrateArm(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping binary build in -short mode")
	}
	bin := buildCljgo(t)
	work := t.TempDir()
	if out, err := runIn(work, bin, "new", "--template", "web", "demo"); err != nil {
		t.Fatalf("cljgo new: %v\n%s", err, out)
	}
	app := filepath.Join(work, "demo")
	if out, err := runIn(app, bin, "generate", "resource", "Note", "title:string", "body:text"); err != nil {
		t.Fatalf("cljgo generate resource: %v\n%s", err, out)
	}
	// the migrate arm is spliced into -main
	if m := readFile(t, filepath.Join(app, "src", "app", "main.cljg")); !strings.Contains(m, `"migrate" (let [db (bri.core.data/connect`) {
		t.Fatalf("generated -main missing the migrate arm:\n%s", m)
	}

	// build the app to a binary and run `./app migrate`
	appBin := filepath.Join(app, "demoapp"+emit.ExeSuffix)
	build := exec.Command(bin, "build", "-o", appBin, filepath.Join("src", "app", "main.cljg"))
	build.Dir = app
	build.Env = append(os.Environ(), "CLJGO_SRC="+repoRoot(t), "CGO_ENABLED=0")
	if out, err := build.CombinedOutput(); err != nil {
		t.Fatalf("cljgo build (app binary): %v\n%s", err, out)
	}
	migrate := exec.Command(appBin, "migrate")
	migrate.Dir = app
	if out, err := migrate.CombinedOutput(); err != nil || !strings.Contains(string(out), "migrations applied") {
		t.Fatalf("./app migrate: err=%v out=%q, want \"migrations applied\"", err, out)
	}
	// and the migration is now applied (nothing pending)
	if out, err := runIn(app, bin, "migrate", "status"); err != nil || !strings.Contains(out, "pending (0)") {
		t.Fatalf("migrate status after ./app migrate: err=%v out=%q, want pending (0)", err, out)
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
		`(in-ns 'app.main) (defn home [_req] (bri.web.http/ok (bri.web.html/page [:h1 "redefined live"])))`)
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
