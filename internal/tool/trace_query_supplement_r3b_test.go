package tool

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/tracebundle"
	"github.com/hanchaoqun/codrax/internal/tracequery"
	"github.com/hanchaoqun/codrax/internal/types"
)

// R3B-DEEP (§13.8, 2026-07-25) — the customer-shape END-TO-END pin: label-form
// user target (comm=unknown, name hits another tid), an unrelated in-window
// wakeup_new conflict, ledger pressure (600 padded observations with the
// supplement appended LAST), supplement rank+critical on the user window.
// On HEAD every leg holds: rank mints causal rows (incl. Rank=0 self rows),
// the supplement observations survive ledger compile, the projection carries
// the user window, and the coverage face knows it. The fifth replay's
// zero-row/all-unknown outcome therefore does NOT reproduce from this shape
// alone — the pin guards the proven legs while the residual divergence
// (composite tracebundle index lane / in-view cancellation partials) stays
// under investigation.
func TestR3BSupplementCustomerShapeReachesLedgerAndProjection(t *testing.T) {
	dir := t.TempDir()
	trace := strings.Join([]string{
		`unknown-32788 (32788) [004] .... 2.990000: sched_switch: prev_comm=idle/4 prev_pid=0 prev_prio=120 prev_state=R ==> next_comm=unknown next_pid=32788 next_prio=53`,
		`unknown-32788 (32788) [004] .... 3.001000: sched_switch: prev_comm=unknown prev_pid=32788 prev_prio=53 prev_state=D ==> next_comm=idle/4 next_pid=0 next_prio=120`,
		`unknown-32788 (32788) [004] .... 3.001200: sched_blocked_reason: pid=32788 iowait=0 caller=timerfd_read+0x70/0x25c`,
		`old-50173 (50173) [001] .... 3.005000: sched_switch: prev_comm=idle/1 prev_pid=0 prev_prio=120 prev_state=R ==> next_comm=old next_pid=50173 next_prio=20`,
		`ss.hm.ugc.aweme-33410 (33410) [005] .... 3.010000: sched_switch: prev_comm=idle/5 prev_pid=0 prev_prio=120 prev_state=R ==> next_comm=ss.hm.ugc.aweme next_pid=33410 next_prio=40`,
		`ss.hm.ugc.aweme-33410 (33410) [005] .... 3.012000: sched_switch: prev_comm=ss.hm.ugc.aweme prev_pid=33410 prev_prio=40 prev_state=S ==> next_comm=idle/5 next_pid=0 next_prio=120`,
		`old-50173 (50173) [001] .... 3.014000: sched_switch: prev_comm=old prev_pid=50173 prev_prio=20 prev_state=S ==> next_comm=idle/1 next_pid=0 next_prio=120`,
		`creator-7 (   7) [001] .... 3.075000: sched_wakeup_new: comm=new pid=50173 prio=20 target_cpu=001`,
		`sysmgr-99 (  99) [004] .... 3.150000: sched_wakeup: comm=unknown pid=32788 prio=53 target_cpu=004`,
		`unknown-32788 (32788) [004] .... 3.150500: sched_switch: prev_comm=idle/4 prev_pid=0 prev_prio=120 prev_state=R ==> next_comm=unknown next_pid=32788 next_prio=53`,
		`unknown-32788 (32788) [004] .... 3.151000: sched_switch: prev_comm=unknown prev_pid=32788 prev_prio=53 prev_state=S ==> next_comm=idle/4 next_pid=0 next_prio=120`,
	}, "\n")
	if err := os.WriteFile(filepath.Join(dir, types.AttachedTraceBlobBasename), []byte(trace), 0o644); err != nil {
		t.Fatal(err)
	}
	ctx := &types.BusContext{RepoRoot: dir, WorkDir: dir, Mutable: types.NewMutableState("分析卡顿")}
	ctx.AnalysisIR = &types.AnalysisIR{RequestModel: types.RequestModel{
		RuntimeTargets: []types.RuntimeTarget{{
			Kind: types.RuntimeTargetKindThread, Thread: "ss.hm.ugc.aweme [32788]",
			Source: "user_explicit", Confidence: 1,
		}},
	}}
	res, err := (&TraceQuery{}).Execute(ctx, json.RawMessage(`{"view":"event_search","pattern":"aweme","time_start":3.0,"time_end":3.2}`))
	if err != nil || !res.Success {
		t.Fatalf("model call failed: %+v %v", res, err)
	}
	ctx.ToolResults = append(ctx.ToolResults, res)
	for pad := 0; pad < 15; pad++ {
		junk := types.ToolResult{ToolName: "trace_query", Success: true}
		for j := 0; j < 40; j++ {
			junk.Observations = append(junk.Observations, types.ObservationRecord{
				ID:              fmt.Sprintf("trace_query:pad%d#io:%d", pad, j),
				Origin:          types.AnswerEvidenceOriginRuntimeArtifact,
				Producer:        "trace_query",
				GroundingPolicy: types.ClaimGroundingHard,
				Predicate:       "io_activity",
				ClaimKey:        fmt.Sprintf("io_activity:pad%d-%d", pad, j),
				Subject:         fmt.Sprintf("padthread-%d-%d", pad, j),
				Object:          "io",
				Value:           "1.000",
				Unit:            "ms",
				RichNotes:       []string{"selected_window=3.000000..3.200000"},
				Confidence:      0.5,
			})
		}
		ctx.ToolResults = append(ctx.ToolResults, junk)
	}
	out := RunTraceQuerySystemSupplement(ctx)
	if meta := ctx.Mutable.SystemTraceSupplementMeta(); meta != nil {
		// AUD-07 (§14.8): the flagship e2e also pins the per-view count and
		// family-census pairing on the customer shape — the rank view must
		// account a nonzero ROOT-CAUSE family (the §13.9 discriminator).
		if len(meta.ViewValueObservations) != len(meta.Views) || len(meta.ViewObservationFamilies) != len(meta.Views) {
			t.Fatalf("per-view counts/families must pair with the views: %+v", meta)
		}
		rootRows := 0
		for _, f := range meta.ViewObservationFamilies {
			rootRows += f.RootCauseRows
		}
		if rootRows == 0 {
			t.Fatalf("the customer-shape supplement must account root-cause family rows: %+v", meta.ViewObservationFamilies)
		}
	} else {
		t.Fatal("supplement meta missing")
	}
	if len(out.Executed) != 2 || out.Executed[0] != "root_cause_rank" {
		t.Fatalf("supplement must run rank+critical on the user window: %+v", out)
	}
	supplementObs := 0
	for _, r := range ctx.Mutable.SystemTraceSupplementResults() {
		supplementObs += len(r.Observations)
	}
	if supplementObs == 0 {
		t.Fatal("supplement results lost their observations")
	}
	input := types.ObservationLedgerInputFromBusContext(ctx, types.ObservationExtractLedgerEvidenceLimit)
	ledger := types.CompileObservationLedger(input)
	rootRows, stateRows := 0, 0
	for _, rec := range ledger.Records {
		if strings.HasPrefix(rec.Predicate, "root_cause") {
			rootRows++
		}
		if rec.Predicate == "target_window_states" {
			stateRows++
		}
	}
	if rootRows == 0 || stateRows == 0 {
		t.Fatalf("supplement causal/state rows must survive ledger pressure: root=%d state=%d records=%d", rootRows, stateRows, len(ledger.Records))
	}
	set := types.CompileTraceCausalProjectionSet(ledger)
	if len(set.Projections) != 1 || set.Projections[0].WindowStartTs != 3.0 || set.Projections[0].WindowEndTs != 3.2 {
		t.Fatalf("projection must carry the user window: %+v", set.Projections)
	}
	authority := runtimeTraceCoverageAuthority(input)
	if !authority.analysisWindowKnown || len(authority.targetStates) != 1 {
		t.Fatalf("coverage face must know the window and the state account: known=%v states=%d", authority.analysisWindowKnown, len(authority.targetStates))
	}
}

// TestR3BSupplementCensusLiteKeepsPerViewCounts — AUD-01 (§14.2, 同事审计
// 2026-07-25): the census-lite adjunct appends to executed but NEVER to
// valueObservations, so the censusLiteRan trim must remove ONLY the Views
// tail — trimming valueObservations too silently drops the LAST windowed
// view's count and resurrects the uncounted 确定性补跑 face C2 exists to
// kill. Production path: windowed views run + vsync-family keyword arms the
// lite adjunct on a trace with no generator census.
func TestR3BSupplementCensusLiteKeepsPerViewCounts(t *testing.T) {
	dir := t.TempDir()
	trace := strings.Join([]string{
		`unknown-32788 (32788) [004] .... 2.990000: sched_switch: prev_comm=idle/4 prev_pid=0 prev_prio=120 prev_state=R ==> next_comm=unknown next_pid=32788 next_prio=53`,
		`unknown-32788 (32788) [004] .... 3.001000: sched_switch: prev_comm=unknown prev_pid=32788 prev_prio=53 prev_state=D ==> next_comm=idle/4 next_pid=0 next_prio=120`,
		`unknown-32788 (32788) [004] .... 3.001200: sched_blocked_reason: pid=32788 iowait=0 caller=timerfd_read+0x70/0x25c`,
		`sysmgr-99 (  99) [004] .... 3.150000: sched_wakeup: comm=unknown pid=32788 prio=53 target_cpu=004`,
		`unknown-32788 (32788) [004] .... 3.150500: sched_switch: prev_comm=idle/4 prev_pid=0 prev_prio=120 prev_state=R ==> next_comm=unknown next_pid=32788 next_prio=53`,
		`unknown-32788 (32788) [004] .... 3.151000: sched_switch: prev_comm=unknown prev_pid=32788 prev_prio=53 prev_state=S ==> next_comm=idle/4 next_pid=0 next_prio=120`,
		`VSyncGenerator-610 ( 610) [002] .... 3.160000: print: C|610|VSYNC-app|1`,
	}, "\n")
	if err := os.WriteFile(filepath.Join(dir, types.AttachedTraceBlobBasename), []byte(trace), 0o644); err != nil {
		t.Fatal(err)
	}
	ctx := &types.BusContext{RepoRoot: dir, WorkDir: dir, Mutable: types.NewMutableState("分析丢帧")}
	ctx.AnalysisIR = &types.AnalysisIR{RequestModel: types.RequestModel{
		AnalyzerHints: types.AnalyzerHints{Keywords: []string{"丢帧"}},
		RuntimeTargets: []types.RuntimeTarget{{
			Kind: types.RuntimeTargetKindThread, Thread: "unknown [32788]",
			Source: "user_explicit", Confidence: 1,
		}},
	}}
	res, err := (&TraceQuery{}).Execute(ctx, json.RawMessage(`{"view":"event_search","pattern":"unknown","time_start":3.0,"time_end":3.2}`))
	if err != nil || !res.Success {
		t.Fatalf("model call failed: %+v %v", res, err)
	}
	ctx.ToolResults = append(ctx.ToolResults, res)
	// Pre-seed every generic family EXCEPT critical (typed predicates the
	// family detector reads) + frame evidence present, so the supplement runs
	// exactly ONE windowed view (critical_blocking_calls, which carries no
	// vsync census) and the lite adjunct fires after it.
	seed := types.ToolResult{ToolName: "trace_query", Success: true,
		TraceEvidenceAuthority: &types.TraceEvidenceAuthority{FrameEvidenceStatus: "present"}}
	for _, fam := range []struct{ pred, key string }{
		{"root_cause_primary", "root_cause_primary:seed"},
		{"wakeup_chain", "wakeup_chain:seed"},
		{"target_window_states", "target_window_states:seed"},
		{"blocked_reason_census", "blocked_reason_census:seed"},
		{"wakeup_edge_census", "wakeup_edge_census:seed"},
	} {
		seed.Observations = append(seed.Observations, types.ObservationRecord{
			ID: "trace_query:seed#" + fam.pred, Origin: types.AnswerEvidenceOriginRuntimeArtifact,
			Producer: "trace_query", GroundingPolicy: types.ClaimGroundingHard,
			Predicate: fam.pred, ClaimKey: fam.key, Subject: "unknown-32788",
			Object: "seed", Value: "1.000", Unit: "ms",
			RichNotes: []string{"selected_window=3.000000..3.200000"}, Confidence: 0.9,
		})
	}
	ctx.ToolResults = append(ctx.ToolResults, seed)
	out := RunTraceQuerySystemSupplement(ctx)
	meta := ctx.Mutable.SystemTraceSupplementMeta()
	if meta == nil {
		t.Fatalf("supplement meta missing: %+v", out)
	}
	if !meta.CensusLite {
		t.Fatalf("fixture precondition: the census-lite adjunct must have run (vsync family armed, no generator census): %+v", meta)
	}
	if len(meta.Views) == 0 {
		t.Fatalf("fixture precondition: windowed views must have executed: %+v", meta)
	}
	if len(meta.ViewValueObservations) != len(meta.Views) {
		t.Fatalf("AUD-01: per-view counts must stay aligned with the windowed views (views=%v counts=%v)", meta.Views, meta.ViewValueObservations)
	}
	if meta.ViewValueObservations[len(meta.ViewValueObservations)-1] <= 0 {
		t.Fatalf("AUD-01: the LAST windowed view's count must survive the lite trim: %+v", meta)
	}
	// AUD-02: the typed family census pairs with the counts and its
	// non-diagnostic sum equals the paired total.
	if len(meta.ViewObservationFamilies) != len(meta.Views) {
		t.Fatalf("AUD-02: family census must pair with the windowed views: %+v", meta)
	}
	for i, f := range meta.ViewObservationFamilies {
		if sum := f.RootCauseRows + f.WakeupChainRows + f.TargetStateRows + f.CriticalBlockingRows + f.OtherRows; sum != meta.ViewValueObservations[i] {
			t.Fatalf("AUD-02: non-diagnostic family sum %d must equal the paired total %d: %+v", sum, meta.ViewValueObservations[i], f)
		}
	}
}

// TestRuntimeTraceSupplementCountMismatchFailsLoud — AUD-01 (§14.2) renderer
// arm: a present-but-misaligned count slice must disclose the inconsistency,
// never silently drop suffixes (the silent form re-mints the uncounted
// 确定性补跑 face).
func TestRuntimeTraceSupplementCountMismatchFailsLoud(t *testing.T) {
	zh := runtimeTraceSupplementViewListWithCounts([]string{"root_cause_rank", "critical_blocking_calls"}, []int{5}, nil, true)
	if !strings.Contains(zh, "值观测计数不可用（内部不一致）") {
		t.Fatalf("misaligned counts must fail loud on the zh face: %q", zh)
	}
	en := runtimeTraceSupplementViewListWithCounts([]string{"root_cause_rank", "critical_blocking_calls"}, []int{5}, nil, false)
	if !strings.Contains(en, "[value observation count unavailable — internal mismatch]") {
		t.Fatalf("misaligned counts must fail loud on the en face: %q", en)
	}
	// nil counts (legacy runs) stay suffix-free — absence is not mismatch.
	if got := runtimeTraceSupplementViewListWithCounts([]string{"root_cause_rank"}, nil, nil, true); strings.Contains(got, "值观测") {
		t.Fatalf("nil counts must stay suffix-free: %q", got)
	}
}

// TestRuntimeTraceSupplementFamilyBreakdownRenders — AUD-02 (§14.3): the
// per-view family census renders beside the total so the replay
// discrimination reads the ROOT-CAUSE family directly; zero families are
// omitted and the legacy total token stays byte-stable as a prefix.
func TestRuntimeTraceSupplementFamilyBreakdownRenders(t *testing.T) {
	families := []types.TraceSupplementViewFamilyCensus{{
		RootCauseRows: 3, WakeupChainRows: 2, TargetStateRows: 1, OtherRows: 4,
	}}
	zh := runtimeTraceSupplementViewListWithCounts([]string{"root_cause_rank"}, []int{10}, families, true)
	if !strings.Contains(zh, "·值观测10条（根因3·链2·状态1·其他4）") {
		t.Fatalf("zh family breakdown missing: %q", zh)
	}
	en := runtimeTraceSupplementViewListWithCounts([]string{"root_cause_rank"}, []int{10}, families, false)
	if !strings.Contains(en, " [value observations: 10] [families: root_cause 3, chain 2, states 1, other 4]") {
		t.Fatalf("en family breakdown missing: %q", en)
	}
	// The zero-value clause stays byte-identical (no family suffix).
	zero := runtimeTraceSupplementViewListWithCounts([]string{"root_cause_rank"}, []int{0}, []types.TraceSupplementViewFamilyCensus{{DiagnosticRows: 2}}, true)
	if !strings.Contains(zero, "·值观测0条（本视图未产出可用观测，勿视为已补齐）") || strings.Contains(zero, "勿视为已补齐）（") {
		t.Fatalf("zero-value clause must stay byte-identical with no family suffix: %q", zero)
	}
}

// TestR3BSupplementBundleLaneMixedCompile — S14-D 载体覆盖 (§14.8, 2026-07-25):
// the customer runs with a SIBLING tracebundle — the model's own calls go
// source=path against the .systrace (sibling promotion → provenance-aware
// composite index), while the supplement always re-runs on the attached
// blob (attached_trace.txt has no promotable suffix, so its lane is
// structurally bare). The R3B suspect is the MIXED compile: bundle-lane
// observations + bare-supplement observations entering one ledger. This pin
// proves the mixed shape keeps the full carriage chain (ledger causal/state
// rows, projection window, per-view counts/families) — the bare-only e2e
// above cannot cover it.
func TestR3BSupplementBundleLaneMixedCompile(t *testing.T) {
	dir := t.TempDir()
	trace := strings.Join([]string{
		`unknown-32788 (32788) [004] .... 2.990000: sched_switch: prev_comm=idle/4 prev_pid=0 prev_prio=120 prev_state=R ==> next_comm=unknown next_pid=32788 next_prio=53`,
		`unknown-32788 (32788) [004] .... 3.001000: sched_switch: prev_comm=unknown prev_pid=32788 prev_prio=53 prev_state=D ==> next_comm=idle/4 next_pid=0 next_prio=120`,
		`unknown-32788 (32788) [004] .... 3.001200: sched_blocked_reason: pid=32788 iowait=0 caller=timerfd_read+0x70/0x25c`,
		`ss.hm.ugc.aweme-33410 (33410) [005] .... 3.010000: sched_switch: prev_comm=idle/5 prev_pid=0 prev_prio=120 prev_state=R ==> next_comm=ss.hm.ugc.aweme next_pid=33410 next_prio=40`,
		`ss.hm.ugc.aweme-33410 (33410) [005] .... 3.012000: sched_switch: prev_comm=ss.hm.ugc.aweme prev_pid=33410 prev_prio=40 prev_state=S ==> next_comm=idle/5 next_pid=0 next_prio=120`,
		`sysmgr-99 (  99) [004] .... 3.150000: sched_wakeup: comm=unknown pid=32788 prio=53 target_cpu=004`,
		`unknown-32788 (32788) [004] .... 3.150500: sched_switch: prev_comm=idle/4 prev_pid=0 prev_prio=120 prev_state=R ==> next_comm=unknown next_pid=32788 next_prio=53`,
		`unknown-32788 (32788) [004] .... 3.151000: sched_switch: prev_comm=unknown prev_pid=32788 prev_prio=53 prev_state=S ==> next_comm=idle/4 next_pid=0 next_prio=120`,
	}, "\n")
	// The customer's real artifact pair: capture.systrace + sibling bundle.
	capture := filepath.Join(dir, "capture.systrace")
	if err := os.WriteFile(capture, []byte(trace), 0o644); err != nil {
		t.Fatal(err)
	}
	digest := sha256.Sum256([]byte(trace))
	sha := hex.EncodeToString(digest[:])
	captureID, err := tracebundle.CaptureID([]tracebundle.CaptureMember{{
		Type: "systrace", Path: "capture.systrace", Bytes: int64(len(trace)), SHA256: sha,
	}})
	if err != nil {
		t.Fatal(err)
	}
	manifest, err := json.Marshal(map[string]any{
		"schema": tracebundle.SchemaV2, "capture_id": captureID, "version": "test",
		"systrace": "capture.systrace",
		"artifacts": []map[string]any{{
			"type": "systrace", "path": "capture.systrace", "bytes": len(trace), "sha256": sha,
		}},
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "capture.tracebundle.json"), manifest, 0o644); err != nil {
		t.Fatal(err)
	}
	// The attached blob the supplement lane re-runs on (bare by design).
	if err := os.WriteFile(filepath.Join(dir, types.AttachedTraceBlobBasename), []byte(trace), 0o644); err != nil {
		t.Fatal(err)
	}
	ctx := &types.BusContext{RepoRoot: dir, WorkDir: dir, Mutable: types.NewMutableState("分析卡顿")}
	ctx.AnalysisIR = &types.AnalysisIR{RequestModel: types.RequestModel{
		RuntimeTargets: []types.RuntimeTarget{{
			Kind: types.RuntimeTargetKindThread, Thread: "ss.hm.ugc.aweme [32788]",
			Source: "user_explicit", Confidence: 1,
		}},
	}}
	// Model lane: source=path against the systrace — the sibling bundle
	// promotes to the composite index (bundle-lane observations).
	res, err := (&TraceQuery{}).Execute(ctx, json.RawMessage(
		`{"view":"event_search","pattern":"aweme","source":"path","path":"capture.systrace","time_start":3.0,"time_end":3.2}`))
	if err != nil || !res.Success {
		t.Fatalf("bundle-lane model call failed: %+v %v", res, err)
	}
	// Fixture precondition: the sibling bundle promotes — the engine binds
	// the .systrace request to its bundle index (bundle_membership pin form).
	if idx, err := tracequery.BuildIndex(context.Background(), capture); err != nil ||
		!strings.HasSuffix(idx.Path, ".tracebundle.json") {
		t.Fatalf("fixture precondition: sibling bundle must promote: path=%q err=%v", idx.Path, err)
	}
	ctx.ToolResults = append(ctx.ToolResults, res)
	out := RunTraceQuerySystemSupplement(ctx)
	if len(out.Executed) == 0 {
		t.Fatalf("supplement must run on the mixed shape: %+v", out)
	}
	meta := ctx.Mutable.SystemTraceSupplementMeta()
	if meta == nil || len(meta.ViewValueObservations) != len(meta.Views) ||
		len(meta.ViewObservationFamilies) != len(meta.Views) {
		t.Fatalf("per-view counts/families must pair on the mixed shape: %+v", meta)
	}
	rootFamily := 0
	for _, f := range meta.ViewObservationFamilies {
		rootFamily += f.RootCauseRows
	}
	if rootFamily == 0 {
		t.Fatalf("the supplement must account root-cause family rows on the mixed shape: %+v", meta.ViewObservationFamilies)
	}
	input := types.ObservationLedgerInputFromBusContext(ctx, types.ObservationExtractLedgerEvidenceLimit)
	ledger := types.CompileObservationLedger(input)
	rootRows, stateRows := 0, 0
	for _, rec := range ledger.Records {
		if strings.HasPrefix(rec.Predicate, "root_cause") {
			rootRows++
		}
		if rec.Predicate == "target_window_states" {
			stateRows++
		}
	}
	if rootRows == 0 || stateRows == 0 {
		t.Fatalf("mixed bundle+bare compile must keep causal/state rows: root=%d state=%d", rootRows, stateRows)
	}
	set := types.CompileTraceCausalProjectionSet(ledger)
	if len(set.Projections) == 0 {
		t.Fatalf("mixed compile must still project: %+v", set.Projections)
	}
}

// TestR3BSupplementPartialCancellationKeepsCountFace — S14-D 载体覆盖
// (§14.8): a model-lane result that carries a typed in-view cancellation
// record (warm-index partial, SUPP-CANCEL) still contributes its REAL
// observations to the ledger compile and to the supplement family-presence
// read — a partial is a smaller account, never a poisoned one.
func TestR3BSupplementPartialCancellationKeepsCountFace(t *testing.T) {
	dir := t.TempDir()
	trace := strings.Join([]string{
		`unknown-32788 (32788) [004] .... 2.990000: sched_switch: prev_comm=idle/4 prev_pid=0 prev_prio=120 prev_state=R ==> next_comm=unknown next_pid=32788 next_prio=53`,
		`unknown-32788 (32788) [004] .... 3.001000: sched_switch: prev_comm=unknown prev_pid=32788 prev_prio=53 prev_state=D ==> next_comm=idle/4 next_pid=0 next_prio=120`,
		`sysmgr-99 (  99) [004] .... 3.150000: sched_wakeup: comm=unknown pid=32788 prio=53 target_cpu=004`,
		`unknown-32788 (32788) [004] .... 3.150500: sched_switch: prev_comm=idle/4 prev_pid=0 prev_prio=120 prev_state=R ==> next_comm=unknown next_pid=32788 next_prio=53`,
	}, "\n")
	if err := os.WriteFile(filepath.Join(dir, types.AttachedTraceBlobBasename), []byte(trace), 0o644); err != nil {
		t.Fatal(err)
	}
	ctx := &types.BusContext{RepoRoot: dir, WorkDir: dir, Mutable: types.NewMutableState("分析卡顿")}
	ctx.AnalysisIR = &types.AnalysisIR{RequestModel: types.RequestModel{
		RuntimeTargets: []types.RuntimeTarget{{
			Kind: types.RuntimeTargetKindThread, Thread: "unknown [32788]",
			Source: "user_explicit", Confidence: 1,
		}},
	}}
	partial := types.ToolResult{ToolName: "trace_query", Success: true,
		TraceViewCancellation: &types.TraceViewCancellation{View: "window_stats", Reason: "deadline_exceeded"}}
	partial.Observations = append(partial.Observations, types.ObservationRecord{
		ID: "trace_query:partial#target_window_states:1", Origin: types.AnswerEvidenceOriginRuntimeArtifact,
		Producer: "trace_query", GroundingPolicy: types.ClaimGroundingHard,
		Predicate: "target_window_states", ClaimKey: "target_window_states:32788",
		Subject: "unknown-32788", Object: "d_sleep", Value: "12.000", Unit: "ms",
		RichNotes: []string{"selected_window=3.000000..3.200000"}, Confidence: 0.9,
	})
	ctx.ToolResults = append(ctx.ToolResults, partial)
	res, err := (&TraceQuery{}).Execute(ctx, json.RawMessage(`{"view":"event_search","pattern":"unknown","time_start":3.0,"time_end":3.2}`))
	if err != nil || !res.Success {
		t.Fatalf("model call failed: %+v %v", res, err)
	}
	ctx.ToolResults = append(ctx.ToolResults, res)
	out := RunTraceQuerySystemSupplement(ctx)
	if len(out.Executed) == 0 && out.SkipReason == "" {
		t.Fatalf("supplement must run or disclose on the partial shape: %+v", out)
	}
	input := types.ObservationLedgerInputFromBusContext(ctx, types.ObservationExtractLedgerEvidenceLimit)
	ledger := types.CompileObservationLedger(input)
	stateRows := 0
	for _, rec := range ledger.Records {
		if rec.Predicate == "target_window_states" {
			stateRows++
		}
	}
	if stateRows == 0 {
		t.Fatal("a cancellation-carrying partial's real observations must reach the ledger")
	}
}
