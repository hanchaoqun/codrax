package repl

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/llm"
	"github.com/hanchaoqun/codrax/internal/types"
)

// This captures the actual classifier request; scripted decisions verify
// preservation of independent domains, not natural-language classification.
func TestHMC249SourceDomainActualClassifier(t *testing.T) {
	for _, tc := range []struct{ name, source, mode, outcomes string }{
		{"native_in_repo", "artifact", "optional", "measurement"},
		{"current_implementation", "repo", "required", "source_explanation"},
		{"mixed_implementation", "mixed", "required", "measurement\",\"source_explanation"},
	} {
		for _, single := range []bool{false, true} {
			t.Run(tc.name+map[bool]string{false: "/repl", true: "/single"}[single], func(t *testing.T) {
				a := &scriptedChatAdapter{responses: []llm.Response{turnPolicyResp(`{"route":"repo","needs_repo_access":true,"current_source_evidence_mode":"` + tc.mode + `","operation":"investigate","source":"` + tc.source + `","confidence":0.95,"reason":"declared evidence domain","requires_diagram":false,"required_outcomes":["` + tc.outcomes + `"]}`)}}
				c := NewChitchatClassifier(a)
				var got TurnPolicy
				var err error
				if single {
					got, err = c.(SingleShotTurnPolicyClassifier).ClassifyPolicySingleShot(context.Background(), "比较采集中的调用位置和检查进度", "", false)
				} else {
					got, err = c.(TurnPolicyClassifier).ClassifyPolicy(context.Background(), "比较采集中的调用位置和检查进度", "", false)
				}
				if err != nil || len(a.calls) != 1 {
					t.Fatalf("classifier: %v calls=%d", err, len(a.calls))
				}
				if got.Source != tc.source || got.CurrentSourceEvidenceMode != types.TurnRouteCurrentSourceEvidenceMode(tc.mode) {
					t.Fatalf("typed domain was rewritten: %+v", got)
				}
				var schema struct {
					Properties map[string]struct {
						Description string `json:"description"`
					} `json:"properties"`
				}
				if err := json.Unmarshal(a.calls[0].tools[0].Parameters, &schema); err != nil {
					t.Fatal(err)
				}
				var prompt string
				for _, m := range a.calls[0].messages {
					if m.Role == "system" {
						prompt += m.Content
					}
				}
				for name, body := range map[string]string{"schema": schema.Properties["source"].Description, "prompt": prompt} {
					for _, want := range []string{"current checkout's implementation", "storage location does not determine", "recorded callsites"} {
						if !strings.Contains(body, want) {
							t.Errorf("%s missing %q", name, want)
						}
					}
					for _, retired := range []string{"repo = requires reading repository files", "repo            — answer requires repository access"} {
						if strings.Contains(body, retired) {
							t.Errorf("%s retains access/source conflation %q", name, retired)
						}
					}
				}
				if !strings.Contains(prompt, "current_source_evidence_mode is orthogonal to needs_repo_access") {
					t.Fatal("source/access separation lost")
				}
			})
		}
	}
}
