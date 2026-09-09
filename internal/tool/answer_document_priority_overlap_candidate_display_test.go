package tool

import (
	"encoding/json"
	"math"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/hanchaoqun/codrax/internal/render"
	"github.com/hanchaoqun/codrax/internal/types"
)

func TestB1635bActualPublicationKeepsRunnableOverlapCandidateBoundary(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "overlap.systrace")
	// The real engine's target-self overlap channel requires no holder or
	// synchronization dependency: these are only two scheduler switches.
	trace := `app-20 (20) [001] .... 8.000000: cpu_frequency: state=1000000 cpu_id=1
rival-30 (30) [001] .... 8.010000: sched_switch: prev_comm=app prev_pid=20 prev_prio=53 prev_state=R+ ==> next_comm=rival next_pid=30 next_prio=20
rival-30 (30) [001] .... 8.090000: sched_switch: prev_comm=rival prev_pid=30 prev_prio=20 prev_state=R+ ==> next_comm=app next_pid=20 next_prio=53
`
	if err := os.WriteFile(path, []byte(trace), 0o600); err != nil {
		t.Fatal(err)
	}
	params, err := json.Marshal(map[string]any{
		"source": "path", "path": path, "view": "root_cause_rank", "pid": 20,
		"time_start": 8, "time_end": 8.1, "trace_flavor": "harmony_hitrace", "min_duration_ms": 0.05,
	})
	if err != nil {
		t.Fatal(err)
	}
	result, err := (&TraceQuery{}).Execute(&types.BusContext{RepoRoot: dir, WorkDir: dir}, params)
	if err != nil || !result.Success {
		t.Fatalf("actual query failed: %v / %s", err, result.Summary)
	}
	projection := types.CompileTraceCausalProjection(types.ObservationLedger{Records: result.Observations})
	var self *types.TraceCausalProjectionNode
	for i := range projection.OnChainCauses {
		if node := &projection.OnChainCauses[i]; node.TypeToken == "priority_inversion_runnable_wait" && node.Subject == "app-20" {
			self = node
		}
	}
	if self == nil || self.ChainCredentialCensus != "target_self" || self.Rank < 1 || self.Tier != "primary" || math.Abs(self.EffectiveImpactMS-80) > 0.000001 {
		t.Fatalf("fixture must publish the real positive target-self overlap seat unchanged: %+v", self)
	}
	if self.FixDirection != "lock_priority" || !strings.Contains(self.Summary, "priority-inversion (runnable-overlap) candidate") {
		t.Fatalf("fixture lost the existing direction or explicit candidate provenance: %+v", self)
	}
	before, err := json.Marshal(result.Observations)
	if err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		lang, word string
	}{
		{"zh", "优先级反转候选·同核可运行重叠"},
		{"en", "priority inversion candidate · same-CPU runnable overlap"},
	} {
		t.Run(tc.lang, func(t *testing.T) {
			bus := newBusForMutationTest()
			bus.Language = tc.lang
			bus.AnalysisIR = &types.AnalysisIR{RequestModel: types.RequestModel{Intent: types.IntentTrace, Scenario: types.ScenarioPerformanceBottleneck}}
			bus.ToolResults = []types.ToolResult{result}
			modelBlock := types.AnswerBlock{ID: "model-summary", Kind: types.BlockSummary, Text: "模型原句 / Model-authored sentence: no rewrite."}
			doc := &types.AnswerDocumentV2{DocumentModel: "v2", Blocks: []types.AnswerBlock{modelBlock}}
			got, err := ApplyAndPersistMutation(bus, "test_emit", types.NewReplaceAllMutation(doc), nil, time.Now())
			if err != nil || !got.Success {
				t.Fatalf("actual publication failed: %v / %s", err, got.Summary)
			}
			accepted := bus.Mutable.AnswerDocumentV2()
			foundModel := false
			for _, block := range accepted.Blocks {
				if block.ID == modelBlock.ID {
					foundModel = true
					if !reflect.DeepEqual(block, modelBlock) {
						t.Fatalf("system display changed the model block: %+v", block)
					}
				}
			}
			// The mutation may append system-owned blocks to this document;
			// the original model block itself must remain byte-for-byte equal.
			if !foundModel || len(doc.Blocks) == 0 || !reflect.DeepEqual(doc.Blocks[0], modelBlock) {
				t.Fatal("publication removed or rewrote the original model block")
			}
			after, _ := json.Marshal(result.Observations)
			if string(before) != string(after) {
				t.Fatal("display mutated the source token, numerical ruler, rank, direction or source")
			}
			md := render.RenderAnswerDocument(accepted, tc.lang)
			unfolded := small3SquashSpaces(small3UnfoldElim(md))
			if !strings.Contains(unfolded, small3SquashSpaces(tc.word)) {
				t.Fatalf("real publication dropped candidate/same-CPU overlap boundary; want %q:\n%s", tc.word, md)
			}
			if !strings.Contains(md, "80.000") || !strings.Contains(md, "app-20") {
				t.Fatalf("display lost the unchanged target and 80ms positive seat:\n%s", md)
			}
			for _, invented := range []string{"已证锁持有者", "已证依赖反转", "confirmed lock holder", "confirmed dependency inversion"} {
				if strings.Contains(md, invented) {
					t.Fatalf("scheduler-only fixture invented a synchronization mechanism: %q", invented)
				}
			}
		})
	}
}

func TestB1635bCandidateChannelsKeepDistinctSharedLabels(t *testing.T) {
	for _, tc := range []struct {
		zh                  bool
		chainWord, selfWord string
	}{
		{true, "优先级反转候选", "优先级反转候选·同核可运行重叠"},
		{false, "priority inversion (candidate)", "priority inversion candidate · same-CPU runnable overlap"},
	} {
		chain := gatedCalCompositeNode(2)
		self := a5RunnableWaitSeat(3, 7.727)
		self.ChainCredentialCensus = "target_self"
		for _, row := range []struct {
			node types.TraceCausalProjectionNode
			word string
		}{{chain, tc.chainWord}, {self, tc.selfWord}} {
			before := row.node
			for _, got := range []string{
				TraceRootCauseTypeDisplayLabel(row.node.TypeToken, tc.zh),
				firstWordA5(runtimeTraceProjCauseCategoryWord(row.node, runtimeTraceProjTreeRowChain, tc.zh)),
				firstWordA5(runtimeTraceCausalProjectionImpactShapeCellTyped(row.node, tc.zh)),
				runtimeTraceProjImpactFormFamilyWord(row.node, tc.zh),
			} {
				if got != row.word {
					t.Errorf("typed channel %q has mismatched display: got %q want %q", row.node.TypeToken, got, row.word)
				}
			}
			if !reflect.DeepEqual(row.node, before) {
				t.Fatal("label changed candidate authority")
			}
		}
	}
}
