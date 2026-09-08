package tool

import (
	"encoding/json"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/types"
)

func b1617CitationGenerationFixture(t *testing.T) (*types.BusContext, []types.Citation) {
	t.Helper()
	repo, err := filepath.Abs("../../eval/fixtures/cpp-sink-hierarchy")
	if err != nil {
		t.Fatal(err)
	}
	mu := types.NewMutableState("explain source observations")
	mu.SetRepoRoot(repo)
	ctx := &types.BusContext{RepoRoot: repo, WorkDir: t.TempDir(), Mutable: mu,
		AnalysisIR: &types.AnalysisIR{RequestModel: types.RequestModel{Intent: types.IntentExplain}}}
	pool := []types.Citation{
		{File: "src/logger.cpp", Line: 36}, {File: "src/logger.cpp", Line: 33},
		{File: "src/logger.cpp", Line: 35}, {File: "include/logx/console_sink.hpp", Line: 10},
		{File: "include/logx/console_sink.hpp", Line: 8}, {File: "src/registry.cpp", Line: 17},
		{File: "src/registry.cpp", Line: 20}, {File: "src/registry.cpp", Line: 23},
	}
	ctx.EvidenceItems = []types.EvidenceItem{{ID: "ev-added-guard", Kind: types.EvidenceConditional,
		Source: "src/logger.cpp", LineStart: 30, Scope: types.ScopeLine, AnchorKind: types.AnchorCondition,
		Subject: "Logger.log", AnchorSymbol: "if (sink_ == nullptr || level < min_level_)",
		Condition: "sink_ == nullptr || level < min_level_", Snippet: "if (sink_ == nullptr || level < min_level_) return;",
		GroundingStatus: types.GroundingGrounded, Origin: types.ClaimOriginCurrentRepo}}
	mu.AppendEvidence(ctx.EvidenceItems)
	view := types.BuildAnswerSemanticViewForBusContext(ctx)
	if view == nil || !view.ItemEvidenceIdentityAvailable {
		t.Fatal("fixture must expose stable evidence IDs")
	}
	return ctx, pool
}

func b1617Marshal(t *testing.T, value any) json.RawMessage {
	t.Helper()
	raw, err := json.Marshal(value)
	if err != nil {
		t.Fatal(err)
	}
	return raw
}

func b1617GrowthBlock(withBadID bool) map[string]any {
	items := []map[string]any{{"id": "growth", "label": "已读前置条件", "text": "保留所选条件事实。", "evidence_ids": []string{"ev-added-guard"}}}
	if withBadID {
		items = append(items, map[string]any{"id": "repair", "label": "待修条目", "text": "只修此处。", "evidence_ids": []string{"ev-missing"}})
	}
	return map[string]any{"id": "work", "kind": "bullet_list", "items": items}
}

// Real r1036 transport shape: the first submission has eight citations and an
// invalid ninth reference. A different row grows the pool before the rejected
// draft becomes a patch base; the next patch leaves the old row unchanged.
func b1617RejectedGrowingPool(t *testing.T) (*types.BusContext, *types.AnswerDocumentV2) {
	t.Helper()
	ctx, pool := b1617CitationGenerationFixture(t)
	res, err := (&EmitAnswerDocument{}).Execute(ctx, b1617Marshal(t, map[string]any{
		"citations": pool, "blocks": []any{
			map[string]any{"id": "summary", "kind": "summary", "text": "Model-authored source observations."},
			b1617GrowthBlock(true),
			map[string]any{"id": "history", "kind": "section", "items": []any{map[string]any{
				"id": "old", "label": "原观察", "text": "此项不是新添条件的声明。", "citation_ref": 8}}},
		}}))
	if err != nil || res.Success || !strings.Contains(res.Summary, "ev-missing") {
		t.Fatalf("first full emit must reject the unrelated missing ID: err=%v res=%+v", err, res)
	}
	first := ctx.Mutable.LastRejectedAnswerDocumentV2()
	if first == nil || len(first.Citations) <= 8 || first.Citations[8].File != "src/logger.cpp" || first.Citations[8].Line != 30 {
		t.Fatalf("real normalization must grow the rejected pool at old invalid index 8: %+v", first)
	}
	old := blockByID(t, first, "history").Items[0]
	if !reflect.DeepEqual(old.CitationRefsModelSubmittedValues, []int{8}) || len(old.EvidenceIDs) != 0 || len(types.AnswerBlockItemCitationRefs(old)) != 0 {
		t.Fatalf("first generation must preserve raw invalid index for audit but quarantine active support: %+v", old)
	}
	return ctx, first
}

func TestB1617ActualRejectedFullThenPatchCannotReviveOldOutOfRangeCitation(t *testing.T) {
	ctx, first := b1617RejectedGrowingPool(t)
	old := blockByID(t, first, "history").Items[0]
	res, err := (&EmitAnswerDocumentPatch{}).Execute(ctx, b1617Marshal(t, map[string]any{
		"unchanged_block_ids": []string{"summary", "history"}, "replace_blocks": []any{b1617GrowthBlock(false)},
	}))
	if err != nil || !res.Success {
		t.Fatalf("unrelated repair should complete: err=%v res=%+v", err, res)
	}
	got := blockByID(t, ctx.Mutable.AnswerDocumentV2(), "history").Items[0]
	if len(got.EvidenceIDs) != 0 || len(types.AnswerBlockItemCitationRefs(got)) != 0 {
		t.Fatalf("unchanged old invalid index was resurrected into newly appended evidence: %+v", got)
	}
	if got.Label != old.Label || got.Text != old.Text || !reflect.DeepEqual(got.CitationRefsModelSubmittedValues, []int{8}) {
		t.Fatalf("repair must preserve model text and original submission audit: before=%+v after=%+v", old, got)
	}
}

func TestB1617ActualNewSubmittedReferenceCanSelectCurrentPool(t *testing.T) {
	for _, operation := range []string{"replace", "add"} {
		t.Run(operation, func(t *testing.T) {
			ctx, _ := b1617RejectedGrowingPool(t)
			newBlock := map[string]any{"id": "history", "kind": "section", "items": []any{map[string]any{
				"id": "new-selection", "label": "当前选择", "text": "本次明确选用已读条件。", "citation_ref": 8}}}
			patch := map[string]any{"unchanged_block_ids": []string{"summary"}, "replace_blocks": []any{b1617GrowthBlock(false), newBlock}}
			if operation == "add" {
				newBlock["id"] = "new-notes"
				patch["replace_blocks"] = []any{b1617GrowthBlock(false)}
				patch["unchanged_block_ids"] = []string{"summary", "history"}
				patch["add_blocks"] = []any{newBlock}
			}
			res, err := (&EmitAnswerDocumentPatch{}).Execute(ctx, b1617Marshal(t, patch))
			if err != nil || !res.Success {
				t.Fatalf("explicit new selection must remain legal: err=%v res=%+v", err, res)
			}
			got := blockByID(t, ctx.Mutable.AnswerDocumentV2(), newBlock["id"].(string)).Items[0]
			if !reflect.DeepEqual(got.EvidenceIDs, []string{"ev-added-guard"}) || got.Text != "本次明确选用已读条件。" {
				t.Fatalf("new invocation index must select its current pool, not be tainted by old invalid index: %+v", got)
			}
		})
	}
}

func TestB1617AmbiguousOldSelectionCannotFollowReorderedPool(t *testing.T) {
	ctx, _ := b1617CitationGenerationFixture(t)
	oldCitation := types.Citation{File: "src/registry.cpp", Line: 17}
	for _, id := range []string{"ev-original-a", "ev-original-b"} {
		ctx.EvidenceItems = append(ctx.EvidenceItems, types.EvidenceItem{ID: id, Kind: types.EvidenceConditional,
			Source: oldCitation.File, LineStart: oldCitation.Line, Scope: types.ScopeLine, AnchorKind: types.AnchorCondition,
			Subject: "SinkRegistry.create", AnchorSymbol: "kind", GroundingStatus: types.GroundingGrounded, Origin: types.ClaimOriginCurrentRepo})
	}
	ctx.Mutable.AppendEvidence(ctx.EvidenceItems)
	pctx := newPreEmitCheckContext(ctx)
	view := &types.AnswerSemanticView{ItemEvidenceIdentityAvailable: true}
	doc := &types.AnswerDocumentV2{Citations: []types.Citation{oldCitation}, Blocks: []types.AnswerBlock{{
		ID: "old", Kind: types.BlockSection, Items: []types.AnswerBlockItem{{ID: "i", Text: "unchanged", CitationRef: 0}},
	}}}
	markModelSubmittedItemCitationRefs(doc)
	markModelSubmittedItemEvidenceIDAdoptionRequired(doc, view, pctx)
	if !doc.Blocks[0].Items[0].CitationRefsEvidenceIDAdoptionRequired || normalizeUniqueModelCitationRefsToEvidenceIDs(doc, view, pctx) != 0 {
		t.Fatal("original exact source must require model disambiguation, not choose a row")
	}
	doc.Citations[0] = types.Citation{File: "src/logger.cpp", Line: 30}
	markModelSubmittedItemEvidenceIDAdoptionRequired(doc, view, pctx)
	if got := normalizeUniqueModelCitationRefsToEvidenceIDs(doc, view, pctx); got != 0 || len(doc.Blocks[0].Items[0].EvidenceIDs) != 0 {
		t.Fatalf("old ambiguous origin followed reordered pool to unrelated unique fact: fixed=%d item=%+v", got, doc.Blocks[0].Items[0])
	}
	if hints := preCheckItemEvidenceIdentity(doc, view, pctx); len(hints) != 1 || hints[0].HardSignal != preEmitHardSignalTypedItemEvidenceIdentity {
		t.Fatalf("original ambiguity must remain on the original precise repair lane: %+v", hints)
	}
}

func TestB1617SubmissionSnapshotIsClonedAndNotSerialized(t *testing.T) {
	ctx, doc := b1617RejectedGrowingPool(t)
	before := blockByID(t, doc, "history").Items[0]
	if !before.CitationRefsEvidenceIDAdoptionEvaluated || len(before.CitationRefsModelSubmittedCitations) != 1 || before.CitationRefsModelSubmittedCitations[0].File != "" {
		t.Fatalf("invalid original selection must retain a frozen zero source slot: %+v", before)
	}
	doc.Blocks[2].Items[0].CitationRefsModelSubmittedCitations[0] = types.Citation{File: "reader-corruption", Line: 1}
	again := blockByID(t, ctx.Mutable.LastRejectedAnswerDocumentV2(), "history").Items[0]
	if again.CitationRefsModelSubmittedCitations[0].File != "" {
		t.Fatalf("getter snapshot aliases mutable state: %+v", again)
	}
	ctx.Mutable.SetAnswerDocumentV2WithMutation(types.MutationReplaceAll, doc)
	doc.Blocks[2].Items[0].CitationRefsModelSubmittedCitations[0].File = "writer-corruption"
	stored := blockByID(t, ctx.Mutable.AnswerDocumentV2(), "history").Items[0]
	if stored.CitationRefsModelSubmittedCitations[0].File != "reader-corruption" {
		t.Fatalf("setter snapshot aliases caller slice: %+v", stored)
	}
	raw := string(b1617Marshal(t, stored))
	if strings.Contains(raw, "reader-corruption") || strings.Contains(raw, "AdoptionEvaluated") || strings.Contains(raw, "ModelSubmittedCitations") {
		t.Fatalf("run-local submission provenance leaked onto wire: %s", raw)
	}
}

func TestB1617ActualPatchSnapshotsDeclaredPoolBeforePreservationRemap(t *testing.T) {
	ctx, _ := b1617CitationGenerationFixture(t)
	ctx.Mutable.SetAnswerDocumentV2WithMutation(types.MutationReplaceAll, &types.AnswerDocumentV2{
		DocumentModel: "v2", Citations: []types.Citation{{File: "src/registry.cpp", Line: 17}},
		Blocks: []types.AnswerBlock{
			{ID: "summary", Kind: types.BlockSummary, Text: "Original summary."},
			{ID: "old", Kind: types.BlockSection, Items: []types.AnswerBlockItem{{ID: "original", Label: "原引用", CitationRef: 0}}},
		},
	})
	res, err := (&EmitAnswerDocumentPatch{}).Execute(ctx, b1617Marshal(t, map[string]any{
		"unchanged_block_ids": []string{"summary", "old"},
		"replace_citations":   []types.Citation{{File: "src/logger.cpp", Line: 30}},
		"add_blocks": []any{map[string]any{"id": "new", "kind": "section", "items": []any{map[string]any{
			"id": "selected", "label": "本次明确所选来源", "citation_ref": 0}}}},
	}))
	if err != nil || !res.Success {
		t.Fatalf("declared replacement-pool selection should survive preserved-pool tolerance: err=%v res=%+v", err, res)
	}
	doc := ctx.Mutable.AnswerDocumentV2()
	got := blockByID(t, doc, "new").Items[0]
	if !reflect.DeepEqual(got.EvidenceIDs, []string{"ev-added-guard"}) || len(got.CitationRefsModelSubmittedCitations) != 1 ||
		got.CitationRefsModelSubmittedCitations[0].File != "src/logger.cpp" || got.CitationRefsModelSubmittedCitations[0].Line != 30 {
		t.Fatalf("source snapshot used remapped inherited pool rather than submitted pool: %+v", got)
	}
	if citation := doc.Citations[got.CitationRef]; citation.File != "src/logger.cpp" || citation.Line != 30 {
		t.Fatalf("new exact evidence selection was rebound to old index-zero source: %+v", citation)
	}
}

func TestB1617LegacyUnknownOrPartlyInvalidSubmissionCannotMintSnapshot(t *testing.T) {
	for _, form := range []string{"legacy_unknown", "partly_invalid"} {
		t.Run(form, func(t *testing.T) {
			ctx, _ := b1617CitationGenerationFixture(t)
			pctx := newPreEmitCheckContext(ctx)
			view := &types.AnswerSemanticView{ItemEvidenceIdentityAvailable: true}
			doc := &types.AnswerDocumentV2{Citations: []types.Citation{{File: "src/logger.cpp", Line: 30}},
				Blocks: []types.AnswerBlock{{ID: "old", Kind: types.BlockSection, Items: []types.AnswerBlockItem{{
					ID: "i", CitationRef: 0, CitationRefs: []int{8}, CitationRefsModelSubmitted: true, CitationRefsModelSubmittedValues: []int{0},
				}}}}}
			if form == "partly_invalid" {
				markModelSubmittedItemCitationRefs(doc)
			}
			markModelSubmittedItemEvidenceIDAdoptionRequired(doc, view, pctx)
			for len(doc.Citations) <= 8 {
				doc.Citations = append(doc.Citations, types.Citation{File: "src/logger.cpp", Line: 30})
			}
			markModelSubmittedItemEvidenceIDAdoptionRequired(doc, view, pctx)
			if fixed := normalizeUniqueModelCitationRefsToEvidenceIDs(doc, view, pctx); fixed != 0 || len(doc.Blocks[0].Items[0].EvidenceIDs) != 0 || doc.Blocks[0].Items[0].CitationRefsEvidenceIDAdoptionRequired {
				t.Fatalf("unproven submission origin acquired new authority after pool growth: fixed=%d item=%+v", fixed, doc.Blocks[0].Items[0])
			}
		})
	}
}

func TestB1617UnsubmittedCitationProvenanceRemainsUnchanged(t *testing.T) {
	for _, item := range []types.AnswerBlockItem{
		{ID: "unowned", Text: "Model-independent display attachment", CitationRef: types.CitationRefUnset},
		{ID: "system-ref", Text: "System-bound source", CitationRef: 0, EvidenceIDs: []string{"ev-added-guard"}},
	} {
		t.Run(item.ID, func(t *testing.T) {
			ctx, _ := b1617CitationGenerationFixture(t)
			doc := &types.AnswerDocumentV2{Citations: []types.Citation{{File: "src/logger.cpp", Line: 30}},
				Blocks: []types.AnswerBlock{{ID: "sibling", Kind: types.BlockSection, Items: []types.AnswerBlockItem{item}}}}
			markModelSubmittedItemEvidenceIDAdoptionRequired(doc, &types.AnswerSemanticView{ItemEvidenceIdentityAvailable: true}, newPreEmitCheckContext(ctx))
			if !reflect.DeepEqual(doc.Blocks[0].Items[0], item) {
				t.Fatalf("citation evaluation mutated an item without a model citation submission: before=%+v after=%+v", item, doc.Blocks[0].Items[0])
			}
		})
	}
}
