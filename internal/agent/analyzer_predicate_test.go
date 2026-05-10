package agent

import (
	"testing"

	"github.com/hanchaoqun/codrax/internal/types"
)

func TestReconcileSemanticPredicates(t *testing.T) {
	t.Run("single exact-target config trace clears cross component", func(t *testing.T) {
		rm := types.RequestModel{
			RawRequest: "explore_mid_loop_hint_budget 的最终有效值是怎么计算出来的？给我 code default / codrax.yaml / CLI 三层的覆盖优先级。",
			Intent:     types.IntentConfigQuery,
			Scenario:   types.ScenarioConfigTrace,
			AnalyzerHints: types.AnalyzerHints{
				Kind:              "config_mapping",
				Entities:          []string{"explore_mid_loop_hint_budget", "codrax.yaml"},
				PrimaryEntities:   []string{"explore_mid_loop_hint_budget", "codrax.yaml"},
				MentionedEntities: []string{"explore_mid_loop_hint_budget", "codrax.yaml"},
			},
			Predicates: types.SemanticPredicates{
				IsCrossComponent: true,
			},
		}
		got, reason := reconcileSemanticPredicates(rm)
		if got.IsCrossComponent {
			t.Fatalf("IsCrossComponent = true, want false")
		}
		if reason == "" {
			t.Fatal("reason = empty, want reconcile reason")
		}
	})

	t.Run("single-topic structural trace clears cross component", func(t *testing.T) {
		rm := types.RequestModel{
			RawRequest:    "X 是怎么一路调用到 Y 的？",
			Intent:        types.IntentTrace,
			PredicateAxis: types.AxisCall,
			AnalyzerHints: types.AnalyzerHints{
				Kind:              "call_chain",
				Entities:          []string{"X", "Y"},
				PrimaryEntities:   []string{"X", "Y"},
				MentionedEntities: []string{"X", "Y"},
				ExactTargets:      []string{"X"},
			},
			Predicates: types.SemanticPredicates{
				IsCrossComponent: true,
			},
		}
		got, reason := reconcileSemanticPredicates(rm)
		if got.IsCrossComponent {
			t.Fatalf("IsCrossComponent = true, want false")
		}
		if reason == "" {
			t.Fatal("reason = empty, want reconcile reason")
		}
	})

	t.Run("relational lookup keeps cross component", func(t *testing.T) {
		rm := types.RequestModel{
			RawRequest: "哪些 handler 会返回 Foo？",
			Intent:     types.IntentEnumerate,
			AnalyzerHints: types.AnalyzerHints{
				Entities:          []string{"handler", "Foo"},
				PrimaryEntities:   []string{"handler", "Foo"},
				MentionedEntities: []string{"handler", "Foo"},
				ExactTargets:      []string{"Foo"},
			},
			Predicates: types.SemanticPredicates{
				IsCrossComponent:   true,
				IsRelationalLookup: true,
			},
		}
		got, reason := reconcileSemanticPredicates(rm)
		if !got.IsCrossComponent {
			t.Fatalf("IsCrossComponent = false, want true")
		}
		if reason != "" {
			t.Fatalf("reason = %q, want empty", reason)
		}
	})

	t.Run("cross_component without sub_topics preserved (defer to gate retry)", func(t *testing.T) {
		// Pre-2026-05-02 the now-removed R1.2 auto-fix demoted
		// IsCrossComponent=true → false whenever SubTopics was empty.
		// Run 3 of the audit (2026-05-02) showed this swallows the
		// LLM's hard signal and short-circuits the gate's retry path.
		// Reconcile must now LEAVE IsCrossComponent=true and let
		// gate.checkSubtopicCoherence R1.2 fire so the analyzer can
		// retry with a hint to add sub_topics.
		rm := types.RequestModel{
			RawRequest: "对比 read 模式的 explorer 阶段和 write 模式的 verify 阶段，它们的 retry 机制有什么不同？",
			Intent:     types.IntentExplain,
			Scenario:   types.ScenarioArchitectureExplain,
			AnalyzerHints: types.AnalyzerHints{
				Kind:              "mechanism",
				Entities:          []string{"explorer", "verifier"},
				PrimaryEntities:   []string{"explorer", "verifier"},
				MentionedEntities: []string{"explorer", "verifier"},
			},
			Predicates: types.SemanticPredicates{
				IsCrossComponent: true,
			},
		}
		got, reason := reconcileSemanticPredicates(rm)
		if !got.IsCrossComponent {
			t.Fatalf("IsCrossComponent demoted; want preserved (gate R1.2 should drive the retry instead)")
		}
		if reason != "" {
			t.Fatalf("reason = %q; want empty (no auto-fix applied)", reason)
		}
	})

	t.Run("category enumeration with single exact target keeps cross component (B.4)", func(t *testing.T) {
		// Pre-B.4 the rule 1 auto-demote would flip IsCrossComponent=false
		// because exact-target=1 + SubTopics=[] + !IsRelationalLookup +
		// Intent != Trace. But IsCategoryEnumeration is an explicit
		// multi-axis signal — "what kinds of X" iterates across all
		// subtypes / categorisations of X and the answer spans multiple
		// component implementations. Reconcile must now preserve
		// IsCrossComponent=true so the rest of the pipeline (complexity,
		// shape selection, ranker boosts) sees the multi-axis signal.
		rm := types.RequestModel{
			RawRequest: "what kinds of agents extend BaseAgent?",
			Intent:     types.IntentEnumerate,
			AnalyzerHints: types.AnalyzerHints{
				Kind:         "list_of_symbols",
				ExactTargets: []string{"BaseAgent"},
			},
			Predicates: types.SemanticPredicates{
				IsCrossComponent:      true,
				IsCategoryEnumeration: true,
			},
		}
		got, reason := reconcileSemanticPredicates(rm)
		if !got.IsCrossComponent {
			t.Fatalf("category enumeration must preserve IsCrossComponent=true; got false reason=%q", reason)
		}
		if reason != "" {
			t.Fatalf("no demotion expected; got reason=%q", reason)
		}
	})

	t.Run("count question with single exact target keeps cross component (B.4)", func(t *testing.T) {
		// IsCountQuestion by definition aggregates across multiple source
		// units → the cross-aggregation IS the cross-component dimension
		// even when a single named target appears in the question.
		rm := types.RequestModel{
			RawRequest: "how many tools call helper X?",
			Intent:     types.IntentEnumerate,
			AnalyzerHints: types.AnalyzerHints{
				Kind:         "scalar",
				ExactTargets: []string{"X"},
			},
			Predicates: types.SemanticPredicates{
				IsCrossComponent: true,
				IsCountQuestion:  true,
			},
		}
		got, reason := reconcileSemanticPredicates(rm)
		if !got.IsCrossComponent {
			t.Fatalf("count question must preserve IsCrossComponent=true; got false reason=%q", reason)
		}
		if reason != "" {
			t.Fatalf("no demotion expected; got reason=%q", reason)
		}
	})

	t.Run("history lookup with single exact target keeps cross component (B.4)", func(t *testing.T) {
		// IsHistoryLookup → VCS-metadata answer crossing commits / authors;
		// the history axis is orthogonal to the named target.
		rm := types.RequestModel{
			RawRequest: "when was function X last touched?",
			Intent:     types.IntentExplain,
			AnalyzerHints: types.AnalyzerHints{
				Kind:         "history",
				ExactTargets: []string{"X"},
			},
			Predicates: types.SemanticPredicates{
				IsCrossComponent: true,
				IsHistoryLookup:  true,
			},
		}
		got, reason := reconcileSemanticPredicates(rm)
		if !got.IsCrossComponent {
			t.Fatalf("history lookup must preserve IsCrossComponent=true; got false reason=%q", reason)
		}
		if reason != "" {
			t.Fatalf("no demotion expected; got reason=%q", reason)
		}
	})

	t.Run("signalsExplicitMultiAxis helper covers explicit multi-axis signals (B.4)", func(t *testing.T) {
		if signalsExplicitMultiAxis(types.SemanticPredicates{}) {
			t.Errorf("zero-value predicates must NOT be multi-axis")
		}
		if !signalsExplicitMultiAxis(types.SemanticPredicates{IsCategoryEnumeration: true}) {
			t.Errorf("IsCategoryEnumeration must be multi-axis")
		}
		if !signalsExplicitMultiAxis(types.SemanticPredicates{IsCountQuestion: true}) {
			t.Errorf("IsCountQuestion must be multi-axis")
		}
		if !signalsExplicitMultiAxis(types.SemanticPredicates{IsHistoryLookup: true}) {
			t.Errorf("IsHistoryLookup must be multi-axis")
		}
		if !signalsExplicitMultiAxis(types.SemanticPredicates{IsDiagnosticQuestion: true}) {
			t.Errorf("IsDiagnosticQuestion must be multi-axis")
		}
		// IsRelationalLookup is intentionally NOT folded into the helper —
		// rule 1 checks it on its own line for separate audit-log signal.
		if signalsExplicitMultiAxis(types.SemanticPredicates{IsRelationalLookup: true}) {
			t.Errorf("IsRelationalLookup must NOT be folded into signalsExplicitMultiAxis (separate concern)")
		}
	})

	t.Run("multi-topic trace keeps cross component", func(t *testing.T) {
		rm := types.RequestModel{
			RawRequest:    "X 是怎么一路调用到 Y 的？同时 Z 又是怎么接进去的？",
			Intent:        types.IntentTrace,
			PredicateAxis: types.AxisCall,
			SubTopics:     []types.SubTopic{{Summary: "X -> Y"}, {Summary: "Z -> Y"}},
			AnalyzerHints: types.AnalyzerHints{Kind: "call_chain"},
			Predicates:    types.SemanticPredicates{IsCrossComponent: true},
		}
		got, reason := reconcileSemanticPredicates(rm)
		if !got.IsCrossComponent {
			t.Fatalf("IsCrossComponent = false, want true")
		}
		if reason != "" {
			t.Fatalf("reason = %q, want empty", reason)
		}
	})
}
