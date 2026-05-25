package tool

import (
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/skill"
)

func TestToolSchemasDoNotExposeInternalMechanismTerms(t *testing.T) {
	tools := []Tool{
		&ExecCommand{},
		&GrepTool{},
		&ReadFile{},
		&ListFiles{},
		&GitDiff{},
		&GitShow{},
		&GitLog{},
		&GitHistorySearch{},
		&EmitAnalysis{},
		&EmitChangePlan{},
		&EmitPlanSkeleton{},
		&EmitPlanChange{},
		&ApplyPatch{},
		&RunTests{},
		&EmitTestResults{},
		&RecallMemory{},
		&ListMemory{},
		NewProposeSubAgents(),
		&EmitEvidence{},
		&EmitInvestigationComplete{},
		&EmitAnswerSymbol{},
		&EmitHypothesisVerdict{},
		&EmitAnswerDocument{},
		&EmitAnswerDocumentPatch{},
		&EmitLogTriage{},
		&EmitLogSegmentation{},
		&EmitPerfTrace{},
		&EmitPerfSegmentation{},
		&EmitWriteAnalysis{},
	}

	var hits []string
	for _, candidate := range tools {
		if candidate == nil {
			continue
		}
		surfaces := map[string]string{
			candidate.Name() + ".description": candidate.Description(),
			candidate.Name() + ".parameters":  string(candidate.Parameters()),
		}
		for label, body := range surfaces {
			for _, term := range skill.InternalTermsBlocklist {
				if term == "" {
					continue
				}
				if idx := strings.Index(body, term); idx >= 0 {
					hits = append(hits, label+": token="+term+" preview="+toolPromptPreview(body, idx, len(term)))
				}
			}
		}
	}
	if len(hits) == 0 {
		return
	}
	for _, hit := range hits {
		t.Errorf("  %s", hit)
	}
	t.Fatalf("tool schema hygiene found %d internal mechanism term(s)", len(hits))
}

func toolPromptPreview(s string, idx, n int) string {
	start := idx - 48
	if start < 0 {
		start = 0
	}
	end := idx + n + 48
	if end > len(s) {
		end = len(s)
	}
	return strings.ReplaceAll(s[start:end], "\n", " ")
}
