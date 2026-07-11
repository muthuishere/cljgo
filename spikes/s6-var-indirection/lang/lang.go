// Package lang is a minimal stand-in for cljgo's pkg/lang, containing ONLY
// what S6 needs: the emitted-closure type (Fn), the Apply1 fast path, and
// three Var implementations (atomic.Value, atomic.Pointer, sync.Mutex) so we
// can price per-call Var indirection against each other and against raw Go.
//
// Shapes follow design/00-architecture.md §4.2:
//   - closures emit as `lang.Fn func(args ...any) any`
//   - call sites go through Apply0..Apply4 fast paths
//   - Var is the mutable indirection layer; re-def must stay live by default
package lang

import (
	"sync"
	"sync/atomic"
)

// Fn is the emitted-closure calling convention (§4.2).
type Fn func(args ...any) any

// IFn is the interface escape hatch (evaluator fns, keywords, colls...).
type IFn interface {
	Invoke(args ...any) any
}

// Apply1 is the 1-arg fast path the emitter uses at every call site.
// Fast case first: the concrete emitted-closure type. Note the variadic call
// fn(a) still materializes a 1-elem []any — that cost is part of what this
// spike measures (it shows up in allocs/op).
func Apply1(f any, a any) any {
	switch fn := f.(type) {
	case Fn:
		return fn(a)
	case func(args ...any) any:
		return fn(a)
	case IFn:
		return fn.Invoke(a)
	}
	panic("Apply1: not callable")
}

// ---------------------------------------------------------------------------
// Var variants under test
// ---------------------------------------------------------------------------

// Var is the atomic.Value-based var (the planned default: wait-free loads).
// Root values must share a concrete type per atomic.Value's contract; for
// this spike every root is a lang.Fn, which matches emitted defn roots.
type Var struct {
	root atomic.Value
}

func NewVar(v Fn) *Var {
	var vr Var
	if v != nil {
		vr.root.Store(v)
	}
	return &vr
}

func (v *Var) Set(f Fn)    { v.root.Store(f) }
func (v *Var) Deref() any  { return v.root.Load() }
func (v *Var) DerefFn() Fn { return v.root.Load().(Fn) }

// Fn1 is a fixed-arity (1-arg) closure type — the doc 04 §5 performance-
// ladder representation. No variadic slice at call sites.
type Fn1 func(a any) any

// Var1 is a Var whose root is a fixed-arity Fn1. Models the ladder rung
// "fixed-arity fn types" WITH per-call deref kept (REPL-live preserved).
type Var1 struct {
	root atomic.Value
}

func NewVar1(f Fn1) *Var1 {
	var v Var1
	if f != nil {
		v.root.Store(f)
	}
	return &v
}

func (v *Var1) Set(f Fn1)   { v.root.Store(f) }
func (v *Var1) Deref1() Fn1 { return v.root.Load().(Fn1) }

// PtrVar is the atomic.Pointer-based alternative. One extra pointer chase on
// load, but no interface-shape restrictions (can hold any root type via *any).
type PtrVar struct {
	root atomic.Pointer[Fn]
}

func NewPtrVar(v Fn) *PtrVar {
	var p PtrVar
	p.Set(v)
	return &p
}

func (p *PtrVar) Set(f Fn)    { p.root.Store(&f) }
func (p *PtrVar) DerefFn() Fn { return *p.root.Load() }
func (p *PtrVar) Deref() any  { return *p.root.Load() }

// MutexVar is the strawman: lock on every deref. Benchmarked only to confirm
// the atomic choice with numbers.
type MutexVar struct {
	mu   sync.Mutex
	root Fn
}

func NewMutexVar(v Fn) *MutexVar { return &MutexVar{root: v} }

func (m *MutexVar) Set(f Fn) {
	m.mu.Lock()
	m.root = f
	m.mu.Unlock()
}

func (m *MutexVar) Deref() any {
	m.mu.Lock()
	r := m.root
	m.mu.Unlock()
	return r
}

// RWMutexVar: the other lock-based contender (read locks are cheaper than
// Mutex under read-mostly load, but still not wait-free).
type RWMutexVar struct {
	mu   sync.RWMutex
	root Fn
}

func NewRWMutexVar(v Fn) *RWMutexVar { return &RWMutexVar{root: v} }

func (m *RWMutexVar) Set(f Fn) {
	m.mu.Lock()
	m.root = f
	m.mu.Unlock()
}

func (m *RWMutexVar) Deref() any {
	m.mu.RLock()
	r := m.root
	m.mu.RUnlock()
	return r
}
