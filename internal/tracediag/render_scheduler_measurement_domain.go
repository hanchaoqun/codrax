package tracediag

import (
	"fmt"

	"github.com/hanchaoqun/codrax/internal/types"
)

// This optional receipt remains at its original detail position. Its required
// zero coordinates/filter sentinels must not disappear through generic scalar
// omission. Nil is handled by the enclosing walker; this renderer never mints
// a receipt or promotes constructed_partition into capture/causal authority.
func renderSchedulerMeasurementDomainDetail(domain types.TraceSchedulerMeasurementDomain, path string, emit func(string)) {
	emit(fmt.Sprintf("- %s: version=%d status=%s method=%s target_tid=%d window_start_ts=%s window_end_ts=%s query_line_start=%d query_line_end=%d partition_id=%s",
		path, domain.Version, clampToken(domain.Status), clampToken(domain.Method), domain.TargetTID,
		formatSecondsToken(domain.WindowStartTs), formatSecondsToken(domain.WindowEndTs),
		domain.QueryLineStart, domain.QueryLineEnd, clampToken(domain.PartitionID)))
}
