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

// 105 蜂鸟 container cold start #5 (ftrace-20261019_17.35.50), the analysed
// main thread 28252 in [4819.5, 4820.0) (HMC377 round 9, R9-2, register
// R8C-1): the worker TID 28559 is named download_sdk for the window (sched
// names: download_sdk ×121, the creator-inherited LMagex-Work-2 ×3), and
// download_sdk is also its name when its markers are written. All 44 of its
// VerifyClass markers in the window nevertheless carry the row-header comm
// me.ele.hb.frame: the row header prints the comm cached when the trace was
// dumped, a name the thread takes later (the first scheduler payload naming
// 28559 me.ele.hb.frame is at 4824.894820, outside the window; raw oracle
// fix9/r2/oracle_28559.py; review oracle fix8/review-cv1/oracle/
// rename_repro_28559.out; correction round 10, fix9/review-cv1/oracle/
// {marker_time_rows.txt,raw_28559.out,next_comm_28559.out}). This is the
// discriminating real repro of F1: the class-verification span record and
// the span-member observation fold from the complete span inventory after
// publication, and name TID 28559 me.ele.hb.frame-28559 (the row-header
// dump-time comm) when that inventory is not published under the display
// view (publishTraceSpanInventory a no-op: fix9/r2/mutant_f1_noop.log FAILS
// here).
const (
	hmc377HoneybirdContainer5Digest = "0e535301ac34e2ce47dfa28cb72e543225a0acccc74da04f06a973c555226732"
	hmc377HoneybirdContainer5Size   = 114960266
)

func TestHMC377WholeOriginalHoneybirdContainerSpanRecordsNameTheWorkerByItsDisplayName(t *testing.T) {
	path := hmc376Original(t, "CODRAX_HMC377_HONEYBIRD_CONTAINER5", hmc377HoneybirdContainer5Digest, hmc377HoneybirdContainer5Size)
	result, native := hmc376QueryTarget(t, path, "root_cause_rank", map[string]any{"pid": 28252, "target_scope": "thread"}, 4819.5, 4820.0)
	if !result.Success {
		t.Fatalf("query failed: %s", result.Summary)
	}
	identity, ok := hmc377Identity(native, 28559)
	if !ok || identity.DisplayComm != "download_sdk" {
		t.Fatalf("TID 28559 identity %+v", identity)
	}
	// The row-header comm (the comm cached at dump time, not the name the
	// thread had when it wrote its markers) is among the thread's names: a
	// record named by it is a different name than the display.
	rowHeaderName := false
	for _, name := range identity.Names {
		if name.Comm == "me.ele.hb.frame" {
			rowHeaderName = true
		}
	}
	if !rowHeaderName {
		t.Fatalf("TID 28559 must carry the row-header comm me.ele.hb.frame among its names: %+v", identity.Names)
	}
	var spanRecord, memberRecord bool
	for _, record := range result.Observations {
		if !strings.HasSuffix(record.Subject, "-28559") {
			continue
		}
		if record.Subject != "download_sdk-28559" {
			t.Fatalf("%s %s names TID 28559 %q, want its display download_sdk-28559", record.ID, record.ClaimKey, record.Subject)
		}
		switch {
		case record.ClaimKey == "trace_semantic_span:class_verification":
			spanRecord = true
		case record.ClaimKey == "trace_span_members_observation":
			memberRecord = true
			if record.Value != "44" {
				t.Fatalf("the span-member observation of TID 28559 counts %s members, the raw capture 44 VerifyClass markers", record.Value)
			}
		}
	}
	if !spanRecord || !memberRecord {
		t.Fatalf("the class-verification span record (%v) and the span-member observation (%v) of TID 28559 must be published", spanRecord, memberRecord)
	}
}

// Douyin reboot capture 5 [140.911304, 141.835458) TID 11105: TID 11317 is
// published as #tp-io-29 (most of the window).
//
// EVOLUTION RECORD (HMC377 CV-1, 2026-11-20): that thread's class-verification
// record came from one 0.005 ms "VerifyClassLoaderContextMatch" marker at
// 141.681957 s, which the substring rule counted as class verification. It is
// the class-loader context check of opening compiled code, so TID 11317 has
// no class-verification record any more.
//
// EVOLUTION RECORD (HMC377 round 9, R9-2, register R8C-1, 2026-11-21): this
// test was TestHMC377WholeOriginalDouyinSpanRecordsNameTheWorkerByItsDisplayName,
// the whole-original pin of F1 (span records folded from the span inventory
// name a renamed thread by its display name). After CV-1 it could no longer
// tell display naming from marker-time naming: the renamed threads 11105 and
// 11115 mark every span under their display comm, so with F1 reverted
// (publishTraceSpanInventory a no-op) it still passed (review round 8,
// fix8/review-cv1/mutant/mutant_f1_run.log). Its F1 assertions are retired;
// F1 is pinned by the discriminating Honeybird container #5 repro above and
// by the synthetic TestHMC377SpanRecordsNameARenamedThreadByItsDisplayName.
// What remains here discriminates: the identity of 11317, every record
// naming it by #tp-io-29 (a rank row named it #tp-fixed-1176 when display
// naming was mutated, fix8/review-cv1/mutant/mutant_run.log), and the
// class-loader context check not being a class-verification record (CV-1).
//
// EVOLUTION RECORD (HMC377 round 10, register R9C-1 (4), 2026-11-21): the
// R9-2 re-scope also dropped the guard that the span records of the renamed
// threads are published, so a mutant that stops publishing the analysed
// thread's span-member records (review round 9, fix9/review-cv1/mutant/
// M5b_member.go with M5_trace_query.go) passed it (run_M5b.log). The guard is
// restored: the analysed thread 11105 publishes both a semantic span record
// and its span-member record, and 11115 a span or span-member record, each
// named by the thread's display name.
func TestHMC377WholeOriginalDouyinClassLoaderContextCheckIsNoClassVerificationRecord(t *testing.T) {
	path := hmc376Original(t, "CODRAX_HMC377_DOUYIN_REBOOT", hmc377DouyinDigest, hmc377DouyinSize)
	result, native := hmc376QueryTarget(t, path, "root_cause_rank", map[string]any{"pid": 11105, "target_scope": "thread"}, 140.911304, 141.835458)
	identity, ok := hmc377Identity(native, 11317)
	if !ok || identity.DisplayComm != "#tp-io-29" {
		t.Fatalf("TID 11317 identity %+v", identity)
	}
	named := false
	renamed := map[string]string{"-11105": ".ugc.aweme.lite-11105", "-11115": "RenderThread-11115"}
	spanRecords := map[string]bool{}
	for _, record := range result.Observations {
		kind := ""
		switch {
		case strings.HasPrefix(record.ClaimKey, "trace_semantic_span:"):
			kind = "span"
		case record.ClaimKey == "trace_span_members_observation":
			kind = "member"
		}
		for suffix, want := range renamed {
			if kind != "" && strings.HasSuffix(record.Subject, suffix) {
				if record.Subject != want {
					t.Fatalf("%s %s names a renamed thread %q, want %q", record.ID, record.ClaimKey, record.Subject, want)
				}
				spanRecords[want+"/"+kind] = true
			}
		}
		if !strings.HasSuffix(record.Subject, "-11317") {
			continue
		}
		named = true
		if record.Subject != "#tp-io-29-11317" {
			t.Fatalf("%s %s names TID 11317 %q, want #tp-io-29-11317", record.ID, record.ClaimKey, record.Subject)
		}
		if record.ClaimKey == "trace_semantic_span:class_verification" {
			t.Fatalf("TID 11317 runs no class verification in this window; %s is the class-loader context check", record.ID)
		}
	}
	if !named {
		t.Fatalf("TID 11317 must be published on some record of this window")
	}
	if !spanRecords[".ugc.aweme.lite-11105/span"] || !spanRecords[".ugc.aweme.lite-11105/member"] ||
		!(spanRecords["RenderThread-11115/span"] || spanRecords["RenderThread-11115/member"]) {
		t.Fatalf("the span and span-member records of the analysed thread 11105 and a span record of 11115 must be published: %v", spanRecords)
	}
}
