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
			RawRequest: "X 是怎么一路调用到 Y 的？",
			Intent:     types.IntentTrace,
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

	t.Run("multi-topic trace keeps cross component", func(t *testing.T) {
		rm := types.RequestModel{
			RawRequest:     "X 是怎么一路调用到 Y 的？同时 Z 又是怎么接进去的？",
			Intent:         types.IntentTrace,
			PredicateAxis:  types.AxisCall,
			SubTopics:      []types.SubTopic{{Summary: "X -> Y"}, {Summary: "Z -> Y"}},
			AnalyzerHints:  types.AnalyzerHints{Kind: "call_chain"},
			Predicates:     types.SemanticPredicates{IsCrossComponent: true},
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
