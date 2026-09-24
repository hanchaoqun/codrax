package tracequery

import (
	"math/big"
	"strconv"
)

// Compute the full axis count and only its bounded display prefix. Decimal
// endpoint arithmetic avoids phantom tails without allocating a huge axis.
// Interval consumers remain half-open; endpoint-event consumers may explicitly
// include the captured final instant without changing these boundaries.
func boundedDecimalTimeBucketWindows(startTs, endTs, ms float64, limit uint64) (uint64, []timeInterval, string) {
	if !schedulerConcurrencyFinite(startTs) || !schedulerConcurrencyFinite(endTs) || !schedulerConcurrencyFinite(ms) || endTs <= startTs || ms <= 0 {
		return 0, nil, "continuous_time_window_unavailable"
	}
	start, _ := new(big.Rat).SetString(strconv.FormatFloat(startTs, 'f', -1, 64))
	end, _ := new(big.Rat).SetString(strconv.FormatFloat(endTs, 'f', -1, 64))
	step, _ := new(big.Rat).SetString(strconv.FormatFloat(ms, 'f', -1, 64))
	step.Quo(step, big.NewRat(1000, 1))
	ratio := new(big.Rat).Quo(new(big.Rat).Sub(end, start), step)
	count, remainder := new(big.Int), new(big.Int)
	count.QuoRem(ratio.Num(), ratio.Denom(), remainder)
	if remainder.Sign() > 0 {
		count.Add(count, big.NewInt(1))
	}
	if !count.IsUint64() {
		return 0, nil, "bucket_count_overflow"
	}
	n := count.Uint64()
	shown := n
	if shown > limit {
		shown = limit
	}
	out := make([]timeInterval, 0, int(shown))
	for i := uint64(0); i < shown; i++ {
		a := new(big.Rat).Add(start, new(big.Rat).Mul(step, new(big.Rat).SetInt(new(big.Int).SetUint64(i))))
		b := new(big.Rat).Add(a, step)
		if b.Cmp(end) > 0 {
			b = end
		}
		left, _ := a.Float64()
		right, _ := b.Float64()
		if right <= left {
			return n, nil, "bucket_width_below_trace_timestamp_resolution"
		}
		out = append(out, timeInterval{start: left, end: right})
	}
	return n, out, ""
}
