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

	t.Run("trace intent keeps cross component", func(t *testing.T) {
		rm := types.RequestModel{
			RawRequest: "X 是怎么一路调用到 Y 的？",
			Intent:     types.IntentTrace,
			AnalyzerHints: types.AnalyzerHints{
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
		if !got.IsCrossComponent {
			t.Fatalf("IsCrossComponent = false, want true")
		}
		if reason != "" {
			t.Fatalf("reason = %q, want empty", reason)
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
}
