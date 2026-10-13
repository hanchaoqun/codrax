package orchestrator

import (
	"encoding/json"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/hanchaoqun/codrax/internal/agent"
	"github.com/hanchaoqun/codrax/internal/tool"
	"github.com/hanchaoqun/codrax/internal/types"
)

func TestHMC232ReconciliationPublicPreservesNativeIORuler(t *testing.T) {
	path, err := filepath.Abs("../../eval/fixtures/hmosperf_business_io_chain/events.systrace")
	if err != nil {
		t.Fatal(err)
	}
	for _, lang := range []string{"zh", "en"} {
		for _, pid := range []int{0, 100} {
			t.Run(lang+"/"+map[int]string{0: "background", 100: "chain"}[pid], func(t *testing.T) {
				bus := &types.BusContext{RepoRoot: t.TempDir(), WorkDir: t.TempDir(), Mutable: types.NewMutableState("business wait"), Language: lang,
					AnalysisIR: &types.AnalysisIR{RequestModel: types.RequestModel{Intent: types.IntentTrace, Scenario: types.ScenarioPerformanceBottleneck}}}
				params, _ := json.Marshal(map[string]any{"source": "path", "path": path, "view": "root_cause_rank", "pid": pid, "time_start": .999, "time_end": 1.051})
				result, err := (&tool.TraceQuery{}).Execute(bus, params)
				if err != nil || !result.Success {
					t.Fatalf("public query: %v / %s", err, result.Summary)
				}
				bus.ToolResults = []types.ToolResult{result}
				const prose = "The root cause is document-worker-200."
				result, err = tool.ApplyAndPersistMutation(bus, "io_ruler_test", types.NewReplaceAllMutation(psgProseDoc(prose)), nil, time.Now())
				if err != nil || !result.Success {
					t.Fatalf("publication: %v / %s", err, result.Summary)
				}
				before, _ := json.Marshal(bus.Mutable.AnswerDocumentV2())
				rows := tool.RuntimeTraceReconciliationRows(bus)
				var rank *tool.RuntimeTraceReconciliationRow
				for i := range rows {
					if rows[i].Kind == tool.RuntimeTraceReconciliationRankOne {
						rank = &rows[i]
					}
				}
				if pid == 0 {
					if rank != nil {
						t.Fatal("targetless background inventory acquired a root-cause reconciliation seat")
					}
					return
				}
				if rank == nil || rank.CauseToken != "io_latency" || rank.Subject != "document-worker-200" || rank.EffectiveMS != 31 || rank.EvidenceTag == "" {
					t.Fatalf("fixture lost its native chain wait: %+v", rows)
				}
				o := &Orchestrator{busCtx: bus}
				out := &agent.StageOutput{FinalAnswer: prose}
				o.attachSystemCrossCheckAppendix(out, "", nil)
				var appendix strings.Builder
				for _, attachment := range bus.Mutable.AnswerDisplayAttachments() {
					if attachment.Source == types.AnswerDisplayAttachmentSourceSystemCrossCheck {
						appendix.WriteString(attachment.Body)
					}
				}
				want := types.TraceIOValueCaliberLabel(types.TraceIOValueCaliberIssuerBlocked, lang == "zh")
				if !strings.Contains(appendix.String(), want) || !strings.Contains(appendix.String(), "31.000ms ["+rank.EvidenceTag+"]") {
					t.Errorf("actual appendix lost native ruler/value/source, want %q: %s", want, appendix.String())
				}
				after, _ := json.Marshal(bus.Mutable.AnswerDocumentV2())
				if string(before) != string(after) || !strings.Contains(out.FinalAnswer, prose) || !reflect.DeepEqual(rows, tool.RuntimeTraceReconciliationRows(bus)) {
					t.Fatal("reader label changed the accepted model prose or factual rows")
				}
			})
		}
	}
}

func TestHMC232ReconciliationTypedRulersStayIndependent(t *testing.T) {
	for _, caliber := range []string{types.TraceIOValueCaliberRQResidence, types.TraceIOValueCaliberBIOResidence,
		types.TraceIOValueCaliberIssuerBlocked, types.TraceIOValueCaliberMixed, "", "future_measurement"} {
		row := tool.RuntimeTraceReconciliationRow{Subject: "worker-200", CauseToken: "io_latency", IOValueCaliber: caliber,
			EffectiveMS: 31, Rank: 1, EvidenceTag: "E2"}
		before := row
		got := renderRankOneReconciliation(row)
		if !strings.Contains(got.entryZH, types.TraceIOValueCaliberLabel(caliber, true)) ||
			!strings.Contains(got.entry, types.TraceIOValueCaliberLabel(caliber, false)) ||
			!strings.Contains(got.entry, "31.000ms [E2]") {
			t.Errorf("ruler %q lost: %+v", caliber, got)
		}
		if !reflect.DeepEqual(before, row) {
			t.Fatal("display changed the native row")
		}
	}
}
