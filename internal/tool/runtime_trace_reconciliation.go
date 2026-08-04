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
