package types

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"
)

func TestPeerLogObservationReasoningSurfacesCannotPromoteUnboundLabels(t *testing.T) {
	for _, evidence := range []string{"cause=remote: service reported failure", ""} {
		t.Run("evidence="+evidence, func(t *testing.T) {
			bundle := &LogBundle{
				Errors: []LogError{{Type: "error", Message: "request failed"}, {Type: "panic", Message: "bounds exceeded"}},
				Observations: []LogObservation{{
					Kind: LogObservationRuntimeEvent, Subject: "invented culprit identity", Summary: "invented cross-occurrence mechanism",
					Evidence: evidence, LineStart: 4, LineEnd: 4, Severity: LogObservationFailure, Diagnostic: true, Confidence: 1,
				}},
			}
			before, _ := json.Marshal(bundle)
			ledger := CompileObservationLedger(ObservationLedgerInput{LogBundle: bundle})
			var observation *ObservationRecord
			for i := range ledger.Records {
				if ledger.Records[i].ID == "log:observation:0" {
					observation = &ledger.Records[i]
				}
			}
			if evidence == "" && observation != nil {
				t.Errorf("an unbound interpretation with no excerpt became an observed fact: %+v", observation)
			}
			if evidence != "" && (observation == nil || observation.Summary != evidence || observation.RawExcerpt != evidence || observation.Span.LineStart != 4) {
				t.Errorf("literal evidence and its location must remain intact: %+v", observation)
			}
			for _, surface := range []struct {
				name string
				data any
			}{
				{"ledger", ledger},
				{"claim bindings", CompileRuntimeArtifactClaimBindings(&RequestModel{LogTriage: bundle}, nil)},
				{"artifact profile", BuildArtifactObservationProfile(bundle, nil)},
				{"external seeds", CollectExternalObservationSeeds(bundle, nil)},
			} {
				t.Run(surface.name, func(t *testing.T) {
					wire, err := json.Marshal(surface.data)
					if err != nil {
						t.Fatal(err)
					}
					for _, forbidden := range []string{bundle.Observations[0].Subject, bundle.Observations[0].Summary} {
						if strings.Contains(string(wire), forbidden) {
							t.Errorf("unbound model interpretation was promoted into %s: %s", surface.name, wire)
						}
					}
					if evidence != "" && !strings.Contains(string(wire), evidence) {
						t.Errorf("%s lost literal evidence (including any cause-like words it actually contains): %s", surface.name, wire)
					}
				})
			}
			after, _ := json.Marshal(bundle)
			if !bytes.Equal(before, after) {
				t.Fatal("reasoning projection modified the original triager audit fields")
			}
		})
	}
}

func TestLogObservationReasoningProjectionPreservesNonPeerAndNestedCause(t *testing.T) {
	obs := LogObservation{Kind: LogObservationThreadSnapshot, Subject: "worker 9", Summary: "advisory interpretation", Evidence: "worker 9 running", LineStart: 8}
	for _, bundle := range []*LogBundle{
		nil,
		{},
		{Errors: []LogError{{Type: "single"}}},
		{Errors: []LogError{{Type: "outer", Cause: &LogError{Type: "inner"}, CauseRelation: &LogCauseRelation{
			Authority: LogCauseAuthorityExplicitArtifactMarker, Marker: "Caused by: inner",
		}}}},
	} {
		got, ok := ProjectLogObservationForReasoning(bundle, obs)
		if !ok || got != obs {
			t.Fatalf("non-peer/explicit nested-cause observation was changed: %+v", got)
		}
	}
	peer := &LogBundle{Errors: []LogError{{Type: "first"}, {Type: "second"}}}
	got, ok := ProjectLogObservationForReasoning(peer, obs)
	if !ok || got.Subject != "" || got.Summary != obs.Evidence || got.Kind != obs.Kind || got.LineStart != obs.LineStart {
		t.Fatalf("peer snapshot must retain its literal context without inventing an error identity: %+v", got)
	}
}
