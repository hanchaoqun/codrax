package types

import (
	"fmt"
	"math"
	"path/filepath"
	"strings"
)

// TraceRunnablePriorityOverlap is one representative native scheduler
// intersection, not the row's total runnable time or aggregate attribution.
// Event lines are local to SourcePath and may bracket the intersection.
type TraceRunnablePriorityOverlap struct {
	SourcePath       string                `json:"source_path"`
	Ready            TraceThreadCoordinate `json:"ready_thread"`
	Running          TraceThreadCoordinate `json:"running_thread"`
	CPU              int                   `json:"cpu"`
	StartTs          float64               `json:"start_ts"`
	EndTs            float64               `json:"end_ts"`
	ReadyStartLine   int                   `json:"ready_start_line"`
	ReadyEndLine     int                   `json:"ready_end_line"`
	RunningStartLine int                   `json:"running_start_line"`
	RunningEndLine   int                   `json:"running_end_line"`
	RecordedWakeup   bool                  `json:"ready_started_at_recorded_wakeup,omitempty"`
}

// ForRankMember validates display binding, not rank, root-cause authority,
// physical ownership of the board target, or a graph edge.
func (w *TraceRunnablePriorityOverlap) ForRankMember(path string, member TraceThreadCoordinate, start, end float64) *TraceRunnablePriorityOverlap {
	if !w.valid() || path == "" || filepath.Clean(w.SourcePath) != filepath.Clean(path) ||
		!member.valid() || member.TID != w.Ready.TID ||
		(member.TGID > 0 && w.Ready.TGID > 0 && member.TGID != w.Ready.TGID) ||
		!TraceCausalProjectionWindowPresent(start, end) || math.IsInf(end, 0) || w.StartTs < start || w.EndTs > end {
		return nil
	}
	return w.Clone()
}

func (w *TraceRunnablePriorityOverlap) Clone() *TraceRunnablePriorityOverlap {
	if w == nil {
		return nil
	}
	copy := *w
	return &copy
}

func (w *TraceRunnablePriorityOverlap) valid() bool {
	return w != nil && w.SourcePath != "" && w.Ready.valid() && w.Running.valid() &&
		w.Ready.TID != w.Running.TID && w.CPU >= 0 && w.CPU <= 4095 &&
		w.StartTs >= 0 && !math.IsInf(w.StartTs, 0) &&
		!math.IsNaN(w.EndTs) && !math.IsInf(w.EndTs, 0) && w.EndTs > w.StartTs &&
		w.ReadyStartLine > 0 && w.ReadyEndLine > w.ReadyStartLine &&
		w.RunningStartLine > 0 && w.RunningEndLine > w.RunningStartLine
}

// ReaderDescription is shared by decision inputs and the deterministic
// report. It never calls the competing thread a blocker, holder or waker.
func (w *TraceRunnablePriorityOverlap) ReaderDescription(zh bool) string {
	if !w.valid() {
		return ""
	}
	name := func(t TraceThreadCoordinate) string {
		if comm := strings.Join(strings.Fields(t.Comm), " "); comm != "" {
			return fmt.Sprintf("%s-%d", comm, t.TID)
		}
		return fmt.Sprintf("TID %d", t.TID)
	}
	ready, running := name(w.Ready), name(w.Running)
	if zh {
		when := ""
		if w.RecordedWakeup {
			when = fmt.Sprintf("（记录的唤醒 L%d 之后）", w.ReadyStartLine)
		}
		return fmt.Sprintf("一段实测同核重叠：就绪线程 %s（L%d–L%d）与运行的低优先级线程 %s（L%d–L%d）在 CPU%d 的 [%.6f, %.6f) 秒相交%s",
			ready, w.ReadyStartLine, w.ReadyEndLine, running, w.RunningStartLine, w.RunningEndLine, w.CPU, w.StartTs, w.EndTs, when)
	}
	when := ""
	if w.RecordedWakeup {
		when = fmt.Sprintf(" after recorded wakeup (line %d)", w.ReadyStartLine)
	}
	return fmt.Sprintf("one measured same-CPU ready/run overlap: ready thread %s (lines %d–%d), lower-priority running competitor %s (lines %d–%d), CPU%d [%.6f, %.6f) s%s",
		ready, w.ReadyStartLine, w.ReadyEndLine, running, w.RunningStartLine, w.RunningEndLine, w.CPU, w.StartTs, w.EndTs, when)
}
