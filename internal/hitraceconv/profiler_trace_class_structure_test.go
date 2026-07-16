package hitraceconv

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestProfilerTraceClassificationProductionThroatsAreClosed(t *testing.T) {
	files, err := filepath.Glob("*.go")
	if err != nil {
		t.Fatal(err)
	}
	enableCalls := 0
	stageCalls := 0
	for _, path := range files {
		if strings.HasSuffix(path, "_test.go") {
			continue
		}
		raw, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		text := string(raw)
		calls := strings.Count(text, ".enableProfilerTraceClassification()")
		if calls != 0 && (path != "profiler_container.go" || calls != 2) {
			t.Fatalf("unexpected production trace-class enable point: file=%s calls=%d", path, calls)
		}
		enableCalls += calls
		stageCalls += strings.Count(text, ".stageProfilerTraceClass(&row)")
	}
	if enableCalls != 2 || stageCalls != 1 {
		t.Fatalf("trace-class throat count drifted: enable=%d want=2 stage=%d want=1",
			enableCalls, stageCalls)
	}
	sorter, err := os.ReadFile("streamerdb_sorter.go")
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(sorter), "internal/tracequery") ||
		strings.Contains(string(sorter), "parseOwnedSystraceRow") {
		t.Fatal("bounded sorter reacquired trace semantic parser authority")
	}
	adapter, err := os.ReadFile("profiler_trace_class.go")
	if err != nil {
		t.Fatal(err)
	}
	if strings.Count(string(adapter), "parseOwnedSystraceRow(1, row.line)") != 1 {
		t.Fatal("trace-class adapter lost the unique held-validator-compatible parser call")
	}
}
