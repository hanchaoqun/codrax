package agent

import (
	"encoding/json"
	"fmt"
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/tool"
	"github.com/hanchaoqun/codrax/internal/types"
)

var b1621SemanticFamilies = []string{
	"jit_compile", "class_verification", "shader_compile", "runtime_compile",
	"texture_upload", "gc_pause", "trace_span",
}

var b1621PriorityFamilies = []string{
	"priority_inversion_candidate", "priority_inversion_runnable_wait",
}

func b1621AssertSemanticScope(t *testing.T, got string, zh bool) {
	t.Helper()
	wants := []string{"observed semantic work", "measured occupancy", "business meaning", "each row's declared chain position and dependency evidence", "does not grant on-chain or root-cause status"}
	forbidden := "observed on-chain semantic work"
	if zh {
		wants = []string{"已观测到的语义工作", "已测占用", "业务含义", "每条记录的链路位置和依赖凭证", "不赋予链上或根因资格"}
		forbidden = "已观测到的链上语义工作"
	}
	for _, want := range wants {
		if !strings.Contains(got, want) {
			t.Errorf("semantic-family teaching lost %q:\n%s", want, got)
		}
	}
	if strings.Contains(got, forbidden) {
		t.Errorf("category-only teaching promoted every occurrence onto the chain:\n%s", got)
	}
}

func TestB1621SemanticMechanismScopeDoesNotAssignChainMembership(t *testing.T) {
	for _, token := range b1621SemanticFamilies {
		for _, zh := range []bool{false, true} {
			t.Run(fmt.Sprintf("%s/zh=%t", token, zh), func(t *testing.T) {
				permitted, unproved := traceFinalReaderMechanismScope(token, zh)
				b1621AssertSemanticScope(t, permitted+"; "+unproved, zh)
			})
		}
	}
}

func b1621AssertPriorityScope(t *testing.T, got string, zh bool) {
	t.Helper()
	wants := []string{
		"only when each row's declared chain position and dependency evidence establish an on-chain dependency",
		"runnable scheduling delay after wakeup", "explicitly accounted compute-supply opportunity while running", "do not interchange them",
		"does not grant on-chain or root-cause status", "synchronous blocking", "direct causality",
	}
	if zh {
		wants = []string{
			"只有每条记录的链路位置和依赖凭证确认其在链上时，才陈述证据行实际计入的低优先级依赖方贡献",
			"被唤醒后处于 runnable 的调度等待", "running 期间明确计入的算力供给提升空间", "二者不得互换",
			"不赋予链上或根因资格", "不证明同步阻塞、等待其工作完成或直接因果",
		}
	}
	for _, want := range wants {
		if !strings.Contains(got, want) {
			t.Errorf("priority teaching lost conditional role or either measured contribution %q:\n%s", want, got)
		}
	}
}

func TestB1621PriorityMechanismScopePreservesBothConditionalContributions(t *testing.T) {
	for _, token := range b1621PriorityFamilies {
		for _, zh := range []bool{false, true} {
			t.Run(fmt.Sprintf("%s/zh=%t", token, zh), func(t *testing.T) {
				permitted, unproved := traceFinalReaderMechanismScope(token, zh)
				b1621AssertPriorityScope(t, permitted+"; "+unproved, zh)
			})
		}
	}
}

func TestB1621ReaderHandoffIsIndependentOfFirstPoolRole(t *testing.T) {
	for _, lang := range []string{"zh-CN", "en"} {
		t.Run(lang, func(t *testing.T) {
			zh := strings.HasPrefix(lang, "zh")
			var chain, context types.TraceCausalProjection
			families := append(append([]string(nil), b1621SemanticFamilies...), b1621PriorityFamilies...)
			for i, token := range families {
				onChain := types.TraceCausalProjectionNode{
					EvidenceID: "chain-" + token, Subject: "dependency-200", TypeToken: token,
					ImpactMS: float64(i) + .125, EffectiveImpactMS: float64(i) + .075,
					ChainRelevance: "on_chain", Rank: i + 1,
				}
				adjacent := onChain
				adjacent.EvidenceID, adjacent.Subject, adjacent.ChainRelevance, adjacent.Rank = "adjacent-"+token, "neighbor-300", "adjacent", 0
				adjacent.ImpactMS, adjacent.EffectiveImpactMS = float64(i)+.625, 0
				background := adjacent
				background.EvidenceID, background.ChainRelevance = "background-"+token, "background"
				span := adjacent
				span.EvidenceID, span.TypeToken, span.SemanticClass, span.ChainRelevance = "span-"+token, "", token, ""
				chain.PrimaryRootCauses = append(chain.PrimaryRootCauses, onChain)
				chain.RankedSeats = append(chain.RankedSeats, onChain)
				chain.OnChainCauses = append(chain.OnChainCauses, onChain)
				context.AdjacentCauses = append(context.AdjacentCauses, adjacent)
				context.BackgroundCauses = append(context.BackgroundCauses, background)
				context.SemanticSpans = append(context.SemanticSpans, span)
			}
			var original string
			for _, projections := range [][]types.TraceCausalProjection{{chain, context}, {context, chain}, {context}} {
				set := types.TraceCausalProjectionSet{Projections: projections}
				before, _ := json.Marshal(set)
				got := renderTraceFinalReaderFacingLanguageHandoff(set, nil, lang)
				b1621AssertSemanticScope(t, got, zh)
				b1621AssertPriorityScope(t, got, zh)
				for _, token := range families {
					label := tool.TraceRootCauseTypeDisplayLabel(token, zh)
					needle := fmt.Sprintf("permitted_reader_cause_label=%q", label)
					if strings.Count(got, needle) != 1 {
						t.Errorf("one category mapping must survive across all role pools: %s in\n%s", needle, got)
					}
				}
				if original != "" && got != original {
					t.Fatal("a category mapping must not assign scope from whichever occurrence is first")
				}
				original = got
				after, _ := json.Marshal(set)
				if string(before) != string(after) {
					t.Fatal("reader teaching changed original nodes, values, ranks, or chain positions")
				}
			}
		})
	}
}

func TestB1621UnknownMechanismKeepsExistingReaderBoundary(t *testing.T) {
	for _, zh := range []bool{false, true} {
		permitted, unproved := traceFinalReaderMechanismScope("future_exact_family", zh)
		if permitted != "" || unproved != "" {
			t.Fatal("unknown categories must not borrow the semantic-work mechanism scope")
		}
		lang := "en"
		if zh {
			lang = "zh"
		}
		got := renderTraceFinalReaderFacingLanguageHandoff(types.TraceCausalProjectionSet{
			Projections: []types.TraceCausalProjection{{AdjacentCauses: []types.TraceCausalProjectionNode{{TypeToken: "future_exact_family"}}}},
		}, nil, lang)
		if strings.Contains(got, "permitted_reader_mechanism_scope=") || strings.Contains(got, "permitted_reader_cause_label=") {
			t.Fatalf("unknown label acquired new mechanism or display authority:\n%s", got)
		}
	}
}
