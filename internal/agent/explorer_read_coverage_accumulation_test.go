package agent

import (
	"reflect"
	"testing"

	"github.com/hanchaoqun/codrax/internal/types"
)

func TestIngestExplorerReadCoverageAccumulatesAcrossDAGWindows(t *testing.T) {
	closure := types.NewEvidenceClosure("")
	ingestExplorerReadCoverage(closure, "",
		map[string]bool{"internal/analysis/gate/gate.go": true},
		map[string][]types.LineRange{"internal/analysis/gate/gate.go": {{Start: 134, End: 233}}},
		map[string]int{"internal/analysis/gate/gate.go": 518},
	)
	ingestExplorerReadCoverage(closure, "",
		map[string]bool{"internal/agent/analyzer.go": true},
		map[string][]types.LineRange{"internal/agent/analyzer.go": {{Start: 2656, End: 2675}}},
		map[string]int{"internal/agent/analyzer.go": 4443},
	)

	if got, want := explorerClosureReadSet(closure), map[string]bool{
		"internal/analysis/gate/gate.go": true,
		"internal/agent/analyzer.go":     true,
	}; !reflect.DeepEqual(got, want) {
		t.Fatalf("run-wide read set=%v, want %v", got, want)
	}
	if got := closure.ReadRanges("internal/analysis/gate/gate.go"); !reflect.DeepEqual(got, []types.LineRange{{Start: 134, End: 233}}) {
		t.Fatalf("earlier DAG window range was overwritten: %v", got)
	}
	if got := closure.ReadRanges("internal/agent/analyzer.go"); !reflect.DeepEqual(got, []types.LineRange{{Start: 2656, End: 2675}}) {
		t.Fatalf("later DAG window range missing: %v", got)
	}
	if got := closure.FileTotalLines("internal/analysis/gate/gate.go"); got != 518 {
		t.Fatalf("earlier file total=%d, want 518", got)
	}
}
