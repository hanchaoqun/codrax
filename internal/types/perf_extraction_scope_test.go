package types

import (
	"context"
	"encoding/json"
	"reflect"
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/attachment"
)

func TestPerfExtractionScopeSurvivesLedgerWithoutPhysicalAuthority(t *testing.T) {
	scope := PerfObservationSourceScope{ParentPreviewSHA256: "abc", ParentPreviewBytes: 400, ByteStart: 100, ByteEnd: 200, LineStart: 4, LineEnd: 8, LineCoordinates: "parent_preview"}
	bundle := &PerfBundle{Observations: []PerfObservation{{SourceScope: &scope, Authority: PerfObservationAuthorityPreTriageModelExtraction, Subject: "worker", LineStart: 4, LineEnd: 5}, {SourceScope: &scope, Authority: PerfObservationAuthorityDeterministicValidator, Kind: "time_semantics", LineStart: 4, LineEnd: 8, StartTsMs: 5000, EndTsMs: 5005}}}
	var rows []ObservationRecord
	compilePerfBundleObservations(bundle, func(r ObservationRecord) { rows = append(rows, r) })
	if len(rows) != 2 {
		t.Fatalf("rows=%+v", rows)
	}
	for _, r := range rows {
		if got := FormatObservationSourceRef(r.SourceRef, 100); !strings.Contains(got, "parent-preview lines 4-8 (not physical source lines)") {
			t.Fatalf("lost scope in model-facing source: %s", got)
		}
		if r.SourceRef.Path != "" || r.SourceRef.CaptureIdentityPath != "" || !reflect.DeepEqual(r.SourceRef.TraceExcerptScope, &scope) || r.SourceRef.ArtifactID != "attached_trace" {
			t.Fatalf("source=%+v", r.SourceRef)
		}
		encoded, err := json.Marshal(r)
		if err != nil {
			t.Fatal(err)
		}
		var restored ObservationRecord
		if err := json.Unmarshal(encoded, &restored); err != nil || !reflect.DeepEqual(restored.SourceRef, r.SourceRef) {
			t.Fatalf("roundtrip=%+v %v", restored, err)
		}
	}
	if rows[0].Role != AnswerAggregateRoleSupportingCoverage || rows[0].GroundingPolicy != ClaimGroundingSoft {
		t.Fatal("scope promoted model authority")
	}
	index := runtimeArtifactPreflightSourceIndex{byPath: map[string]runtimeArtifactPreflightSource{"capture": {path: "/capture.sys"}}, attachedTraceResolved: true, attachedTraceSource: runtimeArtifactPreflightSource{path: "/capture.sys", artifactID: "capture", artifactKind: "trace"}}
	qualified := index.requalify(rows[1])
	if qualified.SourceRef.CaptureIdentityPath != "/capture.sys" || qualified.SourceRef.Path != "" || !reflect.DeepEqual(qualified.SourceRef.TraceExcerptScope, &scope) || qualified.Span.LineStart != 4 {
		t.Fatalf("capture join changed/lost preview scope: %+v", qualified)
	}
}

func TestPerfExtractionViewIsStageLocalButReachesTool(t *testing.T) {
	parent := "a\nb\n"
	v, err := attachment.NewTraceExcerpt(context.Background(), parent, nil, 2, 4)
	if err != nil {
		t.Fatal(err)
	}
	a := &AgentContext{AttachedHitrace: parent, AttachedTraceExcerpt: v}
	b := ToolBusContext(a, AgentPerfTriager)
	if b.AttachedTraceExcerpt != v || b.AttachedHitrace != parent {
		t.Fatal("lost extraction view/parent")
	}
	if child := SubAgentContext(b, nil); child.AttachedTraceExcerpt != nil {
		t.Fatal("leaked extraction-only view to unrelated child")
	}
}
