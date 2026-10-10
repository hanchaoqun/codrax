package tool

import (
	"bytes"
	"encoding/json"
	"html"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/render"
	"github.com/hanchaoqun/codrax/internal/types"
)

// Independent public-entry coverage includes producer-private population
// metadata, which raw measurements (without MemberSet) cannot exercise.
func hmc221CPUComposition(t *testing.T, singleFirst bool) (*types.BusContext, types.ToolResult, types.ToolResult, string) {
	t.Helper()
	ctx, _, _ := hmc17NamedPathContext(t)
	body, err := os.ReadFile("../../eval/fixtures/hmosperf_cpu_state_frequency/events.systrace")
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(t.TempDir(), "cpu.systrace")
	if err := os.WriteFile(path, body, 0600); err != nil {
		t.Fatal(err)
	}
	path, err = filepath.EvalSymlinks(path)
	if err != nil {
		t.Fatal(err)
	}
	start, end := 1.0, 1.04
	rm := nativeFactDisplayRequest(types.RequestedAnswerDimensionObservedValue)
	rm.Language, rm.PerfTrace = "en", &types.PerfBundle{}
	rm.RuntimeArtifactScopeProfile = &types.RuntimeArtifactScopeProfile{RequestedScope: types.RuntimeArtifactScopeExplicitWindow,
		TimeStart: &start, TimeEnd: &end, SourceQuote: "1 to 1.04 seconds"}
	ctx.AnalysisIR = &types.AnalysisIR{RequestModel: rm}
	ctx.Mutable.SetRequestModel(rm)
	ctx.RuntimeArtifactPreflight = types.RuntimeArtifactPreflightProfile{Active: true, Artifacts: []types.RuntimeArtifactPreflightArtifact{{Kind: "trace", Source: path, Carrier: "request_path"}}}
	args, _ := json.Marshal(map[string]any{"source": "path", "path": path, "view": "cpu_state_frequency", "time_start": start, "time_end": end})
	pairArgs, _ := json.Marshal(map[string]any{"comparison": map[string]any{"baseline": json.RawMessage(args)}})
	query := func(raw json.RawMessage) types.ToolResult {
		result, err := (&TraceQuery{}).Execute(ctx, raw)
		if err != nil || !result.Success {
			t.Fatalf("actual public query: %v / %s", err, result.Summary)
		}
		return result
	}
	var single, pair types.ToolResult
	if singleFirst {
		single, pair = query(args), query(pairArgs)
	} else {
		pair, single = query(pairArgs), query(args)
	}
	return ctx, single, pair, path
}

func hmc221CompositionChoices(ctx *types.BusContext, results ...types.ToolResult) []types.RuntimeMeasurementTable {
	ctx.Mutable.SetTurnAArtifacts(types.TurnAArtifacts{ToolResults: results})
	return types.BuildAnswerSemanticViewForBusContext(ctx).RuntimeMeasurementContract.Choices()
}

func TestHMC221NativeMeasurementCompositionPublicScopeAndOrder(t *testing.T) {
	for _, singleFirst := range []bool{false, true} {
		for _, scope := range []string{"single_source", "multiple_sources", "no_preflight", "other_source"} {
			t.Run(scope+map[bool]string{false: "/pair_first", true: "/single_first_memo"}[singleFirst], func(t *testing.T) {
				ctx, single, pair, path := hmc221CPUComposition(t, singleFirst)
				report, ok := pair.RuntimeMeasurementPair.Report()
				if !ok || report.Sides[0].Status != "available" || report.Sides[1].Status != "not_requested" {
					t.Fatalf("ordinary call order lost independently accepted source: valid=%t baseline=%s current=%s", ok, report.Sides[0].Status, report.Sides[1].Status)
				}
				switch scope {
				case "multiple_sources":
					ctx.RuntimeArtifactPreflight.Artifacts = append(ctx.RuntimeArtifactPreflight.Artifacts, types.RuntimeArtifactPreflightArtifact{Kind: "trace", Source: path + ".other", Carrier: "request_path"})
				case "no_preflight":
					ctx.RuntimeArtifactPreflight = types.RuntimeArtifactPreflightProfile{}
				case "other_source":
					ctx.RuntimeArtifactPreflight.Artifacts[0].Source += ".other"
				}
				original := map[types.RuntimeMeasurementView]types.RuntimeMeasurementTable{}
				for _, table := range hmc221CompositionChoices(ctx, single) {
					original[table.View] = table
				}
				if len(original) != 3 {
					t.Fatal("single producer lost native view prerequisites")
				}
				for _, results := range [][]types.ToolResult{{pair}, {single, pair}, {pair, single}} {
					choices := hmc221CompositionChoices(ctx, results...)
					if len(choices) != 4 {
						t.Fatalf("lost or duplicated whole producer publication: got %d views, want 4", len(choices))
					}
					coverage := 0
					for _, table := range choices {
						if table.ObservationID != report.ID && !reflect.DeepEqual(table, original[table.View]) {
							t.Fatal("composition changed native cells, source metadata or private scope")
						}
						if table.CoversMemberSet(&ctx.AnalysisIR.RequestModel) {
							coverage++
						}
					}
					if (coverage == 1) != (scope == "single_source") || coverage > 1 {
						t.Fatalf("private scope bypassed consumer qualification: scope=%s coverage=%d", scope, coverage)
					}
				}
				blocks := []any{map[string]any{"id": "lead", "kind": "summary", "text": "CPU residency is not a response root cause."}}
				for i, table := range hmc221CompositionChoices(ctx, single, pair) {
					if table.View == types.RuntimeMeasurementDistribution || table.ObservationID == report.ID {
						blocks = append(blocks, map[string]any{"id": string(rune('a' + i)), "kind": "table", "runtime_measurement": map[string]any{"observation_id": table.ObservationID, "view": table.View}})
					}
				}
				if out := b1659bExecuteAnswer(t, ctx, map[string]any{"blocks": blocks}, false); !out.Success {
					t.Fatal(out.Summary)
				}
				visible := html.UnescapeString(render.RenderAnswerDocument(ctx.Mutable.AnswerDocumentV2(), "en"))
				for _, value := range []string{"CPU0", "CPU1", "CPU2", "baseline", "current", "not_requested"} {
					if !strings.Contains(visible, value) {
						t.Errorf("accepted facts did not survive emit/render: %q", value)
					}
				}
			})
		}
	}
}

func TestHMC221NativeMeasurementCompositionRejectsWholeConflictingPublication(t *testing.T) {
	for _, conflict := range []string{"private_authority", "missing_view", "source"} {
		t.Run(conflict, func(t *testing.T) {
			ctx, single, pair, _ := hmc221CPUComposition(t, false)
			for i := range single.Observations {
				r := &single.Observations[i]
				p, ok := types.DecodeRuntimeMeasurementPublication(*r)
				if !ok {
					continue
				}
				switch conflict {
				case "private_authority":
					r.ClaimAuthority = types.ObservationClaimAuthorityModelInference
				case "missing_view":
					p.Tables = p.Tables[:1]
				case "source":
					r.SourceRef.Path += ".other"
					p.Source = r.SourceRef
				}
				raw, _ := json.Marshal(p)
				r.RichNotes = []string{types.TraceNoteKeyRuntimeMeasurement + "=" + string(raw)}
			}
			choices := hmc221CompositionChoices(ctx, single, pair)
			report, _ := pair.RuntimeMeasurementPair.Report()
			if len(choices) != 1 || choices[0].ObservationID != report.ID {
				t.Fatalf("conflict left sibling native views selectable: %+v", choices)
			}
		})
	}
}

func TestHMC221NativeMeasurementCompositionPrivateReceiptLifetime(t *testing.T) {
	for _, boundary := range []string{"copy_out", "foreign_run", "reset_run", "json_replay", "source_changed"} {
		t.Run(boundary, func(t *testing.T) {
			ctx, _, pair, path := hmc221CPUComposition(t, true)
			before, _ := pair.RuntimeMeasurementPair.Report()
			if before.Sides[0].Status != "available" || len(hmc221CompositionChoices(ctx, pair)) != 4 {
				t.Fatal("fixture never accepted its memo-backed source")
			}
			want := 1
			switch boundary {
			case "copy_out":
				copyReport, _ := pair.RuntimeMeasurementPair.Report()
				copyReport.Sides[0].Publications[0].Tables[0].Rows[0][0] = "mutated public row"
				for _, table := range copyReport.Sides[0].Publications[0].Tables {
					if table.MemberSet != nil {
						table.MemberSet.RowIDs[0] = "mutated member"
					}
				}
				after, _ := pair.RuntimeMeasurementPair.Report()
				if !reflect.DeepEqual(before, after) {
					t.Fatal("report copy aliases private rows or membership")
				}
				want = 4
			case "foreign_run":
				ctx.Mutable = types.NewMutableState("unrelated run")
				ctx.Mutable.SetRequestModel(ctx.AnalysisIR.RequestModel)
			case "reset_run":
				ctx.Mutable.ResetTurnAArtifacts()
			case "json_replay":
				raw, _ := json.Marshal(pair)
				// Historical replay is a fresh value, not mutation of a live receipt.
				var historical types.ToolResult
				if err := json.Unmarshal(raw, &historical); err != nil {
					t.Fatal(err)
				}
				pair, want = historical, 0
			case "source_changed":
				body, err := os.ReadFile(path)
				if err != nil || len(body) == 0 {
					t.Fatal("missing source", err)
				}
				replacement := path + ".replacement"
				if err := os.WriteFile(replacement, bytes.Clone(body), 0600); err != nil {
					t.Fatal(err)
				}
				if err := os.Rename(replacement, path); err != nil {
					t.Fatal(err)
				}
			}
			if got := len(hmc221CompositionChoices(ctx, pair)); got != want {
				t.Fatalf("private receipt boundary %s: got %d views, want %d", boundary, got, want)
			}
		})
	}
}
