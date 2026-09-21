package tool

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/types"
	"github.com/mattn/go-runewidth"
)

// Preserve the live case's native observations: public Execute -> typed
// projection -> final mutation/render. No fabricated causal-closure carrier or
// edited measurement may make a complete state account a complete diagnosis.
func TestProjectionCoverageSemanticsPublicWakeupChain(t *testing.T) {
	data, err := os.ReadFile("../../eval/cases/trace_query_wakeup_causal_io_chain.case")
	if err != nil {
		t.Fatal(err)
	}
	_, body, ok := strings.Cut(string(data), "HTRACE='")
	if !ok {
		t.Fatal("missing trace fixture")
	}
	body, _, ok = strings.Cut(body, "\n'\n")
	if !ok {
		t.Fatal("unterminated trace fixture")
	}
	for _, renamed := range []bool{false, true} {
		t.Run(fmt.Sprintf("renamed=%t", renamed), func(t *testing.T) {
			trace := body
			if renamed {
				trace = strings.NewReplacer("app", "client", "cookie", "session", "network", "transport", "threadpool", "executor").Replace(trace)
			}
			path := hmc081WriteTrace(t, trace)
			bus, _, _ := hmc17NamedPathContext(t)
			var records []types.ObservationRecord
			for _, view := range []string{"root_cause_rank", "wakeup_chain", "window_stats"} {
				result := hmc17NamedQuery(t, bus, map[string]any{"source": "path", "path": path, "view": view, "pid": 100, "time_start": 2, "time_end": 2.020, "trace_flavor": "harmony_hitrace"})
				if !result.Success {
					t.Fatalf("public %s: %s", view, result.Summary)
				}
				records = append(records, result.Observations...)
			}
			before, _ := json.Marshal(records)
			for _, lang := range []string{"zh", "en"} {
				md := p3mRenderUserFace(t, records, lang)
				for _, wrong := range []string{"已由链上解释", "链上已归因 20.000ms(100%)", "fully explained on-chain", "explained on-chain.", "On-chain attributed 20.000ms/100%", "真实占时·杠杆=自身工作量", "genuine time · lever: own workload"} {
					if strings.Contains(md, wrong) {
						t.Errorf("state/wakeup coverage or sleep became a cause/work claim (%s): %q", lang, wrong)
					}
				}
				for _, value := range []string{"20.000ms", "17.000ms", "14.000ms", "11.000ms", "1.000ms"} {
					if !strings.Contains(md, value) {
						t.Errorf("wording repair lost native value %s (%s)", value, lang)
					}
				}
				wantCoverage, wantWait := "链路覆盖 20.000ms(100%)", "等待症状；沿唤醒/阻塞依赖下钻"
				if lang == "en" {
					wantCoverage, wantWait = "Chain coverage 20.000ms/100%", "waiting symptom; inspect wakeup/blocking dependencies"
				}
				if !strings.Contains(ioFoldScopeCompact(md), ioFoldScopeCompact(wantCoverage)) || !strings.Contains(ioFoldScopeCompact(md), ioFoldScopeCompact(wantWait)) {
					t.Errorf("final %s display lost neutral coverage/wait guidance: want %q and %q", lang, wantCoverage, wantWait)
				}
			}
			after, _ := json.Marshal(records)
			if string(before) != string(after) {
				t.Fatal("display changed public evidence")
			}
		})
	}
}

func TestProjectionCoverageSemanticsStateAccountPresence(t *testing.T) {
	for _, account := range []string{"nil", "partial", "full"} {
		for _, zh := range []bool{true, false} {
			t.Run(fmt.Sprintf("%s/zh=%t", account, zh), func(t *testing.T) {
				projection := covOpendirProjection(112.175)
				if account != "nil" {
					projection.TargetStateAccount = &types.TraceCausalProjectionTargetStateAccount{
						Subject: "aweme-16547", SleepMS: 131, TotalMS: 131,
						WindowStartTs: projection.WindowStartTs, WindowEndTs: projection.WindowEndTs,
					}
					if account == "partial" {
						projection.TargetStateAccount.SleepMS, projection.TargetStateAccount.TotalMS = 100, 100
					}
				}
				before, _ := json.Marshal(projection)
				model := buildRuntimeTraceProjTreeModel(projection, nil, zh)
				verdict := runtimeTraceProjCoverageVerdictFor(projection, model)
				line := runtimeTraceProjWindowLine(projection, model, zh)
				if verdict.AttributedMS != 112.175 || !strings.Contains(line, "112.175ms") {
					t.Fatalf("coverage amount changed: %+v / %s", verdict, line)
				}
				for _, wrong := range []string{"已由链上解释", "链上已归因", "explained on-chain", "On-chain attributed"} {
					if strings.Contains(line, wrong) {
						t.Errorf("%s account still claims causal closure: %q", account, wrong)
					}
				}
				boundary := "链路覆盖不等于原因已全部查明"
				if !zh {
					boundary = "Chain coverage does not mean all causes are known"
				}
				if !strings.Contains(line, boundary) {
					t.Errorf("coverage must state its evidence ceiling: %s", line)
				}
				after, _ := json.Marshal(projection)
				if string(before) != string(after) {
					t.Fatal("wording changed projection")
				}
			})
		}
	}
}

func TestProjectionCoverageSemanticsUnpricedStateAdvice(t *testing.T) {
	for _, state := range []string{"s_sleep", "sleep", "sleep_wait", "running", "unknown", "mixed", "sleep_unknown", "mixed_unknown", "adjacent"} {
		for _, zh := range []bool{true, false} {
			t.Run(fmt.Sprintf("%s/zh=%t", state, zh), func(t *testing.T) {
				projection := twodimProjectionWithUnpricedRunning()
				node := &projection.OnChainCauses[len(projection.OnChainCauses)-1]
				if state != "mixed" && state != "mixed_unknown" && state != "sleep_unknown" && state != "adjacent" {
					node.StateKind, node.Object, node.TypeToken = state, state, state
				}
				if state == "sleep_unknown" {
					node.StateKind, node.Object, node.TypeToken = "unknown", "unknown", "unknown"
				}
				if state == "mixed" || state == "mixed_unknown" || state == "sleep_unknown" {
					sleep := *node
					sleep.EvidenceID, sleep.Subject = "E-wait", "sleeper-88"
					sleep.StateKind, sleep.Object, sleep.TypeToken = "s_sleep", "s_sleep", "s_sleep"
					projection.OnChainCauses = append(projection.OnChainCauses, sleep)
				}
				if state == "mixed_unknown" {
					unknown := projection.OnChainCauses[len(projection.OnChainCauses)-1]
					unknown.EvidenceID, unknown.Subject = "E-unknown", "unknown-89"
					unknown.StateKind, unknown.Object, unknown.TypeToken = "unknown", "unknown", "unknown"
					projection.OnChainCauses = append(projection.OnChainCauses, unknown)
				}
				if state == "adjacent" {
					node.ChainRelevance = "adjacent"
				}
				before, _ := json.Marshal(projection)
				model, fence := elimRenderOverview(t, projection, zh)
				for _, displayLine := range strings.Split(fence, "\n") {
					if width := runewidth.StringWidth(displayLine); width > runtimeTraceProjTreeRowMaxWidth {
						t.Errorf("state guidance exceeded the %d-cell display boundary (%d): %s", runtimeTraceProjTreeRowMaxWidth, width, displayLine)
					}
				}
				label, own, wait := "未计价占用", "自身工作量", "沿唤醒/阻塞依赖下钻"
				if !zh {
					label, own, wait = "unpriced occupancy", "own workload", "inspect wakeup/blocking dependencies"
				}
				var line string
				// Read the same typed auxiliary row before fixed-width wrapping;
				// the public test above separately checks the final wrapped face.
				for _, candidate := range runtimeTraceProjElimAuxAccountRows(model, nil, zh) {
					if candidate.label == label {
						line = candidate.content
					}
				}
				if state == "adjacent" {
					if line != "" {
						t.Fatalf("adjacent row gained on-chain advice: %s", line)
					}
					return
				}
				if !strings.Contains(line, "41.500ms") {
					t.Fatalf("unpriced amount disappeared: %s", fence)
				}
				// The longest mixed-state row must survive final wrapping in
				// full, including its amount, evidence reference and every clause.
				if !strings.Contains(line, "[E") || !strings.Contains(ioFoldScopeCompact(fence), ioFoldScopeCompact(line)) {
					t.Errorf("final fence lost unpriced evidence or advice: %q\n%s", line, fence)
				}
				wantOwn := state == "running" || state == "mixed" || state == "mixed_unknown"
				wantWait := state == "mixed" || state == "mixed_unknown" || state == "sleep_unknown" || state == "s_sleep" || state == "sleep" || state == "sleep_wait"
				if strings.Contains(line, own) != wantOwn || strings.Contains(line, wait) != wantWait {
					t.Errorf("advice not bound to typed state: own=%t wait=%t: %s", wantOwn, wantWait, line)
				}
				if state == "mixed_unknown" || state == "sleep_unknown" {
					unknownBoundary := "其余"
					if !zh {
						unknownBoundary = "other states: unresolved"
					}
					if !strings.Contains(line, unknownBoundary) {
						t.Errorf("unknown state inherited a known state's direction: %s", line)
					}
				}
				after, _ := json.Marshal(projection)
				if string(before) != string(after) {
					t.Fatal("advice changed measurement or causal position")
				}
			})
		}
	}
}

func TestProjectionCoverageSemanticsToolTeaching(t *testing.T) {
	query := &TraceQuery{}
	for name, face := range map[string]string{"description": query.Description(), "parameters": string(query.Parameters())} {
		for _, want := range []string{
			"root causes have TWO dimensions",
			"raw time occupancy that guides NEW fix directions",
			"scheduler waiting states remain waiting occupancy",
			"do not infer own workload from sleep",
			"unclassified occupancy has no default optimization lever",
		} {
			if !strings.Contains(face, want) {
				t.Errorf("actual %s omitted the state-bound direction: %q", name, want)
			}
		}
		if strings.Contains(face, "report it as a raw-occupancy finding with the own-workload/business lever") {
			t.Errorf("actual %s still gives every context-only state an own-workload lever", name)
		}
	}
}
