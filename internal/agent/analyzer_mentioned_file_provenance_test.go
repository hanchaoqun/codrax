package agent

import (
	"reflect"
	"testing"

	"github.com/hanchaoqun/codrax/internal/types"
)

func TestAnalyzerMentionedEntityCandidatesRequiredFileNeedsVerbatimRequestProvenance(t *testing.T) {
	rm := types.RequestModel{
		RawRequest: "列出 internal/types/evidence.go 中的公开字符串枚举类型",
		AnalyzerHints: types.AnalyzerHints{
			PrimaryEntities: []string{"evidence.go"},
			RequiredFileHints: []types.RequiredFileHint{
				{Path: "internal/types/evidence.go", Confidence: 0.95},
				{Path: "internal/types/context.go", Confidence: 0.79},
				{Path: "internal/types", Confidence: 0.99},
			},
		},
	}
	got := types.MentionedEntitiesFromRawRequest(rm.RawRequest, analyzerMentionedEntityCandidates(rm))
	want := []string{"evidence.go", "internal/types/evidence.go"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("mentioned candidates = %v, want exact entity plus exact required file %v", got, want)
	}
	for _, item := range got {
		if item == "internal/types/context.go" || item == "internal/types" {
			t.Fatalf("low-confidence or non-file hints must not gain request provenance: %v", got)
		}
	}
}
