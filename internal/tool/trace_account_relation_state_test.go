package tool

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/types"
)

// A dependency's dominant sleep state and its priority-inversion sub-account
// can share source lines. The line envelope is not a scheduler-state identity.
func TestTraceAccountRelationPublicDifferentStatesNeverClaimOverlap(t *testing.T) {
	data, err := os.ReadFile("../../eval/cases/trace_query_wakeup_causal_io_chain.case")
	if err != nil {
		t.Fatal(err)
	}
	_, body, ok := strings.Cut(string(data), "HTRACE='")
	if !ok {
		t.Fatal("missing fixture trace")
	}
	body, _, ok = strings.Cut(body, "\n'\n")
	if !ok {
		t.Fatal("unterminated fixture trace")
	}
	for _, rename := range []bool{false, true} {
		t.Run(fmt.Sprintf("renamed=%t", rename), func(t *testing.T) {
			trace := body
			if rename {
				trace = strings.NewReplacer("cookie", "session", "network", "transport", "threadpool", "executor").Replace(trace)
			}
			dir := t.TempDir()
			path := filepath.Join(dir, "states.ftrace")
			if err := os.WriteFile(path, []byte(trace), 0600); err != nil {
				t.Fatal(err)
			}
			params, _ := json.Marshal(map[string]any{"source": "path", "path": path, "view": "root_cause_rank", "pid": 100, "time_start": 2, "time_end": 2.020, "trace_flavor": "harmony_hitrace"})
			result, err := (&TraceQuery{}).Execute(&types.BusContext{RepoRoot: dir, WorkDir: dir}, params)
			if err != nil || !result.Success {
				t.Fatalf("public query: %v %s", err, result.Summary)
			}
			before, _ := json.Marshal(result.Observations)
			projection := types.TraceCausalProjectionFromObservationRecords(result.Observations)
			projectionBefore, _ := json.Marshal(projection)
			for _, zh := range []bool{true, false} {
				model := buildRuntimeTraceProjTreeModel(projection, newRuntimeTraceCausalProjectionEvidenceIndex(), zh)
				rows := runtimeTraceProjSMR1AllRows(&model)
				byTag := map[string]*runtimeTraceProjTreeRow{}
				sleep, pic := 0, 0
				for _, row := range rows {
					byTag[strings.TrimSpace(row.EvidenceTag)] = row
					if row.HasData && row.Node.IsSleepState() && (row.Node.ImpactMS == 17 || row.Node.ImpactMS == 14) {
						sleep++
					}
					if row.HasData && runtimeTracePriorityInversionCandidateType(row.Node.TypeToken) && row.Node.GatedRunnableMS > 0 {
						pic++
					}
				}
				if sleep < 2 || pic < 3 {
					t.Fatalf("sleep context or chain scheduling causes disappeared: sleep=%d pic=%d", sleep, pic)
				}
				for _, row := range rows {
					if row.AccountRelRef == "" {
						continue
					}
					peer := byTag[row.AccountRelRef]
					if peer == nil || row.AccountRelSameSourceFullMS > 0 {
						continue
					}
					if runtimeTraceProjSMR1StateFamily(row.Node) != runtimeTraceProjSMR1StateFamily(peer.Node) {
						t.Errorf("source lines falsely certify same-state physical overlap: %s/%s -> %s/%s", row.Node.Subject, row.Node.TypeToken, peer.Node.Subject, peer.Node.TypeToken)
					}
				}
				fence := runtimeTraceProjTreeFence(model, zh)
				for _, wrong := range []string{"物理时间重叠", "physical time overlaps"} {
					if strings.Contains(fence, wrong) {
						t.Errorf("mutually exclusive scheduler states still described as overlapping: %s", wrong)
					}
				}
			}
			after, _ := json.Marshal(result.Observations)
			projectionAfter, _ := json.Marshal(projection)
			if string(before) != string(after) || string(projectionBefore) != string(projectionAfter) {
				t.Fatal("display relation changed native evidence, candidate values or causal authority")
			}
		})
	}
}

func TestTraceAccountRelationSameLinesRequireCompatibleStateAccount(t *testing.T) {
	for _, variant := range []string{"same_state", "pure_runnable_pic", "different_state", "equal_value_different_state", "different_window", "disjoint_hulls", "equal_value_disjoint_hulls", "mixed_pic", "running_only_pic", "unknown_pic", "unknown_state"} {
		t.Run(variant, func(t *testing.T) {
			projection := smr1C1AccountPairProjection()
			a, b := &projection.OnChainCauses[0], &projection.OnChainCauses[1]
			a.QueryWindowStartTs, a.QueryWindowEndTs = 1, 2
			b.QueryWindowStartTs, b.QueryWindowEndTs = 1, 2
			switch variant {
			case "pure_runnable_pic":
				b.TypeToken, b.StateKind = "priority_inversion_candidate", "s_sleep"
				b.GatedRunnableMS = b.ImpactMS
			case "different_state", "equal_value_different_state":
				a.Object, a.StateKind = "s_sleep", "s_sleep"
				if variant == "equal_value_different_state" {
					b.ImpactMS = a.ImpactMS
				}
			case "different_window":
				b.QueryWindowEndTs = 3
			case "disjoint_hulls", "equal_value_disjoint_hulls":
				a.StartTs, a.EndTs, b.StartTs, b.EndTs = 1, 1.1, 1.2, 1.3
				if variant == "equal_value_disjoint_hulls" {
					b.ImpactMS = a.ImpactMS
				}
			case "mixed_pic":
				b.TypeToken = "priority_inversion_candidate"
				b.GatedRunnableMS, b.GatedRunningDeficitMS = 2, 1.399
			case "running_only_pic":
				b.TypeToken, b.GatedRunningDeficitMS = "priority_inversion_candidate", b.ImpactMS
			case "unknown_pic":
				b.TypeToken = "priority_inversion_candidate"
			case "unknown_state":
				a.Object, a.StateKind = "", ""
			}
			// Isolate only the account annotation pass. Real production lowering
			// and seating are covered by the public-query test above.
			model := runtimeTraceProjTreeModel{TreeRows: []runtimeTraceProjTreeRow{
				{Node: *a, HasData: true, EvidenceTag: "E1"},
				{Node: *b, HasData: true, EvidenceTag: "E2"},
			}}
			runtimeTraceProjMarkAccountRelations(&model, true)
			wantRelation := variant == "same_state" || variant == "pure_runnable_pic"
			for _, row := range model.TreeRows {
				if (row.AccountRelRef != "") != wantRelation {
					t.Errorf("%s: incorrect same-state account annotation %q", variant, row.AccountRelRef)
				}
				if !wantRelation && row.ValueMirrorRef != "" {
					t.Errorf("%s: equal scalar became a same-physical-time mirror %q", variant, row.ValueMirrorRef)
				}
			}
		})
	}
}
