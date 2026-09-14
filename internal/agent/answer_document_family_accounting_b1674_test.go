package agent

import (
	"encoding/json"
	"fmt"
	"strings"
	"testing"

	promptcontext "github.com/hanchaoqun/codrax/internal/context"
	"github.com/hanchaoqun/codrax/internal/skill"
	"github.com/hanchaoqun/codrax/internal/types"
)

// Typed trace_query producer fixtures through both public prompt entrypoints;
// this is a handoff test, not a claim to remeasure native IO or run a live model.
func b1674FamilyRows(typ string, count int, maximum, fold string) []types.ObservationRecord {
	ref := types.ObservationSourceRef{Kind: types.ObservationSourceRuntimeArtifact, Path: "/captures/family.ftrace", ArtifactID: "family.ftrace", ArtifactKind: "trace",
		ToolCallID: "family-query", PayloadRef: "/payload/family.json", QueryWindowKnown: true, QueryWindowStartTs: 10, QueryWindowEndTs: 10.2}
	root := types.ObservationRecord{
		ID: "trace_query:family#root_cause_rank:1", Producer: "trace_query", Origin: types.AnswerEvidenceOriginRuntimeArtifact,
		Role: types.AnswerAggregateRolePrincipalAnswer, GroundingPolicy: types.ClaimGroundingHard,
		Subject: "client-100", Predicate: "root_cause_primary", ClaimKey: "root_cause_primary:client-100", Object: typ,
		Value: "12.658", Unit: "ms", SourceRef: ref, Span: types.ObservationSpan{StartTs: 10.001, EndTs: 10.150, LineStart: 10, LineEnd: 100},
		RichNotes: []string{"selected_window=10.000000..10.200000", "rank=1", "tier=primary", "chain_relevance=on_chain", "causality=self_wall_clock",
			"impact_ms=12.658", "cumulative_impact_ms=12.658", "effective_impact_ms=12.658", "fix_direction=io_dependency",
			types.TraceNoteKeyRankBoardTarget + "=client-100", types.TraceNoteKeyRankBoardParams + "=family-params"},
	}
	if count != 0 {
		root.RichNotes = append(root.RichNotes, fmt.Sprintf("%s=%d", types.TraceNoteKeyMemberCount, count))
	}
	if maximum != "" {
		root.RichNotes = append(root.RichNotes, types.TraceNoteKeyMemberMaxMS+"="+maximum)
	}
	if fold != "" {
		root.RichNotes = append(root.RichNotes, types.TraceNoteKeyMemberFoldCaliber+"="+fold)
	}
	if typ == "runnable_wait" {
		for i, note := range root.RichNotes {
			if note == "fix_direction=io_dependency" {
				root.RichNotes[i] = "fix_direction=scheduling_supply"
			}
		}
	}
	state := root
	state.ID, state.Predicate, state.ClaimKey, state.Object, state.Value = "trace_query:family#target_window_states", "target_window_states", "target_window_states:client-100", "state_partition", "200.000"
	state.Role = types.AnswerAggregateRoleSupportingCoverage
	state.Span.StartTs, state.Span.EndTs = 10, 10.2
	state.RichNotes = []string{"selected_window=10.000000..10.200000", "running=150.000", "runnable=0.000", "sleep=50.000", "d_state=0.000", "io_wait=0.000", "sleep_io_wait=0.000", "total=200.000"}
	rows := []types.ObservationRecord{state, root}
	// A separate capped interval account for the same subject. It must not
	// donate its occurrence count or maximum to the ranked family measurement.
	for i := 0; i < 10; i++ {
		wait := root
		wait.ID = fmt.Sprintf("trace_query:blocked#critical_blocking:%d", i+1)
		wait.SourceRef.ToolCallID, wait.SourceRef.PayloadRef = "blocking-query", "/payload/blocking.json"
		wait.Predicate, wait.ClaimKey, wait.Object, wait.Value = "critical_blocking", fmt.Sprintf("critical_blocking:%d", i), "io_latency", "0.740"
		wait.Role = types.AnswerAggregateRoleSupportingCoverage
		wait.Span.StartTs, wait.Span.EndTs = 10.01+float64(i)*.002, 10.010740+float64(i)*.002
		if i == 9 {
			wait.Value, wait.Span.EndTs = "0.748", wait.Span.StartTs+.000748
		}
		wait.RichNotes = []string{"selected_window=10.000000..10.200000", "type=io_latency", "capacity_truncated=true"}
		rows = append(rows, wait)
	}
	return rows
}

func b1674FamilyPublicPrompt(t *testing.T, rows []types.ObservationRecord, lang string, windows ...[2]float64) (string, string, types.TraceCausalProjectionSet) {
	t.Helper()
	start, end := 10.0, 10.2
	mu := types.NewMutableState("explain the selected response window")
	mu.SetTurnAArtifacts(types.TurnAArtifacts{ToolResults: []types.ToolResult{{ToolName: "trace_query", Success: true, Observations: rows}}})
	mu.SetAnswerDocumentV2WithMutation(types.MutationReplaceAll, &types.AnswerDocumentV2{DocumentModel: "v2", Blocks: []types.AnswerBlock{{ID: "model-summary", Kind: types.BlockSummary, Text: "The model owns the cause and recommendation."}}})
	bus := &types.BusContext{Language: lang, Mutable: mu, AnalysisIR: &types.AnalysisIR{RequestModel: types.RequestModel{Language: lang, Intent: types.IntentRootCause,
		RuntimeQuestionProfile:      &types.RuntimeQuestionProfile{Scope: types.RuntimeQuestionScopeCausalDiagnosis},
		RuntimeTargets:              []types.RuntimeTarget{{Kind: types.RuntimeTargetKindThread, PID: 100, Thread: "client-100", Source: "user_explicit"}},
		RuntimeArtifactScopeProfile: &types.RuntimeArtifactScopeProfile{RequestedScope: types.RuntimeArtifactScopeExplicitWindow, TimeStart: &start, TimeEnd: &end, SourceQuote: "10..10.2"}}}}
	for _, w := range windows {
		lo, hi := w[0], w[1]
		bus.AnalysisIR.RequestModel.RuntimeArtifactScopeProfile.TimeWindows = append(bus.AnalysisIR.RequestModel.RuntimeArtifactScopeProfile.TimeWindows,
			types.RuntimeArtifactTimeWindow{TimeStart: &lo, TimeEnd: &hi, SourceQuote: fmt.Sprintf("%.6f..%.6f", lo, hi)})
	}
	ctx := promptcontext.BuildAgentContext(bus, types.AgentFinalizer, types.StageFinalize)
	snapshot := func() string {
		data, err := json.Marshal([]any{mu.TurnAArtifacts(), mu.AnswerDocumentV2(), ctx.AnalysisIR.RequestModel, types.CompileTraceCausalProjectionSet(answerDocObservationLedger(ctx))})
		if err != nil {
			t.Fatal(err)
		}
		return string(data)
	}
	before := snapshot()
	pc := promptcontext.BuildPromptContext(ctx, &skill.Config{Name: "finalize-answer"})
	var board string
	for _, s := range pc.UserSections {
		if s.Title == promptcontext.SectionTraceRootCauseBoard {
			board = s.Content
		}
	}
	prompt := (&answerDocumentEvaluator{}).BuildInitialInstruction(ctx, nil)
	if before != snapshot() {
		t.Fatal("context rendering mutated observations, projection, scope or model prose")
	}
	return board, prompt, types.CompileTraceCausalProjectionSet(answerDocObservationLedger(ctx))
}

func b1674LineWith(text, needle string) string {
	for _, line := range strings.Split(text, "\n") {
		if strings.Contains(line, needle) {
			return line
		}
	}
	return ""
}

func b1674FamilyNode(t *testing.T, set types.TraceCausalProjectionSet) types.TraceCausalProjectionNode {
	t.Helper()
	for _, p := range set.Projections {
		for _, node := range p.RankedSeats {
			if node.EvidenceID == "trace_query:family#root_cause_rank:1" {
				return node
			}
		}
	}
	t.Fatal("typed fixture did not produce its ranked family")
	return types.TraceCausalProjectionNode{}
}

func TestB1674PublicFinalizerKeepsFamilyRecordsWithTheirOwnValue(t *testing.T) {
	for _, lang := range []string{"en", "zh"} {
		for _, typ := range []string{"io_latency", "runnable_wait"} {
			t.Run(lang+"/"+typ, func(t *testing.T) {
				board, prompt, set := b1674FamilyPublicPrompt(t, b1674FamilyRows(typ, 47, "0.782", "sum_disjoint"), lang)
				node := b1674FamilyNode(t, set)
				if node.FamilyMemberCount != 47 || node.FamilyMemberMaxMS != .782 || node.FamilyFoldCaliber != "sum_disjoint" || node.EffectiveImpactMS != 12.658 || node.MergedCount != 0 {
					t.Fatalf("producer/compiler premise lost family identity/caliber: %+v", node)
				}
				if !strings.Contains(prompt, "proven_blocking_wall_clock=7.408ms") || !strings.Contains(prompt, "occurrence_count=10") {
					t.Fatal("independent capped account premise missing")
				}
				faces := map[string]string{
					"board":  b1674LineWith(board, "#1 root-cause seat"),
					"axisB":  b1674LineWith(prompt, "rank=#1; subject=`client-100`; kind=`"+typ+"`"),
					"reader": b1674LineWith(prompt, "Rank 1, client-100:"),
				}
				if lang == "zh" {
					faces["reader"] = b1674LineWith(prompt, "第 1 位，client-100：")
				}
				for face, line := range faces {
					if !strings.Contains(line, "12.658") {
						t.Fatalf("%s missing displayed-value positive premise: %s", face, line)
					}
					wants := []string{"47 measurement records", "record maximum 0.782 ms", "sum of disjoint intervals"}
					if face == "reader" && lang == "zh" {
						wants = []string{"47 条计量记录", "记录最大值 0.782 毫秒", "互斥区间求和"}
					}
					for _, want := range wants {
						if !strings.Contains(line, want) {
							t.Errorf("%s lost same-row family fact %q: %s", face, want, line)
						}
					}
					if strings.Contains(line, "10 measurement records") || strings.Contains(line, "10 条计量记录") {
						t.Errorf("%s borrowed the capped account count: %s", face, line)
					}
				}
			})
		}
	}
}

func TestB1674PublicFinalizerDoesNotInventUnpublishedFamilyFacts(t *testing.T) {
	for _, tc := range []struct {
		name          string
		count         int
		maximum, fold string
		displayFold   bool
	}{
		{"no family, display fold only", 0, "", "", true},
		{"unpublished maximum", 47, "", "sum_disjoint", false},
		{"zero maximum is not publication", 47, "0", "sum_disjoint", false},
		{"unknown fold", 47, "0.782", "private_future_fold", false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			rows := b1674FamilyRows("io_latency", tc.count, tc.maximum, tc.fold)
			if tc.displayFold {
				rows[1].RichNotes = append(rows[1].RichNotes, "folded_rows=98", "folded_max_ms=9.000")
			}
			board, prompt, set := b1674FamilyPublicPrompt(t, rows, "zh")
			node := b1674FamilyNode(t, set)
			if node.FamilyMemberCount != tc.count {
				t.Fatalf("family compilation changed: %+v", node)
			}
			line := b1674LineWith(prompt, "第 1 位，client-100：")
			if tc.displayFold {
				// Display folding can legitimately remove the reader-ranked
				// seat. Its typed fold and independent board remain the premise.
				if node.MergedCount != 98 || !strings.Contains(board, "12.658ms") {
					t.Fatal("display-fold negative premise missing")
				}
				if strings.Contains(line, "计量记录") || strings.Contains(line, "98") {
					t.Fatal("display-fold members were relabeled as family records")
				}
				return
			}
			if line == "" {
				t.Fatal("reader premise missing")
			}
			if tc.count == 0 && (strings.Contains(line, "计量记录") || strings.Contains(line, "98")) {
				t.Fatal("display-fold members were relabeled as family records")
			}
			if (tc.maximum == "" || tc.maximum == "0") && strings.Contains(line, "记录最大值") {
				t.Fatal("absent/zero maximum became a published measurement")
			}
			for _, face := range []string{board, b1674LineWith(prompt, "rank=#1; subject=`client-100`; kind=`io_latency`"), line} {
				if strings.Contains(face, "private_future_fold") {
					t.Fatal("unknown internal fold token leaked to an answer-writing face")
				}
			}
		})
	}
}

func TestB1674PublicFinalizerKeepsTwoWindowFamiliesSeparate(t *testing.T) {
	for _, separateCapture := range []bool{false, true} {
		for _, reverse := range []bool{false, true} {
			t.Run(fmt.Sprintf("separateCapture=%t/reverse=%t", separateCapture, reverse), func(t *testing.T) {
				rows := b1674FamilyRows("io_latency", 47, "0.782", "sum_disjoint")
				other := b1674FamilyRows("io_latency", 3, "2.001", "interval_union")[:2]
				for i := range other {
					other[i].ID = strings.ReplaceAll(other[i].ID, ":family#", ":family-other#")
					other[i].SourceRef.ToolCallID, other[i].SourceRef.PayloadRef = "second-query", "/payload/second.json"
					if separateCapture {
						other[i].SourceRef.ArtifactID, other[i].SourceRef.Path = "second.ftrace", "/captures/second.ftrace"
					}
					other[i].SourceRef.QueryWindowStartTs, other[i].SourceRef.QueryWindowEndTs = 20, 20.2
					other[i].Span.StartTs += 10
					other[i].Span.EndTs += 10
					for j, note := range other[i].RichNotes {
						note = strings.ReplaceAll(note, "10.000000..10.200000", "20.000000..20.200000")
						other[i].RichNotes[j] = strings.ReplaceAll(note, "12.658", "6.002")
					}
				}
				other[1].Value = "6.002"
				rows = append(rows, other...)
				if reverse {
					for i, j := 0, len(rows)-1; i < j; i, j = i+1, j-1 {
						rows[i], rows[j] = rows[j], rows[i]
					}
				}
				board, prompt, set := b1674FamilyPublicPrompt(t, rows, "zh", [2]float64{10, 10.2}, [2]float64{20, 20.2})
				if len(set.Projections) == 0 || !strings.Contains(board, "20.000000..20.200000") || !strings.Contains(board, "10.000000..10.200000") {
					t.Fatal("independent query/window premise missing")
				}
				shown := map[string]int{}
				for _, tc := range []struct{ value, count, max string }{{"12.658", "47", "0.782"}, {"6.002", "3", "2.001"}} {
					for face, text := range map[string]string{"board": board, "axisB": prompt, "reader": prompt} {
						needle := tc.value + "ms (effective attribution)"
						wantCount, wantMax := tc.count+" measurement records", "record maximum "+tc.max+" ms"
						if face == "axisB" {
							needle = "rank=#1; subject=`client-100`; kind=`io_latency`; effective_attribution=" + tc.value + "ms"
						}
						if face == "reader" {
							needle = "可消除影响 " + tc.value + " 毫秒"
							wantCount = tc.count + " 条计量记录"
							wantMax = "记录最大值 " + tc.max + " 毫秒"
						}
						line := b1674LineWith(text, needle)
						// Preserve the existing projection's principal-window
						// selection. A nonselected same-capture window must not
						// donate its metadata or be promoted by this display fix.
						if !separateCapture && face != "board" && line == "" {
							continue
						}
						shown[face]++
						if !strings.Contains(line, wantCount) || !strings.Contains(line, wantMax) {
							t.Errorf("%s crossed/lost query-local accounting: %s", face, line)
						}
					}
				}
				if shown["board"] != 2 || shown["axisB"] == 0 || shown["reader"] == 0 {
					t.Fatal("window identity check lost all displayed positive premises")
				}
			})
		}
	}
}

func TestB1674PublicBoardDoesNotDeduplicateDifferentFamilyAccounting(t *testing.T) {
	for _, change := range []struct{ old, new, want string }{
		{"member_count=47", "member_count=3", "3 measurement records"},
		{"member_max_ms=0.782", "member_max_ms=0.512", "record maximum 0.512 ms"},
		{"member_fold_caliber=sum_disjoint", "member_fold_caliber=interval_union", "interval union within this family"},
	} {
		for _, reverse := range []bool{false, true} {
			t.Run(change.new+fmt.Sprintf("/reverse=%t", reverse), func(t *testing.T) {
				rows := b1674FamilyRows("io_latency", 47, "0.782", "sum_disjoint")[:2]
				other := rows[1]
				other.ID = "trace_query:another#root_cause_rank:1"
				// Same board, independently published query result. The ledger's
				// own duplicate-merge rules are outside this display regression.
				other.SourceRef.ToolCallID, other.SourceRef.PayloadRef = "another-query", "/payload/another.json"
				other.RichNotes = append([]string(nil), other.RichNotes...)
				for i, n := range other.RichNotes {
					if n == change.old {
						other.RichNotes[i] = change.new
					}
				}
				rows = append(rows, other)
				if reverse {
					rows[1], rows[2] = rows[2], rows[1]
				}
				board, _, _ := b1674FamilyPublicPrompt(t, rows, "en")
				if strings.Count(board, "root-cause seat — client-100") != 2 || !strings.Contains(board, "47 measurement records") || !strings.Contains(board, change.want) {
					t.Fatalf("independent same-board facts collapsed: %s", board)
				}
				// A repeated publication with identical family facts retains the
				// original display suppression even across distinct query receipts.
				rows[2].RichNotes = append([]string(nil), rows[1].RichNotes...)
				board, _, _ = b1674FamilyPublicPrompt(t, rows, "en")
				if strings.Count(board, "root-cause seat — client-100") != 1 {
					t.Fatal("identical measurement duplicated the family")
				}
			})
		}
	}
}
