package orchestrator

// prose_cr4_arm5_test.go — CR-4 臂5 跨口径缝合披露 pins (§29.53 遗留 → CR-4,
// docs/design/real_trace_campaign_20260705.md, 2026-07-12).
//
// Witness (SMR-1 冷读 F-0 排查, donghu 40422): prose "CPU 占用 132.041ms 来自
// window_stats 聚合" — 132.041 = 96.081 (cpu·ms occupancy caliber) + 35.960
// (per-CPU thread_cpu_load caliber), a model-side cross-caliber stitch no
// single engine caliber publishes. The self-sum arm already grounded and
// disclosed it as 自行加和; 臂5 upgrades the disclosure to name the caliber
// stitch when both verified sides are typed-determinate and different.

import (
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/types"
)

// TestCR4Arm5_CrossCaliberSumDisclosure — the 132.041 witness: a cpu·ms
// occupancy row (cross_thread_cpu_ms registry token) + a thread_cpu_load
// row sum to the prose value → the disclosure states the cross-caliber
// stitch with both caliber labels.
func TestCR4Arm5_CrossCaliberSumDisclosure(t *testing.T) {
	occ := psgTraceRecord("trace_query:t#root_cause_rank:9", "root_cause_background", "96.081",
		"type=cpu_pressure", "tier=context_only")
	occ.Subject = ".ugc.aweme.lite-17267"
	occ.Object = "cpu_pressure"
	load := psgTraceRecord("trace_query:t#thread_cpu_load:1", "thread_cpu_load:.ugc.aweme.lite-17267", "35.960",
		"cpu=12", "freq=2340000kHz")
	load.Subject = ".ugc.aweme.lite-17267"
	load.Predicate = "thread_cpu_load"
	mut := psgTraceMutable(occ, load)
	bus := psgBus(mut)
	doc := psgProseDoc("目标线程 CPU 占用 132.041ms 来自 window_stats 聚合，分布在 cpu=12 和 cpu=4。")

	_, advisory := proseScalarResidualAppendixInputs(doc, bus, mut)
	var hit string
	for _, f := range advisory {
		if strings.Contains(f.entryZH, "132.041") {
			hit = f.entryZH
		}
	}
	if hit == "" {
		t.Fatalf("the 132.041 witness must disclose, got %+v", advisory)
	}
	if !strings.Contains(hit, "未在证据面单独发布") || !strings.Contains(hit, "两侧口径不同，不可直接相加") {
		t.Fatalf("the disclosure must state the cross-caliber stitch as facts: %s", hit)
	}
	if !strings.Contains(hit, "96.081[cpu·ms 跨核占用]") || !strings.Contains(hit, "35.960[单核线程时间 ms]") {
		t.Fatalf("the disclosure must label both calibers: %s", hit)
	}
}

// TestCR4Arm5_SameCaliberKeepsPlainWording — green side: a same-caliber
// (both wall-clock) verified pair keeps the plain 自行加和 wording — no
// cross-caliber claim.
func TestCR4Arm5_SameCaliberKeepsPlainWording(t *testing.T) {
	a := psgTraceRecord("trace_query:t#state_drilldown:1", "state_drilldown:a", "6.308")
	a.Subject = "keva-3-17439"
	b := psgTraceRecord("trace_query:t#state_drilldown:2", "state_drilldown:b", "4.795")
	b.Subject = "keva-3-17439"
	mut := psgTraceMutable(a, b)
	bus := psgBus(mut)
	doc := psgProseDoc("阻塞合计 11.103ms。")

	_, advisory := proseScalarResidualAppendixInputs(doc, bus, mut)
	var hit string
	for _, f := range advisory {
		if strings.Contains(f.entryZH, "11.103") {
			hit = f.entryZH
		}
	}
	if hit == "" {
		t.Fatalf("the self-sum disclosure must fire, got %+v", advisory)
	}
	if strings.Contains(hit, "口径不同") {
		t.Fatalf("a same-caliber pair must keep the plain wording: %s", hit)
	}
	if !strings.Contains(hit, "未在证据面单独发布") || !strings.Contains(hit, "可由已发布值") {
		t.Fatalf("the plain self-sum fact wording must render: %s", hit)
	}
}

// TestCR4Arm5_TextUnitWordTagsCPUms — the engine's printed unit word on a
// system evidence line ("running=96.081cpu·ms") tags the row without any
// registry token (banner-shape carrier).
func TestCR4Arm5_TextUnitWordTagsCPUms(t *testing.T) {
	occ := psgTraceRecord("trace_query:t#runnable_occupancy:1", "runnable_occupancy:x", "")
	occ.Subject = ""
	occ.Predicate = "runnable_occupancy"
	occ.Summary = "cpu=12 core_class=big running=96.081cpu·ms top occupiers (cpu·ms)"
	load := psgTraceRecord("trace_query:t#thread_cpu_load:1", "thread_cpu_load:x", "35.960")
	load.Subject = ".ugc.aweme.lite-17267"
	load.Predicate = "thread_cpu_load"
	mut := psgTraceMutable(occ, load)
	bus := psgBus(mut)
	doc := psgProseDoc("CPU 侧合计 132.041ms。")

	_, advisory := proseScalarResidualAppendixInputs(doc, bus, mut)
	var hit string
	for _, f := range advisory {
		if strings.Contains(f.entryZH, "132.041") {
			hit = f.entryZH
		}
	}
	if hit == "" {
		t.Fatalf("the banner-shape witness must disclose, got %+v", advisory)
	}
	if !strings.Contains(hit, "两侧口径不同，不可直接相加") {
		t.Fatalf("the text-tagged cpu·ms side must trigger the cross-caliber wording: %s", hit)
	}
}

// cr4Arm5CaliberRecord pins proseScalarRecordCaliber's typed classification
// directly (registry additivity + predicate + score unit).
func TestCR4Arm5_RecordCaliberClassifier(t *testing.T) {
	mk := func(pred, obj, unit string, notes ...string) types.ObservationRecord {
		rec := psgTraceRecord("trace_query:t#x:1", pred+":x", "1.0", notes...)
		rec.Predicate = pred
		rec.Object = obj
		rec.Unit = unit
		return rec
	}
	cases := []struct {
		rec  types.ObservationRecord
		want string
	}{
		{mk("thread_cpu_load", "cpu=3", "ms"), proseScalarCaliberThreadCPULoad},
		{mk("root_cause_background", "cpu_pressure", "ms"), proseScalarCaliberCPUMS},
		{mk("root_cause_rank_4", "supply_pressure", "ms"), proseScalarCaliberCPUMS},
		{mk("root_cause_caliber_side", "page_cache_churn", "ms"), proseScalarCaliberCountEquiv},
		{mk("root_cause_caliber_side", "block_io_by_inode", "ms"), proseScalarCaliberScore},
		{mk("io_pressure", "io_pressure", "score"), proseScalarCaliberScore},
		{mk("state_drilldown", "s_sleep", "ms"), ""},
		// The type= note wins over Object (rank rows republishing a token).
		{mk("root_cause_primary", "", "ms", "type=compute_supply"), proseScalarCaliberCPUMS},
	}
	for i, c := range cases {
		if got := proseScalarRecordCaliber(c.rec); got != c.want {
			t.Fatalf("case %d (%s/%s): caliber = %q, want %q", i, c.rec.Predicate, c.rec.Object, got, c.want)
		}
	}
}

// TestCR4Arm5_SentenceNamedPairOutranksSameThread — 复放实证 (donghu 73346):
// 「keva-1/keva-3 各自的 io_wait 叠加 2.162ms」 must render its OWN claimed
// decomposition 0.808 + 1.354 (loose single-digit names matched to carrier
// spellings), never the coincidental same-thread pair (0.049 + 2.113).
func TestCR4Arm5_SentenceNamedPairOutranksSameThread(t *testing.T) {
	k1 := psgTraceRecord("trace_query:t#root_cause_rank:5", "root_cause_rank_5", "0.808", "rank=5")
	k1.Subject = "keva-1-17437"
	k3 := psgTraceRecord("trace_query:t#root_cause_rank:4", "root_cause_rank_4", "1.354", "rank=4")
	k3.Subject = "keva-3-17439"
	// The coincidental same-thread pair on the target's rows.
	c1 := psgTraceRecord("trace_query:t#state_drilldown:1", "state_drilldown:a", "0.049")
	c1.Subject = ".ugc.aweme.lite-17267"
	c2 := psgTraceRecord("trace_query:t#state_drilldown:2", "state_drilldown:b", "2.113")
	c2.Subject = ".ugc.aweme.lite-17267"
	mut := psgTraceMutable(k1, k3, c1, c2)
	bus := psgBus(mut)
	doc := psgProseDoc("keva-1/keva-3 各自的 io_wait 叠加 2.162ms,建议对文件 IO 增加缓存。")

	_, advisory := proseScalarResidualAppendixInputs(doc, bus, mut)
	var hit string
	for _, f := range advisory {
		if strings.Contains(f.entryZH, "2.162") {
			hit = f.entryZH
		}
	}
	if hit == "" {
		t.Fatalf("the self-sum disclosure must fire, got %+v", advisory)
	}
	if !strings.Contains(hit, "0.808") || !strings.Contains(hit, "1.354") {
		t.Fatalf("the sentence-named decomposition must render: %s", hit)
	}
	if strings.Contains(hit, "0.049") || strings.Contains(hit, "2.113") {
		t.Fatalf("the coincidental same-thread pair must not render: %s", hit)
	}
	if !strings.Contains(hit, "跨主体") {
		t.Fatalf("the cross-subject qualifier must ride the named pair: %s", hit)
	}
}
