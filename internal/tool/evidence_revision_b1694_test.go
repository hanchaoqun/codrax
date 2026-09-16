package tool

import (
	"encoding/json"
	"reflect"
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/tool/ground"
	"github.com/hanchaoqun/codrax/internal/types"
)

func b1694ToolEvidence(kind types.EvidenceKind, predicate, object string) types.EvidenceItem {
	item := types.EvidenceItem{
		Kind: kind, Scope: types.ScopeLine, Source: "worker.go", LineStart: 2, LineEnd: 2,
		Subject: "Worker", Predicate: predicate, Object: object,
		AnchorKind: types.AnchorDefinition, AnchorSymbol: "Worker",
		GroundingStatus: types.GroundingGrounded, Origin: types.ClaimOriginCurrentRepo,
		Authority: types.AuthorityFactual, Summary: "Same observed source location.",
	}
	item.ID = types.StableEvidenceID(item)
	return item
}

func b1694ToolReports(items []types.EvidenceItem) []ground.Report {
	out := make([]ground.Report, len(items))
	for i, item := range items {
		out[i] = ground.Report{ItemID: item.ID, Status: item.GroundingStatus, OriginalLine: item.LineStart, AdjustedLine: item.LineStart}
	}
	return out
}

func TestB1694PublicEmitSameCoordinateClaimsAreNotAmendments(t *testing.T) {
	const source = "package p\nfunc Worker() int { return 1 }\n"
	ctx := b1698ReadCommentSource(t, "worker.go", source, 0, 100)
	ctx.AnalysisIR = &types.AnalysisIR{}
	definition := map[string]any{
		"evidence_kind": "direct", "scope": "line", "source": "worker.go", "line_start": 2,
		"anchor_kind": "definition", "anchor_symbol": "Worker", "subject": "Worker", "predicate": "defined",
		"summary": "MODEL DEFINITION WORDS", "snippet": "func Worker() int { return 1 }",
	}
	returned := map[string]any{
		"evidence_kind": "direct", "scope": "line", "source": "worker.go", "line_start": 2,
		"anchor_kind": "return", "anchor_symbol": "Worker", "subject": "Worker", "predicate": "returns", "object": "1",
		"summary": "MODEL RETURN WORDS", "snippet": "func Worker() int { return 1 }",
	}
	emitDuplicateOperationAdvisoryItems(t, ctx, definition)
	first := ctx.Mutable.EmittedEvidence()
	if len(first) != 1 || first[0].GroundingStatus != types.GroundingGrounded || first[0].Predicate != "defined" {
		t.Fatalf("real read/definition premise failed: %+v", first)
	}
	result := emitDuplicateOperationAdvisoryItems(t, ctx, returned)
	raw, _ := ctx.Mutable.EmittedEvidenceSince(0)
	if len(raw) != 2 || raw[1].GroundingStatus != types.GroundingGrounded || raw[1].Predicate != "returns" || raw[1].Object != "1" {
		t.Fatalf("real return emission premise failed: %+v", raw)
	}
	if types.EvidenceRevisionKey(raw[0]) != types.EvidenceRevisionKey(raw[1]) || raw[0].ID == raw[1].ID {
		t.Fatalf("fixture needs distinct facts sharing the old revision bucket: %+v", raw)
	}
	if strings.Contains(result.Summary, "Updated 1 existing evidence item") || strings.Contains(result.Summary, "amendment direct") {
		t.Errorf("an independent same-line claim was reported as an amendment: %s", result.Summary)
	}
	got := ctx.Mutable.EmittedEvidence()
	if len(got) != 2 {
		t.Fatalf("definition and return need independent accepted identities: %+v", got)
	}
	for _, item := range got {
		if item.Predicate == "defined" && (item.ID != first[0].ID || item.Summary != "MODEL DEFINITION WORDS") {
			t.Fatalf("new sibling rewrote the accepted definition: %+v", item)
		}
	}
}

func TestB1694PublicSameStableIdentityMetadataStillAmends(t *testing.T) {
	const source = "package p\nfunc Worker() int { return 1 }\n"
	ctx := b1698ReadCommentSource(t, "worker.go", source, 0, 100)
	ctx.AnalysisIR = &types.AnalysisIR{}
	item := map[string]any{
		"evidence_kind": "direct", "scope": "line", "source": "worker.go", "line_start": 2,
		"anchor_kind": "definition", "anchor_symbol": "Worker", "subject": "Worker", "predicate": "defined",
		"summary": "MODEL ORIGINAL SUMMARY", "snippet": "func Worker() int { return 1 }",
	}
	emitDuplicateOperationAdvisoryItems(t, ctx, item)
	item["summary"], item["surface_terms"] = "MODEL AMENDED SUMMARY", []string{"Worker"}
	result := emitDuplicateOperationAdvisoryItems(t, ctx, item)
	raw, total := ctx.Mutable.EmittedEvidenceSince(0)
	if total != 2 || len(raw) != 2 || raw[0].ID != raw[1].ID ||
		raw[0].GroundingStatus != types.GroundingGrounded || raw[1].GroundingStatus != types.GroundingGrounded {
		t.Fatalf("public metadata correction must actually keep the same grounded stable identity: %+v", raw)
	}
	got := ctx.Mutable.EmittedEvidence()
	if len(got) != 1 || got[0].ID != raw[0].ID || !reflect.DeepEqual(got[0].SurfaceTerms, []string{"Worker"}) ||
		!strings.Contains(got[0].Summary, "MODEL ORIGINAL SUMMARY") || !strings.Contains(got[0].Summary, "MODEL AMENDED SUMMARY") {
		t.Fatalf("legal metadata amendment lost identity or accepted model summaries: %+v", got)
	}
	if !strings.Contains(result.Summary, "Updated 1 existing evidence item") || strings.Contains(result.Summary, "duplicate direct") {
		t.Fatalf("same-identity metadata amendment must still be actionable progress: %s", result.Summary)
	}
}

func TestB1694ToolAmendmentAndLookupRespectClaimSiblings(t *testing.T) {
	definition := b1694ToolEvidence(types.EvidenceDirect, "defined", "")
	definition.RequestedDimensionIndices = []int{1}
	returned := b1694ToolEvidence(types.EvidenceDirect, "returns", "1")
	returned.AnchorKind = types.AnchorReturn
	returned.RequestedDimensionIndices = []int{3}
	if types.EvidenceRevisionKey(definition) != types.EvidenceRevisionKey(returned) {
		t.Fatal("fixture must hit the same revision bucket")
	}
	if amended := emitEvidenceAmendedItems([]types.EvidenceItem{definition}, []types.EvidenceItem{returned}); len(amended) != 0 {
		t.Errorf("independent typed return was classified as an amendment: %+v", amended)
	}
	// A historical spelling may have a different stable ID after source-origin
	// backfill. The unique compatible definition may be recovered, not the
	// last same-coordinate sibling's operation indices.
	selected := definition
	selected.Origin, selected.Authority = types.ClaimOriginUnknown, types.AuthorityUnknown
	selected.ID = types.StableEvidenceID(selected)
	selected.RequestedDimensionIndices = nil
	for _, rows := range [][]types.EvidenceItem{{definition, returned}, {returned, definition}} {
		before, _ := json.Marshal(rows)
		got := emitEvidenceOperationAdvisoryItems(nil, []types.EvidenceItem{selected}, rows)
		if len(got) != 1 || got[0].Predicate != "defined" || !reflect.DeepEqual(got[0].RequestedDimensionIndices, []int{1}) {
			t.Errorf("current selection borrowed a sibling's operation ownership: %+v", got)
		}
		after, _ := json.Marshal(rows)
		if string(before) != string(after) {
			t.Fatal("lookup mutated accepted records")
		}
	}
	if got := emitEvidenceOperationAdvisoryItems(nil, []types.EvidenceItem{selected}, []types.EvidenceItem{returned}); len(got) != 0 {
		t.Errorf("missing selected claim must not recover a different same-line fact: %+v", got)
	}
}

func TestB1694ToolSparseRevisionRequiresUniqueCompatibleClaim(t *testing.T) {
	sparse := b1694ToolEvidence(types.EvidenceRegistration, "constructs", "Sink")
	sparse.Subject, sparse.AnchorKind, sparse.Condition = "", types.AnchorReturn, `kind == "sink"`
	sparse.ID = types.StableEvidenceID(sparse)
	complete := sparse
	complete.Subject, complete.Condition = "Factory.create", ""
	complete.ID = types.StableEvidenceID(complete)
	if amended := emitEvidenceAmendedItems([]types.EvidenceItem{sparse}, []types.EvidenceItem{complete}); len(amended) != 1 {
		t.Fatalf("unique endpoint completion must remain a legitimate amendment: %+v", amended)
	}
	other := complete
	other.Subject = "OtherFactory.create"
	other.ID = types.StableEvidenceID(other)
	for _, rows := range [][]types.EvidenceItem{{complete, other}, {other, complete}} {
		if amended := emitEvidenceAmendedItems(rows, []types.EvidenceItem{sparse}); len(amended) != 0 {
			t.Errorf("ambiguous sparse row chose a same-coordinate owner: %+v", amended)
		}
		if got := emitEvidenceOperationAdvisoryItems(nil, []types.EvidenceItem{sparse}, rows); len(got) != 0 {
			t.Errorf("ambiguous sparse lookup borrowed an owner: %+v", got)
		}
		// Exact identity wins over an ambiguous revision bucket.
		if got := emitEvidenceOperationAdvisoryItems(nil, []types.EvidenceItem{complete}, rows); len(got) != 1 || got[0].ID != complete.ID {
			t.Errorf("exact selected identity was obscured by sibling ambiguity: %+v", got)
		}
	}
}

func TestB1694ToolSameBatchReplayAndMetadataRemainStable(t *testing.T) {
	prior := b1694ToolEvidence(types.EvidenceDirect, "defined", "")
	prior.Source = "unrelated.go"
	prior.ID = types.StableEvidenceID(prior)
	definition := b1694ToolEvidence(types.EvidenceDirect, "defined", "")
	returned := b1694ToolEvidence(types.EvidenceDirect, "returns", "1")
	returned.AnchorKind = types.AnchorReturn
	items := []types.EvidenceItem{definition, returned, definition, returned}
	before, _ := json.Marshal(items)
	kept, reports, duplicates := filterNoopDuplicateEmitEvidence([]types.EvidenceItem{prior}, items, b1694ToolReports(items))
	if len(kept) != 2 || len(reports) != 2 || len(duplicates) != 2 || kept[0].ID != definition.ID || kept[1].ID != returned.ID {
		t.Fatalf("same-batch replay changed distinct identities or report alignment: kept=%+v reports=%+v duplicates=%+v", kept, reports, duplicates)
	}
	after, _ := json.Marshal(items)
	if string(before) != string(after) {
		t.Fatal("duplicate filtering mutated caller items")
	}
	corrected := definition
	corrected.AnchorKind = types.AnchorCall
	corrected.RequestedDimensionIndices = []int{1}
	kept, reports, duplicates = filterNoopDuplicateEmitEvidence([]types.EvidenceItem{definition}, []types.EvidenceItem{corrected}, b1694ToolReports([]types.EvidenceItem{corrected}))
	if len(kept) != 1 || len(reports) != 1 || len(duplicates) != 0 || len(emitEvidenceAmendedItems([]types.EvidenceItem{definition}, kept)) != 1 {
		t.Fatal("same-ID mutable anchor/operation-index correction must not become a no-op")
	}
}

func TestB1694ToolSameBatchSparseCompletionKeepsOneLookupCandidate(t *testing.T) {
	prior := b1694ToolEvidence(types.EvidenceDirect, "defined", "")
	prior.Source, prior.ID = "unrelated.go", "prior-unrelated"
	sparse := b1694ToolEvidence(types.EvidenceRegistration, "constructs", "Sink")
	sparse.Subject, sparse.AnchorKind, sparse.Condition = "", types.AnchorReturn, `kind == "sink"`
	sparse.ID = types.StableEvidenceID(sparse)
	complete := sparse
	complete.Subject, complete.Condition = "Factory.create", ""
	complete.ID = types.StableEvidenceID(complete)
	items := []types.EvidenceItem{sparse, complete, complete}
	kept, reports, duplicates := filterNoopDuplicateEmitEvidence([]types.EvidenceItem{prior}, items, b1694ToolReports(items))
	if len(kept) != 2 || len(reports) != 2 || len(duplicates) != 1 || duplicates[0].ID != complete.ID {
		t.Fatalf("an accepted in-batch completion must replace its lookup slot, not create false ambiguity: kept=%+v reports=%+v duplicates=%+v", kept, reports, duplicates)
	}
	if kept[0].ID != sparse.ID || kept[1].ID != complete.ID || reports[0].ItemID != sparse.ID || reports[1].ItemID != complete.ID {
		t.Fatal("filter must preserve original accepted IDs/report alignment; canonical buffer merge remains its own consumer")
	}
}

func TestB1694ToolCrossIDMetadataConflictsDoNotBorrowOwners(t *testing.T) {
	for _, tc := range []struct {
		name string
		edit func(*types.EvidenceItem, *types.EvidenceItem)
	}{
		{"known_owner", func(a, b *types.EvidenceItem) { a.OwnerSymbol, b.OwnerSymbol = "OwnerA", "OwnerB" }},
		{"known_condition", func(a, b *types.EvidenceItem) { a.Condition, b.Condition = "first", "second" }},
		{"selector_owner", func(a, b *types.EvidenceItem) {
			a.SelectorApplication = &types.EvidenceSelectorApplication{Owner: "RegistryA", Literal: "sink"}
			b.SelectorApplication = &types.EvidenceSelectorApplication{Owner: "RegistryB", Literal: "sink"}
		}},
		{"selector_literal", func(a, b *types.EvidenceItem) {
			a.SelectorApplication = &types.EvidenceSelectorApplication{Owner: "Registry", Literal: "a"}
			b.SelectorApplication = &types.EvidenceSelectorApplication{Owner: "Registry", Literal: "b"}
		}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			accepted := b1694ToolEvidence(types.EvidenceDirect, "returns", "Sink")
			accepted.AnchorKind = types.AnchorReturn
			accepted.RequestedDimensionIndices = []int{3}
			incoming := accepted
			incoming.Origin, incoming.Authority = types.ClaimOriginUnknown, types.AuthorityUnknown
			incoming.RequestedDimensionIndices = nil
			tc.edit(&accepted, &incoming)
			accepted.ID, incoming.ID = types.StableEvidenceID(accepted), types.StableEvidenceID(incoming)
			if accepted.ID == incoming.ID || types.EvidenceRevisionKey(accepted) != types.EvidenceRevisionKey(incoming) {
				t.Fatal("fixture must exercise distinct IDs in the same revision bucket")
			}
			if got := emitEvidenceAmendedItems([]types.EvidenceItem{accepted}, []types.EvidenceItem{incoming}); len(got) != 0 {
				t.Errorf("conflicting candidate was called an amendment: %+v", got)
			}
			if got := emitEvidenceOperationAdvisoryItems(nil, []types.EvidenceItem{incoming}, []types.EvidenceItem{accepted}); len(got) != 0 {
				t.Errorf("conflicting candidate borrowed operation ownership: %+v", got)
			}
			kept, _, duplicates := filterNoopDuplicateEmitEvidence([]types.EvidenceItem{accepted}, []types.EvidenceItem{incoming}, nil)
			if len(kept) != 1 || len(duplicates) != 0 {
				t.Fatal("conflicting candidate must remain independent, not a no-progress duplicate")
			}
		})
	}
}
