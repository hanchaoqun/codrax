package index

import (
	"reflect"
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
