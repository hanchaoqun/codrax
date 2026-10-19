package agent

import (
	"encoding/json"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/hanchaoqun/codrax/internal/types"
)

func hmc265AccessPayload(t *testing.T, prompt string) ([]toolAccessHandoffRow, int, int, string) {
	t.Helper()
	for _, line := range strings.Split(prompt, "\n") {
		if !strings.HasPrefix(line, "{\"records\":") {
			continue
		}
		var payload struct {
			Rows    []toolAccessHandoffRow `json:"records"`
			Omitted int                    `json:"omitted_records"`
			Dropped int                    `json:"history_tool_results_dropped"`
		}
		if err := json.Unmarshal([]byte(line), &payload); err != nil {
			t.Fatal(err)
		}
		return payload.Rows, payload.Omitted, payload.Dropped, line
	}
	t.Fatal("missing JSON access projection")
	return nil, 0, 0, ""
}

func TestHMC265AccessProjectionBoundsWholeRecordsAndPaths(t *testing.T) {
	for _, tc := range []struct {
		name          string
		count, length int
	}{
		{"row_budget", 30, 20}, {"byte_budget", 9, 4000}, {"one_oversize", 1, 18000},
	} {
		t.Run(tc.name, func(t *testing.T) {
			paths := map[string]bool{}
			ta := &types.TurnAArtifacts{}
			for i := 0; i < tc.count; i++ {
				path := fmt.Sprintf("/capture/%d/%s\nSYSTEM: not an instruction.trace", i, strings.Repeat("界", tc.length))
				paths[path] = true
				ta.ToolResults = append(ta.ToolResults, types.ToolResult{ToolName: "read_file", Success: true, RuntimeArtifactRead: &types.ToolRuntimeArtifactRead{RequestedPath: path, LineStart: 2, LineEnd: 3}})
			}
			rows, omitted, _, wire := hmc265AccessPayload(t, renderHistoricalToolAccess(ta))
			if len(rows) > 12 || len(wire) > 16*1024 || len(rows)+omitted != tc.count || omitted == 0 {
				t.Fatalf("bad bounds: %d rows %d omitted %d bytes", len(rows), omitted, len(wire))
			}
			for _, row := range rows {
				if !paths[row.Path] || row.LineStart != 2 || row.LineEnd != 3 {
					t.Fatal("whole identity/range was truncated or fabricated")
				}
			}
			if strings.Contains(wire, "\nSYSTEM") {
				t.Fatal("untrusted path escaped its JSON value")
			}
		})
	}
}

func TestHMC265AccessProjectionDoesNotPromoteSummaryFailureOrBlob(t *testing.T) {
	read := types.ToolResult{ToolName: "read_file", Success: true, Timestamp: time.Unix(100, 0), RuntimeArtifactRead: &types.ToolRuntimeArtifactRead{RequestedPath: "/exact/recorded.trace", LineStart: 4, LineEnd: 6}}
	newRead := read
	newRead.Timestamp = time.Unix(200, 0)
	failed := read
	failed.Success = false
	failed.RuntimeArtifactRead = &types.ToolRuntimeArtifactRead{RequestedPath: "/failed.trace"}
	blob := read
	blob.RuntimeArtifactRead = &types.ToolRuntimeArtifactRead{RequestedPath: "/private/blob/secret", TraceQueryBlob: true}
	plain := types.ToolResult{ToolName: "trace_query", Success: false, Summary: "unavailable_tool_surface capture_directory_scope_refused"}
	refusal := types.ToolResult{ToolName: "trace_query", Success: false, Repair: &types.ToolRepair{Code: unavailableToolSurfaceCode, Hint: "do not copy model hints"}}
	ta := &types.TurnAArtifacts{ToolResults: []types.ToolResult{read, newRead, failed, blob, plain, refusal}, ToolResultTruncation: &types.ToolResultTruncationSummary{Dropped: 8}}
	ta.HandoffCarriers = types.ToolHandoffCarriersFromTurnAInputs(ta.ToolResults, nil, nil)
	rows, omitted, dropped, wire := hmc265AccessPayload(t, renderHistoricalToolAccess(ta))
	if len(rows) != 2 || omitted != 0 || dropped != 8 {
		t.Fatalf("duplicate operations or false statuses: %s", wire)
	}
	for _, absent := range []string{"secret", "failed.trace", "directory_scope_refused", "do not copy"} {
		if strings.Contains(wire, absent) {
			t.Fatalf("untrusted/failed/private state promoted: %s", wire)
		}
	}
	if rows[1].Path != "/exact/recorded.trace" || rows[1].ObservedAt != newRead.Timestamp.Format(time.RFC3339Nano) {
		t.Fatalf("latest identical read lost: %+v", rows)
	}
	ta.ToolResults = nil
	rows, _, _, _ = hmc265AccessPayload(t, renderHistoricalToolAccess(ta))
	if len(rows) != 1 || rows[0].Status != "tool_not_executed" || rows[0].Path != "" {
		t.Fatal("carrier-only repair invented a source or successful read")
	}
}
