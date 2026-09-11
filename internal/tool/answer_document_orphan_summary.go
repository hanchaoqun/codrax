package tool

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/hanchaoqun/codrax/internal/types"
)

// This is a diagnostic preview of the newly minted orphan-only lease, not
// another capability compiler. Keep identifiers exact and omit overlong rows
// whole: a shortened identifier must not look like a copyable model selector.
func stagedOrphanDispositionSummary(lease *types.AnswerDiagramRelationRepairLease) string {
	if lease == nil || !lease.OrphanDispositionOnly || len(lease.OptionalOrphanCleanups) == 0 {
		return ""
	}
	const maxRows = 8
	const maxRowBytes = 512
	rows := make([]string, 0, maxRows)
	for _, candidate := range lease.OptionalOrphanCleanups {
		if len(rows) == maxRows {
			break
		}
		actions, err := json.Marshal(candidate.AllowedActions)
		if err != nil {
			continue
		}
		row := fmt.Sprintf("{block=%q, participant=%q, allowed_actions=%s}", candidate.BlockID, candidate.ParticipantID, actions)
		if types.TruncateBytesEllipsis(row, maxRowBytes) != row {
			continue
		}
		rows = append(rows, row)
	}
	text := "; current orphan choices=[" + strings.Join(rows, "; ") + "]"
	if omitted := len(lease.OptionalOrphanCleanups) - len(rows); omitted > 0 {
		text += fmt.Sprintf("; %d additional current orphan choice(s) omitted from this bounded preview", omitted)
	}
	return text + "; choose actions using the complete current schema and optional_orphan_cleanups"
}
