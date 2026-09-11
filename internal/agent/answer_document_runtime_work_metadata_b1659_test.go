package agent

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/types"
)

func b1659RuntimeWorkContext(lang string, profileOnly bool) *types.AgentContext {
	mu := types.NewMutableState("Explain how the observed operation relates to the target")
	mu.SetLogTriage(&types.LogBundle{Errors: []types.LogError{{Type: "runtime observation"}}})
	rm := types.RequestModel{Intent: types.IntentExplain, Language: lang, LogTriage: mu.LogTriage()}
	if profileOnly {
		rm.RuntimeQuestionProfile = &types.RuntimeQuestionProfile{
			Scope: types.RuntimeQuestionScopeRelationAnalysis, RuntimeWorkRelationRequested: true,
		}
	} else {
		rm.RequestedAnswerDimensions = &types.RequestedAnswerDimensionProfile{
			IsDimensionedAnswer: true, Dimensions: []types.RequestedAnswerDimension{{
				Index: 1, Required: true, Label: "operation-to-target relation",
				Role: types.RequestedAnswerDimensionRuntimeWorkRelation,
			}},
		}
	}
	return &types.AgentContext{Language: lang, Mutable: mu, AnalysisIR: &types.AnalysisIR{RequestModel: rm}}
}

func b1659BoundRuntimeWorkDocument(t *testing.T) *types.AnswerDocumentV2 {
	t.Helper()
	r := &types.AnswerRuntimeWorkRelationReceipt{
		ObservationID: "runtime-result#operation:7", Conclusion: types.RuntimeWorkRelationConclusionRelatedCausalityUnproven,
	}
	contract := &types.RuntimeWorkRelationContract{Rows: []types.RuntimeWorkRelationRow{{
		ObservationID: r.ObservationID, WorkLabel: "Observed operation", Subject: "worker-17",
		MeasuredDurationMS: 2.375, Credential: "host_direct_wakeup_edge",
		Boundary:           "work_completion_target_wait_and_frame_causality_unproven",
		AllowedConclusions: []types.RuntimeWorkRelationConclusion{r.Conclusion},
	}}}
	if !types.BindRuntimeWorkRelationReceipt(r, contract) || !r.IsBound() {
		t.Fatal("fixture must bind the model-selected exact row and allowed conclusion through the existing public binder")
	}
	return &types.AnswerDocumentV2{Blocks: []types.AnswerBlock{{
		ID: "model-conclusion", Kind: types.BlockSummary, SurfaceRole: types.SurfacePrincipal,
		Text:                "The observed operation is related; completion causality is not established.\nKeep this model explanation unchanged.",
		FacetIDs:            []string{"runtime_work_relation", "observed_artifact_fact"},
		ClaimUses:           []types.RenderedClaimUse{{ClaimForm: types.ClaimExternalObservation, FacetID: "observed_artifact_fact"}},
		RuntimeWorkRelation: r,
	}}}
}

func b1659AssertExecutableMetadataTeaching(t *testing.T, text, lang string) {
	t.Helper()
	want := []string{
		`surface_role:"principal"`, `facet_ids:["runtime_work_relation","observed_artifact_fact"]`,
		`claim_uses:[{"claim_form":"external_observation","facet_id":"observed_artifact_fact"}]`,
		`runtime_work_relation:{observation_id,conclusion}`, "add_facet_id", "block_id", "value", "replace_blocks",
	}
	if lang == "zh" {
		want = append(want, "已绑定", "不要重新选择", "只补缺少的元数据", "当前工具 schema", "完整目标块", "不是字段合并", "不扫描或改写正文")
	} else {
		want = append(want, "already bound", "do not reselect", "Repair only missing metadata", "current tool schema", "COMPLETE target block", "not a field merge", "without scanning or rewriting prose")
	}
	for _, token := range want {
		if !strings.Contains(text, token) {
			t.Errorf("runtime-work teaching omits %q:\n%s", token, text)
		}
	}
}

func TestB1659ActualInitialAndObservedCoverageTeachSameMetadata(t *testing.T) {
	for _, lang := range []string{"zh", "en"} {
		for _, profileOnly := range []bool{false, true} {
			name := lang + "/dimension"
			if profileOnly {
				name = lang + "/profile_without_dimension"
			}
			t.Run(name, func(t *testing.T) {
				ctx := b1659RuntimeWorkContext(lang, profileOnly)
				e := &answerDocumentEvaluator{}
				initial := e.BuildInitialInstruction(ctx, nil)
				section := renderAnswerDocRequestedAnswerDimensions(ctx)
				if !strings.Contains(initial, section) || section == "" {
					t.Fatal("actual initial instruction did not consume requested-dimension teaching")
				}
				b1659AssertExecutableMetadataTeaching(t, section, lang)
				doc := b1659BoundRuntimeWorkDocument(t)
				// This is the live failure shape: the receipt is valid, but the
				// summary lacks only its runtime-work facet, not another choice.
				doc.Blocks[0].FacetIDs = []string{"observed_artifact_fact"}
				before, _ := json.Marshal(doc)
				// BoundRow is deliberately absent from the model wire JSON.
				// Snapshot it independently, including its slice-valued facts.
				boundBefore, _ := json.Marshal(doc.Blocks[0].RuntimeWorkRelation.BoundRow)
				ctx.Mutable.SetAnswerDocumentV2WithMutation(types.MutationReplaceAll, doc)
				sig := e.Observe(ctx, LoopObservation{Phase: PhaseMidLoop})
				if !sig.HintRequested || sig.HintKey != postEmitAdvisoryHintKey || sig.StopRequested || !sig.BypassBudget {
					t.Fatalf("actual accepted-document coverage must produce the existing soft advisory, got %+v", sig)
				}
				b1659AssertExecutableMetadataTeaching(t, sig.Hint, lang)
				teaching := runtimeWorkRelationMetadataTeaching(lang)
				if strings.Count(initial, teaching) != 1 || strings.Count(sig.Hint, teaching) != 1 {
					t.Fatal("initial and observed repair must each consume the same teaching exactly once")
				}
				if e.retriesUsed != 0 || e.rejectHintsUsed != 0 {
					t.Fatal("metadata guidance consumed a hard rejection budget")
				}
				after, _ := json.Marshal(ctx.Mutable.AnswerDocumentV2())
				boundAfter, _ := json.Marshal(ctx.Mutable.AnswerDocumentV2().Blocks[0].RuntimeWorkRelation.BoundRow)
				if string(before) != string(after) || string(boundBefore) != string(boundAfter) {
					t.Fatal("coverage hint changed model content, metadata, or the system-bound chosen row")
				}
				second := e.Observe(ctx, LoopObservation{Phase: PhaseMidLoop, LastToolResult: &types.ToolResult{
					ToolName: "emit_answer_document_patch", Success: false,
				}})
				if second.HintRequested || !second.StopRequested {
					t.Fatal("a rejected optional advisory patch must still ship the accepted model document")
				}
			})
		}
	}
}

func TestB1659ExistingCoverageAndModelChoiceRemainUnchanged(t *testing.T) {
	for _, tc := range []struct {
		name    string
		edit    func(*types.AnswerBlock)
		missing bool
	}{
		{"complete", func(*types.AnswerBlock) {}, false},
		{"work_facet", func(b *types.AnswerBlock) { b.FacetIDs = []string{"observed_artifact_fact"} }, true},
		// Existing coverage also reads ClaimUses[].FacetID. A facet absent
		// from the block array but present on its claim is not a new gap.
		{"observation_facet_on_claim", func(b *types.AnswerBlock) { b.FacetIDs = []string{"runtime_work_relation"} }, false},
		{"observation_facet_absent", func(b *types.AnswerBlock) {
			b.FacetIDs = []string{"runtime_work_relation"}
			b.ClaimUses[0].FacetID = ""
		}, true},
		{"surface", func(b *types.AnswerBlock) { b.SurfaceRole = "" }, true},
		{"claim", func(b *types.AnswerBlock) { b.ClaimUses = nil }, true},
		{"unbound", func(b *types.AnswerBlock) { b.RuntimeWorkRelation.BoundRow = types.RuntimeWorkRelationRow{} }, true},
		{"system_owned", func(b *types.AnswerBlock) { b.SystemGeneratedKind = types.AnswerSystemGeneratedEvidenceSupplement }, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			ctx := b1659RuntimeWorkContext("en", true)
			e := &answerDocumentEvaluator{}
			_ = e.BuildInitialInstruction(ctx, nil)
			doc := b1659BoundRuntimeWorkDocument(t)
			tc.edit(&doc.Blocks[0])
			before, _ := json.Marshal(doc)
			ctx.Mutable.SetAnswerDocumentV2WithMutation(types.MutationReplaceAll, doc)
			sig := e.Observe(ctx, LoopObservation{Phase: PhaseMidLoop})
			if sig.HintRequested != tc.missing || sig.StopRequested == tc.missing {
				t.Fatalf("existing coverage predicate changed: missing=%t, signal=%+v", tc.missing, sig)
			}
			after, _ := json.Marshal(ctx.Mutable.AnswerDocumentV2())
			if string(before) != string(after) {
				t.Fatal("coverage handling changed the model's block")
			}
		})
	}
	ctx := b1659RuntimeWorkContext("en", false)
	ctx.AnalysisIR.RequestModel.RequestedAnswerDimensions = nil
	if got := renderAnswerDocRequestedAnswerDimensions(ctx); got != "" {
		t.Fatalf("ordinary runtime evidence without a typed request must not acquire this teaching obligation: %s", got)
	}
}
