package agent

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/tool"
	"github.com/hanchaoqun/codrax/internal/types"
)

func filteredSamplingContextHMC242(t *testing.T, direct bool, patterns ...string) (*types.AgentContext, types.ToolResult) {
	t.Helper()
	return filteredSamplingContextInHMC242(t, t.TempDir(), direct, patterns...)
}

// filteredSamplingContextInHMC242 runs the same public filtered query on a
// bundle inside a directory chosen by the caller.
func filteredSamplingContextInHMC242(t *testing.T, dir string, direct bool, patterns ...string) (*types.AgentContext, types.ToolResult) {
	t.Helper()
	ctx, prepared := samplingBundleContextInHMC241(t, dir)
	var path string
	for _, r := range prepared.Observations {
		if p, ok := tool.DecodeTracePerfSamplingTree(r); ok {
			path = p.SourcePath
			if direct {
				path = p.Cohorts[0].SourcePath
			}
			break
		}
	}
	pattern := "symbol=work"
	if len(patterns) == 1 {
		pattern = patterns[0]
	}
	params, _ := json.Marshal(map[string]any{"source": "path", "path": path, "view": "window_stats", "pid": 11, "time_start": 1, "time_end": 2, "pattern": pattern})
	result, err := (&tool.TraceQuery{}).Execute(types.ToolBusContext(ctx, types.AgentExplorer), params)
	if err != nil || !result.Success {
		t.Fatalf("public filtered query: %v %s", err, result.Summary)
	}
	return ctx, result
}

// filteredSamplingPublishedFactHMC242 is one published member joined to the
// native notes its own native_fields_refs resolve to.
type filteredSamplingPublishedFactHMC242 struct {
	DSO, Value, Unit string
	Samples          string
	SampleWeight     string
	ScopeWeight      float64
	ScopeSamples     float64
}

var filteredSamplingNativeNoteHMC242 = regexp.MustCompile(`^native_note_ref="([^"]+)" value=(".*")$`)

// filteredSamplingPublishedFactsHMC242 reads the native section as its two
// typed line kinds. The note table values stay whole, so a fact is compared
// with one exact note and never searched for in text that also prints paths.
func filteredSamplingPublishedFactsHMC242(t *testing.T, part string) (facts []filteredSamplingPublishedFactHMC242, notes map[string]string) {
	t.Helper()
	notes = map[string]string{}
	for _, line := range strings.Split(part, "\n") {
		line = strings.TrimSpace(line)
		if m := filteredSamplingNativeNoteHMC242.FindStringSubmatch(line); m != nil {
			value, err := strconv.Unquote(m[2])
			if err != nil {
				t.Fatalf("native note %s is not one quoted value: %v", m[1], err)
			}
			notes[m[1]] = value
		}
	}
	for _, line := range strings.Split(part, "\n") {
		line = strings.TrimSpace(line)
		if !strings.HasPrefix(line, `{"source_ref":`) {
			continue
		}
		var member struct {
			DSO   string   `json:"dso"`
			Value string   `json:"value"`
			Unit  string   `json:"unit"`
			Refs  []string `json:"native_fields_refs"`
		}
		if err := json.Unmarshal([]byte(line), &member); err != nil {
			t.Fatalf("native member line is not one JSON object: %v", err)
		}
		fact := filteredSamplingPublishedFactHMC242{DSO: member.DSO, Value: member.Value, Unit: member.Unit}
		for _, ref := range member.Refs {
			note, known := notes[ref]
			if !known {
				t.Fatalf("member names unresolved native note %q", ref)
			}
			key, value, _ := strings.Cut(note, "=")
			switch key {
			case "samples":
				fact.Samples = value
			case "sample_weight":
				fact.SampleWeight = value
			case "perf_quality_scope":
				var scope struct {
					Weight  float64 `json:"weight"`
					Samples float64 `json:"samples"`
				}
				if err := json.Unmarshal([]byte(value), &scope); err != nil {
					t.Fatalf("perf_quality_scope note is not JSON: %v", err)
				}
				fact.ScopeWeight, fact.ScopeSamples = scope.Weight, scope.Samples
			}
		}
		facts = append(facts, fact)
	}
	return facts, notes
}

// filteredSamplingExactDSOViolationsHMC242 is the oracle for the dso=/app/A.so
// query: the published members are exactly that library's three cohorts in the
// query window, and no note of the section states another library or another
// member's weight. It returns the violations so the positive cases and the
// widened-filter control share one judge.
func filteredSamplingExactDSOViolationsHMC242(t *testing.T, part string) []string {
	t.Helper()
	facts, notes := filteredSamplingPublishedFactsHMC242(t, part)
	want := map[filteredSamplingPublishedFactHMC242]int{
		{DSO: "/app/A.so", Value: "15", Unit: "sample_weight", Samples: "2", SampleWeight: "15", ScopeWeight: 15, ScopeSamples: 2}:    1,
		{DSO: "/app/A.so", Value: "100", Unit: "sample_weight", Samples: "1", SampleWeight: "100", ScopeWeight: 100, ScopeSamples: 1}: 1,
		{DSO: "/app/A.so", Value: "7", Unit: "sample_weight", Samples: "1", SampleWeight: "7", ScopeWeight: 7, ScopeSamples: 1}:       1,
	}
	var bad []string
	for _, fact := range facts {
		if want[fact] == 0 {
			bad = append(bad, fmt.Sprintf("published member outside the query: %+v", fact))
		}
		want[fact]--
	}
	for fact, n := range want {
		if n > 0 {
			bad = append(bad, fmt.Sprintf("native filtered display lost %+v", fact))
		}
	}
	for ref, note := range notes {
		switch note {
		case "dso=/app/B.so", "sample_weight=900", "sample_weight=20":
			bad = append(bad, fmt.Sprintf("native note %s leaked %s", ref, note))
		}
	}
	sort.Strings(bad)
	return bad
}

// HMC376 re-base: the former guard searched the whole native section for
// "/app/B.so". That section prints the bundle path, so the guard failed when
// the temporary path spelled the literal. The same claims are now read from the
// parsed member lines and the exact note values they reference.
func TestFilteredSamplingExactDSOFinalizerHMC242(t *testing.T) {
	for _, tc := range []struct{ name, sub string }{
		{"default_path", ""},
		// Positive control for the retired section scan: the bundle path itself
		// spells the excluded library.
		{"path_spells_excluded_dso", filepath.Join("app", "B.so")},
	} {
		t.Run(tc.name, func(t *testing.T) {
			dir := t.TempDir()
			if tc.sub != "" {
				dir = filepath.Join(dir, tc.sub)
				if err := os.MkdirAll(dir, 0700); err != nil {
					t.Fatal(err)
				}
			}
			ctx, result := filteredSamplingContextInHMC242(t, dir, false, "dso=/app/A.so")
			ctx.Mutable.SetTurnAArtifacts(types.TurnAArtifacts{ToolResults: []types.ToolResult{result}})
			before, _ := json.Marshal(result.Observations)
			prompt := (&answerDocumentEvaluator{}).BuildInitialInstruction(ctx, nil)
			part := filteredSamplingNativeSectionHMC242(prompt)
			for _, violation := range filteredSamplingExactDSOViolationsHMC242(t, part) {
				t.Error(violation)
			}
			if tc.sub != "" && !strings.Contains(part, "/app/B.so") {
				t.Fatal("path noise no longer reaches the native section; this control is vacuous")
			}
			after, _ := json.Marshal(result.Observations)
			if string(before) != string(after) {
				t.Fatal("display mutated original observations")
			}
			if decided, allowed := types.RuntimeTraceReportShapeAuthority(&ctx.AnalysisIR.RequestModel); !decided || allowed {
				t.Fatal("filtered display gained causal authority")
			}
		})
	}
}

// Negative control: the same oracle must reject a section that really does
// publish the excluded library, which the wider symbol filter produces.
func TestFilteredSamplingExactDSOOracleRejectsAWiderFilterHMC242(t *testing.T) {
	ctx, result := filteredSamplingContextHMC242(t, false, "symbol=work")
	ctx.Mutable.SetTurnAArtifacts(types.TurnAArtifacts{ToolResults: []types.ToolResult{result}})
	part := filteredSamplingNativeSectionHMC242((&answerDocumentEvaluator{}).BuildInitialInstruction(ctx, nil))
	joined := strings.Join(filteredSamplingExactDSOViolationsHMC242(t, part), "\n")
	for _, want := range []string{"DSO:/app/B.so", "native note", "dso=/app/B.so", "sample_weight=20"} {
		if !strings.Contains(joined, want) {
			t.Errorf("oracle did not report %q for the wider filter:\n%s", want, joined)
		}
	}
}

func TestFilteredSamplingPreflightDecorationHMC242(t *testing.T) {
	ctx, result := filteredSamplingContextHMC242(t, false, "dso=/app/A.so")
	var original types.ObservationRecord
	for _, r := range result.Observations {
		if _, valid, _ := tool.TraceQueryFilteredPerfMemberSelection(r); valid {
			original = r
			break
		}
	}
	ctx.RuntimeArtifactPreflight = types.RuntimeArtifactPreflightProfile{Active: true, Artifacts: []types.RuntimeArtifactPreflightArtifact{{Kind: "trace", Source: original.SourceRef.Path, Carrier: "attachment"}}}
	ctx.Mutable.SetTurnAArtifacts(types.TurnAArtifacts{ToolResults: []types.ToolResult{result}})
	ledger := answerDocObservationLedger(ctx)
	decorated := false
	for _, r := range ledger.Records {
		if r.ID == original.ID {
			decorated = r.SourceRef.ToolCallID != "" && r.SourceRef.CaptureIdentityPath != "" && r.SourceRef.ArtifactID != original.SourceRef.ArtifactID
			if _, valid, _ := tool.TraceQueryFilteredPerfMemberSelection(r); !valid {
				t.Fatal("precise preflight decoration invalidated native selection")
			}
		}
	}
	if !decorated {
		t.Fatal("fixture missed actual ledger/preflight source decoration")
	}
	prompt := (&answerDocumentEvaluator{}).BuildInitialInstruction(ctx, nil)
	if !strings.Contains(filteredSamplingNativeSectionHMC242(prompt), `"sample_weight=15"`) {
		t.Fatal("preflight dropped independently selected native fields")
	}
}

func TestFilteredSamplingBudgetPublicationHMC242(t *testing.T) {
	ctx, first := filteredSamplingContextHMC242(t, false)
	var path string
	for _, r := range first.Observations {
		if _, valid, _ := tool.TraceQueryFilteredPerfMemberSelection(r); valid {
			path = r.SourceRef.Path
			break
		}
	}
	results := []types.ToolResult{first}
	for i := 1; i < 12; i++ {
		args, _ := json.Marshal(map[string]any{"source": "path", "path": path, "view": "window_stats", "pid": 11, "time_start": 1, "time_end": 2, "pattern": "symbol=work", "line_end": 20 + i})
		r, err := (&tool.TraceQuery{}).Execute(types.ToolBusContext(ctx, types.AgentExplorer), args)
		if err != nil || !r.Success {
			t.Fatalf("query %d: %v %s", i, err, r.Summary)
		}
		results = append(results, r)
	}
	ctx.Mutable.SetTurnAArtifacts(types.TurnAArtifacts{ToolResults: results})
	ledger := answerDocObservationLedger(ctx)
	selected, _ := answerDocFinalizerObservationRecords(ctx, ledger.Records)
	promptLedger := ledger
	promptLedger.Records = selected
	publication := &answerDocNativePresentationReceipt{}
	text := renderAnswerDocScopedPerfMemberFacts(ctx, ledger, promptLedger, publication)
	if text == "" || len(text) > 32<<10 {
		t.Fatalf("native field budget not exercised: %d bytes", len(text))
	}
	blocked := false
	for key, complete := range publication.queries {
		if strings.HasPrefix(key, "perf_flat_members:") && !complete {
			blocked = true
		}
	}
	if !blocked || publication.completeFor(ctx) {
		t.Fatal("budget-omitted members acquired full publication or removed narrative")
	}
}

func filteredSamplingNativeSectionHMC242(prompt string) string {
	_, part, found := strings.Cut(prompt, "### Recorded sampling hotspot fields")
	if !found {
		return ""
	}
	if next := strings.Index(part, "\n### "); next >= 0 {
		part = part[:next]
	}
	return part
}

// filteredSamplingMemberHMC242 is one member line of the native section. The
// section as a whole also carries source paths, payload references and
// content ids, so member claims are read from these parsed lines and never by
// searching the section for a bare value.
type filteredSamplingMemberHMC242 struct {
	DSO   string `json:"dso"`
	Value string `json:"value"`
}

func filteredSamplingNativeMembersHMC242(t *testing.T, part string) []filteredSamplingMemberHMC242 {
	t.Helper()
	var out []filteredSamplingMemberHMC242
	for _, line := range strings.Split(part, "\n") {
		line = strings.TrimSpace(line)
		if !strings.HasPrefix(line, `{"source_ref":`) {
			continue
		}
		var member filteredSamplingMemberHMC242
		if err := json.Unmarshal([]byte(line), &member); err != nil {
			t.Fatalf("native member line is not one JSON object: %v", err)
		}
		out = append(out, member)
	}
	return out
}

// filteredSamplingMixedMemberViolationsHMC242 is the oracle for "a valid
// sibling neither qualifies an altered member nor is lost with it": the
// published members must be exactly the unaltered ones. It returns the
// violations so the same function serves the positive cases and the tamper
// control.
func filteredSamplingMixedMemberViolationsHMC242(members, valid []filteredSamplingMemberHMC242, alteredValue string) []string {
	var bad []string
	count := map[filteredSamplingMemberHMC242]int{}
	for _, member := range valid {
		count[member]++
	}
	for _, member := range members {
		if member.Value == alteredValue {
			bad = append(bad, fmt.Sprintf("altered member was published: %+v", member))
		}
		count[member]--
	}
	for member, n := range count {
		switch {
		case n > 0:
			bad = append(bad, fmt.Sprintf("valid sibling lost: %+v", member))
		case n < 0:
			bad = append(bad, fmt.Sprintf("member published without a valid receipt: %+v", member))
		}
	}
	return bad
}

// filteredSamplingPathNoiseHMC242 names the bundle directory of the path-noise
// control; it spells the value written into the altered member.
const filteredSamplingPathNoiseHMC242 = "run-424242"

// HMC376 re-base (mixed_member): the former guard searched the native section
// for the altered value. That section prints the bundle path, so the guard
// failed whenever the temporary path spelled the value; it also accepted any
// non-empty section, even one that had lost a valid sibling. The same claim is
// now read from the parsed member lines.
func TestFilteredSamplingFinalizerRequestBoundariesHMC242(t *testing.T) {
	const alteredValue = "424242"
	for _, tc := range []struct{ name, mode, sub string }{
		{"other_thread", "other_thread", ""}, {"process_not_thread", "process_not_thread", ""}, {"other_window", "other_window", ""},
		{"missing_receipt", "missing_receipt", ""}, {"wrong_source", "wrong_source", ""}, {"wrong_window", "wrong_window", ""},
		{"mixed_member", "mixed_member", ""},
		// Positive control for the retired section scan: the bundle path
		// itself spells the altered value.
		{"mixed_member_path_noise", "mixed_member", filteredSamplingPathNoiseHMC242},
	} {
		t.Run(tc.name, func(t *testing.T) {
			dir := t.TempDir()
			if tc.sub != "" {
				dir = filepath.Join(dir, tc.sub)
				if err := os.MkdirAll(dir, 0700); err != nil {
					t.Fatal(err)
				}
			}
			ctx, result := filteredSamplingContextInHMC242(t, dir, false)
			var valid []filteredSamplingMemberHMC242
			switch tc.mode {
			case "other_thread":
				ctx.AnalysisIR.RequestModel.RuntimeTargets[0].PID = 12
			case "process_not_thread":
				ctx.AnalysisIR.RequestModel.RuntimeTargets[0].Kind = types.RuntimeTargetKindProcess
			case "other_window":
				a, b := 3., 4.
				p := ctx.AnalysisIR.RequestModel.RuntimeArtifactScopeProfile
				p.TimeStart, p.TimeEnd = &a, &b
				p.TimeWindows = nil
			default:
				changed := false
				for i := range result.Observations {
					r := &result.Observations[i]
					if _, ok, _ := tool.TraceQueryFilteredPerfMemberSelection(*r); !ok {
						continue
					}
					switch tc.mode {
					case "missing_receipt":
						var notes []string
						for _, n := range r.RichNotes {
							if !strings.HasPrefix(n, types.TraceNoteKeyPerfFilteredMember+"=") {
								notes = append(notes, n)
							}
						}
						r.RichNotes = notes
					case "wrong_source":
						r.SourceRef.Path += ".other"
					case "wrong_window":
						r.SourceRef.QueryWindowEndTs = 4
					case "mixed_member":
						if !changed {
							r.Value = alteredValue
							changed = true
							continue
						}
						member := filteredSamplingMemberHMC242{Value: r.Value}
						for _, n := range r.RichNotes {
							if dso, ok := strings.CutPrefix(n, "dso="); ok {
								member.DSO = dso
							}
						}
						valid = append(valid, member)
					}
				}
			}
			ctx.Mutable.SetTurnAArtifacts(types.TurnAArtifacts{ToolResults: []types.ToolResult{result}})
			prompt := (&answerDocumentEvaluator{}).BuildInitialInstruction(ctx, nil)
			part := filteredSamplingNativeSectionHMC242(prompt)
			if tc.mode == "mixed_member" {
				members := filteredSamplingNativeMembersHMC242(t, part)
				if part == "" || len(valid) == 0 || len(members) == 0 {
					t.Fatalf("valid sibling qualified an altered member or lost all valid members: valid=%d published=%d", len(valid), len(members))
				}
				for _, v := range filteredSamplingMixedMemberViolationsHMC242(members, valid, alteredValue) {
					t.Errorf("valid sibling qualified an altered member or lost all valid members: %s", v)
				}
				if tc.sub != "" && !strings.Contains(part, alteredValue) {
					t.Fatal("path noise no longer reaches the native section; this control is vacuous")
				}
				// Tamper control: the same section with one member line
				// carrying the altered value must be rejected.
				tampered := strings.Replace(part, `"value":"`+members[0].Value+`","unit"`, `"value":"`+alteredValue+`","unit"`, 1)
				if tampered == part || len(filteredSamplingMixedMemberViolationsHMC242(filteredSamplingNativeMembersHMC242(t, tampered), valid, alteredValue)) == 0 {
					t.Fatal("oracle accepted a published member carrying the altered value")
				}
				return
			}
			if part != "" {
				t.Fatalf("unrequested/unbound filtered facts bypassed shared projection: %s", tc.name)
			}
		})
	}
}

func TestFilteredSamplingActualFinalizerHMC242(t *testing.T) {
	for _, direct := range []bool{false, true} {
		t.Run(map[bool]string{false: "prepared_member", true: "direct_file"}[direct], func(t *testing.T) {
			ctx, result := filteredSamplingContextHMC242(t, direct)
			unavailable, members := false, 0
			for _, r := range result.Observations {
				if p, ok := tool.DecodeTracePerfSamplingTree(r); ok {
					unavailable = p.Status == "unavailable" && p.Reason == "requires_time_and_thread_selectors_only"
				}
				if r.Predicate == "perf_sample_top_symbol" {
					members++
				}
			}
			if !unavailable || members != 8 {
				t.Fatalf("fixture tree unavailable=%t native flat members=%d", unavailable, members)
			}
			ctx.Mutable.SetTurnAArtifacts(types.TurnAArtifacts{ToolResults: []types.ToolResult{result}})
			prompt := (&answerDocumentEvaluator{}).BuildInitialInstruction(ctx, nil)
			for _, want := range []string{"### Recorded sampling hotspot fields", `"samples=2"`, `"sample_weight=15"`, `"sample_weight=20"`, "requires_time_and_thread_selectors_only"} {
				if !strings.Contains(prompt, want) {
					t.Errorf("actual finalizer lost %q", want)
				}
			}
		})
	}
}
