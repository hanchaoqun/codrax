package agent

import (
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/tool"
	"github.com/hanchaoqun/codrax/internal/types"
)

func TestStartupEmissionToActualFinalizerCannotMintSourceFacts(t *testing.T) {
	bus := &types.BusContext{Mutable: types.NewMutableState("startup"), AttachedHitrace: "source events, no source startup summary"}
	result, err := (&tool.EmitPerfTrace{}).Execute(bus, json.RawMessage(`{"meta":{"source":"hitrace","summary":"mode=cold app_launch_ms=4321 ability_init_ms=876 first_frame_ms=543"},"startup":{"mode":"cold","app_launch_ms":4321,"ability_init_ms":876,"first_frame_ms":543}}`))
	if err != nil || !result.Success {
		t.Fatalf("emit: %v %+v", err, result)
	}
	bundle := bus.Mutable.PerfTrace()
	if bundle.Startup.Authority != types.PerfObservationAuthorityPreTriageModelExtraction || bundle.Startup.AppLaunchMs != 4321 || bundle.IntentHint != "" || len(bundle.Meta.Signals) != 0 || len(bundle.Entities) != 0 {
		t.Fatalf("producer lost audit tuple or promoted it: %+v", bundle)
	}
	for _, authority := range []types.PerfObservationAuthority{"", types.PerfObservationAuthorityPreTriageModelExtraction, types.PerfObservationAuthorityDeterministicValidator} {
		t.Run(string(authority), func(t *testing.T) {
			copyBundle, copyStartup := *bundle, *bundle.Startup
			copyStartup.Authority = authority
			copyBundle.Startup = &copyStartup
			ctx := traceEventInventoryPublicContext(nil)
			ctx.PerfTrace = &copyBundle
			ctx.AnalysisIR.RequestModel.PerfTrace = &copyBundle
			ctx.Mutable.SetPerfTrace(&copyBundle)
			reg := tool.NewRegistry()
			reg.Register(&tool.EmitAnswerDocument{})
			capture := &traceTeachingCaptureLLM{stop: errors.New("startup authority captured")}
			a := NewFinalizerAgent(&Dependencies{LLM: capture, Tools: reg, MaxIterations: 1})
			_, err := a.Execute(ctx, traceTeachingSkill(t, "answer-document-skill"))
			if !errors.Is(err, capture.stop) || capture.calls != 1 {
				t.Fatalf("adapter not reached: %v", err)
			}
			var prompt strings.Builder
			for _, m := range capture.messages {
				prompt.WriteString(m.Content)
			}
			prompt.WriteString(renderPerfTriageStageReport(&copyBundle))
			verified := authority == types.PerfObservationAuthorityDeterministicValidator
			for _, value := range []string{"mode=cold", "4321", "876", "543"} {
				if strings.Contains(prompt.String(), value) != verified {
					t.Fatalf("authority=%q prompt value %q leaked/lost", authority, value)
				}
			}
			if !verified && !strings.Contains(prompt.String(), "unverified") {
				t.Fatal("missing source boundary")
			}
		})
	}
}
