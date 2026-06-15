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
		&TraceQuery{},
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
		&EmitWriteWorkflowDecision{},
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

func TestGrepToolPromptDocumentsRuntimeArtifactControls(t *testing.T) {
	grep := &GrepTool{}
	description := grep.Description()
	for _, want := range []string{
		"fixed_string=true",
		"line_start/line_end",
		"large log/trace/systrace",
		"do not grep for E|pid|name",
		`trace_query(view="span_window"`,
		"Do NOT use the result to count matches by eye",
	} {
		if !strings.Contains(description, want) {
			t.Fatalf("grep description missing %q:\n%s", want, description)
		}
	}
	parameters := string(grep.Parameters())
	for _, want := range []string{
		`"fixed_string"`,
		`"line_start"`,
		`"line_end"`,
		`"context_lines"`,
	} {
		if !strings.Contains(parameters, want) {
			t.Fatalf("grep parameters missing %q:\n%s", want, parameters)
		}
	}
}

func TestReadFilePromptDocumentsLineOffsetCoordinates(t *testing.T) {
	readFile := &ReadFile{}
	description := readFile.Description()
	for _, want := range []string{
		"line_offset/limit",
		"line_offset=100 starts at source line 101",
		"not a byte or character offset",
	} {
		if !strings.Contains(description, want) {
			t.Fatalf("read_file description missing %q:\n%s", want, description)
		}
	}
	parameters := string(readFile.Parameters())
	for _, want := range []string{
		`"line_offset"`,
		"zero-based LINE offset",
		"not a byte or character offset",
	} {
		if !strings.Contains(parameters, want) {
			t.Fatalf("read_file parameters missing %q:\n%s", want, parameters)
		}
	}
	if strings.Contains(parameters, `"offset"`) {
		t.Fatalf("read_file model-facing schema must not expose legacy offset:\n%s", parameters)
	}
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
