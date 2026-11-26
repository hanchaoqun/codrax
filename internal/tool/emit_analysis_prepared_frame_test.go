package tool

import (
	"context"
	"fmt"
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/loginput"
	"github.com/hanchaoqun/codrax/internal/traceinput"
	"github.com/hanchaoqun/codrax/internal/types"
)

func TestEmitAnalysisKeepsVerifiedPreparedFrameWindow(t *testing.T) {
	material, err := traceinput.Prepare(context.Background(), traceinput.Options{
		InputPath:     "../../eval/fixtures/hmosperf_rendering_candidates/events.systrace",
		RuntimeAnchor: t.TempDir(), PreviewBytes: 1024,
	})
	if err != nil {
		t.Fatal(err)
	}
	const fact = `{"record_kind":"frame","side":"target","metric_kind":"jank","count":1,"thread_id":42,"start_ns":236829991714000,"end_ns":236830347598000,"observation_id":"o","source_id":"s","frame_metrics":{"duration_ns":355884000}}`
	catalog, err := loginput.Prepare(context.Background(), []loginput.Input{{Name: "saved fact", Data: []byte(fact)}}, loginput.Options{})
	if err != nil {
		t.Fatal(err)
	}
	request := fmt.Sprintf("# 单帧原因调查\n\n目标 Trace: %s；线程=42；帧窗口=236829.991714000..236830.347598000秒。\n解释这个帧窗口内的原因。", material.SourcePath())
	for _, tc := range []struct {
		name    string
		raw     string
		bounded bool
	}{
		{"verified frame", request, true},
		{"different thread", strings.Replace(request, "线程=42", "线程=43", 1), false},
		{"different window", strings.Replace(request, "236830.347598000", "236830.347598001", 1), false},
		{"different attached path", strings.Replace(request, material.SourcePath(), material.SourcePath()+".other", 1), false},
		{"ordinary trace request", strings.Replace(request, "# 单帧原因调查", "# 全部轨迹分析", 1), false},
		{"additional window", request + "\n；帧窗口=236830.4..236830.5秒。", false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			ctx := &types.BusContext{
				Mutable: types.NewMutableState(tc.raw), AttachedTraceMaterial: material,
				AttachedHitrace: material.Preview(), AttachedLogCatalog: catalog,
			}
			params := boundedScopeAuthorityPublicParams(t, map[string]any{
				"requested_scope": "full_artifact", "source_quote": "目标 Trace", "confidence": .9,
			}, tc.raw)
			result, err := (&EmitAnalysis{}).Execute(ctx, params)
			if err != nil || !result.Success {
				t.Fatalf("emit_analysis: %v %+v", err, result)
			}
			rm := ctx.Mutable.RequestModel()
			start, end, bounded := rm.RuntimeArtifactScopeProfile.ExplicitTimeWindow()
			if bounded != tc.bounded {
				t.Fatalf("got scope %+v", rm.RuntimeArtifactScopeProfile)
			}
			if bounded && (start != 236829.991714 || end != 236830.347598 || len(rm.RuntimeTargets) != 1 ||
				rm.RuntimeTargets[0].PID != 42 || rm.RuntimeTargets[0].Kind != types.RuntimeTargetKindThread) {
				t.Fatalf("wrong scoped target %v .. %v %+v", start, end, rm.RuntimeTargets)
			}
		})
	}
}
