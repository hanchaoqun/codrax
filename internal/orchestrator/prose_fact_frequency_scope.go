package orchestrator

import (
	"encoding/json"
	"fmt"
	"math"
	"strconv"
	"strings"

	"github.com/hanchaoqun/codrax/internal/types"
)

// The old frequency display showed at most four values per selected CPU.
// Keep that capacity, but each displayed item is now one scoped source row.
const proseFactFrequencyRecordCap = 4

type proseFactFrequencyRecord struct {
	record types.ObservationRecord
	scope  types.TraceRuntimeAccountScope
	khz    float64
	known  bool
}

func proseFactFrequencyScopeFindings(ledger types.ObservationLedger, prose []proseTextUnit) []proseScalarBindingFinding {
	selected := proseFactPresentCPUs(prose) // Original pure-token selector and CPU cap.
	if len(selected) == 0 {
		return nil
	}
	byCPU := map[int][]proseFactFrequencyRecord{}
	seen := map[string]bool{}
	for _, r := range ledger.Records {
		if r.Origin != types.AnswerEvidenceOriginRuntimeArtifact || !types.RuntimeObservationProducerIsDeterministicQuery(r.Producer) {
			continue
		}
		cpu, ok := proseFactNoteInt(r.RichNotes, "cpu")
		if !ok || cpu < 0 {
			continue
		}
		scope := types.TraceRuntimeAccountRecordScope(r)
		if key := proseFactFrequencyRecordKey(r, scope); key != "" {
			if seen[key] {
				continue
			}
			seen[key] = true
		}
		khz, known := proseFactNoteFloat(r.RichNotes, "freq")
		known = known && khz > 0 && !math.IsNaN(khz) && !math.IsInf(khz, 0)
		byCPU[cpu] = append(byCPU[cpu], proseFactFrequencyRecord{record: r, scope: scope, khz: khz, known: known})
	}
	if len(byCPU) == 0 {
		return nil // Preserve the old quiet lane when no CPU facts were supplied.
	}
	var out []proseScalarBindingFinding
	for _, cpu := range selected {
		rows := byCPU[cpu]
		var positive []proseFactFrequencyRecord
		for _, row := range rows {
			if row.known {
				positive = append(positive, row)
			}
		}
		// Like the old positive-value list, do not spend its display budget
		// on missing values when actual representative values are available.
		if len(positive) > 0 {
			rows = positive
		}
		out = append(out, proseScalarBindingFinding{
			entry: proseFactFrequencyRowsText(cpu, rows, false), entryZH: proseFactFrequencyRowsText(cpu, rows, true),
		})
	}
	return out
}

// Full receipt and query identity are prerequisites for deduplication, not
// measurement authority. Unknown records remain independent. Equality uses
// every typed source/bucket/value field (including state notes and coordinates),
// never numerical proximity, a basename, a CPU, or a record ID alone.
func proseFactFrequencyRecordKey(r types.ObservationRecord, scope types.TraceRuntimeAccountScope) string {
	if !scope.Complete() || r.SourceRef.Kind != types.ObservationSourceRuntimeArtifact ||
		strings.TrimSpace(r.SourceRef.Path) == "" || strings.TrimSpace(r.SourceRef.QueryScopeID) == "" ||
		types.RuntimeArtifactCaptureIdentityPath(r.SourceRef) == "" ||
		strings.TrimSpace(r.ObservedAt) == "" || !types.TraceRuntimeAccountRecordsSameResult(r, r) {
		return ""
	}
	encoded, err := json.Marshal(r)
	if err != nil {
		return ""
	}
	return string(encoded)
}

func proseFactFrequencyRowsText(cpu int, rows []proseFactFrequencyRecord, zh bool) string {
	var b strings.Builder
	if zh {
		fmt.Fprintf(&b, "事实对照：CPU%d 的部分相关记录；以下为各线程、各口径记录的附带频率，不证明该 CPU 的原始采样、全程恒定频率、频率驻留或限频效果。", cpu)
	} else {
		fmt.Fprintf(&b, "Evidence reference: selected records for CPU%d; these are record-carried representative frequencies for their own thread and measurement scope, not proof of raw CPU samples, a constant frequency over the entire window, frequency residency, or policy effects.", cpu)
	}
	if len(rows) == 0 {
		if zh {
			b.WriteString(" 当前证据未提供可列出的该 CPU 频率记录；不表示整条 Trace 没有频率数据。")
		} else {
			b.WriteString(" The current evidence provides no frequency record to list for this CPU; this does not mean the entire trace lacks frequency data.")
		}
		return b.String()
	}
	shown := len(rows)
	if shown > proseFactFrequencyRecordCap {
		shown = proseFactFrequencyRecordCap
	}
	for _, row := range rows[:shown] {
		b.WriteString("\n  - ")
		b.WriteString(proseFactFrequencyRowText(cpu, row, zh))
	}
	if shown < len(rows) {
		if zh {
			fmt.Fprintf(&b, "\n  - 本处显示 %d 条，另省略 %d 条记录；未列出不表示没有频率值。", shown, len(rows)-shown)
		} else {
			fmt.Fprintf(&b, "\n  - Showing %d records; %d more omitted here. Omission does not establish absence of frequency values.", shown, len(rows)-shown)
		}
	}
	return b.String()
}

func proseFactFrequencyRowText(cpu int, row proseFactFrequencyRecord, zh bool) string {
	r := row.record
	lang := "en"
	if zh {
		lang = "zh"
	}
	source, subject := strings.TrimSpace(r.SourceRef.Path), strings.TrimSpace(r.Subject)
	if source == "" {
		source = "source not stated"
		if zh {
			source = "来源未明确"
		}
	}
	if subject == "" {
		subject = "thread not stated"
		if zh {
			subject = "线程未明确"
		}
	}
	value := "positive representative frequency not provided"
	if zh {
		value = "未提供正的附带频率值"
	}
	if row.known {
		value = strconv.FormatFloat(row.khz/1000, 'f', -1, 64) + "MHz"
	}
	var b strings.Builder
	if zh {
		fmt.Fprintf(&b, "CPU%d；线程=%q；口径=%s；工件=%q；查询范围=%s；附带频率=%s", cpu, subject, proseFactFrequencyBucketLabel(r, zh), source, types.FormatTraceRuntimeAccountWindow(row.scope.WindowStartTs, row.scope.WindowEndTs, lang), value)
	} else {
		fmt.Fprintf(&b, "CPU%d; thread=%q; measurement=%s; artifact=%q; query window=%s; representative frequency=%s", cpu, subject, proseFactFrequencyBucketLabel(r, zh), source, types.FormatTraceRuntimeAccountWindow(row.scope.WindowStartTs, row.scope.WindowEndTs, lang), value)
	}
	if capture := strings.TrimSpace(r.SourceRef.CaptureIdentityPath); capture != "" && capture != source {
		if zh {
			fmt.Fprintf(&b, "；捕获来源=%q", capture)
		} else {
			fmt.Fprintf(&b, "; capture source=%q", capture)
		}
	}
	var refs []string
	for _, ref := range []string{r.SourceRef.PayloadRef, r.SourceRef.RawRef} {
		if ref = strings.TrimSpace(ref); ref != "" && (len(refs) == 0 || refs[0] != ref) {
			refs = append(refs, ref)
		}
	}
	if len(refs) > 0 {
		if zh {
			fmt.Fprintf(&b, "；查询结果=%q", strings.Join(refs, " | "))
		} else {
			fmt.Fprintf(&b, "; query result=%q", strings.Join(refs, " | "))
		}
	} else if zh {
		b.WriteString("；查询结果来源未明确")
	} else {
		b.WriteString("; query result source not stated")
	}
	if r.ObservedAt != "" {
		if zh {
			fmt.Fprintf(&b, "；查询记录时间=%q", r.ObservedAt)
		} else {
			fmt.Fprintf(&b, "; query recorded at=%q", r.ObservedAt)
		}
	}
	if r.Span.LineStart > 0 {
		if zh {
			fmt.Fprintf(&b, "；记录行=%d", r.Span.LineStart)
		} else {
			fmt.Fprintf(&b, "; record line=%d", r.Span.LineStart)
		}
		if r.Span.LineEnd > r.Span.LineStart {
			fmt.Fprintf(&b, "-%d", r.Span.LineEnd)
		}
	}
	return b.String()
}

func proseFactFrequencyBucketLabel(r types.ObservationRecord, zh bool) string {
	labels := map[string][2]string{
		"running_time":       {"运行状态统计桶", "running-state bucket"},
		"runnable_wait":      {"可运行等待统计桶", "runnable-wait bucket"},
		"sleep_wait":         {"睡眠状态统计桶", "sleep-state bucket"},
		"d_state_or_io_wait": {"D 状态统计桶", "D-state bucket"},
		"io_wait":            {"IO 等待统计桶", "IO-wait bucket"},
		"thread_cpu_load":    {"线程 CPU 负载记录", "thread CPU-load record"},
		"runnable_context":   {"可运行等待上下文", "runnable-wait context"},
	}
	if label, ok := labels[strings.TrimSpace(r.Predicate)]; ok {
		if zh {
			return label[0]
		}
		return label[1]
	}
	if zh {
		return "记录口径未明确"
	}
	return "record measurement scope not stated"
}
