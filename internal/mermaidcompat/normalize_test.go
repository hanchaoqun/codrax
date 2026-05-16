package mermaidcompat

import (
	"strings"
	"testing"
)

func TestNormalizeSequenceStops_RewritesBareStop(t *testing.T) {
	in := strings.Join([]string{
		"sequenceDiagram",
		"    participant Explorer as Explorer",
		"    participant Runtime as Runtime",
		"    alt failed",
		"        Runtime-->>Explorer: Original Output",
		"        stop",
		"    end",
	}, "\n")
	got := NormalizeSequenceStops(in)
	if strings.Contains(got, "\n        stop\n") {
		t.Fatalf("bare stop survived:\n%s", got)
	}
	if !strings.Contains(got, "Note over Explorer,Runtime: stop") {
		t.Fatalf("stop note missing:\n%s", got)
	}
}

func TestNormalizeSequenceStops_LeavesNonSequenceBodiesAlone(t *testing.T) {
	in := "flowchart TD\n  stop[stop] --> B"
	if got := NormalizeSequenceStops(in); got != in {
		t.Fatalf("flowchart body changed:\n%s", got)
	}
}
