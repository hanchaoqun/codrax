package types

import (
	"encoding/json"
	"strings"
	"testing"
)

func memberCoverageFixture(t *testing.T) (ObservationLedgerInput, AnswerRuntimeMeasurementReceipt) {
	t.Helper()
	r := measurementProviderRecord(t)
	p, _ := DecodeRuntimeMeasurementPublication(r)
	p.Tables[0].View = RuntimeMeasurementMembers
	p.Tables[0].MemberSet = &RuntimeMeasurementMemberSet{PopulationID: "observed_pairs", RowIDs: []string{"pair-a"}, TotalRows: 1, Complete: true}
	b, _ := json.Marshal(p)
	r.RichNotes = []string{TraceNoteKeyRuntimeMeasurement + "=" + string(b)}
	input := measurementProviderInput(r)
	input.RuntimeArtifactPreflight = RuntimeArtifactPreflightProfile{Active: true, Artifacts: []RuntimeArtifactPreflightArtifact{{Kind: "trace", Source: r.SourceRef.Path, Carrier: "attachment"}}}
	start, end := 0.0, 1.0
	input.RequestModel = &RequestModel{RuntimeArtifactScopeProfile: &RuntimeArtifactScopeProfile{RequestedScope: RuntimeArtifactScopeExplicitWindow, TimeStart: &start, TimeEnd: &end, SourceQuote: "0..1"}}
	return input, AnswerRuntimeMeasurementReceipt{ObservationID: r.ID, View: RuntimeMeasurementMembers}
}

func TestRuntimeMeasurementMemberSetCoverageExactBoundaries(t *testing.T) {
	for _, bad := range []string{"", "summary", "timeline", "partial", "missing_metadata", "missing_ids", "duplicate_ids", "count", "model", "unknown_authority", "source", "no_preflight", "multiple_sources", "target", "named_target", "narrow_window", "full_artifact", "unknown_window", "duplicate_json", "casefold_json"} {
		t.Run(bad, func(t *testing.T) {
			input, selector := memberCoverageFixture(t)
			r := input.ToolResults[0].Observations[0]
			p, _ := DecodeRuntimeMeasurementPublication(r)
			table := &p.Tables[0]
			switch bad {
			case "summary":
				table.View = RuntimeMeasurementSummary
				selector.View = table.View
			case "timeline":
				table.View = RuntimeMeasurementTimeline
				selector.View = table.View
			case "partial":
				table.MemberSet.Complete = false
				table.MemberSet.TotalRows = 2
			case "missing_metadata":
				table.MemberSet = nil
			case "missing_ids":
				table.MemberSet.RowIDs = nil
			case "duplicate_ids":
				table.Rows = append(table.Rows, table.Rows[0])
				table.MemberSet.RowIDs = []string{"pair-a", "pair-a"}
				table.MemberSet.TotalRows = 2
			case "count":
				table.MemberSet.TotalRows = 2
			case "model":
				r.ClaimAuthority = ObservationClaimAuthorityModelInference
			case "unknown_authority":
				r.ClaimAuthority = "unsupported"
			case "source":
				input.RuntimeArtifactPreflight = RuntimeArtifactPreflightProfile{Active: true, Artifacts: []RuntimeArtifactPreflightArtifact{{Kind: "trace", Source: "/capture/other", Carrier: "attachment"}}}
			case "no_preflight":
				input.RuntimeArtifactPreflight = RuntimeArtifactPreflightProfile{}
			case "multiple_sources":
				input.RuntimeArtifactPreflight = RuntimeArtifactPreflightProfile{Active: true, Artifacts: []RuntimeArtifactPreflightArtifact{{Kind: "trace", Source: r.SourceRef.Path, Carrier: "attachment"}, {Kind: "trace", Source: "/capture/other", Carrier: "attachment"}}}
			case "target":
				r.SourceRef.QueryTargetPID = 12
				p.Source = r.SourceRef
			case "named_target":
				input.RequestModel.RuntimeTargetProfile = &RuntimeTargetProfile{Declaration: RuntimeTargetDeclarationNamedTarget, SourceQuote: "12"}
			case "narrow_window":
				r.SourceRef.QueryWindowEndTs = .5
				p.Source = r.SourceRef
			case "unknown_window":
				r.SourceRef.QueryWindowKnown = false
				r.SourceRef.QueryWindowEndTs = 0
				p.Source = r.SourceRef
			case "full_artifact":
				input.RequestModel.RuntimeArtifactScopeProfile = &RuntimeArtifactScopeProfile{RequestedScope: RuntimeArtifactScopeFullArtifact, SourceQuote: "all"}
			}
			b, _ := json.Marshal(p)
			if bad == "duplicate_json" {
				b = []byte(strings.Replace(string(b), `"complete":true`, `"complete":false,"complete":true`, 1))
			}
			if bad == "casefold_json" {
				b = []byte(strings.Replace(string(b), `"complete":true`, `"Complete":false,"complete":true`, 1))
			}
			r.RichNotes = []string{TraceNoteKeyRuntimeMeasurement + "=" + string(b)}
			input.ToolResults[0].Observations[0] = r
			contract := BuildRuntimeMeasurementContract(input)
			if !contract.Active() {
				t.Fatal("coverage restriction erased existing display capability")
			}
			_, got := RuntimeMeasurementMemberSetSelections([]AnswerRuntimeMeasurementReceipt{selector}, contract, input.RequestModel)
			if got != (bad == "") {
				t.Fatalf("member coverage=%t", got)
			}
		})
	}
}

func TestRuntimeMeasurementMemberSetSelectionIdentityAndWindowSet(t *testing.T) {
	input, selector := memberCoverageFixture(t)
	contract := BuildRuntimeMeasurementContract(input)
	selected, ok := RuntimeMeasurementMemberSetSelections([]AnswerRuntimeMeasurementReceipt{selector, selector}, contract, input.RequestModel)
	if !ok || len(selected) != 1 {
		t.Fatal("duplicate selector manufactured second coverage seat")
	}
	selected[0].BoundTable.MemberSet.RowIDs[0] = "changed"
	if contract.Tables[0].MemberSet.RowIDs[0] != "pair-a" {
		t.Fatal("selection leaked mutable coverage alias")
	}
	start, end := 2.0, 3.0
	profile := input.RequestModel.RuntimeArtifactScopeProfile
	profile.TimeWindows = append(profile.ExplicitTimeWindows(), RuntimeArtifactTimeWindow{TimeStart: &start, TimeEnd: &end, SourceQuote: "2..3"})
	profile.TimeStart, profile.TimeEnd = nil, nil
	if _, ok := RuntimeMeasurementMemberSetSelections([]AnswerRuntimeMeasurementReceipt{selector}, contract, input.RequestModel); ok {
		t.Fatal("one window claimed the two-window roster")
	}
	selector.ObservationID = "stale"
	if _, ok := RuntimeMeasurementMemberSetSelections([]AnswerRuntimeMeasurementReceipt{selector}, contract, input.RequestModel); ok {
		t.Fatal("stale selector accepted")
	}
}

func TestRuntimeMeasurementMemberSetJSONStringsAreNotMemberKeys(t *testing.T) {
	if !runtimeMeasurementUniqueKeys(`{"label":"example {\"complete\":true,\"complete\":false}","rows":[["0"]]}`) {
		t.Fatal("JSON-like string misclassified")
	}
}
