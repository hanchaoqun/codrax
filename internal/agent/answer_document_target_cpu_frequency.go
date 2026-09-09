package agent

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/hanchaoqun/codrax/internal/types"
)

// The optional full-target carrier supplements the legacy TopN bucket. It
// describes an existing query measurement; it never derives a sample from
// another thread, a nearby CPU, or the text of the model's answer.
type answerDocTargetFrequencyRepresentative struct {
	khz         int64
	donorCPU    int
	donorSource string
}

func answerDocTargetFrequencyFromRecord(r types.ObservationRecord, cpu int) *answerDocTargetFrequencyRepresentative {
	if r.Predicate != "target_cpu_running" || cpu < 0 {
		return nil
	}
	note := func(key string) string { return strings.TrimSpace(traceDecisionRichNoteValue(r.RichNotes, key)) }
	if note(types.TraceNoteKeyTargetCPURunningRepresentativeFrequencyCaliber) != "last_positive_running_segment_start" {
		return nil
	}
	freq, err := strconv.ParseInt(note(types.TraceNoteKeyTargetCPURunningRepresentativeFrequencyKHz), 10, 64)
	if err != nil || freq <= 0 {
		return nil
	}
	result := &answerDocTargetFrequencyRepresentative{khz: freq, donorCPU: -1}
	donor, source := note(types.TraceNoteKeyTargetCPURunningRepresentativeFrequencyDonorCPU), note(types.TraceNoteKeyTargetCPURunningRepresentativeFrequencyDonorSource)
	if donor == "" && source == "" {
		return result
	}
	donorCPU, err := strconv.Atoi(donor)
	if err != nil || donorCPU < 0 || donorCPU == cpu || (source != "explicit_topology" && source != "freq_change_point_derived") {
		return nil
	}
	result.donorCPU, result.donorSource = donorCPU, source
	return result
}

func answerDocTargetCPUFrequencyRowText(row answerDocFrequencySourceJoinRow, zh bool) string {
	r := row.representative
	if r == nil {
		return answerDocTargetCPUFrequencyText(row.frequencies)
	}
	text := fmt.Sprintf("%dkHz — last positive frequency sample at a running-segment start in this target/CPU bucket; not a constant, residency, weighted average, or policy-effect proof", r.khz)
	if zh {
		text = fmt.Sprintf("%dkHz，为该目标线程在此CPU运行桶最近一次有正值的运行段起点频率样本；不是全程恒定频率、驻留时长、加权平均或策略效果证明", r.khz)
	}
	if r.donorCPU >= 0 {
		basis := "explicit topology"
		if r.donorSource == "freq_change_point_derived" {
			basis = "frequency change-point-derived topology"
		}
		if zh {
			basis = "显式拓扑"
			if r.donorSource == "freq_change_point_derived" {
				basis = "频率变化点推导的拓扑"
			}
			text += fmt.Sprintf("；复用同簇 CPU %d（%s）的频率，并非本CPU直接采样", r.donorCPU, basis)
		} else {
			text += fmt.Sprintf("; reused from same-cluster CPU %d (%s), not a direct sample on this CPU", r.donorCPU, basis)
		}
	}
	different := map[int64]bool{}
	for value := range row.frequencies {
		if value > 0 && value != r.khz {
			different[value] = true
		}
	}
	if len(different) > 0 {
		if zh {
			text += "；同结果另有不同运行桶代表值，分别保留，不合并：" + strings.Split(answerDocTargetCPUFrequencyText(different), "(")[0]
		} else {
			text += "; other same-result running-bucket representatives remain separate: " + answerDocTargetCPUFrequencyText(different)
		}
	}
	return text
}
