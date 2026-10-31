package types

import (
	"fmt"
	"strings"
	"sync"
	"testing"
)

// Tests only the fixed storage policy after publication validation, not a
// synthetic producer-authority path. Public native admission has separate
// real TraceQuery tests. Neither production cap is reduced for this test.
func TestHMC325NativeRetentionEncodedBudgetIsAtomic(t *testing.T) {
	var pool nativeFactRetentionPool
	whole := &retainedNativeFact{key: "complete", encoded: strings.Repeat("x", nativeFactRetentionByteLimit), queries: 1}
	if stored, omitted := pool.admit(whole); omitted || stored != whole || pool.bytes != nativeFactRetentionByteLimit {
		t.Fatal("exact encoded boundary did not retain the whole immutable core")
	}
	if stored, omitted := pool.admit(&retainedNativeFact{key: "next", encoded: "y", queries: 1}); !omitted || stored != nil || len(pool.entries) != 1 {
		t.Fatal("overflow was clipped, replaced an original, or retained outside the fixed budget")
	}
	if stored, omitted := pool.admit(whole); omitted || stored != whole || pool.queries != 1 {
		t.Fatal("exact duplicate at capacity consumed budget or became an omission")
	}
}

func TestHMC325NativeRetentionConcurrentPoolCapacity(t *testing.T) {
	var pool nativeFactRetentionPool
	var workers sync.WaitGroup
	for i := 0; i < nativeFactRetentionQueryLimit*2; i++ {
		workers.Add(1)
		go func(i int) {
			defer workers.Done()
			pool.admit(&retainedNativeFact{key: fmt.Sprint(i), encoded: "whole", queries: 1})
		}(i)
	}
	workers.Wait()
	if pool.queries != nativeFactRetentionQueryLimit || len(pool.entries) != nativeFactRetentionQueryLimit || pool.bytes != nativeFactRetentionQueryLimit*len("whole") {
		t.Fatalf("shared capacity raced: queries=%d entries=%d bytes=%d", pool.queries, len(pool.entries), pool.bytes)
	}
}
