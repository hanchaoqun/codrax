package tool

import (
	"encoding/json"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/analysis/tracefinding"
	"github.com/hanchaoqun/codrax/internal/types"
)

// Native evidence, not manufactured ranked seats: the target enters Running
// at 2.020020, outside the requested 2.000000..2.020000 interval. Exploratory
// queries genuinely measured their own .480/.380 ms; neither may be relabeled
// as a measurement of the narrower request, erased, or numerically clipped.
func selfRunningScopePublicTrace(t *testing.T) string {
	t.Helper()
	data, err := os.ReadFile("../../eval/cases/trace_query_wakeup_background_demotion.case")
	if err != nil {
		t.Fatal(err)
	}
	_, rest, opened := strings.Cut(string(data), "HTRACE='")
	raw, _, closed := strings.Cut(rest, "\n'\n")
	if !opened || !closed || !strings.Contains(raw, "2.020020: sched_switch:") {
		t.Fatal("background case must still provide the physical out-of-window switch-in")
	}
	return raw + "\n"
}

func selfRunningScopePublicQuery(t *testing.T, ctx *types.BusContext, path string, end float64) {
	t.Helper()
	for _, view := range []string{"window_stats", "wakeup_chain", "root_cause_rank"} {
		result := businessRefTestQuery(t, ctx, map[string]any{
			"source": "path", "path": path, "view": view, "pid": 100,
			"time_start": 2.0, "time_end": end, "max_depth": 8, "limit": 32,
			"trace_flavor": "harmony_hitrace",
		})
		if !result.Success || len(result.Observations) == 0 {
			t.Fatalf("native %s failed: %s", view, result.Summary)
		}
	}
}

func selfRunningScopePublicPath(t *testing.T, path string) string {
	t.Helper()
	resolved, err := filepath.EvalSymlinks(path)
	if err != nil {
		t.Fatal(err)
	}
	return resolved
}

// JSON pins are deliberate: RED compiles against the old projection without
// the optional provenance fields, and also covers their serialized contract.
func selfRunningScopePublicOrigins(t *testing.T, p types.TraceCausalProjection, path string, expected map[float64]float64) {
	t.Helper()
	data, err := json.Marshal(p.SelfRunningFoldUnmeasured)
	if err != nil {
		t.Fatal(err)
	}
	var rows []struct {
		Subject string                                  `json:"subject"`
		Running float64                                 `json:"running_ms"`
		Unknown float64                                 `json:"unknown_ms"`
		Source  *types.ObservationSourceRef             `json:"query_source_ref"`
		Window  *types.TraceCausalProjectionQueryWindow `json:"selected_window"`
	}
	if err := json.Unmarshal(data, &rows); err != nil {
		t.Fatal(err)
	}
	if len(rows) != len(expected) {
		t.Errorf("distinct native measurement scopes were merged or duplicated: got %s, want %v", data, expected)
	}
	seen := map[float64]bool{}
	for _, row := range rows {
		if row.Source == nil || row.Window == nil {
			t.Errorf("native disclosure lost its own source/query/window receipt: %s", data)
			continue
		}
		want, ok := expected[row.Window.EndTs]
		if !ok || seen[row.Window.EndTs] || row.Window.StartTs != 2 || row.Subject != "app-100" || math.Abs(row.Running-want) > 1e-9 || row.Unknown != row.Running {
			t.Errorf("native scope or unscaled values changed: %+v, want %v", row, expected)
		}
		seen[row.Window.EndTs] = true
		ref := row.Source
		if ref.Path != path || ref.CaptureIdentityPath != path || ref.QueryScopeID == "" || !ref.QueryWindowKnown || ref.QueryWindowStartTs != 2 || ref.QueryWindowEndTs != row.Window.EndTs || ref.QueryTargetPID != 100 {
			t.Errorf("disclosure acquired another capture/target/query receipt: %+v", ref)
		}
	}
}

func selfRunningScopePublicFace(t *testing.T, text, lang string, expected map[float64]float64) {
	t.Helper()
	flat := partsplitSquash(text)
	for end, value := range expected {
		want := fmt.Sprintf("app-100 查询窗 2.000000..%.6fs running %.3fms", end, value)
		old := fmt.Sprintf("app-100 窗内 running %.3fms", value)
		if lang == "en" {
			want = fmt.Sprintf("app-100 query window 2.000000..%.6fs: ran %.3fms", end, value)
			old = fmt.Sprintf("app-100 ran %.3fms in-window", value)
		}
		if !strings.Contains(flat, partsplitSquash(want)) || strings.Contains(flat, partsplitSquash(old)) {
			t.Errorf("supplementary measurement must disclose its original scope, not inherit the board window; want %q:\n%s", want, text)
		}
	}
}

func selfRunningScopePublicPrincipal(t *testing.T, ctx *types.BusContext, p types.TraceCausalProjection) string {
	t.Helper()
	if p.WindowStartTs != 2 || p.WindowEndTs != 2.020 || p.TargetStateAccount == nil || p.TargetStateAccount.RunningMS != 0 || p.TargetStateAccount.RunnableMS != 0 || p.TargetStateAccount.SleepMS != 20 || p.TargetStateAccount.TotalMS != 20 {
		t.Fatalf("native requested-window account changed: window %.6f..%.6f account %+v", p.WindowStartTs, p.WindowEndTs, p.TargetStateAccount)
	}
	ledger := types.CompileObservationLedger(types.ObservationLedgerInputFromBusContext(ctx, types.ObservationExtractLedgerEvidenceLimit))
	set := types.CompileTraceCausalProjectionSet(ledger)
	contract, err := tracefinding.CompileCandidateContract(ledger, set, tracefinding.SeatFrameCausalityAuthority{})
	if err != nil {
		t.Fatal(err)
	}
	contract.RootCauseReportEnabled = true
	selection := &types.TraceRootCauseReportV2{SchemaVersion: types.TraceRootCauseReportSchemaVersion}
	for _, candidate := range contract.Candidates {
		if candidate.Decision.Token.FixDirection == "io_dependency" || candidate.Decision.Token.Token == "priority_inversion_candidate" {
			selection.RootCauses = append(selection.RootCauses, &types.TraceRootCauseItemV2{CandidateID: candidate.Decision.CandidateID})
		}
	}
	bound, err := tracefinding.BindRootCauseReportSelection(selection, contract)
	if err != nil || bound == nil || len(bound.RootCauses) != 4 {
		t.Fatalf("native four principal sidecar receipts must stay selectable: %v %+v", err, bound)
	}
	for _, cause := range bound.RootCauses {
		if cause.ThreadName == "app-100" || cause.ThreadName == "logger-900" || cause.ImpactSeconds == nil || (math.Abs(*cause.ImpactSeconds-.011) > 1e-9 && math.Abs(*cause.ImpactSeconds-.001) > 1e-9) {
			t.Fatalf("supplementary measurement acquired root-cause authority or changed attribution: %+v", cause)
		}
	}
	bytes, err := json.Marshal(bound)
	if err != nil {
		t.Fatal(err)
	}
	return string(bytes)
}

func TestSelfRunningScopePublicNativeWindows(t *testing.T) {
	for _, lang := range []string{"zh", "en"} {
		for _, tc := range []struct {
			name   string
			ends   []float64
			values map[float64]float64
			finite bool
		}{
			{"wide_then_exact", []float64{2.0205, 2.020}, map[float64]float64{2.0205: .480}, false},
			{"exact_then_wide", []float64{2.020, 2.0205}, map[float64]float64{2.0205: .480}, false},
			{"two_wide_measurements", []float64{2.0205, 2.0204, 2.020}, map[float64]float64{2.0205: .480, 2.0204: .380}, false},
			{"two_wide_reverse", []float64{2.020, 2.0204, 2.0205}, map[float64]float64{2.0205: .480, 2.0204: .380}, false},
			{"exact_only_no_disclosure", []float64{2.020}, nil, false},
			{"finite_no_causal_expansion", []float64{2.0205, 2.020}, map[float64]float64{2.0205: .480}, true},
		} {
			t.Run(lang+"/"+tc.name, func(t *testing.T) {
				ctx, path := businessRefTestContext(t, selfRunningScopePublicTrace(t))
				path = selfRunningScopePublicPath(t, path)
				optimizationCaliberPublicRequest(ctx, path, lang, "app-100", 100, 2, 2.020, tc.finite)
				for _, end := range tc.ends {
					selfRunningScopePublicQuery(t, ctx, path, end)
				}
				_, before := optimizationCaliberPublicSnapshot(t, ctx)
				if len(before.Projections) != 1 {
					t.Fatalf("single capture must remain one projection: %d", len(before.Projections))
				}
				principal := selfRunningScopePublicPrincipal(t, ctx, before.Projections[0])
				text, after := optimizationCaliberPublicPublish(t, ctx, path, lang, tc.finite)
				selfRunningScopePublicOrigins(t, after.Projections[0], path, tc.values)
				if !tc.finite {
					selfRunningScopePublicFace(t, text, lang, tc.values)
				}
				if got := selfRunningScopePublicPrincipal(t, ctx, after.Projections[0]); got != principal {
					t.Error("publication/no-op patch changed the original four root-cause JSON entries")
				}
			})
		}
	}
}

func TestSelfRunningScopePublicNativeCaptureIsolation(t *testing.T) {
	for _, lang := range []string{"zh", "en"} {
		for _, reverse := range []bool{false, true} {
			t.Run(fmt.Sprintf("%s/reverse=%t", lang, reverse), func(t *testing.T) {
				ctx, first := businessRefTestContext(t, selfRunningScopePublicTrace(t))
				_, second := businessRefTestContext(t, selfRunningScopePublicTrace(t))
				first, second = selfRunningScopePublicPath(t, first), selfRunningScopePublicPath(t, second)
				optimizationCaliberPublicRequest(ctx, first, lang, "app-100", 100, 2, 2.020, false)
				ctx.RuntimeArtifactPreflight.Artifacts = append(ctx.RuntimeArtifactPreflight.Artifacts, types.RuntimeArtifactPreflightArtifact{Kind: "trace", Source: second, Carrier: "path"})
				paths := []string{first, second}
				if reverse {
					paths[0], paths[1] = paths[1], paths[0]
				}
				for _, path := range paths {
					end := 2.0205
					if path == second {
						end = 2.0204
					}
					selfRunningScopePublicQuery(t, ctx, path, end)
					selfRunningScopePublicQuery(t, ctx, path, 2.020)
				}
				text, set := optimizationCaliberPublicPublish(t, ctx, first, lang, false)
				if len(set.Projections) != 2 {
					t.Fatalf("two physical captures merged: %d", len(set.Projections))
				}
				labels := map[string]bool{}
				for _, p := range set.Projections {
					end, value := 2.0205, .480
					if p.ArtifactPath == second {
						end, value = 2.0204, .380
					} else if p.ArtifactPath != first {
						t.Fatalf("foreign capture: %s", p.ArtifactPath)
					}
					selfRunningScopePublicOrigins(t, p, p.ArtifactPath, map[float64]float64{end: value})
					if p.ArtifactLabel == "" || labels[p.ArtifactLabel] || !strings.Contains(text, p.ArtifactLabel) {
						t.Errorf("same basename captures lack distinct visible artifact labels: %q", p.ArtifactLabel)
					}
					labels[p.ArtifactLabel] = true
					heading := "## Trace 因果投影 — " + p.ArtifactLabel + "\n"
					if lang == "en" {
						heading = "## Trace Causal Projection — " + p.ArtifactLabel + "\n"
					}
					_, section, present := strings.Cut(text, heading)
					if !present {
						t.Fatalf("missing artifact-owned chart heading %q", heading)
					}
					section, _, _ = strings.Cut(section, "\n## ")
					selfRunningScopePublicFace(t, section, lang, map[float64]float64{end: value})
					foreign := .380
					if value == .380 {
						foreign = .480
					}
					if strings.Contains(section, fmt.Sprintf("%.3fms", foreign)) {
						t.Errorf("artifact-owned chart contains another capture's running value: %q", p.ArtifactLabel)
					}
				}
				selfRunningScopePublicFace(t, text, lang, map[float64]float64{2.0205: .480, 2.0204: .380})
			})
		}
	}
}

// Legacy typed-carrier control, explicitly not a new native measurement.
func TestSelfRunningScopePublicLegacyUnknownRange(t *testing.T) {
	for _, lang := range []string{"zh", "en"} {
		t.Run(lang, func(t *testing.T) {
			p := selfrunDiscProjection()
			before, _ := json.Marshal(p)
			model := buildRuntimeTraceProjTreeModel(p, newRuntimeTraceCausalProjectionEvidenceIndex(), lang == "zh")
			text := partsplitSquash(runtimeTraceProjElimOverviewFence(p, model, lang == "zh"))
			want := "app-100 查询范围未明确 running 19.800ms"
			if lang == "en" {
				want = "app-100 query range unspecified: ran 19.800ms"
			}
			if !strings.Contains(text, partsplitSquash(want)) {
				t.Errorf("legacy range must remain unknown, with original measured value: %s", text)
			}
			after, _ := json.Marshal(p)
			if !reflect.DeepEqual(before, after) {
				t.Error("render filled a legacy receipt from the parent board")
			}
		})
	}
}

// Renderer-only malformed-carrier defense: no new observation authority.
func TestSelfRunningScopePublicZeroAndInvalidWindowDisplay(t *testing.T) {
	for _, lang := range []string{"zh", "en"} {
		for _, tc := range []struct {
			name       string
			start, end float64
			known      bool
		}{
			{"zero", 0, .02, true}, {"reversed", 2, 1, false}, {"zero width", 0, 0, false},
			{"nonfinite start", math.NaN(), 2, false}, {"nonfinite end", 0, math.Inf(1), false},
		} {
			t.Run(lang+"/"+tc.name, func(t *testing.T) {
				p := selfrunDiscProjection()
				p.SelfRunningFoldUnmeasured[0].SelectedWindow = &types.TraceCausalProjectionQueryWindow{StartTs: tc.start, EndTs: tc.end}
				model := buildRuntimeTraceProjTreeModel(p, newRuntimeTraceCausalProjectionEvidenceIndex(), lang == "zh")
				text := partsplitSquash(runtimeTraceProjElimOverviewFence(p, model, lang == "zh"))
				want := "app-100 查询范围未明确 running 19.800ms"
				if lang == "en" {
					want = "app-100 query range unspecified: ran 19.800ms"
				}
				if tc.known {
					want = "app-100 查询窗 0.000000..0.020000s running 19.800ms"
					if lang == "en" {
						want = "app-100 query window 0.000000..0.020000s: ran 19.800ms"
					}
				}
				if !strings.Contains(text, partsplitSquash(want)) {
					t.Errorf("selected-window display guessed or rejected zero: want %q\n%s", want, text)
				}
			})
		}
	}
}
