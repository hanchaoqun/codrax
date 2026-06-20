package tool

import (
	"context"
	"time"

	repotypes "github.com/hanchaoqun/codrax/internal/tool/repomap/types"
	"github.com/hanchaoqun/codrax/internal/types"
)

const (
	sourceInventoryExecBudgetFileThreshold         = 500
	sourceInventoryExecBudgetDefaultTopN           = 60
	sourceInventoryExecBudgetMaterializeMultiplier = 4
	sourceInventoryExecBudgetScanMultiplier        = 16
	sourceInventoryExecBudgetMinPerRole            = 64
	sourceInventoryExecBudgetMaxPerRole            = 512
	sourceInventoryExecBudgetMinScanPerRole        = 512
	sourceInventoryExecBudgetMaxScanPerRole        = 4096
	sourceInventoryExecBudgetMaxElapsed            = 3 * time.Second
	sourceInventoryExecBudgetQueryScanMultiplier   = 4
	sourceInventoryExecBudgetQueryScanMaxPerRole   = 16384
)

// sourceInventoryExecBudget is the single bounded-execution authority for
// source-inventory candidate construction. It owns materialization limits,
// scan limits, query-scan widening, wall-clock deadline, cooperative
// cancellation, and the current page cursor. Callers should ask this object
// before scanning or appending; they should not open-code per-role guards.
type sourceInventoryExecBudget struct {
	maxPerRole          int
	maxScanPerRole      int
	maxQueryScanPerRole int
	deadline            time.Time
	ctx                 context.Context
	cursorOffset        int
	pageLimit           int
}

func sourceInventoryExecBudgetForLens(ctx *types.BusContext, query types.SourceInventoryLensQuery, forceAdvisoryOnly bool, graph *repotypes.Graph) sourceInventoryExecBudget {
	budget := sourceInventoryExecBudget{
		ctx:          context.Background(),
		cursorOffset: sourceInventoryLensQueryOffset(query),
		pageLimit:    sourceInventoryObservationPageLimit(query),
	}
	if ctx != nil && ctx.Context() != nil {
		budget.ctx = ctx.Context()
	}
	if !forceAdvisoryOnly || graph == nil || len(graph.FileIndex) < sourceInventoryExecBudgetFileThreshold {
		return budget
	}
	topN := query.TopN
	if topN <= 0 {
		topN = sourceInventoryExecBudgetDefaultTopN
		if tiered := repotypes.DefaultTopN("source_inventory", repotypes.RepoSizeTier(len(graph.FileIndex))); tiered > 0 {
			topN = tiered
		}
	}
	limit := topN * sourceInventoryExecBudgetMaterializeMultiplier
	if limit < sourceInventoryExecBudgetMinPerRole {
		limit = sourceInventoryExecBudgetMinPerRole
	}
	if limit > sourceInventoryExecBudgetMaxPerRole {
		limit = sourceInventoryExecBudgetMaxPerRole
	}
	scanLimit := limit * sourceInventoryExecBudgetScanMultiplier
	if scanLimit < sourceInventoryExecBudgetMinScanPerRole {
		scanLimit = sourceInventoryExecBudgetMinScanPerRole
	}
	if scanLimit > sourceInventoryExecBudgetMaxScanPerRole {
		scanLimit = sourceInventoryExecBudgetMaxScanPerRole
	}
	queryScanLimit := scanLimit * sourceInventoryExecBudgetQueryScanMultiplier
	if queryScanLimit > sourceInventoryExecBudgetQueryScanMaxPerRole {
		queryScanLimit = sourceInventoryExecBudgetQueryScanMaxPerRole
	}
	budget.maxPerRole = limit
	budget.maxScanPerRole = scanLimit
	budget.maxQueryScanPerRole = queryScanLimit
	budget.deadline = time.Now().Add(sourceInventoryExecBudgetMaxElapsed)
	return budget
}

func (budget sourceInventoryExecBudget) materializationEnabled() bool {
	return budget.maxPerRole > 0
}

func (budget sourceInventoryExecBudget) materializationExceeded(count int) bool {
	return budget.materializationEnabled() && count >= budget.maxPerRole
}

func (budget sourceInventoryExecBudget) scanEnabled() bool {
	return budget.maxScanPerRole > 0
}

func (budget sourceInventoryExecBudget) interrupted() bool {
	if budget.ctx != nil {
		select {
		case <-budget.ctx.Done():
			return true
		default:
		}
	}
	return !budget.deadline.IsZero() && time.Now().After(budget.deadline)
}

func (budget sourceInventoryExecBudget) scanExceeded(scanned int) bool {
	if budget.interrupted() {
		return true
	}
	return budget.scanEnabled() && scanned >= budget.maxScanPerRole
}

func (budget sourceInventoryExecBudget) queryScanExceeded(scanned int) bool {
	if budget.interrupted() {
		return true
	}
	if !budget.scanEnabled() {
		return false
	}
	limit := budget.maxQueryScanPerRole
	if limit <= 0 {
		limit = budget.maxScanPerRole
	}
	return scanned >= limit
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
