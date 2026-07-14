package tracequery

// cluster_rail_evidence_cap2_test.go — CAP-2 batch (§28.4 五重门+负向筛 /
// §28.5 两级证据梯, docs/design/real_trace_campaign_20260705.md, 2026-07-09)
// pins. Witness material is VERBATIM from the customer specimen fragments
// /Users/han/opt/customlogs/cust_trace_cpu.txt (keyword-filtered excerpt of
// the same trace as cust_trace_texture_upload.txt):
//
//	m3_c0..c3_freq   keyed rails, anchors 0/2/10/12, values 417000/417000/
//	                 1200000/2350000 (§28.2-T1);
//	thermal_inte1/2/3 the thermal index family — inte1 wanders anchors
//	                 2/3/4/5/7 (§28.5-T6 ③ witness), inte2/inte3 constant
//	                 anchors 10/12 (the short-window pseudo family ⑥ kills);
//	heca_info        single name sweeping every CPU, non-frequency values
//	                 (§28.5-T9 — ③+⑥ double kill);
//	pid_freq         name-heuristic whitelisted lane with the 10240923
//	                 non-kHz encoding (§28.5-T8 → 任务6 filter);
//	cpu_frequency    full sweeps {0,1}{2..9}{10,11}{12,13} — the fragment
//	                 keeps 10..13 constant-equal (§28.5-T5: honest Tier-1
//	                 merge, Tier-2 subdivides);
//	cpu_frequency_limits cpu0 1750000→1550000 (dynamic), cpu2 2200000,
//	                 cpu10 2295000.

import (
	"fmt"
	"strings"
	"testing"
)

// --- verbatim specimen lines --------------------------------------------------

const cap2M3RailLines = `
    tppmgr-idle-0-296   (    2) [000] .... 15151.855460: clock_set_rate: m3_c0_freq state=417000 cpu_id=0
    tppmgr-idle-0-296   (    2) [000] .... 15151.855461: clock_set_rate: m3_vote_delay state=305 cpu_id=0
    tppmgr-idle-0-296   (    2) [000] .... 15151.855462: clock_set_rate: m3_c1_freq state=417000 cpu_id=2
    tppmgr-idle-0-296   (    2) [000] .... 15151.855462: clock_set_rate: m3_c2_freq state=1200000 cpu_id=10
    tppmgr-idle-0-296   (    2) [000] .... 15151.855463: clock_set_rate: m3_c3_freq state=2350000 cpu_id=12
`

const cap2ThermalInte1Lines = `
     binder:226_8-61360 (11427) [004] .... 15152.033937: clock_set_rate: thermal_inte1 state=1850000 cpu_id=4
  android.display-11850 (11808) [003] .... 15152.034081: clock_set_rate: thermal_inte1 state=2200000 cpu_id=3
    tppmgr-idle-3-299   (    2) [003] .... 15152.034142: clock_set_rate: thermal_inte1 state=1850000 cpu_id=3
    tppmgr-idle-5-301   (    2) [005] .... 15152.034205: clock_set_rate: thermal_inte1 state=1850000 cpu_id=5
    tppmgr-idle-2-298   (    2) [002] .... 15152.034300: clock_set_rate: thermal_inte1 state=1850000 cpu_id=2
      mmi_service-2559  (  982) [003] .... 15152.034902: clock_set_rate: thermal_inte1 state=2200000 cpu_id=3
`

const cap2ThermalInte23Lines = `
   tppmgr-idle-12-308   (    2) [012] .... 15152.034379: clock_set_rate: thermal_inte3 state=2350000 cpu_id=12
     RenderThread-51342 (50820) [010] .... 15152.041723: clock_set_rate: thermal_inte2 state=2295000 cpu_id=10
   tppmgr-idle-10-306   (    2) [010] .... 15152.041743: clock_set_rate: thermal_inte2 state=1990000 cpu_id=10
`

const cap2HecaInfoLines = `
    tppmgr-idle-0-296   (    2) [000] .... 15151.846360: clock_set_rate: heca_info state=23303 cpu_id=5
    tppmgr-idle-0-296   (    2) [000] .... 15151.846362: clock_set_rate: heca_info state=7 cpu_id=6
    tppmgr-idle-0-296   (    2) [000] .... 15151.846363: clock_set_rate: heca_info state=23303 cpu_id=7
    tppmgr-idle-0-296   (    2) [000] .... 15151.846363: clock_set_rate: heca_info state=40967 cpu_id=8
`

const cap2PidFreqLines = `
  unyuan.app.chat-50820 (50820) [012] .... 15151.848021: clock_set_rate: pid_freq state=10240923 cpu_id=12
    tppmgr-idle-8-304   (    2) [008] .... 15151.848017: clock_set_rate: pid_freq state=190091 cpu_id=8
`

// cap2SweepLines are the first three verbatim full sweeps (rows 8631-8871 of
// the fragment): {0,1} 417000→1090000→1090000, {2..9} 1744000→417000→417000,
// {10..13} constant 1200000 — the Tier-1 honest-merge shape.
const cap2SweepLines = `
  RSUniRenderThre-2301  ( 1855) [000] .... 15151.824209: cpu_frequency: state=417000 cpu_id=0
  RSUniRenderThre-2301  ( 1855) [000] .... 15151.824210: cpu_frequency: state=417000 cpu_id=1
  RSUniRenderThre-2301  ( 1855) [000] .... 15151.824229: cpu_frequency: state=1744000 cpu_id=2
  RSUniRenderThre-2301  ( 1855) [000] .... 15151.824231: cpu_frequency: state=1744000 cpu_id=3
  RSUniRenderThre-2301  ( 1855) [000] .... 15151.824231: cpu_frequency: state=1744000 cpu_id=4
  RSUniRenderThre-2301  ( 1855) [000] .... 15151.824232: cpu_frequency: state=1744000 cpu_id=5
  RSUniRenderThre-2301  ( 1855) [000] .... 15151.824233: cpu_frequency: state=1744000 cpu_id=6
  RSUniRenderThre-2301  ( 1855) [000] .... 15151.824233: cpu_frequency: state=1744000 cpu_id=7
  RSUniRenderThre-2301  ( 1855) [000] .... 15151.824234: cpu_frequency: state=1744000 cpu_id=8
  RSUniRenderThre-2301  ( 1855) [000] .... 15151.824234: cpu_frequency: state=1744000 cpu_id=9
  RSUniRenderThre-2301  ( 1855) [000] .... 15151.824241: cpu_frequency: state=1200000 cpu_id=10
  RSUniRenderThre-2301  ( 1855) [000] .... 15151.824242: cpu_frequency: state=1200000 cpu_id=11
  RSUniRenderThre-2301  ( 1855) [000] .... 15151.824246: cpu_frequency: state=1200000 cpu_id=12
  RSUniRenderThre-2301  ( 1855) [000] .... 15151.824247: cpu_frequency: state=1200000 cpu_id=13
       hilogd.pst-647   (  629) [000] .... 15151.825217: cpu_frequency: state=1090000 cpu_id=0
       hilogd.pst-647   (  629) [000] .... 15151.825218: cpu_frequency: state=1090000 cpu_id=1
       hilogd.pst-647   (  629) [000] .... 15151.825242: cpu_frequency: state=417000 cpu_id=2
       hilogd.pst-647   (  629) [000] .... 15151.825243: cpu_frequency: state=417000 cpu_id=3
       hilogd.pst-647   (  629) [000] .... 15151.825244: cpu_frequency: state=417000 cpu_id=4
       hilogd.pst-647   (  629) [000] .... 15151.825245: cpu_frequency: state=417000 cpu_id=5
       hilogd.pst-647   (  629) [000] .... 15151.825246: cpu_frequency: state=417000 cpu_id=6
       hilogd.pst-647   (  629) [000] .... 15151.825247: cpu_frequency: state=417000 cpu_id=7
       hilogd.pst-647   (  629) [000] .... 15151.825247: cpu_frequency: state=417000 cpu_id=8
       hilogd.pst-647   (  629) [000] .... 15151.825248: cpu_frequency: state=417000 cpu_id=9
       hilogd.pst-647   (  629) [000] .... 15151.825253: cpu_frequency: state=1200000 cpu_id=10
       hilogd.pst-647   (  629) [000] .... 15151.825254: cpu_frequency: state=1200000 cpu_id=11
       hilogd.pst-647   (  629) [000] .... 15151.825258: cpu_frequency: state=1200000 cpu_id=12
       hilogd.pst-647   (  629) [000] .... 15151.825258: cpu_frequency: state=1200000 cpu_id=13
  hwc_vsync_threa-11441 (11398) [000] .... 15151.826221: cpu_frequency: state=1090000 cpu_id=0
  hwc_vsync_threa-11441 (11398) [000] .... 15151.826222: cpu_frequency: state=1090000 cpu_id=1
  hwc_vsync_threa-11441 (11398) [000] .... 15151.826243: cpu_frequency: state=417000 cpu_id=2
  hwc_vsync_threa-11441 (11398) [000] .... 15151.826243: cpu_frequency: state=417000 cpu_id=3
  hwc_vsync_threa-11441 (11398) [000] .... 15151.826244: cpu_frequency: state=417000 cpu_id=4
  hwc_vsync_threa-11441 (11398) [000] .... 15151.826245: cpu_frequency: state=417000 cpu_id=5
  hwc_vsync_threa-11441 (11398) [000] .... 15151.826245: cpu_frequency: state=417000 cpu_id=6
  hwc_vsync_threa-11441 (11398) [000] .... 15151.826246: cpu_frequency: state=417000 cpu_id=7
  hwc_vsync_threa-11441 (11398) [000] .... 15151.826247: cpu_frequency: state=417000 cpu_id=8
  hwc_vsync_threa-11441 (11398) [000] .... 15151.826248: cpu_frequency: state=417000 cpu_id=9
  hwc_vsync_threa-11441 (11398) [000] .... 15151.826254: cpu_frequency: state=1200000 cpu_id=10
  hwc_vsync_threa-11441 (11398) [000] .... 15151.826254: cpu_frequency: state=1200000 cpu_id=11
  hwc_vsync_threa-11441 (11398) [000] .... 15151.826263: cpu_frequency: state=1200000 cpu_id=12
  hwc_vsync_threa-11441 (11398) [000] .... 15151.826263: cpu_frequency: state=1200000 cpu_id=13
`

const cap2LimitsLines = `
  RSUniRenderThre-2301  ( 1855) [000] .... 15151.824213: cpu_frequency_limits: min=417000 max=1750000 cpu_id=0
       hilogd.pst-647   (  629) [000] .... 15151.825220: cpu_frequency_limits: min=417000 max=1550000 cpu_id=0
    tppmgr-idle-0-296   (    2) [000] .... 15151.827241: cpu_frequency_limits: min=417000 max=2200000 cpu_id=2
    tppmgr-idle-0-296   (    2) [000] .... 15151.827244: cpu_frequency_limits: min=1200000 max=2295000 cpu_id=10
`

// cap2SchedFiller emits one idle sched_switch per CPU 0..13 so the gate-⑤
// universe and the membership presumption bound cover the specimen platform.
func cap2SchedFiller() string {
	var b strings.Builder
	for cpu := 0; cpu <= 13; cpu++ {
		fmt.Fprintf(&b, "   filler-%d (  9%02d) [%03d] .... 15151.820%03d: sched_switch: prev_comm=idle/%d prev_pid=0 prev_prio=120 prev_state=R ==> next_comm=filler next_pid=9%02d next_prio=120\n",
			800+cpu, cpu, cpu, cpu, cpu, cpu)
	}
	return b.String()
}

// cap2SchedCPUs is the matching gate-⑤ set for direct scan calls.
func cap2SchedCPUs() map[int]bool {
	set := map[int]bool{}
	for cpu := 0; cpu <= 13; cpu++ {
		set[cpu] = true
	}
	return set
}

// cap2DepBody synthesizes the app/dep wakeup pattern with the dependency
// RUNNING on depCPU for ~9.9ms inside [15152.000, 15152.010].
func cap2DepBody(depCPU int) string {
	return fmt.Sprintf(`
        app-100 (100) [001] .... 15151.990000: sched_switch: prev_comm=idle/1 prev_pid=0 prev_prio=120 prev_state=R ==> next_comm=app next_pid=100 next_prio=52
        app-100 (100) [001] .... 15152.000000: sched_switch: prev_comm=app prev_pid=100 prev_prio=52 prev_state=S ==> next_comm=idle/1 next_pid=0 next_prio=120
        dep-200 (100) [%03d] .... 15152.000000: sched_switch: prev_comm=idle/%d prev_pid=0 prev_prio=120 prev_state=R ==> next_comm=dep next_pid=200 next_prio=20
        dep-200 (100) [%03d] .... 15152.009900: sched_wakeup: comm=app pid=100 prio=52 target_cpu=001
        dep-200 (100) [%03d] .... 15152.010000: sched_switch: prev_comm=dep prev_pid=200 prev_prio=20 prev_state=S ==> next_comm=idle/%d next_pid=0 next_prio=120
        app-100 (100) [001] .... 15152.010000: sched_switch: prev_comm=idle/1 prev_pid=0 prev_prio=120 prev_state=R ==> next_comm=app next_pid=100 next_prio=52
`, depCPU, depCPU, depCPU, depCPU, depCPU)
}

var cap2FoldQuery = Query{PID: 100, TimeStart: 15152.0, TimeEnd: 15152.010, MaxDepth: 4, MinDurationMs: 0.05, TraceFlavorHint: TraceFlavorHarmonyHitrace}

// cap2DepBodyLate is the same pattern shifted into [15152.034, 15152.044] so
// the governance window covers the verbatim thermal_inte1 samples.
func cap2DepBodyLate(depCPU int) string {
	return fmt.Sprintf(`
        app-100 (100) [001] .... 15152.024000: sched_switch: prev_comm=idle/1 prev_pid=0 prev_prio=120 prev_state=R ==> next_comm=app next_pid=100 next_prio=52
        app-100 (100) [001] .... 15152.034000: sched_switch: prev_comm=app prev_pid=100 prev_prio=52 prev_state=S ==> next_comm=idle/1 next_pid=0 next_prio=120
        dep-200 (100) [%03d] .... 15152.034000: sched_switch: prev_comm=idle/%d prev_pid=0 prev_prio=120 prev_state=R ==> next_comm=dep next_pid=200 next_prio=20
        dep-200 (100) [%03d] .... 15152.043900: sched_wakeup: comm=app pid=100 prio=52 target_cpu=001
        dep-200 (100) [%03d] .... 15152.044000: sched_switch: prev_comm=dep prev_pid=200 prev_prio=20 prev_state=S ==> next_comm=idle/%d next_pid=0 next_prio=120
        app-100 (100) [001] .... 15152.044000: sched_switch: prev_comm=idle/1 prev_pid=0 prev_prio=120 prev_state=R ==> next_comm=app next_pid=100 next_prio=52
`, depCPU, depCPU, depCPU, depCPU, depCPU)
}

var cap2FoldQueryLate = Query{PID: 100, TimeStart: 15152.034, TimeEnd: 15152.044, MaxDepth: 4, MinDurationMs: 0.05, TraceFlavorHint: TraceFlavorHarmonyHitrace}

// --- ① 族形门 -----------------------------------------------------------------

func TestCAP2FamilyGateAdoptsM3FamilyAndRejectsFamilyless(t *testing.T) {
	idx := buildTraceIndex(t, "cap2_family.systrace", cap2SchedFiller()+cap2M3RailLines+cap2HecaInfoLines+cap2PidFreqLines)
	scan := scanClusterRailEvidence(idx.Events, cap2SchedCPUs())
	if scan.adoption == nil || scan.adoption.family != "m3_c#_freq" {
		t.Fatalf("① the m3_c#_freq family (≥2 rails, one digit-run mask) must adopt: %+v", scan)
	}
	// Witness anchors + values (§28.4 verbatim数值): 0/2/10/12 →
	// 417000/417000/1200000/2350000; membership = anchor contiguity to the
	// highest scheduler-observed CPU (13).
	wantAnchors := []int{0, 2, 10, 12}
	wantFmax := []int{417000, 417000, 1200000, 2350000}
	wantMembers := [][]int{{0, 1}, {2, 3, 4, 5, 6, 7, 8, 9}, {10, 11}, {12, 13}}
	if len(scan.adoption.clusters) != 4 {
		t.Fatalf("want 4 rail clusters, got %+v", scan.adoption.clusters)
	}
	for i, cluster := range scan.adoption.clusters {
		if cluster.anchor != wantAnchors[i] || cluster.fmaxKHz != wantFmax[i] {
			t.Fatalf("cluster %d anchor/fmax = %d/%d, want %d/%d", i, cluster.anchor, cluster.fmaxKHz, wantAnchors[i], wantFmax[i])
		}
		if fmt.Sprint(cluster.members) != fmt.Sprint(wantMembers[i]) {
			t.Fatalf("cluster %d members = %v, want %v (锚点连续推定)", i, cluster.members, wantMembers[i])
		}
	}
	// ① negative: heca_info / pid_freq carry no digit-run family shape — no
	// mask of theirs may appear anywhere in the scan output.
	for family := range scan.rejected {
		if strings.Contains(family, "heca") || strings.Contains(family, "pid") {
			t.Fatalf("family-less rails must never form a candidate family: %+v", scan.rejected)
		}
	}
	if masks := railFamilyMasks("heca_info"); len(masks) != 0 {
		t.Fatalf("heca_info has no digit run and therefore no family mask: %v", masks)
	}
}

func TestCAP2FamilyGateSingleRailNeverAdopts(t *testing.T) {
	single := `
    tppmgr-idle-0-296   (    2) [000] .... 15151.855460: clock_set_rate: m3_c0_freq state=417000 cpu_id=0
`
	idx := buildTraceIndex(t, "cap2_single_rail.systrace", cap2SchedFiller()+single)
	scan := scanClusterRailEvidence(idx.Events, cap2SchedCPUs())
	if scan.adoption != nil {
		t.Fatalf("① a 1-member mask is not a family (≥2 required): %+v", scan.adoption)
	}
}

// --- ② 异锚门 -----------------------------------------------------------------

// The keyless positional shape (hmtrace flavor form "clock_set_rate:
// cpu-cluster.N <rate>") has NO cpu_id key — the emitting-CPU fallback is not
// a key, so the family fails ② (§28.4 补充: flavor-neutral wiring, the
// keyless FORM falls back honestly on any flavor — 回退形 pin; the keyed form
// passing above is the 过门形).
func TestCAP2AnchorGateRejectsKeylessPositionalForm(t *testing.T) {
	positional := `
          clk-90 (  90) [000] .... 15151.850000: clock_set_rate: cpu-cluster.0 2200000.0
          clk-90 (  90) [000] .... 15151.850001: clock_set_rate: cpu-cluster.1 1800000.0
          clk-90 (  90) [000] .... 15151.850002: clock_set_rate: cpu-cluster.2 1000000.0
`
	idx := buildTraceIndex(t, "cap2_positional.systrace", cap2SchedFiller()+positional)
	scan := scanClusterRailEvidence(idx.Events, cap2SchedCPUs())
	if scan.adoption != nil {
		t.Fatalf("② keyless rails must never adopt (绑定语义只认 cpu_id 键控字段): %+v", scan.adoption)
	}
	if scan.rejected["cpu-cluster.#"] != clusterRailRejectAnchorKeyMissing {
		t.Fatalf("② rejection reason = %q, want %q (%+v)", scan.rejected["cpu-cluster.#"], clusterRailRejectAnchorKeyMissing, scan.rejected)
	}
}

// Two family members claiming ONE anchor cannot both be cluster keys. The
// witness motivating the composite gate is the CROSS-family anchor collision
// (thermal_inte1 anchors cpu_id=2 — the same anchor as m3_c1_freq), so the
// within-family form is built from the specimen line SHAPES with the anchor
// duplicated.
func TestCAP2AnchorGateRejectsCollidingAnchors(t *testing.T) {
	colliding := `
    tppmgr-idle-0-296   (    2) [000] .... 15151.855460: clock_set_rate: m3_c0_freq state=417000 cpu_id=2
    tppmgr-idle-0-296   (    2) [000] .... 15151.855462: clock_set_rate: m3_c1_freq state=417000 cpu_id=2
`
	idx := buildTraceIndex(t, "cap2_collision.systrace", cap2SchedFiller()+colliding)
	scan := scanClusterRailEvidence(idx.Events, cap2SchedCPUs())
	if scan.adoption != nil || scan.rejected["m3_c#_freq"] != clusterRailRejectAnchorCollision {
		t.Fatalf("② duplicate anchors must reject the family: %+v / %+v", scan.adoption, scan.rejected)
	}
}

// --- ③ 不变式门 ---------------------------------------------------------------

// The specimen witnesses: heca_info sweeps every CPU (单名扫全 CPU) and
// thermal_inte1 wanders 2/3/4/5/7 — their cpu_id is telemetry attribution,
// not a key. The gate function is pinned DIRECTLY on the verbatim candidates
// (the pipeline never shows them to ③: heca_info has no family, thermal is
// ⑥-excluded — defense in depth is exactly the point).
func TestCAP2InvariantGateRejectsWanderingAnchors(t *testing.T) {
	idx := buildTraceIndex(t, "cap2_invariant.systrace", cap2SchedFiller()+cap2HecaInfoLines+cap2ThermalInte1Lines+cap2M3RailLines)
	candidates := collectClusterRailCandidates(idx.Events)
	if cand := candidates["heca_info"]; cand == nil || clusterRailInvariantGateOK(cand) {
		t.Fatalf("③ heca_info (anchors 5/6/7/8) must fail the invariant gate: %+v", cand)
	}
	if cand := candidates["thermal_inte1"]; cand == nil || clusterRailInvariantGateOK(cand) {
		t.Fatalf("③ thermal_inte1 (anchors 4/3/3/5/2/3) must fail the invariant gate: %+v", cand)
	}
	if cand := candidates["m3_c0_freq"]; cand == nil || !clusterRailInvariantGateOK(cand) {
		t.Fatalf("③ a constant-anchor rail must pass: %+v", cand)
	}
	// Family-level fail-loud (违者整体回退): a family carrying one wandering
	// member is rejected WHOLE — built from the m3 line shape with one rail
	// re-anchored mid-trace.
	wandering := cap2M3RailLines + `
    tppmgr-idle-0-296   (    2) [000] .... 15151.856000: clock_set_rate: m3_c3_freq state=2350000 cpu_id=13
`
	idx2 := buildTraceIndex(t, "cap2_invariant_family.systrace", cap2SchedFiller()+wandering)
	scan := scanClusterRailEvidence(idx2.Events, cap2SchedCPUs())
	if scan.adoption != nil || scan.rejected["m3_c#_freq"] != clusterRailRejectAnchorUnstable {
		t.Fatalf("③ a wandering member must reject the WHOLE family: %+v / %+v", scan.adoption, scan.rejected)
	}
}

// --- ④ 量纲门 -----------------------------------------------------------------

func TestCAP2DimensionGate(t *testing.T) {
	idx := buildTraceIndex(t, "cap2_dimension.systrace", cap2SchedFiller()+cap2HecaInfoLines+cap2M3RailLines)
	candidates := collectClusterRailCandidates(idx.Events)
	if cand := candidates["heca_info"]; clusterRailDimensionGateOK(cand) {
		t.Fatalf("④ heca_info values (7/23303/40967 — non-frequency encodings) must fail the band: %+v", cand)
	}
	if cand := candidates["m3_c3_freq"]; !clusterRailDimensionGateOK(cand) {
		t.Fatalf("④ 2350000 kHz must pass the band: %+v", cand)
	}
	// Family-level: one member carrying the specimen's pid_freq 10240923
	// encoding (10.24GHz as kHz) rejects the family whole.
	outOfBand := `
    tppmgr-idle-0-296   (    2) [000] .... 15151.855460: clock_set_rate: m3_c0_freq state=417000 cpu_id=0
    tppmgr-idle-0-296   (    2) [000] .... 15151.855462: clock_set_rate: m3_c1_freq state=10240923 cpu_id=2
`
	idx2 := buildTraceIndex(t, "cap2_dimension_family.systrace", cap2SchedFiller()+outOfBand)
	scan := scanClusterRailEvidence(idx2.Events, cap2SchedCPUs())
	if scan.adoption != nil || scan.rejected["m3_c#_freq"] != clusterRailRejectDimension {
		t.Fatalf("④ an out-of-band member must reject the family: %+v / %+v", scan.adoption, scan.rejected)
	}
}

// --- ⑤ 相容门 -----------------------------------------------------------------

func TestCAP2CompatibilityGateAnchorsMustBeScheduled(t *testing.T) {
	idx := buildTraceIndex(t, "cap2_compat.systrace", cap2SchedFiller()+cap2M3RailLines)
	// A universe without cpu12 (anchor of m3_c3_freq) rejects the family.
	small := map[int]bool{}
	for cpu := 0; cpu <= 9; cpu++ {
		small[cpu] = true
	}
	small[10] = true
	scan := scanClusterRailEvidence(idx.Events, small)
	if scan.adoption != nil || scan.rejected["m3_c#_freq"] != clusterRailRejectAnchorOutsideCPUs {
		t.Fatalf("⑤ an anchor outside the scheduler-observed set must reject: %+v / %+v", scan.adoption, scan.rejected)
	}
	// Positive control: the full 0..13 universe adopts (see the ① pin).
	if scan := scanClusterRailEvidence(idx.Events, cap2SchedCPUs()); scan.adoption == nil {
		t.Fatalf("⑤ positive control must adopt: %+v", scan.rejected)
	}
}

// --- ⑥ 负向词汇筛 (§28.5-T6) ---------------------------------------------------

// The short-window pseudo family: only thermal_inte2 (constant anchor 10) +
// thermal_inte3 (constant anchor 12) are visible — they would pass ①-⑤
// (constant distinct keyed anchors, kHz band, scheduled CPUs). The
// exclusion-only vocabulary filter is the ruled guard; deleting it is the
// named strongest mutation ("负向筛删除后 thermal 伪族过门") and this pin
// catches it.
func TestCAP2NegativeVocabularyFilterKillsThermalPseudoFamily(t *testing.T) {
	idx := buildTraceIndex(t, "cap2_thermal_pseudo.systrace", cap2SchedFiller()+cap2ThermalInte23Lines)
	scan := scanClusterRailEvidence(idx.Events, cap2SchedCPUs())
	if scan.adoption != nil {
		t.Fatalf("⑥ the thermal_inte2/3 pseudo family must be excluded BEFORE family formation: %+v", scan.adoption)
	}
	if _, evaluated := scan.rejected["thermal_inte#"]; evaluated {
		t.Fatalf("⑥ is exclusion-only and pre-family: thermal must never even be evaluated: %+v", scan.rejected)
	}
	// The other specimen exclusions stay out too (ddr/gpu/vote/delay/info/load).
	for _, name := range []string{"m3_vote_delay", "heca_ddr_freq", "gpufreq_info", "gpuload"} {
		if !clusterRailNameExcluded(name) {
			t.Fatalf("⑥ %s must be vocabulary-excluded", name)
		}
	}
	if clusterRailNameExcluded("m3_c0_freq") {
		t.Fatalf("⑥ is exclusion-only — it must never veto the keyed family name")
	}
}

// 复核收尾 P3-2 (§28.5-T6 残洞实证收口): a temperature index family whose
// milli-°C values land inside the kHz band (soc_temp1=65000 → 65.000°C ↔
// 65000 kHz = 65MHz) clears gates ①-⑤ with constant distinct keyed anchors —
// the vocabulary filter's "temp"/"tsens" tokens are the ruled guard.
func TestCAP2NegativeVocabularyFilterKillsTempPseudoFamily(t *testing.T) {
	tempRails := `
          soctrl-90 (  90) [000] .... 15151.850000: clock_set_rate: soc_temp1 state=65000 cpu_id=0
          soctrl-90 (  90) [000] .... 15151.850001: clock_set_rate: soc_temp2 state=72000 cpu_id=4
          soctrl-90 (  90) [000] .... 15151.850002: clock_set_rate: tsens0_cpu state=68000 cpu_id=2
`
	idx := buildTraceIndex(t, "cap2_soc_temp.systrace", cap2SchedFiller()+tempRails)
	scan := scanClusterRailEvidence(idx.Events, cap2SchedCPUs())
	if scan.adoption != nil {
		t.Fatalf("⑥ the soc_temp pseudo family must be vocabulary-excluded: %+v", scan.adoption)
	}
	if _, evaluated := scan.rejected["soc_temp#"]; evaluated {
		t.Fatalf("⑥ is pre-family: soc_temp must never be evaluated: %+v", scan.rejected)
	}
	for _, name := range []string{"soc_temp1", "tsens0_cpu"} {
		if !clusterRailNameExcluded(name) {
			t.Fatalf("⑥ %s must be vocabulary-excluded (temp/tsens tokens)", name)
		}
	}
}

// Ambiguity: two structurally-valid families cannot both be adopted (不部分
// 猜测) — both fall back.
func TestCAP2AmbiguousFamiliesFallBack(t *testing.T) {
	second := `
          clk-90 (  90) [000] .... 15151.850000: clock_set_rate: p9_c0_rail state=500000 cpu_id=0
          clk-90 (  90) [000] .... 15151.850001: clock_set_rate: p9_c1_rail state=800000 cpu_id=4
`
	idx := buildTraceIndex(t, "cap2_ambiguous.systrace", cap2SchedFiller()+cap2M3RailLines+second)
	scan := scanClusterRailEvidence(idx.Events, cap2SchedCPUs())
	if scan.adoption != nil {
		t.Fatalf("two passing families are ambiguous — adopt neither: %+v", scan.adoption)
	}
	if scan.rejected["m3_c#_freq"] != clusterRailRejectAmbiguousFamilies || scan.rejected["p9_c#_rail"] != clusterRailRejectAmbiguousFamilies {
		t.Fatalf("both families must record the ambiguity reason: %+v", scan.rejected)
	}
}

// --- Tier-1 样本数下限门 -------------------------------------------------------

func TestCAP2ComovementFloor(t *testing.T) {
	single := map[int][]freqSample{
		0: {{ts: 1.0, khz: 1000000}},
		1: {{ts: 1.000001, khz: 1000000}},
		4: {{ts: 1.0, khz: 2000000}},
	}
	capability := resolveCoreCapability(deriveClusterFreqDomains(single), single)
	if capability.source != CoreCapabilitySourceFreqOnly || !capability.comoveFloorTripped {
		t.Fatalf("a multi-CPU merge on ONE coincident sample is not co-movement — fail loud: %+v", capability.source)
	}
	// ≥2 samples: the same merge is witnessed co-movement and judges.
	two := map[int][]freqSample{
		0: {{ts: 1.0, khz: 1000000}, {ts: 2.0, khz: 1500000}},
		1: {{ts: 1.000001, khz: 1000000}, {ts: 2.000001, khz: 1500000}},
		4: {{ts: 1.0, khz: 2000000}, {ts: 2.0, khz: 2500000}},
	}
	capability = resolveCoreCapability(deriveClusterFreqDomains(two), two)
	if capability.source != CoreCapabilitySourceDefault || capability.comoveFloorTripped {
		t.Fatalf("a ≥2-sample merge passes the floor: %+v", capability.source)
	}
	if capability.topologySource != CoreCapabilityTopologyComovement {
		t.Fatalf("Tier-1 judgment must carry the co-movement topology token, got %q", capability.topologySource)
	}
	// Singleton domains make no co-movement claim — the floor never fires
	// (the existing CAP pins' single-sample-per-CPU shapes stay judged).
	singletons := map[int][]freqSample{0: {{ts: 1.0, khz: 1000000}}, 4: {{ts: 1.0, khz: 2000000}}}
	capability = resolveCoreCapability(deriveClusterFreqDomains(singletons), singletons)
	if capability.source != CoreCapabilitySourceDefault || capability.comoveFloorTripped {
		t.Fatalf("singleton domains must stay judged (floor is merge-only): %+v", capability.source)
	}
}

// --- 交叉验证 (两级并存) --------------------------------------------------------

func TestCAP2CrossValidationUnit(t *testing.T) {
	adoption := &clusterRailAdoption{family: "m3_c#_freq", clusters: []clusterRailCluster{{
		rail: "m3_c2_freq", anchor: 10, members: []int{10, 11},
		samples: []freqSample{{ts: 10.0, khz: 1200000}},
	}}}
	agree := map[int][]freqSample{10: {{ts: 10.005, khz: 1195000}}}
	if !clusterRailCrossValidate(adoption, agree) {
		t.Fatalf("a proximate sample within 10%% must agree")
	}
	contradict := map[int][]freqSample{10: {{ts: 10.005, khz: 900000}}}
	if clusterRailCrossValidate(adoption, contradict) {
		t.Fatalf("a proximate sample off by >10%% is a positive contradiction")
	}
	vacuous := map[int][]freqSample{10: {{ts: 10.5, khz: 900000}}}
	if !clusterRailCrossValidate(adoption, vacuous) {
		t.Fatalf("no proximate sample = vacuous pass (absence never convicts)")
	}
}

// 弃 Tier-2 形 (§28.5): rail samples landing within 10ms of contradicting
// member cpu_frequency samples discard the WHOLE rail evidence — Tier-1
// membership and wording stand.
func TestCAP2CrossValidationDiscardsTier2KeepsTier1(t *testing.T) {
	shifted := `
    tppmgr-idle-0-296   (    2) [000] .... 15151.826000: clock_set_rate: m3_c0_freq state=417000 cpu_id=0
    tppmgr-idle-0-296   (    2) [000] .... 15151.826001: clock_set_rate: m3_c1_freq state=417000 cpu_id=2
    tppmgr-idle-0-296   (    2) [000] .... 15151.826002: clock_set_rate: m3_c2_freq state=1200000 cpu_id=10
    tppmgr-idle-0-296   (    2) [000] .... 15151.826003: clock_set_rate: m3_c3_freq state=2350000 cpu_id=12
`
	idx := buildTraceIndex(t, "cap2_crossval.systrace", cap2SchedFiller()+cap2SweepLines+cap2LimitsLines+shifted)
	cache := newChainQueryCache(idx, nil)
	capability := cache.coreCapability("")
	if capability.railRejectReason != clusterRailRejectCrossValidation {
		t.Fatalf("the m3_c3 rail (2350000) contradicts cpu12's proximate 1200000 samples — Tier-2 must be discarded: %q", capability.railRejectReason)
	}
	if capability.topologySource != CoreCapabilityTopologyComovement {
		t.Fatalf("Tier-1 stands after the discard, got %q", capability.topologySource)
	}
	if len(capability.railByCluster) != 0 {
		t.Fatalf("a discarded rail must leave no timeline behind: %+v", capability.railByCluster)
	}
	// The honest merge stays: {10..13} is ONE cluster (3 clusters total).
	if len(capability.classByCluster) != 3 {
		t.Fatalf("Tier-1 keeps its 3 clusters: %+v", capability.classByCluster)
	}
}

// structure_conflict 形: a rail range straddling two MEASURED Tier-1 domains
// refutes the anchor presumption — Tier-2 is discarded whole, Tier-1 stands
// (测量胜于推定).
func TestCAP2RailStructureConflictDiscardsTier2(t *testing.T) {
	timelines := map[int][]freqSample{
		0: {{ts: 1.0, khz: 1000000}, {ts: 2.0, khz: 1200000}},
		1: {{ts: 1.5, khz: 2000000}, {ts: 2.5, khz: 2400000}},
	}
	domains := deriveClusterFreqDomains(timelines) // two measured singleton domains
	adoption := &clusterRailAdoption{family: "zz_c#_rail", clusters: []clusterRailCluster{
		{rail: "zz_c0_rail", anchor: 0, members: []int{0, 1, 2, 3}, samples: []freqSample{{ts: 1.0, khz: 1000000}}, fmaxKHz: 1000000},
		{rail: "zz_c1_rail", anchor: 4, members: []int{4, 5}, samples: []freqSample{{ts: 1.0, khz: 2000000}}, fmaxKHz: 2000000},
	}}
	refined := refineDomainsWithRails(domains, adoption)
	if refined.ok || refined.reason != clusterRailRejectStructureConflict {
		t.Fatalf("a range covering two measured domains must reject Tier-2 whole: %+v", refined)
	}
	capability := resolveCoreCapabilityEvidence(domains, timelines, nil, adoption)
	if capability.railRejectReason != clusterRailRejectStructureConflict {
		t.Fatalf("the capability lane must record the discard: %q", capability.railRejectReason)
	}
	if capability.topologySource != CoreCapabilityTopologyComovement || capability.source != CoreCapabilitySourceDefault {
		t.Fatalf("Tier-1 stands after the discard: %q/%q", capability.topologySource, capability.source)
	}
}

// --- 两级组合:Tier-2 细分 Tier-1 恒同值簇 (§28.5 witness form) -----------------

func TestCAP2RailSubdividesConstantEqualTier1Cluster(t *testing.T) {
	idx := buildTraceIndex(t, "cap2_subdivide.systrace", cap2SchedFiller()+cap2SweepLines+cap2LimitsLines+cap2M3RailLines)
	cache := newChainQueryCache(idx, nil)
	capability := cache.coreCapability("")
	if capability.source != CoreCapabilitySourceDefault {
		t.Fatalf("the specimen shape must judge, got %q (railReject=%q floor=%v)", capability.source, capability.railRejectReason, capability.comoveFloorTripped)
	}
	if capability.topologySource != CoreCapabilityTopologyKeyedRail {
		t.Fatalf("anchor subdivision used Tier-2 structure — the keyed_rail token must ride: %q", capability.topologySource)
	}
	// {10..13} (constant-equal in the fragment — Tier-1's honest merge) is
	// subdivided by the anchors 10/12 into {10,11}+{12,13}; the four classes
	// land exactly on the platform truth (§28.5-T5 相容验证).
	wantMembers := map[string][]int{"small": {0, 1}, "middle": {2, 3, 4, 5, 6, 7, 8, 9}, "big": {10, 11}, "prime": {12, 13}}
	for class, want := range wantMembers {
		if got := capability.classClusterMembers(class); fmt.Sprint(got) != fmt.Sprint(want) {
			t.Fatalf("%s cluster members = %v, want %v", class, got, want)
		}
	}
	// Ladder witnesses: limits order the sampled clusters (observed-only
	// ordering would misclass {2..9}=1744000 above {10,11}=1200000); the
	// rail rung carries {12,13}=2350000 where no limits row exists.
	wantCap := map[int]float64{0: 1.0, 5: 2.3, 10: 2.53, 13: 3.036}
	for cpu, want := range wantCap {
		if got := capability.capabilityFor(cpu); got != want {
			t.Fatalf("cpu%d coefficient = %v, want %v", cpu, got, want)
		}
	}
}

// 纯 Tier-2 (cpu_frequency 缺位, §28.5): the keyed rails + limits alone judge
// the structure; membership is wholly the anchor presumption.
func TestCAP2PureTier2Judgment(t *testing.T) {
	idx := buildTraceIndex(t, "cap2_pure_rail.systrace", cap2SchedFiller()+cap2LimitsLines+cap2M3RailLines)
	cache := newChainQueryCache(idx, nil)
	capability := cache.coreCapability("")
	if capability.source != CoreCapabilitySourceDefault || capability.topologySource != CoreCapabilityTopologyKeyedRail {
		t.Fatalf("pure Tier-2 must judge with the keyed_rail token: %q/%q", capability.source, capability.topologySource)
	}
	if capability.domains.source != ClusterFreqSourceKeyedRail {
		t.Fatalf("pure Tier-2 domains carry the keyed-rail source: %q", capability.domains.source)
	}
	if got := capability.classClusterMembers("prime"); fmt.Sprint(got) != fmt.Sprint([]int{12, 13}) {
		t.Fatalf("prime = %v, want [12 13]", got)
	}
	if cap, known := capability.capabilityForKnown(7); !known || cap != 2.3 {
		t.Fatalf("anchor-inferred member cpu7 must price middle (2.3): %v/%v", cap, known)
	}
	// 禁掷币 control: WITHOUT the limits rows the rail values alone tie
	// (m3_c0 == m3_c1 == 417000) — no defensible order, fail-loud freq_only.
	tieIdx := buildTraceIndex(t, "cap2_pure_rail_tie.systrace", cap2SchedFiller()+cap2M3RailLines)
	tie := newChainQueryCache(tieIdx, nil).coreCapability("")
	if tie.source != CoreCapabilitySourceFreqOnly {
		t.Fatalf("the rail-value tie must fail loud (no coin flip): %q", tie.source)
	}
}

// 优先级: an explicit core_topology outranks both tiers — rail evidence is
// not consulted and no topology token is minted (legacy wording preserved).
func TestCAP2ExplicitTopologyOutranksRail(t *testing.T) {
	idx := buildTraceIndex(t, "cap2_explicit.systrace", cap2SchedFiller()+cap2SweepLines+cap2LimitsLines+cap2M3RailLines)
	cache := newChainQueryCache(idx, nil)
	capability := cache.coreCapability("small=0-1;middle=2-9;big=10-13")
	if capability.domains.source != ClusterFreqSourceExplicit {
		t.Fatalf("explicit topology must win outright: %q", capability.domains.source)
	}
	if capability.topologySource != "" {
		t.Fatalf("no topology token on the explicit lane (byte-preserving absence): %q", capability.topologySource)
	}
	if len(capability.railByCluster) != 0 {
		t.Fatalf("rail timelines must not attach under explicit topology: %+v", capability.railByCluster)
	}
}

// --- 端到端: fold 消费 + 披露 (任务4) -------------------------------------------

// Pure Tier-2 e2e: the dependency runs on cpu5 (anchor-inferred middle) with
// no cpu_frequency anywhere — the rail timeline governs the slice, the big
// cluster's limits row anchors the fmax, and the typed disclosures ride the
// basis (fold_cluster_topology=keyed_rail + rail roster + family).
func TestCAP2PureTier2FoldEndToEnd(t *testing.T) {
	idx := buildTraceIndex(t, "cap2_e2e_rail.systrace", cap2SchedFiller()+cap2LimitsLines+cap2M3RailLines+cap2DepBody(5))
	chain := BuildWakeupChain(idx, cap2FoldQuery)
	dep := supplyFoldDepImpact(t, chain)
	basis := dep.SupplyFoldBasis
	if basis == nil || !basis.AllKnown() {
		t.Fatalf("rail governance books KNOWN basis: %+v", basis)
	}
	if basis.ClusterTopologySource != CoreCapabilityTopologyKeyedRail {
		t.Fatalf("the keyed_rail disclosure must ride the basis: %+v", basis)
	}
	if basis.RailFamily != "m3_c#_freq" || len(basis.RailGoverned) != 1 ||
		basis.RailGoverned[0].CPU != 5 || basis.RailGoverned[0].Rail != "m3_c1_freq" {
		t.Fatalf("the rail-governed roster must name the slice CPU and its rail: %+v", basis)
	}
	if basis.FmaxKHz != 2295000 || basis.FmaxSource != SupplyFoldFmaxSourceLimit {
		t.Fatalf("big cluster {10,11} anchors the basis at limits 2295000: %+v", basis)
	}
	// ~9.9ms @417000×2.3 vs 2295000×2.53: deficit ≈ 9.9×(1−0.1652) ≈ 8.26ms.
	if dep.SupplyFoldDeficitMs < 8.0 || dep.SupplyFoldDeficitMs > 8.5 {
		t.Fatalf("rail-governed middle-class deficit ≈8.26ms, got %.3f", dep.SupplyFoldDeficitMs)
	}
	if basis.CapabilitySource != CoreCapabilitySourceDefault {
		t.Fatalf("the default table priced the fold: %+v", basis)
	}
}

// Pure Tier-2 e2e, prime slice: the anchor-inferred prime member folds ABOVE
// the big-class reference and clamps — never a negative deficit; the rail
// fmax rung (§28.4 fmax=轨 trace 内最大) is disclosed when the reference
// cluster itself has no limits row.
func TestCAP2PureTier2PrimeSliceClampsAndRailRung(t *testing.T) {
	idx := buildTraceIndex(t, "cap2_e2e_prime.systrace", cap2SchedFiller()+cap2LimitsLines+cap2M3RailLines+cap2DepBody(12))
	chain := BuildWakeupChain(idx, cap2FoldQuery)
	dep := supplyFoldDepImpact(t, chain)
	basis := dep.SupplyFoldBasis
	if basis == nil || !basis.AllKnown() || dep.SupplyFoldDeficitMs != 0 {
		t.Fatalf("prime above the big reference clamps to zero deficit: %+v / %.3f", basis, dep.SupplyFoldDeficitMs)
	}
	if len(basis.RailGoverned) != 1 || basis.RailGoverned[0].Rail != "m3_c3_freq" {
		t.Fatalf("the prime slice folds on its own rail: %+v", basis)
	}
	// Rail rung witness (§28.4 fmax=轨 trace 内最大): a two-rail pure-Tier-2
	// trace with NO limits rows anywhere — the reference cluster {12,13} can
	// only anchor on its own rail timeline, so the fmax source says "rail".
	twoRails := `
    tppmgr-idle-0-296   (    2) [000] .... 15151.855462: clock_set_rate: m3_c2_freq state=1200000 cpu_id=10
    tppmgr-idle-0-296   (    2) [000] .... 15151.855463: clock_set_rate: m3_c3_freq state=2350000 cpu_id=12
`
	idx2 := buildTraceIndex(t, "cap2_e2e_rail_rung.systrace", cap2SchedFiller()+twoRails+cap2DepBody(12))
	chain2 := BuildWakeupChain(idx2, cap2FoldQuery)
	dep2 := supplyFoldDepImpact(t, chain2)
	if dep2.SupplyFoldBasis == nil || dep2.SupplyFoldBasis.FmaxSource != SupplyFoldFmaxSourceRail ||
		dep2.SupplyFoldBasis.FmaxKHz != 2350000 {
		t.Fatalf("a limits-less rail cluster anchors the basis on the rail rung (governed max 2350000): %+v", dep2.SupplyFoldBasis)
	}
	if dep2.SupplyFoldDeficitMs != 0 {
		t.Fatalf("the big-class slice at its own rail value folds to zero deficit: %.3f", dep2.SupplyFoldDeficitMs)
	}
}

// Tier-1 e2e disclosure: the co-movement judgment stamps freq_comovement on
// the fold basis and the gated lane (no rail in the trace).
func TestCAP2Tier1FoldDisclosureEndToEnd(t *testing.T) {
	idx := buildTraceIndex(t, "cap2_e2e_tier1.systrace", cap2SchedFiller()+cap2SweepLines+cap2LimitsLines+cap2DepBody(5))
	chain := BuildWakeupChain(idx, cap2FoldQuery)
	dep := supplyFoldDepImpact(t, chain)
	if dep.SupplyFoldBasis == nil || dep.SupplyFoldBasis.ClusterTopologySource != CoreCapabilityTopologyComovement {
		t.Fatalf("Tier-1 folds disclose freq_comovement: %+v", dep.SupplyFoldBasis)
	}
	if dep.SupplyFoldBasis.RailFamily != "" || len(dep.SupplyFoldBasis.RailGoverned) != 0 {
		t.Fatalf("no rail claims on a Tier-1 fold: %+v", dep.SupplyFoldBasis)
	}
}

// --- THERM (§28.5-T7, disclosure-only) -----------------------------------------

func TestCAP2ThermalCapDisclosure(t *testing.T) {
	t.Run("thermal rail presses the dominant cluster", func(t *testing.T) {
		body := cap2SchedFiller() + cap2SweepLines + cap2LimitsLines + cap2ThermalInte1Lines + cap2DepBodyLate(5)
		idx := buildTraceIndex(t, "cap2_therm_rail.systrace", body)
		chain := BuildWakeupChain(idx, cap2FoldQueryLate)
		dep := supplyFoldDepImpact(t, chain)
		basis := dep.SupplyFoldBasis
		if basis == nil {
			t.Fatalf("fold must run")
		}
		// thermal_inte1 anchors 4/3/3/5/2/3 all sit inside {2..9}; its
		// governed window minimum 1850000 < the cluster fmax 2200000
		// (limits) — the press is disclosed, numbers untouched.
		if basis.ThermalCapKHz != 1850000 || basis.ThermalCapClusterClass != "middle" {
			t.Fatalf("thermal press must disclose 1850000@middle: %+v", basis)
		}
		// CR-3 件⑥ F-10: the rail samples inside the governance window ARE
		// the in-window witness — the 受热限压 word is earned.
		if !basis.ThermalCapWitnessed {
			t.Fatalf("in-window thermal rail samples must mint the witness bit: %+v", basis)
		}
	})
	t.Run("dynamic limits press without any thermal rail", func(t *testing.T) {
		idx := buildTraceIndex(t, "cap2_therm_limits.systrace", cap2SchedFiller()+cap2SweepLines+cap2LimitsLines+cap2DepBody(0))
		chain := BuildWakeupChain(idx, cap2FoldQuery)
		dep := supplyFoldDepImpact(t, chain)
		// cpu0's limits dropped 1750000→1550000 (specimen dynamic witness):
		// the window-governing Max 1550000 < the {0,1} fmax 1750000.
		if dep.SupplyFoldBasis == nil || dep.SupplyFoldBasis.ThermalCapKHz != 1550000 {
			t.Fatalf("the dynamic limits press must disclose 1550000: %+v", dep.SupplyFoldBasis)
		}
		// CR-3 件⑥ F-10 (CR-2 冷读 D5 shape): every limits sample here sits
		// BEFORE the fold window — a carry-in governance value presses the
		// cap but earns NO in-window witness (the display words it 运行于
		// X(限压原因未见证), never 受热限压至 X).
		if dep.SupplyFoldBasis.ThermalCapWitnessed {
			t.Fatalf("a pre-window carry-in cap must stay unwitnessed: %+v", dep.SupplyFoldBasis)
		}
	})
	t.Run("修复轮 P4: in-window RELEASE sample earns no witness", func(t *testing.T) {
		// The carry-in press (1550000 < fmax 1750000) still governs; the
		// only IN-WINDOW limits sample restores fmax (a release, not a
		// press) — the thermal word stays unearned.
		release := "\n       hilogd.pst-647   (  629) [000] .... 15152.005000: cpu_frequency_limits: min=417000 max=1750000 cpu_id=0\n"
		idx := buildTraceIndex(t, "cap2_therm_release.systrace", cap2SchedFiller()+cap2SweepLines+cap2LimitsLines+release+cap2DepBody(0))
		chain := BuildWakeupChain(idx, cap2FoldQuery)
		dep := supplyFoldDepImpact(t, chain)
		if dep.SupplyFoldBasis == nil || dep.SupplyFoldBasis.ThermalCapKHz != 1550000 {
			t.Fatalf("the carry-in press must still disclose 1550000: %+v", dep.SupplyFoldBasis)
		}
		if dep.SupplyFoldBasis.ThermalCapWitnessed {
			t.Fatalf("an in-window release-only sample must not mint the witness: %+v", dep.SupplyFoldBasis)
		}
	})
	t.Run("no press, no sentence (双向)", func(t *testing.T) {
		idx := buildTraceIndex(t, "cap2_therm_none.systrace", cap2SchedFiller()+cap2SweepLines+cap2LimitsLines+cap2DepBody(10))
		chain := BuildWakeupChain(idx, cap2FoldQuery)
		dep := supplyFoldDepImpact(t, chain)
		// {10..13}: the single limits row 2295000 IS the fmax — no press.
		if dep.SupplyFoldBasis == nil || dep.SupplyFoldBasis.ThermalCapKHz != 0 {
			t.Fatalf("cap == fmax must emit nothing: %+v", dep.SupplyFoldBasis)
		}
	})
	t.Run("零权重机械 pin: 双 trace 数值恒等 (复核收尾 P2-1)", func(t *testing.T) {
		// THERM is a disclosure-only appendix: the SAME fold over the SAME
		// scheduling data, with and without the thermal rails present, must
		// produce BYTE-IDENTICAL fold numbers — deficit, ideal, basis split,
		// fmax value+source, reference class, topology token. The only
		// permitted delta is the ThermalCap* pair itself. A mutation pressing
		// ThermalCapKHz into the fold reference (复核 M3 form,
		// supplyFoldRunningIntervals bigFmax) changes the deficit on the
		// with-thermal side only and reds here.
		base := cap2SchedFiller() + cap2SweepLines + cap2LimitsLines + cap2DepBodyLate(5)
		withTherm := supplyFoldDepImpact(t, BuildWakeupChain(
			buildTraceIndex(t, "cap2_zero_weight_with.systrace", base+cap2ThermalInte1Lines), cap2FoldQueryLate))
		without := supplyFoldDepImpact(t, BuildWakeupChain(
			buildTraceIndex(t, "cap2_zero_weight_without.systrace", base), cap2FoldQueryLate))
		wb, ob := withTherm.SupplyFoldBasis, without.SupplyFoldBasis
		if wb == nil || ob == nil {
			t.Fatalf("both folds must run: %+v / %+v", wb, ob)
		}
		if wb.ThermalCapKHz != 1850000 || ob.ThermalCapKHz != 0 {
			t.Fatalf("the thermal delta must be exactly the ThermalCap pair: with=%d without=%d", wb.ThermalCapKHz, ob.ThermalCapKHz)
		}
		if withTherm.SupplyFoldDeficitMs != without.SupplyFoldDeficitMs ||
			withTherm.SupplyFoldIdealMs != without.SupplyFoldIdealMs {
			t.Fatalf("零权重: deficit/ideal must be byte-identical with vs without thermal rails: %.9f/%.9f vs %.9f/%.9f",
				withTherm.SupplyFoldDeficitMs, withTherm.SupplyFoldIdealMs, without.SupplyFoldDeficitMs, without.SupplyFoldIdealMs)
		}
		if wb.KnownMs != ob.KnownMs || wb.UnknownMs != ob.UnknownMs ||
			wb.FmaxKHz != ob.FmaxKHz || wb.FmaxSource != ob.FmaxSource ||
			wb.ReferenceClass != ob.ReferenceClass ||
			wb.CapabilitySource != ob.CapabilitySource ||
			wb.ClusterTopologySource != ob.ClusterTopologySource {
			t.Fatalf("零权重: every non-ThermalCap basis field must be identical:\nwith:    %+v\nwithout: %+v", wb, ob)
		}
	})
	t.Run("freq_only cluster attribution unavailable emits nothing", func(t *testing.T) {
		// No cpu_frequency, no rails: capability is freq_only — 归属不可得,
		// absence never guesses.
		idx := buildTraceIndex(t, "cap2_therm_freqonly.systrace", cap2SchedFiller()+cap2LimitsLines+cap2ThermalInte1Lines+cap2DepBody(5))
		chain := BuildWakeupChain(idx, cap2FoldQuery)
		dep := supplyFoldDepImpact(t, chain)
		if dep.SupplyFoldBasis == nil || dep.SupplyFoldBasis.ThermalCapKHz != 0 {
			t.Fatalf("no cluster attribution → no THERM claim: %+v", dep.SupplyFoldBasis)
		}
	})
}

// --- 任务6: pid_freq 量纲筛 (soft corroboration lane) ---------------------------

func TestCAP2ClockLanePlausibilityFilter(t *testing.T) {
	// Unit band checks (identity + Hz + ×1e6 + MHz hypotheses).
	if clockLanePlausibleCPUFrequency(10240923) {
		t.Fatalf("the specimen pid_freq 10240923 encoding is implausible under every unit hypothesis")
	}
	for _, keep := range []int{190091, 2200, 3000000, 96000000, 2200000000} {
		if !clockLanePlausibleCPUFrequency(keep) {
			t.Fatalf("%d is plausible under some hypothesis and must survive (pinned corroboration shapes)", keep)
		}
	}
	// Index-level: the noise sample never enters the corroboration lane; the
	// plausible pid_freq sample stays (the filter is surgical, not a ban).
	idx := buildTraceIndex(t, "cap2_pid_freq.systrace", cap2SchedFiller()+cap2PidFreqLines)
	cache := newChainQueryCache(idx, nil)
	cache.buildClockLaneIndex()
	saw190091 := false
	for _, sample := range cache.clockLaneSamples {
		if sample.khz == 10240923 {
			t.Fatalf("10240923 must be dropped at the source (噪音源头消除): %+v", cache.clockLaneSamples)
		}
		if sample.khz == 190091 {
			saw190091 = true
		}
	}
	if !saw190091 {
		t.Fatalf("the plausible 190091 sample must stay in the soft lane: %+v", cache.clockLaneSamples)
	}
}
