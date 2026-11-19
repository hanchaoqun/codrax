package tool

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/tracequery"
	"github.com/hanchaoqun/codrax/internal/types"
)

// HMC377 thread identity (D3/D4/E5), whole originals at the public trace_query
// entry. Opt-in: each test skips without its variable and fails when the file
// is not the pinned original. The expected names and rows were read
// independently from the raw captures (lane oracle ti_oracle.py).

const (
	hmc377HoneybirdDigest = "21280a96618781b37be37f9289be9ba286d6a85d358e60ff72d9c73c827cf30e"
	hmc377HoneybirdSize   = 106896120
	hmc377DouyinDigest    = "8087138a5810347675fe7764974f5103cad4a9a71e56d7d1702d65da2256892e"
	hmc377DouyinSize      = 138438125
	hmc377NetEaseDigest   = "48cf2fd8b91a81f2276a16a5988ef14729c078e5d1f23615eb166d2f6e2c8cbc"
	hmc377NetEaseSize     = 104144939
)

// hmc377TypedNames returns every name the native result gives tid in typed
// thread fields (the identity disclosure itself excluded).
func hmc377TypedNames(t *testing.T, native tracequery.Result, tid int) map[string]bool {
	t.Helper()
	native.ThreadIdentities = nil
	data, err := json.Marshal(native)
	if err != nil {
		t.Fatal(err)
	}
	var decoded any
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatal(err)
	}
	out := map[string]bool{}
	var walk func(any)
	walk = func(v any) {
		switch x := v.(type) {
		case map[string]any:
			if pid, ok := x["pid"].(float64); ok && int(pid) == tid {
				if comm, ok := x["comm"].(string); ok && comm != "" {
					out[comm] = true
				}
			}
			for _, vv := range x {
				walk(vv)
			}
		case []any:
			for _, vv := range x {
				walk(vv)
			}
		}
	}
	walk(decoded)
	return out
}

func hmc377Identity(native tracequery.Result, tid int) (tracequery.ThreadDisplayIdentity, bool) {
	for _, identity := range native.ThreadIdentities {
		if identity.TID == tid {
			return identity, true
		}
	}
	return tracequery.ThreadDisplayIdentity{}, false
}

// Honeybird: the waker 64176 is created at line 20301 under its creator's
// name ele.crowdsource and names itself Signal Catcher between lines 25220
// and 25229; one thread, one name. The protected board does not move.
func TestHMC377WholeOriginalHoneybirdOneIdentityPerThread(t *testing.T) {
	path := hmc376Original(t, "CODRAX_HMC365_HONEYBIRD", hmc377HoneybirdDigest, hmc377HoneybirdSize)
	result, native := hmc376QueryTarget(t, path, "root_cause_rank", map[string]any{"pid": 64156, "target_scope": "thread"}, 11343.248, 11343.310)
	if native.RootCauseRank == nil {
		t.Fatal("no rank board")
	}
	if got := hmc377TypedNames(t, native, 64176); len(got) != 1 || !got["Signal Catcher"] {
		t.Fatalf("waker 64176 published under %v", got)
	}
	if got := hmc377TypedNames(t, native, 64156); len(got) != 1 || !got["ele.crowdsource"] {
		t.Fatalf("main thread 64156 published under %v", got)
	}
	var inversion, verification, running, overlap, adjacentOverlap *tracequery.RootCauseRankItem
	for i := range native.RootCauseRank.Items {
		item := &native.RootCauseRank.Items[i]
		if item.Thread.TGID == 64156 && item.ProcessComm != "" && item.ProcessComm != "ele.crowdsource" {
			t.Fatalf("process of 64156 named %q", item.ProcessComm)
		}
		switch {
		case item.Type == "priority_inversion_candidate" && item.Thread.PID == 64176:
			inversion = item
		case item.Type == "class_verification" && item.Thread.PID == 64156:
			verification = item
		case item.Type == "running" && item.Thread.PID == 64156:
			running = item
		case item.Type == "priority_inversion_runnable_wait" && item.Thread.PID == 64156:
			overlap = item
		case item.Type == "priority_inversion_runnable_wait" && item.Thread.PID == 64176:
			t.Fatalf("one physical thread, one object (R-377-4): the waker's window account must not stay a separate board row: %s", hmc376RowLabel(*item))
		}
	}
	// EVOLUTION RECORD (HMC377 R-377-4, owner ruling): the waker's whole
	// window ready account (45.878 ms, overlap 8.408 ms) used to ride the ◇
	// lane as a second object beside its own chain seat; it is now that
	// seat's lossless detail on the absorbed carrier, values unchanged.
	for i := range native.RootCauseRank.AbsorbedItems {
		item := &native.RootCauseRank.AbsorbedItems[i]
		if item.Type == "priority_inversion_runnable_wait" && item.Thread.PID == 64176 {
			adjacentOverlap = item
		}
	}
	if inversion == nil || verification == nil || running == nil || overlap == nil || adjacentOverlap == nil {
		t.Fatal("board rows missing")
	}
	// Protected numbers (raw oracle): 36.311 / 3.868 / 2.600 / 0.013 and the
	// measured 45.378 ms ready of the waker in the dependency window.
	if !hmc376Near(inversion.EffectiveImpactMs, 36.311) || !hmc376Near(inversion.RunnableMs, 45.378) || inversion.Rank != 1 ||
		!hmc376Near(verification.EffectiveImpactMs, 3.868) || !hmc376Near(running.EffectiveImpactMs, 2.600) ||
		!hmc376Near(overlap.EffectiveImpactMs, 0.013) {
		t.Fatalf("protected board moved: %s | %s | %s | %s", hmc376RowLabel(*inversion), hmc376RowLabel(*verification), hmc376RowLabel(*running), hmc376RowLabel(*overlap))
	}
	// The same-CPU overlap of the waker's ready time is bounded by the time a
	// lower-priority thread actually ran on that CPU (raw oracle: 8.678 ms by
	// switch-in CPU over its 45.878 ms ready time in the asked window).
	if adjacentOverlap.EffectiveImpactMs > 8.678+0.0015 || adjacentOverlap.Thread.Comm != "Signal Catcher" {
		t.Fatalf("overlap row exceeds measured lower-priority running or keeps an old name: %s", hmc376RowLabel(*adjacentOverlap))
	}
	identity, ok := hmc377Identity(native, 64176)
	if !ok || identity.DisplayComm != "Signal Catcher" || len(identity.Names) != 2 ||
		identity.Names[0].Comm != "ele.crowdsource" || !identity.Names[0].InheritedAtCreation || identity.Names[0].FirstLine != 20301 || identity.Names[0].LastLine != 25220 ||
		identity.Names[1].Comm != "Signal Catcher" || identity.Names[1].FirstLine != 25229 {
		t.Fatalf("waker identity disclosure: %+v", identity)
	}
	// The projection keeps one subject for the waker and its measured
	// component split.
	projection := hmc376Projection(result)
	for _, nodes := range [][]types.TraceCausalProjectionNode{projection.OnChainCauses, projection.AdjacentCauses, projection.BackgroundCauses} {
		for _, node := range nodes {
			if strings.HasSuffix(node.Subject, "-64176") && node.Subject != "Signal Catcher-64176" {
				t.Fatalf("projection subject %q", node.Subject)
			}
			if node.TypeToken == "priority_inversion_candidate" && node.Subject == "Signal Catcher-64176" {
				measured, uncounted, ok := runtimeTraceProjInversionMeasuredRunnable(node)
				if !ok || !hmc376Near(measured, 45.378) || !hmc376Near(uncounted, 9.912) ||
					!hmc376Near(runtimeTraceProjInversionMeasuredQualifiedRunning(node), 1.314) {
					t.Fatalf("component split: measured=%.3f uncounted=%.3f running=%.3f node=%+v", measured, uncounted,
						runtimeTraceProjInversionMeasuredQualifiedRunning(node), node)
				}
			}
		}
	}
	// The waker's own timeline over the asked window reports its creation,
	// never a generation boundary.
	_, timeline := hmc376QueryTarget(t, path, "thread_timeline", map[string]any{"pid": 64176}, 11343.248, 11343.310)
	if timeline.Timeline == nil {
		t.Fatal("no waker timeline")
	}
	created := false
	for _, caveat := range timeline.Timeline.Caveats {
		if strings.Contains(caveat, "thread_generation_boundary") {
			t.Fatalf("first creation reported as a generation boundary: %s", caveat)
		}
		created = created || strings.HasPrefix(caveat, "thread created at 11343.250632 s (line 20301): TID 64176")
	}
	if !created {
		t.Fatalf("creation not disclosed: %v", timeline.Timeline.Caveats)
	}
}

// Douyin reboot capture 5: the main thread 11105 records main ->
// .ugc.aweme.lite -> re-initialized> -> .ugc.aweme.lite; inside the asked
// window it is .ugc.aweme.lite, and so is its process.
func TestHMC377WholeOriginalDouyinMainThreadAndProcessOneName(t *testing.T) {
	path := hmc376Original(t, "CODRAX_HMC377_DOUYIN_REBOOT", hmc377DouyinDigest, hmc377DouyinSize)
	_, native := hmc376QueryTarget(t, path, "root_cause_rank", map[string]any{"pid": 11105, "target_scope": "thread"}, 140.911304, 141.835458)
	if native.RootCauseRank == nil {
		t.Fatal("no rank board")
	}
	if got := hmc377TypedNames(t, native, 11105); len(got) != 1 || !got[".ugc.aweme.lite"] {
		t.Fatalf("main thread 11105 published under %v", got)
	}
	for _, item := range native.RootCauseRank.Items {
		if item.Thread.TGID == 11105 && item.ProcessComm != "" && item.ProcessComm != ".ugc.aweme.lite" {
			t.Fatalf("process of 11105 named %q on %s", item.ProcessComm, hmc376RowLabel(item))
		}
	}
	identity, ok := hmc377Identity(native, 11105)
	if !ok || identity.DisplayComm != ".ugc.aweme.lite" {
		t.Fatalf("main thread identity: %+v", identity)
	}
	names := map[string]bool{}
	for _, name := range identity.Names {
		names[name.Comm] = true
	}
	if !names["main"] || !names["re-initialized>"] || !names[".ugc.aweme.lite"] {
		t.Fatalf("recorded names not disclosed: %+v", identity.Names)
	}
}

// NetEase reboot capture 5: the protected collection-wait seat keeps rank 1
// at 123.003 ms; the main thread keeps its single name.
func TestHMC377WholeOriginalNetEaseProtectedSeatUnchanged(t *testing.T) {
	path := hmc376Original(t, "CODRAX_HMC376_NETEASE_REBOOT_FRAME", hmc377NetEaseDigest, hmc377NetEaseSize)
	_, native := hmc376QueryTarget(t, path, "root_cause_rank", map[string]any{"pid": 11077, "target_scope": "thread"}, 153.763595, 153.887340)
	if native.RootCauseRank == nil || len(native.RootCauseRank.Items) == 0 {
		t.Fatal("no rank board")
	}
	first := native.RootCauseRank.Items[0]
	if first.Rank != 1 || first.Thread.PID != 11077 || !hmc376Near(first.EffectiveImpactMs, 123.003) || first.ChainRelevance != "on_chain" {
		t.Fatalf("protected seat moved: %s", hmc376RowLabel(first))
	}
	if got := hmc377TypedNames(t, native, 11077); len(got) != 1 || !got["ease.cloudmusic"] {
		t.Fatalf("main thread 11077 published under %v", got)
	}
}
