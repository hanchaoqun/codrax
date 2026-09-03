package tool

import (
	"strings"

	"github.com/hanchaoqun/codrax/internal/types"
)

// RuntimeTraceReconciliationKind identifies one neutral, typed comparison
// row that a display-only appendix may publish. These rows are not verdicts
// about model prose: a noisy prose selector may choose whether to show one,
// but every emitted word and number comes from this typed carrier.
type RuntimeTraceReconciliationKind string

const (
	RuntimeTraceReconciliationTargetState RuntimeTraceReconciliationKind = "target_state_account"
	RuntimeTraceReconciliationRankOne     RuntimeTraceReconciliationKind = "rank_one"
)

// RuntimeTraceReconciliationRow is projected from the same compiled trace
// projection, renderer model and evidence index as the visible Trace causal
// projection. EvidenceTag is therefore a real E# in that section, never a
// separately numbered or prose-derived reference.
type RuntimeTraceReconciliationRow struct {
	Kind          RuntimeTraceReconciliationKind
	ArtifactLabel string
	Subject       string
	EvidenceTag   string

	WindowStartTs float64
	WindowEndTs   float64
	WindowMS      float64
	RunningMS     float64
	RunnableMS    float64
	SleepMS       float64
	DStateMS      float64
	IOWaitMS      float64
	TotalMS       float64

	Rank         int
	EffectiveMS  float64
	CauseToken   string
	FixDirection string
}

// TraceStateNonIODStateWord is the ONE customer-face word for the exclusive
// non-IO D-state lane — the five-lane partition member, NOT the published
// uninterruptible fold. §40.49 合流复核收编 (G-target-state #1): the body
// wall-clock partition (answer_document_mutation_runtime.go), the
// wait-coverage face (answer_document_mutation_runtime_wait_coverage.go), the
// fact-juxtaposition appendix (orchestrator prose_fact_juxtaposition.go) and
// the reconciliation row (orchestrator prose_typed_reconciliation.go) all
// label this lane with THIS byte sequence; before the fold-in the row said a
// bare "D-state" for 4.039 while the body four-state line said "D-state" for
// the 5.379 fold on the same customer page (two calibers under one word), and
// the two appendix faces spelled "非IO D-state" without the space. The bare
// word "D-state" on a customer face is reserved for the fold, which the
// four-state line prints as "D-state …(其中 IO等待 …)".
func TraceStateNonIODStateWord(zh bool) string {
	if zh {
		return "非 IO D-state"
	}
	return "non-IO D-state"
}

// RuntimeTraceReconciliationRows returns only rows whose E# roster is already
// present in the shipped document. It intentionally stands down when the full
// causal projection was not materialized (for example a narrow status query),
// because an appendix must never point at an invisible evidence index.
func RuntimeTraceReconciliationRows(ctx *types.BusContext) []RuntimeTraceReconciliationRow {
	if ctx == nil || ctx.Mutable == nil {
		return nil
	}
	doc := ctx.Mutable.ShippedAnswerDocumentV2()
	if doc == nil || !answerDocumentHasRuntimeTraceCausalProjectionBlock(doc) {
		return nil
	}
	input := types.ObservationLedgerInputFromBusContext(ctx, types.ObservationExtractLedgerEvidenceLimit)
	ledger := types.CompileObservationLedger(input)
	set := types.CompileTraceCausalProjectionSet(ledger)
	if !runtimeTraceCausalProjectionMaterializationAllowed(ctx, set) {
		return nil
	}
	zh := runtimeTraceCausalProjectionUseChinese(requestedAnswerDocumentLanguage(ctx))
	var out []RuntimeTraceReconciliationRow
	for _, projection := range set.Projections {
		if !projection.Active() {
			continue
		}
		evidence := newRuntimeTraceCausalProjectionEvidenceIndex()
		evidence.flatChain = len(runtimeTraceCausalProjectionCleanPath(projection.WakeupPath)) < 2
		model := buildRuntimeTraceProjTreeModel(projection, evidence, zh)
		runtimeTraceProjStampTargetStateEvidence(projection, &model, evidence, zh)
		label := strings.TrimSpace(projection.ArtifactLabel)
		if label == "" {
			label = strings.TrimSpace(projection.ArtifactPath)
		}

		if account := runtimeTraceProjFourStateAccountProvable(projection, model); account != nil &&
			strings.TrimSpace(model.TargetStateEvidenceTag) != "" {
			out = append(out, RuntimeTraceReconciliationRow{
				Kind:          RuntimeTraceReconciliationTargetState,
				ArtifactLabel: label,
				Subject:       strings.TrimSpace(account.Subject),
				EvidenceTag:   strings.TrimSpace(model.TargetStateEvidenceTag),
				WindowStartTs: account.WindowStartTs,
				WindowEndTs:   account.WindowEndTs,
				WindowMS:      model.WindowMS,
				RunningMS:     account.RunningMS,
				RunnableMS:    account.RunnableMS,
				SleepMS:       account.SleepMS,
				DStateMS:      account.DStateMS,
				IOWaitMS:      account.IOWaitMS,
				TotalMS:       account.TotalMS,
			})
		}

		board := runtimeTraceProjRankBoard(runtimeTraceProjLeadElectionRows(model))
		if len(board) == 0 || board[0].Node.Rank != 1 || board[0].Node.EffectiveImpactMS <= 0 {
			continue
		}
		row := board[0]
		tag := strings.TrimSpace(row.EvidenceTag)
		if tag == "" || !evidence.has(row.Node) {
			continue
		}
		out = append(out, RuntimeTraceReconciliationRow{
			Kind:          RuntimeTraceReconciliationRankOne,
			ArtifactLabel: label,
			Subject:       strings.TrimSpace(row.Node.Subject),
			EvidenceTag:   tag,
			Rank:          row.Node.Rank,
			EffectiveMS:   row.Node.EffectiveImpactMS,
			CauseToken:    strings.TrimSpace(row.Node.TypeToken),
			FixDirection:  strings.TrimSpace(row.Node.FixDirection),
		})
	}
	return out
}
