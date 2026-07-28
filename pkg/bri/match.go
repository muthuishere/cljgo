// match.go — the Go host half of clojure.core.match (ADR 0097): the
// Maranget decision-tree pattern-match COMPILER, exposed as one private
// macroexpand-time primitive, -match-compile, interned into
// clojure.core.match. The thin macro surface (match/matchv/matchm/match-let)
// lives in core/match.cljg and calls this to turn a clause pattern-matrix
// into an optimized nested test/dispatch form.
//
// Why a Go primitive and not pure Clojure (MANDATE A, performance is never
// compromised): the compiler is algorithm-heavy — column selection plus the
// specialization/default matrix decomposition of Luc Maranget, "Compiling
// Pattern Matching to Good Decision Trees" (ML'08). The pure-Clojure LINEAR
// compiler (spike s54) was rejected because it drops that optimization and
// scans clause-by-clause. This builds a real decision tree: each occurrence's
// column test is SHARED across every clause that reaches it, so a wide match
// dispatches in tree-depth tests, not clause-count.
//
// What ships in a compiled binary: NOTHING from here. -match-compile runs at
// macroexpand time only; its OUTPUT is plain clojure.core (let/if/=/nth/get/
// count/vector?/subvec/throw), so an AOT binary that uses `match` links zero
// match runtime, and a binary that never requires clojure.core.match pays zero
// bytes (MANDATE B — lazy + opt-in, the cljg.io model).
//
// clojure.core.match rides the same name-generic embedded-namespace registry
// as cljg.io / cljg.os (the pkg/bri package name is a legacy of bri being the
// first tenant — ADR 0087 §1). Pure Go, no dependency, non-OptIn.
package bri

import (
	"fmt"
	"sort"
	"sync/atomic"

	"github.com/muthuishere/cljgo/pkg/lang"
)

// gensymCounter makes occurrence/thunk locals unique across every
// -match-compile call in a process (a macro may expand many times).
var gensymCounter int64

// installMatchShims interns clojure.core.match's single macroexpand-time
// primitive. -match-compile takes (vars clauses): `vars` is the vector of
// match expressions (the occurrences) and `clauses` a vector of alternating
// pattern-row / action forms (a bare :else keyword is the catch-all row). It
// returns the compiled decision-tree form.
func installMatchShims(def func(name string, fn func(args ...any) any)) {
	def("-match-compile", func(args ...any) any {
		if len(args) != 2 {
			panic(fmt.Errorf("-match-compile expects 2 args (vars clauses), got %d", len(args)))
		}
		return compileMatch(args[0], args[1])
	})
}

// --- form constructors -------------------------------------------------------

func sym(s string) *lang.Symbol { return lang.NewSymbol(s) }
func list(xs ...any) any        { return lang.NewList(xs...) }

// core builds a fully-qualified clojure.core symbol so the emitted form is
// hygienic regardless of the caller's refers (core.match does the same).
func cc(name string) *lang.Symbol { return lang.NewSymbol("clojure.core/" + name) }

// notFoundKW is core.match's absent-key sentinel, ::not-found in
// clojure.core.match. Every map sub-occurrence is bound to
// (get m k :clojure.core.match/not-found) — NOT (get m k) — because the JVM
// hands that sentinel to a key's value pattern when the key is missing
// (clojure/core/match.clj: map-pattern-matrix-ocr-sym binds
// (val-at-expr focr k ::not-found)). That is observable: a :guard on an
// absent key runs against ::not-found and blows up exactly like the JVM's.
var notFoundKW = lang.NewKeyword("clojure.core.match/not-found")

// --- pattern model -----------------------------------------------------------

const (
	pWild = iota // _ or a binding symbol (irrefutable; binds carries the vars)
	pLit         // a literal: number / string / keyword / bool / nil / char
	pVec         // [p ...] with optional trailing & rest
	pSeq         // (p ... :seq)
	pMap         // {k p ...}
)

// pat is a parsed pattern node. binds are the vars bound to THIS node's
// occurrence (a binding symbol, and any :as). guards are predicate forms
// (already (pred var)) that must hold for the whole clause; they bubble up
// from anywhere in the subtree and are checked once the clause is irrefutable.
type pat struct {
	kind   int
	lit    any
	binds  []*lang.Symbol
	guards []any
	sub    []pat        // pVec / pSeq element patterns (fixed head)
	rest   *lang.Symbol // pVec / pSeq: & rest binding (nil if none, or _)
	keys   []any        // pMap keys (sorted)
	vals   []pat        // pMap value patterns, aligned to keys
}

func (p pat) isWild() bool { return p.kind == pWild }

// bind is one (symbol -> occurrence-form) obligation accumulated as columns
// are consumed; it becomes a let binding at the clause's action leaf.
type bind struct {
	sym *lang.Symbol
	occ any
}

// row is one clause in the matrix: its remaining columns (aligned to the
// current occurrences), the bindings/guards gathered so far, and its action.
type row struct {
	cols   []pat
	binds  []bind
	guards []any
	action any

	// presence holds the map-key EXISTENCE tests a row accumulated
	// ((not= <sub-occ> ::not-found), the JVM's MapKeyPattern). They are kept
	// apart from guards because the JVM evaluates them LAST: core.match only
	// wraps a key's value pattern in MapKeyPattern when that pattern is a
	// bare wildcard (wrap-values), and the resulting existential test is
	// deprioritized against real patterns, so a :guard on ANY key of the row
	// runs before the presence test of any other key. Oracle: for
	//   (match [m] [{:a x :b (y :guard even?)}] :hit :else :none)
	// the JVM throws on {} — the guard sees ::not-found — instead of
	// answering :none off a failed (contains? m :a).
	presence []any
}

// --- entry -------------------------------------------------------------------

func compileMatch(varsForm, clausesForm any) any {
	vars := toSlice(varsForm)
	clauses := toSlice(clausesForm)

	c := &compiler{}

	// Bind each match expression to a gensym local so an occurrence is
	// evaluated once even though the decision tree tests it many times.
	occs := make([]any, len(vars))
	letBinds := make([]any, 0, len(vars)*2)
	for i, v := range vars {
		g := c.gensym("occ")
		occs[i] = g
		letBinds = append(letBinds, g, v)
	}

	rows := c.parseClauses(clauses, len(occs), occs)
	fail := list(sym("throw"), list(cc("ex-info"), "No matching clause", lang.NewMap()))
	body, _ := c.compile(occs, rows, fail)

	if len(letBinds) == 0 {
		return body
	}
	return list(cc("let"), lang.NewVectorOwning(letBinds), body)
}

type compiler struct {
	// mapSub is the set of sub-occurrence gensyms bound by (get occ k
	// ::not-found). The sentinel is an INTERNAL value: core.match applies
	// value patterns and :guards to it, but the JVM binds nil into the
	// action body when an absent key's value is bound (verified against
	// core.match 1.1.0: `(match [m] [{:a (x :guard keyword?)}] [:kw x
	// (nil? x)] :else :none)` on {} => [:kw nil true] — the guard saw the
	// sentinel and passed, yet x is nil). withLet consults this set so the
	// sentinel can never escape into user code.
	mapSub map[*lang.Symbol]bool
}

func (c *compiler) markMapSub(g *lang.Symbol) {
	if c.mapSub == nil {
		c.mapSub = map[*lang.Symbol]bool{}
	}
	c.mapSub[g] = true
}

func (c *compiler) isMapSub(occ any) bool {
	s, ok := occ.(*lang.Symbol)
	return ok && c.mapSub[s]
}

func (c *compiler) gensym(prefix string) *lang.Symbol {
	n := atomic.AddInt64(&gensymCounter, 1)
	return lang.NewSymbol(fmt.Sprintf("%s__%d__m", prefix, n))
}

// parseClauses turns the alternating pattern-row/action list into rows,
// expanding :or (and nested :or) by row duplication so the core matrix
// algorithm never sees an alternative.
func (c *compiler) parseClauses(clauses []any, arity int, occs []any) []row {
	var rows []row
	for i := 0; i+1 < len(clauses); i += 2 {
		rowForm := clauses[i]
		action := clauses[i+1]

		var cols [][]pat // per-column alternatives
		if kw, ok := rowForm.(lang.Keyword); ok && kw.Name() == "else" {
			// :else — the catch-all, a row of wildcards over every occurrence.
			cols = make([][]pat, arity)
			for j := range cols {
				cols[j] = []pat{{kind: pWild}}
			}
		} else {
			elems := toSlice(rowForm)
			if len(elems) != arity {
				panic(fmt.Errorf("core.match: pattern row %v has %d patterns, expected %d (one per occurrence)", lang.PrintString(rowForm), len(elems), arity))
			}
			cols = make([][]pat, arity)
			for j, e := range elems {
				cols[j] = parsePat(e)
			}
		}

		// Cartesian product across columns → one row per combination of
		// per-column alternatives (the :or desugaring).
		for _, combo := range cartesian(cols) {
			rows = append(rows, row{cols: combo, action: action})
		}
	}
	return rows
}

// --- parsing -----------------------------------------------------------------

// parsePat parses one pattern form into its alternatives (len>1 only for :or).
func parsePat(form any) []pat {
	switch f := form.(type) {
	case *lang.Symbol:
		if f.Name() == "_" {
			return []pat{{kind: pWild}}
		}
		return []pat{{kind: pWild, binds: []*lang.Symbol{f}}}
	case *lang.Vector:
		return parseSeqLike(f, pVec)
	case lang.IPersistentVector:
		return parseSeqLike(f, pVec)
	case lang.IPersistentMap:
		return []pat{parseMap(f)}
	case lang.IPersistentList:
		return parseList(f)
	case lang.ISeq:
		return parseList(f)
	default:
		// number / string / keyword / bool / nil / char — a literal.
		return []pat{{kind: pLit, lit: form}}
	}
}

func parseSeqLike(v lang.IPersistentVector, kind int) []pat {
	n := v.Count()
	var heads []any
	var rest *lang.Symbol
	for i := 0; i < n; i++ {
		e := v.Nth(i)
		if s, ok := e.(*lang.Symbol); ok && s.Name() == "&" {
			if i+1 < n {
				if rs, ok := v.Nth(i + 1).(*lang.Symbol); ok {
					if rs.Name() != "_" {
						rest = rs
					}
				}
			}
			break
		}
		heads = append(heads, e)
	}
	// Each head may itself carry :or alternatives → cartesian.
	alts := make([][]pat, len(heads))
	for i, h := range heads {
		alts[i] = parsePat(h)
	}
	var out []pat
	for _, combo := range cartesian(alts) {
		out = append(out, pat{kind: kind, sub: combo, rest: rest})
	}
	if out == nil {
		out = []pat{{kind: kind, rest: rest}}
	}
	return out
}

func parseMap(m lang.IPersistentMap) pat {
	type kv struct {
		k any
		v any
	}
	var kvs []kv
	for s := lang.Seq(m); s != nil; s = s.Next() {
		e := s.First().(lang.IMapEntry)
		kvs = append(kvs, kv{e.Key(), e.Val()})
	}
	// Deterministic key order (drift-safe output).
	sort.Slice(kvs, func(i, j int) bool {
		return lang.PrintString(kvs[i].k) < lang.PrintString(kvs[j].k)
	})
	p := pat{kind: pMap}
	for _, e := range kvs {
		alts := parsePat(e.v)
		// Map value alternatives (:or) are rare; take the first alternative
		// and fold the rest into it is not sound, so require a single one.
		if len(alts) != 1 {
			panic(fmt.Errorf("core.match: :or inside a map value pattern is not supported (key %v)", lang.PrintString(e.k)))
		}
		p.keys = append(p.keys, e.k)
		p.vals = append(p.vals, alts[0])
	}
	return p
}

// parseList handles the special seq forms: (:or ...), (p :guard pred),
// (p :as sym), (p :seq).
func parseList(form any) []pat {
	elems := toSlice(form)
	if len(elems) == 0 {
		// The empty list as a pattern — treat as an empty seq literal is out of
		// scope; reject clearly.
		panic(fmt.Errorf("core.match: empty list () is not a valid pattern"))
	}
	if kw, ok := elems[0].(lang.Keyword); ok && kw.Name() == "or" {
		var out []pat
		for _, e := range elems[1:] {
			out = append(out, parsePat(e)...)
		}
		return out
	}
	// (inner :modifier arg) shapes.
	if len(elems) >= 2 {
		if kw, ok := elems[1].(lang.Keyword); ok {
			inner := parsePat(elems[0])
			switch kw.Name() {
			case "guard":
				if len(elems) != 3 {
					panic(fmt.Errorf("core.match: (p :guard pred) takes exactly one predicate"))
				}
				pred := elems[2]
				for i := range inner {
					v := bindingVar(&inner[i])
					inner[i].guards = append(inner[i].guards, list(pred, v))
				}
				return inner
			case "as":
				if len(elems) != 3 {
					panic(fmt.Errorf("core.match: (p :as name) takes exactly one name"))
				}
				name, ok := elems[2].(*lang.Symbol)
				if !ok {
					panic(fmt.Errorf("core.match: :as wants a symbol, got %v", lang.PrintString(elems[2])))
				}
				for i := range inner {
					inner[i].binds = append(inner[i].binds, name)
				}
				return inner
			case "seq":
				// (inner :seq): inner is a vector-shaped seq pattern.
				if v, ok := elems[0].(lang.IPersistentVector); ok {
					return parseSeqLike(v, pSeq)
				}
				panic(fmt.Errorf("core.match: (p :seq) wants a vector pattern, got %v", lang.PrintString(elems[0])))
			}
		}
	}
	panic(fmt.Errorf("core.match: unsupported list pattern %v (supported: (:or ...), (p :guard f), (p :as x), (v :seq))", lang.PrintString(form)))
}

// bindingVar returns the var a modifier (:guard) applies to: an existing
// binding symbol if the inner pattern already binds one, else a fresh gensym
// bound to the same occurrence.
func bindingVar(p *pat) *lang.Symbol {
	if len(p.binds) > 0 {
		return p.binds[0]
	}
	g := lang.NewSymbol(fmt.Sprintf("g__guard__%d__m", atomic.AddInt64(&gensymCounter, 1)))
	p.binds = append(p.binds, g)
	return g
}

// --- the Maranget core -------------------------------------------------------

// compile emits the decision tree for `rows` over `occs`, with `fail` the
// form to run when no row matches. It returns (form, usedFail) where usedFail
// reports whether `fail` was referenced (so callers can decide to share it).
func (c *compiler) compile(occs []any, rows []row, fail any) (any, bool) {
	if len(rows) == 0 {
		return fail, true
	}

	r0 := rows[0]
	col := firstRefutable(r0)
	if col == -1 {
		// Row 0 is structurally irrefutable — it matches. Fold its binding
		// columns into bindings, then guard (if any) decides accept vs
		// fall-through to the remaining rows.
		binds := append([]bind(nil), r0.binds...)
		for i, p := range r0.cols {
			for _, s := range p.binds {
				binds = append(binds, bind{s, occs[i]})
			}
		}
		guards := append([]any(nil), r0.guards...)
		for _, p := range r0.cols {
			guards = append(guards, p.guards...)
		}
		// Key-existence tests go LAST — see row.presence.
		guards = append(guards, r0.presence...)
		if len(guards) == 0 {
			return c.withLet(binds, c.unsentinel(binds, r0.action)), false // remaining rows unreachable
		}
		// Guards must see the row's bindings, so the let wraps the whole test:
		// (let [binds…] (if (and guards…) action <fall-through>)).
		rest, restUsed := c.compile(occs, rows[1:], fail)
		// Only the ACTION is unsentinel-ed; the guards above it must still
		// see the raw ::not-found occurrence.
		inner := list(sym("if"), andForm(guards), c.unsentinel(binds, r0.action), rest)
		return c.withLet(binds, inner), restUsed
	}

	// Dispatch on column `col`: gather its distinct head constructors, build a
	// specialized matrix per constructor plus the default matrix (wildcard
	// rows), and chain the tests.
	occ := occs[col]
	ctors := distinctCtors(rows, col)

	defOccs := removeAt(occs, col)
	defRows := defaultMatrix(rows, col, occ)
	defForm, defUsed := c.compile(defOccs, defRows, fail)

	// A shared thunk for the default, referenced by every branch that can
	// fall through (deep guard/pattern failure) and by the tail else — only
	// hoisted when a branch actually references it, so the common exhaustive
	// literal path stays a plain if-chain with no closure allocation.
	thunk := c.gensym("fail")
	thunkCall := list(thunk)

	branchForms := make([]any, len(ctors))
	anyBranchUsedFail := false
	for i, ct := range ctors {
		subOccs, letPairs := c.expandOccs(occ, ct)
		newOccs := spliceAt(occs, col, subOccs)
		specRows := specialize(rows, col, ct, occ, subOccs)
		bform, used := c.compile(newOccs, specRows, thunkCall)
		if len(letPairs) > 0 {
			bform = list(cc("let"), lang.NewVectorOwning(letPairs), bform)
		}
		branchForms[i] = bform
		anyBranchUsedFail = anyBranchUsedFail || used
	}

	var tail any
	if anyBranchUsedFail {
		tail = thunkCall
	} else {
		tail = defForm
	}

	// Build the right-nested if-chain from the bottom up.
	chain := tail
	for i := len(ctors) - 1; i >= 0; i-- {
		chain = list(sym("if"), ctorTest(occ, ctors[i]), branchForms[i], chain)
	}

	if anyBranchUsedFail {
		chain = list(cc("let"),
			lang.NewVectorOwning([]any{thunk, list(cc("fn"), lang.NewVector(), defForm)}),
			chain)
	}
	return chain, defUsed
}

// firstRefutable returns the first column whose pattern needs a runtime test,
// or -1 when the row is entirely wildcards/bindings.
func firstRefutable(r row) int {
	for i, p := range r.cols {
		if !p.isWild() {
			return i
		}
	}
	return -1
}

// ctor is a constructor signature extracted from a column: a literal value, a
// fixed/rest vector or seq of some arity, or a map of some key set.
type ctor struct {
	kind    int
	lit     any
	arity   int
	hasRest bool
	keys    []any
}

func ctorOf(p pat) ctor {
	switch p.kind {
	case pLit:
		return ctor{kind: pLit, lit: p.lit}
	case pVec:
		return ctor{kind: pVec, arity: len(p.sub), hasRest: p.rest != nil}
	case pSeq:
		return ctor{kind: pSeq, arity: len(p.sub), hasRest: p.rest != nil}
	case pMap:
		return ctor{kind: pMap, keys: p.keys}
	}
	return ctor{}
}

func ctorEq(a, b ctor) bool {
	if a.kind != b.kind {
		return false
	}
	switch a.kind {
	case pLit:
		return lang.Equals(a.lit, b.lit)
	case pVec, pSeq:
		return a.arity == b.arity && a.hasRest == b.hasRest
	case pMap:
		if len(a.keys) != len(b.keys) {
			return false
		}
		for i := range a.keys {
			if !lang.Equals(a.keys[i], b.keys[i]) {
				return false
			}
		}
		return true
	}
	return false
}

// distinctCtors collects the head constructors present in column `col`, in
// first-appearance order (only refutable patterns contribute).
//
// Map patterns are the one family that is NOT mutually exclusive: {:kind
// :click} and {:kind :click :button "left"} both match the same map, so
// treating each key set as its own constructor makes the if-chain
// (if (map? o) A (if (map? o) B …)) — the second arm is dead and its rows are
// silently dropped (the "other click" bug). Maranget dispatch requires
// disjoint constructors, so EVERY map pattern in a column collapses into ONE
// constructor over the UNION of the column's keys — the same all-keys union
// core.match builds. Each row is then re-constrained in specialize: keys whose
// value pattern is a bare wildcard get an existence test, keys with a real
// pattern get that pattern applied to (get m k ::not-found). Oracle:
// (match [{}] [{:a _}] :has-a :else :no) => :no.
func distinctCtors(rows []row, col int) []ctor {
	var out []ctor
	var mapKeys []any
	mapIdx := -1
	for _, r := range rows {
		p := r.cols[col]
		if p.isWild() {
			continue
		}
		ct := ctorOf(p)
		if ct.kind == pMap {
			for _, k := range ct.keys {
				dup := false
				for _, e := range mapKeys {
					if lang.Equals(e, k) {
						dup = true
						break
					}
				}
				if !dup {
					mapKeys = append(mapKeys, k)
				}
			}
			if mapIdx == -1 {
				mapIdx = len(out)
				out = append(out, ctor{kind: pMap})
			}
			continue
		}
		dup := false
		for _, e := range out {
			if ctorEq(e, ct) {
				dup = true
				break
			}
		}
		if !dup {
			out = append(out, ct)
		}
	}
	if mapIdx >= 0 {
		// Deterministic key order (drift-safe output), same rule as parseMap.
		sort.Slice(mapKeys, func(i, j int) bool {
			return lang.PrintString(mapKeys[i]) < lang.PrintString(mapKeys[j])
		})
		out[mapIdx].keys = mapKeys
	}
	return out
}

// ctorTest builds the boolean test that occ matches constructor ct.
func ctorTest(occ any, ct ctor) any {
	switch ct.kind {
	case pLit:
		if ct.lit == nil {
			return list(cc("nil?"), occ)
		}
		return list(cc("="), occ, ct.lit)
	case pVec:
		if ct.hasRest {
			return list(cc("and"), list(cc("vector?"), occ),
				list(cc(">="), list(cc("count"), occ), int64(ct.arity)))
		}
		return list(cc("and"), list(cc("vector?"), occ),
			list(cc("="), list(cc("count"), occ), int64(ct.arity)))
	case pSeq:
		if ct.hasRest {
			return list(cc("and"), list(cc("sequential?"), occ),
				list(cc(">="), list(cc("count"), occ), int64(ct.arity)))
		}
		return list(cc("and"), list(cc("sequential?"), occ),
			list(cc("="), list(cc("count"), occ), int64(ct.arity)))
	case pMap:
		return list(cc("map?"), occ)
	}
	return true
}

// expandOccs binds the sub-occurrences of constructor ct (accessors into occ)
// to fresh locals so each is evaluated once, and returns them plus the let
// pairs. Literals have no sub-occurrences.
func (c *compiler) expandOccs(occ any, ct ctor) (subOccs []any, letPairs []any) {
	switch ct.kind {
	case pVec:
		for i := 0; i < ct.arity; i++ {
			g := c.gensym("v")
			subOccs = append(subOccs, g)
			letPairs = append(letPairs, g, list(cc("nth"), occ, int64(i)))
		}
	case pSeq:
		for i := 0; i < ct.arity; i++ {
			g := c.gensym("s")
			subOccs = append(subOccs, g)
			letPairs = append(letPairs, g, list(cc("nth"), occ, int64(i)))
		}
	case pMap:
		for _, k := range ct.keys {
			g := c.gensym("m")
			c.markMapSub(g)
			subOccs = append(subOccs, g)
			// (get occ k ::not-found), not (get occ k): an absent key must be
			// distinguishable from a nil value, and the sentinel is what a
			// value pattern (literal / nested map / :guard) is applied to.
			letPairs = append(letPairs, g, list(cc("get"), occ, k, notFoundKW))
		}
	}
	return subOccs, letPairs
}

// specialize builds the matrix of rows that can still match once occ is known
// to be constructor ct: rows whose column is exactly ct (sub-patterns spliced
// in), and wildcard rows (arity wildcards spliced in, binding recorded). Rows
// with a different constructor are dropped. A rest binding is recorded against
// (subvec occ arity) / (nthrest occ arity).
func specialize(rows []row, col int, ct ctor, occ any, subOccs []any) []row {
	var out []row
	for _, r := range rows {
		p := r.cols[col]
		nr := row{
			binds:    append([]bind(nil), r.binds...),
			guards:   append([]any(nil), r.guards...),
			presence: append([]any(nil), r.presence...),
			action:   r.action,
		}
		var mid []pat
		if p.isWild() {
			for _, s := range p.binds {
				nr.binds = append(nr.binds, bind{s, occ})
			}
			nr.guards = append(nr.guards, p.guards...)
			mid = arityWilds(ct)
		} else {
			ctp := ctorOf(p)
			if ct.kind == pMap {
				// Every map pattern shares the column's single unified map
				// constructor (see distinctCtors); only non-maps are dropped.
				if ctp.kind != pMap {
					continue
				}
			} else if !ctorEq(ctp, ct) {
				continue
			}
			for _, s := range p.binds {
				nr.binds = append(nr.binds, bind{s, occ})
			}
			switch ct.kind {
			case pMap:
				// This row only constrains the keys IT names: realign its value
				// patterns onto the column's key union, wildcarding the keys it
				// omits. Whether a named key gets an EXISTENCE test depends on
				// its value pattern, exactly as core.match's `wrap-values`
				// decides: a bare wildcard/binding becomes a MapKeyPattern
				// ((not= ocr ::not-found)) — so {:a x} matches {:a nil} but not
				// {} — while any real pattern (literal, nested map, vector,
				// :guard) is instead applied straight to the sub-occurrence,
				// which is ::not-found when the key is absent. That is why a
				// :guard on a missing key RUNS and throws on the sentinel
				// rather than being short-circuited away.
				nr.guards = append(nr.guards, p.guards...)
				for i, k := range ct.keys {
					sub := pat{kind: pWild}
					named := false
					for j, pk := range p.keys {
						if lang.Equals(pk, k) {
							sub = p.vals[j]
							named = true
							break
						}
					}
					if named && sub.isWild() && len(sub.guards) == 0 {
						nr.presence = append(nr.presence,
							list(cc("not="), subOccs[i], notFoundKW))
					}
					mid = append(mid, sub)
				}
			case pVec, pSeq:
				nr.guards = append(nr.guards, p.guards...)
				mid = append(mid, p.sub...)
				if ct.hasRest && p.rest != nil {
					var acc any
					if ct.kind == pVec {
						acc = list(cc("subvec"), occ, int64(ct.arity))
					} else {
						acc = list(cc("nthrest"), occ, int64(ct.arity))
					}
					nr.binds = append(nr.binds, bind{p.rest, acc})
				}
			default:
				nr.guards = append(nr.guards, p.guards...)
			}
		}
		nr.cols = splicePat(r.cols, col, mid)
		out = append(out, nr)
	}
	return out
}

// arityWilds returns the wildcard sub-patterns a wildcard row contributes when
// specialized against constructor ct.
func arityWilds(ct ctor) []pat {
	n := ct.arity
	if ct.kind == pLit || ct.kind == pMap {
		n = len(ct.keys) // 0 for lit, key-count for map
	}
	out := make([]pat, n)
	for i := range out {
		out[i] = pat{kind: pWild}
	}
	return out
}

// defaultMatrix builds the rows that survive when occ matches NONE of the
// dispatched constructors: those with a wildcard in the column (column
// removed, binding recorded).
func defaultMatrix(rows []row, col int, occ any) []row {
	var out []row
	for _, r := range rows {
		p := r.cols[col]
		if !p.isWild() {
			continue
		}
		nr := row{
			binds:    append([]bind(nil), r.binds...),
			guards:   append(append([]any(nil), r.guards...), p.guards...),
			presence: append([]any(nil), r.presence...),
			action:   r.action,
		}
		for _, s := range p.binds {
			nr.binds = append(nr.binds, bind{s, occ})
		}
		nr.cols = removeAtPat(r.cols, col)
		out = append(out, nr)
	}
	return out
}

// --- form helpers ------------------------------------------------------------

// withLet emits the clause's binding let. The occurrences are bound RAW —
// a map sub-occurrence still carries the ::not-found sentinel here, because
// the row's :guards are evaluated INSIDE this let and core.match applies
// guards to the sentinel (that is what makes `even?` throw on an absent key).
func (c *compiler) withLet(binds []bind, action any) any {
	if len(binds) == 0 {
		return action
	}
	pairs := make([]any, 0, len(binds)*2)
	for _, b := range binds {
		pairs = append(pairs, b.sym, b.occ)
	}
	return list(cc("let"), lang.NewVectorOwning(pairs), action)
}

// unsentinel re-binds, for the ACTION body only, every symbol whose value came
// from a map sub-occurrence, mapping the sentinel to nil.
//
// The guard and the binding see DIFFERENT values on the JVM, verified against
// core.match 1.1.0:
//
//	(match [m] [{:a (x :guard keyword?)}] [:kw x (nil? x)] :else :none)
//	{} => [:kw nil true]
//
// The guard received ::not-found (keyword? passed, so the row matched), yet x
// is nil inside the action. Emitting one let would leak the sentinel into user
// code; emitting two keeps both halves true.
func (c *compiler) unsentinel(binds []bind, action any) any {
	pairs := make([]any, 0, len(binds)*2)
	for _, b := range binds {
		if !c.isMapSub(b.occ) {
			continue
		}
		// `if` is a SPECIAL FORM — it has no var, so it stays unqualified
		// (clojure.core/if does not resolve).
		pairs = append(pairs, b.sym,
			list(lang.NewSymbol("if"), list(cc("="), b.sym, notFoundKW), nil, b.sym))
	}
	if len(pairs) == 0 {
		return action
	}
	return list(cc("let"), lang.NewVectorOwning(pairs), action)
}

func andForm(guards []any) any {
	if len(guards) == 1 {
		return guards[0]
	}
	return list(append([]any{cc("and")}, guards...)...)
}

// --- slice helpers -----------------------------------------------------------

func toSlice(x any) []any {
	if x == nil {
		return nil
	}
	if v, ok := x.(lang.IPersistentVector); ok {
		out := make([]any, v.Count())
		for i := range out {
			out[i] = v.Nth(i)
		}
		return out
	}
	var out []any
	for s := lang.Seq(x); s != nil; s = s.Next() {
		out = append(out, s.First())
	}
	return out
}

func cartesian(cols [][]pat) [][]pat {
	out := [][]pat{{}}
	for _, alts := range cols {
		var next [][]pat
		for _, prefix := range out {
			for _, a := range alts {
				row := append(append([]pat(nil), prefix...), a)
				next = append(next, row)
			}
		}
		out = next
	}
	return out
}

func removeAt(xs []any, i int) []any {
	out := make([]any, 0, len(xs)-1)
	out = append(out, xs[:i]...)
	out = append(out, xs[i+1:]...)
	return out
}

func removeAtPat(xs []pat, i int) []pat {
	out := make([]pat, 0, len(xs)-1)
	out = append(out, xs[:i]...)
	out = append(out, xs[i+1:]...)
	return out
}

// spliceAt replaces element i of xs with the elements of mid.
func spliceAt(xs []any, i int, mid []any) []any {
	out := make([]any, 0, len(xs)-1+len(mid))
	out = append(out, xs[:i]...)
	out = append(out, mid...)
	out = append(out, xs[i+1:]...)
	return out
}

// splicePat replaces element i of xs with the elements of mid ([]pat).
func splicePat(xs []pat, i int, mid []pat) []pat {
	out := make([]pat, 0, len(xs)-1+len(mid))
	out = append(out, xs[:i]...)
	out = append(out, mid...)
	out = append(out, xs[i+1:]...)
	return out
}
