package lang

// Spike s73 prototype: an INTERNAL-ONLY transient HAMT used solely to
// speed up bulk construction (NewPersistentHashMap / NewMap above the
// array-map threshold). It deliberately does NOT implement the public
// ITransientMap/IEditableCollection protocol (no assoc!/dissoc!/persistent!
// exposed to Clojure code) — that would be a second, user-visible code
// path with its own semantics to get right (transient-out-of-scope
// panics, single-thread ownership checks, etc.) for a benefit that only
// matters during construction. Build cost is the only thing measured
// here, so the fix stays scoped to the one call site that needs it.
//
// Design mirrors clojure.lang.PersistentHashMap's transient nodes: each
// node gains an `edit` owner token (a unique *int32; nil means
// "persistent, copy-on-write"). assocT mutates a node's array in place
// when the node is already owned by the running transient's token,
// instead of path-copying it on every assoc like the persistent Assoc
// does. Nodes not owned by this build are cloned once (and the clone is
// tagged with the token so subsequent assocs down the same path mutate
// it directly). BitmapIndexedNode additionally over-allocates its array
// by 2*4 slots when it must grow, so up to 4 more keys can land in the
// same node without another allocation — this is the amortizing trick;
// without it every single-key growth would still allocate every time.

func newEdit() *int32 {
	e := new(int32)
	return e
}

// transientHashMapBuild is used only inside NewPersistentHashMap; it is
// never returned to callers and its token never escapes this file.
type transientHashMapBuild struct {
	edit  *int32
	root  Node
	count int
	leaf  Box // reused across assocs; avoids one *Box alloc per key
}

func newTransientHashMapBuild() *transientHashMapBuild {
	return &transientHashMapBuild{edit: newEdit()}
}

func (t *transientHashMapBuild) assoc(key, val any) {
	t.leaf.val = nil
	var newroot Node
	if t.root == nil {
		newroot = (&BitmapIndexedNode{edit: t.edit}).assocT(t.edit, 0, HashEq(key), key, val, &t.leaf)
	} else {
		newroot = t.root.assocT(t.edit, 0, HashEq(key), key, val, &t.leaf)
	}
	t.root = newroot
	if t.leaf.val != nil {
		t.count++
	}
}

func (t *transientHashMapBuild) persistent() *PersistentHashMap {
	if t.root != nil {
		t.root = finalizeHashNode(t.root)
	}
	return &PersistentHashMap{count: t.count, root: t.root}
}

// finalizeHashNode ends a node's transient lifetime: it re-slices any
// over-allocated array back to its tight, real-content length (a slice
// re-slice, not a copy — the invariant every non-transient reader relies
// on, e.g. newNodeSeq's linear scan, is "array holds exactly the live
// pairs") and clears edit so the node reads as ordinary-immutable from
// here on. Safe to mutate in place: finalizeHashNode is only ever called
// (via persistent()) on the root of a build that started from an empty
// map, so every node it reaches was created fresh by this one build —
// nothing outside it holds a reference yet. It always recurses (an
// ArrayNode created by the >=16-child conversion is never itself
// edit-tagged, but its BitmapIndexedNode children still are and still
// need trimming), and always clears edit, even where it was already nil.
func finalizeHashNode(n Node) Node {
	switch t := n.(type) {
	case *BitmapIndexedNode:
		cnt := bitCount(t.bitmap)
		t.array = t.array[:2*cnt]
		for i := 0; i < 2*cnt; i += 2 {
			if child, ok := t.array[i+1].(Node); ok {
				t.array[i+1] = finalizeHashNode(child)
			}
		}
		t.edit = nil
		return t
	case *HashCollisionNode:
		t.array = t.array[:2*t.count]
		t.edit = nil
		return t
	case *ArrayNode:
		for i, child := range t.array {
			if child != nil {
				t.array[i] = finalizeHashNode(child)
			}
		}
		t.edit = nil
		return t
	default:
		return n
	}
}

////////////////////////////////////////////////////////////////////////////////
// BitmapIndexedNode transient path

func (b *BitmapIndexedNode) ensureEditable(edit *int32) *BitmapIndexedNode {
	if b.edit == edit {
		return b
	}
	n := bitCount(b.bitmap)
	// Over-allocate by 4 kv pairs of headroom so the next few grow-inserts
	// into this node reuse the array instead of reallocating again.
	newArray := make([]any, 2*(n+4))
	copy(newArray, b.array[:2*n])
	return &BitmapIndexedNode{edit: edit, bitmap: b.bitmap, array: newArray}
}

func (b *BitmapIndexedNode) editAndSet1(edit *int32, i int, a any) *BitmapIndexedNode {
	e := b.ensureEditable(edit)
	e.array[i] = a
	return e
}

func (b *BitmapIndexedNode) editAndSet2(edit *int32, i int, a any, j int, val any) *BitmapIndexedNode {
	e := b.ensureEditable(edit)
	e.array[i] = a
	e.array[j] = val
	return e
}

func (b *BitmapIndexedNode) assocT(edit *int32, shift uint, hash uint32, key any, val any, addedLeaf *Box) Node {
	bit := bitpos(hash, shift)
	idx := b.index(bit)

	if b.bitmap&bit != 0 {
		keyOrNull := b.array[2*idx]
		valOrNode := b.array[2*idx+1]
		if node, ok := valOrNode.(Node); ok {
			n := node.assocT(edit, shift+5, hash, key, val, addedLeaf)
			if n == valOrNode {
				return b
			}
			return b.editAndSet1(edit, 2*idx+1, n)
		}
		if Equiv(key, keyOrNull) {
			if val == valOrNode {
				return b
			}
			return b.editAndSet1(edit, 2*idx+1, val)
		}
		addedLeaf.val = addedLeaf
		return b.editAndSet2(edit, 2*idx, nil, 2*idx+1,
			createNodeT(edit, shift+5, keyOrNull, valOrNode, hash, key, val))
	}

	n := bitCount(b.bitmap)
	if 2*n < len(b.array) {
		// Room in the (possibly over-allocated) array: shift the tail
		// right in place and drop the new pair in — no new allocation.
		addedLeaf.val = addedLeaf
		e := b.ensureEditable(edit)
		copy(e.array[2*(idx+1):2*(n+1)], e.array[2*idx:2*n])
		e.array[2*idx] = key
		e.array[2*idx+1] = val
		e.bitmap |= bit
		return e
	}

	if n >= 16 {
		nodes := make([]Node, 32)
		jdx := mask(hash, shift)
		nodes[jdx] = emptyIndexedNode.assocT(edit, shift+5, hash, key, val, addedLeaf)
		j := 0
		var i uint
		for i = 0; i < 32; i++ {
			if (b.bitmap>>i)&1 != 0 {
				if node, ok := b.array[j+1].(Node); ok {
					nodes[i] = node
				} else {
					nodes[i] = emptyIndexedNode.assocT(edit, shift+5, HashEq(b.array[j]), b.array[j], b.array[j+1], addedLeaf)
				}
				j += 2
			}
		}
		return &ArrayNode{count: n + 1, array: nodes}
	}

	// Grow: allocate with 4 extra kv-slot headroom (see ensureEditable).
	newArray := make([]any, 2*(n+4))
	copy(newArray, b.array[:2*idx])
	newArray[2*idx] = key
	newArray[2*idx+1] = val
	copy(newArray[2*(idx+1):2*(n+1)], b.array[2*idx:2*n])
	addedLeaf.val = addedLeaf
	return &BitmapIndexedNode{edit: edit, bitmap: b.bitmap | bit, array: newArray}
}

////////////////////////////////////////////////////////////////////////////////
// ArrayNode transient path

func (n *ArrayNode) ensureEditableArr(edit *int32) *ArrayNode {
	if n.edit == edit {
		return n
	}
	arr := make([]Node, len(n.array))
	copy(arr, n.array)
	return &ArrayNode{edit: edit, count: n.count, array: arr}
}

func (n *ArrayNode) assocT(edit *int32, shift uint, hash uint32, key any, val any, addedLeaf *Box) Node {
	idx := mask(hash, shift)
	node := n.array[idx]
	if node == nil {
		e := n.ensureEditableArr(edit)
		e.array[idx] = emptyIndexedNode.assocT(edit, shift+5, hash, key, val, addedLeaf)
		e.count++
		return e
	}
	nn := node.assocT(edit, shift+5, hash, key, val, addedLeaf)
	if nn == node {
		return n
	}
	e := n.ensureEditableArr(edit)
	e.array[idx] = nn
	return e
}

////////////////////////////////////////////////////////////////////////////////
// HashCollisionNode transient path

func (n *HashCollisionNode) ensureEditableColl(edit *int32) *HashCollisionNode {
	if n.edit == edit {
		return n
	}
	arr := make([]any, 2*(n.count+1))
	copy(arr, n.array[:2*n.count])
	return &HashCollisionNode{edit: edit, hash: n.hash, count: n.count, array: arr}
}

func (n *HashCollisionNode) assocT(edit *int32, shift uint, hash uint32, key any, val any, addedLeaf *Box) Node {
	if hash == n.hash {
		idx := n.findIndex(key)
		if idx != -1 {
			if n.array[idx+1] == val {
				return n
			}
			e := n.ensureEditableColl(edit)
			e.array[idx+1] = val
			return e
		}
		e := n.ensureEditableColl(edit)
		e.array[2*n.count] = key
		e.array[2*n.count+1] = val
		e.count++
		addedLeaf.val = addedLeaf
		return e
	}
	return (&BitmapIndexedNode{edit: edit, bitmap: bitpos(n.hash, shift), array: []any{nil, n}}).
		assocT(edit, shift, hash, key, val, addedLeaf)
}

// createNodeT is the assocT-path twin of createNode: it tags the
// freshly built subtree with the transient's edit token directly at
// creation, since nothing outside this build can be holding a reference
// to it yet.
func createNodeT(edit *int32, shift uint, key1 any, val1 any, key2hash uint32, key2 any, val2 any) Node {
	key1hash := HashEq(key1)
	if key1hash == key2hash {
		return &HashCollisionNode{
			edit:  edit,
			hash:  key1hash,
			count: 2,
			array: []any{key1, val1, key2, val2},
		}
	}
	addedLeaf := &Box{}
	return (&BitmapIndexedNode{edit: edit}).assocT(edit, shift, key1hash, key1, val1, addedLeaf).
		assocT(edit, shift, key2hash, key2, val2, addedLeaf)
}
