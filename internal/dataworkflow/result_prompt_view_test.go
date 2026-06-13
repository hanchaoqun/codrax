package dataworkflow

import (
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/dataquery"
)

func TestSampleArtifactsForPromptCompactsNestedArtifacts(t *testing.T) {
	parentSample := []string{"a", "b", "c", "d", "e"}
	childSample := []string{"c1", "c2", "c3"}
	grandchildSample := []string{"g1", "g2", "g3"}
	artifacts := []dataquery.DataArtifact{{
		ID:      "records",
		Kind:    string(dataquery.DataActionExtractRecords),
		Summary: strings.Repeat("summary ", 100),
		Sample:  parentSample,
		Fields: map[string]string{
			"json_shape": strings.Repeat("shape ", 100),
		},
		Children: []dataquery.DataArtifact{{
			ID:      "records.csv#records",
			Kind:    "extract_records/csv",
			Headers: []string{"id", "value"},
			Sample:  childSample,
			Fields: map[string]string{
				"record_completeness": "complete",
				"total_rows":          "20",
				"huge_field_catalog":  strings.Repeat("field ", 100),
			},
			Children: []dataquery.DataArtifact{{
				ID:     "records.csv#nested",
				Kind:   "diagnostic",
				Sample: grandchildSample,
				Children: []dataquery.DataArtifact{{
					ID:     "records.csv#too-deep",
					Kind:   "diagnostic",
					Sample: []string{"must not be visible"},
				}},
			}},
		}},
	}}
	got := SampleArtifactsForPrompt(artifacts, 4)
	if len(got) != 1 || len(got[0].Children) != 1 {
		t.Fatalf("got=%+v, want parent and one child", got)
	}
	if len(got[0].Sample) != 5 || len(got[0].Children[0].Sample) != 3 {
		t.Fatalf("samples parent=%v child=%v, want compacted samples", got[0].Sample, got[0].Children[0].Sample)
	}
	if len(got[0].Children[0].Children) != 1 || len(got[0].Children[0].Children[0].Children) != 0 {
		t.Fatalf("nested children=%+v, want depth-bounded artifact prompt", got[0].Children[0].Children)
	}
	if len(got[0].Fields["json_shape"]) > 260 || len(got[0].Children[0].Fields["huge_field_catalog"]) > 260 {
		t.Fatalf("fields were not clamped: parent=%q child=%q", got[0].Fields["json_shape"], got[0].Children[0].Fields["huge_field_catalog"])
	}
}

func TestInferAnswerItemCountSingleJSONFieldArray(t *testing.T) {
	got := InferAnswerItemCount(`{"ids":["u1","u3"]}`, dataquery.OutputContract{Format: dataquery.OutputJSONOnly})
	if got != 2 {
		t.Fatalf("InferAnswerItemCount=%d, want single JSON field array length", got)
	}
}

func TestBuildResultPromptViewProjectsLedgersAndCollections(t *testing.T) {
	result := dataquery.Result{
		Rows: []dataquery.RowDecision{
			{Decision: "include"},
			{Decision: "exclude"},
			{Decision: "include"},
		},
		EntityResolutions: []dataquery.EntityResolutionRecord{
			{Status: dataquery.LooseText("resolved"), SourceValue: dataquery.LooseText("a")},
			{Status: dataquery.LooseText("unresolved"), SourceValue: dataquery.LooseText("b")},
			{Status: dataquery.LooseText("resolved"), SourceValue: dataquery.LooseText("c")},
		},
		Contributions: []dataquery.ContributionRecord{
			{GroupKey: dataquery.LooseText("g1"), Metric: dataquery.LooseText("amount"), Role: dataquery.LooseText("target")},
			{GroupKey: dataquery.LooseText("g2"), Metric: dataquery.LooseText("amount"), Role: dataquery.LooseText("target")},
		},
		Artifacts: []dataquery.DataArtifact{{
			ID:          "scan/a.png",
			Kind:        "image",
			SourcePaths: []string{"scan/a.png"},
			Fields:      map[string]string{"text_evidence_paths": "evidence/a.txt"},
		}},
	}
	view := BuildCompactResultPromptView(result, 100, 100, 1, 1, 1)
	if view == nil {
		t.Fatalf("view=nil")
	}
	if len(view.EntityResolutionSamples) != 2 {
		t.Fatalf("EntityResolutionSamples=%+v, want compact bounded samples", view.EntityResolutionSamples)
	}
	projections := map[string]LedgerProjection{}
	for _, projection := range view.LedgerProjection {
		projections[projection.Kind] = projection
	}
	if projections["decision_records"].DecisionCount["include"] != 2 || projections["decision_records"].DecisionCount["exclude"] != 1 {
		t.Fatalf("decision projection=%+v", projections["decision_records"])
	}
	if projections["entity_resolutions"].StatusCounts["resolved"] != 2 || projections["entity_resolutions"].StatusCounts["unresolved"] != 1 {
		t.Fatalf("entity projection=%+v", projections["entity_resolutions"])
	}
	if projections["contributions"].Count != 2 || !containsString(projections["contributions"].GroupKeys, "g1/amount") {
		t.Fatalf("contribution projection=%+v", projections["contributions"])
	}
	if len(view.MaterialSetHandles) != 1 || view.MaterialSetHandles[0].Kind != "related_text_evidence" {
		t.Fatalf("MaterialSetHandles=%+v, want related text evidence handle", view.MaterialSetHandles)
	}
}
