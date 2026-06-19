package agent

// P2.1 Session 1 Phase 1 — explorer evaluator harness.
//
// Purpose: lock the current explorerEvaluator behavior before the
// Turn A / Turn B split lands in P2.1 Session 2. Today's
// explorer_test.go covers helpers (containsIdentifier, primary-file
// gate, MidLoopCheck) but ParseOutput has zero tests and ShouldStop
// only has the primary-file gate scenarios. This file fills both
// gaps so the Session 2 refactor cannot silently regress quality
// floors, ERM gating, or mechanism-kind filtering.
//
// Scope deliberately limited to the four Evaluator-interface methods
// that Turn A inherits in the split: ShouldStop, ParseOutput,
// DetermineMissingPiece. (BuildInitialInstruction is well-covered by
// existing tests.) Synthesis-side behavior is out of scope here —
// see stage_report_render_test.go for deterministic StageReport
// coverage.

import (
	"fmt"
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/llm"
	"github.com/hanchaoqun/codrax/internal/tool/repomap"
	"github.com/hanchaoqun/codrax/internal/types"
)

// -----------------------------------------------------------------------------
// ShouldStop early-return paths
// -----------------------------------------------------------------------------
//
// The existing primary-file-gate tests
// (TestShouldStop_PrimaryFileReadGate_*) cover the "ERM satisfied
// AND notes show terminal evidence AND primary file gate" branch.
// They DO NOT cover the four early-return guards that come before
// that gate. The split in Session 2 keeps these guards in Turn A,
// so we pin them here.

func TestShouldStop_ToolCallsPresent_ReturnsFalse(t *testing.T) {
	eval := &explorerEvaluator{
		phase:           1,
		ermRequirements: []EvidenceRequirement{{Kind: "mechanism", Entities: []string{"Foo"}, Status: "satisfied"}},
	}
	resp := llm.Response{
		Content:   "",
		ToolCalls: []llm.ToolCall{{Name: "grep"}},
	}
	if eval.ShouldStop(resp, 5) {
		t.Fatal("ShouldStop must not fire while the LLM is still calling tools")
	}
}

func TestShouldStop_Phase0_ReturnsFalse(t *testing.T) {
	eval := &explorerEvaluator{
		phase:              0,
		ermRequirements:    []EvidenceRequirement{{Kind: "mechanism", Entities: []string{"Foo"}, Status: "satisfied"}},
		investigationNotes: []string{"## Evidence from a.go\n- [DIRECT] Foo line 1: returns true"},
	}
	if eval.ShouldStop(llm.Response{Content: "wrap-up"}, 3) {
		t.Fatal("ShouldStop must not fire in Phase 0 (breadth scan); semantic early-stop is a depth-phase concept")
	}
}

func TestShouldStop_NoERMRequirements_ReturnsFalse(t *testing.T) {
	eval := &explorerEvaluator{
		phase:              1,
		ermRequirements:    nil,
		investigationNotes: []string{"## Evidence from a.go\n- [DIRECT] Foo line 1: returns true"},
	}
	if eval.ShouldStop(llm.Response{Content: "wrap-up"}, 5) {
		t.Fatal("ShouldStop must not fire when no ERM exists; without requirements there is nothing to declare 'satisfied'")
	}
}

func TestShouldStop_NoTerminalEvidence_ReturnsFalse(t *testing.T) {
	// ERM "satisfied", primary-file gate skipped (no graph symbol in
	// graph fixture), but investigationNotes contain only [MECHANISM]
	// / [CONDITIONAL] tags — no [DIRECT] / [REGISTRATION]. Without
	// terminal evidence the loop must keep running.
	eval := &explorerEvaluator{
		phase:              1,
		searchResult:       &keywordSearchResult{Graph: &repomap.Graph{SymbolDefs: map[string][]*repomap.Symbol{}}},
		ermRequirements:    []EvidenceRequirement{{Kind: "mechanism", Entities: []string{"abstract concept"}, Status: "satisfied"}},
		investigationNotes: []string{"## Evidence from a.go\n- [MECHANISM] line 10: invokes the analyzer\n- [CONDITIONAL] line 20: when X then Y"},
	}
	if eval.ShouldStop(llm.Response{Content: "wrap-up"}, 5) {
		t.Fatal("ShouldStop must not fire without terminal evidence (no [DIRECT] / [REGISTRATION] in notes)")
	}
}

func TestShouldStop_AllCriteriaMet_ReturnsTrue(t *testing.T) {
	// Concept-word ERM (skips the primary-file gate via the
	// no-graph-symbol branch). ERM satisfied. Notes carry a
	// [REGISTRATION] tag — this is what hasTerminalEvidence accepts
	// as terminal (see erm.go:hasTerminalEvidence — registration shape
	// is the canonical terminal; bare [DIRECT] does not qualify). Phase
	// 1, no tool calls. Expected: S1 fires.
	eval := &explorerEvaluator{
		phase:              1,
		searchResult:       &keywordSearchResult{Graph: &repomap.Graph{SymbolDefs: map[string][]*repomap.Symbol{}}},
		ermRequirements:    []EvidenceRequirement{{Kind: "registration", Entities: []string{"abstract concept"}, Status: "satisfied"}},
		investigationNotes: []string{"## Evidence from a.go\n- [REGISTRATION] line 10: Register binds Foo to NewFoo"},
	}
	if !eval.ShouldStop(llm.Response{Content: "wrap-up"}, 5) {
		t.Fatal("ShouldStop must fire when ERM satisfied + [REGISTRATION] terminal evidence + concept-word entities skip primary gate")
	}
}

func TestExplorer_BuildInitialInstruction_AttributeBearingEnumerationIsCrossLanguage(t *testing.T) {
	ctx := &types.AgentContext{
		Objective: "list all modules and each module entry point",
		RepoRoot:  ".",
		Mutable:   types.NewMutableState(""),
		AnalysisIR: &types.AnalysisIR{
			RequestModel: types.RequestModel{
				RawRequest: "list all modules and each module entry point",
				Intent:     types.IntentEnumerate,
				Predicates: types.SemanticPredicates{
					IsCategoryEnumeration: true,
					IsRelationalLookup:    true,
				},
				PredicateAxis: types.AxisCall,
				EnumerationBoundary: &types.RequestedEnumerationBoundary{
					DeclaredCount: 4,
					SourceQuote:   "all modules",
				},
				CompletenessObligation: &types.CompletenessObligation{
					Required:    true,
					SourceQuote: "all modules",
				},
			},
		},
	}
	prompt := (&explorerEvaluator{}).BuildInitialInstruction(ctx, nil)
	for _, want := range []string{
		"Attribute-bearing Enumeration Discipline",
		"Support all language surfaces",
		"Do not assume Go-only",
		"unresolved-attribute note",
		"Do not keep searching solely to prove a unique attribute",
	} {
		if !strings.Contains(prompt, want) {
			t.Fatalf("attribute-bearing explorer prompt missing %q:\n%s", want, prompt)
		}
	}
}

func TestExplorer_BuildInitialInstruction_RendersSourceInventoryAdvisory(t *testing.T) {
	mut := types.NewMutableState("list package entrypoints")
	mut.SetSourceInventoryAdvisory(types.SourceInventoryAdvisory{
		Active:       true,
		AdvisoryOnly: true,
		Complete:     true,
		Scopes:       []string{"src"},
		Provenance:   []string{"pre_explore_typed_request"},
		Sets: []types.SourceInventoryAdvisorySet{{
			Role:     types.AnswerCandidateRoleFunction,
			Complete: true,
			Candidates: []types.SourceInventoryAdvisoryCandidate{{
				Member:   "Run",
				File:     "src/run.py",
				Line:     7,
				Language: "python",
				Role:     types.AnswerCandidateRoleFunction,
			}},
		}},
	})
	ctx := &types.AgentContext{
		Objective: "list package entrypoints",
		Mutable:   mut,
		AnalysisIR: &types.AnalysisIR{RequestModel: types.RequestModel{
			Intent: types.IntentEnumerate,
			Predicates: types.SemanticPredicates{
				IsCategoryEnumeration: true,
			},
			SourceInventoryProfile: &types.SourceInventoryProfile{
				IsSourceInventory: true,
				TargetRoles:       []types.AnswerCandidateRole{types.AnswerCandidateRoleFunction},
				Confidence:        0.95,
			},
		}},
	}

	prompt := (&explorerEvaluator{}).BuildInitialInstruction(ctx, nil)
	for _, want := range []string{
		"Structured Source Inventory Progress",
		"verified navigation/candidate-universe facts",
		"repo-lens observation invariants",
		"Cascaded Repo Lens Guide",
		`"view": "source_inventory"`,
		"count=1 len(members)=1",
		"`Run`",
		"src/run.py:7",
	} {
		if !strings.Contains(prompt, want) {
			t.Fatalf("prompt missing %q:\n%s", want, prompt)
		}
	}
}

func TestExplorer_BuildInitialInstruction_CheckpointContinuationSkipsBreadthScan(t *testing.T) {
	ctx := &types.AgentContext{
		Objective: "explain the current implementation from the accepted evidence",
		RepoRoot:  ".",
		Mutable:   types.NewMutableState("explain the current implementation from the accepted evidence"),
		RetryHint: "A transient model stream error interrupted the previous explore dispatch after durable progress was preserved.\n\nCheckpoint summary (non-authoritative counts): structured evidence rows=4; aggregate facts=1.",
	}
	prompt := (&explorerEvaluator{}).BuildInitialInstruction(ctx, nil)
	for _, want := range []string{
		"Checkpoint Continuation",
		"not a fresh investigation",
		"call `emit_investigation_complete",
		"one narrow `grep` / `read_file` / `repo_map` follow-up",
	} {
		if !strings.Contains(prompt, want) {
			t.Fatalf("checkpoint continuation prompt missing %q:\n%s", want, prompt)
		}
	}
	for _, forbidden := range []string{
		"## Breadth Scan",
		"produce a FILE LIST",
		"Search broadly",
	} {
		if strings.Contains(prompt, forbidden) {
			t.Fatalf("checkpoint continuation must not render fresh breadth prompt %q:\n%s", forbidden, prompt)
		}
	}
}

func TestExplorer_BuildInitialInstruction_ObservationOnlyTraceStartsWithTraceQuery(t *testing.T) {
	ctx := &types.AgentContext{
		Objective: `分析 record_trace_20260526174055.systrace 中 com.tencent.mm-36379 在 2942.124416 到 2942.260210 的 sleep 原因`,
		Stage:     types.StageExplore,
		AnalysisIR: &types.AnalysisIR{
			RequestModel: types.RequestModel{
				PerfTrace: &types.PerfBundle{
					Frames: []types.PerfFrame{{FrameNo: 1, DurationMs: 33.3, Janky: true}},
				},
				ExternalObservationPolicy: &types.ExternalObservationPolicy{
					CurrentSourceMode: types.ExternalObservationCurrentSourceExclude,
					SourceQuotes:      []string{"只分析 trace"},
					Confidence:        0.9,
				},
			},
		},
	}

	eval := &explorerEvaluator{}
	prompt := eval.BuildInitialInstruction(ctx, nil)
	if eval.phase != 1 {
		t.Fatalf("explicit trace path should skip generic breadth scan, got phase=%d", eval.phase)
	}
	for _, want := range []string{
		"Explicit Runtime Trace Path Start",
		"Start with `trace_query`",
		"`state_churn` context",
		"fragmented state-churn causes",
		"dominant_state",
		"`cumulative_impact_ms`",
		"compare same-chain rows by `cumulative_impact_ms` before `score`",
		"follow the rendered `next_step`",
		"`occurrence_windows`",
		"representative repeated windows",
		"`sync_like`",
		"`blocking_candidate`",
		"pattern=\"<literal>\"",
		"not regex",
		"carry that same `time_start`/`time_end`",
		"thread/pid alone is not a bounded query",
		"Trace timestamps are seconds",
		"trace_query` result already publishes typed runtime-artifact observations",
		"do not call `emit_evidence` just to repackage those rows",
		"Complete with `emit_investigation_complete`",
	} {
		if !strings.Contains(prompt, want) {
			t.Fatalf("explicit trace start prompt missing %q:\n%s", want, prompt)
		}
	}
	if strings.Contains(prompt, "## Breadth Scan") {
		t.Fatalf("explicit trace path should not render fresh source breadth prompt:\n%s", prompt)
	}
}

func TestExplorer_BuildInitialInstruction_ExplicitTracePathKeepsNormalBreadthByDefault(t *testing.T) {
	ctx := &types.AgentContext{
		Objective: `分析 record_trace_20260526174055.systrace 中 com.tencent.mm-36379 的 sleep 原因`,
		Stage:     types.StageExplore,
	}

	eval := &explorerEvaluator{}
	prompt := eval.BuildInitialInstruction(ctx, nil)
	if strings.Contains(prompt, "Explicit Runtime Trace Path Start") {
		t.Fatalf("mixed trace+source request must not use runtime-only trace start:\n%s", prompt)
	}
	if !strings.Contains(prompt, "## Breadth Scan") {
		t.Fatalf("mixed trace+source request should keep normal source exploration path:\n%s", prompt)
	}
}

func TestExplorer_BuildInitialInstruction_SourceOptionalTraceStartsWithTraceQuery(t *testing.T) {
	rm := types.RequestModel{
		Intent:   types.IntentRootCause,
		Scenario: types.ScenarioPerformanceBottleneck,
		PerfTrace: &types.PerfBundle{Observations: []types.PerfObservation{{
			Kind:    "attached_trace",
			Subject: "com.baidu.tieba-59566",
			Summary: "attached runtime trace artifact is present",
		}}},
		ExternalObservationPolicy: &types.ExternalObservationPolicy{
			CurrentSourceMode: types.ExternalObservationCurrentSourceDefault,
			Confidence:        0.8,
		},
	}
	ctx := &types.AgentContext{
		Objective:  `分析 attached trace 中 com.baidu.tieba-59566 的卡顿原因`,
		Stage:      types.StageExplore,
		PerfTrace:  rm.PerfTrace,
		AnalysisIR: &types.AnalysisIR{RequestModel: rm},
	}

	eval := &explorerEvaluator{}
	prompt := eval.BuildInitialInstruction(ctx, nil)
	if eval.phase != 1 {
		t.Fatalf("source-optional runtime trace should skip breadth scan, got phase=%d", eval.phase)
	}
	for _, want := range []string{
		"Explicit Runtime Trace Path Start",
		"Start with `trace_query`",
		"Treat this as a runtime-artifact investigation first",
		"trace_query` result already publishes typed runtime-artifact observations",
		"do not call `emit_evidence` just to repackage those rows",
		"Complete with `emit_investigation_complete`",
	} {
		if !strings.Contains(prompt, want) {
			t.Fatalf("source-optional trace start prompt missing %q:\n%s", want, prompt)
		}
	}
	if strings.Contains(prompt, "## Breadth Scan") {
		t.Fatalf("source-optional runtime trace should not render source breadth prompt:\n%s", prompt)
	}
}

func TestExplorer_BuildInitialInstruction_SourceOptionalLogDoesNotUseTraceQueryStart(t *testing.T) {
	rm := types.RequestModel{
		Intent:   types.IntentRootCause,
		Scenario: types.ScenarioRootCause,
		LogTriage: &types.LogBundle{Errors: []types.LogError{{
			Type:    "panic",
			Message: "runtime error in external process",
		}}},
		ExternalObservationPolicy: &types.ExternalObservationPolicy{
			CurrentSourceMode: types.ExternalObservationCurrentSourceDefault,
			Confidence:        0.8,
		},
	}
	ctx := &types.AgentContext{
		Objective:  `分析 attached log 的异常原因`,
		Stage:      types.StageExplore,
		LogTriage:  rm.LogTriage,
		AnalysisIR: &types.AnalysisIR{RequestModel: rm},
	}

	eval := &explorerEvaluator{}
	prompt := eval.BuildInitialInstruction(ctx, nil)
	if eval.phase != 1 {
		t.Fatalf("source-optional runtime log should skip breadth scan, got phase=%d", eval.phase)
	}
	if !strings.Contains(prompt, "Runtime Artifact Only Start") {
		t.Fatalf("source-optional runtime log should use runtime artifact start:\n%s", prompt)
	}
	if strings.Contains(prompt, "Explicit Runtime Trace Path Start") || strings.Contains(prompt, "Start with `trace_query`") {
		t.Fatalf("source-optional runtime log must not use trace_query start:\n%s", prompt)
	}
	if strings.Contains(prompt, "## Breadth Scan") {
		t.Fatalf("source-optional runtime log should not render source breadth prompt:\n%s", prompt)
	}
}

func TestExplorer_BuildInitialInstruction_ExternalObservationFirstSkipsSourceBreadth(t *testing.T) {
	ctx := &types.AgentContext{
		Objective: `请解释 MCP 资源 mcp://fixture/trace/sleep-wakeup 的 line 7 和 line 12`,
		Stage:     types.StageExplore,
		TurnRouteHint: types.TurnRouteHint{
			Route:      "repo",
			Source:     "external_tool",
			Confidence: 0.92,
		},
		AnalysisIR: &types.AnalysisIR{
			RequestModel: types.RequestModel{},
		},
	}

	eval := &explorerEvaluator{}
	prompt := eval.BuildInitialInstruction(ctx, nil)
	if eval.phase != 1 {
		t.Fatalf("external-observation-first should skip source breadth, got phase=%d", eval.phase)
	}
	for _, want := range []string{
		"External Observation First Start",
		"typed external observations",
		"line, row, selector",
		"emit_investigation_complete",
		"Do not run repository breadth search",
	} {
		if !strings.Contains(prompt, want) {
			t.Fatalf("external-observation prompt missing %q:\n%s", want, prompt)
		}
	}
	if strings.Contains(prompt, "## Breadth Scan") {
		t.Fatalf("external-observation-first should not render source breadth prompt:\n%s", prompt)
	}
}

func TestExplorer_BuildInitialInstruction_ExternalObservationFirstKeepsBreadthWhenSourceRequired(t *testing.T) {
	ctx := &types.AgentContext{
		Objective: `请用 MCP 资源并结合当前源码解释实现`,
		Stage:     types.StageExplore,
		TurnRouteHint: types.TurnRouteHint{
			Route:      "repo",
			Source:     "external_tool",
			Confidence: 0.92,
		},
		AnalysisIR: &types.AnalysisIR{
			RequestModel: types.RequestModel{
				CurrentSourceExplanationProfile: &types.CurrentSourceExplanationProfile{
					IsCurrentSourceExplanationRequested: true,
					Modes: []types.CurrentSourceExplanationMode{
						types.CurrentSourceExplanationExplainCurrentMechanism,
					},
					SourceQuotes: []string{"结合当前源码"},
					Confidence:   0.9,
				},
			},
		},
	}

	prompt := (&explorerEvaluator{}).BuildInitialInstruction(ctx, nil)
	if strings.Contains(prompt, "External Observation First Start") {
		t.Fatalf("current-source-required turn must not use external-only start:\n%s", prompt)
	}
	if !strings.Contains(prompt, "## Breadth Scan") {
		t.Fatalf("current-source-required turn should keep source breadth prompt:\n%s", prompt)
	}
}

func TestRenderExtractorSourceInventoryAdvisory_RendersCandidateAttributes(t *testing.T) {
	out := renderExtractorSourceInventoryAdvisory(&types.TurnAArtifacts{
		SourceInventoryAdvisory: types.SourceInventoryAdvisory{
			Active:       true,
			AdvisoryOnly: true,
			Sets: []types.SourceInventoryAdvisorySet{{
				Role:     types.AnswerCandidateRolePackage,
				Complete: true,
				Candidates: []types.SourceInventoryAdvisoryCandidate{{
					Member:   "alpha",
					File:     "src/alpha",
					Language: "python",
					Role:     types.AnswerCandidateRolePackage,
					Attributes: []types.SourceInventoryAdvisoryAttribute{{
						Member:   "run_alpha",
						File:     "src/alpha/a.py",
						Line:     7,
						Language: "python",
						Role:     types.AnswerCandidateRoleFunction,
					}},
				}},
			}},
		},
	})
	for _, want := range []string{
		"package/directory/module scope candidates",
		"Cascaded Repo Lens Guide",
		"languages=python:2",
		"count=1 len(members)=1",
		"related_candidate_attributes",
		"function `run_alpha` @ src/alpha/a.py:7",
	} {
		if !strings.Contains(out, want) {
			t.Fatalf("rendered advisory missing %q:\n%s", want, out)
		}
	}
}

func TestExplorerSourceInventoryPrompt_RendersObservationOnlyRows(t *testing.T) {
	mut := types.NewMutableState("list mixed-language entrypoints")
	mut.SetSourceInventoryObservation(types.SourceInventoryObservation{
		Active:       true,
		AdvisoryOnly: true,
		Complete:     true,
		Scopes:       []string{"src"},
		Provenance:   []string{"repo_lens:tool_query"},
		Sets: []types.SourceInventoryObservationSet{{
			Role:     types.AnswerCandidateRoleFunction,
			Complete: true,
			Count:    1,
			Members: []types.SourceInventoryObservationMember{{
				Name:          "run_alpha",
				Key:           "run_alpha",
				SupportRef:    "run_alpha: src/alpha/a.py:7",
				Role:          types.AnswerCandidateRoleFunction,
				File:          "src/alpha/a.py",
				Line:          7,
				Language:      "python",
				CoverageState: types.SourceInventoryCoverageObserved,
			}},
		}},
	})
	ctx := &types.AgentContext{
		Objective: "list mixed-language entrypoints",
		Mutable:   mut,
		AnalysisIR: &types.AnalysisIR{RequestModel: types.RequestModel{
			Intent: types.IntentEnumerate,
			Predicates: types.SemanticPredicates{
				IsCategoryEnumeration: true,
			},
		}},
	}

	prompt := (&explorerEvaluator{}).BuildInitialInstruction(ctx, nil)
	for _, want := range []string{
		"Structured Source Inventory Progress",
		"repo-lens observation invariants",
		"Cascaded Repo Lens Guide",
		"role `function`",
		"count=1 len(members)=1",
		"`run_alpha`",
		"src/alpha/a.py:7",
		"coverage_state=observed",
	} {
		if !strings.Contains(prompt, want) {
			t.Fatalf("observation-only prompt missing %q:\n%s", want, prompt)
		}
	}
}

func TestExplorerBuildInitialInstruction_RendersWriteExplorationRequest(t *testing.T) {
	mut := types.NewMutableState("add retry telemetry")
	mut.SetWriteExplorationRequest(&types.WriteExplorationRequest{
		BatchID:              "batch-2",
		Goal:                 "understand existing retry telemetry before changing it",
		ExplorationQuestions: []string{"Where is retry telemetry recorded?"},
		CandidatePaths:       []string{"internal/orchestrator/retry_state.go"},
		Constraints:          []string{"read-only exploration"},
		EvidenceRequirements: []string{"identify tests that should change"},
	})
	ctx := &types.AgentContext{
		Objective: "add retry telemetry",
		Mutable:   mut,
		RepoRoot:  "/tmp/repo",
	}

	prompt := (&explorerEvaluator{}).BuildInitialInstruction(ctx, nil)
	reqIdx := strings.Index(prompt, "## Write-mode targeted source exploration")
	breadthIdx := strings.Index(prompt, "## Breadth Scan")
	if reqIdx < 0 {
		t.Fatalf("write exploration request section missing:\n%s", prompt)
	}
	if breadthIdx < 0 || reqIdx > breadthIdx {
		t.Fatalf("write exploration request should appear before breadth scan; req=%d breadth=%d\n%s", reqIdx, breadthIdx, prompt)
	}
	for _, want := range []string{
		"batch-2",
		"understand existing retry telemetry",
		"Where is retry telemetry recorded?",
		"internal/orchestrator/retry_state.go",
		"identify tests that should change",
		"read-only source exploration subflow",
	} {
		if !strings.Contains(prompt, want) {
			t.Fatalf("write exploration prompt missing %q:\n%s", want, prompt)
		}
	}
}

func TestRenderExtractorSourceInventoryAdvisory_RendersObservationOnlyRows(t *testing.T) {
	out := renderExtractorSourceInventoryAdvisory(&types.TurnAArtifacts{
		SourceInventoryObservation: types.SourceInventoryObservation{
			Active:       true,
			AdvisoryOnly: true,
			Complete:     true,
			Scopes:       []string{"routes"},
			Provenance:   []string{"repo_lens:tool_query"},
			Sets: []types.SourceInventoryObservationSet{{
				Role:     types.AnswerCandidateRoleRoute,
				Complete: true,
				Count:    1,
				Members: []types.SourceInventoryObservationMember{{
					Name:          "GET /v1/users",
					Key:           "GET /v1/users",
					SupportRef:    "GET /v1/users: routes/users.ts:42",
					Role:          types.AnswerCandidateRoleRoute,
					File:          "routes/users.ts",
					Line:          42,
					Language:      "typescript",
					CoverageState: types.SourceInventoryCoverageObserved,
					Attributes: []types.SourceInventoryObservationAttribute{{
						Name:          "handleUsers",
						SupportRef:    "handleUsers: routes/users.ts:43",
						Role:          types.AnswerCandidateRoleFunction,
						File:          "routes/users.ts",
						Line:          43,
						Language:      "typescript",
						CoverageState: types.SourceInventoryCoverageObserved,
					}},
				}},
			}},
		},
	})
	for _, want := range []string{
		"repo-lens observation invariants",
		"Cascaded Repo Lens Guide",
		"languages=typescript:2",
		"role `route`",
		"observation rows",
		"`GET /v1/users`",
		"routes/users.ts:42",
		"related_candidate_attributes",
		"function `handleUsers` @ routes/users.ts:43",
	} {
		if !strings.Contains(out, want) {
			t.Fatalf("observation-only advisory missing %q:\n%s", want, out)
		}
	}
}

// -----------------------------------------------------------------------------
// ParseOutput quality-floor branches
// -----------------------------------------------------------------------------
//
// ParseOutput today has zero direct test coverage. The split in
// Session 2 will gut this method (Turn A keeps coverage tracking,
// Turn B owns AnswerSymbol emission via emit_answer_symbol). Pin
// each surviving branch here so the refactor's diff makes the
// regression visible.
//
// Test fixtures pre-populate e.structuredEvidence so
// ensureStructuredEvidence early-returns without touching the
// dataflow / concrete-values pipeline (those are independently
// tested elsewhere).

// parseOutputCtx builds a minimal AgentContext suitable for
// driving ParseOutput. The kind is settable so each test can
// switch between mechanism and other kinds. The trailing string
// argument was once the AnalyzerHints.Shape; that field is gone
// with the AnswerShape retirement, so callers pass any value
// (it is ignored).
func parseOutputCtx(kind, _ string) *types.AgentContext {
	return &types.AgentContext{
		Objective: "test question",
		RepoRoot:  ".",
		AnalysisIR: &types.AnalysisIR{
			RequestModel: types.RequestModel{
				AnalyzerHints: types.AnalyzerHints{Kind: kind},
			},
		},
	}
}

// pinnedEvidenceItem returns a [DIRECT]-shaped EvidenceItem that
// looks "real enough" for downstream ranking and answer-chain
// extraction without invoking the full extraction pipeline.
func pinnedEvidenceItem(file, subject string, line int) types.EvidenceItem {
	item := types.EvidenceItem{
		Kind:       types.EvidenceDirect,
		Subject:    subject,
		Predicate:  "returns",
		Object:     "true",
		Source:     file,
		LineStart:  line,
		LineEnd:    line,
		Producer:   "test.fixture",
		Scope:      types.ScopeLine,
		Confidence: 0.9,
	}
	item.ID = types.StableEvidenceID(item)
	return item
}

func TestParseOutput_SingleSourceBypass(t *testing.T) {
	// Single evidence tool type (grep only) is accepted via the
	// single-source investigation bypass — simple queries answered by
	// one tool type don't need multi-tool diversity.
	eval := &explorerEvaluator{
		userQuestion:       "question",
		structuredEvidence: []types.EvidenceItem{pinnedEvidenceItem("a.go", "Foo", 1)},
		investigationNotes: []string{"## Evidence from a.go\n- [DIRECT] Foo line 1: returns true\n- [DIRECT] Foo line 2: returns false"},
	}
	out, err := eval.ParseOutput(parseOutputCtx("", ""), nil, []types.ToolResult{
		{ToolName: "grep", Success: true, Summary: "a.go:1\na.go:2"},
	}, nil)
	if err != nil {
		t.Fatalf("ParseOutput error: %v", err)
	}
	if out.SignalUpdates == nil || !out.SignalUpdates.HasEnoughFacts {
		t.Fatal("hasEnough must be true for single-source investigation (bypass)")
	}
}

func TestParseOutput_EvidenceQualityFails(t *testing.T) {
	// Tool diversity OK (grep + read_file), file coverage OK (3 reads),
	// but investigation notes carry only one [DIRECT] tag (need ≥2).
	eval := &explorerEvaluator{
		userQuestion:       "question",
		structuredEvidence: []types.EvidenceItem{pinnedEvidenceItem("a.go", "Foo", 1)},
		investigationNotes: []string{"## Evidence from a.go\n- [DIRECT] Foo line 1: returns true"},
	}
	out, err := eval.ParseOutput(parseOutputCtx("", ""), nil, []types.ToolResult{
		{ToolName: "grep", Success: true, Summary: "a.go:1\nb.go:1\nc.go:1"},
		{ToolName: "read_file", Success: true, Summary: "a.go (full file)\nb.go (full file)\nc.go (full file)"},
	}, nil)
	if err != nil {
		t.Fatalf("ParseOutput error: %v", err)
	}
	if out.SignalUpdates == nil || out.SignalUpdates.HasEnoughFacts {
		t.Fatal("hasEnough must be false when fewer than 2 [DIRECT]/[REGISTRATION] tags present")
	}
	if !strings.Contains(out.RetryHint, "[DIRECT]") {
		t.Errorf("RetryHint should diagnose evidence-quality failure, got: %q", out.RetryHint)
	}
}

func TestParseOutput_MCPObservationsDoNotBypassDefaultSourceQuestion(t *testing.T) {
	question := "请使用 MCP fixture 工具查询 mcp://fixture/trace/sleep-wakeup 的 line 7 和 line 12"
	ctx := parseOutputCtx("", "")
	ctx.Objective = question
	ctx.AnalysisIR.RequestModel.RawRequest = question
	ctx.Mutable = types.NewMutableState(question)
	eval := &explorerEvaluator{userQuestion: question}

	out, err := eval.ParseOutput(ctx, nil, nil, []types.MCPResponse{{
		ServerName:  "fixture",
		Method:      "tools/call",
		ResourceURI: "mcp://fixture/trace/sleep-wakeup",
		Success:     true,
		Observations: []types.MCPTypedObservation{
			{Summary: "target enters S sleep", ResourceURI: "mcp://fixture/trace/sleep-wakeup", LineStart: 7, LineEnd: 7},
			{Summary: "helper wakes target", ResourceURI: "mcp://fixture/trace/sleep-wakeup", LineStart: 12, LineEnd: 12},
		},
	}})
	if err != nil {
		t.Fatalf("ParseOutput error: %v", err)
	}
	if out.SignalUpdates != nil && out.SignalUpdates.HasEnoughFacts {
		t.Fatalf("MCP observations alone must not satisfy the default mixed source+external request, got %+v", out.SignalUpdates)
	}
	ta := ctx.Mutable.TurnAArtifacts()
	if ta == nil || ta.RuntimeObservationOnlyCompletion {
		t.Fatalf("default MCP resource request must not mark Turn A as observation-only complete, got %+v", ta)
	}
	if ta == nil || len(ta.MCPResponses) != 1 {
		t.Fatalf("Turn A handoff should preserve MCP responses for scheduler/finalizer, got %+v", ta)
	}
}

func TestExplorer_ObserveExternalObservationSufficiencyHintsCompletion(t *testing.T) {
	question := "请解释 MCP fixture 返回的 line 7 和 line 12"
	ctx := parseOutputCtx("", "")
	ctx.Objective = question
	ctx.TurnRouteHint = types.TurnRouteHint{
		Route:      "repo",
		Source:     "external_tool",
		Confidence: 0.9,
	}
	ctx.Mutable = types.NewMutableState(question)

	eval := &explorerEvaluator{}
	_ = eval.BuildInitialInstruction(ctx, nil)
	sig := eval.Observe(ctx, LoopObservation{
		Phase:     PhaseMidLoop,
		Iteration: 0,
		AllMCPResponses: []types.MCPResponse{{
			ServerName:  "fixture",
			Method:      "tools/call",
			ResourceURI: "mcp://fixture/trace/sleep-wakeup",
			Success:     true,
			Observations: []types.MCPTypedObservation{
				{Summary: "target enters S sleep", ResourceURI: "mcp://fixture/trace/sleep-wakeup", LineStart: 7, LineEnd: 7},
				{Summary: "helper wakes target", ResourceURI: "mcp://fixture/trace/sleep-wakeup", LineStart: 12, LineEnd: 12},
			},
		}},
	})
	if !sig.HintRequested {
		t.Fatalf("expected external observation sufficiency hint, got %+v", sig)
	}
	if sig.HintKey != "explorer.mid-loop.external-observation-sufficient" {
		t.Fatalf("unexpected hint key: %q", sig.HintKey)
	}
	if !strings.Contains(sig.Hint, "current-source lane is optional") || !strings.Contains(sig.Hint, "emit_investigation_complete") {
		t.Fatalf("hint should steer completion without source sidecars:\n%s", sig.Hint)
	}
	if !sig.BypassThrottle || !sig.BypassBudget {
		t.Fatalf("sufficiency hint should bypass throttle/budget, got %+v", sig)
	}
}

func TestParseOutput_MCPObservationsSatisfyExternalObservationFirstSourceOptional(t *testing.T) {
	question := "请解释 MCP fixture 返回的 line 7 和 line 12"
	ctx := parseOutputCtx("", "")
	ctx.Objective = question
	ctx.AnalysisIR.RequestModel.RawRequest = question
	ctx.TurnRouteHint = types.TurnRouteHint{
		Route:      "repo",
		Source:     "external_tool",
		Confidence: 0.9,
	}
	ctx.Mutable = types.NewMutableState(question)
	eval := &explorerEvaluator{
		userQuestion:  question,
		analysisIR:    ctx.AnalysisIR,
		turnRouteHint: ctx.TurnRouteHint,
	}

	out, err := eval.ParseOutput(ctx, nil, nil, []types.MCPResponse{{
		ServerName:  "fixture",
		Method:      "tools/call",
		ResourceURI: "mcp://fixture/trace/sleep-wakeup",
		Success:     true,
		Observations: []types.MCPTypedObservation{
			{Summary: "target enters S sleep", ResourceURI: "mcp://fixture/trace/sleep-wakeup", LineStart: 7, LineEnd: 7},
			{Summary: "helper wakes target", ResourceURI: "mcp://fixture/trace/sleep-wakeup", LineStart: 12, LineEnd: 12},
		},
	}})
	if err != nil {
		t.Fatalf("ParseOutput error: %v", err)
	}
	if out.SignalUpdates == nil || !out.SignalUpdates.HasEnoughFacts {
		t.Fatalf("external-observation-first MCP rows should satisfy facts, got %+v", out.SignalUpdates)
	}
	ta := ctx.Mutable.TurnAArtifacts()
	if ta == nil || len(ta.MCPResponses) != 1 {
		t.Fatalf("Turn A handoff should preserve MCP responses, got %+v", ta)
	}
	if ta.RuntimeObservationOnlyCompletion {
		t.Fatalf("external-observation-first sufficiency is not the same as runtime-only completion: %+v", ta)
	}
}

func TestParseOutput_MCPObservationsDoNotBypassMixedSourceQuestion(t *testing.T) {
	question := "请使用 MCP fixture 工具查询 mcp://fixture/trace/sleep-wakeup，并结合源码解释实现"
	ctx := parseOutputCtx("", "")
	ctx.Objective = question
	ctx.AnalysisIR.RequestModel.RawRequest = question
	ctx.AnalysisIR.RequestModel.CurrentSourceExplanationProfile = &types.CurrentSourceExplanationProfile{
		IsCurrentSourceExplanationRequested: true,
		Modes: []types.CurrentSourceExplanationMode{
			types.CurrentSourceExplanationExplainCurrentMechanism,
		},
		SourceQuotes: []string{"结合源码"},
		Confidence:   0.9,
	}
	eval := &explorerEvaluator{userQuestion: question}

	out, err := eval.ParseOutput(ctx, nil, nil, []types.MCPResponse{{
		ServerName:  "fixture",
		Method:      "tools/call",
		ResourceURI: "mcp://fixture/trace/sleep-wakeup",
		Success:     true,
		Observations: []types.MCPTypedObservation{
			{Summary: "target enters S sleep", ResourceURI: "mcp://fixture/trace/sleep-wakeup", LineStart: 7, LineEnd: 7},
		},
	}})
	if err != nil {
		t.Fatalf("ParseOutput error: %v", err)
	}
	if out.SignalUpdates != nil && out.SignalUpdates.HasEnoughFacts {
		t.Fatal("MCP observations alone must not satisfy a mixed MCP+source question")
	}
}

func TestParseOutput_EnumerationEvidenceQualityHintUsesDynamicFloor(t *testing.T) {
	var evidence []types.EvidenceItem
	var notes []string
	for i := 0; i < 4; i++ {
		file := fmt.Sprintf("pkg/f%d.go", i)
		subject := fmt.Sprintf("Thing%d", i)
		evidence = append(evidence, pinnedEvidenceItem(file, subject, i+1))
		notes = append(notes, fmt.Sprintf("- [DIRECT] %s line %d: returns true", subject, i+1))
	}
	var grepLines []string
	for i := 0; i < 15; i++ {
		grepLines = append(grepLines, fmt.Sprintf("pkg/f%d.go:1:match", i))
	}
	eval := &explorerEvaluator{
		userQuestion:       "list all things",
		isEnumerationQuery: true,
		structuredEvidence: evidence,
		investigationNotes: []string{strings.Join(notes, "\n")},
	}
	out, err := eval.ParseOutput(parseOutputCtx(string(types.ReqEnumeration), ""), nil, []types.ToolResult{
		{ToolName: "grep", Success: true, Summary: strings.Join(grepLines, "\n")},
		{ToolName: "read_file", Success: true, Summary: "[pkg/f0.go: showing lines 1-20 of 20 total]\n"},
		{ToolName: "read_file", Success: true, Summary: "[pkg/f1.go: showing lines 1-20 of 20 total]\n"},
		{ToolName: "read_file", Success: true, Summary: "[pkg/f2.go: showing lines 1-20 of 20 total]\n"},
	}, nil)
	if err != nil {
		t.Fatalf("ParseOutput error: %v", err)
	}
	if out.SignalUpdates == nil || out.SignalUpdates.HasEnoughFacts {
		t.Fatal("enumeration query must keep exploring when direct evidence is below the dynamic floor")
	}
	if !strings.Contains(out.RetryHint, "needs ≥5") {
		t.Fatalf("RetryHint should report the dynamic evidence floor, got: %q", out.RetryHint)
	}
	if strings.Contains(out.RetryHint, "need ≥2") {
		t.Fatalf("RetryHint must not report the generic floor for enumeration coverage, got: %q", out.RetryHint)
	}
}

func TestParseOutput_MechanismEvidenceDoesNotUseDirectRegistrationFloor(t *testing.T) {
	ctx := parseOutputCtx(string(types.ReqMechanism), "")
	ctx.AnalysisIR.RequestModel.Intent = types.IntentExplain
	ctx.AnalysisIR.RequestModel.Scenario = types.ScenarioArchitectureExplain
	ctx.AnalysisIR.RequestModel.Predicates.IsCrossComponent = true
	eval := &explorerEvaluator{
		userQuestion:       "compare mechanisms and output logical views",
		isEnumerationQuery: true, // stale/over-broad flag must not impose enumeration floors.
		analysisIR:         ctx.AnalysisIR,
		structuredEvidence: []types.EvidenceItem{
			{
				Kind:            types.EvidenceMechanism,
				Subject:         "GroundItem",
				Predicate:       "validates",
				Source:          "a.go",
				LineStart:       10,
				GroundingStatus: types.GroundingGrounded,
				Summary:         "GroundItem validates evidence",
			},
			{
				Kind:            types.EvidenceMechanism,
				Subject:         "OpenAIResponsesLanguageModel",
				Predicate:       "routes",
				Source:          "b.go",
				LineStart:       20,
				GroundingStatus: types.GroundingGrounded,
				Summary:         "OpenAIResponsesLanguageModel routes requests",
			},
		},
	}
	out, err := eval.ParseOutput(ctx, nil, []types.ToolResult{
		{ToolName: "grep", Success: true, Summary: "a.go:10:match\nb.go:20:match\nc.go:30:match"},
		{ToolName: "read_file", Success: true, Summary: "[a.go: showing lines 1-50 of 50 total]"},
		{ToolName: "read_file", Success: true, Summary: "[b.go: showing lines 1-50 of 50 total]"},
		{ToolName: "read_file", Success: true, Summary: "[c.go: showing lines 1-50 of 50 total]"},
	}, nil)
	if err != nil {
		t.Fatalf("ParseOutput error: %v", err)
	}
	if out.SignalUpdates == nil || !out.SignalUpdates.HasEnoughFacts {
		t.Fatalf("grounded mechanism evidence should satisfy mechanism/comparison readiness, retry=%q", out.RetryHint)
	}
	if strings.Contains(out.RetryHint, "[DIRECT]/[REGISTRATION]") {
		t.Fatalf("mechanism/comparison readiness must not ask for direct/registration evidence, got %q", out.RetryHint)
	}
}

func TestParseOutput_ERMUnsatisfiedDoesNotDemoteOtherwiseReady(t *testing.T) {
	// All quantitative floors pass (2 tool types, 2 covered reads, 2 DIRECT
	// tags) but ERM has an unsatisfied requirement. ERM is advisory
	// breadth guidance, so it must not be the sole reason to reopen
	// exploration.
	eval := &explorerEvaluator{
		userQuestion: "question",
		structuredEvidence: []types.EvidenceItem{
			pinnedEvidenceItem("a.go", "Foo", 1),
			pinnedEvidenceItem("b.go", "Bar", 5),
		},
		investigationNotes: []string{
			"## Evidence from a.go\n- [DIRECT] Foo line 1: returns true\n- [DIRECT] Bar line 5: returns false",
		},
		ermRequirements: []EvidenceRequirement{
			{Kind: "registration", Entities: []string{"NeverMentionedSymbol"}, Status: "unsatisfied"},
		},
	}
	out, err := eval.ParseOutput(parseOutputCtx("", ""), nil, []types.ToolResult{
		{ToolName: "grep", Success: true, Summary: "a.go:1\nb.go:5"},
		{ToolName: "read_file", Success: true, Summary: "[a.go: showing lines 1-20 of 20 total]"},
		{ToolName: "read_file", Success: true, Summary: "[b.go: showing lines 1-20 of 20 total]"},
	}, nil)
	if err != nil {
		t.Fatalf("ParseOutput error: %v", err)
	}
	if out.SignalUpdates == nil || !out.SignalUpdates.HasEnoughFacts {
		t.Fatalf("otherwise-ready investigation must stay complete despite ERM-only gaps, retry=%q", out.RetryHint)
	}
	if strings.Contains(out.RetryHint, "evidence requirements unsatisfied") {
		t.Errorf("RetryHint must not turn ERM-only gaps into fact retry, got: %q", out.RetryHint)
	}
}

func TestParseOutput_ERMAllSatisfiedPromotes(t *testing.T) {
	// File coverage and evidence quality both fail the quantitative
	// floors (only 1 read, 1 DIRECT tag) but ERM is fully satisfied.
	// S1-alignment branch must promote hasEnough to true so the
	// orchestrator does not re-dispatch and hit the visit cap.
	// Trigger source: 2026-04-12 `有多少个agent可以调用subagent`
	// re-test.
	eval := &explorerEvaluator{
		userQuestion: "question",
		structuredEvidence: []types.EvidenceItem{
			pinnedEvidenceItem("a.go", "Foo", 1),
		},
		investigationNotes: []string{
			"## Evidence from a.go\n- [DIRECT] Foo line 1: returns true",
		},
		ermRequirements: []EvidenceRequirement{
			{Kind: "registration", Entities: []string{"Foo"}, Status: "satisfied"},
		},
	}
	out, err := eval.ParseOutput(parseOutputCtx("", ""), nil, []types.ToolResult{
		{ToolName: "grep", Success: true, Summary: "a.go:1"},
		{ToolName: "read_file", Success: true, Summary: "a.go (full file)"},
	}, nil)
	if err != nil {
		t.Fatalf("ParseOutput error: %v", err)
	}
	if out.SignalUpdates == nil || !out.SignalUpdates.HasEnoughFacts {
		t.Fatal("ERM all-satisfied must promote hasEnough=true even when quantitative floors fail")
	}
}

func TestParseOutput_ERMAllSatisfiedDoesNotPromoteWithoutEvidence(t *testing.T) {
	// A malformed emit_evidence call can leave ERM notes satisfied
	// while structured evidence is empty. That must not terminate the
	// explorer: Turn B would otherwise receive a 0-evidence handoff and
	// the finalizer may infer an absence answer from missing citations.
	eval := &explorerEvaluator{
		userQuestion:       "question",
		investigationNotes: []string{"Requirement Foo was investigated"},
		ermRequirements: []EvidenceRequirement{
			{Kind: "mechanism", Entities: []string{"Foo"}, Status: "satisfied"},
		},
	}
	out, err := eval.ParseOutput(parseOutputCtx("mechanism", "step_list"), nil, []types.ToolResult{
		{ToolName: "grep", Success: true, Summary: "a.go:1"},
		{ToolName: "read_file", Success: true, Summary: "a.go (full file)"},
	}, nil)
	if err != nil {
		t.Fatalf("ParseOutput error: %v", err)
	}
	if out.SignalUpdates == nil || out.SignalUpdates.HasEnoughFacts {
		t.Fatal("ERM all-satisfied must not promote hasEnough=true with zero structured evidence")
	}
	if !strings.Contains(out.RetryHint, "[DIRECT]") {
		t.Errorf("RetryHint should ask for structured evidence, got: %q", out.RetryHint)
	}
}

func TestParseOutput_MechanismKindDropsAnswerChains(t *testing.T) {
	// Mechanism questions feed the finalizer Evidence Items, not
	// answer chains. ParseOutput must drop AnswerChains/strict items
	// when irQuestionKind == "mechanism" so polymorphic-method
	// sibling drift cannot reach the finalizer.
	eval := &explorerEvaluator{
		userQuestion: "question",
		structuredEvidence: []types.EvidenceItem{
			{
				ID: "ev1", Kind: types.EvidenceRegistration, Subject: "Register", Predicate: "binds",
				Object: "Foo", Source: "internal/agent/explorer.go", LineStart: 100,
				LineEnd: 100, Producer: "test.fixture", Confidence: 0.9,
			},
			{
				ID: "ev2", Kind: types.EvidenceDirect, Subject: "Foo", Predicate: "returns",
				Object: "true", Source: "internal/agent/explorer.go", LineStart: 200,
				LineEnd: 200, Producer: "test.fixture", Confidence: 0.9,
			},
		},
		investigationNotes: []string{
			"## Evidence from internal/agent/explorer.go\n- [REGISTRATION] line 100: Register binds Foo\n- [DIRECT] Foo line 200: returns true",
		},
	}
	ctx := parseOutputCtx("mechanism", "step_list")
	ctx.Mutable = types.NewMutableState("question")
	out, err := eval.ParseOutput(ctx, nil, []types.ToolResult{
		{ToolName: "grep", Success: true, Summary: "internal/agent/explorer.go:100\ninternal/agent/explorer.go:200"},
		{ToolName: "read_file", Success: true, Summary: "internal/agent/explorer.go (full file)"},
	}, nil)
	if err != nil {
		t.Fatalf("ParseOutput error: %v", err)
	}
	if len(out.AnswerChains) != 0 {
		t.Errorf("mechanism-kind must drop AnswerChains, got %d", len(out.AnswerChains))
	}
	if len(out.AnswerSymbols) != 0 {
		t.Errorf("mechanism-kind must also drop AnswerSymbols (built from strictAnswerItems), got %d", len(out.AnswerSymbols))
	}
	ta := ctx.Mutable.TurnAArtifacts()
	if ta == nil {
		t.Fatal("mechanism-kind must still write TurnAArtifacts")
	}
	if len(ta.EvidenceItems) != 2 {
		t.Fatalf("mechanism-kind handoff must keep ranked evidence after dropping answer chains, got %d", len(ta.EvidenceItems))
	}
}

func TestParseOutput_MechanismHandoffCapsRankedEvidence(t *testing.T) {
	var items []types.EvidenceItem
	for i := 0; i < extractorMaxEvidence+5; i++ {
		items = append(items, pinnedEvidenceItem("a.go", fmt.Sprintf("Foo%d", i), i+1))
	}
	eval := &explorerEvaluator{
		userQuestion:       "how does Foo continue?",
		structuredEvidence: items,
		investigationNotes: []string{
			"## Evidence from a.go\n- [DIRECT] Foo line 1: returns true\n- [DIRECT] Foo line 2: returns false",
		},
	}
	ctx := parseOutputCtx("mechanism", "step_list")
	ctx.Mutable = types.NewMutableState("how does Foo continue?")
	_, err := eval.ParseOutput(ctx, nil, []types.ToolResult{
		{ToolName: "grep", Success: true, Summary: "a.go:1\na.go:2"},
		{ToolName: "read_file", Success: true, Summary: "[a.go: showing lines 1-100 of 100]"},
	}, nil)
	if err != nil {
		t.Fatalf("ParseOutput error: %v", err)
	}
	ta := ctx.Mutable.TurnAArtifacts()
	if ta == nil {
		t.Fatal("mechanism-kind must write TurnAArtifacts")
	}
	if len(ta.EvidenceItems) != extractorMaxEvidence {
		t.Fatalf("mechanism handoff evidence = %d, want cap %d", len(ta.EvidenceItems), extractorMaxEvidence)
	}
}

func TestParseOutput_StageOutputFieldsAllPopulated(t *testing.T) {
	// Positive-shape test: every field that downstream consumers
	// rely on is set (Data, StageReport, NewFacts, EvidenceItems,
	// SignalUpdates). Captures the contract Session 2 must preserve
	// when Turn A trims its responsibilities.
	eval := &explorerEvaluator{
		userQuestion: "question",
		structuredEvidence: []types.EvidenceItem{
			pinnedEvidenceItem("a.go", "Foo", 1),
			pinnedEvidenceItem("b.go", "Bar", 5),
		},
		investigationNotes: []string{
			"## Evidence from a.go\n- [DIRECT] Foo line 1: returns true",
			"## Evidence from b.go\n- [DIRECT] Bar line 5: returns false",
		},
	}
	toolResults := []types.ToolResult{
		{ToolName: "grep", Success: true, Summary: "a.go:1\nb.go:5"},
		{ToolName: "read_file", Success: true, Summary: "a.go (full file)\nb.go (full file)\nc.go (full file)"},
	}
	out, err := eval.ParseOutput(parseOutputCtx("", ""), nil, toolResults, nil)
	if err != nil {
		t.Fatalf("ParseOutput error: %v", err)
	}
	if string(out.Data) != "{}" {
		t.Errorf("P1.2 contract: Data must be `{}` (prose channel closed), got %q", string(out.Data))
	}
	if out.StageReport == "" {
		t.Error("StageReport must be populated by deterministic renderer")
	}
	if len(out.NewFacts) == 0 {
		t.Error("NewFacts must be populated from successful tool results")
	}
	if len(out.EvidenceItems) == 0 {
		t.Error("EvidenceItems must be populated from structuredEvidence")
	}
	if out.SignalUpdates == nil {
		t.Fatal("SignalUpdates must be set")
	}
}

func TestParseOutput_EnumerationStricterFloors(t *testing.T) {
	// isEnumerationQuery raises the bar: file coverage must be ≥80%
	// of discovered files and minDirect scales with discovered count.
	// Same fixture that passes a non-enumeration test (3 reads, 2
	// DIRECT tags) must FAIL when isEnumerationQuery is set and
	// discovered set is large.
	eval := &explorerEvaluator{
		userQuestion:       "list every X",
		isEnumerationQuery: true,
		structuredEvidence: []types.EvidenceItem{
			pinnedEvidenceItem("a.go", "Foo", 1),
			pinnedEvidenceItem("b.go", "Bar", 5),
		},
		investigationNotes: []string{
			"## Evidence from a.go\n- [DIRECT] Foo line 1: returns true\n- [DIRECT] Bar line 5: returns false",
		},
	}
	out, err := eval.ParseOutput(parseOutputCtx("", "list_of_symbols"), nil, []types.ToolResult{
		{ToolName: "grep", Success: true, Summary: "a.go:1\nb.go:1\nc.go:1\nd.go:1\ne.go:1\nf.go:1\ng.go:1\nh.go:1\ni.go:1\nj.go:1"},
		{ToolName: "read_file", Success: true, Summary: "a.go (full file)\nb.go (full file)"},
	}, nil)
	if err != nil {
		t.Fatalf("ParseOutput error: %v", err)
	}
	if out.SignalUpdates == nil || out.SignalUpdates.HasEnoughFacts {
		t.Fatal("enumeration query with 2/10 file coverage must not reach hasEnough=true")
	}
}

func TestExplorerReadiness_BoundedTraceSuppressesEnumerationCoverageFloors(t *testing.T) {
	ctx := parseOutputCtx(string(types.ReqEnumeration), "")
	ctx.AnalysisIR.RequestModel.Intent = types.IntentTrace
	ctx.AnalysisIR.RequestModel.PredicateAxis = types.AxisCall
	eval := &explorerEvaluator{
		isEnumerationQuery: true, // stale over-broad flag from analyzer must not impose enumeration coverage.
		analysisIR:         ctx.AnalysisIR,
		structuredEvidence: []types.EvidenceItem{{
			Kind:            types.EvidenceRelationship,
			Predicate:       "calls",
			Subject:         "buildAnalysisIR",
			Object:          "Run",
			Source:          "internal/agent/analyzer.go",
			LineStart:       100,
			GroundingStatus: types.GroundingGrounded,
			Summary:         "buildAnalysisIR calls the gate path",
		}},
	}

	if eval.strictEnumerationReadinessFloor() {
		t.Fatal("single-topic structural trace must not use strict enumeration readiness floors")
	}
	if eval.shouldRunEnumerationCoverageMidLoop() {
		t.Fatal("single-topic structural trace must not run enumeration mid-loop coverage")
	}
	readiness := eval.completionReadinessWithCoverage(nil, 2, false, false,
		[]string{"a.go", "b.go", "c.go", "d.go", "e.go", "f.go", "g.go", "h.go", "i.go", "j.go"},
		map[string]bool{"a.go": true, "b.go": true},
	)
	if !readiness.FileCoverage {
		t.Fatalf("bounded trace with grounded chain evidence should not require broad file coverage, got %+v", readiness)
	}
	if !readiness.HasEnough {
		t.Fatalf("bounded trace should be ready once tool/evidence quality is satisfied, got %+v", readiness)
	}
}

func TestExplorerReadiness_MultiSubtopicCallChainTraceSuppressesEnumerationCoverage(t *testing.T) {
	ctx := parseOutputCtx(string(types.ReqCallChain), "")
	rm := &ctx.AnalysisIR.RequestModel
	rm.Intent = types.IntentTrace
	rm.PredicateAxis = types.AxisCall
	rm.Predicates.IsCrossComponent = true
	rm.DiagramHint = &types.DiagramHint{Kind: types.DiagramSequence}
	rm.AnalyzerHints.Entities = []string{"buildAnalysisIR", "gate.Run", "analyzer.go"}
	rm.SubTopics = []types.SubTopic{
		{Summary: "调用链从 buildAnalysisIR 到 gate.Run 的完整顺序", Entities: []string{"buildAnalysisIR", "gate.Run"}},
		{Summary: "关键中间函数的列表", Entities: []string{"buildAnalysisIR", "gate.Run"}},
	}
	eval := &explorerEvaluator{
		isEnumerationQuery: true, // stale fork-local flag must not override the typed trace contract.
		analysisIR:         ctx.AnalysisIR,
	}

	if enumerationIntentForContext(ctx) {
		t.Fatal("typed call-chain trace must not be treated as an exhaustive enumeration request")
	}
	if !eval.boundedStructuralTraceRequest() {
		t.Fatal("multi-subtopic call-chain trace should stay in the bounded trace lane")
	}
	if eval.shouldRunEnumerationCoverageMidLoop() {
		t.Fatal("bounded trace must not run broad discovered-file enumeration coverage")
	}
}

func TestExplorerReadiness_SequenceTraceSuppressesEnumerationAndOverviewHints(t *testing.T) {
	eval := &explorerEvaluator{
		isEnumerationQuery: true, // stale fork-local flag must not override the typed path contract.
		analysisIR: &types.AnalysisIR{RequestModel: types.RequestModel{
			Intent:     types.IntentTrace,
			Scenario:   types.ScenarioArchitectureExplain,
			Complexity: types.ComplexityModerate,
			Predicates: types.SemanticPredicates{
				IsCategoryEnumeration: false,
				IsRelationalLookup:    false,
				IsCountQuestion:       false,
			},
			AnalyzerHints: types.AnalyzerHints{
				Kind:     string(types.ReqCallChain),
				Entities: []string{"analyzer.go", "buildAnalysisIR", "gate.Run"},
			},
			DiagramHint: &types.DiagramHint{Kind: types.DiagramSequence},
		}},
	}

	if !typedTracePathRequestModel(eval.analysisIR.RequestModel) {
		t.Fatal("sequence call-chain trace should be treated as a bounded path contract")
	}
	if !eval.boundedStructuralTraceRequest() {
		t.Fatal("sequence call-chain trace should stay in the bounded trace lane")
	}
	if eval.strictEnumerationReadinessFloor() {
		t.Fatal("sequence call-chain trace must not use strict enumeration readiness floors")
	}
	if eval.shouldRunEnumerationCoverageMidLoop() {
		t.Fatal("sequence call-chain trace must not run broad discovered-file enumeration coverage")
	}
	if eval.typedStructuralOverviewRequiresWideRead() {
		t.Fatal("sequence call-chain trace must not ask for whole-file architecture overview coverage")
	}
}

func TestExplorerReadiness_SingleMechanismExplainSuppressesOverviewHints(t *testing.T) {
	eval := &explorerEvaluator{
		analysisIR: &types.AnalysisIR{RequestModel: types.RequestModel{
			Intent:     types.IntentExplain,
			Scenario:   types.ScenarioArchitectureExplain,
			Complexity: types.ComplexityModerate,
			Predicates: types.SemanticPredicates{
				IsCrossComponent: true,
			},
			AnalyzerHints: types.AnalyzerHints{Kind: string(types.ReqMechanism)},
		}},
	}

	if !types.IsSingleTopicMechanismExplanation(eval.analysisIR.RequestModel) {
		t.Fatal("fixture should be a typed single-topic mechanism explanation")
	}
	if eval.typedStructuralOverviewRequiresWideRead() {
		t.Fatal("single-topic mechanism explanation must not be upgraded into whole-file overview coverage")
	}
}

func TestExplorerReadiness_ArchitectureNarrativeStillRequiresWideRead(t *testing.T) {
	eval := &explorerEvaluator{
		analysisIR: &types.AnalysisIR{RequestModel: types.RequestModel{
			Intent:     types.IntentExplain,
			Scenario:   types.ScenarioArchitectureExplain,
			Complexity: types.ComplexityComplex,
			Predicates: types.SemanticPredicates{
				IsCrossComponent: true,
			},
			SubTopics: []types.SubTopic{
				{Summary: "入口层", Entities: []string{"router"}},
				{Summary: "执行层", Entities: []string{"runtime"}},
			},
		}},
	}

	if !eval.typedStructuralOverviewRequiresWideRead() {
		t.Fatal("true architecture narrative should still request wider structural coverage")
	}
}

func TestExplorerReadiness_TypedCarrierCountSatisfiesBoundedTrace(t *testing.T) {
	ctx := parseOutputCtx(string(types.ReqCallChain), "")
	ctx.AnalysisIR.RequestModel.Intent = types.IntentTrace
	ctx.AnalysisIR.RequestModel.PredicateAxis = types.AxisCall
	eval := &explorerEvaluator{
		analysisIR: ctx.AnalysisIR,
		structuredEvidence: []types.EvidenceItem{{
			Kind:            types.EvidenceRelationship,
			AnchorKind:      types.AnchorCall,
			Predicate:       "calls",
			Subject:         "caller",
			Object:          "callee",
			Source:          "a.go",
			LineStart:       12,
			GroundingStatus: types.GroundingGrounded,
		}},
	}

	readiness := eval.completionReadinessWithCoverage(nil, 2, false, false,
		[]string{"a.go", "b.go", "c.go", "d.go"},
		map[string]bool{"a.go": true},
	)
	if readiness.RequirementCarrierCount != 1 || readiness.MinRequirementCarrier != 1 {
		t.Fatalf("typed carrier readiness = %d/%d, want 1/1: %+v",
			readiness.RequirementCarrierCount, readiness.MinRequirementCarrier, readiness)
	}
	if !readiness.EvidenceQuality || !readiness.HasEnough {
		t.Fatalf("bounded trace should be ready with one grounded typed carrier and two tool sources, got %+v", readiness)
	}
}

// -----------------------------------------------------------------------------
// DetermineMissingPiece
// -----------------------------------------------------------------------------

func TestDetermineMissingPiece_HasEnoughReturnsMissingNone(t *testing.T) {
	eval := &explorerEvaluator{}
	out := &StageOutput{SignalUpdates: &types.ExecutionSignals{HasEnoughFacts: true}}
	if got := eval.DetermineMissingPiece(nil, out); got != types.MissingNone {
		t.Errorf("HasEnoughFacts=true should yield MissingNone, got %v", got)
	}
}

func TestDetermineMissingPiece_NotEnoughReturnsMissingFacts(t *testing.T) {
	eval := &explorerEvaluator{}
	cases := []*StageOutput{
		{SignalUpdates: nil},
		{SignalUpdates: &types.ExecutionSignals{HasEnoughFacts: false}},
	}
	for i, out := range cases {
		if got := eval.DetermineMissingPiece(nil, out); got != types.MissingFacts {
			t.Errorf("case %d: expected MissingFacts, got %v", i, got)
		}
	}
}

// -----------------------------------------------------------------------------
// Turn A — Completeness claim + TurnAArtifacts handoff
// -----------------------------------------------------------------------------
//
// Contract: ParseOutput computes TerminalEvidenceCount over
// strictAnswerItems and writes a full TurnAArtifacts snapshot via
// Mutable.SetTurnAArtifacts. StageOutput.AnswerSymbols is nil and
// AnswerSymbolCompleteness stays zero — the extractor (Turn B) is
// the sole producer of both.

// phase11Eval returns an explorerEvaluator primed with enough state
// that ParseOutput reaches the answer-symbol branch: two files read,
// tool diversity (grep + read_file), 2 DIRECT-tagged notes. The
// inner structuredEvidence carries two registration-shape items so
// strictAnswerItems is non-empty and terminalEvidenceCount > 0.
func phase11Eval(question string) *explorerEvaluator {
	regItem := types.EvidenceItem{
		Kind:       types.EvidenceRegistration,
		Subject:    "Register",
		Predicate:  "binds",
		Object:     "NewFoo",
		Source:     "reg.go",
		LineStart:  10,
		LineEnd:    10,
		Producer:   "test.fixture",
		Confidence: 0.9,
		Scope:      types.ScopeLine,
	}
	regItem.ID = types.StableEvidenceID(regItem)
	directItem := pinnedEvidenceItem("a.go", "Foo", 1)
	return &explorerEvaluator{
		userQuestion:       question,
		structuredEvidence: []types.EvidenceItem{regItem, directItem},
		investigationNotes: []string{
			"## Evidence from reg.go\n- [REGISTRATION] line 10: Register binds NewFoo",
			"## Evidence from a.go\n- [DIRECT] Foo line 1: returns true",
		},
	}
}

func phase11ToolResults() []types.ToolResult {
	// read_file summaries must start with "[path: ...]" for
	// extractFileCoverage to pick up the read path. Each call covers
	// exactly one file; that is the real shape the tool produces.
	return []types.ToolResult{
		{ToolName: "grep", Success: true, Summary: "reg.go:10\na.go:1\nb.go:1"},
		{ToolName: "read_file", Success: true, Summary: "[reg.go: showing lines 1-20 of 20]"},
		{ToolName: "read_file", Success: true, Summary: "[a.go: showing lines 1-30 of 30]"},
		{ToolName: "read_file", Success: true, Summary: "[b.go: showing lines 1-40 of 40]"},
	}
}

func TestParseOutput_WritesTurnAArtifactsAndLeavesSlateEmpty(t *testing.T) {
	eval := phase11Eval("which handlers register Foo?")
	ctx := parseOutputCtx("registration", "list_of_symbols")
	ctx.Mutable = types.NewMutableState("")

	out, err := eval.ParseOutput(ctx, nil, phase11ToolResults(), nil)
	if err != nil {
		t.Fatalf("ParseOutput error: %v", err)
	}
	// StageOutput.AnswerSymbols MUST be nil and completeness MUST
	// stay zero. The extractor (Turn B) will populate both.
	if len(out.AnswerSymbols) != 0 {
		t.Errorf("Turn A must leave AnswerSymbols empty, got %d: %+v", len(out.AnswerSymbols), out.AnswerSymbols)
	}
	if out.AnswerSymbolCompleteness != types.CompletenessUnknown {
		t.Errorf("Turn A completeness = %q, want zero", out.AnswerSymbolCompleteness)
	}
	// TurnAArtifacts MUST be written with the user question, read
	// files, tool results, evidence items and a non-negative
	// TerminalEvidenceCount.
	ta := ctx.Mutable.TurnAArtifacts()
	if ta == nil {
		t.Fatal("flag=on must write TurnAArtifacts")
	}
	if ta.UserQuestion != "which handlers register Foo?" {
		t.Errorf("TurnAArtifacts.UserQuestion = %q", ta.UserQuestion)
	}
	if len(ta.InvestigationNotes) != 2 {
		t.Errorf("TurnAArtifacts.InvestigationNotes = %d, want 2", len(ta.InvestigationNotes))
	}
	if len(ta.ReadFiles) == 0 {
		t.Error("TurnAArtifacts.ReadFiles must be populated from coverage set")
	}
	if len(ta.ToolResults) != 4 {
		t.Errorf("TurnAArtifacts.ToolResults = %d, want 4", len(ta.ToolResults))
	}
	if ta.TerminalEvidenceCount < 0 {
		t.Errorf("TurnAArtifacts.TerminalEvidenceCount negative: %d", ta.TerminalEvidenceCount)
	}
	if len(ta.EvidenceItems) <= ta.TerminalEvidenceCount {
		t.Errorf("support evidence should not inflate terminal baseline: evidence=%d terminal=%d", len(ta.EvidenceItems), ta.TerminalEvidenceCount)
	}
	// Defensive-copy invariant: mutating the caller's
	// investigationNotes after handoff must not leak into the
	// snapshot. Session 1 Phase 6 locked this invariant; Phase 11
	// re-verifies through the real producer path.
	eval.investigationNotes = append(eval.investigationNotes, "post-handoff note")
	ta2 := ctx.Mutable.TurnAArtifacts()
	if len(ta2.InvestigationNotes) != 2 {
		t.Errorf("post-handoff append leaked into snapshot: len = %d", len(ta2.InvestigationNotes))
	}
}

func TestParseOutput_ExhaustiveEnumerationRequiresMemberSetHandoffBeforeTurnB(t *testing.T) {
	eval := phase11Eval("List all public enum types")
	eval.isEnumerationQuery = true
	ctx := parseOutputCtx(string(types.ReqEnumeration), "list_of_symbols")
	ctx.Mutable = types.NewMutableState("List all public enum types")
	ctx.AnalysisIR.RequestModel.Intent = types.IntentEnumerate
	ctx.AnalysisIR.RequestModel.Predicates = types.SemanticPredicates{IsCategoryEnumeration: true}
	ctx.AnalysisIR.RequestModel.CompletenessObligation = &types.CompletenessObligation{
		Required:    true,
		SourceQuote: "all public enum types",
	}
	ctx.AnalysisIR.RequestModel.AnalyzerHints = types.AnalyzerHints{
		Kind:     string(types.ReqEnumeration),
		Entities: []string{"Intent", "QuestionFamily", "Scenario"},
	}
	eval.analysisIR = ctx.AnalysisIR

	out, err := eval.ParseOutput(ctx, nil, phase11ToolResults(), nil)
	if err != nil {
		t.Fatalf("ParseOutput error: %v", err)
	}
	if out.SignalUpdates == nil {
		t.Fatal("SignalUpdates must be populated")
	}
	if out.SignalUpdates.HasEnoughFacts {
		t.Fatal("exhaustive enumeration must not advance to extractor without accepted aggregate_facts.member_set")
	}
	if !strings.Contains(out.RetryHint, "aggregate_facts.member_set") {
		t.Fatalf("retry hint should direct a structured member_set handoff, got %q", out.RetryHint)
	}

	ctx.Mutable.SetInvestigationAggregateFacts([]types.AnswerAggregateFact{{
		Kind:    types.AnswerAggregateMemberSet,
		Label:   "public enum types",
		Value:   "3",
		Members: []string{"Intent", "QuestionFamily", "Scenario"},
	}})
	ctx.Mutable.SetInvestigationComplete("member set accepted")
	out, err = eval.ParseOutput(ctx, nil, phase11ToolResults(), nil)
	if err != nil {
		t.Fatalf("ParseOutput with member_set error: %v", err)
	}
	if out.SignalUpdates == nil || !out.SignalUpdates.HasEnoughFacts {
		t.Fatalf("accepted member_set should satisfy structured handoff, got signals=%+v hint=%q", out.SignalUpdates, out.RetryHint)
	}
}

func TestParseOutput_SourceOperationSiteRequestRequiresMemberSetHandoff(t *testing.T) {
	question := "当前代码仓进程切换cgroup分组，其pid和tid是在哪儿写入cgroup分组下面的文件里的，都有哪些写入点？"
	eval := phase11Eval(question)
	ctx := parseOutputCtx(string(types.ReqMechanism), "")
	ctx.Objective = question
	ctx.Mutable = types.NewMutableState(question)
	ctx.AnalysisIR.RequestModel.RawRequest = question
	ctx.AnalysisIR.RequestModel.Intent = types.IntentRootCause
	ctx.AnalysisIR.RequestModel.Predicates.HasPerMemberTable = true
	ctx.AnalysisIR.RequestModel.AnalyzerHints = types.AnalyzerHints{Kind: string(types.ReqMechanism)}
	ctx.AnalysisIR.RequestModel.SourceInventoryProfile = &types.SourceInventoryProfile{
		IsSourceInventory: true,
		TargetRoles:       []types.AnswerCandidateRole{types.AnswerCandidateRoleFunction, types.AnswerCandidateRoleMethod},
	}
	eval.analysisIR = ctx.AnalysisIR

	out, err := eval.ParseOutput(ctx, nil, phase11ToolResults(), nil)
	if err != nil {
		t.Fatalf("ParseOutput error: %v", err)
	}
	if out.SignalUpdates == nil {
		t.Fatal("SignalUpdates must be populated")
	}
	if out.SignalUpdates.HasEnoughFacts {
		t.Fatal("source operation-site request must not advance without accepted aggregate_facts.member_set")
	}
	if !strings.Contains(out.RetryHint, "aggregate_facts.member_set") {
		t.Fatalf("retry hint should direct source operation-site member_set handoff, got %q", out.RetryHint)
	}

	ctx.Mutable.SetInvestigationAggregateFacts([]types.AnswerAggregateFact{{
		Kind:        types.AnswerAggregateMemberSet,
		Label:       "cgroup 写入点",
		Value:       "2",
		Members:     []string{"SetPidToCgroup @ awarecpu/aware_cpuctl.c:131", "write call @ awarecpu/aware_cpuctl.c:154"},
		SupportRefs: []string{"awarecpu/aware_cpuctl.c:131", "awarecpu/aware_cpuctl.c:154"},
	}})
	ctx.Mutable.SetInvestigationComplete("source operation sites accepted")
	out, err = eval.ParseOutput(ctx, nil, phase11ToolResults(), nil)
	if err != nil {
		t.Fatalf("ParseOutput with member_set error: %v", err)
	}
	if out.SignalUpdates == nil || !out.SignalUpdates.HasEnoughFacts {
		t.Fatalf("accepted source operation-site member_set should satisfy handoff, got signals=%+v hint=%q", out.SignalUpdates, out.RetryHint)
	}
}

func TestParseOutput_PartialSourceInventoryMemberSetBlocksCompletePromotion(t *testing.T) {
	eval := phase11Eval("List all source scopes")
	eval.isEnumerationQuery = true
	ctx := parseOutputCtx(string(types.ReqEnumeration), "list_of_symbols")
	ctx.Mutable = types.NewMutableState("List all source scopes")
	ctx.Mutable.SetSourceInventoryObservation(types.SourceInventoryObservation{
		Active:       true,
		AdvisoryOnly: true,
		Complete:     true,
		Scopes:       []string{"src"},
		Provenance:   []string{"tool:list_files:direct"},
		Lens:         []string{"direct_children", "count"},
		Sets: []types.SourceInventoryObservationSet{{
			Role:     types.AnswerCandidateRolePackage,
			Complete: true,
			Count:    3,
			Members: []types.SourceInventoryObservationMember{{
				Name:       "alpha",
				Key:        "src/alpha",
				File:       "src/alpha",
				Role:       types.AnswerCandidateRolePackage,
				Provenance: []string{"tool:list_files:direct"},
			}, {
				Name:       "beta",
				Key:        "src/beta",
				File:       "src/beta",
				Role:       types.AnswerCandidateRolePackage,
				Provenance: []string{"tool:list_files:direct"},
			}, {
				Name:       "gamma",
				Key:        "src/gamma",
				File:       "src/gamma",
				Role:       types.AnswerCandidateRolePackage,
				Provenance: []string{"tool:list_files:direct"},
			}},
		}},
	})
	ctx.Mutable.SetInvestigationAggregateFacts([]types.AnswerAggregateFact{{
		Kind:    types.AnswerAggregateMemberSet,
		Label:   "source scopes",
		Value:   "2",
		Members: []string{"alpha", "beta"},
	}})
	ctx.Mutable.SetInvestigationComplete("partial source inventory member set accepted")
	ctx.AnalysisIR.RequestModel.Intent = types.IntentEnumerate
	ctx.AnalysisIR.RequestModel.Predicates = types.SemanticPredicates{IsCategoryEnumeration: true}
	ctx.AnalysisIR.RequestModel.CompletenessObligation = &types.CompletenessObligation{
		Required:    true,
		SourceQuote: "all source scopes",
	}
	ctx.AnalysisIR.RequestModel.AnalyzerHints = types.AnalyzerHints{
		Kind: string(types.ReqEnumeration),
	}
	eval.analysisIR = ctx.AnalysisIR

	out, err := eval.ParseOutput(ctx, nil, phase11ToolResults(), nil)
	if err != nil {
		t.Fatalf("ParseOutput error: %v", err)
	}
	if out.SignalUpdates == nil {
		t.Fatal("SignalUpdates must be populated")
	}
	if out.SignalUpdates.HasEnoughFacts {
		t.Fatal("partial member_set must not be promoted complete while an exact candidate universe gap is blocking")
	}
	if !strings.Contains(out.RetryHint, "exact source-inventory candidate universe") ||
		!strings.Contains(out.RetryHint, "gamma") {
		t.Fatalf("retry hint should prioritize the scoped candidate universe gap, got %q", out.RetryHint)
	}
	if strings.Contains(out.RetryHint, "fewer than 2 distinct evidence tool types") ||
		strings.Contains(out.RetryHint, "Read more of the discovered files") {
		t.Fatalf("candidate universe gap should beat generic retry hints, got %q", out.RetryHint)
	}
}

func TestParseOutput_RelationEnumerationRequiresMemberSetHandoffBeforeTurnB(t *testing.T) {
	eval := phase11Eval("Which agents can call sub-agents?")
	ctx := parseOutputCtx(string(types.ReqRegistration), "list_of_symbols")
	ctx.Mutable = types.NewMutableState("Which agents can call sub-agents?")
	ctx.AnalysisIR.RequestModel.Intent = types.IntentEnumerate
	ctx.AnalysisIR.RequestModel.Predicates = types.SemanticPredicates{
		IsRelationalLookup:    true,
		IsCategoryEnumeration: true,
	}
	ctx.AnalysisIR.RequestModel.AnalyzerHints = types.AnalyzerHints{
		Kind:     string(types.ReqRegistration),
		Entities: []string{"SubExplorer", "propose_sub_agents"},
	}
	eval.analysisIR = ctx.AnalysisIR

	out, err := eval.ParseOutput(ctx, nil, phase11ToolResults(), nil)
	if err != nil {
		t.Fatalf("ParseOutput error: %v", err)
	}
	if out.SignalUpdates == nil {
		t.Fatal("SignalUpdates must be populated")
	}
	if out.SignalUpdates.HasEnoughFacts {
		t.Fatal("relation enumeration must not advance to extractor without accepted aggregate_facts.member_set")
	}
	if !strings.Contains(out.RetryHint, "aggregate_facts.member_set") {
		t.Fatalf("retry hint should direct a structured relation member_set handoff, got %q", out.RetryHint)
	}

	ctx.Mutable.SetInvestigationAggregateFacts([]types.AnswerAggregateFact{{
		Kind:    types.AnswerAggregateMemberSet,
		Label:   "agents that can call sub-agents",
		Value:   "1",
		Members: []string{"SubExplorer"},
	}})
	ctx.Mutable.SetInvestigationComplete("relation member set accepted")
	out, err = eval.ParseOutput(ctx, nil, phase11ToolResults(), nil)
	if err != nil {
		t.Fatalf("ParseOutput with member_set error: %v", err)
	}
	if out.SignalUpdates == nil || !out.SignalUpdates.HasEnoughFacts {
		t.Fatalf("accepted relation member_set should satisfy structured handoff, got signals=%+v hint=%q", out.SignalUpdates, out.RetryHint)
	}
}

func TestParseOutput_NoMutableStateGracefullyDegrades(t *testing.T) {
	// Defensive: ParseOutput must not panic when ctx.Mutable is nil.
	// The TurnA handoff snapshot is simply skipped and the rest of
	// the StageOutput fields still get populated (minus the
	// answer-symbol slate, which is Turn B's responsibility).
	eval := phase11Eval("q")
	ctx := parseOutputCtx("registration", "list_of_symbols")
	ctx.Mutable = nil

	out, err := eval.ParseOutput(ctx, nil, phase11ToolResults(), nil)
	if err != nil {
		t.Fatalf("ParseOutput error with nil Mutable: %v", err)
	}
	if len(out.AnswerSymbols) != 0 {
		t.Errorf("flag=on must leave AnswerSymbols empty, got %d", len(out.AnswerSymbols))
	}
}
