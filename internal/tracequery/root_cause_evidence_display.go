package tracequery

import (
	"fmt"
	"strings"

	"github.com/hanchaoqun/codrax/internal/types"
)

// RootCauseRankDisplayChannel describes this board's published relation, not
// its ordinal-allocation channel or authority over the user's whole request.
// A targetless board legitimately retains ranked observations, but its engine
// fail-open ordinal must not become a claim that those observations caused a
// target response. A name-only unresolved target likewise supplies no identity.
// This is presentation only: no rank, measurement or causal carrier is changed.
func RootCauseRankDisplayChannel(board RootCauseRankResult, item RootCauseRankItem) string {
	if board.Target.PID <= 0 {
		return rootCauseOrdinalChannelBackground
	}
	if rootCauseItemIsOnChain(item) {
		return rootCauseOrdinalChannelChain
	}
	relevance := strings.TrimSpace(item.ChainRelevance)
	if relevance == "" {
		relevance = chainRelevanceFromCausality(item.Causality)
	}
	if relevance == "adjacent" {
		return rootCauseOrdinalChannelAdjacent
	}
	return rootCauseOrdinalChannelBackground
}

func rootCauseEvidenceDisplayPosition(board RootCauseRankResult, item RootCauseRankItem) string {
	channel := RootCauseRankDisplayChannel(board, item)
	position := fmt.Sprintf("%s background observation (original rank=%d; support only)", item.Tier, item.Rank)
	switch channel {
	case rootCauseOrdinalChannelChain:
		position = fmt.Sprintf("%s cause #%d (board target=%s)", item.Tier, item.Rank, threadLabel(board.Target))
	case rootCauseOrdinalChannelAdjacent:
		position = fmt.Sprintf("%s adjacent observation #%d (support only; board target=%s)", item.Tier, item.Rank, threadLabel(board.Target))
	}
	if item.Rank <= 0 {
		position = fmt.Sprintf("%s %s observation (no rank seat)", item.Tier, channel)
	}
	if item.AbsorbedByRankFamily {
		position = fmt.Sprintf("absorbed %s observation (no rank seat; absorbed_into=%s)", channel, item.AbsorbedIntoFamily)
	}
	return position
}

// RootCauseRankIOValueDescription reuses the same ruler vocabulary as native
// tables and diagrams. Completion closure is not used to guess a missing ruler:
// an off-chain completed request can still carry request residence, not wait.
func RootCauseRankIOValueDescription(item RootCauseRankItem) string {
	if !types.TraceUsesIOValueCaliber(item.Type, item.IOValueCaliber) {
		return ""
	}
	caliber := types.NormalizeTraceIOValueCaliber(item.IOValueCaliber)
	if caliber == "" {
		caliber = "unspecified"
	}
	return fmt.Sprintf("io_value_caliber=%s value_measurement=%q", caliber, types.TraceIOValueCaliberLabel(item.IOValueCaliber, false))
}
