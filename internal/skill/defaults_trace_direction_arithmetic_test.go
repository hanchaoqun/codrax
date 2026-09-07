package skill_test

import (
	"strings"
	"testing"

	promptcontext "github.com/hanchaoqun/codrax/internal/context"
	"github.com/hanchaoqun/codrax/internal/skill"
	"github.com/hanchaoqun/codrax/internal/types"
)

// These pins exercise the actual skill registry -> context renderer -> LLM
// message path. They constrain system teaching, not the model's conclusion,
// wording, chosen root causes, or JSON fields.
func directionTeachingPrompt(t *testing.T, name string, withTrace bool) (*skill.Config, string) {
	t.Helper()
	r := skill.NewRegistry()
	skill.RegisterDefaults(r)
	cfg, err := r.Get(name)
	if err != nil {
		t.Fatal(err)
	}
	ac := &types.AgentContext{
		AgentName: types.AgentFinalizer,
		Stage:     types.StageFinalize,
		Objective: "Explain the measured contributors and repair opportunities.",
	}
	if name == "explore-skill" {
		ac.AgentName = types.AgentExplorer
		ac.Stage = types.StageExplore
	}
	if withTrace {
		ac.RuntimeArtifactPreflight = types.NormalizeRuntimeArtifactPreflightProfile(types.RuntimeArtifactPreflightProfile{
			SourceNavigationOptional: true,
			Artifacts: []types.RuntimeArtifactPreflightArtifact{{
				Kind: "trace", Source: "capture.systrace", Carrier: "request_path",
			}},
		})
	}
	var rendered strings.Builder
	for _, message := range promptcontext.ToMessages(promptcontext.BuildPromptContext(ac, cfg)) {
		rendered.WriteString(message.Content)
		rendered.WriteByte('\n')
	}
	return cfg, rendered.String()
}

func TestTraceDirectionTeachingSeparatesLeaderSubtotalAndEstimateInRenderedPrompt(t *testing.T) {
	_, prompt := directionTeachingPrompt(t, "answer-document-skill", true)
	for _, want := range []string{
		"largest SINGLE ranked contribution, not that direction's total or recoverable ceiling",
		"exact same-direction subtotal is published",
		"covers its listed members only",
		"a direction total has not been established",
		"a supply-fold/head-room estimate is not an observed saving",
		"a published lower bound or conservative fallback must not become an upper bound or a guaranteed improvement",
		"arithmetic permission alone does not establish a guaranteed combined gain",
	} {
		t.Run(want, func(t *testing.T) {
			if !strings.Contains(prompt, want) {
				t.Fatalf("rendered Trace teaching lacks %q", want)
			}
		})
	}
	if strings.Contains(prompt, "the largest seat's published value is that direction's recoverable ceiling") {
		t.Fatal("a single-row maximum must not be taught as a complete direction ceiling")
	}
	if strings.Contains(prompt, "guaranteed combined gain unless an exact typed additive") {
		t.Fatal("arithmetic authorization alone must not be taught as guaranteed repair benefit")
	}
}

func TestTraceDirectionTeachingMatchesExistingArithmeticAuthority(t *testing.T) {
	// A same-direction maximum is already smaller than a legitimate subtotal
	// under the production predicate. The variants show why the shared label
	// alone cannot authorize the same arithmetic when the evidence changes.
	members := []types.TraceCausalProjectionNode{
		{RankBoardTarget: "ui-10", StartTs: 1, EndTs: 1.009, EffectiveImpactMS: 9},
		{RankBoardTarget: "ui-10", StartTs: 1.020, EndTs: 1.023, EffectiveImpactMS: 3},
	}
	// Keep a complete shared query identity while the cases below vary the
	// occurrence envelope or target; a label alone is not a board identity.
	for i := range members {
		members[i].RankBoardParamsFingerprint = "teaching-board"
		members[i].RankQueryWindowStartTs, members[i].RankQueryWindowEndTs = 1, 1.1
	}
	_, prompt := directionTeachingPrompt(t, "answer-document-skill", true)
	for _, tc := range []struct {
		name     string
		mutate   func([]types.TraceCausalProjectionNode)
		wantTier types.TraceAnswerDirectionArithmetic
		wantMS   float64
		teaching string
	}{
		{"disjoint", nil, types.TraceAnswerDirectionArithmeticSubtotal, 12, "covers its listed members only"},
		{"overlapping", func(rows []types.TraceCausalProjectionNode) { rows[1].StartTs = 1.005 }, types.TraceAnswerDirectionArithmeticOverlap, 0, "physical overlap proves shared measured time"},
		{"missing_window", func(rows []types.TraceCausalProjectionNode) { rows[1].StartTs, rows[1].EndTs = 0, 0 }, types.TraceAnswerDirectionArithmeticNone, 0, "a direction total has not been established"},
		{"different_board", func(rows []types.TraceCausalProjectionNode) { rows[1].RankBoardTarget = "other-20" }, types.TraceAnswerDirectionArithmeticNone, 0, "do not replace the missing total with a maximum"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			rows := append([]types.TraceCausalProjectionNode(nil), members...)
			if tc.mutate != nil {
				tc.mutate(rows)
			}
			tier, value := types.TraceAnswerDirectionSectionArithmetic("lock_priority", rows, false, true)
			if tier != tc.wantTier || value != tc.wantMS {
				t.Fatalf("production arithmetic changed: got (%s, %v), want (%s, %v)", tier, value, tc.wantTier, tc.wantMS)
			}
			if !strings.Contains(prompt, tc.teaching) {
				t.Fatalf("the actual prompt contradicts or omits the arithmetic boundary %q", tc.teaching)
			}
		})
	}
}

func TestTraceDirectionTeachingPhysicalRelationIsNotRepairCounterfactual(t *testing.T) {
	_, prompt := directionTeachingPrompt(t, "answer-document-skill", true)
	for _, want := range []string{
		"physical overlap proves shared measured time, not that either repair necessarily recovers that time",
		"A guaranteed saved duration or joint benefit needs separate counterfactual evidence",
		"small-overlap filter or roster cap",
		"never invent a value or an independence claim from omission",
		"does NOT prove that the seats overlap, that one depends on another, or that fixing one makes another disappear",
	} {
		t.Run(want, func(t *testing.T) {
			if !strings.Contains(prompt, want) {
				t.Fatalf("rendered relation teaching lacks %q", want)
			}
		})
	}
	for _, retired := range []string{
		"so fixing either side already recovers the PUBLISHED shared part",
		"read that as 'no significant overlap'",
	} {
		if strings.Contains(prompt, retired) {
			t.Errorf("rendered relation teaching retains unsupported inference %q", retired)
		}
	}
}

func TestTraceDirectionTeachingAggregateExceptionsAgreeAcrossStages(t *testing.T) {
	var supplyBody string
	for _, name := range []string{"explore-skill", "answer-document-skill"} {
		t.Run(name, func(t *testing.T) {
			cfg, prompt := directionTeachingPrompt(t, name, true)
			for _, item := range cfg.WorkflowTierB {
				if !strings.HasPrefix(item.Body, "TRACE MIXED SUPPLY VERDICT:") {
					continue
				}
				if !item.AppliesTo.RequiresTrace || len(item.OnViolation) != 0 {
					t.Fatalf("supply teaching must remain trace-only soft guidance: %+v", item)
				}
				if supplyBody != "" && supplyBody != item.Body {
					t.Fatal("explorer and finalizer must not teach different supply aggregation authority")
				}
				supplyBody = item.Body
				for _, want := range []string{
					"exact same-direction subtotal",
					"only for its explicitly listed members and caliber",
					"arithmetic permission is not proof of an achieved or guaranteed repair benefit",
				} {
					if !strings.Contains(item.Body, want) || !strings.Contains(prompt, item.Body) {
						t.Errorf("rendered supply guidance lacks %q", want)
					}
				}
				return
			}
			t.Fatal("supply guidance missing")
		})
	}
	_, finalPrompt := directionTeachingPrompt(t, "answer-document-skill", true)
	if !strings.Contains(finalPrompt, "an exact typed direction subtotal licenses only the listed members of that same board and direction") {
		t.Fatal("the general no-self-sums rule must allow the already-published exact direction subtotal")
	}
}

func TestTraceDirectionTeachingRetainsAxesAndSoftApplicability(t *testing.T) {
	cfg, prompt := directionTeachingPrompt(t, "answer-document-skill", true)
	_, plain := directionTeachingPrompt(t, "answer-document-skill", false)
	for _, item := range cfg.WorkflowTierB {
		if !strings.HasPrefix(item.Body, "TYPED WORD-FACE CONSUMPTION:") {
			continue
		}
		if !item.AppliesTo.RequiresTrace || len(item.OnViolation) != 0 {
			t.Fatalf("direction guidance must not add a reject/retry lane: %+v", item)
		}
		if strings.Count(prompt, item.Body) != 1 || strings.Contains(plain, item.Body) {
			t.Fatal("the shared direction guidance must render once only on the typed Trace lane")
		}
		for _, want := range []string{
			"EVERY direction value published on ON-CHAIN seated causes",
			"never a proven cause",
			"discounted (折算) caliber enters the enumeration as its OWN direction entry",
			"Root causes have TWO dimensions",
			"raw time occupancy guiding NEW fix directions",
			"name the top member spans",
			"never promote a mention row into the primary-cause discussion",
		} {
			if !strings.Contains(item.Body, want) {
				t.Errorf("existing chain/caliber/business teaching lost %q", want)
			}
		}
		return
	}
	t.Fatal("direction guidance missing")
}
