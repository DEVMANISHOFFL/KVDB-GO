package main

import (
	"hash/fnv"
	"math"
)

type BloomFilters struct {
	bitset []uint64
	m      uint // Total number of bits
	k      uint // Number of hash functions
}

// NewBloomFilter calculates the optimal bit size (m) and hash functions (k)
// based on the expected number of items (n) and the desired false positive rate (p).
func NewBloomFilter(expectedItems int, falsePositiveRate float64) *BloomFilters {

	// Prevent division by zero or tiny filters during testing
	n := math.Max(float64(expectedItems), 100)

	// m = - (n * log(p)) / (log(2)^2)
	m := uint(math.Ceil(-(n * math.Log(falsePositiveRate)) / math.Pow(math.Log(2.0), 2.0)))

	// k = (m / n) * log(2)
	k := uint(math.Round((float64(m) / n) * math.Log(2.0)))

	// Ensure at least 1 hash function
	if k == 0 {
		k = 1
	}

	return &BloomFilters{
		bitset: make([]uint64, (m/64)+1), // 64 bits per uint64
		m:      m,
		k:      k,
	}
}

// hash returns two 32-bit hashes from one 64-bit FNV hash
func (bf *BloomFilters) hash(key string) (uint32, uint32) {
	h := fnv.New64a()
	h.Write([]byte(key))
	sum := h.Sum64()
	return uint32(sum), uint32(sum >> 32)
}

func (bf *BloomFilters) Add(key string) {
	h1, h2 := bf.hash(key)
	for i := uint(0); i < bf.k; i++ {
		// Explicitly cast to uint64 to prevent any 32-bit wrap-around
		bitPosition := (uint64(h1) + uint64(i)*uint64(h2)) % uint64(bf.m)

		arrayIndex := bitPosition / 64
		bitOffset := bitPosition % 64

		bf.bitset[arrayIndex] |= (uint64(1) << bitOffset)
	}
}

func (bf *BloomFilters) MightContain(key string) bool {
	h1, h2 := bf.hash(key)
	for i := uint(0); i < bf.k; i++ {
		bitPosition := (uint64(h1) + uint64(i)*uint64(h2)) % uint64(bf.m)

		arrayIndex := bitPosition / 64
		bitOffset := bitPosition % 64

		// CRITICAL FIX: Explicitly cast 1 to uint64 before shifting
		if (bf.bitset[arrayIndex] & (uint64(1) << bitOffset)) == 0 {
			return false
		}
	}
	return true
}
