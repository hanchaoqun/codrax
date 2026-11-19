package tool

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// HMC377 F1 — the seam between the published result and the records the
// trace_query tool folds from it afterwards. The semantic span records and the
// span-member observation fold from the complete span inventory behind the
// bounded listing; a renamed thread must be named on them by the display name
// the result publishes, like on every other face.

// hmc377InventoryCaptureLines: app.main 56 is woken by Signal Catcher 76; the
// pool worker 90 records thirty class-verification spans while it still carries
// the name "worker" and is "task-b" for most of [1.0, 1.1]. Nine spans exceed
// the eight-span listing.
func hmc377InventoryCaptureLines() []string {
	lines := []string{
		" zygote-1 (1) [000] .... 1.000000: sched_wakeup_new: comm=zygote pid=56 prio=120 target_cpu=000",
		" <idle>-0 (0) [000] .... 1.001000: sched_switch: prev_comm=swapper/0 prev_pid=0 prev_prio=120 prev_state=R ==> next_comm=zygote next_pid=56 next_prio=120",
		" app.main-56 (56) [000] .... 1.002000: sched_switch: prev_comm=app.main prev_pid=56 prev_prio=120 prev_state=R+ ==> next_comm=swapper/0 next_pid=0 next_prio=120",
		" <idle>-0 (0) [000] .... 1.003000: sched_switch: prev_comm=swapper/0 prev_pid=0 prev_prio=120 prev_state=R ==> next_comm=app.main next_pid=56 next_prio=120",
		" app.main-56 (56) [000] .... 1.004000: sched_wakeup_new: comm=app.main pid=76 prio=120 target_cpu=001",
		" app.main-56 (56) [000] .... 1.010000: sched_switch: prev_comm=app.main prev_pid=56 prev_prio=120 prev_state=S ==> next_comm=swapper/0 next_pid=0 next_prio=120",
		" Signal Catcher-76 (56) [001] .... 1.020000: sched_switch: prev_comm=swapper/1 prev_pid=0 prev_prio=120 prev_state=R ==> next_comm=app.main next_pid=76 next_prio=120",
		" Signal Catcher-76 (56) [001] .... 1.030000: sched_switch: prev_comm=Signal Catcher prev_pid=76 prev_prio=120 prev_state=R+ ==> next_comm=worker next_pid=90 next_prio=120",
	}
	for i := 0; i < 30; i++ {
		begin := 1.0310 + float64(i)*0.00028
		lines = append(lines,
			fmt.Sprintf(" worker-90 (56) [001] .... %.6f: tracing_mark_write: B|56|VerifyClass com.example.K%d", begin, i),
			fmt.Sprintf(" worker-90 (56) [001] .... %.6f: tracing_mark_write: E|56", begin+0.0002))
	}
	return append(lines,
		" task-b-90 (56) [001] .... 1.040000: sched_switch: prev_comm=task-a prev_pid=90 prev_prio=120 prev_state=S ==> next_comm=Signal Catcher next_pid=76 next_prio=120",
		" Signal Catcher-76 (56) [001] .... 1.050000: sched_wakeup: comm=app.main pid=56 prio=120 target_cpu=000",
		" <idle>-0 (0) [000] .... 1.051000: sched_switch: prev_comm=swapper/0 prev_pid=0 prev_prio=120 prev_state=R ==> next_comm=app.main next_pid=56 next_prio=120",
		" Signal Catcher-76 (56) [001] .... 1.060000: sched_switch: prev_comm=Signal Catcher prev_pid=76 prev_prio=120 prev_state=S ==> next_comm=task-b next_pid=90 next_prio=120",
		" task-b-90 (56) [001] .... 1.090000: sched_switch: prev_comm=task-b prev_pid=90 prev_prio=120 prev_state=S ==> next_comm=swapper/1 next_pid=0 next_prio=120",
		" app.main-56 (56) [000] .... 1.095000: sched_switch: prev_comm=app.main prev_pid=56 prev_prio=120 prev_state=S ==> next_comm=swapper/0 next_pid=0 next_prio=120",
		" <idle>-0 (0) [000] .... 1.100000: cpu_idle: state=0 cpu_id=0",
	)
}

// hmc377AssertNoFormerLabel fails when a published text names TID 90 by a
// name it carried before its display name.
func hmc377AssertNoFormerLabel(t *testing.T, view string, texts []string) {
	t.Helper()
	for _, text := range texts {
		for _, former := range []string{"worker-90", "task-a-90"} {
			if strings.Contains(text, former) {
				t.Fatalf("%s: a record names TID 90 %q, not its display task-b-90:\n%s", view, former, text)
			}
		}
	}
}

func TestHMC377SpanRecordsNameARenamedThreadByItsDisplayName(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "inventory.ftrace")
	if err := os.WriteFile(path, []byte("# tracer: nop\n"+strings.Join(hmc377InventoryCaptureLines(), "\n")+"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	for _, view := range []string{"window_stats", "root_cause_rank"} {
		result, native := hmc376QueryTarget(t, path, view, map[string]any{"pid": 56, "target_scope": "thread"}, 1.0, 1.1)
		if !result.Success {
			t.Fatalf("%s: %s", view, result.Summary)
		}
		identity, ok := hmc377Identity(native, 90)
		if !ok || identity.DisplayComm != "task-b" {
			t.Fatalf("%s: TID 90 identity %+v", view, identity)
		}
		var spanRecord, memberRecord bool
		var texts []string
		for _, record := range result.Observations {
			texts = append(texts, record.Subject, record.Object, record.Summary)
			if !strings.HasSuffix(record.Subject, "-90") {
				continue
			}
			switch {
			case strings.HasPrefix(record.ClaimKey, "trace_semantic_span:"):
				spanRecord = true
			case record.ClaimKey == "trace_span_members_observation":
				memberRecord = true
				texts = append(texts, strings.Join(record.RichNotes, "\n"))
			default:
				continue
			}
			if record.Subject != "task-b-90" {
				t.Fatalf("%s: %s names TID 90 %q, want the published display task-b-90", view, record.ClaimKey, record.Subject)
			}
		}
		if !spanRecord || !memberRecord {
			t.Fatalf("%s: the fixture must publish the semantic span record (%v) and the span-member observation (%v) of TID 90", view, spanRecord, memberRecord)
		}
		hmc377AssertNoFormerLabel(t, view, texts)
	}
}

// Douyin reboot capture 5 [140.911304, 141.835458) TID 11105 (reviewer
// repro): TID 11317 is published as #tp-io-29 (most of the window); its
// class-verification record and its span-member observation named it by the
// marker-time comm #tp-fixed-1176.
func TestHMC377WholeOriginalDouyinSpanRecordsNameTheWorkerByItsDisplayName(t *testing.T) {
	path := hmc376Original(t, "CODRAX_HMC377_DOUYIN_REBOOT", hmc377DouyinDigest, hmc377DouyinSize)
	result, native := hmc376QueryTarget(t, path, "root_cause_rank", map[string]any{"pid": 11105, "target_scope": "thread"}, 140.911304, 141.835458)
	identity, ok := hmc377Identity(native, 11317)
	if !ok || identity.DisplayComm != "#tp-io-29" {
		t.Fatalf("TID 11317 identity %+v", identity)
	}
	seen := map[string]bool{}
	for _, record := range result.Observations {
		if !strings.HasSuffix(record.Subject, "-11317") {
			continue
		}
		if record.Subject != "#tp-io-29-11317" {
			t.Fatalf("%s %s names TID 11317 %q, want #tp-io-29-11317", record.ID, record.ClaimKey, record.Subject)
		}
		seen[record.ClaimKey] = true
	}
	if !seen["trace_semantic_span:class_verification"] || !seen["trace_span_members_observation"] {
		t.Fatalf("the repro records must be published for TID 11317: %v", seen)
	}
}
