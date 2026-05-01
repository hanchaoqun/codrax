package orchestrator

import (
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/types"
)

// TestRunAnswerShapeOracle_RepairFieldsPopulated pins the
// commit-58 Batch D fix (audit #7): every violation produced by
// the Answer Shape Oracle MUST carry a non-empty Repair field so
// renderViolations / RenderCompact surfaces concrete fix
// instructions to the next-Run reviewer LLM (or, for hard
// violations, to the same-Run retry).
//
// Pre-fix the 4 violations carried Detail-only and the LLM saw
// "intent=explain expects explanation-class shape" without any
// "do this" guidance — a field that's already in the violation
// struct since session 11 (Violation.Repair). The audit caught
// commit 53/55 forgetting to populate it.
func TestRunAnswerShapeOracle_RepairFieldsPopulated(t *testing.T) {
	cases := []struct {
		name      string
		setupDoc  func() *types.AnswerDocument
		setupRm   func() *types.RequestModel
		setupMu   func() *types.MutableState
		wantKind  types.ViolationKind
		wantRepair string // substring assertion; full strings are over-specified
	}{
		{
			name: "ShapeIntentMismatch — explain → value triggers repair",
			setupDoc: func() *types.AnswerDocument {
				return &types.AnswerDocument{Shape: types.ShapeValue}
			},
			setupRm: func() *types.RequestModel {
				return &types.RequestModel{Intent: types.IntentExplain}
			},
			setupMu:    func() *types.MutableState { return nil },
			wantKind:   types.ViolShapeIntentMismatch,
			wantRepair: "shape=explanation",
		},
		{
			name: "ShapeIntentMismatch — return_value → explanation triggers repair",
			setupDoc: func() *types.AnswerDocument {
				return &types.AnswerDocument{Shape: types.ShapeExplanation}
			},
			setupRm: func() *types.RequestModel {
				return &types.RequestModel{Intent: types.IntentReturnValue}
			},
			setupMu:    func() *types.MutableState { return nil },
			wantKind:   types.ViolShapeIntentMismatch,
			wantRepair: "shape=value",
		},
		{
			name: "SubTopicCountMismatch — under-coverage repair tells LLM to widen",
			setupDoc: func() *types.AnswerDocument {
				symbols := []types.AnswerSymbol{
					{File: "a.go"}, // 1 bucket vs 4 declared sub-topics
				}
				return &types.AnswerDocument{Shape: types.ShapeListOfSymbols, Symbols: symbols}
			},
			setupRm: func() *types.RequestModel {
				return &types.RequestModel{
					SubTopics: []types.SubTopic{{}, {}, {}, {}},
				}
			},
			setupMu:    func() *types.MutableState { return nil },
			wantKind:   types.ViolSubTopicCountMismatch,
			wantRepair: "widen the answer",
		},
		{
			name: "SubTopicCountMismatch — over-decomposition repair tells LLM to collapse",
			setupDoc: func() *types.AnswerDocument {
				symbols := []types.AnswerSymbol{}
				for i := 0; i < 8; i++ { // 8 buckets vs 2 declared
					symbols = append(symbols, types.AnswerSymbol{
						File: "f" + string(rune('a'+i)) + ".go",
					})
				}
				return &types.AnswerDocument{Shape: types.ShapeListOfSymbols, Symbols: symbols}
			},
			setupRm: func() *types.RequestModel {
				return &types.RequestModel{
					SubTopics: []types.SubTopic{{}, {}},
				}
			},
			setupMu:    func() *types.MutableState { return nil },
			wantKind:   types.ViolSubTopicCountMismatch,
			wantRepair: "over-decomposed",
		},
		{
			name: "DeclaredCountDrift — repair tells LLM the new count to use",
			setupDoc: func() *types.AnswerDocument {
				return &types.AnswerDocument{
					Shape: types.ShapeListOfSymbols,
					Symbols: []types.AnswerSymbol{
						{File: "a.go"}, {File: "b.go"}, {File: "c.go"},
					},
				}
			},
			setupRm: func() *types.RequestModel { return &types.RequestModel{} },
			setupMu: func() *types.MutableState {
				mu := types.NewMutableState("test")
				mu.SetEmittedAnswerSymbolDeclaredCount(7)
				return mu
			},
			wantKind:   types.ViolDeclaredCountDrift,
			wantRepair: "count=3",
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			vs := runAnswerShapeOracle(c.setupDoc(), c.setupRm(), c.setupMu())
			var found *types.Violation
			for i := range vs {
				if vs[i].Kind == c.wantKind {
					found = &vs[i]
					break
				}
			}
			if found == nil {
				t.Fatalf("expected violation kind %s; got %v", c.wantKind, vs)
			}
			if strings.TrimSpace(found.Repair) == "" {
				t.Errorf("Repair field is empty for kind=%s; commit 58 Batch D requires every soft violation to carry concrete fix guidance", c.wantKind)
			}
			if !strings.Contains(found.Repair, c.wantRepair) {
				t.Errorf("Repair %q does not contain expected fragment %q", found.Repair, c.wantRepair)
			}
		})
	}
}
