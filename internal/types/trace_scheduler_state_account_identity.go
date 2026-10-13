package types

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"math"
	"sort"
	"strings"
)

// TraceSchedulerStateAccountIdentity preserves the producer's v2 inventory
// identity byte-for-byte. It is not a source receipt or causal authorization.
// Consumers may compare a proposed exact inventory with a producer-minted key
// only after independently qualifying the native source and event instance.
func TraceSchedulerStateAccountIdentity(pid int, state string, windowStart, windowEnd float64, intervals [][2]float64, expectedMS float64) string {
	state = strings.TrimSpace(strings.ToLower(state))
	if pid <= 0 || state == "" || windowEnd <= windowStart ||
		len(intervals) == 0 || expectedMS <= 0 || math.IsNaN(expectedMS) || math.IsInf(expectedMS, 0) {
		return ""
	}
	ordered := append([][2]float64(nil), intervals...)
	sort.SliceStable(ordered, func(i, j int) bool {
		if ordered[i][0] == ordered[j][0] {
			return ordered[i][1] < ordered[j][1]
		}
		return ordered[i][0] < ordered[j][0]
	})
	total, lastEnd := 0.0, 0.0
	var body strings.Builder
	// The view window is not physical inventory identity. Every member must
	// lie within it, but a wider query may publish the same exact account.
	fmt.Fprintf(&body, "v2|pid=%d|state=%s", pid, state)
	for i, interval := range ordered {
		start, end := interval[0], interval[1]
		if end <= start || math.IsNaN(start) || math.IsNaN(end) || math.IsInf(start, 0) || math.IsInf(end, 0) ||
			start < windowStart || end > windowEnd || (i > 0 && start < lastEnd) {
			return ""
		}
		total += (end - start) * 1000
		fmt.Fprintf(&body, "|%.9f..%.9f", start, end)
		lastEnd = end
	}
	if math.Abs(total-expectedMS) >= TraceCausalProjectionSameValueTieMS {
		return ""
	}
	sum := sha256.Sum256([]byte(body.String()))
	return "state_account:v2:" + hex.EncodeToString(sum[:16])
}
