package types

import (
	"encoding/json"
	"reflect"
	"strings"
	"testing"
)

func TestHMC352ValueContextSurvivesIndependentProjectionBudgets(t *testing.T) {
	r := ObservationRecord{ID: "native:q1", Origin: AnswerEvidenceOriginRuntimeArtifact, Producer: "trace_query", ClaimKey: "cost", Predicate: "cost", Value: "0", Summary: "optional summary", RichNotes: []string{"optional note"}, SourceRef: ObservationSourceRef{Kind: ObservationSourceRuntimeArtifact, Path: "capture.trace", PayloadRef: "q1.json", QueryScopeID: "q1"}, ValueContext: []ObservationValueContext{
		{Fields: "cost", Method: "lower_bound", Detail: strings.Repeat("source-qualified ", 50) + "unknown remainder is not measured zero"},
		{Fields: "other_operand", Method: "model", Detail: "independent operand; no measured physical relation"},
	}}
	before, _ := json.Marshal(r)
	for _, opts := range []ObservationPromptProjectionOptions{DefaultObservationPromptProjectionOptions(1), SemanticReviewObservationPromptProjectionOptions(1)} {
		opts.NoteLimit, opts.PrincipalNoteLimit, opts.OriginSpecificSupportingNoteLimit, opts.NoteMaxLen, opts.SummaryMaxLen = 1, 1, 1, 1, 1
		got := ProjectObservationPromptRecords([]ObservationRecord{r}, nil, nil, opts)
		if len(got) != 1 || got[0].Value != "0" || !reflect.DeepEqual(got[0].ValueContext, r.ValueContext) {
			t.Fatal("optional preview limits changed the numeric interpretation")
		}
		got[0].ValueContext[0].Detail = "changed"
		if r.ValueContext[0].Detail == "changed" {
			t.Fatal("projection aliases source context")
		}
	}
	after, _ := json.Marshal(r)
	if string(before) != string(after) {
		t.Fatal("projection changed original record")
	}
	if text := FormatObservationValueContexts(r.ValueContext); !strings.Contains(text, "unknown remainder is not measured zero") || strings.Contains(text, "[truncated") {
		t.Fatal("formatting clipped an essential qualification")
	}
}

func TestHMC352ValueContextMergeScopeJSONAndLegacy(t *testing.T) {
	a := ObservationRecord{ID: "native:a", Origin: AnswerEvidenceOriginRuntimeArtifact, Producer: "trace_query", ClaimKey: "cost", Value: "0", SourceRef: ObservationSourceRef{Kind: ObservationSourceRuntimeArtifact, Path: "a.trace", QueryScopeID: "q1"}, ValueContext: []ObservationValueContext{{Method: "estimate", Detail: "a qualification"}}}
	b := a
	b.ID, b.ValueContext = "native:b", []ObservationValueContext{{Method: "estimate", Detail: "another precise limit"}}
	c := b
	c.ID, c.SourceRef.Path, c.SourceRef.QueryScopeID = "native:c", "b.trace", "q2"
	merged := dedupeObservationRecords([]ObservationRecord{a, b, a, c})
	if len(merged) != 2 || len(merged[0].ValueContext) != 2 || len(merged[1].ValueContext) != 1 || merged[0].Value != "0" {
		t.Fatalf("context merge crossed source/query or lost distinct limits: %+v", merged)
	}
	raw, _ := json.Marshal(merged)
	var roundtrip []ObservationRecord
	if err := json.Unmarshal(raw, &roundtrip); err != nil || !reflect.DeepEqual(merged, roundtrip) {
		t.Fatal("context lost in original JSON handoff", err)
	}
	ledger := CompileObservationLedger(ObservationLedgerInput{ToolResults: []ToolResult{{ToolName: "trace_query", Success: true, Observations: []ObservationRecord{a}}}})
	if len(ledger.Records) != 1 || !reflect.DeepEqual(ledger.Records[0].ValueContext, a.ValueContext) {
		t.Fatal("accepted tool observation lost methodology")
	}
	ledger.Records[0].ValueContext[0].Detail = "mutated"
	if a.ValueContext[0].Detail != "a qualification" {
		t.Fatal("ledger aliases tool observation")
	}
	legacy := a
	legacy.ValueContext, legacy.Summary = nil, "model unknown fallback maybe zero"
	got := ProjectObservationPromptRecords([]ObservationRecord{legacy}, nil, nil, DefaultObservationPromptProjectionOptions(1))
	if len(got) != 1 || len(got[0].ValueContext) != 0 || FormatObservationValueContexts(nil) != "" {
		t.Fatal("legacy prose was classified into producer metadata")
	}
}

// Field ownership means this metadata participates in the complete native
// core, not that every supporting calculation obtains a retention census.
func TestHMC353NativeCoreAndCopyOwnCompleteValueContext(t *testing.T) {
	r := ToolResult{ToolName: "trace_query", Success: true, Observations: []ObservationRecord{{ID: "native:q", Value: "0", ValueContext: []ObservationValueContext{{Fields: "value", Method: "estimate", Detail: "unmeasured is not measured zero"}}}}}
	before, ok := nativeFactCoreDigest(r)
	if !ok {
		t.Fatal("native encoding failed")
	}
	copy := cloneTraceBusinessSpanToolResult(r)
	copy.Observations[0].ValueContext[0].Detail = "changed qualification"
	after, ok := nativeFactCoreDigest(copy)
	if !ok || before == after {
		t.Fatal("native core did not bind complete value qualification")
	}
	unchanged, ok := nativeFactCoreDigest(r)
	if !ok || unchanged != before || r.Observations[0].ValueContext[0].Detail != "unmeasured is not measured zero" {
		t.Fatal("copy-out mutation aliased the original native qualification")
	}
}
