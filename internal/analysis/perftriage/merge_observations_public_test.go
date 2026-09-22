package perftriage_test

import (
	"encoding/json"
	"reflect"
	"testing"

	"github.com/hanchaoqun/codrax/internal/analysis/perftriage"
	"github.com/hanchaoqun/codrax/internal/types"
)

// The public merge entry point must preserve complete observation records.
// Equal measurements do not establish equal origin or semantic authority.
func TestMergePerfBundlesPublicPreservesObservationRoster(t *testing.T) {
	model := types.PerfObservation{Authority: types.PerfObservationAuthorityPreTriageModelExtraction, Kind: "span", Subject: "GC:Collect", Summary: "8ms interval", Evidence: "capture-A segment-1 lines 5-9", LineStart: 5, LineEnd: 9, StartTsMs: 100, EndTsMs: 108, DurationMs: 8, Tags: []string{"gc", "thread-42"}, Confidence: 0.6}
	validator := model
	validator.Authority, validator.Evidence = types.PerfObservationAuthorityDeterministicValidator, "capture-B segment-1 lines 5-9"
	unknown := model
	unknown.Authority, unknown.Evidence = "", "capture-A segment-2 lines 5-9"
	a := &types.PerfBundle{Meta: types.PerfMeta{Source: "hitrace"}, Observations: []types.PerfObservation{model, validator}}
	b := &types.PerfBundle{Meta: types.PerfMeta{Source: "atrace"}, Observations: []types.PerfObservation{unknown, model}}
	for _, tc := range []struct {
		name  string
		parts []*types.PerfBundle
		want  []types.PerfObservation
	}{
		{"two_parts", []*types.PerfBundle{a, b}, []types.PerfObservation{model, validator, unknown, model}},
		{"nil_interleaved", []*types.PerfBundle{nil, a, nil, b, nil}, []types.PerfObservation{model, validator, unknown, model}},
		{"reverse_order", []*types.PerfBundle{b, a}, []types.PerfObservation{unknown, model, model, validator}},
		{"repeated_part_is_not_origin_dedup", []*types.PerfBundle{a, a, b}, []types.PerfObservation{model, validator, model, validator, unknown, model}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			before := perfMergeInputJSON(t, tc.parts)
			got := perftriage.MergePerfBundles(tc.parts, 1000)
			if got == nil {
				t.Fatal("nonempty observation roster disappeared")
			}
			if !reflect.DeepEqual(got.Observations, tc.want) {
				t.Errorf("complete observation roster/order changed: got=%+v want=%+v", got.Observations, tc.want)
			}
			if !got.HasStructuredObservations() || !got.IsExternalSource() {
				t.Error("observation-only bundle lost its existing typed runtime presence")
			}
			for i, row := range got.Observations {
				if i < len(tc.want) && row.IsNavigationOnly() != tc.want[i].IsNavigationOnly() {
					t.Errorf("authority changed at row %d", i)
				}
			}
			if after := perfMergeInputJSON(t, tc.parts); after != before {
				t.Fatal("merge mutated its input bundles")
			}
		})
	}
}

func TestMergePerfBundlesPublicCardinalityUsesOneDerivation(t *testing.T) {
	for _, parts := range [][]*types.PerfBundle{nil, {}, {nil}, {nil, nil, nil}} {
		if got := perftriage.MergePerfBundles(parts, 100); got != nil {
			t.Errorf("no nonnil input should remain nil, got %+v", got)
		}
	}
	in := &types.PerfBundle{
		Meta:         types.PerfMeta{Source: "hitrace", Signals: []string{"z", "a", "z"}},
		Observations: []types.PerfObservation{{Authority: types.PerfObservationAuthorityDeterministicValidator, Kind: "span", Subject: "GC", DurationMs: 8}},
		Janks:        []types.PerfJank{{StartTsMs: 5, DurationMs: 20, TriggerSpan: "Draw"}},
		Stalls:       []types.PerfStall{{StartTsMs: 5, DurationMs: 20, Symbol: "Widget.draw", File: "src/Widget.ets", Kind: "io"}},
		Residue:      []string{"tail"}, Coverage: 0.125, Entities: []string{"stale entity"}, ResolvedFiles: []string{"stale.ets"}, IntentHint: "stale",
	}
	before := perfMergeInputJSON(t, []*types.PerfBundle{in})
	for _, rawBytes := range []int{0, 100, 200} {
		single := perftriage.MergePerfBundles([]*types.PerfBundle{in}, rawBytes)
		padded := perftriage.MergePerfBundles([]*types.PerfBundle{nil, in, nil}, rawBytes)
		if single == nil || padded == nil {
			t.Fatal("nonnull input disappeared")
		}
		if single == in {
			t.Error("single merge returned caller-owned bundle instead of a derived result")
		}
		if !reflect.DeepEqual(single, padded) {
			t.Errorf("one part and one nonnil part derive differently: single=%+v padded=%+v", single, padded)
		}
		wantCoverage := 1.0
		if rawBytes > 0 {
			wantCoverage = 1 - float64(len("tail"))/float64(rawBytes)
		}
		// This pins the existing residue ratio only, not scanned trace coverage.
		if single.Coverage != wantCoverage {
			t.Errorf("single merge retained stale residue ratio: got=%v want=%v", single.Coverage, wantCoverage)
		}
		if single.IntentHint != "performance" || !reflect.DeepEqual(single.ResolvedFiles, []string{"src/Widget.ets"}) || !reflect.DeepEqual(single.Entities, []string{"Draw", "Widget.draw", "io"}) || !reflect.DeepEqual(single.Meta.Signals, []string{"a", "z"}) {
			t.Errorf("single Layer-4 derivation missing: %+v", single)
		}
	}
	if after := perfMergeInputJSON(t, []*types.PerfBundle{in}); after != before {
		t.Fatal("derivation overwrote original values")
	}
}

func TestMergePerfBundlesPublicPreservesBugClassSet(t *testing.T) {
	race := types.DetectedBugClass{Class: types.BugClassRace, HumanZh: "数据竞争", HumanEn: "data race", MatchedSignature: "WARNING: DATA RACE", SourceLang: "go"}
	otherRace := race
	otherRace.MatchedSignature, otherRace.SourceLang = "ThreadSanitizer: data race", "tsan"
	deadlock := types.DetectedBugClass{Class: types.BugClassDeadlock, HumanZh: "死锁", HumanEn: "deadlock", MatchedSignature: "all goroutines are asleep", SourceLang: "go"}
	a := &types.PerfBundle{Meta: types.PerfMeta{BugClasses: []types.DetectedBugClass{race, deadlock}}}
	b := &types.PerfBundle{Meta: types.PerfMeta{BugClasses: []types.DetectedBugClass{otherRace}}}
	for _, tc := range []struct {
		name  string
		parts []*types.PerfBundle
		want  []types.DetectedBugClass
	}{
		{"single_retains_metadata", []*types.PerfBundle{a}, []types.DetectedBugClass{race, deadlock}},
		{"class_set_first_evidence", []*types.PerfBundle{a, nil, b}, []types.DetectedBugClass{race, deadlock}},
		{"reversed_first_evidence", []*types.PerfBundle{b, a}, []types.DetectedBugClass{otherRace, deadlock}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			before := perfMergeInputJSON(t, tc.parts)
			got := perftriage.MergePerfBundles(tc.parts, 100)
			if got == nil || !reflect.DeepEqual(got.Meta.BugClasses, tc.want) {
				t.Errorf("bug-class set/order/evidence changed: got=%+v want=%+v", got, tc.want)
			}
			if after := perfMergeInputJSON(t, tc.parts); after != before {
				t.Fatal("bug-class merge mutated inputs")
			}
		})
	}
}

func TestMergePerfBundlesPublicExistingSpanPoliciesRemain(t *testing.T) {
	frame := types.PerfFrame{FrameNo: 3, TsMs: 10, DurationMs: 20, Janky: true, JankAuthority: types.PerfObservationAuthorityPreTriageModelExtraction}
	jank := types.PerfJank{StartTsMs: 10, DurationMs: 20, TriggerSpan: "Draw", VerdictAuthority: types.PerfObservationAuthorityPreTriageModelExtraction}
	stall := types.PerfStall{Authority: types.PerfObservationAuthorityPreTriageModelExtraction, StartTsMs: 10, DurationMs: 20, Symbol: "Widget.draw", File: "src/Widget.ets"}
	stronger := stall
	stronger.Authority = types.PerfObservationAuthorityDeterministicValidator
	longest := &types.PerfStartup{Mode: "cold", AppLaunchMs: 2000}
	a := &types.PerfBundle{Meta: types.PerfMeta{Source: "hitrace", AppPID: 8, DurationMs: 30, Summary: "first"}, Frames: []types.PerfFrame{frame}, Janks: []types.PerfJank{jank}, Stalls: []types.PerfStall{stall}, Startup: &types.PerfStartup{AppLaunchMs: 1000}, Residue: []string{"same"}}
	b := &types.PerfBundle{Meta: types.PerfMeta{Source: "atrace", AppPID: 9, DurationMs: 50, Summary: "second"}, Frames: []types.PerfFrame{frame, {FrameNo: 4, TsMs: 40, DurationMs: 5}}, Janks: []types.PerfJank{jank}, Stalls: []types.PerfStall{stronger}, Startup: longest, Residue: []string{"same", "tail"}}
	before := perfMergeInputJSON(t, []*types.PerfBundle{a, b})
	got := perftriage.MergePerfBundles([]*types.PerfBundle{a, b}, 100)
	if !reflect.DeepEqual(got.Frames, []types.PerfFrame{frame, b.Frames[1]}) || !reflect.DeepEqual(got.Janks, []types.PerfJank{jank}) || !reflect.DeepEqual(got.Stalls, []types.PerfStall{stronger}) {
		t.Fatalf("existing span dedup/authority rules changed: %+v", got)
	}
	if got.Startup != longest || got.Meta.Source != "hitrace" || got.Meta.AppPID != 8 || got.Meta.DurationMs != 50 || got.Meta.Summary != "first; second" || !reflect.DeepEqual(got.Residue, []string{"same", "tail"}) || got.Coverage != 0.92 {
		t.Errorf("existing metadata/startup/residue merge changed: %+v", got)
	}
	if after := perfMergeInputJSON(t, []*types.PerfBundle{a, b}); after != before {
		t.Fatal("existing span merge mutated inputs")
	}
}

func perfMergeInputJSON(t *testing.T, parts []*types.PerfBundle) string {
	t.Helper()
	data, err := json.Marshal(parts)
	if err != nil {
		t.Fatal(err)
	}
	return string(data)
}
