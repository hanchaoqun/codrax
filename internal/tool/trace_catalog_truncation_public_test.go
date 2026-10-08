package tool

import (
	"context"
	"database/sql"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/hanchaoqun/codrax/internal/tracecatalog"
	"github.com/hanchaoqun/codrax/internal/tracequery"
	"github.com/hanchaoqun/codrax/internal/types"
)

func TestTraceCatalogPublicResourceTruncationUsesOriginalQueryNotHandoff(t *testing.T) {
	for _, tc := range []struct {
		name        string
		prepare     string
		limit       int
		truncated   bool
		handoffOnly bool
	}{
		{name: "normal"},
		{name: "event_limit", limit: 1, truncated: true},
		{name: "frame_limit", prepare: `DELETE FROM native_hook WHERE id<>1; DELETE FROM native_hook_frame;
			WITH RECURSIVE seq(n) AS (SELECT 1 UNION ALL SELECT n+1 FROM seq WHERE n<129)
			INSERT INTO native_hook_frame SELECT n,7,n-1,n,100,200,0,0,'0x0' FROM seq;`, truncated: true},
		{name: "handoff_event_limit_only", prepare: `DELETE FROM native_hook;
			WITH RECURSIVE seq(n) AS (SELECT 1 UNION ALL SELECT n+1 FROM seq WHERE n<10)
			INSERT INTO native_hook SELECT n,10005000000,NULL,'AllocEvent',512,1,1,512,7,n,NULL FROM seq;`, handoffOnly: true},
		{name: "handoff_byte_limit_only", prepare: `DELETE FROM native_hook WHERE id<>1;
			UPDATE data_dict SET data=replace(hex(zeroblob(2048)),'0',char(10)) WHERE id IN (100,101,102,200,201);`, handoffOnly: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			bus, root := catalogPublicFixture(t)
			path := filepath.Join(root, "capture.data")
			if tc.prepare != "" {
				db, err := sql.Open("sqlite", path)
				if err != nil {
					t.Fatal(err)
				}
				_, err = db.Exec(tc.prepare)
				closeErr := db.Close()
				if err != nil || closeErr != nil {
					t.Fatalf("prepare capture: %v %v", err, closeErr)
				}
			}
			catalog := catalogPublicDiscover(t, bus)
			args := map[string]any{"source": "path", "path": path, "view": "resource_stack", "pid": 101, "time_start": 10, "time_end": 10.05}
			if tc.limit > 0 {
				args["limit"] = tc.limit
			}
			raw, _ := json.Marshal(args)
			result, err := (&TraceQuery{}).Execute(bus, raw)
			if err != nil || !result.Success {
				t.Fatalf("actual public query: %v %s", err, result.Summary)
			}
			var payloadPath string
			handoffOmitted := false
			for _, record := range result.Observations {
				if p, ok := DecodeTraceResourceStack(record); ok {
					payloadPath = record.SourceRef.PayloadRef
					handoffOmitted = resourceStackPublicOmitted(p)
				}
			}
			body, err := os.ReadFile(payloadPath)
			if err != nil {
				t.Fatal(err)
			}
			var original tracequery.Result
			if err := json.Unmarshal(body, &original); err != nil || original.ResourceStack == nil {
				t.Fatalf("original query absent: %v", err)
			}
			if resourceStackPublicOmitted(*original.ResourceStack) != tc.truncated {
				t.Fatalf("fixture does not exercise expected original result cap: %+v", original.ResourceStack)
			}
			if tc.handoffOnly && !handoffOmitted {
				t.Fatal("fixture did not exercise independent handoff omission")
			}
			if tc.truncated {
				if result.EnumerationAuthority == nil || result.EnumerationAuthority.Status != "incomplete" || len(result.EnumerationAuthority.Boundaries) == 0 {
					t.Fatal("original result truncation lacks precise shared metadata")
				}
				for _, b := range result.EnumerationAuthority.Boundaries {
					if !b.TotalKnown || b.Total <= b.Emitted {
						t.Fatalf("known source census was lost: %+v", b)
					}
				}
			} else if result.EnumerationAuthority != nil && result.EnumerationAuthority.Status == "incomplete" {
				t.Fatal("handoff omission became original result truncation")
			}
			ref, err := saveTraceCatalog(bus, catalog)
			if err != nil {
				t.Fatal(err)
			}
			saved, err := tracecatalog.Load(context.Background(), ref)
			if err != nil {
				t.Fatal(err)
			}
			attempts := 0
			for _, q := range saved.Queries {
				for _, a := range q.Attempts {
					attempts++
					if a.Outcome != tracecatalog.OutcomeSuccess || a.Truncated != tc.truncated || a.PayloadRef != payloadPath || a.SourceRevision == "" {
						t.Fatalf("persisted query lost status/source/ref/truncation: %+v", a)
					}
				}
			}
			if attempts != 1 {
				t.Fatalf("actual query attempts=%d", attempts)
			}
		})
	}
}

func TestTraceQueryResourceStackEnumerationPreservesSharedBoundariesWithoutGrant(t *testing.T) {
	path, _ := filepath.Abs("../../eval/fixtures/hmosperf_native_resource_stack/capture.data")
	_, _, p := nativeStackPublicQuery(t, path, map[string]any{"pid": 101, "time_start": 10, "time_end": 10.05})
	complete := tracequery.Result{View: tracequery.ViewResourceStack, ResourceStack: &p}
	if traceQueryResourceStackIncompleteEnumeration(complete) != nil {
		t.Fatal("untruncated stream gained a new completeness grant")
	}
	p.MatchedEvents++
	p.OmittedEvents++
	result := tracequery.Result{View: tracequery.ViewResourceStack, ResourceStack: &p,
		Compactions: []tracequery.ViewCompaction{{View: "existing_view", Dimension: "existing_rows", Total: 5, Emitted: 2}}}
	before, _ := json.Marshal(result)
	authority := traceQueryEnumerationAuthority(result)
	if authority.Status != "incomplete" || len(authority.Boundaries) != 2 {
		t.Fatalf("existing or new precise boundary lost: %+v", authority)
	}
	combined := types.BuildRuntimeArtifactEnumerationAuthority([]types.ToolResult{{ToolName: "trace_query", EnumerationAuthority: authority}})
	if !combined.Incomplete || len(combined.Boundaries) != 2 || !combined.Boundaries[1].TotalKnown {
		t.Fatalf("shared consumer lost source census: %+v", combined)
	}
	after, _ := json.Marshal(result)
	if string(before) != string(after) {
		t.Fatal("enumeration adapter mutated original payload")
	}
	for _, unavailable := range []tracequery.Result{{View: tracequery.ViewResourceStack}, {View: tracequery.ViewResourceStack, ResourceStack: &tracequery.ResourceStackResult{Status: "unavailable", Reason: "no_resource_stack_observations"}}} {
		if traceQueryResourceStackIncompleteEnumeration(unavailable) != nil {
			t.Fatal("unavailable result gained an empty/complete certificate")
		}
	}
	p.MatchedEvents-- // Malformed counters cannot introduce a new boundary.
	if traceQueryResourceStackIncompleteEnumeration(complete) != nil {
		t.Fatal("invalid original result granted enumeration metadata")
	}
}

func resourceStackPublicOmitted(p tracequery.ResourceStackResult) bool {
	if p.OmittedEvents > 0 {
		return true
	}
	for _, e := range p.Events {
		if e.OmittedFrames > 0 {
			return true
		}
	}
	return false
}
