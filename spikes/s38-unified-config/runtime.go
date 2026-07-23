package main

import (
	"sort"
	"sync"
	"time"
)

// --- vault layer (stub) -----------------------------------------------------

// Vault is the secrets layer between env and the DB override. Stubbed as an
// interface so the blessed form can bind it to a real backend (Infisical,
// vsync, cloud KMS) later. It returns typed scalars by path.
type Vault interface {
	Lookup(path Path) (any, bool)
	// SecretPaths lets the resolver know which keys must be masked even
	// before a value is present at a higher layer.
	SecretPaths() []Path
}

// stubVault is an in-memory Vault for the demo.
type stubVault struct{ data map[string]any }

func newStubVault() *stubVault { return &stubVault{data: map[string]any{}} }

func (v *stubVault) Put(path Path, val any) { v.data[join(path)] = val }

func (v *stubVault) Lookup(path Path) (any, bool) {
	val, ok := v.data[join(path)]
	return val, ok
}

func (v *stubVault) SecretPaths() []Path {
	out := make([]Path, 0, len(v.data))
	for k := range v.data {
		out = append(out, splitJoin(k))
	}
	return out
}

// --- DB-primary runtime layer ----------------------------------------------

// runtimeRow models one row of the reqsume `runtime_config` table: the value
// wins over files/env, is flagged secret (AES-GCM at rest in prod; plain here),
// and carries who/when audit. `.env`/files are bootstrap seed only — a present
// row is authoritative (reqsume-kernel §2.1).
type runtimeRow struct {
	Value     any
	IsSecret  bool
	UpdatedBy string
	UpdatedAt time.Time
}

// RuntimeStore is the DB-primary override layer with a read-through cache.
// In prod this fronts a Postgres table; here an in-memory map stands in, but
// the read-through + invalidate + audit mechanics are modeled faithfully so
// the hot-rotate-without-restart property (criterion 4) is real.
type RuntimeStore struct {
	mu    sync.RWMutex
	rows  map[string]runtimeRow // "db.pool-size" -> row  (the "table")
	cache map[string]runtimeRow // read-through snapshot
	fresh bool                  // false => next Lookup rebuilds the cache
	reads int                   // table reads, to prove caching
}

func NewRuntimeStore() *RuntimeStore {
	return &RuntimeStore{rows: map[string]runtimeRow{}, cache: map[string]runtimeRow{}}
}

// Set writes/updates a row (the PUT /admin/runtime-config path) and BUSTS the
// cache, exactly like UpdateManagedRuntimeConfig. Audit fields are stamped
// here. This is the only mutation path — direct-SQL staleness (kernel gotcha
// #1) is out of scope by construction.
func (s *RuntimeStore) Set(path Path, val any, secret bool, by string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.rows[join(path)] = runtimeRow{Value: val, IsSecret: secret, UpdatedBy: by, UpdatedAt: time.Now()}
	s.fresh = false // invalidate read-through cache
}

// ensureFresh rebuilds the cache from the "table" on demand (read-through).
func (s *RuntimeStore) ensureFresh() {
	s.mu.RLock()
	fresh := s.fresh
	s.mu.RUnlock()
	if fresh {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.fresh {
		return
	}
	snap := make(map[string]runtimeRow, len(s.rows))
	for k, v := range s.rows {
		snap[k] = v
	}
	s.cache = snap
	s.fresh = true
	s.reads++ // one table read per (re)load, not per Lookup
}

// Lookup returns the override value for a path, serving from cache.
func (s *RuntimeStore) Lookup(path Path) (runtimeRow, bool) {
	s.ensureFresh()
	s.mu.RLock()
	defer s.mu.RUnlock()
	r, ok := s.cache[join(path)]
	return r, ok
}

// Rows returns a stable snapshot for the audit dump.
func (s *RuntimeStore) Rows() []struct {
	Path Path
	Row  runtimeRow
} {
	s.ensureFresh()
	s.mu.RLock()
	defer s.mu.RUnlock()
	keys := make([]string, 0, len(s.cache))
	for k := range s.cache {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	out := make([]struct {
		Path Path
		Row  runtimeRow
	}, 0, len(keys))
	for _, k := range keys {
		out = append(out, struct {
			Path Path
			Row  runtimeRow
		}{splitJoin(k), s.cache[k]})
	}
	return out
}

// TableReads reports how many times the underlying "table" was read — proves
// the cache serves repeated Lookups without re-reading (criterion 4).
func (s *RuntimeStore) TableReads() int {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.reads
}

func join(p Path) string { return joinDot(p) }
