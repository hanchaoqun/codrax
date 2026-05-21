package agent

import (
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/llm"
	"github.com/hanchaoqun/codrax/internal/tool/repomap"
	"github.com/hanchaoqun/codrax/internal/types"
)

// Session 22 — mid-loop coverage pushbacks. Two new checks plug gaps
// the existing detectors did not cover:
//
//   - Check 5 (intent-window mismatch): the LLM declares a structural
//     intent in its assistant content ("整体结构", "overall layout")
//     while its accompanying read_file covers a tiny slice of a large
//     file. Symbol-range partial-read detection cannot see this
//     because a narrow targeted read may not enter any function's
//     entry zone — the semantic "my intent needs more than my window"
//     mismatch is invisible at the symbol level.
//
//   - Check 6 (ranker-coverage): late in the explorer loop the LLM
//     has opened only a small fraction of the ranker's top-K files.
//     The existing enumeration coverage gate keys on
//     isEnumerationQuery; mechanism / explain / root_cause questions
//     slip through when the LLM cherry-picks 2-3 files and exits.

// ── isStructuralIntent — surface tokens ─────────────────────────

func TestIsStructuralIntent_ChineseTokens(t *testing.T) {
	cases := []string{
		"最后确认 agent.go 中 ReAct 循环的大致结构",
		"先看下整体结构，再看具体实现",
		"想了解这个模块的整体流程",
		"先梳理一下大致流程",
		"看下系统的整体架构",
		"想要一个全貌",
		"先看概览",
	}
	for _, c := range cases {
		t.Run(c, func(t *testing.T) {
			if !isStructuralIntent(c) {
				t.Errorf("expected structural intent for %q", c)
			}
		})
	}
}

func TestIsStructuralIntent_EnglishTokens(t *testing.T) {
	cases := []string{
		"Let me confirm the overall structure of agent.go",
		"Getting the big picture of how dispatch works",
		"High-level overview of the pipeline",
		"I want to see the whole structure",
		"Reading the whole file to understand layout",
	}
	for _, c := range cases {
		t.Run(c, func(t *testing.T) {
			if !isStructuralIntent(c) {
				t.Errorf("expected structural intent for %q", c)
			}
		})
	}
}

func TestIsStructuralIntent_Negative(t *testing.T) {
	cases := []string{
		"",
		"Let me read the function body",
		"Reading the exact line where the error is raised",
		"需要查看这个函数的返回值",
		"Reading lines 527-586",
	}
	for _, c := range cases {
		t.Run(c, func(t *testing.T) {
			if isStructuralIntent(c) {
				t.Errorf("unexpected structural intent for %q", c)
			}
		})
	}
}

// ── Check 5 — intent-window mismatch ────────────────────────────

// TestMidLoopCheck_IntentWindow_FiresOnStructuralIntentWithNarrowWindow
// is the direct regression for the customer trace: the LLM said "最后
// 确认 … 大致结构" then read 60 lines of a 1549-line file. The new
// check must surface a structural-intent hint.
func TestMidLoopCheck_IntentWindow_FiresOnStructuralIntentWithNarrowWindow(t *testing.T) {
	eval := &explorerEvaluator{
		phase:        1,
		userQuestion: "agent.go 里 ReAct 循环怎么写的",
		searchResult: &keywordSearchResult{Graph: &repomap.Graph{}},
		heuristics:   types.DefaultExploreHeuristics(),
	}
	history := []types.ToolResult{
		{ToolName: "grep", Success: true, Summary: "[grep: 2 files]\ninternal/agent/agent.go\n"},
		{
			ToolName: "read_file",
			Success:  true,
			Summary:  "[internal/agent/agent.go: showing lines 527-586 of 1549 total]\n...body...",
		},
	}
	sig := eval.observeMidLoop(LoopObservation{
		Phase:     PhaseMidLoop,
		Iteration: 19,
		Response: llm.Response{
			Content: "最后确认 agent.go 中 ReAct 循环的大致结构",
		},
		LastToolResult: &history[1],
		AllToolResults: history,
	})
	if !sig.HintRequested {
		t.Fatalf("structural-intent + 60/1549 window should fire; got %+v", sig)
	}
	if sig.HintKey != "explorer.mid-loop.intent-window-mismatch" {
		t.Fatalf("HintKey = %q, want explorer.mid-loop.intent-window-mismatch", sig.HintKey)
	}
	if !strings.Contains(sig.Hint, "list_files") {
		t.Errorf("hint should guide the LLM toward list_files, got: %s", sig.Hint)
	}
	if !strings.Contains(sig.Hint, "limit>=300") {
		t.Errorf("hint should suggest a wider window, got: %s", sig.Hint)
	}
}

// No structural intent in the content → the check must NOT fire.
// Otherwise every targeted read on a large file would trip the
// detector and drown the LLM in irrelevant pushbacks.
func TestMidLoopCheck_IntentWindow_NoFireWithoutStructuralIntent(t *testing.T) {
	eval := &explorerEvaluator{
		phase:        1,
		userQuestion: "where is nil check",
		searchResult: &keywordSearchResult{Graph: &repomap.Graph{}},
		heuristics:   types.DefaultExploreHeuristics(),
	}
	history := []types.ToolResult{
		{
			ToolName: "read_file",
			Success:  true,
			Summary:  "[internal/agent/agent.go: showing lines 527-586 of 1549 total]\n...body...",
		},
	}
	sig := eval.observeMidLoop(LoopObservation{
		Phase:     PhaseMidLoop,
		Iteration: 19,
		Response: llm.Response{
			Content: "Let me check the nil guard at the top of Run",
		},
		LastToolResult: &history[0],
		AllToolResults: history,
	})
	if sig.HintKey == "explorer.mid-loop.intent-window-mismatch" {
		t.Fatalf("targeted read with specific intent must not trip structural-intent gate, got %+v", sig)
	}
}

// A wide read on a small file must NOT fire even with a structural
// intent — the LLM has already covered enough of the file.
func TestMidLoopCheck_IntentWindow_NoFireOnWideWindow(t *testing.T) {
	eval := &explorerEvaluator{
		phase:        1,
		userQuestion: "agent.go 里 ReAct 循环怎么写的",
		searchResult: &keywordSearchResult{Graph: &repomap.Graph{}},
		heuristics:   types.DefaultExploreHeuristics(),
	}
	history := []types.ToolResult{
		{
			ToolName: "read_file",
			Success:  true,
			Summary:  "[internal/agent/agent.go: showing lines 1-400 of 1549 total]\n...body...",
		},
	}
	sig := eval.observeMidLoop(LoopObservation{
		Phase:     PhaseMidLoop,
		Iteration: 19,
		Response: llm.Response{
			Content: "最后确认 agent.go 中 ReAct 循环的大致结构",
		},
		LastToolResult: &history[0],
		AllToolResults: history,
	})
	if sig.HintKey == "explorer.mid-loop.intent-window-mismatch" {
		t.Fatalf("400-line window over 1549-line file is wide enough (>15%%); got %+v", sig)
	}
}

// Small files (< 300 lines) are excluded from the structural-intent
// gate — a 60-line read of a 150-line file is already 40% coverage,
// which is structurally reasonable.
func TestMidLoopCheck_IntentWindow_NoFireOnSmallFile(t *testing.T) {
	eval := &explorerEvaluator{
		phase:        1,
		userQuestion: "agent.go 里 ReAct 循环怎么写的",
		searchResult: &keywordSearchResult{Graph: &repomap.Graph{}},
		heuristics:   types.DefaultExploreHeuristics(),
	}
	history := []types.ToolResult{
		{
			ToolName: "read_file",
			Success:  true,
			Summary:  "[internal/agent/agent.go: showing lines 1-60 of 150 total]\n...body...",
		},
	}
	sig := eval.observeMidLoop(LoopObservation{
		Phase:     PhaseMidLoop,
		Iteration: 19,
		Response: llm.Response{
			Content: "最后确认 agent.go 的大致结构",
		},
		LastToolResult: &history[0],
		AllToolResults: history,
	})
	if sig.HintKey == "explorer.mid-loop.intent-window-mismatch" {
		t.Fatalf("small file (150 lines) should not trip the structural gate; got %+v", sig)
	}
}

// One-shot: once the hint fires, subsequent iterations with the same
// narrow-intent pattern must not re-fire (prevent noise when the LLM
// keeps reading targeted windows after seeing the hint).
func TestMidLoopCheck_IntentWindow_OneShot(t *testing.T) {
	eval := &explorerEvaluator{
		phase:        1,
		userQuestion: "agent.go 里 ReAct 循环怎么写的",
		searchResult: &keywordSearchResult{Graph: &repomap.Graph{}},
		heuristics:   types.DefaultExploreHeuristics(),
	}
	history := []types.ToolResult{{
		ToolName: "read_file",
		Success:  true,
		Summary:  "[internal/agent/agent.go: showing lines 527-586 of 1549 total]\n...",
	}}
	first := eval.observeMidLoop(LoopObservation{
		Phase:          PhaseMidLoop,
		Iteration:      19,
		Response:       llm.Response{Content: "最后确认 ReAct 循环的大致结构"},
		LastToolResult: &history[0],
		AllToolResults: history,
	})
	if !first.HintRequested {
		t.Fatalf("first fire expected: %+v", first)
	}
	second := eval.observeMidLoop(LoopObservation{
		Phase:          PhaseMidLoop,
		Iteration:      20,
		Response:       llm.Response{Content: "继续确认整体结构"},
		LastToolResult: &history[0],
		AllToolResults: history,
	})
	if second.HintKey == "explorer.mid-loop.intent-window-mismatch" {
		t.Fatalf("structural-intent hint should be one-shot; got re-fire: %+v", second)
	}
}

// ── Check 6 — ranker coverage ───────────────────────────────────

// TestMidLoopCheck_RankerCoverage_FiresWhenTopKMissing is the direct
// regression for the customer trace: allScoredFiles=36 while readSet
// contained only 4 files, with the top-5 ranked files largely absent.
// The new check must surface an "open the top unread" hint.
func TestMidLoopCheck_RankerCoverage_FiresWhenTopKMissing(t *testing.T) {
	eval := &explorerEvaluator{
		phase:        1,
		userQuestion: "explorer 是怎么调用 subagent 的",
		searchResult: &keywordSearchResult{Graph: &repomap.Graph{}},
		heuristics:   types.DefaultExploreHeuristics(),
		// Ranker returned these 6 files in score order; only the
		// last one was actually opened, so top-5 has 5 missing.
		allScoredFiles: []string{
			"internal/agent/sub_explorer.go",
			"internal/agent/subagent_runtime.go",
			"internal/agent/explorer.go",
			"internal/agent/agent.go",
			"internal/tool/dispatch.go",
			"internal/agent/keyword_search.go",
		},
	}
	history := []types.ToolResult{
		{
			ToolName: "read_file",
			Success:  true,
			Summary:  "[internal/agent/keyword_search.go: showing lines 1-80 of 200 total]\n...",
		},
	}
	sig := eval.observeMidLoop(LoopObservation{
		Phase:          PhaseMidLoop,
		Iteration:      10,
		Response:       llm.Response{Content: "I have read the keyword search file."},
		LastToolResult: &history[0],
		AllToolResults: history,
	})
	if !sig.HintRequested {
		t.Fatalf("top-5 unread with 5 misses should fire ranker-coverage, got %+v", sig)
	}
	if sig.HintKey != "explorer.mid-loop.ranker-coverage" {
		t.Fatalf("HintKey = %q, want explorer.mid-loop.ranker-coverage", sig.HintKey)
	}
	if !strings.Contains(sig.Hint, "sub_explorer.go") {
		t.Errorf("hint should name the top missing files, got: %s", sig.Hint)
	}
}

// Covering the top-K must silence the pushback — the LLM already
// opened the most relevant work.
func TestMidLoopCheck_RankerCoverage_NoFireWhenTopKCovered(t *testing.T) {
	eval := &explorerEvaluator{
		phase:        1,
		userQuestion: "explorer 是怎么调用 subagent 的",
		searchResult: &keywordSearchResult{Graph: &repomap.Graph{}},
		heuristics:   types.DefaultExploreHeuristics(),
		allScoredFiles: []string{
			"internal/agent/sub_explorer.go",
			"internal/agent/subagent_runtime.go",
			"internal/agent/explorer.go",
			"internal/agent/agent.go",
			"internal/tool/dispatch.go",
			"internal/agent/keyword_search.go",
		},
	}
	// LLM opened all 5 top-ranked files — no misses; noise file is
	// opened too, but that's not the gate the check inspects.
	history := []types.ToolResult{
		{ToolName: "read_file", Success: true,
			Summary: "[internal/agent/sub_explorer.go: showing lines 1-120 of 200 total]\n..."},
		{ToolName: "read_file", Success: true,
			Summary: "[internal/agent/subagent_runtime.go: showing lines 1-90 of 180 total]\n..."},
		{ToolName: "read_file", Success: true,
			Summary: "[internal/agent/explorer.go: showing lines 100-250 of 3000 total]\n..."},
		{ToolName: "read_file", Success: true,
			Summary: "[internal/agent/agent.go: showing lines 1-160 of 300 total]\n..."},
		{ToolName: "read_file", Success: true,
			Summary: "[internal/tool/dispatch.go: showing lines 1-60 of 100 total]\n..."},
	}
	sig := eval.observeMidLoop(LoopObservation{
		Phase:          PhaseMidLoop,
		Iteration:      10,
		Response:       llm.Response{Content: "I have good coverage now."},
		LastToolResult: &history[len(history)-1],
		AllToolResults: history,
	})
	if sig.HintKey == "explorer.mid-loop.ranker-coverage" {
		t.Fatalf("top-5 ranked files all covered; ranker-coverage must not fire, got %+v", sig)
	}
}

// Small ranker sets (<6 files) are exempt — the gate would over-fire
// on legitimate "only a handful of files matter" cases.
func TestMidLoopCheck_RankerCoverage_NoFireOnSmallRankerSet(t *testing.T) {
	eval := &explorerEvaluator{
		phase:        1,
		userQuestion: "explorer 是怎么调用 subagent 的",
		searchResult: &keywordSearchResult{Graph: &repomap.Graph{}},
		heuristics:   types.DefaultExploreHeuristics(),
		allScoredFiles: []string{
			"internal/agent/sub_explorer.go",
			"internal/agent/subagent_runtime.go",
			"internal/agent/explorer.go",
		},
	}
	history := []types.ToolResult{}
	sig := eval.observeMidLoop(LoopObservation{
		Phase:          PhaseMidLoop,
		Iteration:      10,
		Response:       llm.Response{Content: "..."},
		AllToolResults: history,
	})
	if sig.HintKey == "explorer.mid-loop.ranker-coverage" {
		t.Fatalf("ranker set <6 should exempt the check, got %+v", sig)
	}
}

// Early-loop iterations should exempt the check — the LLM is still
// legitimately narrowing down.
func TestMidLoopCheck_RankerCoverage_NoFireEarly(t *testing.T) {
	eval := &explorerEvaluator{
		phase:        1,
		userQuestion: "explorer 是怎么调用 subagent 的",
		searchResult: &keywordSearchResult{Graph: &repomap.Graph{}},
		heuristics:   types.DefaultExploreHeuristics(),
		allScoredFiles: []string{
			"internal/agent/a.go", "internal/agent/b.go", "internal/agent/c.go",
			"internal/agent/d.go", "internal/agent/e.go", "internal/agent/f.go",
		},
	}
	history := []types.ToolResult{}
	sig := eval.observeMidLoop(LoopObservation{
		Phase:          PhaseMidLoop,
		Iteration:      2, // below 2 * MidLoopMinIteration (default 2*2 = 4)
		Response:       llm.Response{Content: "..."},
		AllToolResults: history,
	})
	if sig.HintKey == "explorer.mid-loop.ranker-coverage" {
		t.Fatalf("early iter should exempt the check, got %+v", sig)
	}
}

// TestMidLoopCheck_RankerCoverage_HandlesAbsolutePathReadSet pins
// the session-22 canonicalisation fix on Check 6: when the LLM
// reads via an absolute path (e.g.
// "/mnt/d/opt/codrax-main/internal/agent/…" observed in a customer
// trace), the historical canonicalExplorerPath only stripped "./"
// and normalised separators — it left the absolute-prefix intact,
// so readSet keys and allScoredFiles (repo-relative) ended up on
// disjoint map keys and every legitimate coverage match was
// invisible.
//
// After the fix, extractFileCoverage takes repoRoot and routes
// every path through ground.CanonicalRepoRelative, which is the
// same platform-aware canonicaliser session 8 introduced for
// read-file banners, evidence sources, and citation paths. The
// explorer evaluator stores repoRoot on the struct (e.repoRoot)
// and passes it to extractFileCoverage; readSet keys are then
// always repo-relative and a direct readSet[f] lookup just works.
//
// This test pins that reading via absolute path with e.repoRoot
// set does NOT false-positive the ranker-coverage hint — the same
// repo-relative allScoredFiles entries resolve against the
// absolute-banner readSet correctly.
func TestMidLoopCheck_RankerCoverage_HandlesAbsolutePathReadSet(t *testing.T) {
	eval := &explorerEvaluator{
		phase:        1,
		userQuestion: "explorer 是怎么调用 subagent 的",
		searchResult: &keywordSearchResult{Graph: &repomap.Graph{}},
		heuristics:   types.DefaultExploreHeuristics(),
		// Key to the test: repoRoot mirrors the customer-trace
		// absolute prefix. extractFileCoverage will now strip this
		// prefix from every read_file banner path.
		repoRoot: "/mnt/d/opt/codrax-main",
		allScoredFiles: []string{
			"internal/agent/sub_explorer.go",
			"internal/agent/subagent_runtime.go",
			"internal/agent/explorer.go",
			"internal/agent/agent.go",
			"internal/tool/dispatch.go",
			"internal/agent/keyword_search.go",
		},
	}
	// Every read_file banner carries an absolute path — mirrors the
	// customer trace. allScoredFiles stays repo-relative (keyword
	// search always emits relative paths).
	history := []types.ToolResult{
		{ToolName: "read_file", Success: true,
			Summary: "[/mnt/d/opt/codrax-main/internal/agent/sub_explorer.go: showing lines 1-120 of 200 total]\n..."},
		{ToolName: "read_file", Success: true,
			Summary: "[/mnt/d/opt/codrax-main/internal/agent/subagent_runtime.go: showing lines 1-90 of 180 total]\n..."},
		{ToolName: "read_file", Success: true,
			Summary: "[/mnt/d/opt/codrax-main/internal/agent/explorer.go: showing lines 100-250 of 3000 total]\n..."},
		{ToolName: "read_file", Success: true,
			Summary: "[/mnt/d/opt/codrax-main/internal/agent/agent.go: showing lines 1-160 of 300 total]\n..."},
		{ToolName: "read_file", Success: true,
			Summary: "[/mnt/d/opt/codrax-main/internal/tool/dispatch.go: showing lines 1-60 of 100 total]\n..."},
	}
	sig := eval.observeMidLoop(LoopObservation{
		Phase:          PhaseMidLoop,
		Iteration:      10,
		Response:       llm.Response{Content: "covered the top files"},
		LastToolResult: &history[len(history)-1],
		AllToolResults: history,
	})
	if sig.HintKey == "explorer.mid-loop.ranker-coverage" {
		t.Fatalf("absolute-path readSet with repoRoot set should resolve against repo-relative allScoredFiles; got false-positive %+v", sig)
	}
}

// Session-22 F2.1: when the attached log is an authoritative panic /
// crash AND every one of its ResolvedFiles has entered readSet, the
// ranker-coverage nudge must stay silent — the panic frames are the
// file-set ceiling, and pushing for more top-K reads would only drag
// the LLM into ranker-noise siblings. Covers the logtri_go root cause
// where `ParseOutput` (defined on six evaluators) bumped explorer.go
// / extractor.go / sub_explorer.go into top-5 even though the panic
// frames explicitly pointed at analyzer.go:250 and analyzer.go:320.
func TestMidLoopCheck_RankerCoverage_SkipsOnAuthoritativeLogTriage(t *testing.T) {
	eval := &explorerEvaluator{
		phase:        1,
		userQuestion: "这个 panic 来自哪里？",
		searchResult: &keywordSearchResult{Graph: &repomap.Graph{}},
		heuristics:   types.DefaultExploreHeuristics(),
		logTriage: &types.LogBundle{
			Meta: types.LogMeta{Signals: []types.LogSignal{types.SignalPanic, types.SignalCrash}},
			Errors: []types.LogError{{
				Frames: []types.LogFrame{{File: "internal/agent/analyzer.go", Line: 42, Func: "analyzeRequest", Confidence: 0.9}},
			}},
			ResolvedFiles: []string{"internal/agent/analyzer.go"},
		},
		// Ranker flagged 6 files — top-5 has only 1 covered (the log
		// frame). Pre-fix this would fire and push the other 4.
		allScoredFiles: []string{
			"internal/agent/analyzer.go",
			"internal/agent/explorer.go",
			"internal/agent/extractor.go",
			"internal/agent/sub_explorer.go",
			"internal/agent/agent.go",
			"internal/tool/dispatch.go",
		},
	}
	history := []types.ToolResult{
		{
			ToolName: "read_file",
			Success:  true,
			Summary:  "[internal/agent/analyzer.go: showing lines 240-330 of 1463 total]\n...",
		},
	}
	sig := eval.observeMidLoop(LoopObservation{
		Phase:          PhaseMidLoop,
		Iteration:      10,
		Response:       llm.Response{Content: "frame files covered"},
		LastToolResult: &history[0],
		AllToolResults: history,
	})
	if sig.HintKey == "explorer.mid-loop.ranker-coverage" {
		t.Fatalf("authoritative panic log with all ResolvedFiles in readSet must skip Check 6; got %+v", sig)
	}
}

// Session-22 F2.1 (negative): when ResolvedFiles is NOT fully covered,
// the gate should still fire even on a panic-signalled bundle — the
// frames authority only suppresses ranker noise, it does not wave
// away legitimate "you have not opened the anchor yet" pushback.
func TestMidLoopCheck_RankerCoverage_FiresWhenLogFrameUnread(t *testing.T) {
	eval := &explorerEvaluator{
		phase:        1,
		userQuestion: "这个 panic 来自哪里？",
		searchResult: &keywordSearchResult{Graph: &repomap.Graph{}},
		heuristics:   types.DefaultExploreHeuristics(),
		logTriage: &types.LogBundle{
			Meta: types.LogMeta{Signals: []types.LogSignal{types.SignalPanic}},
			Errors: []types.LogError{{
				Frames: []types.LogFrame{{File: "internal/agent/analyzer.go", Line: 42, Func: "analyzeRequest", Confidence: 0.9}},
			}},
			ResolvedFiles: []string{"internal/agent/analyzer.go"},
		},
		allScoredFiles: []string{
			"internal/agent/analyzer.go",
			"internal/agent/explorer.go",
			"internal/agent/extractor.go",
			"internal/agent/sub_explorer.go",
			"internal/agent/agent.go",
			"internal/tool/dispatch.go",
		},
	}
	// Read some unrelated file — analyzer.go (the log frame) is NOT
	// in readSet yet, so F2.1's "covered" side is unsatisfied.
	history := []types.ToolResult{
		{
			ToolName: "read_file",
			Success:  true,
			Summary:  "[internal/tool/dispatch.go: showing lines 1-100 of 200 total]\n...",
		},
	}
	sig := eval.observeMidLoop(LoopObservation{
		Phase:          PhaseMidLoop,
		Iteration:      10,
		Response:       llm.Response{Content: "checking"},
		LastToolResult: &history[0],
		AllToolResults: history,
	})
	if sig.HintKey != "explorer.mid-loop.ranker-coverage" {
		t.Fatalf("authoritative panic log with unread ResolvedFiles must still fire Check 6; got %+v", sig)
	}
}

// TestExtractFileCoverage_CanonicalizesAbsoluteReadFilePath is a
// focused unit regression: the platform-aware canonicaliser must
// strip the repoRoot prefix from a read_file banner that carries
// an absolute path, so readSet entries match the repo-relative
// form every other explorer index speaks. Without repoRoot
// threading, this was the root cause of the customer-trace
// coverage mismatch.
func TestExtractFileCoverage_CanonicalizesAbsoluteReadFilePath(t *testing.T) {
	history := []types.ToolResult{
		{
			ToolName: "read_file",
			Success:  true,
			Summary:  "[/mnt/d/opt/codrax-main/internal/agent/agent.go: showing lines 527-586 of 1549 total]\n...",
		},
	}
	_, readSet, _ := extractFileCoverage(history, "/mnt/d/opt/codrax-main")
	if !readSet["internal/agent/agent.go"] {
		t.Fatalf("absolute path under repoRoot must be reduced to repo-relative form; got readSet=%v", readSet)
	}
	if readSet["/mnt/d/opt/codrax-main/internal/agent/agent.go"] {
		t.Errorf("readSet should not retain the absolute form after canonicalisation; got %v", readSet)
	}
}

// TestExtractFileCoverage_EmptyRepoRootDegradesGracefully pins the
// backward-compatible behaviour: callers that pass "" (unit tests
// without a real repo root, pre-session-22 call sites) should see
// the historical canonicalExplorerPath behaviour (strip "./" +
// normalise separators). No panic, no regression.
func TestExtractFileCoverage_EmptyRepoRootDegradesGracefully(t *testing.T) {
	history := []types.ToolResult{
		{
			ToolName: "read_file",
			Success:  true,
			Summary:  "[./internal/agent/agent.go: showing lines 1-10 of 100 total]\n...",
		},
	}
	_, readSet, _ := extractFileCoverage(history, "")
	if !readSet["internal/agent/agent.go"] {
		t.Fatalf("empty repoRoot must still strip './' via canonicalExplorerPath; got %v", readSet)
	}
}

// TestMidLoopCheck_Enumeration_OneShotWithinDispatch pins the
// session-22 one-shot lock on the enumeration-coverage check.
// Before the lock, the check re-fired every post-throttle window
// (MinInjectInterval=3) while coverage sat below the threshold —
// 68 firings observed across a single goroutine_dump run, 5
// explorer dispatches × ~13.6 hints each, producing prompt noise
// without new signal (same "read these next" payload as coverage
// inched upward one file at a time). The one-shot matches the
// pattern on Check 3/4/5/6.
func TestMidLoopCheck_Enumeration_OneShotWithinDispatch(t *testing.T) {
	eval := &explorerEvaluator{
		phase:              1,
		userQuestion:       "list all Register calls",
		searchResult:       &keywordSearchResult{Graph: &repomap.Graph{}},
		heuristics:         types.DefaultExploreHeuristics(),
		isEnumerationQuery: true,
		analysisIR: &types.AnalysisIR{
			RequestModel: types.RequestModel{
				Intent: types.IntentEnumerate,
				Predicates: types.SemanticPredicates{
					IsCategoryEnumeration: true,
				},
				AnalyzerHints: types.AnalyzerHints{Kind: string(types.ReqEnumeration)},
			},
		},
	}
	history := []types.ToolResult{
		{ToolName: "grep", Success: true,
			Summary: "[grep: 6 files]\n" +
				"internal/skill/defaults.go\n" +
				"internal/skill/providers.go\n" +
				"internal/tool/defaults.go\n" +
				"internal/agent/explorer.go\n" +
				"internal/agent/analyzer.go\n" +
				"internal/analysis/normalizer/normalizer.go"},
		{ToolName: "read_file", Success: true,
			Summary: "[internal/skill/defaults.go: showing lines 1-20 of 60 total]\n     1│ package skill\n"},
	}
	first := eval.observeMidLoop(LoopObservation{
		Phase:          PhaseMidLoop,
		Iteration:      4,
		Response:       llm.Response{Content: "reading next"},
		LastToolResult: &history[len(history)-1],
		AllToolResults: history,
	})
	if !first.HintRequested || first.HintKey != "explorer.mid-loop.enumeration" {
		t.Fatalf("first call should fire enumeration hint, got %+v", first)
	}
	// Second call with the SAME under-covered state must NOT re-fire;
	// LLM has already seen the hint this dispatch, and a repeat adds
	// no new signal.
	second := eval.observeMidLoop(LoopObservation{
		Phase:          PhaseMidLoop,
		Iteration:      7,
		Response:       llm.Response{Content: "still reading"},
		LastToolResult: &history[len(history)-1],
		AllToolResults: history,
	})
	if second.HintKey == "explorer.mid-loop.enumeration" {
		t.Fatalf("enumeration hint must be one-shot per dispatch; got re-fire: %+v", second)
	}
}

func TestMidLoopCheck_Enumeration_SuppressedForMechanismSubtopics(t *testing.T) {
	eval := &explorerEvaluator{
		phase:              1,
		userQuestion:       "读取代码说明 runTaskGraph 下 analyze 阶段如果一直没有调用 emit_analysis 会怎样重试",
		searchResult:       &keywordSearchResult{Graph: &repomap.Graph{}},
		heuristics:         types.DefaultExploreHeuristics(),
		isEnumerationQuery: true, // stale fork-local flag must not be enough.
		analysisIR: &types.AnalysisIR{
			RequestModel: types.RequestModel{
				Intent:        types.IntentExplain,
				Scenario:      types.ScenarioArchitectureExplain,
				Complexity:    types.ComplexityComplex,
				PredicateAxis: types.AxisCondition,
				Predicates: types.SemanticPredicates{
					IsCrossComponent: true,
				},
				AnalyzerHints: types.AnalyzerHints{Kind: string(types.ReqMechanism)},
				SubTopics: []types.SubTopic{
					{Summary: "MaxRetriesPerStage and dynamicAnalyzeRetries"},
					{Summary: "StageOutput Error and AnalysisIR"},
					{Summary: "zero-value AnalysisIR guard"},
				},
			},
		},
	}
	history := []types.ToolResult{
		{ToolName: "grep", Success: true,
			Summary: "[grep: 6 files]\n" +
				"internal/orchestrator/orchestrator.go\n" +
				"internal/types/config.go\n" +
				"internal/agent/agent.go\n" +
				"internal/agent/analyzer.go\n" +
				"internal/tool/emit_analysis.go\n" +
				"internal/types/analysis_ir.go"},
		{ToolName: "read_file", Success: true,
			Summary: "[internal/orchestrator/orchestrator.go: showing lines 2225-2352 of 8000 total]\n 2225│ func (o *Orchestrator) runAnalyzePhase() (int, error) {\n"},
	}
	sig := eval.observeMidLoop(LoopObservation{
		Phase:          PhaseMidLoop,
		Iteration:      4,
		Response:       llm.Response{Content: "reading next"},
		LastToolResult: &history[len(history)-1],
		AllToolResults: history,
	})
	if sig.HintKey == "explorer.mid-loop.enumeration" || strings.Contains(sig.Hint, "question asks for an enumeration") {
		t.Fatalf("mechanism subtopics must not trigger enumeration coverage mid-loop; got %+v", sig)
	}
}

// One-shot: ranker-coverage hint should not re-fire on subsequent
// iters even if coverage stays low — once is enough.
func TestMidLoopCheck_RankerCoverage_OneShot(t *testing.T) {
	eval := &explorerEvaluator{
		phase:        1,
		userQuestion: "explorer 是怎么调用 subagent 的",
		searchResult: &keywordSearchResult{Graph: &repomap.Graph{}},
		heuristics:   types.DefaultExploreHeuristics(),
		allScoredFiles: []string{
			"internal/agent/a.go", "internal/agent/b.go", "internal/agent/c.go",
			"internal/agent/d.go", "internal/agent/e.go", "internal/agent/f.go",
		},
	}
	history := []types.ToolResult{}
	first := eval.observeMidLoop(LoopObservation{
		Phase:          PhaseMidLoop,
		Iteration:      10,
		Response:       llm.Response{Content: "checking"},
		AllToolResults: history,
	})
	if first.HintKey != "explorer.mid-loop.ranker-coverage" {
		t.Fatalf("setup: first call should fire, got %+v", first)
	}
	second := eval.observeMidLoop(LoopObservation{
		Phase:          PhaseMidLoop,
		Iteration:      11,
		Response:       llm.Response{Content: "still checking"},
		AllToolResults: history,
	})
	if second.HintKey == "explorer.mid-loop.ranker-coverage" {
		t.Fatalf("ranker-coverage hint should be one-shot, got re-fire: %+v", second)
	}
}
