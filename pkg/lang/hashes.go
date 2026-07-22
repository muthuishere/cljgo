package lang

import (
	"encoding/binary"
	"fmt"
	"hash"
	"hash/fnv"
	"math"
	"math/big"
	"reflect"
	"unicode/utf16"
	"unsafe"

	"github.com/muthuishere/cljgo/pkg/lang/internal/murmur3"
)

const (
	keywordHashMask = 0x7334c790
	symbolHashMask  = 0x9e3779b9

	// TODO: generic hashes for abitrary go types
	reflectTypeHashMask  = 0x49c091a8
	reflectValueHashMask = 0x49c791a8
)

func HashEq(x any) uint32 {
	if IsNil(x) {
		return 0
	}
	switch x := x.(type) {
	case IHashEq:
		return x.HashEq()
	case string:
		return murmur3.HashInt(int32(hashString(x)))
	}

	if IsNumber(x) {
		return hashNumber(x)
	}

	return Hash(x)
}

// HashOrderedColl is clojure.core/hash-ordered-coll: the ordered
// Murmur3 mix (seed 1, then MixCollHash with count) over the element
// hasheqs of a sequence. It is exactly what vectors and lists use for
// their own hasheq, exposed as a standalone fn.
func HashOrderedColl(coll any) uint32 {
	return murmur3.HashOrdered(seqToInternalSeq(Seq(coll)), HashEq)
}

// HashUnorderedColl is clojure.core/hash-unordered-coll: the
// order-independent Murmur3 mix (sum of element hasheqs, then
// MixCollHash) — what maps and sets use for their own hasheq.
func HashUnorderedColl(coll any) uint32 {
	return murmur3.HashUnordered(seqToInternalSeq(Seq(coll)), HashEq)
}

// MixCollectionHash is clojure.core/mix-collection-hash:
// Murmur3.mixCollHash(hashBasis, count).
func MixCollectionHash(hashBasis, count uint32) uint32 {
	return murmur3.MixCollHash(hashBasis, count)
}

// HashCombine is clojure.core/hash-combine == clojure.lang.Util.hashCombine
// over the two int arguments directly.
func HashCombine(seed, hash uint32) uint32 {
	return hashCombine(seed, hash)
}

func Hash(x interface{}) uint32 {
	if IsNil(x) {
		return 0
	}

	if ui32, ok := x.(uint32); ok {
		// special case for uint32
		// Java's hashCode for Integer is the int value itself
		// clojure's case relies on this when hashing a colliding hash code
		return ui32
	}

	if IsNumber(x) {
		return hashNumber(x)
	}

	switch x := x.(type) {
	case Hasher:
		return x.Hash()
	case string:
		h := fnv.New32a()
		h.Write([]byte(x))
		return h.Sum32()
	case reflect.Type:
		h := getHash()
		h.Write([]byte(x.String()))
		return h.Sum32() ^ reflectTypeHashMask
	case reflect.Value:
		if !x.IsValid() {
			return reflectValueHashMask
		}
		return Hash(x.Interface()) ^ reflectValueHashMask
	case bool:
		if x {
			return 1231 // Java's Boolean.TRUE.hashCode()
		}
		return 1237 // Java's Boolean.FALSE.hashCode()
	}

	switch reflect.TypeOf(x).Kind() {
	case reflect.Func, reflect.Chan, reflect.Pointer, reflect.UnsafePointer, reflect.Map, reflect.Slice:
		// hash of pointer
		return hashPtr(reflect.ValueOf(x).Pointer())
	case reflect.Array:
		// Hash fixed-size arrays (e.g. uuid.UUID is [16]byte) by their string representation.
		h := fnv.New32a()
		h.Write([]byte(fmt.Sprintf("%v", x)))
		return h.Sum32()
	case reflect.Struct:
		h := fnv.New32a()
		h.Write([]byte(fmt.Sprintf("%v", x)))
		return h.Sum32()
	}

	panic(fmt.Sprintf("Hash(%v [%T]) not implemented", x, x))
}

func IdentityHash(x interface{}) uint32 {
	if IsNil(x) {
		return 0
	}
	if reflect.TypeOf(x).Kind() == reflect.Ptr {
		return hashPtr(reflect.ValueOf(x).Pointer())
	}
	return Hash(x)
}

func getHash() hash.Hash32 {
	return fnv.New32a()
}

func hashOrdered(seq ISeq) uint32 {
	h := getHash()
	for ; seq != nil; seq = seq.Next() {
		h.Write(uint32ToBytes(Hash(seq.First())))
	}
	return h.Sum32()
}

func uint32ToBytes(i uint32) []byte {
	b := make([]byte, 4)
	binary.LittleEndian.PutUint32(b, i)
	return b
}

// hashString is the Java String.hashCode analog (h = 31*h + utf16unit),
// feeding HashEq's murmur3.HashInt wrapper — matching JVM Clojure's
// hasheq for strings.
// (cljgo S4 surgery: replaces mitchellh/hashstructure, which was an
// arbitrary stable hash with no JVM parity.)
func hashString(s string) uint32 {
	var h uint32
	for _, u := range utf16.Encode([]rune(s)) {
		h = 31*h + uint32(u)
	}
	return h
}

// hashInt64 is the JVM Clojure hasheq for a Long: Murmur3.hashLong(v)
// (clojure.lang.Numbers.hasheq). NOT the raw long. This is what makes
// (hash 1) == 1392991556, matching JVM 1.12.5 byte-for-byte.
func hashInt64(x int64) uint32 { return murmur3.HashLong(x) }

// hashUint64 routes unsigned Go-interop integers through the same
// Murmur3.hashLong path so that a uint64 and an int64 of equal magnitude
// (which Equiv treats as equal) still hash equal. Values above MaxInt64
// wrap on the int64 cast — a Go-interop-only edge JVM never sees, since
// on the JVM such a value would be a BigInt.
func hashUint64(x uint64) uint32 { return murmur3.HashLong(int64(x)) }

// hashFloat64 is the JVM Clojure hasheq for a Double: Double.hashCode,
// i.e. (int)(bits ^ (bits >>> 32)) over the IEEE-754 bit pattern
// (clojure.lang.Numbers.hasheq). Callers special-case 0.0/-0.0 to 0.
func hashFloat64(x float64) uint32 {
	b := math.Float64bits(x)
	return uint32(b ^ (b >> 32))
}

// hashCombine is clojure.lang.Util.hashCombine (a la boost):
// seed ^ (hash + 0x9e3779b9 + (seed << 6) + (seed >> 2)), all 32-bit,
// with an ARITHMETIC (signed) right shift on seed to mirror Java's `>>`.
func hashCombine(seed, hash uint32) uint32 {
	return seed ^ (hash + 0x9e3779b9 + (seed << 6) + uint32(int32(seed)>>2))
}

// symbolHashEq is clojure.lang.Symbol.hasheq:
// hashCombine(Murmur3.hashUnencodedChars(name), hash(ns)), where a
// missing namespace contributes hash 0 (String.hashCode("") == 0, the
// same value as JVM's Util.hash(null) for a null ns).
func symbolHashEq(ns, name string) uint32 {
	return hashCombine(murmur3.HashUnencodedChars(name), hashString(ns))
}

// javaBigIntegerHashCode reproduces java.math.BigInteger.hashCode:
// fold the big-endian 32-bit magnitude words with h = 31*h + word, then
// multiply by the sign. Clojure's hasheq for a Ratio XORs the two
// components' BigInteger.hashCode (clojure.lang.Ratio.hashCode), and a
// too-big Long/BigInt falls back to this, so it must match the JVM.
func javaBigIntegerHashCode(x *big.Int) uint32 {
	mag := x.Bytes() // big-endian magnitude, minimal length
	pad := (4 - len(mag)%4) % 4
	buf := make([]byte, pad+len(mag))
	copy(buf[pad:], mag)
	var h uint32
	for i := 0; i < len(buf); i += 4 {
		word := uint32(buf[i])<<24 | uint32(buf[i+1])<<16 | uint32(buf[i+2])<<8 | uint32(buf[i+3])
		h = 31*h + word
	}
	if x.Sign() < 0 {
		return uint32(-int32(h))
	}
	return h
}

func hashByteSlice(b []byte) uint32 {
	h := fnv.New32a()
	h.Write(b)
	return h.Sum32()
}

func hashPtr(ptr uintptr) uint32 {
	h := getHash()
	b := make([]byte, unsafe.Sizeof(ptr))
	b[0] = byte(ptr)
	b[1] = byte(ptr >> 8)
	b[2] = byte(ptr >> 16)
	b[3] = byte(ptr >> 24)
	if unsafe.Sizeof(ptr) == 8 {
		b[4] = byte(ptr >> 32)
		b[5] = byte(ptr >> 40)
		b[6] = byte(ptr >> 48)
		b[7] = byte(ptr >> 56)
	}
	h.Write(b)
	return h.Sum32()
}

func hashNumber(x any) uint32 {
	switch x := x.(type) {
	case int64:
		return hashInt64(x)
	case int:
		return hashInt64(int64(x))
	case int32:
		return hashInt64(int64(x))
	case int16:
		return hashInt64(int64(x))
	case int8:
		return hashInt64(int64(x))
	case uint64:
		return hashUint64(x)
	case uint:
		return hashUint64(uint64(x))
	case uint32:
		return hashUint64(uint64(x))
	case uint16:
		return hashUint64(uint64(x))
	case uint8:
		return hashUint64(uint64(x))
	case float64:
		if x == 0 {
			return 0
		}
		return hashFloat64(x)
	case float32:
		if x == 0 {
			return 0
		}
		// float32 widens exactly to float64, so equal float32/float64
		// values hash identically.
		return hashFloat64(float64(x))
	case *Ratio:
		// clojure.lang.Ratio.hasheq == numerator.hashCode() ^
		// denominator.hashCode() over the two BigIntegers.
		return javaBigIntegerHashCode(x.Numerator()) ^ javaBigIntegerHashCode(x.Denominator())
	case *BigInt:
		// clojure.lang.Numbers.hasheq for a BigInt: Murmur3.hashLong when
		// it fits a Long, otherwise BigInteger.hashCode.
		if x.IsInt64() {
			return hashInt64(x.Int64())
		}
		return javaBigIntegerHashCode(x.ToBigInteger())
	case *big.Int:
		if x.IsInt64() {
			return hashInt64(x.Int64())
		}
		return javaBigIntegerHashCode(x)
	case Hasher:
		return x.Hash()
	}

	panic(fmt.Sprintf("hashNumber(%v [%T]) not implemented", x, x))
}
