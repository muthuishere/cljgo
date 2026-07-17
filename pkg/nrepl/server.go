// Package nrepl is cljgo's nREPL server (ADR 0031): bencode over TCP,
// babashka's proven-minimal 13-op surface, one shared evaluator, one
// goroutine per session. Adapted from spike S15
// (spikes/s15-nrepl-minimal), which passed this wire session against a
// real nREPL 1.3.1 client.
//
// Model: one shared eval.Evaluator per server (the namespace/var world
// in pkg/lang is process-global anyway, exactly like a JVM nREPL
// server), one goroutine per nREPL session. Dynamic bindings in pkg/lang
// are goroutine-keyed, so a session goroutine pushes the shared
// repl.Session frame — *ns* *1 *2 *3 *e — plus *out* bound to a writer
// that streams nREPL "out" messages, and every eval/load-file/complete/
// lookup for that session executes ON that goroutine. Per-session
// namespaces, result history, and output streaming come for free; the
// spike's server-wide eval mutex is gone because the print family
// honors lang.VarOut (design/08 batch E).
package nrepl

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
	"github.com/muthuishere/cljgo/pkg/keel"
	"github.com/muthuishere/cljgo/pkg/lang"
	"github.com/muthuishere/cljgo/pkg/reader"
	"github.com/muthuishere/cljgo/pkg/repl"
	"github.com/muthuishere/cljgo/pkg/version"
)

// serverOps is what describe advertises: babashka.nrepl's proven-minimal
// surface plus interrupt (an honest stub — ADR 0031).
var serverOps = []string{
	"clone", "close", "describe", "eval", "load-file",
	"complete", "completions", "lookup", "info", "eldoc",
	"ls-sessions", "interrupt", "ns-list",
}

// Server is one nREPL server over one shared evaluator.
type Server struct {
	ev *eval.Evaluator

	mu       sync.Mutex
	sessions map[string]*session
}

// session is one nREPL session: a goroutine holding the binding frame,
// fed requests over reqs. out is the *out* writer bound in the frame.
type session struct {
	id   string
	rs   *repl.Session
	out  *sessionOut
	reqs chan request
	quit chan struct{}
	busy atomic.Bool
}

// request is one unit of session work. run executes on the session
// goroutine and RETURNS its final (done-status) reply instead of sending
// it: the session loop clears busy BEFORE sending, so any client that
// has read "done" is guaranteed a subsequent interrupt sees the session
// idle — the ordering editors rely on. Intermediate messages (values,
// out chunks, err/ex) are still sent from inside run.
type request struct {
	c   *conn
	run func() map[string]any
}

// NewServer boots a fresh evaluator and returns a server fronting it.
func NewServer() *Server {
	ev := eval.New()
	keel.Register(ev) // keel.* namespaces requireable, loaded lazily (ADR 0041)
	return &Server{ev: ev, sessions: map[string]*session{}}
}

// Serve accepts nREPL connections until the listener closes.
func (s *Server) Serve(ln net.Listener) {
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

func (s *Server) handleConn(nc net.Conn) {
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

// sessionOut streams the session's *out* as nREPL "out" messages. The
// target (conn + request message to echo id from) is set per request on
// the session goroutine; the mutex covers writers on other goroutines
// (futures convey the *out* binding, so they may write concurrently and
// after the eval that spawned them finished — same as real nREPL).
type sessionOut struct {
	sessID string

	mu  sync.Mutex
	c   *conn
	msg map[string]any
}

func (o *sessionOut) setTarget(c *conn, msg map[string]any) {
	o.mu.Lock()
	o.c, o.msg = c, msg
	o.mu.Unlock()
}

func (o *sessionOut) Write(p []byte) (int, error) {
	o.mu.Lock()
	c, msg := o.c, o.msg
	o.mu.Unlock()
	if c != nil {
		c.send(resp(msg, o.sessID, "out", string(p)))
	}
	return len(p), nil
}

// newSession spawns the session goroutine that owns the binding frame.
func (s *Server) newSession() *session {
	sess := &session{
		id:   newID(),
		rs:   repl.NewSession(s.ev),
		reqs: make(chan request, 16),
		quit: make(chan struct{}),
	}
	sess.out = &sessionOut{sessID: sess.id}
	s.mu.Lock()
	s.sessions[sess.id] = sess
	s.mu.Unlock()
	go func() {
		// The shared session frame (repl.Session, ADR 0031) plus *out*
		// bound to the out-message streamer. All work for this session runs
		// on this goroutine, so *ns* / *1 *2 *3 *e / *out* are per-session
		// (bindings are goroutine-keyed in pkg/lang).
		frame := sess.rs.Bindings().Assoc(lang.VarOut, sess.out).(lang.IPersistentMap)
		lang.PushThreadBindings(frame)
		defer lang.PopThreadBindings()
		for {
			select {
			case r := <-sess.reqs:
				sess.busy.Store(true)
				final := r.run()
				// Clear busy BEFORE the final done-status reply goes on the
				// wire: a client that has read "done" must be guaranteed a
				// subsequent interrupt sees session-idle (the eval→interrupt
				// ordering editors rely on; caught by CI on a slow runner).
				sess.busy.Store(false)
				if final != nil {
					r.c.send(final)
				}
			case <-sess.quit:
				return
			}
		}
	}()
	return sess
}

func (s *Server) session(id string) *session {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.sessions[id]
}

// sessionFor resolves the message's session, creating one for
// sessionless messages (the spec allows them).
func (s *Server) sessionFor(msg map[string]any) *session {
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

func (s *Server) dispatch(c *conn, msg map[string]any) {
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
		// Honest stub (ADR 0031): the tree-walk evaluator has no
		// cancellation checkpoints, so a running eval CANNOT be aborted.
		// Idle sessions answer session-idle per spec; busy ones admit the
		// eval keeps running. babashka shipped without interrupt for years
		// and Calva/CIDER cope.
		sess := s.session(str(msg, "session"))
		switch {
		case sess == nil:
			c.send(resp(msg, "", "status", []string{"done", "interrupt-id-mismatch"}))
		case !sess.busy.Load():
			c.send(resp(msg, sess.id, "status", []string{"done", "session-idle"}))
		default:
			c.send(resp(msg, sess.id, "status", []string{"done"},
				"err", "interrupt: not supported (eval continues); see ADR 0031\n"))
		}
	case "eval":
		sess := s.sessionFor(msg)
		sess.reqs <- request{c, func() map[string]any {
			return s.doEval(c, sess, msg, str(msg, "code"), "NO_SOURCE_FILE", true)
		}}
	case "load-file":
		sess := s.sessionFor(msg)
		file := str(msg, "file")
		name := str(msg, "file-path", "file-name")
		if name == "" {
			name = "NO_SOURCE_FILE"
		}
		sess.reqs <- request{c, func() map[string]any {
			return s.doEval(c, sess, msg, file, name, false)
		}}
	case "complete", "completions":
		sess := s.sessionFor(msg)
		sess.reqs <- request{c, func() map[string]any { return s.doComplete(sess, msg) }}
	case "lookup", "info", "eldoc":
		sess := s.sessionFor(msg)
		sess.reqs <- request{c, func() map[string]any { return s.doLookup(sess, msg, op) }}
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

// doEval runs on the session goroutine (under its binding frame) and
// returns the final done-status reply (sent by the session loop after
// the busy flag clears). perForm: eval sends one value message per
// top-level form (like nREPL); load-file sends only the last. Printed
// output flows through the session's *out* binding (sessionOut) — no
// global state.
func (s *Server) doEval(c *conn, sess *session, msg map[string]any, code, filename string, perForm bool) map[string]any {
	// Point this session's *out* stream at the requesting message so the
	// out chunks echo its id. Left set after the eval: a future spawned by
	// this eval keeps streaming here (bindings convey), as real nREPL does.
	sess.out.setTarget(c, msg)

	// Optional ns targeting: eval in the requested namespace when it
	// exists (Calva/CIDER send "ns" with most evals).
	if nsName := str(msg, "ns"); nsName != "" {
		if ns := lang.FindNamespace(lang.NewSymbol(nsName)); ns != nil {
			lang.VarCurrentNS.Set(ns)
		}
	}

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
			return s.evalErrorReply(c, sess, msg, err)
		}
		res, err := s.evalOne(form)
		if err != nil {
			sess.rs.RecordError(err)
			return s.evalErrorReply(c, sess, msg, err)
		}
		evaluated = true
		last = res
		sess.rs.RecordResult(res)
		if perForm {
			c.send(resp(msg, sess.id, "value", printString(res), "ns", s.ev.CurrentNS().Name().Name()))
		}
	}
	if !perForm && evaluated {
		c.send(resp(msg, sess.id, "value", printString(last), "ns", s.ev.CurrentNS().Name().Name()))
	}
	return resp(msg, sess.id, "status", []string{"done"})
}

// evalOne guards the evaluator seam: EvalForm recovers evaluator panics
// itself, but printing/binding seams can still panic (same belt-and-
// braces as Driver.evalAndPrint).
func (s *Server) evalOne(form any) (res any, err error) {
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

// evalErrorReply sends the err/ex messages and returns the final done
// reply for the session loop to send after clearing busy.
func (s *Server) evalErrorReply(c *conn, sess *session, msg map[string]any, err error) map[string]any {
	c.send(resp(msg, sess.id, "err", err.Error()+"\n"))
	c.send(resp(msg, sess.id, "ex", fmt.Sprintf("%T", err), "status", []string{"eval-error"}))
	return resp(msg, sess.id, "status", []string{"done"})
}

// doComplete: prefix completion over the current namespace's mappings
// (which include core refers) plus namespace names. Runs on the session
// goroutine so "current namespace" is the session's; returns its single
// (done-status) reply for the session loop to send.
func (s *Server) doComplete(sess *session, msg map[string]any) map[string]any {
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
	return resp(msg, sess.id, "completions", cands, "status", []string{"done"})
}

// doLookup answers lookup/info/eldoc from var metadata (:doc :arglists
// :file :line — the analyzer already stamps position + docstrings).
// Returns its single (done-status) reply for the session loop to send.
func (s *Server) doLookup(sess *session, msg map[string]any, op string) map[string]any {
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
		return resp(msg, sess.id, "status", []string{"done", "no-info"})
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
	return reply
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
