package tool

// answer_document_projection_ptv6_test.go — PTV6 批①+③ (real-trace campaign
// 2026-07-06, 标本归因 #1/#2/#9/#11) + PTV6-B tree.go 移交件 pins.
//
// 标本回放 (eval/results 无损面重建法, PTV4 批先例):
//   标本1 = trace_query_path_question_absolute_donghu_short-20260706-013848
//     (16 critical_blocking chain_relevance=background rows double-cast into
//      SupportingHops → five └─唤醒─ phantom children of the 🎯 target +
//      链/▒ double seats).
//   标本2 = trace_query_donghu_real_short_runnable-20260706-013200
//     (three CookieMonsterCl root_cause_primary rows → ❶ main row and a
//      same-value ❷ on its own 成因 child).
//
// MUTATIONS pinned here:
//   - 恢复 predicate-only hops 准入 → background subjects re-enter the main
//     tree (specimen replay assertions red);
//   - 恢复 depthless 硬编码唤醒边 → fences regain 唤醒─ (edge pins red);
//   - 去 badge 去重 → the same-subject cause row regains ❷ (badge pins red);
//   - 摘除 near 道带宽 → the 1.354/1.383 boundary-resampling pair stops
//     folding (near-lane pins red).

import (
	"fmt"
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/types"
)

// --- 标本1: donghu_short absolute (E3-E7 wake-edge misattachment) ---------------

func ptv6SpecimenCriticalBlocking(id, claimKey, subject, peer, value string, impact float64, lineStart, lineEnd int, blockType, nearest string) types.ObservationRecord {
	return types.ObservationRecord{
		ID: id, Origin: types.AnswerEvidenceOriginRuntimeArtifact, Producer: "trace_query",
		GroundingPolicy: types.ClaimGroundingHard, Predicate: "critical_blocking", ClaimKey: claimKey,
		Subject: subject, Object: peer, Value: value, Unit: "ms", Confidence: 0.8,
		Span: types.ObservationSpan{LineStart: lineStart, LineEnd: lineEnd},
		RichNotes: []string{
			fmt.Sprintf("impact_ms=%.3f", impact),
			"type=" + blockType,
			"chain_relevance=background",
			"nearest_chain_thread=" + nearest,
			"selected_window=34579.472865..34579.475857",
		},
	}
}

// ptv6Specimen1Records rebuilds the lossless 系统补充 surface of
// trace_query_path_question_absolute_donghu_short-20260706-013848: the wakeup
// path, the two CookieMonsterCl primaries, all 16 background critical_blocking
// rows and the two adjacent irq tertiary aggregates.
func ptv6Specimen1Records() []types.ObservationRecord {
	records := []types.ObservationRecord{
		{
			ID: "path", Origin: types.AnswerEvidenceOriginRuntimeArtifact, Producer: "trace_query",
			GroundingPolicy: types.ClaimGroundingHard, Predicate: "wakeup_chain", ClaimKey: "wakeup_chain:path",
			Object:    "CookieMonsterCl-59843 -> com.baidu.tieba-59566",
			RichNotes: []string{"window=34579.472865..34579.475857"},
		},
		projV3Obs("primary-1", "root_cause_primary", "root_cause_primary:CookieMonsterCl-59843",
			"CookieMonsterCl-59843", "runnable_wait", "1.661", 1.661, 2891, 3226,
			"rank=1", "tier=primary", "causality=on_wakeup_chain", "type=runnable_wait",
			"effective_impact_ms=1.661", "dominant_state=runnable",
			"selected_window=34579.472865..34579.475857"),
		projV3Obs("primary-2", "root_cause_primary", "root_cause_primary:CookieMonsterCl-59843:window_stats",
			"CookieMonsterCl-59843", "runnable_wait", "1.661", 1.661, 2916, 3175,
			"rank=2", "tier=primary", "causality=on_wakeup_chain", "type=runnable_wait",
			"effective_impact_ms=1.661", "dominant_state=runnable"),
		projV3Obs("adj-burst", "root_cause_tertiary", "root_cause_tertiary:irq_burst",
			"unknown-thread", "irq_burst", "1.997", 1.997, 2900, 3212,
			"rank=4", "tier=tertiary", "causality=adjacent_to_wakeup_chain", "type=irq_burst",
			"effective_impact_ms=1.997"),
		projV3Obs("adj-activity", "root_cause_tertiary", "root_cause_tertiary:irq_activity",
			"unknown-thread", "irq_activity", "1.047", 1.047, 2787, 3207,
			"rank=12", "tier=tertiary", "causality=adjacent_to_wakeup_chain", "type=irq_activity"),
	}
	cb := func(id, claimKey, subject, peer, value string, impact float64, ls, le int, blockType, nearest string) {
		records = append(records, ptv6SpecimenCriticalBlocking(id, claimKey, subject, peer, value, impact, ls, le, blockType, nearest))
	}
	cb("cb-01", "critical_blocking:blocking_span", "HwBinder:60194_-60201", "unknown-thread", "0.081", 0.081, 3035, 3050, "blocking_span", "CookieMonsterCl-59843")
	cb("cb-02", "critical_blocking:blocking_span", "OMXCallbackDisp-61436", "unknown-thread", "0.065", 0.065, 3022, 3028, "blocking_span", "CookieMonsterCl-59843")
	cb("cb-03", "critical_blocking:d_state_or_io_wait", "BdAsyncTask #8-59953", "unknown-thread", "0.174", 0.174, 3071, 3115, "d_state_or_io_wait", "com.baidu.tieba-59566")
	cb("cb-04", "critical_blocking:d_state_or_io_wait", "BdAsyncTask #8-59953", "unknown-thread", "2.213", 2.213, 2370, 3058, "d_state_or_io_wait", "com.baidu.tieba-59566")
	cb("cb-05", "critical_blocking:d_state_or_io_wait", "OS_FFRT_2_777-57436", "unknown-thread", "0.051", 0.051, 2887, 2892, "d_state_or_io_wait", "com.baidu.tieba-59566")
	cb("cb-06", "critical_blocking:d_state_or_io_wait", "T7@ZeusThreadPo-61839", "unknown-thread", "1.634", 1.634, 2945, 3264, "d_state_or_io_wait", "com.baidu.tieba-59566")
	cb("cb-07", "critical_blocking:d_state_or_io_wait", "sysevent_store-47924", "unknown-thread", "1.302", 1.302, 2918, 3100, "d_state_or_io_wait", "com.baidu.tieba-59566")
	cb("cb-08", "critical_blocking:io_latency", "BdAsyncTask #8-59953", "udk-irq-3-65", "0.188", 0.188, 3069, 3113, "io_latency", "CookieMonsterCl-59843")
	cb("cb-09", "critical_blocking:io_latency", "BdAsyncTask #8-59953", "udk-irq-3-65", "0.371", 0.371, 3128, 3194, "io_latency", "CookieMonsterCl-59843")
	cb("cb-10", "critical_blocking:io_latency", "hmfs_txn-260-67-453", "udk-irq-3-65", "2.326", 2.326, 2734, 3041, "io_latency", "CookieMonsterCl-59843")
	cb("cb-11", "critical_blocking:io_latency", "sysevent_store-47924", "udk-irq-1-63", "1.354", 1.354, 2908, 3094, "io_latency", "CookieMonsterCl-59843")
	cb("cb-12", "critical_blocking:io_latency", "sysevent_store-47924", "udk-irq-1-63", "1.382", 1.382, 2911, 3114, "io_latency", "CookieMonsterCl-59843")
	cb("cb-13", "critical_blocking:io_latency", "sysevent_store-47924", "udk-irq-1-63", "1.383", 1.383, 2913, 3120, "io_latency", "CookieMonsterCl-59843")
	cb("cb-14", "critical_blocking:io_latency", "wk:0/-20/0/6-23082", "udk-irq-4-67", "0.375", 0.375, 3118, 3182, "io_latency", "CookieMonsterCl-59843")
	cb("cb-15", "critical_blocking:io_latency", "wk:0/-20/0/6-23082", "udk-irq-4-67", "0.906", 0.906, 2966, 3078, "io_latency", "CookieMonsterCl-59843")
	cb("cb-16", "critical_blocking:blocked_reason", "OS_IPC_0_1207-1207", "unknown-thread", "0.000", 0, 3000, 3010, "blocked_reason", "CookieMonsterCl-59843")
	return records
}

var ptv6Specimen1BackgroundSubjects = []string{
	"sysevent_store-47924",
	"hmfs_txn-260-67-453",
	"BdAsyncTask #8-59953",
	"T7@ZeusThreadPo-61839",
	"wk:0/-20/0/6-23082",
}

func TestTraceProjectionPTV6Specimen1BackgroundRowsLeaveMainTree(t *testing.T) {
	projection := types.TraceCausalProjectionFromObservationRecords(ptv6Specimen1Records())
	// 编译面负向 pin: no background-classified record reaches SupportingHops.
	for _, hop := range projection.SupportingHops {
		if strings.TrimSpace(hop.ChainRelevance) == "background" {
			t.Fatalf("background rows must not double-cast into SupportingHops: %+v", hop)
		}
	}
	model := buildRuntimeTraceProjTreeModel(projection, newRuntimeTraceCausalProjectionEvidenceIndex(), true)
	// #9 fence 负向 pin (a): 禁 background 行进主树 — the on-chain tree keeps
	// only the CookieMonsterCl trunk row family.
	for _, row := range model.TreeRows {
		subject := strings.TrimSpace(row.Node.Subject)
		for _, banned := range ptv6Specimen1BackgroundSubjects {
			if subject == banned {
				t.Fatalf("background subject %q re-entered the main tree (predicate-only hops admission revived?): %+v", banned, row)
			}
		}
	}
	// #9: depth-1 of the main tree is exactly the ❶ CookieMonsterCl trunk row.
	depth1 := 0
	for _, row := range model.TreeRows {
		if row.Kind == runtimeTraceProjTreeRowChain && row.Depth == 1 {
			depth1++
			if !strings.Contains(row.Node.Subject, "CookieMonsterCl") {
				t.Fatalf("depth-1 must be the CookieMonsterCl trunk row, got %+v", row.Node)
			}
			if row.Badge != 1 {
				t.Fatalf("the trunk row must carry ❶: %+v", row)
			}
		}
	}
	if depth1 != 1 {
		t.Fatalf("main tree depth-1 must keep exactly one row, got %d", depth1)
	}
	// #11 (标本1 面): exactly one badge in this replay — no same-value ❶❷ pair.
	badges := 0
	for _, rows := range [][]runtimeTraceProjTreeRow{model.TreeRows, model.SelfRows, model.Adjacent, model.Background} {
		for _, row := range rows {
			if row.Badge > 0 {
				badges++
			}
		}
	}
	if badges != 1 {
		t.Fatalf("specimen replay must seat exactly one badge, got %d", badges)
	}
	// #2 ▒ 单席: the background stanza carries the specimen rows, and no node
	// key seats in BOTH the chain lane and a stanza.
	seatKeys := map[string]string{}
	for _, row := range model.TreeRows {
		seatKeys[runtimeTraceCausalProjectionNodeKey(row.Node)] = "tree"
	}
	for _, row := range model.Background {
		key := runtimeTraceCausalProjectionNodeKey(row.Node)
		if seatKeys[key] == "tree" {
			t.Fatalf("链/▒ double seat: %+v", row.Node)
		}
	}
	bgSubjects := map[string]bool{}
	for _, row := range model.Background {
		bgSubjects[strings.TrimSpace(row.Node.Subject)] = true
	}
	for _, want := range ptv6Specimen1BackgroundSubjects {
		if !bgSubjects[want] {
			t.Fatalf("background stanza must keep specimen row %q: %+v", want, bgSubjects)
		}
	}
	// #1b fence 负向 pin (b): 禁裸唤醒边 — this replay has a 1-node trunk
	// (下钻 only); ANY 唤醒─ edge is the retired hardcoded default coming back.
	fence := runtimeTraceProjTreeFence(model, true)
	if strings.Contains(fence, "唤醒─") {
		t.Fatalf("specimen replay must not draw a wake edge anywhere:\n%s", fence)
	}
	if !strings.Contains(fence, "下钻─") || !strings.Contains(fence, "CookieMonsterCl") {
		t.Fatalf("trunk drill row must survive:\n%s", fence)
	}
	// Background subjects render only after the ▒ stanza header.
	bgStart := strings.Index(fence, "▒ ")
	if bgStart < 0 {
		t.Fatalf("background stanza missing:\n%s", fence)
	}
	for _, subject := range []string{"hmfs_txn-260-67-453", "T7@ZeusThreadPo-61839", "wk:0/-20/0/6-23082"} {
		if idx := strings.Index(fence, subject); idx >= 0 && idx < bgStart {
			t.Fatalf("background subject %q rendered before the ▒ stanza:\n%s", subject, fence)
		}
	}
}

// --- 标本2: donghu_short short_runnable (❶❷ same-value double seat) -------------

func ptv6Specimen2Records() []types.ObservationRecord {
	records := []types.ObservationRecord{
		{
			ID: "path", Origin: types.AnswerEvidenceOriginRuntimeArtifact, Producer: "trace_query",
			GroundingPolicy: types.ClaimGroundingHard, Predicate: "wakeup_chain", ClaimKey: "wakeup_chain:path",
			Object:    "CookieMonsterCl-59843 -> com.baidu.tieba-59566",
			RichNotes: []string{"window=34579.472865..34579.475857"},
		},
		// The three CookieMonsterCl primaries of the 系统补充 surface: the
		// first two share value + line range (R1 same-fact merge → the main
		// row), the third is the window_stats runnable_wait fact that used to
		// take a same-value ❷ as the 成因 child.
		projV3Obs("primary-1", "root_cause_primary", "root_cause_primary:CookieMonsterCl-59843",
			"CookieMonsterCl-59843", "priority_inversion_candidate", "1.661", 1.661, 2892, 3227,
			"rank=1", "tier=primary", "causality=on_wakeup_chain", "type=priority_inversion_candidate",
			"effective_impact_ms=1.661", "dominant_state=runnable", "priority_inversion_candidate=true",
			"selected_window=34579.472865..34579.475857"),
		projV3Obs("primary-2", "root_cause_primary", "root_cause_primary:CookieMonsterCl-59843:inversion_wait",
			"CookieMonsterCl-59843", "priority_inversion_runnable_wait", "1.661", 1.661, 2892, 3227,
			"rank=2", "tier=primary", "causality=on_wakeup_chain", "type=priority_inversion_runnable_wait",
			"effective_impact_ms=1.661", "dominant_state=runnable"),
		projV3Obs("primary-3", "root_cause_primary", "root_cause_primary:CookieMonsterCl-59843:window_stats",
			"CookieMonsterCl-59843", "runnable_wait", "1.661", 1.661, 2917, 3176,
			"rank=3", "tier=primary", "causality=on_wakeup_chain", "type=runnable_wait",
			"effective_impact_ms=1.661", "dominant_state=runnable"),
	}
	records = append(records,
		ptv6SpecimenCriticalBlocking("cb-06", "critical_blocking:d_state_or_io_wait", "T7@ZeusThreadPo-61839", "unknown-thread", "1.634", 1.634, 2946, 3265, "d_state_or_io_wait", "com.baidu.tieba-59566"),
		ptv6SpecimenCriticalBlocking("cb-10", "critical_blocking:io_latency", "hmfs_txn-260-67-453", "udk-irq-3-65", "2.326", 2.326, 2735, 3042, "io_latency", "CookieMonsterCl-59843"),
		ptv6SpecimenCriticalBlocking("cb-13", "critical_blocking:io_latency", "sysevent_store-47924", "udk-irq-1-63", "1.383", 1.383, 2914, 3121, "io_latency", "CookieMonsterCl-59843"),
	)
	return records
}

func TestTraceProjectionPTV6Specimen2BadgesNeverPairSameSubjectSameValue(t *testing.T) {
	projection := types.TraceCausalProjectionFromObservationRecords(ptv6Specimen2Records())
	model := buildRuntimeTraceProjTreeModel(projection, newRuntimeTraceCausalProjectionEvidenceIndex(), true)
	var badged []runtimeTraceProjTreeRow
	var causeRows int
	for _, row := range model.TreeRows {
		if row.Badge > 0 {
			badged = append(badged, row)
		}
		if row.Kind == runtimeTraceProjTreeRowCause {
			causeRows++
			// #11 mutation pin (去 badge 去重必红): the same-subject same-value
			// 成因 child must never regain its ❷ seat.
			if row.Badge != 0 {
				t.Fatalf("same-subject cause row must not seat a badge: %+v", row)
			}
		}
	}
	if causeRows == 0 {
		t.Fatalf("specimen 2 replay must keep its 成因 child (fixture drift?): %+v", model.TreeRows)
	}
	if len(badged) != 1 {
		t.Fatalf("specimen 2 must seat exactly ❶ (was ❶❷ same-value), got %d badged rows", len(badged))
	}
	if badged[0].Kind != runtimeTraceProjTreeRowChain || badged[0].Badge != 1 ||
		!strings.Contains(badged[0].Node.Subject, "CookieMonsterCl") {
		t.Fatalf("❶ must sit on the CookieMonsterCl trunk row: %+v", badged[0])
	}
	// Fence face: ❶ present, ❷ gone.
	fence := runtimeTraceProjTreeFence(model, true)
	if !strings.Contains(fence, "❶") || strings.Contains(fence, "❷") {
		t.Fatalf("specimen 2 fence must render ❶ without a same-value ❷:\n%s", fence)
	}
	if strings.Contains(fence, "唤醒─") {
		t.Fatalf("specimen 2 replay must not draw a wake edge anywhere:\n%s", fence)
	}
}

// --- #11 model-level: one badge seat per canonical subject ----------------------

func TestPTV6TopBadgesOneSeatPerSubject(t *testing.T) {
	row := func(id, subject string, kind string, rank int, eff float64, hasData bool) runtimeTraceProjTreeRow {
		return runtimeTraceProjTreeRow{
			Node: types.TraceCausalProjectionNode{
				EvidenceID: id, Subject: subject, Object: "runnable_wait",
				StateKind: "runnable", ChainRelevance: "on_chain",
				Rank: rank, ImpactMS: eff, CumulativeImpactMS: eff, EffectiveImpactMS: eff,
			},
			Kind: kind, Depth: 1, HasData: hasData,
		}
	}
	model := runtimeTraceProjTreeModel{
		Target:   "app-1",
		WindowMS: 100,
		TreeRows: []runtimeTraceProjTreeRow{
			row("a", "worker-1", runtimeTraceProjTreeRowChain, 1, 30.0, true),
			row("b", "worker-1", runtimeTraceProjTreeRowCause, 2, 30.0, true), // same subject, same value
			row("c", "worker-2", runtimeTraceProjTreeRowChain, 3, 20.0, true),
			row("d", "worker-3", runtimeTraceProjTreeRowChain, 4, 10.0, true),
		},
	}
	runtimeTraceProjAssignTopBadges(&model)
	got := map[string]int{}
	for i, row := range model.TreeRows {
		got[fmt.Sprintf("%d:%s", i, row.Node.Subject)] = row.Badge
	}
	// MUTATION (去 badge 去重必红): without subject dedupe the same-value
	// worker-1 cause row takes ❷ and worker-3 loses ❸.
	want := map[string]int{"0:worker-1": 1, "1:worker-1": 0, "2:worker-2": 2, "3:worker-3": 3}
	for key, badge := range want {
		if got[key] != badge {
			t.Fatalf("badge for %s: got %d want %d (%v)", key, got[key], badge, got)
		}
	}
	// 父行无数据时成因行仍可入选: the data-less parent is no candidate, the
	// cause row seats the subject's badge.
	orphan := runtimeTraceProjTreeModel{
		Target:   "app-1",
		WindowMS: 100,
		TreeRows: []runtimeTraceProjTreeRow{
			row("a", "worker-1", runtimeTraceProjTreeRowChain, 1, 0, false),
			row("b", "worker-1", runtimeTraceProjTreeRowCause, 2, 30.0, true),
		},
	}
	orphan.TreeRows[0].Node.EffectiveImpactMS = 0
	runtimeTraceProjAssignTopBadges(&orphan)
	if orphan.TreeRows[0].Badge != 0 || orphan.TreeRows[1].Badge != 1 {
		t.Fatalf("with a data-less parent the cause row must seat the badge: %+v", orphan.TreeRows)
	}
}

// --- #1b/#1c: depthless dedicated edge word + relation cell sync ----------------

func TestPTV6DepthlessEdgeWordNeverDefaultsToWake(t *testing.T) {
	projection := types.TraceCausalProjection{
		WakeupPath:    []string{"worker-9", "app-100"},
		WindowStartTs: 100.0,
		WindowEndTs:   100.2,
		OnChainCauses: []types.TraceCausalProjectionNode{
			{Role: types.TraceCausalRoleRootCauseContext, EvidenceID: "trunk-1", Subject: "worker-9",
				Object: "running_burst", StateKind: "running", ChainRelevance: "on_chain",
				ImpactMS: 30, Confidence: 0.8},
			{Role: types.TraceCausalRoleCausalHop, EvidenceID: "detached-1", Subject: "detached-7",
				Object: "runnable_wait", StateKind: "runnable", ChainRelevance: "on_chain",
				ImpactMS: 12, Confidence: 0.8},
		},
	}
	model := buildRuntimeTraceProjTreeModel(projection, newRuntimeTraceCausalProjectionEvidenceIndex(), true)
	var depthless *runtimeTraceProjTreeRow
	for i := range model.TreeRows {
		if model.TreeRows[i].Kind == runtimeTraceProjTreeRowDepthless {
			depthless = &model.TreeRows[i]
		}
	}
	if depthless == nil {
		t.Fatalf("fixture must produce a depthless remaining-on-chain row: %+v", model.TreeRows)
	}
	// MUTATION (恢复硬编码唤醒边必红): the depthless lane's edge is the typed
	// dedicated edge, never the wake default.
	if depthless.Edge != runtimeTraceProjTreeEdgeChainUnresolved {
		t.Fatalf("depthless row must carry the chain-unresolved edge, got %q", depthless.Edge)
	}
	fence := runtimeTraceProjTreeFence(model, true)
	if !strings.Contains(fence, "链上·深度未解析─") {
		t.Fatalf("fence must draw the dedicated depthless edge word:\n%s", fence)
	}
	if strings.Contains(fence, "唤醒─") {
		t.Fatalf("no wake edge may render for a depthless row:\n%s", fence)
	}
	// #1c: the relation cell follows the edge on both faces.
	if got := runtimeTraceProjDetailRelationCell(*depthless, true, false); got != "链上·深度未解析" {
		t.Fatalf("zh relation cell must sync with the edge: %q", got)
	}
	if got := runtimeTraceProjDetailRelationCell(*depthless, false, false); got != "on-chain · depth unresolved" {
		t.Fatalf("en relation cell must sync with the edge: %q", got)
	}
	// EN edge word face.
	enFence := runtimeTraceProjTreeFence(buildRuntimeTraceProjTreeModel(projection, newRuntimeTraceCausalProjectionEvidenceIndex(), false), false)
	if !strings.Contains(enFence, "on-chain·depth-unresolved─") || strings.Contains(enFence, "wakes─") {
		t.Fatalf("EN fence must use the dedicated edge word:\n%s", enFence)
	}
}

// --- #2: 链/▒·◇ double-seat defense is independent of the #1a root fix ----------

func TestPTV6StanzaSeatsSkipChainConsumedNodeKeys(t *testing.T) {
	chained := types.TraceCausalProjectionNode{
		Role: types.TraceCausalRoleRootCauseContext, EvidenceID: "dup-1", Subject: "worker-9",
		Object: "running_burst", StateKind: "running", ChainRelevance: "on_chain",
		ImpactMS: 30, Confidence: 0.8,
	}
	bgCopy := chained
	bgCopy.ChainRelevance = "background"
	adjCopy := chained
	adjCopy.ChainRelevance = "adjacent"
	projection := types.TraceCausalProjection{
		WakeupPath:       []string{"worker-9", "app-100"},
		WindowStartTs:    100.0,
		WindowEndTs:      100.2,
		OnChainCauses:    []types.TraceCausalProjectionNode{chained},
		BackgroundCauses: []types.TraceCausalProjectionNode{bgCopy},
		AdjacentCauses:   []types.TraceCausalProjectionNode{adjCopy},
	}
	model := buildRuntimeTraceProjTreeModel(projection, newRuntimeTraceCausalProjectionEvidenceIndex(), true)
	treeSeats := 0
	for _, row := range model.TreeRows {
		if runtimeTraceCausalProjectionNodeKey(row.Node) == "dup-1" {
			treeSeats++
		}
	}
	if treeSeats != 1 {
		t.Fatalf("the chain lane must seat the node exactly once: %d", treeSeats)
	}
	if len(model.Background) != 0 || len(model.Adjacent) != 0 {
		t.Fatalf("stanzas must skip chain-consumed node keys (双席防御): bg=%+v adj=%+v",
			model.Background, model.Adjacent)
	}
}

// --- PTV6-B 移交件: legend wording verbatim pins ---------------------------------

func TestPTV6LegendWordingVerbatimPins(t *testing.T) {
	wants := map[runtimeTraceProjMark][2]string{
		// 修正轮 Med (如实化): near-duplicates (≤3%) fold; clearly different
		// overlapping measurements legitimately accumulate (B 批 S3) — the
		// entry no longer over-claims that every same-thread overlap folds.
		runtimeTraceProjMarkOverWindowShare: {
			"- 占窗>100% = 跨CPU/多段累计投影,可合法超出窗口长度(时长条已封顶);同一线程的近似重复测量(≤3%)已按重复发布折叠;差异明显的重叠测量按多段累计。",
			"- A >100% window share = a multi-CPU / multi-span cumulative projection that may legitimately exceed the window (the bar is capped); same-thread near-duplicate measurements (≤3%) are folded as duplicate publications; clearly different overlapping measurements accumulate as multiple spans.",
		},
		// 修正轮 Low: no single-step percentage (transitive folds may drift
		// beyond it); the caliber is "the largest published copy in the fold".
		runtimeTraceProjMarkMergedDedup: {
			"- `×N同值` = 同一测量被重复发布 N 次(边界重取样时数值可有漂移,显示取合并中的最大一次发布),数值就是那一次测量,不是 N 份。",
			"- `×N same-value` = one measurement published N times (values may drift under boundary resampling; the display keeps the largest published copy in the fold); the value IS that single measurement, never N shares.",
		},
		// PTV6-C ruling C (#73): the legend's pointer target moved from the
		// intermediate trace_query record to the report's own evidence index
		// (trace line coordinates).
		runtimeTraceProjMarkEdgeChainUnresolved: {
			"- `└─链上·深度未解析─` = 链上项但树位深度未解析:不声称唤醒/下钻关系,不编造树位;该行 [E#] 经证据索引给出 trace 行号区间。",
			"- `└─on-chain·depth-unresolved─` = an on-chain row whose tree depth is unresolved: no wake/drill relation is claimed and no position is invented; the row's [E#] resolves to trace line spans via the evidence index.",
		},
	}
	seen := map[runtimeTraceProjMark]bool{}
	for _, entry := range runtimeTraceProjLegendCatalog() {
		want, ok := wants[entry.Mark]
		if !ok {
			continue
		}
		seen[entry.Mark] = true
		if entry.ZH != want[0] {
			t.Fatalf("mark %d zh legend wording drifted:\n got %q\nwant %q", entry.Mark, entry.ZH, want[0])
		}
		if entry.EN != want[1] {
			t.Fatalf("mark %d en legend wording drifted:\n got %q\nwant %q", entry.Mark, entry.EN, want[1])
		}
	}
	for mark := range wants {
		if !seen[mark] {
			t.Fatalf("mark %d missing from the legend catalog", mark)
		}
	}
}

// --- PTV6-B 移交件: display safety-net near lane mirrors the types authority -----

func TestPTV6AdjacentNearDuplicateFoldMirrorsTypesLane(t *testing.T) {
	node := func(id string, impact float64, object string, ls, le int) types.TraceCausalProjectionNode {
		return types.TraceCausalProjectionNode{
			EvidenceID: id, Subject: "sysevent_store-47924", Object: object,
			TypeToken: "io_latency", ImpactMS: impact, CumulativeImpactMS: impact,
			LineStart: ls, LineEnd: le, Confidence: 0.8,
		}
	}
	// 标本对 (donghu_short, PTV6-B): 1.354 vs 1.383 = 2.1% boundary-resampling
	// drift over overlapping ranges → ONE measurement, folded, display=MAX.
	// MUTATION (带宽摘除必红): an exact-only revert stops this fold.
	folded := runtimeTraceProjAdjacentNodesForDisplay([]types.TraceCausalProjectionNode{
		node("n1", 1.354, "udk-irq-1-63", 2908, 3094),
		node("n2", 1.383, "udk-irq-1-63", 2913, 3120),
	})
	if len(folded) != 1 {
		t.Fatalf("near-duplicate pair must fold to one row: %+v", folded)
	}
	if folded[0].DuplicatePublications != 2 {
		t.Fatalf("fold must disclose the publication count: %+v", folded[0])
	}
	if folded[0].ImpactMS != 1.383 || folded[0].CumulativeImpactMS != 1.383 {
		t.Fatalf("near fold must keep the member MAX (显示取最大): %+v", folded[0])
	}
	// Sentinel-object gate (types authority TraceCausalProjectionKnownSubject):
	// an unresolved peer carries no identity — never a near fold.
	sentinel := runtimeTraceProjAdjacentNodesForDisplay([]types.TraceCausalProjectionNode{
		node("s1", 112.223, "unknown-thread", 100, 200),
		node("s2", 112.214, "unknown-thread", 120, 220),
	})
	if len(sentinel) != 2 {
		t.Fatalf("sentinel-object near values must stay separate rows: %+v", sentinel)
	}
	// Outside the ≤3% band: distinct facts, never folded.
	wide := runtimeTraceProjAdjacentNodesForDisplay([]types.TraceCausalProjectionNode{
		node("w1", 1.000, "udk-irq-1-63", 100, 200),
		node("w2", 1.100, "udk-irq-1-63", 120, 220),
	})
	if len(wide) != 2 {
		t.Fatalf(">3%% drift must stay separate rows: %+v", wide)
	}
	// No overlap: two real bursts, never folded (RF2a unchanged on both lanes).
	apart := runtimeTraceProjAdjacentNodesForDisplay([]types.TraceCausalProjectionNode{
		node("a1", 1.354, "udk-irq-1-63", 100, 200),
		node("a2", 1.383, "udk-irq-1-63", 300, 400),
	})
	if len(apart) != 2 {
		t.Fatalf("non-overlapping rows must stay separate: %+v", apart)
	}
	// Exact lane byte-identical: equal values fold with the value untouched.
	exact := runtimeTraceProjAdjacentNodesForDisplay([]types.TraceCausalProjectionNode{
		node("e1", 1.354, "udk-irq-1-63", 100, 200),
		node("e2", 1.354, "udk-irq-1-63", 120, 220),
	})
	if len(exact) != 1 || exact[0].ImpactMS != 1.354 || exact[0].DuplicatePublications != 2 {
		t.Fatalf("exact lane must stay byte-identical: %+v", exact)
	}
	// [Med 修正轮] sentinel-SUBJECT gate: an unknown-thread subject pair with
	// near values over overlapping ranges carries no identity either — never a
	// near fold (types-layer gate mirrored through the same export).
	subjNode := func(id, subject string, impact float64, ls, le int) types.TraceCausalProjectionNode {
		n := node(id, impact, "udk-irq-1-63", ls, le)
		n.Subject = subject
		return n
	}
	subjSentinel := runtimeTraceProjAdjacentNodesForDisplay([]types.TraceCausalProjectionNode{
		subjNode("u1", "unknown-thread", 1.354, 2908, 3094),
		subjNode("u2", "unknown-thread", 1.383, 2913, 3120),
	})
	if len(subjSentinel) != 2 {
		t.Fatalf("sentinel-subject near values must stay separate rows: %+v", subjSentinel)
	}
}

// --- 修正轮 Med: off-chain-relevance primaries never take the depthless lane ----

// TestPTV6OffChainPrimaryNeverTakesDepthlessChainLane pins the [Med 修正轮
// 2026-07-06] finding: a background/adjacent-relevance PRIMARY (the rank lane
// has no #1a admission gate) must not ride the chain-universe depthless lane —
// that lane's 链上·深度未解析 edge claims chain identity for a typed off-chain
// row while the #2 defense suppressed its honest stanza seat. It renders on
// its ◇/▒ seat instead; the stray pass guarantees the seat even when the
// stanza bucket copy is missing (bucket-cap shape).
func TestPTV6OffChainPrimaryNeverTakesDepthlessChainLane(t *testing.T) {
	trunk := types.TraceCausalProjectionNode{
		Role: types.TraceCausalRoleRootCauseContext, EvidenceID: "trunk-1", Subject: "worker-9",
		Object: "running_burst", StateKind: "running", ChainRelevance: "on_chain",
		ImpactMS: 30, Confidence: 0.8,
	}
	bgPrimary := types.TraceCausalProjectionNode{
		Role: types.TraceCausalRolePrimaryRootCause, EvidenceID: "bg-primary-1", Subject: "pressure-11",
		Object: "io_pressure", StateKind: "d_state", ChainRelevance: "background",
		Rank: 1, Tier: "primary", ImpactMS: 25, CumulativeImpactMS: 25, Confidence: 0.8,
	}
	adjPrimary := types.TraceCausalProjectionNode{
		Role: types.TraceCausalRolePrimaryRootCause, EvidenceID: "adj-primary-2", Subject: "neighbor-12",
		Object: "runnable_wait", StateKind: "runnable", ChainRelevance: "adjacent",
		Rank: 2, Tier: "primary", ImpactMS: 15, CumulativeImpactMS: 15, Confidence: 0.8,
	}
	assertSeats := func(t *testing.T, model runtimeTraceProjTreeModel) {
		t.Helper()
		for _, row := range model.TreeRows {
			key := runtimeTraceCausalProjectionNodeKey(row.Node)
			if key == "bg-primary-1" || key == "adj-primary-2" {
				t.Fatalf("off-chain primary must not enter the chain tree: %+v", row)
			}
			if row.Kind == runtimeTraceProjTreeRowDepthless {
				t.Fatalf("no depthless row may exist in this shape: %+v", row)
			}
		}
		fence := runtimeTraceProjTreeFence(model, true)
		if strings.Contains(fence, "链上·深度未解析─") || strings.Contains(fence, "唤醒─") {
			t.Fatalf("off-chain primaries must not receive a chain edge word:\n%s", fence)
		}
		bgSeats, adjSeats := 0, 0
		for _, row := range model.Background {
			if runtimeTraceCausalProjectionNodeKey(row.Node) == "bg-primary-1" {
				bgSeats++
			}
		}
		for _, row := range model.Adjacent {
			if runtimeTraceCausalProjectionNodeKey(row.Node) == "adj-primary-2" {
				adjSeats++
			}
		}
		if bgSeats != 1 || adjSeats != 1 {
			t.Fatalf("off-chain primaries must keep exactly one honest stanza seat each: ▒=%d ◇=%d", bgSeats, adjSeats)
		}
	}
	// Arm A — the stanza buckets carry the classified copies (the compile
	// shape): the bucket copy renders, no double seat.
	withBuckets := types.TraceCausalProjection{
		WakeupPath:        []string{"worker-9", "app-100"},
		WindowStartTs:     100.0,
		WindowEndTs:       100.2,
		OnChainCauses:     []types.TraceCausalProjectionNode{trunk},
		PrimaryRootCauses: []types.TraceCausalProjectionNode{bgPrimary, adjPrimary},
		BackgroundCauses:  []types.TraceCausalProjectionNode{bgPrimary},
		AdjacentCauses:    []types.TraceCausalProjectionNode{adjPrimary},
	}
	assertSeats(t, buildRuntimeTraceProjTreeModel(withBuckets, newRuntimeTraceCausalProjectionEvidenceIndex(), true))
	// Arm B — the bucket copies are MISSING (cap-dropped shape): the stray
	// pass still seats them (PTS 永不静默丢), routed by their own relevance.
	withoutBuckets := types.TraceCausalProjection{
		WakeupPath:        []string{"worker-9", "app-100"},
		WindowStartTs:     100.0,
		WindowEndTs:       100.2,
		OnChainCauses:     []types.TraceCausalProjectionNode{trunk},
		PrimaryRootCauses: []types.TraceCausalProjectionNode{bgPrimary, adjPrimary},
	}
	model := buildRuntimeTraceProjTreeModel(withoutBuckets, newRuntimeTraceCausalProjectionEvidenceIndex(), true)
	assertSeats(t, model)
	for _, row := range model.Background {
		if runtimeTraceCausalProjectionNodeKey(row.Node) == "bg-primary-1" &&
			row.Node.Role == types.TraceCausalRolePrimaryRootCause {
			t.Fatalf("stray seat must normalize the primary role like the demote lane: %+v", row.Node)
		}
	}
}

// --- 修正轮 Low: KnownSubject single authority ----------------------------------

// TestPTV6KnownSubjectSingleAuthority pins the twin convergence: the tool-side
// helper is a pure delegate of the exported types gate — both surfaces answer
// identically on every input class.
func TestPTV6KnownSubjectSingleAuthority(t *testing.T) {
	cases := []struct {
		in   string
		want bool
	}{
		{"", false},
		{"   ", false},
		{"unknown-thread", false},
		{"Unknown-Thread", false},
		{"unknown", false},
		{" unknown ", false},
		{"worker-1", true},
		{" sysevent_store-47924 ", true},
	}
	for _, tc := range cases {
		if got := runtimeTraceCausalProjectionKnownSubject(tc.in); got != tc.want {
			t.Fatalf("tool KnownSubject(%q) = %v, want %v", tc.in, got, tc.want)
		}
		if got := types.TraceCausalProjectionKnownSubject(tc.in); got != tc.want {
			t.Fatalf("types KnownSubject(%q) = %v, want %v", tc.in, got, tc.want)
		}
	}
}
