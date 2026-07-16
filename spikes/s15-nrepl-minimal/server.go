// Prototype nREPL server fronting cljgo's pkg/eval — SPIKE code (ADR
// 0027), never merged, only informs ADR 0031.
//
// Model: one shared Evaluator (the namespace/var world is process-global
// in pkg/lang, exactly like a JVM nREPL server), one goroutine per nREPL
// session. Dynamic bindings in pkg/lang are goroutine-keyed
// (lang.PushThreadBindings uses goid), so a session goroutine pushes the
// same frame pkg/repl.Driver.Run pushes — *ns* *1 *2 *3 *e — and every
// eval/load-file/complete/lookup for that session executes ON that
// goroutine. That gives per-session namespaces and result history with
// zero changes to pkg/.
package main

import (
	"bufio"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"net"
	"sort"
	"strings"
	"sync"
	"sync/atomic"

	"github.com/muthuishere/cljgo/pkg/eval"
	"github.com/muthuishere/cljgo/pkg/lang"
	"github.com/muthuishere/cljgo/pkg/reader"
	"github.com/muthuishere/cljgo/pkg/version"
)

// serverOps is what describe advertises: babashka.nrepl's proven-minimal
// surface (clone close eval load-file complete/completions describe
// ls-sessions lookup/info eldoc ns-list) plus interrupt (stubbed — see
// VERDICT).
var serverOps = []string{
	"clone", "close", "describe", "eval", "load-file",
	"complete", "completions", "lookup", "info", "eldoc",
	"ls-sessions", "interrupt", "ns-list",
}

type server struct {
	ev             *eval.Evaluator
	v1, v2, v3, ve *lang.Var

	mu       sync.Mutex
	sessions map[string]*session

	// evalMu serializes evals server-wide so the eval.Out swap (see
	// captureOut) is race-free. A real pkg/nrepl must instead make
	// println honor *out* — recorded as a gap in VERDICT.md.
	evalMu sync.Mutex
}

type session struct {
	id   string
	reqs chan func()
	quit chan struct{}
	busy atomic.Bool
}

func newServer() *server {
	ev := eval.New()
	find := func(name string) *lang.Var {
		return lang.NSCore.FindInternedVar(lang.NewSymbol(name))
	}
	return &server{
		ev:       ev,
		v1:       find("*1"),
		v2:       find("*2"),
		v3:       find("*3"),
		ve:       find("*e"),
		sessions: map[string]*session{},
	}
}

// serve accepts nREPL connections until the listener closes.
func (s *server) serve(ln net.Listener) {
	for {
		c, err := ln.Accept()
		if err != nil {
			return
		}
		go s.handleConn(c)
	}
}

// conn wraps one client socket; writes are mutex-guarded because the
// connection goroutine (clone/describe/...) and session goroutines
// (eval results, out chunks) both respond on it.
type conn struct {
	net.Conn
	wmu sync.Mutex
}

func (c *conn) send(msg map[string]any) {
	c.wmu.Lock()
	defer c.wmu.Unlock()
	_ = bencodeWrite(c.Conn, msg)
}

func (s *server) handleConn(nc net.Conn) {
	c := &conn{Conn: nc}
	defer c.Close()
	r := bufio.NewReader(nc)
	for {
		v, err := bencodeRead(r)
		if err != nil {
			return // EOF or garbage: drop the connection
		}
		msg, ok := v.(map[string]any)
		if !ok {
			continue
		}
		s.dispatch(c, msg)
	}
}

// resp starts a reply that echoes id and session, per the nREPL spec.
func resp(msg map[string]any, sessionID string, kvs ...any) map[string]any {
	m := map[string]any{}
	if id, ok := msg["id"].(string); ok {
		m["id"] = id
	}
	if sessionID != "" {
		m["session"] = sessionID
	} else if sid, ok := msg["session"].(string); ok {
		m["session"] = sid
	}
	for i := 0; i+1 < len(kvs); i += 2 {
		m[kvs[i].(string)] = kvs[i+1]
	}
	return m
}

func newID() string {
	var b [8]byte
	_, _ = rand.Read(b[:])
	return hex.EncodeToString(b[:])
}

// newSession spawns the session goroutine that owns the binding frame.
func (s *server) newSession() *session {
	sess := &session{id: newID(), reqs: make(chan func(), 16), quit: make(chan struct{})}
	s.mu.Lock()
	s.sessions[sess.id] = sess
	s.mu.Unlock()
	go func() {
		// The session frame — identical to pkg/repl.Driver.Run's. All work
		// for this session runs on this goroutine, so *ns* / *1 *2 *3 *e
		// are per-session (bindings are goroutine-keyed in pkg/lang).
		lang.PushThreadBindings(lang.NewMap(
			lang.VarCurrentNS, s.ev.CurrentNS(),
			s.v1, nil, s.v2, nil, s.v3, nil, s.ve, nil,
		))
		defer lang.PopThreadBindings()
		for {
			select {
			case f := <-sess.reqs:
				sess.busy.Store(true)
				f()
				sess.busy.Store(false)
			case <-sess.quit:
				return
			}
		}
	}()
	return sess
}

func (s *server) session(id string) *session {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.sessions[id]
}

// sessionFor resolves the message's session, creating one for
// sessionless messages (the spec allows them).
func (s *server) sessionFor(msg map[string]any) *session {
	if id, ok := msg["session"].(string); ok {
		if sess := s.session(id); sess != nil {
			return sess
		}
	}
	return s.newSession()
}

func str(msg map[string]any, keys ...string) string {
	for _, k := range keys {
		if v, ok := msg[k].(string); ok {
			return v
		}
	}
	return ""
}

func (s *server) dispatch(c *conn, msg map[string]any) {
	op := str(msg, "op")
	switch op {
	case "clone":
		sess := s.newSession()
		c.send(resp(msg, "", "new-session", sess.id, "status", []string{"done"}))
	case "close":
		id := str(msg, "session")
		s.mu.Lock()
		sess := s.sessions[id]
		delete(s.sessions, id)
		s.mu.Unlock()
		if sess != nil {
			close(sess.quit)
		}
		c.send(resp(msg, id, "status", []string{"done", "session-closed"}))
	case "describe":
		ops := map[string]any{}
		for _, o := range serverOps {
			ops[o] = map[string]any{}
		}
		c.send(resp(msg, str(msg, "session"),
			"ops", ops,
			"versions", map[string]any{
				"cljgo":   map[string]any{"version-string": version.Version},
				"clojure": map[string]any{"version-string": version.ClojureVersion},
				"nrepl":   map[string]any{"version-string": "1.0", "major": int64(1), "minor": int64(0)},
			},
			"aux", map[string]any{},
			"status", []string{"done"}))
	case "ls-sessions":
		s.mu.Lock()
		ids := make([]string, 0, len(s.sessions))
		for id := range s.sessions {
			ids = append(ids, id)
		}
		s.mu.Unlock()
		sort.Strings(ids)
		c.send(resp(msg, str(msg, "session"), "sessions", ids, "status", []string{"done"}))
	case "interrupt":
		// Honest stub: the tree-walk evaluator has no cancellation
		// checkpoints, so a running eval CANNOT be aborted (Driver.Interrupt
		// only discards pending input). Idle sessions answer session-idle
		// per spec; busy ones admit the eval keeps running. babashka
		// shipped without interrupt and Calva/CIDER cope.
		sess := s.session(str(msg, "session"))
		switch {
		case sess == nil:
			c.send(resp(msg, "", "status", []string{"done", "interrupt-id-mismatch"}))
		case !sess.busy.Load():
			c.send(resp(msg, sess.id, "status", []string{"done", "session-idle"}))
		default:
			c.send(resp(msg, sess.id, "status", []string{"done"},
				"err", "interrupt: not supported (eval continues); see spike S15 VERDICT\n"))
		}
	case "eval":
		sess := s.sessionFor(msg)
		sess.reqs <- func() { s.doEval(c, sess, msg, str(msg, "code"), "NO_SOURCE_FILE", true) }
	case "load-file":
		sess := s.sessionFor(msg)
		file := str(msg, "file")
		name := str(msg, "file-path", "file-name")
		if name == "" {
			name = "NO_SOURCE_FILE"
		}
		sess.reqs <- func() { s.doEval(c, sess, msg, file, name, false) }
	case "complete", "completions":
		sess := s.sessionFor(msg)
		sess.reqs <- func() { s.doComplete(c, sess, msg) }
	case "lookup", "info", "eldoc":
		sess := s.sessionFor(msg)
		sess.reqs <- func() { s.doLookup(c, sess, msg, op) }
	case "ns-list":
		names := []string{}
		for seq := lang.AllNamespaces(); seq != nil; seq = seq.Next() {
			if ns, ok := seq.First().(*lang.Namespace); ok {
				names = append(names, ns.Name().Name())
			}
		}
		sort.Strings(names)
		c.send(resp(msg, str(msg, "session"), "ns-list", names, "status", []string{"done"}))
	default:
		c.send(resp(msg, str(msg, "session"), "status", []string{"done", "error", "unknown-op"}))
	}
}

// outForwarder streams printed output as nREPL "out" messages.
type outForwarder struct {
	c    *conn
	msg  map[string]any
	sess *session
}

func (o *outForwarder) Write(p []byte) (int, error) {
	o.c.send(resp(o.msg, o.sess.id, "out", string(p)))
	return len(p), nil
}

// doEval runs on the session goroutine (under its binding frame).
// perForm: eval sends one value message per top-level form (like nREPL);
// load-file sends only the last.
func (s *server) doEval(c *conn, sess *session, msg map[string]any, code, filename string, perForm bool) {
	// Optional ns targeting: eval in the requested namespace when it
	// exists (Calva/CIDER send "ns" with most evals).
	if nsName := str(msg, "ns"); nsName != "" {
		if ns := lang.FindNamespace(lang.NewSymbol(nsName)); ns != nil {
			lang.VarCurrentNS.Set(ns)
		}
	}

	// GAP (for ADR 0031): println writes to the package-global eval.Out,
	// not the *out* dynamic var, so per-session capture needs a global
	// swap — serialized server-wide here. pkg/nrepl needs println (and
	// friends) to honor *out*.
	s.evalMu.Lock()
	saved := eval.Out
	eval.Out = &outForwarder{c: c, msg: msg, sess: sess}
	defer func() {
		eval.Out = saved
		s.evalMu.Unlock()
	}()

	rd := reader.New(strings.NewReader(code), reader.WithFilename(filename),
		reader.WithResolver(s.ev.ReaderResolver()))
	var last any
	evaluated := false
	for {
		form, err := rd.ReadOne()
		if errors.Is(err, reader.ErrEOF) {
			break
		}
		if err != nil {
			s.sendEvalError(c, sess, msg, err)
			return
		}
		res, err := s.evalOne(form)
		if err != nil {
			s.ve.Set(err)
			s.sendEvalError(c, sess, msg, err)
			return
		}
		evaluated = true
		last = res
		s.v3.Set(s.v2.Deref())
		s.v2.Set(s.v1.Deref())
		s.v1.Set(res)
		if perForm {
			c.send(resp(msg, sess.id, "value", printString(res), "ns", s.ev.CurrentNS().Name().Name()))
		}
	}
	if !perForm && evaluated {
		c.send(resp(msg, sess.id, "value", printString(last), "ns", s.ev.CurrentNS().Name().Name()))
	}
	c.send(resp(msg, sess.id, "status", []string{"done"}))
}

// evalOne guards the evaluator seam: EvalForm recovers evaluator panics
// itself, but printing/binding seams can still panic (same belt-and-
// braces as Driver.evalAndPrint).
func (s *server) evalOne(form any) (res any, err error) {
	defer func() {
		if r := recover(); r != nil {
			e, ok := r.(error)
			if !ok {
				e = fmt.Errorf("%v", r)
			}
			err = e
		}
	}()
	return s.ev.EvalForm(form)
}

func printString(v any) (out string) {
	defer func() {
		if r := recover(); r != nil {
			out = fmt.Sprintf("#object[unprintable %v]", r)
		}
	}()
	return lang.PrintString(v)
}

func (s *server) sendEvalError(c *conn, sess *session, msg map[string]any, err error) {
	c.send(resp(msg, sess.id, "err", err.Error()+"\n"))
	c.send(resp(msg, sess.id, "ex", fmt.Sprintf("%T", err), "status", []string{"eval-error"}))
	c.send(resp(msg, sess.id, "status", []string{"done"}))
}

// doComplete: prefix completion over the current namespace's mappings
// (which include core refers) plus namespace names. Runs on the session
// goroutine so "current namespace" is the session's.
func (s *server) doComplete(c *conn, sess *session, msg map[string]any) {
	prefix := str(msg, "prefix", "symbol")
	curNS := s.ev.CurrentNS()
	if nsName := str(msg, "ns"); nsName != "" {
		if ns := lang.FindNamespace(lang.NewSymbol(nsName)); ns != nil {
			curNS = ns
		}
	}
	var cands []any
	add := func(name, typ string) {
		if strings.HasPrefix(name, prefix) {
			cands = append(cands, map[string]any{"candidate": name, "type": typ})
		}
	}
	for seq := lang.Seq(curNS.Mappings()); seq != nil; seq = seq.Next() {
		e := seq.First().(lang.IMapEntry)
		sym, ok := e.Key().(*lang.Symbol)
		if !ok {
			continue
		}
		typ := "var"
		if v, ok := e.Val().(*lang.Var); ok && v.IsMacro() {
			typ = "macro"
		}
		add(sym.Name(), typ)
	}
	for seq := lang.AllNamespaces(); seq != nil; seq = seq.Next() {
		if ns, ok := seq.First().(*lang.Namespace); ok {
			add(ns.Name().Name(), "namespace")
		}
	}
	sort.Slice(cands, func(i, j int) bool {
		return cands[i].(map[string]any)["candidate"].(string) < cands[j].(map[string]any)["candidate"].(string)
	})
	c.send(resp(msg, sess.id, "completions", cands, "status", []string{"done"}))
}

// doLookup answers lookup/info/eldoc from var metadata (:doc :arglists
// :file :line — the analyzer already stamps these).
func (s *server) doLookup(c *conn, sess *session, msg map[string]any, op string) {
	symName := str(msg, "sym", "symbol")
	curNS := s.ev.CurrentNS()
	if nsName := str(msg, "ns"); nsName != "" {
		if ns := lang.FindNamespace(lang.NewSymbol(nsName)); ns != nil {
			curNS = ns
		}
	}
	var v *lang.Var
	if symName != "" {
		sym := lang.NewSymbol(symName)
		if sym.Namespace() != "" {
			if ns := lang.FindNamespace(lang.NewSymbol(sym.Namespace())); ns != nil {
				v = ns.FindInternedVar(lang.NewSymbol(sym.Name()))
			}
		} else if m, ok := curNS.GetMapping(sym).(*lang.Var); ok {
			v = m
		}
	}
	if v == nil {
		c.send(resp(msg, sess.id, "status", []string{"done", "no-info"}))
		return
	}
	meta := v.Meta()
	metaStr := func(k string) string {
		if meta == nil {
			return ""
		}
		if s, ok := meta.ValAt(lang.NewKeyword(k)).(string); ok {
			return s
		}
		return ""
	}
	info := map[string]any{
		"name": v.Symbol().Name(),
		"ns":   v.Namespace().Name().Name(),
	}
	if doc := metaStr("doc"); doc != "" {
		info["doc"] = doc
		info["docstring"] = doc
	}
	if file := metaStr("file"); file != "" {
		info["file"] = file
	}
	var arglists any
	if meta != nil {
		arglists = meta.ValAt(lang.NewKeyword("arglists"))
	}
	if arglists != nil {
		info["arglists-str"] = printString(arglists)
	}
	if line, ok := metaAsInt(meta, "line"); ok {
		info["line"] = line
	}
	reply := resp(msg, sess.id, "status", []string{"done"})
	if op == "eldoc" {
		var eld []any
		for seq := lang.Seq(arglists); seq != nil; seq = seq.Next() {
			var one []any
			for as := lang.Seq(seq.First()); as != nil; as = as.Next() {
				one = append(one, printString(as.First()))
			}
			if one == nil {
				one = []any{}
			}
			eld = append(eld, one)
		}
		if eld == nil {
			eld = []any{}
		}
		reply["eldoc"] = eld
		reply["name"] = info["name"]
		reply["ns"] = info["ns"]
		reply["type"] = "function"
		if d, ok := info["docstring"]; ok {
			reply["docstring"] = d
		}
	} else {
		// info puts fields at the top level; lookup nests under "info".
		// Send both shapes — clients read what they know.
		for k, val := range info {
			reply[k] = val
		}
		reply["info"] = info
	}
	c.send(reply)
}

func metaAsInt(m lang.IPersistentMap, k string) (int64, bool) {
	if m == nil {
		return 0, false
	}
	switch n := m.ValAt(lang.NewKeyword(k)).(type) {
	case int:
		return int64(n), true
	case int64:
		return n, true
	}
	return 0, false
}
