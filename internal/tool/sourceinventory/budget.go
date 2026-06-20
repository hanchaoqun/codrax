package sourceinventory

import (
	"context"
	"strconv"
	"strings"
	"time"

	repotypes "github.com/hanchaoqun/codrax/internal/tool/repomap/types"
	"github.com/hanchaoqun/codrax/internal/types"
)

const (
	FileThreshold         = 500
	DefaultTopN           = 60
	MaterializeMultiplier = 4
	ScanMultiplier        = 16
	MinPerRole            = 64
	MaxPerRole            = 512
	MinScanPerRole        = 512
	MaxScanPerRole        = 4096
	MaxElapsed            = 3 * time.Second
	QueryScanMultiplier   = 4
	QueryScanMaxPerRole   = 16384
)

type BudgetOptions struct {
	Context           context.Context
	TopN              int
	RepoFileCount     int
	GraphFileCount    int
	ForceAdvisoryOnly bool
	Cursor            string
	Offset            int
	MaxPerRole        int
	MaxScanPerRole    int
	MaxQueryScanRole  int
	Deadline          time.Time
}

// Budget is the bounded-execution authority for source-inventory scans. It
// owns materialization limits, scan limits, query-scan widening, wall-clock
// deadline, cooperative cancellation, and cursor/page metadata. It deliberately
// contains no candidate-shape logic; callers decide what a candidate is, while
// this kernel decides when scanning/materialization must stop.
type Budget struct {
	maxPerRole          int
	maxScanPerRole      int
	maxQueryScanPerRole int
	deadline            time.Time
	ctx                 context.Context
	cursorOffset        int
	pageLimit           int
}

func NewBudget(options BudgetOptions) Budget {
	budget := Budget{
		ctx:          context.Background(),
		cursorOffset: CursorOffset(options.Offset, options.Cursor),
		pageLimit:    PageLimit(options.TopN, options.RepoFileCount),
	}
	if options.Context != nil {
		budget.ctx = options.Context
	}
	if !options.ForceAdvisoryOnly || options.GraphFileCount < FileThreshold {
		return budget
	}
	topN := options.TopN
	if topN <= 0 {
		topN = PageLimit(0, options.GraphFileCount)
	}
	limit := topN * MaterializeMultiplier
	if limit < MinPerRole {
		limit = MinPerRole
	}
	if limit > MaxPerRole {
		limit = MaxPerRole
	}
	scanLimit := limit * ScanMultiplier
	if scanLimit < MinScanPerRole {
		scanLimit = MinScanPerRole
	}
	if scanLimit > MaxScanPerRole {
		scanLimit = MaxScanPerRole
	}
	queryScanLimit := scanLimit * QueryScanMultiplier
	if queryScanLimit > QueryScanMaxPerRole {
		queryScanLimit = QueryScanMaxPerRole
	}
	budget.maxPerRole = firstPositive(options.MaxPerRole, limit)
	budget.maxScanPerRole = firstPositive(options.MaxScanPerRole, scanLimit)
	budget.maxQueryScanPerRole = firstPositive(options.MaxQueryScanRole, queryScanLimit)
	budget.deadline = options.Deadline
	if budget.deadline.IsZero() {
		budget.deadline = time.Now().Add(MaxElapsed)
	}
	return budget
}

func PageLimit(topN, repoFileCount int) int {
	if topN > 0 {
		return topN
	}
	if repoFileCount > 0 {
		if tiered := repotypes.DefaultTopN("source_inventory", repotypes.RepoSizeTier(repoFileCount)); tiered > 0 {
			return tiered
		}
	}
	return DefaultTopN
}

func CursorOffset(offset int, cursor string) int {
	if offset > 0 {
		return offset
	}
	cursor = strings.TrimSpace(cursor)
	if cursor == "" {
		return 0
	}
	parsed, err := strconv.Atoi(cursor)
	if err != nil || parsed < 0 {
		return 0
	}
	return parsed
}

func Page(offset, limit, total int, budgetTruncated bool) (types.SourceInventoryObservationPage, bool) {
	if total <= 0 {
		return types.SourceInventoryObservationPage{}, false
	}
	if offset > total {
		offset = total
	}
	if offset < 0 {
		offset = 0
	}
	emitted := 0
	if limit > 0 && offset < total {
		emitted = limit
		if remaining := total - offset; emitted > remaining {
			emitted = remaining
		}
	}
	nextCursor := ""
	if offset+emitted < total {
		nextCursor = strconv.Itoa(offset + emitted)
	}
	return types.SourceInventoryObservationPage{
		Offset:     offset,
		Limit:      limit,
		Total:      total,
		Emitted:    emitted,
		NextCursor: nextCursor,
		Complete:   nextCursor == "" && !budgetTruncated,
	}, true
}

func (budget Budget) Page(total int, budgetTruncated bool) (types.SourceInventoryObservationPage, bool) {
	return Page(budget.cursorOffset, budget.pageLimit, total, budgetTruncated)
}

func (budget Budget) MaterializationEnabled() bool {
	return budget.maxPerRole > 0
}

func (budget Budget) MaterializationExceeded(count int) bool {
	return budget.MaterializationEnabled() && count >= budget.maxPerRole
}

func (budget Budget) ScanEnabled() bool {
	return budget.maxScanPerRole > 0
}

func (budget Budget) Interrupted() bool {
	if budget.ctx != nil {
		select {
		case <-budget.ctx.Done():
			return true
		default:
		}
	}
	return !budget.deadline.IsZero() && time.Now().After(budget.deadline)
}

func (budget Budget) ScanExceeded(scanned int) bool {
	if budget.Interrupted() {
		return true
	}
	return budget.ScanEnabled() && scanned >= budget.maxScanPerRole
}

func (budget Budget) QueryScanExceeded(scanned int) bool {
	if budget.Interrupted() {
		return true
	}
	if !budget.ScanEnabled() {
		return false
	}
	limit := budget.maxQueryScanPerRole
	if limit <= 0 {
		limit = budget.maxScanPerRole
	}
	return scanned >= limit
}

func (budget Budget) MaxPerRole() int {
	return budget.maxPerRole
}

func (budget Budget) MaxScanPerRole() int {
	return budget.maxScanPerRole
}

func (budget Budget) MaxQueryScanPerRole() int {
	return budget.maxQueryScanPerRole
}

func (budget Budget) CursorOffset() int {
	return budget.cursorOffset
}

func (budget Budget) PageLimit() int {
	return budget.pageLimit
}

func firstPositive(values ...int) int {
	for _, value := range values {
		if value > 0 {
			return value
		}
	}
	return 0
}
