package tool

import (
	"encoding/json"
	"github.com/hanchaoqun/codrax/internal/tracequery"
	"github.com/hanchaoqun/codrax/internal/tracewire"
	"github.com/hanchaoqun/codrax/internal/types"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
)

func TestPreferredFrameRatePublicReceipt(t *testing.T) {
	pid := 100
	integer := func(v string) tracewire.ProcessMeasureScalar {
		return tracewire.ProcessMeasureScalar{Status: "known", StorageClass: "integer", Value: v}
	}
	r := tracewire.ProcessMeasureInterval{RowID: 1, FilterID: integer("1"), IPID: integer("1"), StartNS: integer("0"), DurationNS: integer("1000000000"), Value: tracewire.ProcessMeasureScalar{Status: "invalid_storage", StorageClass: "real", Value: "119.88"}, Name: "H:PreferredFrameRate", NameKnown: true, PID: &pid, ProcessName: "render_service", OwnerStatus: "known"}
	line, err := tracewire.FormatProcessMeasureInterval(r)
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(t.TempDir(), "capture.systrace")
	if err = os.WriteFile(path, []byte(line+"\n"), 0600); err != nil {
		t.Fatal(err)
	}
	ctx, _, _ := hmc17NamedPathContext(t)
	args, _ := json.Marshal(map[string]any{"source": "path", "path": path, "view": "preferred_frame_rate", "time_start": 0, "time_end": 1})
	out, err := (&TraceQuery{}).Execute(ctx, args)
	if err != nil || !out.Success {
		t.Fatalf("query: %v %s", err, out.Summary)
	}
	found := false
	for _, r := range out.Observations {
		if r.Predicate != "preferred_frame_rate_observation" {
			continue
		}
		p, ok := types.DecodeRuntimeMeasurementPublication(r)
		if !ok || len(p.Tables) != 3 {
			t.Fatalf("missing three native tables: %+v", r)
		}
		found = true
	}
	if !found {
		t.Fatal("no source-bound preferred-frame-rate receipt")
	}
}

func TestPreferredFrameRatePublicScopeAndSchema(t *testing.T) {
	path, _ := filepath.Abs("../../eval/fixtures/hmosperf_preferred_frame_rate/capture.data")
	for _, tc := range []struct {
		name      string
		focus     []types.RuntimeTarget
		pid, rows int
	}{
		{"all", nil, 0, 3},
		{"thread is not process", []types.RuntimeTarget{{Kind: types.RuntimeTargetKindThread, PID: 100, Source: "user_explicit", Confidence: 1}}, 0, 3},
		{"typed process", []types.RuntimeTarget{{Kind: types.RuntimeTargetKindProcess, PID: 100, Source: "user_explicit", Confidence: 1}}, 100, 2},
		{"absent process", nil, 999, 0},
	} {
		t.Run(tc.name, func(t *testing.T) {
			ctx, _, _ := hmc17NamedPathContext(t)
			ctx.AnalysisIR = &types.AnalysisIR{RequestModel: types.RequestModel{RuntimeTargets: tc.focus}}
			params := map[string]any{"source": "path", "path": path, "view": "preferred_frame_rate", "time_start": 1, "time_end": 2}
			if tc.name == "absent process" {
				params["pid"] = 999
			}
			args, _ := json.Marshal(params)
			out, err := (&TraceQuery{}).Execute(ctx, args)
			if err != nil || !out.Success {
				t.Fatalf("scope query: %v %s", err, out.Summary)
			}
			found := false
			for _, r := range out.Observations {
				if r.Predicate != TracePreferredFrameRatePredicate {
					continue
				}
				p, ok := types.DecodeRuntimeMeasurementPublication(r)
				if !ok || r.SourceRef.QueryTargetPID != tc.pid || r.SourceRef.QueryTargetScope != "process" || r.SourceRef.QueryTargetThread != "" {
					t.Fatalf("wrong authority %+v", r)
				}
				for _, table := range p.Tables {
					if table.MemberSet != nil {
						t.Fatal("display became completion")
					}
					if table.View == types.RuntimeMeasurementSummary && len(table.Rows) != tc.rows {
						t.Fatalf("summary=%d want%d", len(table.Rows), tc.rows)
					}
				}
				found = true
			}
			if !found {
				t.Fatal("missing source-bound rate tables")
			}
		})
	}
	for _, args := range []string{`{"view":"preferred_frame_rate","thread":"render_service"}`, `{"view":"preferred_frame_rate","pid":100,"target_scope":"thread"}`} {
		out, err := (&TraceQuery{}).Execute(nil, json.RawMessage(args))
		if err != nil || out.Success || out.Repair == nil || out.Repair.Code != "trace_query_process_owned_view" || out.RawRef != "" {
			t.Fatalf("thread was silently treated as process: %v %+v", err, out)
		}
	}
	if !strings.Contains(string((&TraceQuery{}).Parameters()), tracequery.PreferredFrameRateTeaching) {
		t.Fatal("schema lost shared protocol")
	}
}

func TestPreferredFrameRatePublicReceiptWholeSeriesBudget(t *testing.T) {
	for _, tc := range []struct {
		name              string
		owners, nameBytes int
		empty             bool
	}{{"some retained", 32, 2000, false}, {"no complete series fits", 1, 70000, true}} {
		t.Run(tc.name, func(t *testing.T) {
			integer := func(v int) tracewire.ProcessMeasureScalar {
				return tracewire.ProcessMeasureScalar{Status: "known", StorageClass: "integer", Value: strconv.Itoa(v)}
			}
			var source strings.Builder
			for i := 0; i < tc.owners; i++ {
				pid := 100 + i
				r := tracewire.ProcessMeasureInterval{RowID: int64(i + 1), FilterID: integer(1), IPID: integer(i + 1), StartNS: integer(0), DurationNS: integer(1000000000), Value: integer(120), Name: "H:PreferredFrameRate", NameKnown: true, PID: &pid, ProcessName: strings.Repeat("n", tc.nameBytes) + strconv.Itoa(i), OwnerStatus: "known"}
				line, err := tracewire.FormatProcessMeasureInterval(r)
				if err != nil {
					t.Fatal(err)
				}
				source.WriteString(line + "\n")
			}
			path := filepath.Join(t.TempDir(), "capture.systrace")
			if err := os.WriteFile(path, []byte(source.String()), 0600); err != nil {
				t.Fatal(err)
			}
			ctx, _, _ := hmc17NamedPathContext(t)
			args, _ := json.Marshal(map[string]any{"source": "path", "path": path, "view": "preferred_frame_rate", "time_start": 0, "time_end": 1, "limit": 32})
			out, err := (&TraceQuery{}).Execute(ctx, args)
			if err != nil || !out.Success {
				t.Fatalf("public budget query: %v %s", err, out.Summary)
			}
			found := false
			for _, r := range out.Observations {
				if r.Predicate != TracePreferredFrameRatePredicate {
					continue
				}
				p, ok := types.DecodeRuntimeMeasurementPublication(r)
				if !ok {
					t.Fatal("budget destroyed complete publication")
				}
				found = true
				raw, _ := json.Marshal(p)
				if len(raw) > 64<<10 {
					t.Fatal("receipt exceeds byte cap")
				}
				kept := len(p.Tables[0].Rows)
				if (kept == 0) != tc.empty || kept >= tc.owners {
					t.Fatalf("unexpected retained-series budget %d/%d", kept, tc.owners)
				}
				for _, table := range p.Tables {
					if len(table.Rows) != kept || !strings.Contains(strings.Join(table.Notes, " "), "匹配序列"+strconv.Itoa(tc.owners)) || !strings.Contains(strings.Join(table.Notes, " "), "另省略"+strconv.Itoa(tc.owners-kept)) {
						t.Fatal("three-table or omission contract drift", table.View)
					}
					for _, row := range table.Rows {
						if !strings.Contains(row[1], strings.Repeat("n", tc.nameBytes)) {
							t.Fatal("name truncated into another owner")
						}
					}
				}
				for _, row := range p.Tables[0].Rows {
					if row[4] != "1000000000" {
						t.Fatal("budget changed denominator")
					}
				}
			}
			if !found {
				t.Fatal("receipt dropped instead of bounded empty disclosure")
			}
		})
	}
}
