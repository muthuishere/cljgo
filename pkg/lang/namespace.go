package lang

import (
	"errors"
	"fmt"
	"os"
	"reflect"
	"strings"
	"sync"
	"sync/atomic"
)

type Namespace struct {
	name *Symbol

	// atomic references to maps
	mappings atomic.Value
	aliases  atomic.Value

	meta IPersistentMap
}

var (
	SymbolCoreNamespace = NewSymbol("clojure.core")

	namespaces = map[string]*Namespace{}
	nsMtx      sync.RWMutex
)

func AllNamespaces() ISeq {
	nsMtx.RLock()
	defer nsMtx.RUnlock()
	ns := make([]*Namespace, 0, len(namespaces))
	for _, n := range namespaces {
		ns = append(ns, n)
	}
	return Seq(ns)
}

func FindNamespace(sym *Symbol) *Namespace {
	nsMtx.RLock()
	defer nsMtx.RUnlock()
	return namespaces[sym.String()]
}

func FindOrCreateNamespace(sym *Symbol) *Namespace {
	ns := FindNamespace(sym)
	if ns != nil {
		return ns
	}
	nsMtx.Lock()
	defer nsMtx.Unlock()
	ns = namespaces[sym.String()]
	if ns != nil {
		return ns
	}
	ns = NewNamespace(sym)
	namespaces[sym.String()] = ns
	return ns
}

// RemoveNamespace removes the named namespace from the registry,
// returning it (nil when no such namespace existed) — the return value
// clojure.core/remove-ns needs. The refusal message matches the JVM
// oracle: (remove-ns 'clojure.core) => "Cannot remove clojure namespace"
// (Clojure 1.12.5; cljgo batch A4 surgery, see PROVENANCE.md).
func RemoveNamespace(sym *Symbol) *Namespace {
	if sym.String() == "clojure.core" {
		panic(errors.New("Cannot remove clojure namespace"))
	}

	nsMtx.Lock()
	defer nsMtx.Unlock()
	ns := namespaces[sym.String()]
	delete(namespaces, sym.String())
	return ns
}

func NamespaceFor(inns *Namespace, sym *Symbol) *Namespace {
	//note, presumes non-nil sym.ns
	// first check against currentNS' aliases...
	nsSym := NewSymbol(sym.Namespace())
	ns := inns.LookupAlias(nsSym)
	if ns != nil {
		return ns
	}

	return FindNamespace(nsSym)
}

func NewNamespace(name *Symbol) *Namespace {
	ns := &Namespace{
		name: name,
	}

	ns.mappings.Store(NewBox(seedHostClassImports(emptyMap)))
	ns.aliases.Store(NewBox(emptyMap))

	return ns
}

// seedHostClassImports is a no-op in the severed runtime: there is no
// pkgmap host-class registry. The AOT compiler resolves Go symbols
// statically; nothing needs to be pre-imported into namespaces.
// (cljgo S4 surgery: pkgmap dependency removed.)
func seedHostClassImports(m IPersistentMap) IPersistentMap {
	return m
}

func (ns *Namespace) String() string {
	return ns.Name().String()
}

func (ns *Namespace) Name() *Symbol {
	return ns.name
}

func (ns *Namespace) mappingsBox() *Box {
	return ns.mappings.Load().(*Box)
}

func (ns *Namespace) Mappings() IPersistentMap {
	return ns.mappingsBox().val.(IPersistentMap)
}

// CompareAndSetMappings swaps this namespace's whole mapping table from
// old to new in one CAS, returning false (installing nothing) when the
// table has moved past `old` — callers must then take the per-symbol
// reference path. (cljgo boot-refer surgery, see PROVENANCE.md: boot
// refers all of clojure.core into each fresh lib namespace; overlaying the
// namespace's few own vars onto one prebuilt, structurally shared snapshot
// beats ~900 per-symbol path-copying Assocs per namespace.)
func (ns *Namespace) CompareAndSetMappings(old, new IPersistentMap) bool {
	mb := ns.mappingsBox()
	if mb.val.(IPersistentMap) != old {
		return false
	}
	return ns.mappings.CompareAndSwap(mb, NewBox(new))
}

// OwnsInternedVar reports whether v is a Var interned in this namespace
// under sym — the isInternedMapping test, exported for the boot-refer fast
// path (cljgo surgery, see PROVENANCE.md).
func (ns *Namespace) OwnsInternedVar(sym *Symbol, v interface{}) bool {
	return ns.isInternedMapping(sym, v)
}

// Unmap removes sym's mapping from this namespace (clojure.core/
// ns-unmap; cljgo batch A4 surgery, see PROVENANCE.md). It retries a CAS
// on the SAME mappings Box every other mutation (Intern/reference/
// CompareAndSetMappings) swings, so it composes with the boot-refer
// bulk path: a concurrent CompareAndSetMappings either lands first (and
// this unmap then removes from the installed snapshot) or observes a
// moved table, returns false, and its caller takes the per-symbol
// reference path — a completed unmap is never silently resurrected by a
// stale whole-table install. Unmapping a name with no mapping is a
// no-op, as on the JVM.
func (ns *Namespace) Unmap(sym *Symbol) {
	if sym.Namespace() != "" {
		// JVM oracle (1.12.5): (ns-unmap *ns* 'clojure.core/map) =>
		// IllegalArgumentException "Can't unintern namespace-qualified
		// symbol".
		panic(errors.New("Can't unintern namespace-qualified symbol"))
	}
	mb := ns.mappingsBox()
	for mb.val.(IPersistentMap).ContainsKey(sym) {
		newMap := mb.val.(IPersistentMap).Without(sym)
		ns.mappings.CompareAndSwap(mb, NewBox(newMap))
		mb = ns.mappingsBox()
	}
}

// RemoveAlias removes an alias from this namespace (clojure.core/
// ns-unalias; cljgo batch A4 surgery, see PROVENANCE.md). Same CAS-retry
// discipline as AddAlias; removing an absent alias is a no-op (JVM
// parity: Namespace.removeAlias dissocs without checking presence).
func (ns *Namespace) RemoveAlias(alias *Symbol) {
	ab := ns.aliasesBox()
	for ab.val.(IPersistentMap).ContainsKey(alias) {
		newAliases := ab.val.(IPersistentMap).Without(alias)
		ns.aliases.CompareAndSwap(ab, NewBox(newAliases))
		ab = ns.aliasesBox()
	}
}

func (ns *Namespace) aliasesBox() *Box {
	return ns.aliases.Load().(*Box)
}

func (ns *Namespace) Aliases() IPersistentMap {
	return ns.aliasesBox().val.(IPersistentMap)
}

func (ns *Namespace) isInternedMapping(sym *Symbol, v interface{}) bool {
	vr, ok := v.(*Var)
	return ok && vr.Namespace() == ns && Equals(vr.Symbol(), sym)
}

// Intern creates a new Var in this namespace with the given name.
func (ns *Namespace) Intern(sym *Symbol) *Var {
	if sym.Namespace() != "" {
		panic(fmt.Errorf("can't intern qualified name: %s", sym))
	}
	mb := ns.mappingsBox()

	var v *Var
	var o interface{}
	for {
		o = mb.val.(IPersistentMap).ValAt(sym)
		if o != nil {
			break
		}

		if v == nil {
			v = NewVar(ns, sym)
		}
		newMap := mb.val.(IPersistentMap).Assoc(sym, v)
		ns.mappings.CompareAndSwap(mb, NewBox(newMap))
		mb = ns.mappingsBox()
	}
	if ns.isInternedMapping(sym, o) {
		return o.(*Var)
	}
	if v == nil {
		v = NewVar(ns, sym)
	}
	if ns.checkReplacement(sym, o, v) {
		for !ns.mappings.CompareAndSwap(mb, NewBox(mb.val.(IPersistentMap).Assoc(sym, v))) {
			mb = ns.mappingsBox()
		}
		return v
	}

	return o.(*Var)
}

func (ns *Namespace) checkReplacement(sym *Symbol, old, neu interface{}) bool {
	/*
		 This method checks if a namespace's mapping is applicable and warns on problematic cases.
		 It will return a boolean indicating if a mapping is replaceable.
		 The semantics of what constitutes a legal replacement mapping is summarized as follows:

		| classification | in namespace ns        | newval = anything other than ns/name | newval = ns/name                    |
		|----------------+------------------------+--------------------------------------+-------------------------------------|
		| native mapping | name -> ns/name        | no replace, warn-if newval not-core  | no replace, warn-if newval not-core |
		| alias mapping  | name -> other/whatever | warn + replace                       | warn + replace                      |
	*/

	// cljgo S4 surgery: was GlobalEnv.Stderr(); Environment (interpreter
	// glue) is deleted, warnings go to the process stderr.
	var errOut = os.Stderr

	if _, ok := old.(*Var); ok {
		var nns *Namespace
		if neuVar, ok := neu.(*Var); ok {
			nns = neuVar.Namespace()
		}
		if ns.isInternedMapping(sym, old) {
			if nns != FindNamespace(SymbolCoreNamespace) {
				fmt.Fprintf(errOut, "REJECTED: attempt to replace interned var %s with %s in %s, you must ns-unmap first\n", old, neu, ns.name)
			}
			return false
		}
	}

	fmt.Fprintf(errOut, "WARNING: %s already refers to %s in namespace: %s, being replaced by: %s\n", sym, old, ns.name, neu)
	return true
}

func (ns *Namespace) InternWithValue(sym *Symbol, value interface{}, replaceRoot bool) *Var {
	v := ns.Intern(sym)
	if !v.HasRoot() || replaceRoot {
		v.BindRoot(value)
	}
	return v
}

func (ns *Namespace) GetMapping(sym *Symbol) interface{} {
	m := ns.Mappings()
	return m.ValAt(sym)
}

func (ns *Namespace) FindInternedVar(sym *Symbol) *Var {
	m := ns.Mappings()
	v := m.ValAt(sym)
	if v == nil {
		return nil
	}
	vr, ok := v.(*Var)
	if !ok {
		return nil
	}
	if vr.Namespace() != ns {
		return nil
	}
	return vr
}

func (ns *Namespace) LookupAlias(sym *Symbol) *Namespace {
	m := ns.Aliases()
	v := m.ValAt(sym)
	if v == nil {
		return nil
	}
	return v.(*Namespace)
}

func (ns *Namespace) AddAlias(alias *Symbol, ns2 *Namespace) {
	if alias == nil || ns2 == nil {
		panic(fmt.Errorf("add-alias: expecting symbol (%v) + namespace (%v)", alias, ns2))
	}
	ab := ns.aliasesBox()
	for !ab.val.(IPersistentMap).ContainsKey(alias) {
		newAliases := ab.val.(IPersistentMap).Assoc(alias, ns2)
		ns.aliases.CompareAndSwap(ab, NewBox(newAliases))
		ab = ns.aliasesBox()
	}
	if v := ab.val.(IPersistentMap).ValAt(alias); v != ns2 {
		panic(fmt.Errorf("add-alias: alias %s already refers to %s", alias, v))
	}
}

// Import references an export from a Go package. The export is a
// fully-qualified name ("pkg/path.Name"); the mapping key is the part
// after the last dot. (cljgo S4 surgery: inlined pkgmap.SplitExport.)
func (ns *Namespace) Import(export string, v interface{}) interface{} {
	name := export
	if i := strings.LastIndex(export, "."); i != -1 {
		name = export[i+1:]
	}
	ns.reference(NewSymbol(name), v)
	return v
}

// Refer adds a reference to an existing Var, possibly in another
// namespace, to this namespace.
func (ns *Namespace) Refer(sym *Symbol, v *Var) *Var {
	return ns.reference(sym, v).(*Var)
}

func (ns *Namespace) reference(sym *Symbol, v interface{}) interface{} {
	if sym.Namespace() != "" {
		panic(fmt.Errorf("can't intern qualified name: %s", sym))
	}
	if v == nil {
		panic(fmt.Errorf("can't refer to nil (%s)", sym))
	}

	mb := ns.mappingsBox()
	var o interface{}
	for {
		o = mb.val.(IPersistentMap).ValAt(sym)
		if o != nil {
			break
		}
		newMap := mb.val.(IPersistentMap).Assoc(sym, v)
		ns.mappings.CompareAndSwap(mb, NewBox(newMap))
		mb = ns.mappingsBox()
	}
	if ns.isInternedMapping(sym, o) {
		return o.(*Var)
	}

	// NB: in Go, some types are not comparable.
	oCmp := reflect.TypeOf(o).Comparable()
	vCmp := reflect.TypeOf(v).Comparable()
	if oCmp && vCmp {
		if o == v {
			return o
		}
	} else if oCmp == vCmp {
		// TODO: what to do here? for now, assume equal
		return o
	}

	if ns.checkReplacement(sym, o, v) {
		for !ns.mappings.CompareAndSwap(mb, NewBox(mb.val.(IPersistentMap).Assoc(sym, v))) {
			mb = ns.mappingsBox()
		}
		return v
	}

	return o
}

func (ns *Namespace) Meta() IPersistentMap {
	return ns.meta
}

func (ns *Namespace) AlterMeta(alter IFn, args ISeq) IPersistentMap {
	meta := alter.ApplyTo(NewCons(ns.Meta(), args)).(IPersistentMap)
	ns.ResetMeta(meta)
	return meta
}

func (ns *Namespace) ResetMeta(meta IPersistentMap) IPersistentMap {
	ns.meta = meta
	return meta
}
