package tool

import (
	"encoding/json"
	"math"
	"os"
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/tracequery"
	"github.com/hanchaoqun/codrax/internal/types"
)

func TestHMC367HoneybirdWholeCompilerScopePreservesBusinessWork(t *testing.T) {
	path := os.Getenv("CODRAX_HMC365_HONEYBIRD")
	if path == "" {
		t.Skip("requires the pinned complete Honeybird ftrace")
	}
	const digest = "21280a96618781b37be37f9289be9ba286d6a85d358e60ff72d9c73c827cf30e"
	if h, n := hmc176bRepresentativeDigest(t, path); h != digest || n != 106896120 {
		t.Fatal("not the original capture")
	}
	hmc176DisableExternalProviders(t)
	ctx, _, _ := hmc17NamedPathContext(t)
	query := func(view string, extra map[string]any) tracequery.Result {
		t.Helper()
		params := map[string]any{"source": "path", "path": path, "view": view, "pid": 64156, "target_scope": "thread", "time_start": 11343.248, "time_end": 11343.310}
		for k, v := range extra {
			params[k] = v
		}
		r := hmc17NamedQuery(t, ctx, params)
		return hmc367NativeResult(t, r, view)
	}
	rank := query("root_cause_rank", nil).RootCauseRank
	if rank == nil {
		t.Fatal("no native ranking")
	}
	classVerification, scheduling := false, false
	for _, row := range rank.Items {
		if strings.Contains(row.SpanName, "JitMemoryRegion::Initialize") && row.Type == "jit_compile" {
			t.Fatalf("initialization still enters the native compile lane: %+v", row)
		}
		classVerification = classVerification || (row.Type == "class_verification" && row.ChainRelevance == "on_chain" && math.Abs(row.EffectiveImpactMs-3.868) < 0.001)
		scheduling = scheduling || (row.Type == "priority_inversion_candidate" && row.ChainRelevance == "on_chain" && math.Abs(row.EffectiveImpactMs-36.311) < 0.001)
	}
	if !classVerification || !scheduling {
		t.Fatalf("lost existing on-chain semantic or scheduler evidence: verification=%t scheduling=%t", classVerification, scheduling)
	}
	markers := query("event_search", map[string]any{"pattern": "JitMemoryRegion::Initialize"})
	for _, e := range markers.Events {
		if e.Line == 20198 && e.PID == 64156 && strings.Contains(e.SpanName, "JitMemoryRegion::Initialize") {
			return // the original business marker and identity remain queryable
		}
	}
	t.Fatal("the original initialization marker was lost from the public event query")
}

func hmc367NativeResult(t *testing.T, r types.ToolResult, view string) tracequery.Result {
	t.Helper()
	seen := map[string]bool{}
	for _, row := range r.Observations {
		p := row.SourceRef.PayloadRef
		if p == "" || seen[p] {
			continue
		}
		seen[p] = true
		data, err := os.ReadFile(p)
		var v tracequery.Result
		if err == nil && json.Unmarshal(data, &v) == nil && v.View == view {
			return v
		}
	}
	t.Fatalf("missing public %s payload", view)
	return tracequery.Result{}
}
