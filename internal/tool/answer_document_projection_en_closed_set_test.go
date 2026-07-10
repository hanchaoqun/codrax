package tool

import (
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/preview"
	"github.com/hanchaoqun/codrax/internal/render"
	"github.com/hanchaoqun/codrax/internal/types"
)

// TestRuntimeTraceProjectionEnglishClosedSet keeps system-authored report copy
// on one English vocabulary while proving that similarly-spelled trace payload
// is still passed through verbatim. The raw audit lane is checked separately:
// rank=N is a wire/audit token there, not user-facing rank prose.
func TestRuntimeTraceProjectionEnglishClosedSet(t *testing.T) {
	ranked := types.TraceCausalProjectionNode{
		Role: types.TraceCausalRoleRootCauseContext, EvidenceID: "rank-seat",
		Subject: "ArtifactWorker-200", Predicate: "root_cause_secondary", Object: "running",
		TypeToken: "running", StateKind: "running", ChainRelevance: "on_chain",
		Causality: "on_wakeup_chain", ChainDepth: 1, Rank: 7,
		ImpactMS: 30, CumulativeImpactMS: 30, EffectiveImpactMS: 30, Confidence: 0.8,
	}
	projection := types.TraceCausalProjection{
		WakeupPath:    []string{"ArtifactWorker-200", "TargetRenderer-100"},
		WindowStartTs: 5.0,
		WindowEndTs:   5.1,
		OnChainCauses: []types.TraceCausalProjectionNode{
			ranked,
			{
				Role: types.TraceCausalRoleRootCauseContext, EvidenceID: "fold",
				Predicate: "root_cause_context", ChainRelevance: "on_chain",
				OnChainOverflowFold: true, MergedCount: 1,
				MergedSubjects: []string{"ArtifactWorker-200"},
				MergedMinMS:    3, MergedMaxMS: 3, ImpactMS: 3, CumulativeImpactMS: 3,
				Confidence: 0.6,
			},
		},
	}
	model := buildRuntimeTraceProjTreeModel(projection, newRuntimeTraceCausalProjectionEvidenceIndex(), false)
	tree := runtimeTraceProjTreeFence(model, false)
	for _, raw := range []string{"TargetRenderer-100", "ArtifactWorker-200"} {
		if !strings.Contains(tree, raw) {
			t.Fatalf("raw trace identity %q must survive English copy normalization:\n%s", raw, tree)
		}
	}
	detail := runtimeTraceProjDetailFullText(model, false)
	if !strings.Contains(detail, "see root-cause rank #7") {
		t.Fatalf("lossless fold roster must use the canonical root-cause rank wording:\n%s", detail)
	}

	coverageModel := runtimeTraceProjTreeModel{
		Target:   "TargetRenderer-100",
		WindowMS: 100,
		SelfRows: []runtimeTraceProjTreeRow{
			{
				Kind: runtimeTraceProjTreeRowSelf, HasData: true,
				Node: types.TraceCausalProjectionNode{
					Subject: "TargetRenderer-100", TypeToken: "runnable", StateKind: "runnable", ImpactMS: 20,
				},
			},
			{
				Kind: runtimeTraceProjTreeRowSelf, HasData: true,
				Node: types.TraceCausalProjectionNode{
					Subject: "TargetRenderer-100", TypeToken: "binder_wait", ImpactMS: 40,
				},
			},
		},
		TreeRows: []runtimeTraceProjTreeRow{{
			Kind: runtimeTraceProjTreeRowChain, Depth: 1, HasData: true,
			Node: types.TraceCausalProjectionNode{
				Subject: "ArtifactWorker-200", TypeToken: "running", StateKind: "running",
				ImpactMS: 20, CumulativeImpactMS: 20,
			},
		}},
	}
	windowLine := runtimeTraceProjWindowLine(projection, coverageModel, false)
	for _, want := range []string{"Analysis window", "focused-thread state row(s)"} {
		if !strings.Contains(windowLine, want) {
			t.Fatalf("English window copy missing canonical phrase %q:\n%s", want, windowLine)
		}
	}

	comparison := runtimeTraceProjCompareOverviewBlocks([]types.TraceCausalProjection{
		enClosedSetCompareProjection("TargetRenderer.trace", 5.0, 5.1),
		enClosedSetCompareProjection("ArtifactWorker.trace", 6.0, 6.3),
	}, types.ObservationLedger{}, "en", false, runtimeTraceProjUserFocus{})
	if len(comparison) == 0 {
		t.Fatal("two trace projections must render an English comparison overview")
	}
	overview := comparison[0]
	columns := strings.Join(overview.Columns, " | ")
	for _, want := range []string{
		"Trace file",
		"Primary root cause (root-cause rank #1)",
		"Focused-thread symptom duration",
		"Analysis window",
	} {
		if !strings.Contains(columns, want) {
			t.Fatalf("comparison header missing canonical phrase %q: %s", want, columns)
		}
	}
	comparisonText := enClosedSetBlockText(overview)
	if !strings.Contains(comparisonText, "Analysis-window lengths differ") {
		t.Fatalf("unequal normalization windows must use analysis-window wording:\n%s", comparisonText)
	}

	systemCopy := strings.Join([]string{tree, detail, windowLine, columns, comparisonText}, "\n")
	for _, retired := range []string{
		"Requested window",
		"Target wait",
		"target wait",
		"target state row",
		"target sleep",
		"Of the target",
		"Target symptom",
		"Projection window lengths differ",
		"see rank",
		"(rank=",
	} {
		if strings.Contains(systemCopy, retired) {
			t.Fatalf("retired system-authored phrase %q leaked into English report:\n%s", retired, systemCopy)
		}
	}
	for _, col := range overview.Columns {
		if col == "Artifact" {
			t.Fatalf("retired comparison header Artifact leaked into English report: %v", overview.Columns)
		}
	}

	// The user panel consumes the same Markdown document that feeds the HTML
	// report. Pin the canonical comparison vocabulary on both rendered faces,
	// rather than only on the intermediate AnswerDocument table.
	doc := &types.AnswerDocumentV2{DocumentModel: "v2", Blocks: comparison}
	md := render.RenderAnswerDocument(doc, "en")
	html, err := preview.RenderStandaloneMarkdownHTML("trace", []byte(md))
	if err != nil {
		t.Fatalf("render English trace HTML: %v", err)
	}
	for _, want := range []string{"Trace file", "Focused-thread symptom duration", "Analysis window"} {
		if !strings.Contains(md, want) || !strings.Contains(html, want) {
			t.Fatalf("Markdown/HTML parity lost canonical phrase %q\nmarkdown:\n%s\nhtml:\n%s", want, md, html)
		}
	}

	audit := runtimeTraceCausalProjectionAuditDetail(ranked, false, false)
	if !strings.Contains(audit, "rank=7") {
		t.Fatalf("raw audit rank token must remain lossless, got %q", audit)
	}
}

func enClosedSetCompareProjection(label string, start, end float64) types.TraceCausalProjection {
	return types.TraceCausalProjection{
		ArtifactLabel: label,
		WindowStartTs: start,
		WindowEndTs:   end,
		BackgroundCauses: []types.TraceCausalProjectionNode{{
			Role: types.TraceCausalRoleRootCauseContext, EvidenceID: label + "-pressure",
			Subject: "cpu-pressure", Predicate: "root_cause_context", Object: "supply_pressure",
			TypeToken: "supply_pressure", SubjectKind: types.TraceCausalSubjectKindAggregateMetric,
			ChainRelevance: "background", Causality: "background",
			ImpactMS: 100, CumulativeImpactMS: 100, EffectiveImpactMS: 100,
		}},
	}
}

func enClosedSetBlockText(block types.AnswerBlock) string {
	parts := []string{block.Title, block.Text, strings.Join(block.Columns, " | ")}
	for _, item := range block.Items {
		parts = append(parts, strings.Join(item.Cells, " | "))
	}
	return strings.Join(parts, "\n")
}
