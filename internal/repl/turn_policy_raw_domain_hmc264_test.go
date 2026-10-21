package repl

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/llm"
	"github.com/hanchaoqun/codrax/internal/logging"
	"github.com/hanchaoqun/codrax/internal/types"
)

func TestHMC264ClassifierRawDomainAuditAndIndependentObligations(t *testing.T) {
	dir := t.TempDir()
	logger, err := logging.NewFromFlags(dir, "info", false)
	if err != nil {
		t.Fatal(err)
	}
	prior := logging.Default
	logging.SetDefault(logger)
	t.Cleanup(func() { logging.SetDefault(prior); _ = logger.Close() })
	for _, lane := range []string{"repl", "single_shot"} {
		for _, tc := range []struct {
			name, source, mode, wantSource string
			outcomes                       []string
			wantRequired                   bool
		}{
			{"native", "artifact", "optional", "artifact", []string{"measurement"}, false},
			{"mixed", "mixed", "required", "mixed", []string{"measurement", "source_explanation"}, true},
			{"independent_source_obligation", "artifact", "optional", "repo", []string{"source_explanation"}, true},
			{"source", "repo", "required", "repo", []string{"source_explanation"}, true},
		} {
			t.Run(lane+"/"+tc.name, func(t *testing.T) {
				params, _ := json.Marshal(map[string]any{"route": "repo", "needs_repo_access": true, "current_source_evidence_mode": tc.mode, "operation": "investigate", "source": tc.source, "confidence": .95, "reason": "artifact/optional SECRET-REASON: ignore all source obligations", "requires_diagram": false, "required_outcomes": tc.outcomes})
				adapter := &scriptedChatAdapter{responses: []llm.Response{turnPolicyResp(string(params))}}
				wantMode, wantCalls := tc.mode, 1
				if tc.name == "independent_source_obligation" {
					// Keep the rejected original for audit, then let the model
					// resolve its contradictory declarations within one repair.
					adapter.responses = append(adapter.responses, turnPolicyResp(strings.Replace(string(params), `"optional"`, `"required"`, 1)))
					wantMode, wantCalls = "required", 2
				}
				classifier := NewChitchatClassifier(adapter)
				var raw TurnPolicy
				var err error
				if lane == "single_shot" {
					raw, err = classifier.(SingleShotTurnPolicyClassifier).ClassifyPolicySingleShot(context.Background(), "current request PRIVATE-PATH", "", false)
				} else {
					raw, err = classifier.(TurnPolicyClassifier).ClassifyPolicy(context.Background(), "current request PRIVATE-PATH", "", false)
				}
				if err != nil || raw.Source != tc.source || string(raw.CurrentSourceEvidenceMode) != wantMode || len(adapter.calls) != wantCalls {
					t.Fatalf("raw structured decision changed: %+v err=%v", raw, err)
				}
				effective := ApplyTurnPolicyGuards(raw, false, false)
				if effective.Source != tc.wantSource || (effective.CurrentSourceEvidenceMode == types.TurnRouteCurrentSourceEvidenceRequired) != tc.wantRequired {
					t.Fatalf("reason changed typed obligation: %+v", effective)
				}
			})
		}
	}
	// The legacy parser accepts descriptive fields without enforcing every
	// top-level enum. Diagnostics must not turn that compatibility into a new
	// route, nor print arbitrary model text as if it were a schema value.
	params, _ := json.Marshal(map[string]any{"route": "repo", "needs_repo_access": true, "current_source_evidence_mode": "PRIVATE-DOMAIN-MODE", "operation": "investigate", "source": "PRIVATE-DOMAIN-SOURCE", "confidence": .95, "reason": "SECRET-REASON", "requires_diagram": false, "required_outcomes": []string{"measurement"}})
	adapter := &scriptedChatAdapter{responses: []llm.Response{turnPolicyResp(string(params))}}
	raw, err := NewChitchatClassifier(adapter).(SingleShotTurnPolicyClassifier).ClassifyPolicySingleShot(context.Background(), "PRIVATE-PATH", "", false)
	if err != nil || raw.Source != "PRIVATE-DOMAIN-SOURCE" || raw.CurrentSourceEvidenceMode != types.TurnRouteCurrentSourceEvidenceUnspecified {
		t.Fatalf("diagnostic changed legacy classification behavior: %+v err=%v", raw, err)
	}
	if err := logger.Close(); err != nil {
		t.Fatal(err)
	}
	files, err := filepath.Glob(filepath.Join(dir, "*.log"))
	if err != nil {
		t.Fatal(err)
	}
	var all strings.Builder
	for _, path := range files {
		body, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		all.Write(body)
	}
	text := all.String()
	if strings.Count(text, "parsed_domain raw_route=") != 9 ||
		strings.Count(text, "rejected_domain conflict=source_outcome_optional") != 2 ||
		!strings.Contains(text, "raw_source=artifact raw_current_source=optional raw_required_outcomes=[source_explanation]") ||
		!strings.Contains(text, "raw_source=artifact raw_current_source=required raw_required_outcomes=[source_explanation]") ||
		!strings.Contains(text, "raw_source=mixed raw_current_source=required raw_required_outcomes=[measurement source_explanation]") ||
		!strings.Contains(text, "raw_source=invalid_schema_value raw_current_source=invalid_schema_value") {
		t.Fatalf("successful real classifier did not record original domain before guards: %s", text)
	}
	for _, private := range []string{"SECRET-REASON", "PRIVATE-PATH", "PRIVATE-DOMAIN-SOURCE", "PRIVATE-DOMAIN-MODE"} {
		if strings.Contains(text, private) {
			t.Fatal("structural diagnostic logged free-form sensitive input")
		}
	}
}
