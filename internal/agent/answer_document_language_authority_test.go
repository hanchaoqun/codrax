package agent

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/types"
)

func TestAnswerDocumentLanguageAuthorityInstruction(t *testing.T) {
	for _, tc := range []struct{ configured, contract, request, want, source string }{
		{"zh", "en", "en", "zh", "configured"},
		{"en", "zh", "zh", "en", "configured"},
		{"简体中文", "en", "en", "zh", "configured"},
		{"auto", "en", "zh", "en", "analysis_contract"},
		{"follow", "zh", "en", "zh", "analysis_contract"},
		{"", "", "zh", "zh", "analysis_request"},
		{"off", "zh", "zh", "en", "disabled"},
		{"none", "zh", "zh", "en", "disabled"},
		{"", "", "", "en", "default"},
	} {
		t.Run(tc.configured+"/"+tc.source+"/"+tc.want, func(t *testing.T) {
			ctx := &types.AgentContext{Language: tc.configured, AnalysisIR: &types.AnalysisIR{
				AnswerContract: types.AnswerContract{Language: tc.contract},
				RequestModel:   types.RequestModel{Language: tc.request},
			}}
			before, _ := json.Marshal(ctx)
			e := &answerDocumentEvaluator{}
			instruction := e.BuildInitialInstruction(ctx, nil)
			got, source := resolveAnswerDocLang(ctx)
			if got != tc.want || e.language != tc.want || string(source) != tc.source {
				t.Fatalf("instruction/renderer locale differs: %q/%q/%q", got, e.language, source)
			}
			if strings.Contains(instruction, "## Response Language") == (tc.source == "disabled") {
				t.Fatal("language teaching enabled/disabled changed")
			}
			if tc.source == "configured" && !strings.Contains(instruction, "Project configuration locks the answer language") {
				t.Fatal("project language priority missing")
			}
			after, _ := json.Marshal(ctx)
			if string(before) != string(after) {
				t.Fatal("language instruction mutated context")
			}
		})
	}
}
