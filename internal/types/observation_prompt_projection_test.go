package types

import (
	"fmt"
	"strings"
	"testing"
)

func TestProjectObservationPromptRecords_DedupesSummaryFromNotes(t *testing.T) {
	resultCount := 0
	records := []ObservationRecord{{
		ID:              "aggregate:0#current_source",
		Origin:          AnswerEvidenceOriginCurrentSource,
		Role:            AnswerAggregateRolePrincipalAnswer,
		GroundingPolicy: ClaimGroundingHard,
		SourceRef:       ObservationSourceRef{Kind: ObservationSourceCurrentSource, Path: "internal/types/kind.go"},
		Span:            ObservationSpan{LineStart: 42},
		AnchorKind:      AnchorDefinition,
		EvidenceScope:   ScopeLine,
		GroundingStatus: GroundingGrounded,
		Summary:         "KindSymbolPresent 用于符号存在性判定",
		RichNotes: []string{
			"KindSymbolPresent 用于符号存在性判定",
			"KindSymbolPresent 用于符号存在性判定，检查目标符号是否能解析。",
			"KindSymbolPresent 用于符号存在性判定，检查目标符号是否能解析。",
		},
		ResultCount: &resultCount,
	}}
	got := ProjectObservationPromptRecords(records, nil, nil, DefaultObservationPromptProjectionOptions(4))
	if len(got) != 1 {
		t.Fatalf("got %d records, want 1", len(got))
	}
	if got[0].Summary != "KindSymbolPresent 用于符号存在性判定" {
		t.Fatalf("summary changed unexpectedly: %+v", got[0])
	}
	if len(got[0].Notes) != 1 || !strings.Contains(got[0].Notes[0], "检查目标符号") {
		t.Fatalf("notes should keep only non-duplicated richer detail: %+v", got[0].Notes)
	}
	if got[0].ResultCount == nil || *got[0].ResultCount != 0 {
		t.Fatalf("result_count pointer/value should survive projection: %+v", got[0].ResultCount)
	}
}

func TestProjectObservationPromptRecords_MixedOriginRankingAndBudget(t *testing.T) {
	records := []ObservationRecord{
		{
			ID:      "tool:0#vcs_diff",
			Origin:  AnswerEvidenceOriginVCSDiff,
			Role:    AnswerAggregateRolePrincipalAnswer,
			Summary: "diff hunk shows scheduler hook was added",
		},
		{
			ID:     "evidence:current",
			Origin: AnswerEvidenceOriginCurrentSource,
			Role:   AnswerAggregateRoleSupportingCoverage,
			SourceRef: ObservationSourceRef{
				Kind: ObservationSourceCurrentSource,
				Path: "internal/scheduler.go",
			},
			Span:            ObservationSpan{LineStart: 42},
			AnchorKind:      AnchorDefinition,
			EvidenceScope:   ScopeLine,
			GroundingStatus: GroundingGrounded,
			Summary:         "current scheduler entrypoint still exists",
		},
	}
	rm := RequestModel{
		Intent: IntentExplain,
		Predicates: SemanticPredicates{
			IsHistoryLookup: true,
		},
		ChangeImpactProfile: &ChangeImpactProfile{IsChangeImpact: true},
	}
	got := ProjectObservationPromptRecords(records, &rm, nil, DefaultObservationPromptProjectionOptions(2))
	if len(got) != 2 || got[0].ID != "evidence:current" || got[1].ID != "tool:0#vcs_diff" {
		t.Fatalf("projection should share mixed-origin ranking, got %+v", got)
	}
	if got[0].Span != "line 42" || !strings.Contains(got[0].Source, "internal/scheduler.go") {
		t.Fatalf("current-source projection lost source/span: %+v", got[0])
	}
}

func TestProjectObservationPromptRecords_SuppressesCurrentSourceExcerptOnly(t *testing.T) {
	records := []ObservationRecord{
		{
			ID:         "evidence:source",
			Origin:     AnswerEvidenceOriginCurrentSource,
			Role:       AnswerAggregateRolePrincipalAnswer,
			RawExcerpt: "func Run() {}",
			Summary:    "Run definition",
		},
		{
			ID:         "tool:0#vcs_metadata",
			Origin:     AnswerEvidenceOriginVCSMetadata,
			Role:       AnswerAggregateRolePrincipalAnswer,
			RawExcerpt: "commit abc123\nAdd scheduling feature\nImpact: changes startup ordering",
			Summary:    "commit abc123",
		},
	}
	got := ProjectObservationPromptRecords(records, nil, nil, DefaultObservationPromptProjectionOptions(4))
	byID := map[string]ObservationPromptRecord{}
	for _, record := range got {
		byID[record.ID] = record
	}
	if byID["evidence:source"].Excerpt != "" {
		t.Fatalf("current-source excerpts should stay out of compact observation prompt: %+v", byID["evidence:source"])
	}
	if !strings.Contains(byID["tool:0#vcs_metadata"].Excerpt, "Add scheduling feature") {
		t.Fatalf("external-origin excerpt should survive: %+v", byID["tool:0#vcs_metadata"])
	}
}

func TestProjectObservationPromptRecords_OriginSpecificPrincipalGetsRicherBudget(t *testing.T) {
	notes := make([]string, 0, 5)
	for i := 0; i < 5; i++ {
		notes = append(notes, fmt.Sprintf("第 %d 条日志观察说明", i+1))
	}
	records := []ObservationRecord{{
		ID:        "log:error:0",
		Origin:    AnswerEvidenceOriginRuntimeArtifact,
		Role:      AnswerAggregateRolePrincipalAnswer,
		Summary:   "TypeError",
		RichNotes: notes,
	}}
	got := ProjectObservationPromptRecords(records, nil, nil, SemanticReviewObservationPromptProjectionOptions(2))
	if len(got) != 1 {
		t.Fatalf("got %d records, want 1", len(got))
	}
	if len(got[0].Notes) != 4 {
		t.Fatalf("runtime principal records should keep origin-specific note budget, got %+v", got[0].Notes)
	}
}

func TestFormatObservationSpan_CoversExternalCoordinates(t *testing.T) {
	got := FormatObservationSpan(ObservationSpan{
		OldLine:     12,
		NewLine:     18,
		HunkHeader:  "@@ -12 +18 @@",
		JSONPointer: "/items/0/name",
		Row:         3,
		StartTsMs:   1.25,
		EndTsMs:     4.5,
	}, 80)
	for _, want := range []string{"old_line 12", "new_line 18", "hunk", "row 3", "1.250-4.500ms", `json "/items/0/name"`} {
		if !strings.Contains(got, want) {
			t.Fatalf("formatted span missing %q: %s", want, got)
		}
	}
}
