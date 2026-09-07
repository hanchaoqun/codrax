package tool

import (
	"encoding/json"
	"fmt"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/types"
)

// r1033 selected these exact definition evidence IDs and submitted no row IDs.
// The same principal members can also be mechanically addressed as row IDs;
// that second available carrier must not become a second selected owner.
func b1600PrincipalOwnerFixture(t *testing.T) (*types.BusContext, *types.AnswerDocumentV2) {
	t.Helper()
	repo, err := filepath.Abs("../../eval/fixtures/cpp-sink-hierarchy")
	if err != nil {
		t.Fatal(err)
	}
	mu := types.NewMutableState("Sink 的具体实现类有哪些")
	mu.SetRepoRoot(repo)
	ctx := &types.BusContext{
		RepoRoot: repo, WorkDir: t.TempDir(), Mutable: mu,
		AnalysisIR: &types.AnalysisIR{RequestModel: types.RequestModel{
			Intent:     types.IntentEnumerate,
			Predicates: types.SemanticPredicates{IsCategoryEnumeration: true},
		}},
	}
	doc := &types.AnswerDocumentV2{Blocks: []types.AnswerBlock{
		{ID: "summary", Kind: types.BlockSummary, Text: "ConsoleSink and FileSink inherit Sink; RotatingSink inherits FileSink."},
		{ID: "members", Kind: types.BlockOrderedList, SurfaceRole: types.SurfacePrincipal,
			Title: "Sink implementations", FacetIDs: []string{string(types.FacetEnumerationItem)},
			ClaimUses: []types.RenderedClaimUse{{ClaimForm: types.ClaimDefinitionFact}}},
	}}
	fact := types.AnswerAggregateFact{
		Kind: types.AnswerAggregateMemberSet, Label: "Sink implementations", Value: "3",
		Role: types.AnswerAggregateRolePrincipalAnswer,
	}
	for _, member := range []struct {
		name, file, base, evidenceID string
		line                         int
	}{
		{"ConsoleSink", "console_sink.hpp", "Sink", "ev-33a12d0dca17e6c0", 8},
		{"FileSink", "file_sink.hpp", "Sink", "ev-c11a6e039a6fecbe", 10},
		{"RotatingSink", "rotating_sink.hpp", "FileSink", "ev-9bc9df9aba1e026", 10},
	} {
		path := "include/logx/" + member.file
		quote := fmt.Sprintf("class %s : public %s {", member.name, member.base)
		ctx.EvidenceItems = append(ctx.EvidenceItems, types.EvidenceItem{
			ID: member.evidenceID, Kind: types.EvidenceDirect, Source: path,
			LineStart: member.line, Scope: types.ScopeLine, AnchorKind: types.AnchorDefinition,
			AnchorSymbol: member.name, Subject: member.name, Snippet: quote,
			GroundingStatus: types.GroundingGrounded, Origin: types.ClaimOriginCurrentRepo,
		})
		doc.Citations = append(doc.Citations, types.Citation{File: path, Line: member.line, Quote: quote})
		doc.Blocks[1].Items = append(doc.Blocks[1].Items, types.AnswerBlockItem{
			ID: member.name, Label: member.name,
			Text:        fmt.Sprintf("Inherits %s; defined at %s:%d.", member.base, path, member.line),
			EvidenceIDs: []string{member.evidenceID}, CitationRef: types.CitationRefUnset,
		})
		fact.Members = append(fact.Members, member.name)
		fact.SupportRefs = append(fact.SupportRefs, fmt.Sprintf("%s @ %s:%d", member.name, path, member.line))
	}
	mu.AppendEvidence(ctx.EvidenceItems)
	mu.SetInvestigationAggregateFacts([]types.AnswerAggregateFact{fact})
	mu.RetainInvestigationAggregateFacts()
	if sets := preEmitSourceInventoryTypedPrincipalSets(ctx); len(sets) == 0 {
		t.Fatal("fixture must expose the competing principal-row carrier")
	}
	return ctx, doc
}

func b1600Marshal(t *testing.T, value any) json.RawMessage {
	t.Helper()
	raw, err := json.Marshal(value)
	if err != nil {
		t.Fatal(err)
	}
	return raw
}

func b1600AssertSelectedEvidence(t *testing.T, got *types.AnswerDocumentV2, want []types.AnswerBlock) {
	t.Helper()
	if got == nil {
		t.Fatal("accepted document missing")
	}
	for _, block := range want {
		var actual *types.AnswerBlock
		for i := range got.Blocks {
			if got.Blocks[i].ID == block.ID {
				actual = &got.Blocks[i]
				break
			}
		}
		if actual == nil || actual.Text != block.Text || actual.Title != block.Title || len(actual.Items) != len(block.Items) {
			t.Fatalf("model block changed or disappeared: want=%+v got=%+v", block, actual)
		}
		for i, item := range block.Items {
			row := actual.Items[i]
			if row.ID != item.ID || row.Label != item.Label || row.Text != item.Text ||
				!reflect.DeepEqual(row.Cells, item.Cells) || !reflect.DeepEqual(row.EvidenceIDs, item.EvidenceIDs) || row.SourceInventoryRowID != "" {
				t.Fatalf("model-selected evidence/content acquired a competing row owner: want=%+v got=%+v", item, row)
			}
			if row.CitationRef < 0 || row.CitationRef >= len(got.Citations) {
				t.Fatalf("selected evidence lost its citable source: %+v", row)
			}
			citation := got.Citations[row.CitationRef]
			if !strings.Contains(item.Text+strings.Join(item.Cells, " "), fmt.Sprintf("%s:%d", citation.File, citation.Line)) {
				t.Fatalf("selected evidence was rebound away from its exact definition: item=%+v citation=%+v", item, citation)
			}
		}
	}
}

func TestB1600ActualEmitKeepsSourceOwnerAndRejectsModelDualOwner(t *testing.T) {
	for _, selected := range []string{"source_row", "dual_owner", "legacy_citation"} {
		t.Run(selected, func(t *testing.T) {
			ctx, doc := b1600PrincipalOwnerFixture(t)
			for i := range doc.Blocks[1].Items {
				item := &doc.Blocks[1].Items[i]
				item.EvidenceIDs = nil
				item.CitationRef = i
			}
			if n := normalizePrincipalEnumerationRowIDsByExactIdentityAndCitationWithContext(doc, ctx); n != 3 {
				t.Fatalf("expected three exact source owners, got %d", n)
			}
			selectedRows := make([]string, 3)
			for i := range doc.Blocks[1].Items {
				item := &doc.Blocks[1].Items[i]
				selectedRows[i] = item.SourceInventoryRowID
				if selected == "dual_owner" {
					item.EvidenceIDs = []string{ctx.EvidenceItems[i].ID}
				} else if selected == "legacy_citation" {
					item.SourceInventoryRowID = ""
				}
			}
			res, err := (&EmitAnswerDocument{}).Execute(ctx, b1600Marshal(t, map[string]any{"blocks": doc.Blocks, "citations": doc.Citations}))
			if err != nil {
				t.Fatal(err)
			}
			if selected == "dual_owner" {
				if res.Success || !strings.Contains(res.Summary, "cannot both select") || ctx.Mutable.AnswerDocumentV2() != nil {
					t.Fatalf("model-authored dual selection must fail without persisting an answer: %+v", res)
				}
				return
			}
			if !res.Success {
				t.Fatalf("legitimate source-only or legacy citation must stay valid: %s", res.Summary)
			}
			got := ctx.Mutable.AnswerDocumentV2()
			for i, item := range got.Blocks[1].Items {
				validRow := item.SourceInventoryRowID == selectedRows[i] && len(item.EvidenceIDs) == 0
				// The existing earlier legacy-citation migration can already
				// select exact evidence IDs before the row binder runs.
				validLegacyEvidence := selected == "legacy_citation" && item.SourceInventoryRowID == "" &&
					reflect.DeepEqual(item.EvidenceIDs, []string{ctx.EvidenceItems[i].ID})
				if (!validRow && !validLegacyEvidence) || item.Text != doc.Blocks[1].Items[i].Text {
					t.Fatalf("source owner or model text changed: %+v", item)
				}
				if item.CitationRef < 0 || item.CitationRef >= len(got.Citations) || !preEmitCitationSameLocation(got.Citations[item.CitationRef], doc.Citations[i]) {
					t.Fatalf("legacy or row carrier lost its exact source: %+v", item)
				}
			}
		})
	}
}

func TestB1600ExplicitEvidenceDoesNotWaiveRequiredSourceInventoryRow(t *testing.T) {
	ctx, doc := b1600PrincipalOwnerFixture(t)
	ctx.AnalysisIR.RequestModel.SourceInventoryProfile = &types.SourceInventoryProfile{
		IsSourceInventory: true, TargetRoles: []types.AnswerCandidateRole{types.AnswerCandidateRoleType},
	}
	set := types.SourceInventoryObservationSet{Role: types.AnswerCandidateRoleType, Complete: true, Count: 3, Total: 3}
	for i, evidence := range ctx.EvidenceItems {
		set.Members = append(set.Members, types.SourceInventoryObservationMember{
			Name: evidence.Subject, Role: types.AnswerCandidateRoleType, File: evidence.Source,
			Line: evidence.LineStart, CoverageState: types.SourceInventoryCoverageObserved,
		})
		doc.Blocks[1].Items[i].CitationRef = i
	}
	ctx.Mutable.SetSourceInventoryObservation(types.SourceInventoryObservation{
		Active: true, Complete: true, Scopes: []string{"."}, Sets: []types.SourceInventoryObservationSet{set},
	})
	if !sourceInventoryPrincipalAnswerIsModelOwned(ctx) {
		t.Fatal("negative fixture must activate the existing required-row contract")
	}
	before := b1600Marshal(t, doc)
	if fixed := normalizePrincipalEnumerationRowIDsByExactIdentityAndCitationWithContext(doc, ctx); fixed != 0 || string(before) != string(b1600Marshal(t, doc)) {
		t.Fatal("a required-row contract must not rewrite a different explicitly selected owner")
	}
	if hints := preCheckSourceInventoryRowIDBindings(doc, ctx); len(hints) != 3 {
		t.Fatalf("evidence selection must not waive the existing source inventory row contract: %+v", hints)
	} else {
		for _, hint := range hints {
			if !hint.ForceHard || !strings.Contains(hint.ExpectedShape, "row_id") {
				t.Fatalf("required source-row identity lost its precise gate: %+v", hint)
			}
		}
	}
}

func TestB1600R1033NormalizedRelationProfileAndOriginalFirstEmit(t *testing.T) {
	ctx, _ := b1600PrincipalOwnerFixture(t)
	rm := &ctx.AnalysisIR.RequestModel
	// These are the ownership-relevant raw emit_analysis fields from r1033,
	// not a hand-authored false/absent profile. The production analyzer's
	// typed relation normalization emitted the warning at log line 621.
	rm.PredicateAxis = types.AxisImplement
	rm.Predicates.IsRelationalLookup = true
	rm.SourceInventoryProfile = &types.SourceInventoryProfile{
		IsSourceInventory: true, TargetRoles: []types.AnswerCandidateRole{types.AnswerCandidateRoleType},
		RequestedFields: []types.SourceInventoryRequestedField{types.SourceInventoryFieldName, types.SourceInventoryFieldLocation},
		SourceQuotes:    []string{"Sink 的具体实现类有哪些", "请给出每个实现类的定义文件"}, Confidence: .9,
		Rationale: "用户要求枚举 Sink 的所有实现类（FileSink、RotatingSink、ConsoleSink）并给出定义文件。",
	}
	if dropped, warning := dropSourceInventoryProfileForTypedRelation(rm); !dropped || !strings.Contains(warning, "membership authority disabled") {
		t.Fatalf("r1033 production analyzer normalization did not run: dropped=%t warning=%q", dropped, warning)
	}
	if rm.SourceInventoryProfile == nil || rm.SourceInventoryProfile.IsSourceInventory || len(rm.SourceInventoryProfile.TargetRoles) != 0 ||
		!reflect.DeepEqual(rm.SourceInventoryProfile.RequestedFields, []types.SourceInventoryRequestedField{types.SourceInventoryFieldName, types.SourceInventoryFieldLocation}) {
		t.Fatalf("relation membership must retain the original presentation-only receipt: %+v", rm.SourceInventoryProfile)
	}
	obs := types.SourceInventoryObservation{
		Active: true, Complete: true,
		Scopes:     []string{"include/logx/console_sink.hpp", "include/logx/file_sink.hpp", "include/logx/rotating_sink.hpp"},
		Provenance: []string{"request_traits:typed_source_enumeration_query", "repo_lens:query_roles", "repomap_graph", "tool:repo_map"},
		Sets: []types.SourceInventoryObservationSet{
			{Role: types.AnswerCandidateRoleMethod, Complete: true, Count: 3, Total: 3},
			{Role: types.AnswerCandidateRoleType, Complete: true, Count: 3, Total: 3},
		},
	}
	for _, member := range []struct {
		name, file string
		line       int
	}{
		{"FileSink", "file_sink.hpp", 12}, {"~FileSink", "file_sink.hpp", 15}, {"RotatingSink", "rotating_sink.hpp", 12},
	} {
		obs.Sets[0].Members = append(obs.Sets[0].Members, types.SourceInventoryObservationMember{
			Name: member.name, Role: types.AnswerCandidateRoleMethod, File: "include/logx/" + member.file,
			Line: member.line, Language: "cpp", CoverageState: types.SourceInventoryCoverageObserved,
		})
	}
	for _, ev := range ctx.EvidenceItems {
		obs.Sets[1].Members = append(obs.Sets[1].Members, types.SourceInventoryObservationMember{
			Name: ev.Subject, Role: types.AnswerCandidateRoleType, File: ev.Source, Line: ev.LineStart,
			Language: "cpp", CoverageState: types.SourceInventoryCoverageObserved,
		})
	}
	ctx.Mutable.SetSourceInventoryObservation(obs)
	if sourceInventoryPrincipalAnswerIsModelOwned(ctx) {
		t.Fatal("complete method/type observations must not reactivate normalized-away source membership authority")
	}
	// The real completion had a principal but ungrounded model-emitted set,
	// not the exact support_refs used by the generic carrier matrix above.
	ctx.Mutable.SetInvestigationAggregateFacts([]types.AnswerAggregateFact{{
		Kind: types.AnswerAggregateMemberSet, Label: "Sink 实现类（直接和间接）", Value: "3",
		Role: types.AnswerAggregateRolePrincipalAnswer, Provenance: "model_emitted",
		Members:     []string{"ConsoleSink", "FileSink", "RotatingSink"},
		MemberNotes: []string{"直接继承 Sink，写入 stderr，无缓冲", "直接继承 Sink，以追加模式写文件，支持 flush", "继承 FileSink，文件大小超限时自动轮转"},
	}})
	ctx.Mutable.RetainInvestigationAggregateFacts()
	importEvidence := types.EvidenceItem{
		ID: "ev-3dc2c564b6e31294", Kind: types.EvidenceRelationship, Source: "include/logx/rotating_sink.hpp",
		LineStart: 3, Scope: types.ScopeLine, AnchorKind: types.AnchorImport, AnchorSymbol: "logx/file_sink.hpp",
		Subject: "RotatingSink", Predicate: "声明继承", Object: "FileSink", Snippet: "#include \"logx/file_sink.hpp\"",
		GroundingStatus: types.GroundingGrounded, Origin: types.ClaimOriginCurrentRepo,
	}
	ctx.EvidenceItems = append(ctx.EvidenceItems, importEvidence)
	ctx.Mutable.AppendEvidence([]types.EvidenceItem{importEvidence})
	// Verbatim first emit_answer_document payload from the r1033 tool call.
	raw := json.RawMessage(`{"blocks": [{"claim_uses": [{"claim_form": "definition_fact", "facet_id": "enumeration_item"}], "facet_ids": ["enumeration_item"], "id": "sink-implementations", "items": [{"evidence_ids": ["ev-33a12d0dca17e6c0"], "id": "consolesink-item", "label": "ConsoleSink", "text": "直接继承 Sink，将日志写入 stderr，无缓冲机制。定义于 include/logx/console_sink.hpp:8。"}, {"evidence_ids": ["ev-c11a6e039a6fecbe"], "id": "filesink-item", "label": "FileSink", "text": "直接继承 Sink，以追加模式写文件，支持 flush。定义于 include/logx/file_sink.hpp:10。"}, {"evidence_ids": ["ev-9bc9df9aba1e026", "ev-3dc2c564b6e31294"], "id": "rotatingsink-item", "label": "RotatingSink", "text": "继承 FileSink（间接继承 Sink），在 FileSink 基础上添加文件大小检查和轮转逻辑。定义于 include/logx/rotating_sink.hpp:10。"}], "kind": "ordered_list", "surface_role": "principal", "title": "Sink 实现类列表"}, {"id": "summary-sink", "kind": "summary", "surface_role": "principal", "text": "Sink 是 logx 日志库的抽象接口，定义了日志输出的标准契约。其直接实现类有两个：ConsoleSink 和 FileSink；间接实现类一个：RotatingSink。\n\n继承关系上，RotatingSink 继承自 FileSink，而 FileSink 和 ConsoleSink 都直接继承自 Sink，形成两层继承链：Sink <- FileSink <- RotatingSink。RotatingSink 通过 FileSink 复用缓冲和 flush 机制，仅复写 write() 方法添加大小检查和文件轮转逻辑。"}], "citations": [{"file": "include/logx/console_sink.hpp", "line": 8, "quote": "class ConsoleSink : public Sink {"}, {"file": "include/logx/file_sink.hpp", "line": 10, "quote": "class FileSink : public Sink {"}, {"file": "include/logx/rotating_sink.hpp", "line": 10, "quote": "class RotatingSink : public FileSink {"}, {"file": "include/logx/rotating_sink.hpp", "line": 3, "quote": "#include \"logx/file_sink.hpp\""}]}`)
	var submitted types.AnswerDocumentV2
	if err := json.Unmarshal(raw, &submitted); err != nil {
		t.Fatal(err)
	}
	if hints := preCheckSourceInventoryRowIDBindings(&submitted, ctx); len(hints) != 0 {
		t.Fatalf("r1033 has no active required-source-row contract: %+v", hints)
	}
	res, err := (&EmitAnswerDocument{}).Execute(ctx, raw)
	if err != nil || !res.Success {
		t.Fatalf("r1033 original first emit still hard-rejected with its normalized request and observations: %v / %s", err, res.Summary)
	}
	b1600AssertSelectedEvidence(t, ctx.Mutable.AnswerDocumentV2(), submitted.Blocks)
}

func TestB1600FullEmitPreservesEvidenceOwnerAcrossPrincipalCarriers(t *testing.T) {
	for _, kind := range []types.AnswerBlockKind{types.BlockOrderedList, types.BlockBulletList, types.BlockTable, types.BlockSection} {
		for _, grouped := range []bool{false, true} {
			t.Run(fmt.Sprintf("%s/grouped=%t", kind, grouped), func(t *testing.T) {
				ctx, doc := b1600PrincipalOwnerFixture(t)
				doc.Blocks[1].Kind = kind
				if kind == types.BlockTable {
					doc.Blocks[1].Columns = []string{"Implementation", "Definition and inheritance"}
					for i := range doc.Blocks[1].Items {
						item := &doc.Blocks[1].Items[i]
						item.Cells = []string{item.Label, item.Text}
						item.Label, item.Text = "", ""
					}
				}
				if grouped {
					second := doc.Blocks[1]
					second.ID, second.Title = "indirect", "Indirect implementation"
					second.Items = append([]types.AnswerBlockItem(nil), second.Items[2:]...)
					doc.Blocks[1].Items = doc.Blocks[1].Items[:2]
					doc.Blocks = append(doc.Blocks, second)
				}
				beforeEvidence := b1600Marshal(t, ctx.EvidenceItems)
				raw := b1600Marshal(t, map[string]any{"blocks": doc.Blocks, "citations": doc.Citations})
				res, err := (&EmitAnswerDocument{}).Execute(ctx, raw)
				if err != nil || !res.Success {
					t.Fatalf("valid single-owner first emit must not self-reject: err=%v result=%s", err, res.Summary)
				}
				b1600AssertSelectedEvidence(t, ctx.Mutable.AnswerDocumentV2(), doc.Blocks)
				if string(beforeEvidence) != string(b1600Marshal(t, ctx.EvidenceItems)) {
					t.Fatal("citation display changed accepted source evidence")
				}
				// Exercise the actual partial replacement lane, not just the binder.
				patch := b1600Marshal(t, map[string]any{"replace_blocks": doc.Blocks[1:], "unchanged_block_ids": []string{"summary"}})
				patched, patchErr := (&EmitAnswerDocumentPatch{}).Execute(ctx, patch)
				if patchErr != nil || !patched.Success {
					t.Fatalf("single-owner patch must stay single-owner: %v / %s", patchErr, patched.Summary)
				}
				b1600AssertSelectedEvidence(t, ctx.Mutable.AnswerDocumentV2(), doc.Blocks)
			})
		}
	}
}

func TestB1600PreEmitBinderKeepsExplicitOwnersAndLegacyBinding(t *testing.T) {
	for _, tc := range []struct {
		name                string
		evidence, row, bind bool
		invalidEvidence     bool
		blankEvidence       bool
	}{
		{name: "selected_evidence", evidence: true},
		{name: "selected_source_row", row: true},
		{name: "explicit_dual_owner", evidence: true, row: true},
		{name: "legacy_citation_without_owner", bind: true},
		{name: "whitespace_evidence_is_not_selected_owner", bind: true, blankEvidence: true},
		{name: "invalid_evidence_cannot_gain_row_owner", evidence: true, invalidEvidence: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			ctx, doc := b1600PrincipalOwnerFixture(t)
			doc.Blocks[1].Items = doc.Blocks[1].Items[:1]
			item := &doc.Blocks[1].Items[0]
			item.CitationRef = 0
			item.EvidenceIDs = nil
			if n := normalizePrincipalEnumerationRowIDsByExactIdentityAndCitationWithContext(doc, ctx); n != 1 || item.SourceInventoryRowID == "" {
				t.Fatal("fixture must prove that an unowned legacy citation can acquire its exact row owner")
			}
			rowID := item.SourceInventoryRowID
			if !tc.row {
				item.SourceInventoryRowID = ""
			}
			if tc.evidence {
				item.EvidenceIDs = []string{ctx.EvidenceItems[0].ID}
				if tc.invalidEvidence {
					item.EvidenceIDs = []string{"ev-not-an-accepted-source"}
				}
			}
			if tc.blankEvidence {
				item.EvidenceIDs = []string{"", "  ", "\t"}
			}
			before := b1600Marshal(t, *item)
			fixed := normalizePrincipalEnumerationRowIDsByExactIdentityAndCitationWithContext(doc, ctx)
			if tc.bind {
				if fixed != 1 || item.SourceInventoryRowID != rowID {
					t.Fatalf("legacy binding changed: fixed=%d item=%+v", fixed, item)
				}
			} else if fixed != 0 || string(before) != string(b1600Marshal(t, *item)) {
				t.Fatalf("explicit owner must remain untouched: fixed=%d item=%+v", fixed, item)
			}
			hints := preCheckItemEvidenceIdentity(doc, &types.AnswerSemanticView{ItemEvidenceIdentityAvailable: true}, newPreEmitCheckContext(ctx))
			if tc.evidence && tc.row {
				if len(hints) != 1 || !hints[0].ForceHard || !strings.Contains(hints[0].Reason, "cannot both select") {
					t.Fatalf("model-authored dual owner must retain its precise rejection: %+v", hints)
				}
			} else if tc.invalidEvidence {
				if len(hints) != 1 || !hints[0].ForceHard || !strings.Contains(hints[0].ExpectedShape, "ev-not-an-accepted-source") {
					t.Fatalf("unresolved evidence must not be laundered through a row ID: %+v", hints)
				}
			} else if len(hints) != 0 {
				t.Fatalf("legitimate single-owner carrier gained a retry: %+v", hints)
			}
		})
	}
}
