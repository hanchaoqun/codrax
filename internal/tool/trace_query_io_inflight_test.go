package tool

import (
	"encoding/json"
	"fmt"
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/skill"
	"github.com/hanchaoqun/codrax/internal/tracequery"
	"github.com/hanchaoqun/codrax/internal/types"
)

func TestIOInFlightPublicTeachingAndCatalog(t *testing.T) {
	var schema struct {
		Properties map[string]struct{ Description string } `json:"properties"`
	}
	query := &TraceQuery{}
	if err := json.Unmarshal(query.Parameters(), &schema); err != nil {
		t.Fatal(err)
	}
	for name, text := range map[string]string{"description": query.Description(), "view": schema.Properties["view"].Description, "matrix": skill.RenderTraceQueryViewMatrix()} {
		if strings.Count(text, skill.TraceIOInFlightTeaching) != 1 {
			t.Errorf("%s must share occupancy teaching once", name)
		}
	}
	catalog := capabilityPublicCall(t, nil, `{"view":"window_stats","detail":true}`)
	var metric map[string]any
	for _, raw := range catalog["metrics"].([]any) {
		candidate := raw.(map[string]any)
		if candidate["id"] == "io_inflight" {
			metric = candidate
		}
	}
	if metric == nil {
		t.Fatal("public capability catalog omits occupancy")
	}
	units := map[string]string{}
	for _, raw := range metric["outputs"].([]any) {
		out := raw.(map[string]any)
		for _, field := range out["fields"].([]any) {
			units[field.(string)] = out["unit"].(string)
		}
	}
	for field, unit := range map[string]string{"peak_requests": "requests", "mean_requests": "requests", "request_ms": "request·ms", "busy_ms": "ms", "issue_count": "count", "start_ts": "seconds"} {
		if units[field] != unit {
			t.Errorf("%s unit %q != %q", field, units[field], unit)
		}
	}
}

func TestIOInFlightPromptProjectionIsBoundedAndNoncausal(t *testing.T) {
	stats := &tracequery.IOInFlightStats{
		Window: &tracequery.IOInFlightWindow{StartTs: 0, EndTs: 1}, GroupCount: 10, OmittedGroups: 2,
		Coverage: []tracequery.IOInFlightPairingCoverage{{Family: "block", Status: "partial", UnpairedStartCount: 3, AmbiguousCohortCount: 2}},
		Groups:   []tracequery.IOInFlightGroup{{SourcePath: "/capture/a", Layer: "block", EndpointFamily: "block_rq", Dev: "8,0", Operation: "R", IssueCount: 3, AcceptedPairCount: 1, Values: &tracequery.IOInFlightValues{}, OmittedSegments: 4}},
	}
	for i := 0; i < 16; i++ {
		stats.Groups[0].Segments = append(stats.Groups[0].Segments, tracequery.IOInFlightSegment{StartTs: float64(i) / 16, EndTs: float64(i+1) / 16, Requests: i % 2})
	}
	rows := traceQueryTypedIOInFlightObservations(stats, types.ObservationSourceRef{Path: "/wrapper", QueryScopeID: "query1"}, "scope", "now")
	if len(rows) != 2 || rows[0].SourceRef.Path != "/capture/a" || rows[0].SourceRef.QueryScopeID != "query1" {
		t.Fatalf("wrong source or publication shape: %+v", rows)
	}
	if rows[0].Value != "0" || rows[0].Unit != "requests" || !strings.Contains(rows[0].Summary, "request_ms=0 request·ms") {
		t.Fatalf("measured zero/units were lost: %+v", rows[0])
	}
	for _, row := range rows {
		if row.Role != types.AnswerAggregateRoleSupportingCoverage || strings.Contains(row.ClaimKey, "root_cause") {
			t.Fatalf("occupancy acquired causal role: %+v", row)
		}
		for _, note := range row.RichNotes {
			key, _, _ := strings.Cut(note, "=")
			if _, ok := types.TraceNoteKeyLookup(key); !ok {
				t.Errorf("unregistered note %q", key)
			}
		}
	}
	preview := traceQueryIOInFlightTimeline(stats.Groups[0])
	shown := strings.Count(preview, "):0") + strings.Count(preview, "):1")
	if len("io_inflight_timeline="+preview) > 160 || !strings.Contains(preview, fmt.Sprintf("omitted=%d", 20-shown)) {
		t.Fatalf("timeline prefix is not bounded with accurate omissions: %s", preview)
	}
	stats.Groups[0].Values = nil
	unknown := traceQueryTypedIOInFlightObservations(stats, types.ObservationSourceRef{}, "scope", "now")[0]
	if unknown.Value != "" || !strings.Contains(unknown.Summary, "not measured zero") {
		t.Fatalf("unmeasured group became zero: %+v", unknown)
	}
}
