package tool

import (
	"encoding/json"
	"path/filepath"
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/types"
)

func TestHMC343BusinessPopulationPublicReceiptAndNegativeCoordinates(t *testing.T) {
	path, err := filepath.Abs("../../eval/fixtures/hmosperf_marker_tree/events.systrace")
	if err != nil {
		t.Fatal(err)
	}
	ctx, _, _ := hmc17NamedPathContext(t)
	result := businessRefTestQuery(t, ctx, map[string]any{"path": path, "view": "window_stats", "pid": 700, "time_start": 5, "time_end": 5.012})
	var record types.ObservationRecord
	for _, r := range result.Observations {
		if r.Predicate == types.TraceBusinessTreePredicate {
			record = r
			break
		}
	}
	fact, ok := DecodeTraceBusinessTreeFact(record)
	if !ok || fact.Population == nil || fact.Population.Target == nil || fact.NodeCount != 4 || fact.Population.Target.NodeCount != 3 {
		t.Fatal("current public receipt lost all-owner versus selected-thread population")
	}
	text := TraceBusinessTreePopulationText(fact)
	if !strings.Contains(text, "全线程观察库存=4、原生展示=4、原生未展示=0") || !strings.Contains(text, "目标线程 TID=700 的观察库存=3、原生展示=3、原生未展示=0") {
		t.Fatal(text)
	}
	for _, mode := range []string{"scope", "total", "displayed", "target_total", "target_displayed", "target_omitted", "target_tid", "source_target", "source_scope", "target_missing", "legacy"} {
		t.Run(mode, func(t *testing.T) {
			raw, _ := json.Marshal(fact)
			var changed TraceBusinessTreeFact
			if err := json.Unmarshal(raw, &changed); err != nil {
				t.Fatal(err)
			}
			r := record
			switch mode {
			case "scope":
				changed.Population.Scope = "target_only"
			case "total":
				changed.NodeCount++
			case "displayed":
				changed.Population.DisplayedNodes--
			case "target_total":
				changed.Population.Target.NodeCount = 5
			case "target_displayed":
				changed.Population.Target.DisplayedNodes = 4
			case "target_omitted":
				changed.Population.Target.OmittedNodes++
			case "target_tid":
				changed.Population.Target.TID = 900
			case "source_target":
				r.SourceRef.QueryTargetPID = 900
			case "source_scope":
				r.SourceRef.QueryTargetScope = "process"
			case "target_missing":
				changed.Population.Target = nil
			case "legacy":
				changed.Population = nil
			}
			raw, _ = json.Marshal(changed)
			r.RichNotes = []string{types.TraceNoteKeyBusinessTreeNode + "=" + string(raw)}
			got, valid := DecodeTraceBusinessTreeFact(r)
			if valid != (mode == "legacy") {
				t.Fatalf("population/source mismatch borrowed a census: %s valid=%t", mode, valid)
			}
			if mode == "legacy" && (got.Population != nil || !strings.Contains(TraceBusinessTreePopulationText(got), "目标线程实例数未知")) {
				t.Fatal("old missing population was filled from rows or source selectors")
			}
		})
	}
}

func TestHMC343BusinessPopulationExistingThreadSelectorSpellings(t *testing.T) {
	path, err := filepath.Abs("../../eval/fixtures/hmosperf_marker_tree/events.systrace")
	if err != nil {
		t.Fatal(err)
	}
	for _, selector := range []string{"business-700", "700", "business (700)"} {
		t.Run(selector, func(t *testing.T) {
			ctx, _, _ := hmc17NamedPathContext(t)
			result := businessRefTestQuery(t, ctx, map[string]any{"path": path, "view": "window_stats", "thread": selector, "time_start": 5, "time_end": 5.012})
			count := 0
			for _, r := range result.Observations {
				if r.Predicate != types.TraceBusinessTreePredicate {
					continue
				}
				f, ok := DecodeTraceBusinessTreeFact(r)
				if !ok || f.Population == nil || f.Population.Target == nil || f.Population.Target.TID != 700 || f.Population.Target.NodeCount != 3 || r.SourceRef.QueryTargetPID != 0 || r.SourceRef.QueryTargetThread != selector {
					t.Fatalf("accepted normalized thread selector lost original query or census: %+v %+v", r.SourceRef, f.Population)
				}
				count++
			}
			if count != 4 {
				t.Fatalf("legitimate all-emitter business rows withdrawn: %d", count)
			}
		})
	}
}
