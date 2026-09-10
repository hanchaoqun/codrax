package tracequery

import (
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"hash"
	"math"

	"github.com/hanchaoqun/codrax/internal/types"
)

// buildTimelineMeasurementDomain hashes the complete constructed interval
// sequence, before any downstream display cap. It neither scans source events
// again nor hashes only state totals. A head gap or synthetic tail remains part
// of this partition's identity, not a claim that the artifact was fully seen.
// StateAccountKey remains the separate, window-independent physical-entity
// proof; this receipt must not replace it or become a causal qualification.
func buildTimelineMeasurementDomain(q Query, tl TimelineResult) *types.TraceSchedulerMeasurementDomain {
	finite := func(value float64) bool { return !math.IsNaN(value) && !math.IsInf(value, 0) }
	if q.runCancel.sample() || tl.IntegrityFailure != "" || tl.Thread.PID <= 0 || len(tl.Intervals) == 0 ||
		!finite(tl.Window.StartTs) || !finite(tl.Window.EndTs) || tl.Window.EndTs <= tl.Window.StartTs ||
		(tl.Window.StartTs == 0 && !tl.Window.StartSet) || q.LineStart < 0 || q.LineEnd < 0 ||
		(q.LineStart > 0 && q.LineEnd > 0 && q.LineEnd < q.LineStart) {
		return nil
	}
	w := schedulerPartitionHash{digest: sha256.New()}
	w.text("scheduler_partition:v1:thread_timeline")
	w.thread(tl.Thread)
	w.number(tl.Window.StartTs)
	w.number(tl.Window.EndTs)
	w.integer(q.LineStart)
	w.integer(q.LineEnd)
	w.text(tl.IntegrityFailure)
	w.flag(tl.HeadState != nil)
	if head := tl.HeadState; head != nil {
		if !finite(head.BoundaryTs) || !finite(head.ActualStartTs) {
			return nil
		}
		w.text(head.Status)
		w.number(head.BoundaryTs)
		w.text(string(head.State))
		w.number(head.ActualStartTs)
		w.integer(head.SourceLine)
		w.text(head.Reason)
	}
	w.integer(len(tl.Intervals))
	positive := false
	for _, interval := range tl.Intervals {
		if q.runCancel.tick() || interval.Thread.PID != tl.Thread.PID || interval.State == "" {
			return nil
		}
		for _, value := range [...]float64{interval.StartTs, interval.EndTs, interval.DurationMs,
			interval.ActualStartTs, interval.ActualEndTs, interval.ActualDurationMs} {
			if !finite(value) {
				return nil
			}
		}
		if interval.EndTs < interval.StartTs || interval.DurationMs < 0 || interval.ActualDurationMs < 0 ||
			interval.StartTs < tl.Window.StartTs || interval.EndTs > tl.Window.EndTs {
			return nil
		}
		positive = positive || interval.EndTs > interval.StartTs
		w.thread(interval.Thread)
		w.text(string(interval.State))
		w.number(interval.StartTs)
		w.number(interval.EndTs)
		w.number(interval.DurationMs)
		w.integer(interval.CPU)
		w.flag(interval.CPUKnown)
		w.number(interval.ActualStartTs)
		w.number(interval.ActualEndTs)
		w.number(interval.ActualDurationMs)
		w.integer(interval.StartLine)
		w.integer(interval.EndLine)
		w.integer(interval.WakeupLine)
		w.text(interval.PrevStateRaw)
		w.text(interval.BlockedReasonCaller)
		w.integer(interval.BlockedReasonLine)
		w.integer(int(interval.BlockedReasonIOWait))
		w.flag(interval.BlockedReasonIOWaitKnown)
	}
	if !positive || q.runCancel.sample() {
		return nil
	}
	return &types.TraceSchedulerMeasurementDomain{
		Version: 1, Status: "constructed_partition", Method: "thread_timeline",
		TargetTID: tl.Thread.PID, WindowStartTs: tl.Window.StartTs, WindowEndTs: tl.Window.EndTs,
		QueryLineStart: q.LineStart, QueryLineEnd: q.LineEnd,
		PartitionID: "scheduler_partition:v1:" + hex.EncodeToString(w.digest.Sum(nil)),
	}
}

// Fixed-size streaming encoding avoids a JSON copy of the interval inventory.
// Strings are length-delimited and floats retain their exact IEEE bits; there
// is no rounded-time, delimiter, line-envelope, or equal-total identity guess.
type schedulerPartitionHash struct {
	digest hash.Hash
	buffer [256]byte
}

func (w *schedulerPartitionHash) uint64(value uint64) {
	binary.BigEndian.PutUint64(w.buffer[:8], value)
	_, _ = w.digest.Write(w.buffer[:8])
}

func (w *schedulerPartitionHash) integer(value int)    { w.uint64(uint64(value)) }
func (w *schedulerPartitionHash) number(value float64) { w.uint64(math.Float64bits(value)) }
func (w *schedulerPartitionHash) flag(value bool) {
	if value {
		w.uint64(1)
	} else {
		w.uint64(0)
	}
}

func (w *schedulerPartitionHash) text(value string) {
	w.integer(len(value))
	for len(value) > 0 {
		n := copy(w.buffer[:], value)
		_, _ = w.digest.Write(w.buffer[:n])
		value = value[n:]
	}
}

func (w *schedulerPartitionHash) thread(thread ThreadRef) {
	w.integer(thread.PID)
	w.integer(thread.TGID)
	w.text(thread.Comm)
}
