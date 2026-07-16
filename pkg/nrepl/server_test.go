// The scripted wire-session test of ADR 0031 (ported from spike S15's
// exit criterion): a raw-bencode nREPL client over a real TCP socket
// driving clone → describe → eval → out-streaming → *1 → error shape →
// load-file → interrupt → lookup → complete → session isolation →
// ls-sessions → close. Deterministic: ephemeral ports, no sleeps —
// everything syncs on socket reads.
package nrepl

import (
	"bufio"
	"net"
	"strings"
	"testing"
	"time"
)

// client is the scripted raw-bencode nREPL client.
type client struct {
	t *testing.T
	c net.Conn
	r *bufio.Reader
}

func dialServer(t *testing.T) *client {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { ln.Close() })
	go NewServer().Serve(ln)
	c, err := net.Dial("tcp", ln.Addr().String())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { c.Close() })
	return &client{t: t, c: c, r: bufio.NewReader(c)}
}

func (cl *client) send(msg map[string]any) {
	cl.t.Helper()
	if err := bencodeWrite(cl.c, msg); err != nil {
		cl.t.Fatal(err)
	}
}

func (cl *client) recv() map[string]any {
	cl.t.Helper()
	_ = cl.c.SetReadDeadline(time.Now().Add(10 * time.Second))
	v, err := bencodeRead(cl.r)
	if err != nil {
		cl.t.Fatalf("recv: %v", err)
	}
	return v.(map[string]any)
}

// collect reads responses until one carries a "done" status, returning
// all of them (values, out chunks, errs, the final status message).
func (cl *client) collect() []map[string]any {
	cl.t.Helper()
	var msgs []map[string]any
	for {
		m := cl.recv()
		msgs = append(msgs, m)
		for _, st := range statuses(m) {
			if st == "done" {
				return msgs
			}
		}
		if len(msgs) > 64 {
			cl.t.Fatalf("no done status after %d messages", len(msgs))
		}
	}
}

func statuses(m map[string]any) []string {
	var out []string
	if l, ok := m["status"].([]any); ok {
		for _, s := range l {
			if str, ok := s.(string); ok {
				out = append(out, str)
			}
		}
	}
	return out
}

func hasStatus(msgs []map[string]any, want string) bool {
	for _, m := range msgs {
		for _, st := range statuses(m) {
			if st == want {
				return true
			}
		}
	}
	return false
}

func firstField(msgs []map[string]any, key string) (any, bool) {
	for _, m := range msgs {
		if v, ok := m[key]; ok {
			return v, true
		}
	}
	return nil, false
}

func firstString(msgs []map[string]any, key string) string {
	v, _ := firstField(msgs, key)
	s, _ := v.(string)
	return s
}

// lastString: eval sends one value message per top-level form; the
// final result is the last one.
func lastString(msgs []map[string]any, key string) string {
	out := ""
	for _, m := range msgs {
		if s, ok := m[key].(string); ok {
			out = s
		}
	}
	return out
}

// TestScriptedSession is the wire-level session of ADR 0031.
func TestScriptedSession(t *testing.T) {
	cl := dialServer(t)

	// -- clone: get a session -------------------------------------------
	cl.send(map[string]any{"op": "clone", "id": "1"})
	msgs := cl.collect()
	session := firstString(msgs, "new-session")
	if session == "" {
		t.Fatalf("clone: no new-session in %v", msgs)
	}

	// -- describe: ops + versions ---------------------------------------
	cl.send(map[string]any{"op": "describe", "id": "2", "session": session})
	msgs = cl.collect()
	opsAny, ok := firstField(msgs, "ops")
	if !ok {
		t.Fatalf("describe: no ops in %v", msgs)
	}
	ops := opsAny.(map[string]any)
	for _, need := range serverOps {
		if _, ok := ops[need]; !ok {
			t.Errorf("describe: op %q not advertised", need)
		}
	}
	if _, ok := firstField(msgs, "versions"); !ok {
		t.Error("describe: no versions")
	}

	// -- eval "(+ 1 2)" ---------------------------------------------------
	cl.send(map[string]any{"op": "eval", "id": "3", "session": session, "code": "(+ 1 2)"})
	msgs = cl.collect()
	if v := firstString(msgs, "value"); v != "3" {
		t.Fatalf("eval (+ 1 2): value = %q, want \"3\" (msgs %v)", v, msgs)
	}
	if ns := firstString(msgs, "ns"); ns != "user" {
		t.Errorf("eval: ns = %q, want \"user\"", ns)
	}

	// -- eval with printed output: out message precedes value ------------
	// (streams through the session's *out* binding — lang.VarOut, batch E)
	cl.send(map[string]any{"op": "eval", "id": "4", "session": session,
		"code": `(println "hello nrepl") :ok`})
	msgs = cl.collect()
	if out := firstString(msgs, "out"); !strings.Contains(out, "hello nrepl") {
		t.Errorf("eval println: out = %q, want it to contain \"hello nrepl\"", out)
	}
	if v := lastString(msgs, "value"); v != ":ok" {
		t.Errorf("eval println: final value = %q, want \":ok\"", v)
	}

	// -- *1 works (session result history) --------------------------------
	cl.send(map[string]any{"op": "eval", "id": "5", "session": session, "code": "(+ 40 2)"})
	cl.collect()
	cl.send(map[string]any{"op": "eval", "id": "6", "session": session, "code": "*1"})
	msgs = cl.collect()
	if v := firstString(msgs, "value"); v != "42" {
		t.Errorf("*1 = %q, want \"42\"", v)
	}

	// -- eval error shape --------------------------------------------------
	cl.send(map[string]any{"op": "eval", "id": "7", "session": session, "code": "(unresolvable-xyz)"})
	msgs = cl.collect()
	if !hasStatus(msgs, "eval-error") {
		t.Errorf("eval error: no eval-error status in %v", msgs)
	}
	if firstString(msgs, "err") == "" {
		t.Errorf("eval error: no err message in %v", msgs)
	}
	// *e holds the error in this session
	cl.send(map[string]any{"op": "eval", "id": "7e", "session": session, "code": "(some? *e)"})
	msgs = cl.collect()
	if v := firstString(msgs, "value"); v != "true" {
		t.Errorf("*e after eval error = %q, want \"true\"", v)
	}

	// -- doc macro (ADR 0031 ride-along): referred into user ----------------
	// (runs while the session is still in user — doc, like on the JVM,
	// is only referred into user, not into every ns)
	cl.send(map[string]any{"op": "eval", "id": "7d", "session": session,
		"code": `(def answer "The answer to everything." 42) (with-out-str (doc answer))`})
	msgs = cl.collect()
	want := `"-------------------------\nuser/answer\n  The answer to everything.\n"`
	if v := lastString(msgs, "value"); v != want {
		t.Errorf("(doc answer) captured = %q, want %q", v, want)
	}

	// -- load-file: defines vars later evals see ---------------------------
	cl.send(map[string]any{"op": "load-file", "id": "8", "session": session,
		"file":      "(ns nrepl.t) (defn twice [x] (* 2 x)) (def loaded :yes)",
		"file-name": "t.clj", "file-path": "nrepl/t.clj"})
	msgs = cl.collect()
	if !hasStatus(msgs, "done") {
		t.Fatalf("load-file: no done in %v", msgs)
	}
	cl.send(map[string]any{"op": "eval", "id": "9", "session": session,
		"code": "(nrepl.t/twice 21)"})
	msgs = cl.collect()
	if v := firstString(msgs, "value"); v != "42" {
		t.Fatalf("load-file: (nrepl.t/twice 21) = %q, want \"42\" (msgs %v)", v, msgs)
	}

	// -- interrupt: honest session-idle on an idle session -----------------
	cl.send(map[string]any{"op": "interrupt", "id": "10", "session": session})
	msgs = cl.collect()
	if !hasStatus(msgs, "session-idle") {
		t.Errorf("interrupt idle: want session-idle status, got %v", msgs)
	}

	// -- lookup: info for a core var ----------------------------------------
	cl.send(map[string]any{"op": "lookup", "id": "11", "session": session, "sym": "map"})
	msgs = cl.collect()
	if name := firstString(msgs, "name"); name != "map" {
		t.Errorf("lookup map: name = %q, msgs %v", name, msgs)
	}

	// -- complete -----------------------------------------------------------
	cl.send(map[string]any{"op": "complete", "id": "12", "session": session, "prefix": "ma"})
	msgs = cl.collect()
	compsAny, _ := firstField(msgs, "completions")
	comps, _ := compsAny.([]any)
	found := false
	for _, c := range comps {
		if m, ok := c.(map[string]any); ok && m["candidate"] == "map" {
			found = true
		}
	}
	if !found {
		t.Errorf("complete \"ma\": no candidate \"map\" in %d candidates", len(comps))
	}

	// -- session isolation: a second session has its own *ns* and *1 -------
	cl.send(map[string]any{"op": "clone", "id": "13"})
	session2 := firstString(cl.collect(), "new-session")
	cl.send(map[string]any{"op": "eval", "id": "14", "session": session2, "code": "*1"})
	msgs = cl.collect()
	if v := firstString(msgs, "value"); v != "nil" {
		t.Errorf("session isolation: fresh session *1 = %q, want \"nil\"", v)
	}
	cl.send(map[string]any{"op": "eval", "id": "15", "session": session2, "code": "(str *ns*)"})
	msgs = cl.collect()
	if v := firstString(msgs, "value"); v != `"user"` {
		t.Errorf("session isolation: fresh session ns = %q, want %q", v, `"user"`)
	}

	// -- ls-sessions ---------------------------------------------------------
	cl.send(map[string]any{"op": "ls-sessions", "id": "16"})
	msgs = cl.collect()
	sessAny, _ := firstField(msgs, "sessions")
	sessList, _ := sessAny.([]any)
	if len(sessList) < 2 {
		t.Errorf("ls-sessions: %d sessions, want >= 2", len(sessList))
	}

	// -- close ----------------------------------------------------------------
	cl.send(map[string]any{"op": "close", "id": "17", "session": session2})
	msgs = cl.collect()
	if !hasStatus(msgs, "session-closed") {
		t.Errorf("close: no session-closed status in %v", msgs)
	}
}

// TestOutIsolation: two sessions printing route their output to their
// own out streams — the *out* binding is per session goroutine, so no
// server-wide lock and no cross-talk.
func TestOutIsolation(t *testing.T) {
	cl := dialServer(t)

	cl.send(map[string]any{"op": "clone", "id": "a1"})
	sa := firstString(cl.collect(), "new-session")
	cl.send(map[string]any{"op": "clone", "id": "b1"})
	sb := firstString(cl.collect(), "new-session")

	cl.send(map[string]any{"op": "eval", "id": "a2", "session": sa, "code": `(println "from-a") :a`})
	msgsA := cl.collect()
	cl.send(map[string]any{"op": "eval", "id": "b2", "session": sb, "code": `(println "from-b") :b`})
	msgsB := cl.collect()

	for _, m := range msgsA {
		if s, ok := m["session"].(string); ok && s != sa {
			t.Errorf("session A reply carries session %q, want %q", s, sa)
		}
	}
	if out := firstString(msgsA, "out"); !strings.Contains(out, "from-a") {
		t.Errorf("session A out = %q, want it to contain \"from-a\"", out)
	}
	if out := firstString(msgsB, "out"); !strings.Contains(out, "from-b") {
		t.Errorf("session B out = %q, want it to contain \"from-b\"", out)
	}
}
