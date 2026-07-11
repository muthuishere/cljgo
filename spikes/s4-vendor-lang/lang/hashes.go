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

	"cljgo-spike-s4/internal/murmur3"
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

// mix64to32 is the murmur3 fmix64 finalizer folded to 32 bits — a
// stand-in for bitbucket.org/pcastools/hash's integer hashes.
// (cljgo S4 surgery: removes the pcastools dependency. The only
// invariants required are stability within a process and agreement
// across numeric categories that Equiv treats as equal; int64/uint64/
// big.Int of the same magnitude all funnel through the same function.)
func mix64to32(v uint64) uint32 {
	v ^= v >> 33
	v *= 0xff51afd7ed558ccd
	v ^= v >> 33
	v *= 0xc4ceb9fe1a85ec53
	v ^= v >> 33
	return uint32(v) ^ uint32(v>>32)
}

func hashInt64(x int64) uint32     { return mix64to32(uint64(x)) }
func hashUint64(x uint64) uint32   { return mix64to32(x) }
func hashFloat64(x float64) uint32 { return mix64to32(math.Float64bits(x)) }

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
		return hashNumber(x.Numerator()) ^ hashNumber(x.Denominator())
	case *big.Int:
		if x.IsInt64() {
			return hashNumber(x.Int64())
		}
		return hashNumber(hashByteSlice(x.Bytes()))
	case Hasher:
		return x.Hash()
	}

	panic(fmt.Sprintf("hashNumber(%v [%T]) not implemented", x, x))
}
