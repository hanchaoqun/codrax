package orchestrator

import (
	"encoding/json"
	"testing"

	"github.com/hanchaoqun/codrax/internal/types"
)

func TestBlockCountRepairSurvivesScoringAndRetryJSON(t *testing.T) {
	for _, n := range []int{0, 2} {
		doc := &types.AnswerDocumentV2{}
		for i := 0; i < n; i++ {
			doc.Blocks = append(doc.Blocks, summaryBlock("lead"))
		}
		vs := validateRequiredBlockCoverage(doc, minimalSummaryView())
		scored := scoreViolations(vs)
		if len(scored) != 1 || !scored[0].BlockCountRepair.Valid() {
			t.Fatalf("count repair lost: %+v", scored)
		}
		data, _ := json.Marshal(scored)
		var got []types.ScoredViolation
		if err := json.Unmarshal(data, &got); err != nil || !got[0].BlockCountRepair.Valid() {
			t.Fatal("retry JSON lost count instruction")
		}
		want := "add_blocks"
		if n == 2 {
			want = "reduce_blocks"
		}
		if got[0].BlockCountRepair.Operation != want {
			t.Fatal("under/over direction conflated")
		}
	}
}
