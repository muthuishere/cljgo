# 02 — Persistent Data Structures & Core Runtime Value Types

Component design for the runtime library that AOT-emitted Go code links against
(our analog of `cljs.core` in ClojureScript output). The compiler emits plain Go
that imports this package — call it `pkg/lang` (import alias `lang`).

References studied:
- Clojure JVM: `/Users/muthuishere/Downloads/clojure-master/src/jvm/clojure/lang/` (PersistentVector.java, PersistentHashMap.java, Keyword.java, LazySeq.java, Var.java, Atom.java)
- Glojure: `refs/glojure/pkg/lang/` (~15.8k lines, EPL-1.0)
- let-go: `refs/let-go/pkg/vm/` (MIT)

---

## 1. Core abstractions in Go

### 1.1 Value representation: `any`, not a boxed `Value` interface

Two models exist in the references:

- **let-go**: everything implements `Value` (`Type() ValueType`, `Unbox() any`). Great
  for a bytecode VM (uniform dispatch), but AOT-emitted Go would drown in
  `Box()`/`Unbox()` calls and Go interop would need wrappers for every native value.
- **Glojure**: `type Object = any`. Scalars are raw Go values (`int64`, `float64`,
  `string`, `bool`, `nil`); collections are pointer types implementing small
  interfaces; dispatch via type switches and interface assertions.

**Decision: `any`.** Emitted code reads like Go (`lang.Conj(v, int64(1))`), interop is
free, and Go's interface dispatch *is* our polymorphism. This mirrors how CLJS uses
raw JS strings/numbers rather than wrapping them.

Canonical scalar types (the compiler normalizes literals to these):

| Clojure | Go |
|---|---|
| nil | `nil` |
| boolean | `bool` |
| long (default int) | `int64` |
| double | `float64` |
| string | `string` |
| char | `lang.Char` (`type Char rune`) |
| bigint / ratio / bigdec | `*lang.BigInt`, `*lang.Ratio`, `*lang.BigDecimal` (post-v0) |

Truthiness: `IsTruthy(x) = x != nil && x != false` (single helper; emitted `if` uses it).

### 1.2 Interface set

Clojure's Java interfaces translate almost 1:1. Keep them *small* — Go rewards narrow
interfaces. This is essentially Glojure's `interfaces.go` layout, which is faithful:

```go
type (
    ISeq interface {
        IPersistentCollection
        First() any
        Next() ISeq   // nil when exhausted
        More() ISeq   // () — empty list, never nil
    }
    Seqable   interface{ Seq() ISeq }
    IPersistentCollection interface {
        Seqable
        Count() int
        Cons(any) IPersistentCollection // conj for this coll type
        Empty() IPersistentCollection
        Equiv(any) bool
    }
    ILookup     interface{ ValAt(k any) any; ValAtDefault(k, def any) any }
    Associative interface {
        IPersistentCollection; ILookup
        ContainsKey(any) bool
        EntryAt(any) IMapEntry
        Assoc(k, v any) Associative
    }
    IPersistentMap interface {
        Associative
        Without(k any) IPersistentMap
    }
    IPersistentVector interface {
        Associative; IPersistentStack; Reversible; Indexed
        AssocN(i int, v any) IPersistentVector
    }
    Indexed  interface{ Counted; Nth(int) any; NthDefault(int, any) any }
    Counted  interface{ Count() int; xxx_counted() } // marker = O(1) count
    IFn      interface{ Invoke(args ...any) any; ApplyTo(ISeq) any }
    IMeta    interface{ Meta() IPersistentMap }
    IObj     interface{ IMeta; WithMeta(IPersistentMap) any }
    IDeref   interface{ Deref() any }
    IPending interface{ IsRealized() bool }
    IHashEq  interface{ HashEq() uint32 }
    Named    interface{ Name() string; Namespace() string }
    IReduceInit interface{ ReduceInit(f IFn, init any) any }
    IKVReduce   interface{ KVReduce(f IFn, init any) any }
    // transients
    IEditableCollection  interface{ AsTransient() ITransientCollection }
    ITransientCollection interface{ Conj(any) ITransientCollection; Persistent() IPersistentCollection }
)
```

Go has no abstract classes, so `ASeq`/`APersistentMap` behavior (equals, hash,
seq-based fallbacks) becomes shared *functions* (`aseqHashEq(cache *uint32, s ISeq)`,
`apersistentmapEquiv(m IPersistentMap, o any)`) called from each concrete type —
exactly Glojure's pattern (`aseq.go`, `apersistentmap.go`). Marker interfaces
(`Sequential`, `Counted`) use unexported methods (`xxx_sequential()`) so foreign types
can't accidentally satisfy them.

### 1.3 Equality and hashing

Faithful to Clojure means **two equality relations and two hashes**:

- `Equiv(a, b)` — Clojure `=`: nil-safe; numbers compared *by category*
  (`(= 1 1.0)` → false, `(= 1 1N)` → true); collections via
  `IPersistentCollection.Equiv` (vector = list = lazy-seq when elements equiv).
- `Equals(a, b)` — Java `.equals` analog, type-strict; used by interop and by
  `identical?`-adjacent paths. **Glojure conflates these** (`equal.go`:
  `func Equiv(a, b any) bool { return Equals(a, b) }`) — we must not.
- `HashEq(x) uint32` — Murmur3-based, the hash used by map/set keys. Invariant:
  `Equiv(a,b) ⇒ HashEq(a)==HashEq(b)`. Longs hash via `murmur3.HashLong`; strings via
  `murmur3.HashInt(int32(javaStringHash(s)))` (must match so `BigInt(1)` and
  `int64(1)` collide, per category hashing); ordered colls via `hashOrdered`
  (mix each element's hasheq, finalize with count); maps/sets via `hashUnordered`
  (sum of entry hashes, order-independent).

```go
func HashEq(x any) uint32 {
    switch x := x.(type) {
    case nil:        return 0
    case IHashEq:    return x.HashEq()     // colls cache; kw/sym precompute
    case int64:      return murmur3.HashLong(x)
    case string:     return murmur3.HashInt(int32(stringHash(x)))
    case bool:       if x { return 1231 }; return 1237
    case float64:    return hashFloat(x)   // Double.hashCode analog
    ...
    }
}
```

Collections cache `hash, hasheq uint32` fields (0 = unset sentinel) — same as JVM
Clojure and Glojure. No `reflect` on the hot path: Glojure's `Hash` falls through to
`hashstructure`/`fmt.Sprintf` for unknown structs — acceptable escape hatch for interop
values, but our emitted code only produces known types.

---

## 2. The two workhorse structures

### 2.1 PersistentVector — 32-way trie + tail

Per `PersistentVector.java`:

```go
type vnode struct {
    edit  *atomic.Bool  // transient ownership token; nil-equivalent NOEDIT for persistent
    array []any         // 32 slots: leaf = values, interior = *vnode
}
type PersistentVector struct {
    meta         IPersistentMap
    hash, hasheq uint32
    cnt   int
    shift uint     // 5 * (depth-1); 5 for a 1-level trie
    root  *vnode
    tail  []any    // last ≤32 elements, NOT in the trie
}
```

Key algorithms (all in the Java source; Glojure delegates to a vendored clone of
elvish's port at `internal/persistent/vector/`):

- `tailoff() = 0 if cnt < 32 else ((cnt-1) >> 5) << 5`.
- **Nth**: `i >= tailoff` → `tail[i & 0x1f]` (O(1), the common append-read case);
  else descend: `for lvl := shift; lvl > 0; lvl -= 5 { node = node.array[(i>>lvl)&0x1f] }`.
- **Cons**: tail has room (`cnt - tailoff() < 32`) → copy tail + append (no trie
  touch). Tail full → `pushTail` the old tail as a leaf; **root overflow** when
  `(cnt >> 5) > (1 << shift)` → new root with old root as child 0 and a fresh path
  to the tail node (`newPath`), `shift += 5`.
- **Pop**: symmetric `popTail`; collapse root when it has one child.
- **AssocN**: path-copy from root to leaf (or tail copy if `i >= tailoff`).

**Transients** (`TransientVector`): the `edit` token is the ownership marker. JVM uses
`AtomicReference<Thread>`; Go has no thread identity, so the token is just a unique
pointer allocated per `AsTransient()` and *invalidated* (set false) by `Persistent()`:

```go
func (n *vnode) ensureEditable(edit *atomic.Bool) *vnode {
    if n.edit == edit { return n }          // already owned: mutate in place
    return &vnode{edit: edit, array: slices.Clone(n.array)} // clone once per session
}
```

`Persistent()` sets `edit.Store(false)`; every transient op first checks
`edit.Load()` and panics "used after persistent!". We do NOT get JVM's
wrong-thread detection — document that transients are single-goroutine by contract
(Clojure 1.7+ relaxed the thread check anyway).

### 2.2 PersistentHashMap — HAMT

Per `PersistentHashMap.java` (Glojure's `persistenthashmap.go` ports the persistent
half faithfully; **it omits transients entirely** — its only "transient" map is a
fake wrapper over the array-map, `persistentarraymap.go:325`):

```go
type PersistentHashMap struct {
    meta         IPersistentMap
    hash, hasheq uint32
    count     int
    root      node   // nil when empty (bar nil-key)
    hasNil    bool   // nil keys live outside the trie
    nilValue  any
}
type node interface {
    assoc(edit *atomic.Bool, shift uint, hash uint32, k, v any, addedLeaf *box) node
    without(edit *atomic.Bool, shift uint, hash uint32, k any, removedLeaf *box) node
    find(shift uint, hash uint32, k any) (any, any, bool) // key, val, found
    nodeSeq() ISeq
}
```

Three node kinds, hash consumed 5 bits per level (`(hash >> shift) & 0x1f`), max
depth 7:

- **bitmapIndexedNode** `{edit; bitmap uint32; array []any}` — array holds `2n`
  slots as (key, val) pairs; a sub-node is stored as `(nil, node)`. Position:
  `idx = popcount(bitmap & (bit-1))` (`bits.OnesCount32`). On insert collision at a
  slot: both keys → new sub-node one level down (`createNode`), or
  hashCollisionNode if full 32-bit hashes are equal. **Grows to arrayNode when
  n > 16.**
- **arrayNode** `{edit; count int; array [32]node}` — direct index by 5-bit chunk;
  **packs back to bitmapIndexedNode when count < 8** on `without`.
- **hashCollisionNode** `{edit; hash uint32; count int; array []any}` — linear scan
  of equal-hash keys (keys compared with `Equiv`).

Keys hash with `HashEq`; key equality inside nodes is `Equiv`. That pair is the
consistency contract from §1.3.

**Transients**: same `edit` token discipline as the vector — `ensureEditable` per
node, in-place `array[i] = v` when owned (`editAndSet` in the Java source). This is
work Glojure never did; it is required for `into`, `persistent!`, and for the
compiler to emit efficient map literals (build big literals transiently).

**PersistentArrayMap**: flat `[]any` k/v pairs with linear `Equiv` scan, promotes to
HAMT above 8 entries (`HASHTABLE_THRESHOLD/2` = 8 pairs). Map literals ≤8 entries
emit as array-maps. Trivial to implement; do it with the HAMT.

---

## 3. Symbols, keywords, vars, atoms, lazy seqs

### 3.1 Symbol — plain value, not interned

Clojure does not intern Symbol objects (only their name strings). Equality is
structural:

```go
type Symbol struct {
    meta      IPersistentMap
    ns, name  string   // ns == "" means none
    hash, hasheq uint32 // precomputed
}
// Equals: ns == ns && name == name. Comparable, cheap.
```

Use a *value* struct (or pointer with structural Equals — Glojure uses `*Symbol`);
value struct preferred so Go `==` works, with meta forcing a copy on `WithMeta`.

### 3.2 Keyword — globally interned, identity-comparable

`Keyword.java` interns via `ConcurrentHashMap<Symbol, WeakReference<Keyword>>`. In Go:

```go
type Keyword struct{ k *kwImpl }             // comparable 1-word struct
type kwImpl struct{ ns, name string; hasheq uint32 }

var kwTable sync.Map // string "ns/name" -> Keyword
func InternKeyword(ns, name string) Keyword { /* LoadOrStore */ }
```

Interned forever (no weak refs in Go; Glojure accepts the same via `go4.org/intern`
— we skip that dep). Identity: `k1 == k2` is valid Go and O(1) — critical because
**the compiler emits every keyword literal as a package-level var**
(`var kw_foo = lang.InternKeyword("", "foo")`), so runtime lookup cost is zero.
Keyword implements `IFn` (`(:k m)` → `Invoke(m)` → `ILookup.ValAt`) — the compiler
may also emit the direct `lang.Get(m, kw_foo)` form.

### 3.3 Var

```go
type Var struct {
    ns      *Namespace
    sym     Symbol
    root    atomic.Value // rebindable root (def); UnboundVar sentinel
    dynamic bool
    meta    atomic.Value // IPersistentMap
}
func (v *Var) Deref() any { /* dynamic? check binding frame; else root */ }
```

`def` emits `var v_foo = lang.InternVarName(...)` + `v_foo.BindRoot(...)` in the
package `init`; calls through vars emit `v_foo.Invoke(...)` for late binding (or a
direct Go call when the compiler proves the var is never redefined — compiler doc's
concern). **Dynamic binding** is the hard part: JVM uses ThreadLocal frames; Go has
no goroutine-locals. Glojure uses a goroutine-id registry (`internal/goid` +
global mutex map, `var.go`) — works but hacky and leaks frames if a goroutine dies
mid-binding. v0: implement the goid-registry approach behind a narrow API
(`PushThreadBindings/PopThreadBindings`) so we can swap the mechanism (e.g.
compiler-threaded context) later without touching emitted code shape.

### 3.4 Atom

`Atom.java` = `AtomicReference` + validator + watches. Go's `atomic.Value` panics on
inconsistent concrete types, so use pointer CAS (also matches Clojure's
identity-CAS semantics for `compare-and-set!`):

```go
type Atom struct {
    state    atomic.Pointer[any]
    meta     atomic.Value
    validator IFn
    watches   IPersistentMap // guarded by mu
    mu        sync.Mutex
}
func (a *Atom) Swap(f IFn, args ISeq) any {
    for {
        oldp := a.state.Load(); newv := f.Invoke(append([]any{*oldp}, seqSlice(args)...)...)
        a.validate(newv); nv := any(newv)
        if a.state.CompareAndSwap(oldp, &nv) { a.notifyWatches(*oldp, newv); return newv }
    }
}
```

### 3.5 LazySeq — realization must be goroutine-safe

`LazySeq.java`: `sval()`/`seq()` are `synchronized`; `fn` runs at most once; chains
of LazySeqs unwrap iteratively (not recursively) to avoid stack growth. Go version:

```go
type LazySeq struct {
    mu   sync.Mutex
    fn   func() any // nil once realized
    sv   any
    seq  ISeq
    meta IPersistentMap
    hash, hasheq uint32
}
```

Single mutex covering realize+unwrap (Glojure's two-mutex split in `lazyseq.go` is
correct but subtle; one lock is simpler and Java-faithful). `IsRealized()` = `fn == nil`
under lock. Post-v0 optimization: once `seq` is set, publish it via an
`atomic.Pointer[ISeq]` so realized seqs are read lock-free (mirrors Clojure 1.12's
lock-nulling). The compiler emits `(lazy-seq body)` as
`lang.NewLazySeq(func() any { return body })`.

Chunked seqs (`IChunk`, `ChunkedCons`, 32-element buffers) follow the same shapes as
Java; defer to post-v0 — everything works unchunked, just slower.

---

## 4. THE BIG DECISION: Glojure's pkg/lang vs our own

**Assessment of Glojure `pkg/lang`:**

- *Scope/quality*: ~15.8k lines; faithful ports of PHM (persistent half), lists,
  cons, lazy/chunked seqs, sorted maps, sets, subvector, full numeric tower
  (bigint/bigdec/ratio, `numbers.go` ~1k lines), vars/namespaces, atom/agent/ref/
  volatile/delay, multimethods, murmur3 hashing with tests. Genuinely good, tested,
  and its interface layout (`interfaces.go`) is exactly what we'd write.
- *Gaps/defects for our purposes*:
  - **No HAMT transients** (fake transient wraps the array-map only).
  - **Equiv ≡ Equals** (`equal.go`) — breaks Clojure `=` number-category semantics.
  - Reflection-heavy fallbacks (`GetDefault`, `Hash`, `equal.go`) tuned for its
    tree-walking interpreter's "any Go value is a Clojure value" interop; fine as
    slow path, wrong as default for AOT output.
  - Dynamic vars via goroutine-id global map (`internal/goid`).
  - Vector delegates to a vendored elvish port (`internal/persistent/vector`) —
    solid, but transient API is theirs, not Clojure-shaped.
  - `builtins.go`/`class.go`/reflect `FnFunc*` glue couple the package to its
    interpreter's Go-interop story.
- *Importability*: `pkg/lang` is importable as a module and compiles standalone, but
  its guts live in `internal/` (murmur3, goid, seq, persistent/vector) — we could
  never patch behavior without forking anyway. External deps: `go4.org/intern`,
  `bitbucket.org/pcastools/hash`, `mitchellh/hashstructure` (all droppable).
- *License*: **EPL-1.0**, same as Clojure itself. Vendoring/forking is fine with
  preserved notices; EPL is file-scoped weak copyleft — modified files stay EPL,
  the rest of our compiler is unencumbered.

**Options:**

1. *Import as dependency*: zero effort day 1, but we inherit the Equiv bug, no map
   transients, and cannot evolve interfaces the AOT compiler needs (e.g. arity-
   specialized IFn, unexported markers). Every fix requires upstream PRs. **No.**
2. *Write from scratch*: full control, but re-derives ~3–4 months of subtle,
   already-tested work (HAMT `without` packing, popTail, murmur3 coll hashing,
   numeric tower) with no differentiation. **No.**
3. **Hard fork (vendor) `pkg/lang` + its `internal/` deps into our repo — CHOSEN.**
   Copy into `pkg/lang`, keep EPL headers, flatten `internal/murmur3` in, drop
   `go4.org/intern`/`hashstructure`/`pcastools`, delete interpreter glue
   (`builtins.go`, `class.go`, reflect FnFuncs), then fix in place: split
   Equiv/Equals, add HAMT transients, native Clojure-shaped vector transients.
   We get a working, tested runtime on day 1 and full ownership of every line the
   compiler's calling convention depends on.

let-go's `pkg/vm` is the wrong donor: its boxed `Value` model and VM coupling
(`vm.go`, constpool, bytecode) contradict the `any` decision, though its HAMT
(`persistent_map.go`) and numeric promotion tables are useful cross-checks.

---

## 5. Milestone plan

**M0 — scalars + seq kernel** (unblocks reader & v0 evaluator)
- Fork/prune Glojure `pkg/lang`; compiles standalone with zero external deps.
- `any` scalars (nil, bool, int64, float64, string, Char), `IsTruthy`.
- Interfaces of §1.2; `Equiv`/`Equals` split; `HashEq` + vendored murmur3 (tests:
  hash parity against JVM Clojure for sample values).
- `Symbol`, interned `Keyword` (IFn-able), `EmptyList`, `PersistentList`, `Cons`,
  `Seq()/First()/Next()` helpers.

**M1 — collections** (unblocks map/vector literals, `assoc/conj/get/count`)
- PersistentVector (nth/cons/assocN/pop) + Clojure-shaped TransientVector.
- PersistentArrayMap + PersistentHashMap (assoc/without/find, nil key) +
  **TransientHashMap (new work)**; MapEntry.
- `RT`-style helpers the compiler emits: `Get/Assoc/Conj/Count/Nth/SeqOf`.
- Property tests vs Go maps/slices as models; hash-consistency tests.

**M2 — evaluator runtime** (unblocks defn/def/atom/laziness)
- `IFn` + compiled-fn struct (arity-switch `Invoke`), `ApplyTo`.
- `Var` + `Namespace` registry (root bindings only), `Atom`, `LazySeq`.
- Dynamic vars via goid registry behind `PushThreadBindings` API.

**M3 — completeness**
- PersistentHashSet (+transient), sorted map/set, chunked seqs, subvector,
  numeric tower (bigint/ratio/bigdec, category equiv/hash), Delay/Volatile/Reduced,
  `IReduceInit`/`IKVReduce` fast paths, realized-seq lock elision.

v0 evaluator needs exactly M0+M1: list, vector, map, symbol, keyword, string,
numbers, nil, bool.
