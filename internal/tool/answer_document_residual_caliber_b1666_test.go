package tool

import (
	"bytes"
	"encoding/json"
	"fmt"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/hanchaoqun/codrax/internal/render"
	"github.com/hanchaoqun/codrax/internal/types"
)

// These are published-record shapes, not a preinstalled tree. In particular,
// Unit=ms deliberately preserves the legacy carrier: precise registry/tier
// classification, not the old slot name, owns the scalar's dimension.
func b1666ResidualBus(lang, lane, token, tier string, companion bool) *types.BusContext {
	ref := types.ObservationSourceRef{Kind: types.ObservationSourceRuntimeArtifact,
		Path: "/captures/residual.ftrace", ArtifactID: "residual.ftrace",
		PayloadRef: "/results/residual.json", RawRef: "/results/residual.json", QueryScopeID: "residual-scope"}
	row := func(id, predicate, subject, object string, value float64, notes ...string) types.ObservationRecord {
		return types.ObservationRecord{ID: id, Origin: types.AnswerEvidenceOriginRuntimeArtifact,
			Producer: "trace_query", Role: types.AnswerAggregateRolePrincipalAnswer,
			GroundingPolicy: types.ClaimGroundingHard, Predicate: predicate, ClaimKey: predicate + ":" + id,
			Subject: subject, Object: object, Value: fmt.Sprintf("%.3f", value), Unit: "ms", SourceRef: ref,
			ObservedAt: "2026-09-12T00:00:00Z", Confidence: .9,
			Span:        types.ObservationSpan{LineStart: 10, LineEnd: 20, StartTs: 10.01, EndTs: 10.07},
			SupportRefs: []string{"/captures/residual.ftrace:10-20"},
			RichNotes:   append([]string{"selected_window=10.000000..10.100000"}, notes...)}
	}
	self := row("self", "root_cause_target_self_state", "app-100", "sleep_wait", 60,
		"tier=target_self_state", "dominant_state=s_sleep", "chain_relevance=on_chain", "impact_ms=60.000", "cumulative_impact_ms=60.000")
	chain := row("chain", "root_cause_primary", "worker-200", "runnable_wait", 6,
		"rank=1", "tier=primary", "dominant_state=runnable", "chain_relevance=on_chain", "causality=on_wakeup_chain",
		"chain_depth=1", "impact_ms=6.000", "cumulative_impact_ms=6.000", "effective_impact_ms=6.000")
	path := row("path", "wakeup_chain", "app-100", "worker-200 -> app-100", 0)
	rows := []types.ObservationRecord{self, chain, path}
	if token != "" {
		subject := "app-100"
		if lane == "tree" {
			subject = "process-100" // Existing exact same-pid, different-label own-process lane.
		}
		value := 150.0
		if lane == "self" && tier != "caliber_side" && token != "page_cache_churn" && token != "block_io_by_inode" {
			value = 20 // Existing own-wall-clock attribution must leave a positive residual.
		}
		candidate := row("candidate", "root_cause_tertiary", subject, token, value,
			"type="+token, "tier="+tier, "chain_relevance=on_chain", fmt.Sprintf("impact_ms=%.3f", value), fmt.Sprintf("cumulative_impact_ms=%.3f", value))
		if lane == "self" && token != "page_cache_churn" && token != "block_io_by_inode" {
			candidate.Predicate, candidate.ClaimKey = "critical_blocking", "critical_blocking:candidate"
		}
		rows = append(rows, candidate)
		if companion {
			valid := row("valid-wall-clock", "root_cause_tertiary", subject, "io_wait", 20,
				"type=io_wait", "tier=tertiary", "chain_relevance=on_chain", "impact_ms=20.000", "cumulative_impact_ms=20.000")
			// Separate occurrence: the existing same-segment family fold must
			// not hide either selection candidate before this test reaches it.
			valid.Span = types.ObservationSpan{LineStart: 30, LineEnd: 40, StartTs: 10.08, EndTs: 10.09}
			valid.SupportRefs = []string{"/captures/residual.ftrace:30-40"}
			if lane == "self" {
				valid.Predicate, valid.ClaimKey = "critical_blocking", "critical_blocking:valid-wall-clock"
			}
			rows = append(rows, valid)
		}
	}
	return &types.BusContext{Mutable: types.NewMutableState("B1666 residual dimensions"), Language: lang,
		AnalysisIR: &types.AnalysisIR{RequestModel: types.RequestModel{Intent: types.IntentTrace, Scenario: types.ScenarioPerformanceBottleneck},
			AnswerContract: types.AnswerContract{Language: lang}},
		ToolResults: []types.ToolResult{{ToolName: "trace_query", Success: true, Observations: rows}}}
}

func b1666Publish(t *testing.T, bus *types.BusContext) (string, runtimeTraceProjTreeModel) {
	t.Helper()
	before, err := json.Marshal(bus.ToolResults)
	if err != nil {
		t.Fatal(err)
	}
	ledger := types.CompileObservationLedger(types.ObservationLedgerInputFromBusContext(bus, types.ObservationExtractLedgerEvidenceLimit))
	projection := types.CompileTraceCausalProjection(ledger)
	model := buildRuntimeTraceProjTreeModel(projection, newRuntimeTraceCausalProjectionEvidenceIndex(), bus.Language == "zh")
	doc := &types.AnswerDocumentV2{DocumentModel: "v2", Blocks: []types.AnswerBlock{{ID: "model", Kind: types.BlockSummary,
		Text: "Model-owned account stays unchanged. 模型正文保持。"}}}
	wire, err := modelOwnedAnswerBlockWire(doc)
	if err != nil {
		t.Fatal(err)
	}
	published, err := ApplyAndPersistMutation(bus, "test_emit", types.NewReplaceAllMutation(doc), nil, time.Unix(1, 0))
	if err != nil || !published.Success {
		t.Fatalf("public mutation: %v %+v", err, published)
	}
	stored := bus.Mutable.AnswerDocumentV2()
	if err := requireModelOwnedAnswerBlockWirePreserved(wire, stored); err != nil {
		t.Fatal(err)
	}
	after, err := json.Marshal(bus.ToolResults)
	if err != nil || !bytes.Equal(before, after) {
		t.Fatal("display changed source observations")
	}
	if next := types.CompileTraceCausalProjection(types.CompileObservationLedger(types.ObservationLedgerInputFromBusContext(bus, types.ObservationExtractLedgerEvidenceLimit))); !reflect.DeepEqual(projection, next) {
		t.Fatal("display changed projection values, ordering or source identity")
	}
	return render.RenderAnswerDocument(stored, bus.Language), model
}

func TestB1666ResidualCaliberActualPublication(t *testing.T) {
	cases := []struct {
		name, token, tier string
		excluded          bool
	}{
		{"registry-count-legacy-ms", "page_cache_churn", "tertiary", true},
		{"registry-composite-legacy-ms", "block_io_by_inode", "tertiary", true},
		{"explicit-tier-legacy-ms", "io_wait", "caliber_side", true},
		{"explicit-composite-unit", "io_wait", "tertiary", true},
		{"clamped-count-family", "io_wait", "tertiary", true},
		{"wall-clock-wait", "io_wait", "tertiary", false},
		{"wall-clock-latency", "io_latency", "tertiary", false},
		{"wall-clock-episode", "io_burst_episode", "tertiary", false},
	}
	for _, lang := range []string{"zh", "en"} {
		for _, lane := range []string{"self", "tree"} {
			for _, tc := range cases {
				t.Run(lang+"/"+lane+"/"+tc.name, func(t *testing.T) {
					bus := b1666ResidualBus(lang, lane, tc.token, tc.tier, false)
					candidate := &bus.ToolResults[0].Observations[3]
					if tc.name == "explicit-composite-unit" {
						candidate.Unit = "composite_score"
					}
					if tc.name == "clamped-count-family" {
						candidate.RichNotes = append(candidate.RichNotes, "member_count=2", "member_fold_caliber=count_sum", "member_sum_ms=180.000", "member_max_ms=100.000", "member_min_ms=80.000")
					}
					text, model := b1666Publish(t, bus)
					rows := model.SelfRows
					if lane == "tree" {
						rows = model.TreeRows
					}
					found := false
					for _, row := range rows {
						if row.Node.EvidenceID == "candidate" {
							found = true
							if row.Node.TypeToken != tc.token || fmt.Sprintf("%.3f", row.Node.ImpactMS) != candidate.Value || (lane == "tree" && !runtimeTraceProjOwnProcessIORow(row)) {
								t.Fatalf("candidate did not reach requested typed lane: %+v", row)
							}
							if row.EvidenceTag == "" || !strings.Contains(text, "["+row.EvidenceTag+"]") {
								t.Error("separate scalar's evidence reference must remain published")
							}
						}
					}
					if !found {
						t.Fatalf("candidate must remain visible in %s lane", lane)
					}
					clause := "重叠解释"
					if lang == "en" {
						clause = "of the residual is co-explained"
					}
					if strings.Contains(text, clause) == tc.excluded {
						t.Errorf("public residual clause authority: excluded=%t contains=%t", tc.excluded, strings.Contains(text, clause))
						for _, line := range strings.Split(text, "\n") {
							if strings.Contains(line, "关注线程等待(") || strings.Contains(line, "Focused thread wait") {
								t.Log(line)
							}
						}
					}
					// The original denominator, numerator and residual remain
					// 60/6/54, even when the non-wall-clock explanation is omitted.
					for _, value := range []string{"60.000ms", "6.000ms", "54.000ms", candidate.Value} {
						if !strings.Contains(text, value) {
							t.Errorf("original account or separate scalar disappeared: %s", value)
						}
					}
				})
			}
		}
	}
}

func TestB1666ResidualMixedCalibersKeepWallClockExplanation(t *testing.T) {
	for _, lang := range []string{"zh", "en"} {
		for _, lane := range []string{"self", "tree"} {
			for _, reverse := range []bool{false, true} {
				t.Run(fmt.Sprintf("%s/%s/reverse=%t", lang, lane, reverse), func(t *testing.T) {
					bus := b1666ResidualBus(lang, lane, "page_cache_churn", "caliber_side", true)
					if reverse {
						rows := bus.ToolResults[0].Observations
						for i, j := 0, len(rows)-1; i < j; i, j = i+1, j-1 {
							rows[i], rows[j] = rows[j], rows[i]
						}
					}
					text, model := b1666Publish(t, bus)
					value, _, ok := runtimeTraceProjOwnCaliberIOPrimaryRow(model)
					if !ok || value != 20 {
						t.Errorf("largest count must not mask valid wall-clock candidate: value=%v ok=%t", value, ok)
					}
					want := "未归因中最大 20.000ms 与自身 IO 口径行"
					if lang == "en" {
						want = "Up to 20.000ms of the residual is co-explained"
					}
					if !strings.Contains(text, want) || !strings.Contains(text, "150.000") {
						t.Errorf("public output must keep valid explanation and independent count: want=%q", want)
					}
				})
			}
		}
	}
}
