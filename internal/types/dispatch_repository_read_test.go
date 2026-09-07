package types

import (
	"encoding/json"
	"strings"
	"sync"
	"testing"
)

func TestDispatchRepositoryFileReadReceiptBindsAllAxesAndResets(t *testing.T) {
	mutable := NewMutableState("test")
	mutable.RecordDispatchRepositoryFileRead("/first", "oracle.c", "read-ref")
	for _, mismatch := range [][3]string{{"/second", "oracle.c", "read-ref"}, {"/first", "other.c", "read-ref"}, {"/first", "oracle.c", "other-ref"}} {
		if mutable.HasDispatchRepositoryFileRead(mismatch[0], mismatch[1], mismatch[2]) {
			t.Fatalf("mismatched identity authorized: %v", mismatch)
		}
	}
	if !mutable.HasDispatchRepositoryFileRead("/first", "oracle.c", "read-ref") {
		t.Fatal("exact receipt missing")
	}
	encoded, err := json.Marshal(mutable)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(encoded), "oracle.c") || strings.Contains(string(encoded), "read-ref") {
		t.Fatalf("private dispatch receipt leaked to wire: %s", encoded)
	}
	mutable.ResetDispatchToolResults()
	if mutable.HasDispatchRepositoryFileRead("/first", "oracle.c", "read-ref") {
		t.Fatal("receipt survived dispatch reset")
	}
}

func TestDispatchRepositoryFileReadReceiptConcurrentAccess(t *testing.T) {
	mutable := NewMutableState("test")
	var wg sync.WaitGroup
	for range 8 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for range 50 {
				mutable.RecordDispatchRepositoryFileRead("/first", "oracle.c", "read-ref")
				mutable.HasDispatchRepositoryFileRead("/first", "oracle.c", "read-ref")
				mutable.ResetDispatchToolResults()
			}
		}()
	}
	wg.Wait()
	mutable.RecordDispatchRepositoryFileRead("/first", "oracle.c", "read-ref")
	if !mutable.HasDispatchRepositoryFileRead("/first", "oracle.c", "read-ref") {
		t.Fatal("final exact receipt missing")
	}
}
