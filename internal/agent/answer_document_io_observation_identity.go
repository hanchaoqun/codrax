package agent

import (
	"strings"

	"github.com/hanchaoqun/codrax/internal/types"
)

// IO-related evidence facts can republish a blocking-candidate interval under
// the same io_latency predicate used by independent request measurements.
// Choose only the explanatory text from this record's existing measurement
// notes; never infer request identity/closure from its predicate, object,
// interval, prose summary, or another record's matching thread and duration.
// Partial request metadata still belongs to the request explanation, where
// missing/unrecognized proof remains unknown. This is not a validity gate.
func answerDocIOObservationProofMeaning(record types.ObservationRecord, lang string) string {
	for _, key := range []string{
		types.TraceNoteKeyIORequestResidence,
		types.TraceNoteKeyIORequestResidenceCaliber,
		types.TraceNoteKeyIORequestResidenceClock,
	} {
		if traceQueryObservationSupplementNoteValue(record, key) != "" {
			return answerDocIOCompletionProofMeaning(traceQueryObservationSupplementNoteValue(record, types.TraceNoteKeyIOCompletionWokeIssuer), lang)
		}
	}
	if strings.HasPrefix(strings.ToLower(strings.TrimSpace(lang)), "zh") {
		return "本条 IO 相关观测未携带独立的请求计量字段；保留其原始观测区间和来源，不将该区间改称请求生命周期，也不据此判断某个请求的完成唤醒证明是否存在。"
	}
	return "This IO-related observation carries no independent typed request measurement; preserve its observed interval and source rather than relabeling the interval as a request lifetime or inferring whether any request has completion-to-issuer wakeup proof."
}
