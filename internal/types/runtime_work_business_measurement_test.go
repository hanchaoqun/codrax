package types

import (
	"encoding/json"
	"math"
	"reflect"
	"testing"
)

func clippedBusinessMeasurementFixture(notes ...string) ObservationRecord {
	r := measuredBusinessSpanFixture()
	r.Span.StartTs, r.Span.EndTs, r.Value = 1.001, 1.05, "49.000"
	r.SourceRef.QueryWindowStartTs, r.SourceRef.QueryWindowEndTs = 1.001, 1.05
	r.RichNotes = append([]string{TraceNoteKeySelectedWindow + "=1.001000..1.050000"}, notes...)
	return r
}

func TestRuntimeWorkBusinessMeasurementValidatesSameRecordFullPair(t *testing.T) {
	for _, tc := range []struct {
		name, window, ms string
		full             bool
	}{
		{"valid", "1.000000..1.050000", "50.000", true},
		{"rounded_endpoints", "1.000000..1.050001", "50.000", true},
		{"same_as_current", "1.001000..1.050000", "49.000", false},
		{"absent", "", "", false},
		{"duration_only", "", "50.000", false},
		{"window_only", "1.000000..1.050000", "", false},
		{"bad_window", "not-a-window", "50.000", false},
		{"bad_duration", "1.000000..1.050000", "bad", false},
		{"nan_duration", "1.000000..1.050000", "NaN", false},
		{"inf_duration", "1.000000..1.050000", "+Inf", false},
		{"inf_window", "1.000000..+Inf", "50.000", false},
		{"nan_window", "NaN..1.050000", "50.000", false},
		{"does_not_contain_start", "1.002000..1.050000", "48.000", false},
		{"does_not_contain_end", "1.000000..1.049000", "49.000", false},
		{"inconsistent_duration", "1.000000..1.050000", "49.000", false},
		{"negative_duration", "1.000000..1.050000", "-50.000", false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			r := clippedBusinessMeasurementFixture()
			if tc.window != "" {
				r.RichNotes = append(r.RichNotes, TraceNoteKeyActualWindow+"="+tc.window)
			}
			if tc.ms != "" {
				r.RichNotes = append(r.RichNotes, TraceNoteKeyActualImpactMS+"="+tc.ms)
			}
			before, _ := json.Marshal(r)
			contract := BuildRuntimeWorkRelationContract(businessSpanContractInput(r), true)
			if !contract.Active() || len(contract.Rows) != 1 {
				t.Fatalf("optional metadata erased measured row: %+v", contract)
			}
			row := contract.Rows[0]
			m, ok := row.BusinessSpanMeasurement()
			if !ok || m.FullKnown != tc.full || row.MeasuredDurationMS != 49 || m.StartTs != 1.001 || m.EndTs != 1.05 || m.QueryStartTs != 1.001 || m.QueryEndTs != 1.05 {
				t.Fatalf("wrong measurement: row=%+v m=%+v", row, m)
			}
			if !tc.full && (m.FullStartTs != 0 || m.FullEndTs != 0 || m.FullDurationMS != 0) {
				t.Fatalf("bad partial full metadata survived: %+v", m)
			}
			if row.Credential != "none" || !reflect.DeepEqual(row.AllowedConclusions, []RuntimeWorkRelationConclusion{RuntimeWorkRelationConclusionRelationUnproven}) {
				t.Fatalf("measurement minted causal authority: %+v", row)
			}
			after, _ := json.Marshal(r)
			if string(before) != string(after) {
				t.Fatal("measurement compilation mutated observation")
			}
		})
	}
}

func TestRuntimeWorkBusinessMeasurementPrivateCloneAndWire(t *testing.T) {
	r := clippedBusinessMeasurementFixture(TraceNoteKeyActualWindow+"=1.000000..1.050000", TraceNoteKeyActualImpactMS+"=50.000")
	contract := BuildRuntimeWorkRelationContract(businessSpanContractInput(r), true)
	view := cloneAnswerSemanticView(&AnswerSemanticView{RuntimeWorkRelationContract: contract})
	want, _ := contract.Rows[0].BusinessSpanMeasurement()
	got, ok := view.RuntimeWorkRelationContract.Rows[0].BusinessSpanMeasurement()
	if !ok || got != want {
		t.Fatalf("semantic clone lost private ruler: %+v", got)
	}
	got.FullDurationMS = 999
	if again, _ := view.RuntimeWorkRelationContract.Rows[0].BusinessSpanMeasurement(); again != want {
		t.Fatal("measurement accessor aliases its row")
	}
	receipt := &AnswerRuntimeWorkRelationReceipt{ObservationID: r.ID, Conclusion: RuntimeWorkRelationConclusionRelationUnproven}
	if !BindRuntimeWorkRelationReceipt(receipt, view.RuntimeWorkRelationContract) {
		t.Fatal("binding failed")
	}
	state := NewMutableState("measured business")
	state.SetAnswerDocumentV2WithMutation(MutationReplaceAll, &AnswerDocumentV2{Blocks: []AnswerBlock{{ID: "work", Kind: BlockSummary, RuntimeWorkRelation: receipt}}})
	doc := state.AnswerDocumentV2()
	if got, ok := doc.Blocks[0].RuntimeWorkRelation.BoundRow.BusinessSpanMeasurement(); !ok || got != want {
		t.Fatalf("document clone lost private measurement: %+v", got)
	}
	doc.Blocks[0].RuntimeWorkRelation.BoundRow.businessMeasurement.FullDurationMS = 999
	if got, _ := state.AnswerDocumentV2().Blocks[0].RuntimeWorkRelation.BoundRow.BusinessSpanMeasurement(); got != want {
		t.Fatal("mutable clone shares the private measurement")
	}
	wire, _ := json.Marshal(receipt)
	var object map[string]any
	if err := json.Unmarshal(wire, &object); err != nil || len(object) != 2 || object["observation_id"] != r.ID || object["conclusion"] != "relation_unproven" {
		t.Fatalf("receipt wire expanded: %s (%v)", wire, err)
	}
	var unbound AnswerRuntimeWorkRelationReceipt
	if err := json.Unmarshal(wire, &unbound); err != nil || unbound.IsBound() {
		t.Fatalf("serialized receipt became bound without contract validation: %+v (%v)", unbound, err)
	}
	rowWire, _ := json.Marshal(contract.Rows[0])
	var roundTrip RuntimeWorkRelationRow
	if err := json.Unmarshal(rowWire, &roundTrip); err != nil {
		t.Fatal(err)
	}
	if _, ok := roundTrip.BusinessSpanMeasurement(); ok {
		t.Fatal("serialized row minted private ordinary-span authority")
	}
}

func TestRuntimeWorkBusinessMeasurementSameOwnerDoesNotMintExecution(t *testing.T) {
	for _, kind := range []string{"sync", "async"} {
		r := clippedBusinessMeasurementFixture(TraceNoteKeySpanKind + "=" + kind)
		input := businessSpanContractInput(r)
		input.RequestModel = &RequestModel{RuntimeTargets: []RuntimeTarget{{PID: 200, Thread: "worker-200", Source: "user_explicit"}}}
		contract := BuildRuntimeWorkRelationContract(input, true)
		if !contract.Active() {
			t.Fatal("ordinary measured row disappeared")
		}
		for _, conclusion := range []RuntimeWorkRelationConclusion{RuntimeWorkRelationConclusionTargetSelfWorkObserved, RuntimeWorkRelationConclusionRelatedCausalityUnproven, RuntimeWorkRelationConclusionCausalContributionSupported} {
			if BindRuntimeWorkRelationReceipt(&AnswerRuntimeWorkRelationReceipt{ObservationID: r.ID, Conclusion: conclusion}, contract) {
				t.Fatalf("%s matching owner minted %s", kind, conclusion)
			}
		}
	}
}

func TestRuntimeWorkBusinessMeasurementRejectsInvalidCurrentBounds(t *testing.T) {
	for _, tc := range []struct {
		name string
		edit func(*ObservationRecord)
	}{
		{"bad_selected", func(r *ObservationRecord) { r.RichNotes[0] = TraceNoteKeySelectedWindow + "=bad" }},
		{"nan_selected", func(r *ObservationRecord) { r.RichNotes[0] = TraceNoteKeySelectedWindow + "=NaN..1.050000" }},
		{"inf_selected", func(r *ObservationRecord) { r.RichNotes[0] = TraceNoteKeySelectedWindow + "=1.001000..+Inf" }},
		{"nan_span", func(r *ObservationRecord) { r.Span.StartTs = math.NaN() }},
		{"inf_span", func(r *ObservationRecord) { r.Span.EndTs = math.Inf(1) }},
		{"reversed_span", func(r *ObservationRecord) { r.Span.StartTs, r.Span.EndTs = r.Span.EndTs, r.Span.StartTs }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			r := clippedBusinessMeasurementFixture(TraceNoteKeyActualWindow+"=1.000000..1.050000", TraceNoteKeyActualImpactMS+"=50.000")
			tc.edit(&r)
			if contract := BuildRuntimeWorkRelationContract(businessSpanContractInput(r), true); contract.Active() {
				t.Fatalf("invalid current measurement became selectable: %+v", contract)
			}
		})
	}
}
