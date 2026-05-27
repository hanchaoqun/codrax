package index

import (
	"errors"
	"reflect"
	"runtime/debug"
	"testing"
	"time"
)

func TestParseJobOrderLargeFilesFirst(t *testing.T) {
	entries := []FileEntry{
		{RelPath: "small.go", Size: 10},
		{RelPath: "huge.c", Size: 10 << 20},
		{RelPath: "medium.py", Size: 1024},
	}
	order := parseJobOrder(entries)
	var got []string
	for _, idx := range order {
		got = append(got, entries[idx].RelPath)
	}
	want := []string{"huge.c", "medium.py", "small.go"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("parse job order = %v, want %v", got, want)
	}
}

func TestParseChannelBufferSizeIsWorkerBounded(t *testing.T) {
	if got := parseChannelBufferSize(1000, 8); got != 16 {
		t.Fatalf("buffer = %d, want workers*2", got)
	}
	if got := parseChannelBufferSize(3, 8); got != 3 {
		t.Fatalf("small total buffer = %d, want total", got)
	}
	if got := parseChannelBufferSize(10, 0); got != 2 {
		t.Fatalf("zero-worker buffer = %d, want one worker worth", got)
	}
}

func TestParseWorkerBudgetHonorsSoftMemoryLimit(t *testing.T) {
	prevLimit := debug.SetMemoryLimit(1 << 30)
	defer debug.SetMemoryLimit(prevLimit)
	prevBase, prevReserve := baseGOMAXPROCS, scanReserveCPUs
	defer func() {
		baseGOMAXPROCS = prevBase
		scanReserveCPUs = prevReserve
	}()
	baseGOMAXPROCS = 16
	scanReserveCPUs = 0
	entries := make([]FileEntry, 100)
	if got := parseWorkerBudget(entries); got != 2 {
		t.Fatalf("workers = %d, want 2 from 1GiB/512MiB soft limit", got)
	}
}

func TestParseWorkerBudgetKeepsAtLeastOneWorker(t *testing.T) {
	prevLimit := debug.SetMemoryLimit(64 << 20)
	defer debug.SetMemoryLimit(prevLimit)
	prevBase, prevReserve := baseGOMAXPROCS, scanReserveCPUs
	defer func() {
		baseGOMAXPROCS = prevBase
		scanReserveCPUs = prevReserve
	}()
	baseGOMAXPROCS = 8
	scanReserveCPUs = 0
	entries := make([]FileEntry, 100)
	if got := parseWorkerBudget(entries); got != 1 {
		t.Fatalf("workers = %d, want floor of 1 under tiny soft limit", got)
	}
}

func TestTreeSitterParseTimeoutCanBeConfigured(t *testing.T) {
	prev := treeSitterParseTimeout
	defer SetTreeSitterParseTimeout(prev)
	SetTreeSitterParseTimeout(17 * time.Second)
	if got := treeSitterParseTimeout; got != 17*time.Second {
		t.Fatalf("treeSitterParseTimeout = %s, want 17s", got)
	}
	SetTreeSitterParseTimeout(0)
	if got := treeSitterParseTimeout; got != 0 {
		t.Fatalf("treeSitterParseTimeout = %s, want disabled", got)
	}
	SetTreeSitterParseTimeout(-1)
	if got := treeSitterParseTimeout; got != defaultTreeSitterParseTimeout {
		t.Fatalf("treeSitterParseTimeout = %s, want default %s", got, defaultTreeSitterParseTimeout)
	}
}

func TestParseTreeSitterTimeoutReturnsTypedError(t *testing.T) {
	prev := treeSitterParseTimeout
	defer SetTreeSitterParseTimeout(prev)
	SetTreeSitterParseTimeout(1 * time.Nanosecond)
	// Use enough source text that the deadline is already expired by the time
	// tree-sitter checks it on ordinary machines. If a very fast parser wins
	// the race, the safety-valve contract is still covered by the setter test.
	src := []byte("package p\n")
	for i := 0; i < 10000; i++ {
		src = append(src, []byte("func f() {}\n")...)
	}
	_, _, err := parseTreeSitterRoot("go", src)
	if err == nil {
		t.Skip("tree-sitter completed before the 1ns deadline on this host")
	}
	if !errors.Is(err, errTreeSitterParseTimeout) {
		t.Fatalf("parse error = %v, want errTreeSitterParseTimeout", err)
	}
}

func TestActiveFileHeartbeatSkipsSmallFiles(t *testing.T) {
	called := false
	stop := startActiveFileHeartbeat(FileEntry{RelPath: "small.go", Size: 1024}, func(FileEntry) {
		called = true
	})
	stop()
	if called {
		t.Fatal("small files should not emit active-file heartbeat")
	}
}

func TestActiveFileHeartbeatReportsLargeFile(t *testing.T) {
	ch := make(chan string, 1)
	stop := startActiveFileHeartbeat(FileEntry{RelPath: "large.c", Size: 2 << 20}, func(e FileEntry) {
		select {
		case ch <- e.RelPath:
		default:
		}
	})
	defer stop()
	select {
	case got := <-ch:
		if got != "large.c" {
			t.Fatalf("active file = %q, want large.c", got)
		}
	case <-time.After(time.Second):
		t.Fatal("large file did not emit initial active-file notice")
	}
}
