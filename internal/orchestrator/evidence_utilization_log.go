package orchestrator

import (
	"fmt"

	"github.com/hanchaoqun/codrax/internal/agent"
	"github.com/hanchaoqun/codrax/internal/logging"
	"github.com/hanchaoqun/codrax/internal/types"
)

// logEvidenceUtilization emits one INFO line per successful finalize
// describing how many evidence items the answer actually cited vs
// how many the explorer collected.
//
// Why a separate signal: the existing pipeline logs cover step
// budgets / retry counts / tool-call totals, but no single line tells
// an operator "the explorer collected 70 grounded evidence items and
// the answer cited 5". That ratio is the only direct signal of
// over-investigation. The 2026-04-29 production trace at
// /home/chatpp/pytest motivated the project-orientation budget
// carve-out; the next round of tuning (per-level evidence target,
// adaptive coverage gates, etc.) needs this number to be measurable
// before it can be defended.
//
// Output shape (single line, INFO level):
//
//	[finalize/utilization] evidence collected=N cited=M ratio=X.XX
//	    intent=I scenario=S complexity=C orientation=true|false
//	    sub_topics=K primary_entities=P
//
// All fields are pulled from already-populated BusContext / IR /
// AnswerDocument data — no new schema, no new yaml. The
// orientation flag uses the same types.IsProjectOrientationQuestion
// predicate the budget carve-out and the multi-path symbol-anchored
// gate (applyMultiPathAnchorChecks) consult, so cross-stage operators
// can correlate the three signals.
//
// nil-safe: any missing piece (no AnalysisIR, no AnswerDocument, no
// final StageOutput) skips the line silently. The log is observability,
// not a quality gate.
func logEvidenceUtilization(o *Orchestrator, finalize *agent.StageOutput) {
	if o == nil || o.busCtx == nil {
		return
	}
	collected := len(o.busCtx.EvidenceItems)
	cited := finalizerCitationCount(o, finalize)
	ratio := 0.0
	if collected > 0 {
		ratio = float64(cited) / float64(collected)
	}
	rm := types.RequestModel{}
	if o.busCtx.AnalysisIR != nil {
		rm = o.busCtx.AnalysisIR.RequestModel
	}
	logging.Info("[finalize/utilization] evidence collected=%d cited=%d ratio=%.2f intent=%s scenario=%s complexity=%s orientation=%t sub_topics=%d primary_entities=%d",
		collected, cited, ratio,
		rm.Intent, rm.Scenario, rm.Complexity,
		types.IsProjectOrientationQuestion(rm),
		len(rm.SubTopics),
		len(rm.AnalyzerHints.PrimaryEntities),
	)
}

// finalizerCitationCount returns the number of distinct (file, line)
// citations the finalizer emitted. Reads from MutableState's
// AnswerDocument — the canonical surface populated by the
// emit_answer_document tool. Repeated cites of the same anchor
// count once: matches the user-facing semantic "how many sources
// does the answer rest on", not "how many times each source is
// referenced".
//
// Returns 0 when no AnswerDocument has been installed (e.g.
// finalize failed before emit_answer_document). The caller logs the
// zero so the absence is itself a signal.
func finalizerCitationCount(o *Orchestrator, _ *agent.StageOutput) int {
	if o == nil || o.busCtx == nil || o.busCtx.Mutable == nil {
		return 0
	}
	doc := o.busCtx.Mutable.AnswerDocumentV2()
	if doc == nil {
		return 0
	}
	seen := make(map[string]struct{}, len(doc.Citations))
	for _, c := range doc.Citations {
		if c.File == "" || c.Line <= 0 {
			continue
		}
		key := fmt.Sprintf("%s:%d", c.File, c.Line)
		seen[key] = struct{}{}
	}
	return len(seen)
}
