package agent

import (
	"encoding/json"
	"errors"
	"strings"
	"testing"

	ctxbuilder "github.com/hanchaoqun/codrax/internal/context"
	toolpkg "github.com/hanchaoqun/codrax/internal/tool"
	"github.com/hanchaoqun/codrax/internal/types"
)

// Exercise the real context builder and Finalizer's first adapter request. The
// typed fixtures represent accepted source evidence/inventory, not new evidence
// produced by this test; the adapter stops before any model response or mutation.
func TestFinalizerFamilyCitationTeachingPublic(t *testing.T) {
	for _, lang := range []string{"zh", "en"} {
		for _, tc := range []struct {
			name                        string
			config, scalar, table       bool
			profile, inventory, history bool
		}{
			{name: "config_source", config: true},
			{name: "config_scalar", config: true, scalar: true},
			{name: "config_inventory", config: true, profile: true, inventory: true},
			{name: "comparison_sections"},
			{name: "comparison_inventory", profile: true, inventory: true},
			{name: "comparison_profile_only", profile: true},
			{name: "comparison_observation_only", inventory: true},
			{name: "comparison_per_member_table", table: true, profile: true, inventory: true},
			{name: "comparison_history_without_source", history: true},
		} {
			t.Run(lang+"/"+tc.name, func(t *testing.T) {
				rm := types.RequestModel{Intent: types.IntentExplain, Language: lang}
				wantFamily := types.QFComparison
				if tc.config {
					rm.Scenario = types.ScenarioConfigTrace
					rm.AnswerSubject = types.AnswerSubject{Kind: types.SubjectConfigKey}
					wantFamily = types.QFConfigPrecedence
				} else {
					rm.Buckets = []types.QuestionBucket{{Label: "Alpha", Index: 1}, {Label: "Beta", Index: 2}}
				}
				rm.Predicates.IsScalarAnswer = tc.scalar
				rm.Predicates.HasPerMemberTable = tc.table
				rm.Predicates.IsHistoryLookup = tc.history
				if tc.profile {
					rm.SourceInventoryProfile = &types.SourceInventoryProfile{
						IsSourceInventory: true, TargetRoles: []types.AnswerCandidateRole{types.AnswerCandidateRoleFunction},
						RequestedFields: []types.SourceInventoryRequestedField{types.SourceInventoryFieldName, types.SourceInventoryFieldLocation}, Confidence: 1,
					}
				}
				mu := types.NewMutableState("Explain the requested layers or compare the requested groups.")
				mu.SetRequestModel(rm)
				if tc.inventory {
					mu.SetSourceInventoryObservation(types.SourceInventoryObservation{
						Active: true, Complete: true, Scopes: []string{"src"},
						Sets: []types.SourceInventoryObservationSet{{Role: types.AnswerCandidateRoleFunction, Complete: true, Count: 1,
							Members: []types.SourceInventoryObservationMember{{Name: "Dispatch", File: "src/dispatch.go", Line: 12,
								CoverageState: types.SourceInventoryCoverageObserved}},
						}},
					})
				}
				bus := &types.BusContext{Language: lang, Mutable: mu,
					AnalysisIR: &types.AnalysisIR{RequestModel: rm, AnswerContract: types.AnswerContract{Language: lang}},
				}
				if !tc.history {
					bus.EvidenceItems = []types.EvidenceItem{{ID: "dispatch-definition", Kind: types.EvidenceDirect,
						Subject: "Dispatch", AnchorSymbol: "Dispatch", AnchorKind: types.AnchorDefinition,
						Source: "src/dispatch.go", LineStart: 12, Scope: types.ScopeLine,
						GroundingStatus: types.GroundingGrounded, Snippet: "func Dispatch() {}"}}
				}
				before := familyCitationTeachingPublicSnapshot(t, bus)
				ctx := ctxbuilder.BuildAgentContext(bus, types.AgentFinalizer, types.StageFinalize)
				view := types.BuildAnswerSemanticViewForAgentContext(ctx)
				if view == nil || view.Family != wantFamily {
					t.Fatalf("HARNESS: wrong compiled family: %+v", view)
				}
				reg := toolpkg.NewRegistry()
				reg.Register(&toolpkg.EmitAnswerDocument{})
				reg.Register(&toolpkg.EmitAnswerDocumentPatch{})
				capture := &traceTeachingCaptureLLM{stop: errors.New("captured family citation teaching")}
				finalizer := NewFinalizerAgent(&Dependencies{LLM: capture, Tools: reg, MaxIterations: 1})
				_, err := finalizer.Execute(ctx, traceTeachingSkill(t, "answer-document-skill"))
				if !errors.Is(err, capture.stop) || capture.calls != 1 {
					t.Fatalf("HARNESS: initial request not captured: calls=%d err=%v", capture.calls, err)
				}
				var system, user strings.Builder
				for _, msg := range capture.messages {
					if msg.Role == "system" {
						system.WriteString(msg.Content)
					} else if msg.Role == "user" {
						user.WriteString(msg.Content)
					}
				}
				if system.Len() == 0 || user.Len() == 0 {
					t.Fatal("HARNESS: missing actual system/user messages")
				}
				var emitSchema json.RawMessage
				for _, schema := range capture.tools {
					if schema.Name == "emit_answer_document" {
						emitSchema = schema.Parameters
					}
				}
				props := familyCitationTeachingPublicItemProperties(t, emitSchema)
				for field, want := range map[string]bool{
					"evidence_ids": !tc.history, "source_inventory_row_id": tc.profile && tc.inventory,
				} {
					if _, got := props[field]; got != want {
						t.Fatalf("HARNESS: actual dispatch field %s available=%v want=%v", field, got, want)
					}
				}
				for _, field := range []string{"label", "text", "cells"} {
					if _, ok := props[field]; !ok {
						t.Errorf("existing row object lost %s", field)
					}
				}
				// Read only this family-owned subsection: the shared rules may
				// legitimately name citation_ref while explaining compatibility.
				_, blockText, ok := strings.Cut(user.String(), "## Required Answer Blocks\n")
				if !ok {
					t.Fatal("HARNESS: compiled block contract missing from actual user message")
				}
				blockText, _, _ = strings.Cut(blockText, "\n## ")
				if strings.Contains(blockText, "citation_ref") {
					t.Error("family teaching still prescribes manual citation indexes instead of the current shared carrier rules")
				}
				if !tc.table && !strings.Contains(blockText, "shared item evidence/citation rules for the current projected schema") {
					t.Error("family teaching does not defer to the shared dispatch-local citation carrier rules")
				}
				var protected []string
				if tc.config {
					protected = []string{"from low to high", "both that key and literal", "runtime value is unknown", "not treat repository absence as proof that the layer is unset"}
					found := false
					for _, req := range append(append([]types.BlockRequirement(nil), view.RequiredBlocks...), view.OptionalBlocks...) {
						if req.Kind == types.BlockTable && len(req.AlternativeKinds) == 1 && req.AlternativeKinds[0] == types.BlockOrderedList {
							found = req.Required == !tc.scalar && req.MaxCount == 1
						}
					}
					if !found {
						t.Error("precedence carrier lost scalar optionality or table/ordered-list shape")
					}
				} else if tc.table {
					protected = []string{"one principal per-member table with no list or section escape", "one row per requested member", "exact bucket identity visible in a separate row cell", "item.label, or cells[0]", "never spend it on the bucket"}
				} else {
					protected = []string{"One section per user-named bucket", "verbatim bucket label", "label/text/cells", "not duplicate the same roster in a separate global list or table"}
				}
				for _, want := range protected {
					if !strings.Contains(blockText, want) {
						t.Errorf("existing family obligation missing: %q", want)
					}
				}
				if !tc.config {
					kind, count := types.BlockSection, 2
					if tc.table {
						kind, count = types.BlockTable, 1
					}
					found := false
					for _, req := range view.RequiredBlocks {
						if req.Kind == kind && req.Required && req.MinCount == count && req.MaxCount == count {
							found = true
						}
					}
					if !found {
						t.Errorf("comparison lost exact %d %s requirement", count, kind)
					}
				}
				if !strings.Contains(system.String(), types.AnswerDocumentItemCitationCarrierTeaching) {
					t.Error("canonical shared citation ownership rule missing from actual system message")
				}
				if after := familyCitationTeachingPublicSnapshot(t, bus); after != before {
					t.Fatal("initial teaching mutated request, evidence, inventory, or model-owned document")
				}
			})
		}
	}
}

func familyCitationTeachingPublicItemProperties(t *testing.T, raw json.RawMessage) map[string]any {
	t.Helper()
	var schema map[string]any
	if err := json.Unmarshal(raw, &schema); err != nil {
		t.Fatalf("HARNESS: actual emit schema is invalid: %v", err)
	}
	for _, key := range []string{"properties", "blocks", "items", "properties", "items", "items", "properties"} {
		next, ok := schema[key].(map[string]any)
		if !ok {
			t.Fatalf("HARNESS: missing schema object %s", key)
		}
		schema = next
	}
	return schema
}

func familyCitationTeachingPublicSnapshot(t *testing.T, bus *types.BusContext) string {
	t.Helper()
	raw, err := json.Marshal([]any{bus.AnalysisIR, bus.EvidenceItems, bus.Mutable.EmittedEvidence(), bus.Mutable.SourceInventoryObservation(), bus.Mutable.AnswerDocumentV2()})
	if err != nil {
		t.Fatal(err)
	}
	return string(raw)
}
