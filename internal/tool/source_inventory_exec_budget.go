package tool

import (
	"context"

	repotypes "github.com/hanchaoqun/codrax/internal/tool/repomap/types"
	"github.com/hanchaoqun/codrax/internal/tool/sourceinventory"
	"github.com/hanchaoqun/codrax/internal/types"
)

const (
	sourceInventoryExecBudgetFileThreshold         = sourceinventory.FileThreshold
	sourceInventoryExecBudgetDefaultTopN           = sourceinventory.DefaultTopN
	sourceInventoryExecBudgetMaterializeMultiplier = sourceinventory.MaterializeMultiplier
	sourceInventoryExecBudgetScanMultiplier        = sourceinventory.ScanMultiplier
	sourceInventoryExecBudgetMinPerRole            = sourceinventory.MinPerRole
	sourceInventoryExecBudgetMaxPerRole            = sourceinventory.MaxPerRole
	sourceInventoryExecBudgetMinScanPerRole        = sourceinventory.MinScanPerRole
	sourceInventoryExecBudgetMaxScanPerRole        = sourceinventory.MaxScanPerRole
	sourceInventoryExecBudgetMaxElapsed            = sourceinventory.MaxElapsed
	sourceInventoryExecBudgetQueryScanMultiplier   = sourceinventory.QueryScanMultiplier
	sourceInventoryExecBudgetQueryScanMaxPerRole   = sourceinventory.QueryScanMaxPerRole
)

// sourceInventoryExecBudget is the tool-local adapter over the shared
// sourceinventory.Budget kernel. Candidate construction stays in package tool
// because it uses package-private candidate structs; execution limits and page
// semantics live in the kernel package.
type sourceInventoryExecBudget struct {
	kernel sourceinventory.Budget
}

func sourceInventoryExecBudgetForLens(ctx *types.BusContext, query types.SourceInventoryLensQuery, forceAdvisoryOnly bool, graph *repotypes.Graph) sourceInventoryExecBudget {
	options := sourceinventory.BudgetOptions{
		Context:           context.Background(),
		TopN:              query.TopN,
		RepoFileCount:     query.RepoFileCount,
		ForceAdvisoryOnly: forceAdvisoryOnly,
		Cursor:            query.Cursor,
		Offset:            query.Offset,
	}
	if ctx != nil && ctx.Context() != nil {
		options.Context = ctx.Context()
	}
	if graph != nil {
		options.GraphFileCount = len(graph.FileIndex)
	}
	return sourceInventoryExecBudget{kernel: sourceinventory.NewBudget(options)}
}

func (budget sourceInventoryExecBudget) materializationEnabled() bool {
	return budget.kernel.MaterializationEnabled()
}

func (budget sourceInventoryExecBudget) materializationExceeded(count int) bool {
	return budget.kernel.MaterializationExceeded(count)
}

func (budget sourceInventoryExecBudget) scanEnabled() bool {
	return budget.kernel.ScanEnabled()
}

func (budget sourceInventoryExecBudget) interrupted() bool {
	return budget.kernel.Interrupted()
}

func (budget sourceInventoryExecBudget) scanExceeded(scanned int) bool {
	return budget.kernel.ScanExceeded(scanned)
}

func (budget sourceInventoryExecBudget) queryScanExceeded(scanned int) bool {
	return budget.kernel.QueryScanExceeded(scanned)
}

func (budget sourceInventoryExecBudget) page(total int, budgetTruncated bool) (types.SourceInventoryObservationPage, bool) {
	return budget.kernel.Page(total, budgetTruncated)
}

func (budget sourceInventoryExecBudget) appendCandidate(set *sourceInventoryCandidateSet, candidate sourceInventoryCandidate, filter sourceInventoryQueryFilter) bool {
	if set == nil {
		return false
	}
	if candidate.key == "" || (filter.Active() && !sourceInventoryCandidateMatchesQuery(candidate, filter)) {
		return false
	}
	if budget.materializationExceeded(len(set.candidates)) {
		sourceInventoryMarkCandidateBudgetTruncated(set)
		return true
	}
	set.candidates = append(set.candidates, candidate)
	return false
}
