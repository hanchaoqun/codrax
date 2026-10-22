package agent

import (
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/types"
)

// Exercise the actual stage adapter, not only a formatter: the accepted
// investigator value must survive while its display label cannot become a
// separately proved physical relationship in a competing context section.
func TestHMC281CommandValueAuthorityPublicHandoff(t *testing.T) {
	for _, label := range []string{"trace 时间轴到实际时间的固定偏移", "hardware throughput", "available CPU capacity", "matching record count"} {
		for _, stage := range []string{"extract", "finalize"} {
			t.Run(label+"/"+stage, func(t *testing.T) {
				ctx := hmc266NegativeContext(t)
				ctx.AnalysisIR.RequestModel.RuntimeQuestionProfile = nil
				ctx.AnalysisIR.RequestModel.Intent = types.IntentExplain
				ctx.AnalysisIR.RequestModel.Predicates.IsScalarAnswer = true
				fact := types.AnswerAggregateFact{
					Kind: types.AnswerAggregateScalar, Role: types.AnswerAggregateRolePrincipalAnswer,
					Label: label, Value: "1787106904.242698", Unit: "s",
					Provenance: "exec_command arithmetic", SupportRefs: []string{"capture.systrace:429168"},
					Dimensions: []types.AnswerAggregateDimension{{Name: "origin", Value: "command_measurement"}},
				}
				ctx.Mutable.SetInvestigationAggregateFacts([]types.AnswerAggregateFact{fact})
				ctx.Mutable.SetInvestigationComplete("accepted calculation")
				ctx.Mutable.RetainInvestigationAggregateFacts()
				before := hmc266NegativeSnapshot(t, ctx)
				prompt := hmc266NegativePrompt(t, ctx, stage)
				found := false
				for _, line := range strings.Split(prompt, "\n") {
					if strings.HasPrefix(line, "- kind=`scalar_value`") && strings.Contains(line, label) {
						found = true
						for _, want := range []string{"value=`1787106904.242698`", "unit=s", "model_label=", "value_support=`reported_command_value`", "label_authority=`model_inference`"} {
							if !strings.Contains(line, want) {
								t.Errorf("public %s value-only projection missing %q: %s", stage, want, line)
							}
						}
					}
					if strings.HasPrefix(line, "- `aggregate_facts[") && strings.Contains(line, label) {
						if strings.Contains(line, "policy=`hard`") || strings.Contains(line, "authority_ceiling=`factual`") || !strings.Contains(line, "model_label=") {
							t.Errorf("label acquired whole-claim authority: %s", line)
						}
					}
				}
				if !found {
					t.Fatal("valid computed value was hidden from the actual stage request")
				}
				if before != hmc266NegativeSnapshot(t, ctx) {
					t.Fatal("presentation mutated original receipt, native facts, or causal authority")
				}
			})
		}
	}
}
