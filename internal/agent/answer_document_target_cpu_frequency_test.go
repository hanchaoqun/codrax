package agent

import (
	"encoding/json"
	"fmt"
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/types"
)

func b1633TargetFrequencyResult(t *testing.T, donor bool) types.ToolResult {
	t.Helper()
	r := b1631JoinResult(t, "/captures/full.systrace", "full-result.json", "query-a", "1.000", 2100000)
	r.Observations[0].RichNotes = append(r.Observations[0].RichNotes,
		"target_cpu_running_representative_frequency_khz=640000",
		"target_cpu_running_representative_frequency_caliber=last_positive_running_segment_start")
	if donor {
		r.Observations[0].RichNotes = append(r.Observations[0].RichNotes,
			"target_cpu_running_representative_frequency_donor_cpu=7",
			"target_cpu_running_representative_frequency_donor_source=explicit_topology")
	}
	return r
}

func TestB1633ActualFrequencyQueriesReachBothReadersWithoutPolicy(t *testing.T) {
	dir := t.TempDir()
	var results []types.ToolResult
	for _, freq := range []int{640000, 1280000} {
		path := b1631WriteFrequencyCapture(t, dir, fmt.Sprintf("capture-%d/same.systrace", freq), fmt.Sprintf(`idle-0 (0) [004] .... 10.000000: cpu_frequency: state=%d cpu_id=4
idle-0 (0) [004] .... 10.001000: sched_switch: prev_comm=idle prev_pid=0 prev_prio=120 prev_state=R ==> next_comm=target next_pid=41 next_prio=120
target-41 (41) [004] .... 10.002000: sched_switch: prev_comm=target prev_pid=41 prev_prio=120 prev_state=S ==> next_comm=idle next_pid=0 next_prio=120
`, freq))
		r := b1631RunFrequencyQuery(t, dir, path, 0, 0)
		if r.TraceEvidenceAuthority == nil || len(r.TraceEvidenceAuthority.FrequencyLimitWitnesses) != 0 {
			t.Fatal("real fixture unexpectedly requires or produced a policy witness")
		}
		found := false
		for _, record := range r.Observations {
			if record.Predicate == "target_cpu_running" {
				f := answerDocTargetFrequencyFromRecord(record, 4)
				if f == nil || f.khz != int64(freq) || record.Value != "1.000" || record.SourceRef.QueryScopeID == "" {
					t.Fatalf("actual producer premise failed: %+v", record)
				}
				found = true
			}
		}
		if !found {
			t.Fatal("real query omitted target CPU account")
		}
		results = append(results, r)
	}
	for _, reverse := range []bool{false, true} {
		ordered := append([]types.ToolResult(nil), results...)
		if reverse {
			ordered[0], ordered[1] = ordered[1], ordered[0]
		}
		before, _ := json.Marshal(ordered)
		ctx, _ := b1631FrequencyFinalContext(t, dir, ordered)
		for _, lang := range []string{"zh", "en"} {
			ctx.Language, ctx.AnalysisIR.RequestModel.Language = lang, lang
			prompt := (&answerDocumentEvaluator{}).BuildInitialInstruction(ctx, nil)
			rows := b1631JoinRows(prompt)
			if len(rows) != 2 {
				t.Fatalf("two captures must retain independent target measurements: %v", rows)
			}
			marker := "- Per-CPU target running and frequency comparison"
			if lang == "zh" {
				marker = "- 目标线程的逐 CPU 运行与频率对照"
			}
			_, reader, _ := strings.Cut(prompt, marker)
			for _, freq := range []int{640000, 1280000} {
				for _, section := range []string{strings.Join(rows, "\n"), reader} {
					found := false
					for _, line := range strings.Split(section, "\n") {
						if strings.Contains(line, fmt.Sprintf("capture-%d", freq)) && strings.Contains(line, fmt.Sprintf("%dkHz", freq)) && strings.Contains(line, "1.000ms") {
							found = true
						}
					}
					if !found {
						t.Fatalf("actual reader lost capture-local frequency %d: %s", freq, section)
					}
				}
			}
		}
		after, _ := json.Marshal(ordered)
		if string(before) != string(after) {
			t.Fatal("finalizer modified actual producer facts")
		}
	}
}

func TestB1633NoPolicyDoesNotOpenLegacyOrInvalidFrequencyJoin(t *testing.T) {
	for _, missing := range []string{"target_cpu_running_representative_frequency_khz=640000", "target_cpu_running_representative_frequency_caliber=last_positive_running_segment_start"} {
		r := b1633TargetFrequencyResult(t, false)
		r.TraceEvidenceAuthority.FrequencyLimitWitnesses = nil
		for i, note := range r.Observations[0].RichNotes {
			if note == missing {
				r.Observations[0].RichNotes[i] = ""
			}
		}
		_, prompt := b1631JoinPrompt(t, "en", r)
		if strings.Contains(prompt, "Runtime target/CPU policy comparison matrix") || strings.Contains(prompt, "- Per-CPU target running and frequency comparison") {
			t.Fatal("legacy/invalid carrier opened new no-policy join")
		}
	}
}

func TestB1633TargetFrequencyReachesBothFinalizerReaders(t *testing.T) {
	for _, lang := range []string{"zh", "en"} {
		for _, donor := range []bool{false, true} {
			r := b1633TargetFrequencyResult(t, donor)
			before, _ := json.Marshal(r)
			_, prompt := b1631JoinPrompt(t, lang, r)
			rows := b1631JoinRows(prompt)
			if len(rows) != 1 || !strings.Contains(rows[0], "640000kHz") {
				t.Errorf("full target roster frequency missing from actual matrix: %v", rows)
			}
			readerMarker := "- Per-CPU target running and frequency comparison"
			if lang == "zh" {
				readerMarker = "- 目标线程的逐 CPU 运行与频率对照"
			}
			_, reader, ok := strings.Cut(prompt, readerMarker)
			if !ok || !strings.Contains(reader, "640000kHz") {
				t.Errorf("full target roster frequency missing from actual reader: %s", reader)
			}
			if donor {
				boundary := "same-cluster CPU 7"
				if lang == "zh" {
					boundary = "同簇 CPU 7"
				}
				if !strings.Contains(reader, boundary) {
					t.Errorf("donor frequency must not be described as own sampling: %s", reader)
				}
			}
			after, _ := json.Marshal(r)
			if string(before) != string(after) {
				t.Fatal("reader mutated original target evidence")
			}
		}
	}
}

func TestB1633TargetFrequencyInvalidCaliberIsNotInvented(t *testing.T) {
	for _, replace := range []string{
		"target_cpu_running_representative_frequency_caliber=future",
		"target_cpu_running_representative_frequency_caliber=",
	} {
		r := b1633TargetFrequencyResult(t, false)
		r.Observations[0].RichNotes[4] = replace
		_, prompt := b1631JoinPrompt(t, "en", r)
		for _, row := range b1631JoinRows(prompt) {
			if strings.Contains(row, "640000kHz") {
				t.Fatalf("unknown caliber minted frequency authority: %s", row)
			}
		}
	}
}

func TestB1633TargetFrequencyDoesNotRequirePolicyEvent(t *testing.T) {
	for _, lang := range []string{"zh", "en"} {
		r := b1633TargetFrequencyResult(t, false)
		r.TraceEvidenceAuthority.FrequencyLimitWitnesses = nil
		_, prompt := b1631JoinPrompt(t, lang, r)
		rows := b1631JoinRows(prompt)
		if len(rows) != 1 || !strings.Contains(rows[0], "640000kHz") {
			t.Errorf("measured target frequency must survive absent policy events: %v", rows)
		}
		marker := "- Per-CPU target running and frequency comparison"
		if lang == "zh" {
			marker = "- 目标线程的逐 CPU 运行与频率对照"
		}
		_, reader, ok := strings.Cut(prompt, marker)
		if !ok || !strings.Contains(reader, "640000kHz") {
			t.Errorf("reader lost positive frequency without policy events: %s", reader)
		}
	}
}
