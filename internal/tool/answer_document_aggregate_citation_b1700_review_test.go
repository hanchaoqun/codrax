package tool

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"strconv"
	"testing"

	"github.com/hanchaoqun/codrax/internal/types"
)

func b1700AggregateCitationReviewContext(t *testing.T, member string) (*types.BusContext, types.Citation, []byte) {
	t.Helper()
	repo := t.TempDir()
	source := []byte("package sample\nfunc Other() {}\n")
	if err := os.WriteFile(filepath.Join(repo, "other.go"), source, 0600); err != nil {
		t.Fatal(err)
	}
	ctx := &types.BusContext{RepoRoot: repo, WorkDir: repo, Mutable: types.NewMutableState("explain the selected member"),
		AnalysisIR: &types.AnalysisIR{RequestModel: types.RequestModel{Intent: types.IntentExplain}}}
	read, err := (&ReadFile{}).Execute(ctx, json.RawMessage(`{"path":"other.go","limit":20}`))
	if err != nil || !read.Success || read.ReadCoverage == nil {
		t.Fatalf("real source read prerequisite: %v %+v", err, read)
	}
	ctx.Mutable.AppendDispatchToolResult(read)
	emitted, err := (&EmitEvidence{}).Execute(ctx, json.RawMessage(`{"items":[{"evidence_kind":"direct","scope":"line","source":"other.go","line_start":2,"anchor_kind":"definition","anchor_symbol":"Other","subject":"Other","summary":"Declares Other."}]}`))
	if err != nil || !emitted.Success {
		t.Fatalf("real source emit prerequisite: %v %+v", err, emitted)
	}
	ctx.Mutable.AppendDispatchToolResult(emitted)
	items := ctx.Mutable.EmittedEvidence()
	if len(items) != 1 || items[0].AnchorSymbol != "Other" || !items[0].IsCitable() {
		t.Fatalf("expected only the real Other definition: %+v", items)
	}
	fact := types.AnswerAggregateFact{Kind: types.AnswerAggregateMemberSet, Role: types.AnswerAggregateRolePrincipalAnswer,
		Label: "proposed member", Value: "1", Members: []string{member}, SupportRefs: []string{member + " @ other.go:2"}}
	ctx.Mutable.SetInvestigationAggregateFacts([]types.AnswerAggregateFact{fact})
	ctx.Mutable.RetainInvestigationAggregateFacts()
	qualified := types.AnswerAggregateFactAuthorizesPrincipalContractWithSourceContext(fact, &ctx.AnalysisIR.RequestModel,
		types.AnswerAggregateSourceContextFromBusContext(ctx))
	if qualified != (member == "Other") {
		t.Fatalf("qualification prerequisite=%t, member=%q", qualified, member)
	}
	return ctx, types.Citation{File: "other.go", Line: 2, Quote: "func Other() {}"}, source
}

func b1700AggregateCitationReviewBlock(member string, explicit bool) map[string]any {
	item := map[string]any{"id": "selected", "label": member, "text": "A model-authored proposed member; its explanation is unchanged."}
	if explicit {
		item["citation_ref"] = 0
	}
	return map[string]any{"id": "members", "kind": "bullet_list", "items": []any{item}}
}

func TestB1700ActualAggregateCandidateCitationNeedsObservedMember(t *testing.T) {
	for _, tc := range []struct {
		name, member string
		explicit     bool
		wantBound    bool
		existingPool bool
	}{
		{"unobserved_no_selection", "Ghost", false, false, false},
		{"unobserved_unselected_existing_pool", "Ghost", false, false, true},
		{"unobserved_explicit_model_selection_is_not_removed", "Ghost", true, true, true},
		{"observed_member_compatible_auto_binding", "Other", false, true, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			ctx, citation, originalSource := b1700AggregateCitationReviewContext(t, tc.member)
			factsBefore, _ := json.Marshal(ctx.Mutable.StableInvestigationAggregateFacts())
			evidenceBefore, _ := json.Marshal(ctx.Mutable.EmittedEvidence())
			block := b1700AggregateCitationReviewBlock(tc.member, tc.explicit)
			payload := map[string]any{"blocks": []any{
				map[string]any{"id": "summary", "kind": "summary", "text": "Distinguish proposed membership from the actual declaration."}, block,
			}}
			if tc.explicit || tc.existingPool {
				payload["citations"] = []types.Citation{citation}
			}
			doc := b1700QuoteBindingEmit(t, ctx, payload, false)
			check := func(stage string, doc *types.AnswerDocumentV2) {
				t.Helper()
				item := blockByID(t, doc, "members").Items[0]
				refs := types.AnswerBlockItemCitationRefs(item)
				if (len(refs) > 0) != tc.wantBound {
					t.Errorf("%s citation bound=%t want=%t: refs=%v pool=%+v item=%+v", stage, len(refs) > 0, tc.wantBound, refs, doc.Citations, item)
				}
				if tc.wantBound {
					if len(refs) != 1 || refs[0] < 0 || refs[0] >= len(doc.Citations) || doc.Citations[refs[0]].File != "other.go" || doc.Citations[refs[0]].Line != 2 {
						t.Errorf("%s lost the selected/observed citation: refs=%v pool=%+v", stage, refs, doc.Citations)
					}
				}
				if item.Label != tc.member || item.Text != "A model-authored proposed member; its explanation is unchanged." || !tc.explicit && len(item.EvidenceIDs) != 0 {
					t.Errorf("%s rewrote model body/identity: %+v", stage, item)
				}
				if tc.explicit && !item.CitationRefsModelSubmitted {
					t.Errorf("%s lost explicit citation provenance", stage)
				}
			}
			check("full", doc)
			doc = b1700QuoteBindingEmit(t, ctx, map[string]any{"unchanged_block_ids": []string{"summary"}, "replace_blocks": []any{block}}, true)
			check("replace patch", doc)
			doc = b1700QuoteBindingEmit(t, ctx, map[string]any{"unchanged_block_ids": []string{"members"}, "replace_blocks": []any{
				map[string]any{"id": "summary", "kind": "summary", "text": "Only the model-authored summary is revised."},
			}}, true)
			check("unchanged patch", doc)
			factsAfter, _ := json.Marshal(ctx.Mutable.StableInvestigationAggregateFacts())
			evidenceAfter, _ := json.Marshal(ctx.Mutable.EmittedEvidence())
			currentSource, err := os.ReadFile(filepath.Join(ctx.RepoRoot, "other.go"))
			if err != nil || !bytes.Equal(originalSource, currentSource) || !bytes.Equal(factsBefore, factsAfter) || !bytes.Equal(evidenceBefore, evidenceAfter) {
				t.Fatal("citation compatibility changed source or retained investigation state")
			}
		})
	}
}

// Isolate the automatic aggregate-candidate binder using the same real
// producer context: no quote matching, evidence-ID or row-ID selector exists.
func TestB1700IsolatedAggregateCandidateBinderDoesNotSelfAuthorize(t *testing.T) {
	for _, member := range []string{"Ghost", "Other"} {
		t.Run(member, func(t *testing.T) {
			ctx, _, _ := b1700AggregateCitationReviewContext(t, member)
			doc := &types.AnswerDocumentV2{Blocks: []types.AnswerBlock{{ID: "members", Kind: types.BlockBulletList,
				Items: []types.AnswerBlockItem{{ID: "selected", Label: member, Text: "Unchanged explanation.", CitationRef: -1}}}}}
			before := doc.Blocks[0].Items[0]
			fixed := normalizeItemCitationRefsByUniquePreEmitCandidateWithContext(doc, nil, ctx, newPreEmitCheckContext(ctx))
			want := 0
			if member == "Other" {
				want = 1
			}
			if fixed != want {
				t.Errorf("automatic aggregate-candidate binding=%d want=%d: doc=%+v", fixed, want, doc)
			}
			if member == "Ghost" && (!reflect.DeepEqual(before, doc.Blocks[0].Items[0]) || len(doc.Citations) != 0) {
				t.Errorf("unobserved aggregate ref created a citation: %+v", doc)
			}
		})
	}
}

func TestB1700ActualAggregateCandidatePreservesObservedOperationSites(t *testing.T) {
	for _, tc := range []struct {
		name, source, member, subject, object, anchor string
		kind                                          types.AnchorKind
		line                                          int
	}{
		{"callsite", "package sample\nfunc Other() {}\nfunc Caller() {\n\tOther()\n}\n", "Other", "sample.Caller", "Other", "Other", types.AnchorCall, 4},
		{"returned_literal", "package sample\nfunc Name() string { return \"wire_value\" }\n", `"wire_value"`, "Name", `"wire_value"`, "Name", types.AnchorReturn, 2},
	} {
		t.Run(tc.name, func(t *testing.T) {
			repo := t.TempDir()
			if err := os.WriteFile(filepath.Join(repo, "operation.go"), []byte(tc.source), 0600); err != nil {
				t.Fatal(err)
			}
			ctx := &types.BusContext{RepoRoot: repo, WorkDir: repo, Mutable: types.NewMutableState("show the recorded operation"),
				AnalysisIR: &types.AnalysisIR{RequestModel: types.RequestModel{Intent: types.IntentExplain}}}
			read, err := (&ReadFile{}).Execute(ctx, json.RawMessage(`{"path":"operation.go","limit":20}`))
			if err != nil || !read.Success || read.ReadCoverage == nil {
				t.Fatalf("operation source read prerequisite: %v %+v", err, read)
			}
			ctx.Mutable.AppendDispatchToolResult(read)
			ctx.ToolResults = []types.ToolResult{read}
			ctx.Mutable.SetTurnAArtifacts(types.TurnAArtifacts{ToolResults: ctx.ToolResults})
			predicate := "returns"
			if tc.kind == types.AnchorCall {
				predicate = "calls"
			}
			raw, _ := json.Marshal(map[string]any{"items": []any{map[string]any{
				"evidence_kind": "direct", "scope": "line", "source": "operation.go", "line_start": tc.line,
				"anchor_kind": tc.kind, "anchor_symbol": tc.anchor, "subject": tc.subject, "object": tc.object, "predicate": predicate,
				"summary": "The exact recorded operation.",
			}}})
			emitted, err := (&EmitEvidence{}).Execute(ctx, raw)
			if err != nil || !emitted.Success {
				t.Fatalf("operation source emit prerequisite: %v %+v", err, emitted)
			}
			ctx.Mutable.AppendDispatchToolResult(emitted)
			fact := types.AnswerAggregateFact{Kind: types.AnswerAggregateMemberSet, Role: types.AnswerAggregateRolePrincipalAnswer,
				Label: "recorded operation member", Value: "1", Members: []string{tc.member},
				SupportRefs: []string{tc.member + " @ operation.go:" + strconv.Itoa(tc.line)}}
			ctx.Mutable.SetInvestigationAggregateFacts([]types.AnswerAggregateFact{fact})
			ctx.Mutable.RetainInvestigationAggregateFacts()
			if !types.AnswerAggregateFactAuthorizesPrincipalContractWithSourceContext(fact, &ctx.AnalysisIR.RequestModel,
				types.AnswerAggregateSourceContextFromBusContext(ctx)) {
				t.Fatalf("real operation must qualify before testing its citation: %+v", ctx.Mutable.EmittedEvidence())
			}
			block := b1700AggregateCitationReviewBlock(tc.member, false)
			doc := b1700QuoteBindingEmit(t, ctx, map[string]any{"blocks": []any{
				map[string]any{"id": "summary", "kind": "summary", "text": "This location supports the recorded operation only."}, block,
			}}, false)
			for attempt := 0; attempt < 2; attempt++ {
				if attempt == 1 {
					doc = b1700QuoteBindingEmit(t, ctx, map[string]any{"unchanged_block_ids": []string{"summary"}, "replace_blocks": []any{block}}, true)
				}
				item := blockByID(t, doc, "members").Items[0]
				refs := types.AnswerBlockItemCitationRefs(item)
				if len(refs) != 1 || refs[0] < 0 || refs[0] >= len(doc.Citations) ||
					doc.Citations[refs[0]].File != "operation.go" || doc.Citations[refs[0]].Line != tc.line || item.Label != tc.member {
					t.Errorf("attempt %d lost actual %s citation: item=%+v citations=%+v", attempt, tc.kind, item, doc.Citations)
				}
			}
		})
	}
}
