package types

import (
	"strings"
	"testing"
)

func TestLogArtifactFrameOnlyProjectionKeepsPartialStacksWithoutErrorAuthority(t *testing.T) {
	for _, tc := range []struct {
		name  string
		frame LogFrame
		want  string
	}{
		{"raw", LogFrame{Raw: "captured.fn(0x0)", Func: "not-preferred"}, "captured.fn(0x0)"},
		{"function", LogFrame{Func: "external_frame"}, "external_frame"},
		{"file_line", LogFrame{File: "deployed/main.go", Line: 17}, "deployed/main.go:17"},
		{"file", LogFrame{File: "deployed/main.go"}, "deployed/main.go"},
		{"empty", LogFrame{}, ""},
		{"line_is_not_identity", LogFrame{Line: 17}, ""},
	} {
		t.Run(tc.name, func(t *testing.T) {
			bundle := &LogBundle{Errors: []LogError{{Type: "UnverifiedLabel", Frames: []LogFrame{tc.frame}}}}
			ledger := CompileObservationLedger(ObservationLedgerInput{LogBundle: bundle})
			bindings := CompileRuntimeArtifactClaimBindings(&RequestModel{LogTriage: bundle}, nil)
			if tc.want == "" {
				if len(ledger.Records) != 0 || len(bindings) != 0 {
					t.Fatalf("empty fields minted a fact: %+v / %+v", ledger.Records, bindings)
				}
				return
			}
			if len(ledger.Records) != 1 || len(bindings) != 1 {
				t.Fatalf("partial frame lost: %+v / %+v", ledger.Records, bindings)
			}
			row := ledger.Records[0]
			if row.Subject != tc.want || row.Predicate != "stack_frame" || row.ProvenanceLane != ObservationProvenanceArtifactSpan || row.ClaimAuthority != ObservationClaimAuthorityDirectObservation || bindings[0].TargetRef != tc.want || len(bindings[0].SupportRefs) != 1 {
				t.Fatalf("wrong frame boundary: %+v / %+v", row, bindings)
			}
			if row.Origin != AnswerEvidenceOriginRuntimeArtifact || ObservationRecordHasCurrentSourceLineSpan(row) || AnswerClaimBindingAuthorityCeiling(bindings[0]) != AuthorityHistorical {
				t.Fatalf("frame gained current-source authority: %+v / %+v", row, bindings)
			}
		})
	}
}

func TestLogArtifactFrameOnlyNestedAndBindingCannotBorrowHeaderAuthority(t *testing.T) {
	bound := &LogBundle{Errors: []LogError{{Type: "NativeHeader"}}}
	bindTestLogTypeLiterals(t, bound)
	stale := bound.Errors[0]
	stale.Type = "ChangedLabel"
	stale.Frames = []LogFrame{{Raw: "business.load()"}, {Func: "business.allocate"}, {File: "app.cj", Line: 8}}
	bundle := &LogBundle{Errors: []LogError{{Type: "outer guess", Cause: &stale, CauseRelation: &LogCauseRelation{Authority: LogCauseAuthorityExplicitArtifactMarker, Marker: "Caused by: unknown"}}}}
	ledger := CompileObservationLedger(ObservationLedgerInput{LogBundle: bundle})
	bindings := CompileRuntimeArtifactClaimBindings(&RequestModel{LogTriage: bundle}, nil)
	if len(ledger.Records) != 3 || len(bindings) != 3 {
		t.Fatalf("full business stack support lost: %+v / %+v", ledger.Records, bindings)
	}
	for _, row := range ledger.Records {
		if row.Origin != AnswerEvidenceOriginRuntimeArtifact || row.ProvenanceLane != ObservationProvenanceArtifactSpan || row.SourceRef.Path != "" || row.SourceRef.ArtifactID != "attached_log" || row.Span != (ObservationSpan{}) || ObservationRecordHasCurrentSourceLineSpan(row) {
			t.Fatalf("frame borrowed header's cause/source/position: %+v", row)
		}
		if strings.Contains(strings.Join(row.RichNotes, "\n"), "source_generation=") {
			t.Fatalf("frame borrowed header's generation: %+v", row)
		}
		if !strings.Contains(strings.Join(row.RichNotes, "\n"), "causal role are unproven") {
			t.Fatalf("frame-only boundary missing: %+v", row)
		}
	}
	// Existing error/message projection stays one aggregate observation, with
	// all original stack support; it must not also create duplicate frame rows.
	stale.Message = "operation failed"
	bundle.Errors = []LogError{stale}
	ledger = CompileObservationLedger(ObservationLedgerInput{LogBundle: bundle})
	if len(ledger.Records) != 1 || ledger.Records[0].Predicate == "stack_frame" || ledger.Records[0].Summary != stale.Message {
		t.Fatalf("normal error projection duplicated or changed: %+v", ledger.Records)
	}
}
