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

func TestProjectObservationPromptRecords_SourceInventoryDoesNotCrowdMixedOrigins(t *testing.T) {
	records := []ObservationRecord{
		{
			ID:     "evidence:current-entry",
			Origin: AnswerEvidenceOriginCurrentSource,
			Role:   AnswerAggregateRolePrincipalAnswer,
			SourceRef: ObservationSourceRef{
				Kind: ObservationSourceCurrentSource,
				Path: "internal/scheduler.go",
			},
			Span:            ObservationSpan{LineStart: 42},
			AnchorKind:      AnchorDefinition,
			EvidenceScope:   ScopeLine,
			GroundingStatus: GroundingGrounded,
			Summary:         "current scheduler entrypoint explains the diff impact",
		},
		{
			ID:      "tool:0#vcs_diff",
			Origin:  AnswerEvidenceOriginVCSDiff,
			Role:    AnswerAggregateRolePrincipalAnswer,
			Summary: "diff hunk shows scheduler hook was added",
		},
		{
			ID:      "aggregate:command",
			Origin:  AnswerEvidenceOriginCommandMeasurement,
			Role:    AnswerAggregateRolePrincipalAnswer,
			Summary: "command output counted changed files",
		},
		{
			ID:     "source_inventory:set:0",
			Origin: AnswerEvidenceOriginCurrentSource,
			Role:   AnswerAggregateRoleSupportingCoverage,
			SourceRef: ObservationSourceRef{
				Kind:      ObservationSourceCurrentSource,
				Path:      "internal",
				RowSetRef: "blob://rows/source-inventory.jsonl",
			},
			Summary: "source-inventory function count=12 complete=true",
		},
	}
	for i := 0; i < 12; i++ {
		records = append(records, ObservationRecord{
			ID:     fmt.Sprintf("source_inventory:0:%d", i),
			Origin: AnswerEvidenceOriginCurrentSource,
			Role:   AnswerAggregateRoleSupportingCoverage,
			SourceRef: ObservationSourceRef{
				Kind: ObservationSourceCurrentSource,
				Path: fmt.Sprintf("internal/pkg%d/file.go", i),
			},
			Span:            ObservationSpan{LineStart: i + 10},
			AnchorKind:      AnchorDefinition,
			EvidenceScope:   ScopeLine,
			GroundingStatus: GroundingRecovered,
			Summary:         fmt.Sprintf("mechanical source inventory row %d", i),
		})
	}

	rm := RequestModel{
		Intent: IntentExplain,
		Predicates: SemanticPredicates{
			IsHistoryLookup: true,
		},
		ChangeImpactProfile: &ChangeImpactProfile{IsChangeImpact: true},
	}
	got := ProjectObservationPromptRecords(records, &rm, nil, DefaultObservationPromptProjectionOptions(5))
	byID := map[string]ObservationPromptRecord{}
	sourceInventorySeen := 0
	for _, record := range got {
		byID[record.ID] = record
		if strings.HasPrefix(record.ID, "source_inventory:") {
			sourceInventorySeen++
		}
	}
	for _, want := range []string{"evidence:current-entry", "tool:0#vcs_diff", "aggregate:command", "source_inventory:set:0"} {
		if _, ok := byID[want]; !ok {
			t.Fatalf("mixed-origin projection lost %s; got %+v", want, got)
		}
	}
	if sourceInventorySeen > 2 {
		t.Fatalf("source-inventory advisory rows crowded the mixed-origin budget: got %d records in %+v", sourceInventorySeen, got)
	}
	if !strings.Contains(byID["source_inventory:set:0"].Source, "row_set_ref=blob://rows/source-inventory.jsonl") {
		t.Fatalf("source-inventory set record should preserve the row-set pointer: %+v", byID["source_inventory:set:0"])
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

func TestProjectObservationPromptRecords_PreservesExternalExactDetailsAcrossOrigins(t *testing.T) {
	zero := 0
	records := []ObservationRecord{
		{
			ID:         "log:observation:0",
			Origin:     AnswerEvidenceOriginRuntimeArtifact,
			Role:       AnswerAggregateRolePrincipalAnswer,
			SourceRef:  ObservationSourceRef{Kind: ObservationSourceRuntimeArtifact, ArtifactID: "attached_log", ArtifactKind: "log", PayloadRef: "blob://payload/log.txt"},
			Span:       ObservationSpan{LineStart: 3, LineEnd: 5},
			Summary:    "WARN appears in attached artifact",
			RawExcerpt: "3│ WARN retrying slow dependency",
		},
		{
			ID:          "aggregate:command",
			Origin:      AnswerEvidenceOriginCommandMeasurement,
			Role:        AnswerAggregateRolePrincipalAnswer,
			SourceRef:   ObservationSourceRef{Kind: ObservationSourceCommand, Command: "wc -l trace.log", PayloadRef: "blob://payload/wc.txt", RowSetRef: "blob://rows/wc.jsonl"},
			Span:        ObservationSpan{Row: 1},
			Value:       "70693",
			ResultCount: &zero,
			Summary:     "deterministic command count",
		},
		{
			ID:        "index:repo",
			Origin:    AnswerEvidenceOriginCrossRepoIndex,
			Role:      AnswerAggregateRoleSupportingCoverage,
			SourceRef: ObservationSourceRef{Kind: ObservationSourceCrossRepoIndex, Repo: "frameworks/base", Path: "services/core"},
			Summary:   "cross-repo index node",
		},
		{
			ID:        "doc:external",
			Origin:    AnswerEvidenceOriginExternalDocument,
			Role:      AnswerAggregateRoleSupportingCoverage,
			SourceRef: ObservationSourceRef{Kind: ObservationSourceExternalDocument, ResourceURI: "drive://doc/123", PageRef: "page=2"},
			Span:      ObservationSpan{Paragraph: 7},
			Summary:   "external design paragraph",
		},
		{
			ID:        "web:page",
			Origin:    AnswerEvidenceOriginWebPage,
			Role:      AnswerAggregateRoleSupportingCoverage,
			SourceRef: ObservationSourceRef{Kind: ObservationSourceWebPage, URL: "https://example.test/spec", FetchedAt: "2026-05-23T10:00:00Z"},
			Span:      ObservationSpan{Selector: "#api-contract"},
			Summary:   "web page section",
		},
		{
			ID:        "mcp:0",
			Origin:    AnswerEvidenceOriginMCPResource,
			Role:      AnswerAggregateRoleSupportingCoverage,
			SourceRef: ObservationSourceRef{Kind: ObservationSourceMCPResource, Server: "docs", ResourceURI: "mcp://docs/spec", RawRef: "mcp://docs/spec#v1"},
			Span:      ObservationSpan{JSONPointer: "/items/0/title"},
			Summary:   "MCP resource row",
		},
		{
			ID:        "connector:0",
			Origin:    AnswerEvidenceOriginConnectorResource,
			Role:      AnswerAggregateRoleSupportingCoverage,
			SourceRef: ObservationSourceRef{Kind: ObservationSourceConnector, Connector: "jira", ResourceURI: "jira://ISSUE-7"},
			Span:      ObservationSpan{Row: 2},
			Summary:   "connector issue row",
		},
	}
	got := ProjectObservationPromptRecords(records, nil, nil, DefaultObservationPromptProjectionOptions(len(records)))
	byID := map[string]ObservationPromptRecord{}
	for _, record := range got {
		byID[record.ID] = record
	}
	checks := map[string][]string{
		"log:observation:0": {"kind=runtime_artifact", "artifact_id=attached_log", "payload_ref=blob://payload/log.txt", "lines 3-5"},
		"aggregate:command": {"kind=command", "command=wc -l trace.log", "row_set_ref=blob://rows/wc.jsonl", "row 1", "70693"},
		"index:repo":        {"kind=cross_repo_index", "repo=frameworks/base", "path=services/core"},
		"doc:external":      {"kind=external_document", "resource_uri=drive://doc/123", "page_ref=page=2", "paragraph 7"},
		"web:page":          {"kind=web_page", "url=https://example.test/spec", "selector \"#api-contract\""},
		"mcp:0":             {"kind=mcp_resource", "server=docs", "resource_uri=mcp://docs/spec", "json \"/items/0/title\""},
		"connector:0":       {"kind=connector_resource", "connector=jira", "resource_uri=jira://ISSUE-7", "row 2"},
	}
	for id, wants := range checks {
		record, ok := byID[id]
		if !ok {
			t.Fatalf("missing projected record %s in %+v", id, got)
		}
		haystack := record.Source + " " + record.Span + " " + record.Value + " " + record.Summary + " " + record.Excerpt
		for _, want := range wants {
			if !strings.Contains(haystack, want) {
				t.Fatalf("record %s lost exact external detail %q: %+v", id, want, record)
			}
		}
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
