package main

import (
	"fmt"
	"testing"
)

// Test 1: Basic Functionality
func TestBloomFilter_Basic(t *testing.T) {
	bf := NewBloomFilter(100, 0.01) // 100 items, 1% false positive rate

	bf.Add("apple")
	bf.Add("banana")

	// 1. These MUST exist (No false negatives allowed)
	if !bf.MightContain("apple") {
		t.Error("Expected filter to contain 'apple', but it said no!")
	}
	if !bf.MightContain("banana") {
		t.Error("Expected filter to contain 'banana', but it said no!")
	}

	// 2. This should ideally return false (unless we get very unlucky)
	if bf.MightContain("grape") {
		t.Error("Expected filter to NOT contain 'grape'. (Note: Technically a false positive is possible, but highly unlikely with only 2 items in a 100-capacity filter).")
	}
}

// Test 2: Stress Test & False Positive Rate Verification
func TestBloomFilter_FalsePositiveRate(t *testing.T) {
	expectedItems := 10_000
	targetFPRate := 0.01 // 1%

	bf := NewBloomFilter(expectedItems, targetFPRate)

	// 1. Insert 10,000 unique keys
	for i := 0; i < expectedItems; i++ {
		key := fmt.Sprintf("valid-key-%d", i)
		bf.Add(key)
	}

	// 2. Verify all 10,000 keys return TRUE (Zero false negatives)
	for i := 0; i < expectedItems; i++ {
		key := fmt.Sprintf("valid-key-%d", i)
		if !bf.MightContain(key) {
			t.Fatalf("FATAL: False negative detected for '%s'! Bloom filters must never lose data.", key)
		}
	}

	// 3. Test 10,000 completely different keys to calculate our False Positive Rate
	falsePositives := 0
	for i := 0; i < expectedItems; i++ {
		key := fmt.Sprintf("invalid-key-%d", i)
		if bf.MightContain(key) {
			falsePositives++
		}
	}

	// 4. Calculate and evaluate the result
	actualFPRate := float64(falsePositives) / float64(expectedItems)

	// We allow a tiny buffer (0.005) for statistical variance
	if actualFPRate > targetFPRate+0.005 {
		t.Errorf("Math failed! False positive rate too high. Target: ~%f, Actual: %f", targetFPRate, actualFPRate)
	} else {
		t.Logf("Success! Target FPR: %.4f | Actual FPR: %.4f (Total False Positives: %d)", targetFPRate, actualFPRate, falsePositives)
	}
}
