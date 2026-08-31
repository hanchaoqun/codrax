package types

import "testing"

func TestBindRuntimeWorkRelationReceiptRequiresExactPublishedPair(t *testing.T) {
	contract := &RuntimeWorkRelationContract{Rows: []RuntimeWorkRelationRow{{
		ObservationID: "trace_query:test#trace_semantic_span:1",
		WorkLabel:     "VerifyClass Demo", Subject: "worker-7", MeasuredDurationMS: 0.285,
		AllowedConclusions: []RuntimeWorkRelationConclusion{
			RuntimeWorkRelationConclusionRelatedCausalityUnproven,
			RuntimeWorkRelationConclusionRelationUnproven,
		},
		Credential: "host_direct_wakeup_edge",
		Boundary:   "work_completion_target_wait_and_frame_causality_unproven",
	}}}
	receipt := &AnswerRuntimeWorkRelationReceipt{
		ObservationID: "trace_query:test#trace_semantic_span:1",
		Conclusion:    RuntimeWorkRelationConclusionRelatedCausalityUnproven,
	}
	if !BindRuntimeWorkRelationReceipt(receipt, contract) || !receipt.IsBound() || receipt.BoundRow.WorkLabel != "VerifyClass Demo" {
		t.Fatalf("exact receipt did not bind: %+v", receipt)
	}
	conservative := &AnswerRuntimeWorkRelationReceipt{
		ObservationID: receipt.ObservationID,
		Conclusion:    RuntimeWorkRelationConclusionRelationUnproven,
	}
	if !BindRuntimeWorkRelationReceipt(conservative, contract) {
		t.Fatal("the model must remain free to choose a weaker schema-published conclusion")
	}

	wrongConclusion := &AnswerRuntimeWorkRelationReceipt{
		ObservationID: receipt.ObservationID,
		Conclusion:    RuntimeWorkRelationConclusionCausalContributionSupported,
	}
	if BindRuntimeWorkRelationReceipt(wrongConclusion, contract) {
		t.Fatal("a stronger model conclusion must not exceed the exact row's evidence ceiling")
	}
	unknown := &AnswerRuntimeWorkRelationReceipt{
		ObservationID: "trace_query:test#trace_semantic_span:missing",
		Conclusion:    RuntimeWorkRelationConclusionRelatedCausalityUnproven,
	}
	if BindRuntimeWorkRelationReceipt(unknown, contract) {
		t.Fatal("an unknown observation id must not bind")
	}
}

func TestMutableStatePreservesBoundRuntimeWorkRelationReceiptAcrossClone(t *testing.T) {
	state := NewMutableState("runtime-work receipt clone")
	receipt := &AnswerRuntimeWorkRelationReceipt{
		ObservationID: "trace_query:test#trace_semantic_span:1",
		Conclusion:    RuntimeWorkRelationConclusionRelatedCausalityUnproven,
		BoundRow: RuntimeWorkRelationRow{
			ObservationID: "trace_query:test#trace_semantic_span:1",
			WorkLabel:     "VerifyClass Demo",
			AllowedConclusions: []RuntimeWorkRelationConclusion{
				RuntimeWorkRelationConclusionRelatedCausalityUnproven,
			},
		},
	}
	state.SetAnswerDocumentV2WithMutation(MutationReplaceAll, &AnswerDocumentV2{
		DocumentModel: "v2",
		Blocks: []AnswerBlock{{
			ID:                  "principal",
			Kind:                BlockSummary,
			RuntimeWorkRelation: receipt,
		}},
	})

	first := state.AnswerDocumentV2()
	if first == nil || first.Blocks[0].RuntimeWorkRelation == nil || !first.Blocks[0].RuntimeWorkRelation.IsBound() {
		t.Fatalf("bound runtime-work receipt was lost across persistence clone: %+v", first)
	}
	first.Blocks[0].RuntimeWorkRelation.BoundRow.WorkLabel = "mutated reader copy"
	second := state.AnswerDocumentV2()
	if got := second.Blocks[0].RuntimeWorkRelation.BoundRow.WorkLabel; got != "VerifyClass Demo" {
		t.Fatalf("runtime-work receipt clone aliases reader mutation: got %q", got)
	}
}
