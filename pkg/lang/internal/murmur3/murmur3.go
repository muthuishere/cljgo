package murmur3

import (
	"math/bits"
	"unicode/utf16"

	"github.com/muthuishere/cljgo/pkg/lang/internal/seq"
)

const (
	seed = 0
	c1   = 0xcc9e2d51
	c2   = 0x1b873593
)

func HashInt(input int32) uint32 {
	if input == 0 {
		return 0
	}
	k1 := mixK1(uint32(input))
	h1 := mixH1(seed, k1)

	return fmix(h1, 4)
}

func HashLong(input int64) uint32 {
	if input == 0 {
		return 0
	}
	low := uint32(input)
	high := uint32((input >> 32) & 0xffffffff)

	k1 := mixK1(low)
	h1 := mixH1(seed, k1)

	k1 = mixK1(high)
	h1 = mixH1(h1, k1)

	return fmix(h1, 8)
}

// HashUnencodedChars is Clojure's Murmur3.hashUnencodedChars — a murmur3
// over the UTF-16 code units of a string, consumed two-at-a-time. It is
// the basis of Symbol/Keyword hasheq (clojure.lang.Symbol.hasheq feeds a
// symbol's name through it), so it must match the JVM byte-for-byte.
func HashUnencodedChars(s string) uint32 {
	units := utf16.Encode([]rune(s))
	h1 := uint32(seed)
	// step through the code units two at a time
	for i := 1; i < len(units); i += 2 {
		k1 := uint32(units[i-1]) | (uint32(units[i]) << 16)
		k1 = mixK1(k1)
		h1 = mixH1(h1, k1)
	}
	// deal with any remaining code unit
	if len(units)&1 == 1 {
		k1 := mixK1(uint32(units[len(units)-1]))
		h1 ^= k1
	}
	return fmix(h1, uint32(2*len(units)))
}

func HashOrdered(xs seq.Seq, elHash func(any) uint32) uint32 {
	var n uint32
	var hash uint32 = 1
	for ; xs != nil; xs = xs.Next() {
		eh := elHash(xs.First())
		hash = 31*hash + eh
		n++
	}
	return MixCollHash(hash, n)
}

func HashUnordered(xs seq.Seq, elHash func(any) uint32) uint32 {
	var n uint32
	var hash uint32
	for ; xs != nil; xs = xs.Next() {
		eh := elHash(xs.First())
		hash += eh
		n++
	}
	return MixCollHash(hash, n)
}

func MixCollHash(hash, count uint32) uint32 {
	h1 := uint32(seed)
	k1 := mixK1(hash)
	h1 = mixH1(h1, k1)
	return fmix(h1, count)
}

func mixK1(k1 uint32) uint32 {
	k1 *= c1
	k1 = bits.RotateLeft32(k1, 15)
	k1 *= c2
	return k1
}

func mixH1(h1, k1 uint32) uint32 {
	h1 ^= k1
	h1 = bits.RotateLeft32(h1, 13)
	h1 = h1*5 + 0xe6546b64
	return h1
}

func fmix(h1, length uint32) uint32 {
	h1 ^= length
	h1 ^= h1 >> 16
	h1 *= 0x85ebca6b
	h1 ^= h1 >> 13
	h1 *= 0xc2b2ae35
	h1 ^= h1 >> 16
	return h1
}
