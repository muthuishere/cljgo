package corelib

import (
	"testing"

	"github.com/muthuishere/cljgo/pkg/lang"
)

// ADR 0039's honesty contract: every runtime interface a record's
// ancestry claims (typeSupers in protocols.go — clojure.lang.Associative,
// IPersistentMap, IPersistentCollection, Counted, Seqable, IObj, IMeta)
// must be GENUINELY implemented by pkg/lang's *Record. These compile-time
// assertions fail the build if instance.go ever stops implementing one —
// the ancestry table cannot silently drift into fabrication.
var (
	_ lang.Associative           = (*lang.Record)(nil)
	_ lang.IPersistentMap        = (*lang.Record)(nil)
	_ lang.IPersistentCollection = (*lang.Record)(nil)
	_ lang.Counted               = (*lang.Record)(nil)
	_ lang.Seqable               = (*lang.Record)(nil)
	_ lang.IObj                  = (*lang.Record)(nil)
	_ lang.IMeta                 = (*lang.Record)(nil)
)

// TestClassRefSupers pins ADR 0039's class-ref ancestry: concrete
// well-known classes report exactly the flattened Object super; Object
// itself and interface names report none.
func TestClassRefSupers(t *testing.T) {
	object := lookupClassRef("Object")
	if object == nil {
		t.Fatal("Object not in the class-ref table")
	}
	for _, name := range []string{"String", "clojure.lang.PersistentHashSet", "clojure.lang.Keyword", "java.util.UUID"} {
		c := lookupClassRef(name)
		if c == nil {
			t.Fatalf("%s not in the class-ref table", name)
		}
		s := classRefSupers(c)
		if len(s) != 1 || s[0] != object {
			t.Errorf("classRefSupers(%s) = %v, want [Object]", name, s)
		}
	}
	for _, name := range []string{"Object", "clojure.lang.Associative", "clojure.lang.ISeq", "Comparable", "CharSequence"} {
		c := lookupClassRef(name)
		if c == nil {
			t.Fatalf("%s not in the class-ref table", name)
		}
		if s := classRefSupers(c); s != nil {
			t.Errorf("classRefSupers(%s) = %v, want nil", name, s)
		}
	}
}

// TestTypeClassVarFailClosed pins the dotted-name fallback's fail-closed
// edges: no namespace, no var, or a var bound to a non-class value must
// not resolve.
func TestTypeClassVarFailClosed(t *testing.T) {
	if v := typeClassVar(lang.NewSymbol("no.such_ns.Thing")); v != nil {
		t.Errorf("unknown namespace resolved: %v", v)
	}
	if v := typeClassVar(lang.NewSymbol("NoDotsHere")); v != nil {
		t.Errorf("dotless symbol resolved: %v", v)
	}
	// `inc` is interned and bound to a fn — not a class.
	RegisterAll()
	if v := typeClassVar(lang.NewSymbol("clojure.core.inc")); v != nil {
		t.Errorf("var bound to a non-class value resolved: %v", v)
	}
}
