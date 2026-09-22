package tool

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/types"
)

func TestAnswerDocumentLanguageAuthority(t *testing.T) {
	cases := []struct{ name, configured, contract, request, want string }{
		{"project_zh", "zh", "en", "en", "zh"},
		{"project_en", "en", "zh", "zh", "en"},
		{"project_alias", " 简体中文 ", "en", "en", "zh"},
		{"contract_alias", "auto", " EN-US ", "zh", "en"},
		{"auto", "auto", "zh", "en", "zh"},
		{"follow", "follow", "en", "zh", "en"},
		{"request", "", "", "zh", "zh"},
		{"unknown_contract", "auto", "fr", "zh", "zh"},
		{"unknown_config", "fr", "zh", "en", "zh"},
		{"off", "off", "zh", "zh", "en"},
		{"none", " NONE ", "zh", "zh", "en"},
		{"empty", "", "", "", "en"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			ctx := &types.BusContext{Language: tc.configured, AnalysisIR: &types.AnalysisIR{
				AnswerContract: types.AnswerContract{Language: tc.contract},
				RequestModel:   types.RequestModel{Language: tc.request, Scenario: types.ScenarioConfigTrace},
			}}
			ctx.EvidenceItems = []types.EvidenceItem{{ID: "neg", Kind: types.EvidenceAbsent,
				Scope: types.ScopeNegative, GroundingStatus: types.GroundingGrounded,
				NegativeQuery: &types.NegativeQuery{File: "settings.go", Pattern: "business_key"},
				NegativeScope: types.NegativeScopeFile}}
			before, _ := json.Marshal(ctx)
			if got := requestedAnswerDocumentLanguage(ctx); got != tc.want {
				t.Errorf("locale=%q, want %q", got, tc.want)
			}
			if hypothesisVerdictPrefersChinese(ctx) != (tc.want == "zh") {
				t.Error("external evidence annotation disagrees with renderer locale")
			}
			title, _ := materializedScopeCaveatCopy(ctx)
			wantScope, wantNegative := "Scope note", "Search scope for no-match results"
			if tc.want == "zh" {
				wantScope, wantNegative = "范围说明", "未命中结果的搜索范围"
			}
			if title != wantScope {
				t.Errorf("scope note=%q, want %q", title, wantScope)
			}
			wantNote := "Notes"
			if tc.want == "zh" {
				wantNote = "说明"
			}
			if principalEnumerationPrefersZH(ctx) != (tc.want == "zh") || enumerationDisplayDefaultNoteColumn(ctx) != wantNote {
				t.Errorf("enumeration system labels disagree with resolved locale %q", tc.want)
			}
			const model = "Model-owned 原始业务判断"
			doc := &types.AnswerDocumentV2{Blocks: []types.AnswerBlock{{ID: "model", Kind: types.BlockSummary, Text: model}}}
			if !materializeCurrentSourceNegativeScopeAuthority(doc, ctx) || len(doc.Blocks) != 2 {
				t.Fatalf("missing source scope: %+v", doc)
			}
			if doc.Blocks[1].Title != wantNegative || !strings.Contains(doc.Blocks[1].Text, "business_key") || !strings.Contains(doc.Blocks[1].Text, "settings.go") {
				t.Errorf("negative scope changed locale or identifiers: %+v", doc.Blocks[1])
			}
			if doc.Blocks[0].Text != model {
				t.Fatal("model prose rewritten")
			}
			after, _ := json.Marshal(ctx)
			if string(before) != string(after) {
				t.Fatal("language resolution rewrote input authority")
			}
		})
	}
	if got := requestedAnswerDocumentLanguage(nil); got != "en" {
		t.Errorf("nil fallback=%q, want renderer default en", got)
	}
	if title, _ := materializedScopeCaveatCopy(nil); title != "Scope note" {
		t.Errorf("nil scope note=%q", title)
	}
}
