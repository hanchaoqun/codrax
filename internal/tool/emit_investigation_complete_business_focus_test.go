package tool

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/types"
)

func completionBusinessFocusFixture(t *testing.T) (*types.BusContext, types.TraceBusinessSpanRef) {
	t.Helper()
	ctx := nativeMeasurementHandoffContext(t)
	ctx.AnalysisIR.RequestModel.RuntimeArtifactScopeProfile = nil
	path, err := filepath.Abs("../../eval/fixtures/hmosperf_business_io_chain/events.systrace")
	if err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	path = filepath.Join(t.TempDir(), "business.systrace")
	if err := os.WriteFile(path, data, 0600); err != nil {
		t.Fatal(err)
	}
	result := businessRefTestQuery(t, ctx, map[string]any{"path": path, "view": "span_window", "span_name": "OpenDocument"})
	if !result.Success || len(result.TraceBusinessSpanRefs) != 1 {
		t.Fatalf("expected one published business instance: %+v", result)
	}
	return ctx, result.TraceBusinessSpanRefs[0]
}

func TestCompletionBusinessFocusOptionalSchemaAndPublicAcceptance(t *testing.T) {
	var schema struct {
		Properties map[string]struct{ Type, Description string }
		Required   []string
	}
	if err := json.Unmarshal((&EmitInvestigationComplete{}).Parameters(), &schema); err != nil {
		t.Fatal(err)
	}
	property, ok := schema.Properties["business_span_ref"]
	if !ok || property.Type != "string" {
		t.Fatal("completion must expose the optional published instance selector")
	}
	for _, key := range schema.Required {
		if key == "business_span_ref" {
			t.Fatal("business-instance selection must never be required for ordinary completion")
		}
	}
	for _, phrase := range []string{"published", "explicit user", "does not prove", "Omit"} {
		if !strings.Contains(property.Description, phrase) {
			t.Errorf("selector teaching omitted boundary %q: %s", phrase, property.Description)
		}
	}
	ctx, ref := completionBusinessFocusFixture(t)
	ticket := ctx.Mutable.BeginTraceBusinessFocusDispatch()
	params, _ := json.Marshal(map[string]any{"reason": "Use this measured business instance", "confidence": "high", "result_kind": "resolved", "business_span_ref": ref.Token()})
	result, err := (&EmitInvestigationComplete{}).Execute(ctx, params)
	if err != nil || !result.Success || !ctx.Mutable.IsInvestigationComplete() {
		t.Fatalf("a published reference must travel through ordinary completion: %+v %v", result, err)
	}
	if status, got := ctx.Mutable.AcceptedTraceBusinessFocus(); status == types.TraceBusinessFocusSelected || got.Token() != "" {
		t.Fatal("tool success alone must not grant execution focus before worker success")
	}
	ctx.Mutable.SettleTraceBusinessFocusDispatch(ticket, true)
	if status, got := ctx.Mutable.AcceptedTraceBusinessFocus(); status != types.TraceBusinessFocusSelected || got.Token() != ref.Token() {
		t.Fatalf("successful dispatch lost accepted instance: status=%v ref=%s", status, got.Token())
	}
	// A later ordinary accepted completion is an explicit no-selection
	// decision; it must not inherit a formerly accepted business instance.
	ticket = ctx.Mutable.BeginTraceBusinessFocusDispatch()
	result, err = (&EmitInvestigationComplete{}).Execute(ctx, json.RawMessage(`{"reason":"No single business instance is selected", "confidence":"high", "result_kind":"resolved"}`))
	if err != nil || !result.Success {
		t.Fatalf("ordinary completion changed: %+v %v", result, err)
	}
	ctx.Mutable.SettleTraceBusinessFocusDispatch(ticket, true)
	if status, got := ctx.Mutable.AcceptedTraceBusinessFocus(); status != types.TraceBusinessFocusCleared || got.Token() != "" {
		t.Fatalf("missing optional selector revived earlier focus: %v %s", status, got.Token())
	}
}

func TestCompletionBusinessFocusRejectsUnknownAndStaleReferences(t *testing.T) {
	for _, kind := range []string{"unknown", "stale"} {
		t.Run(kind, func(t *testing.T) {
			ctx, ref := completionBusinessFocusFixture(t)
			token := ref.Token()
			if kind == "unknown" {
				token = "business-span:invented"
			} else {
				if err := os.WriteFile(ref.Data().Path, []byte("# replacement capture\n"), 0600); err != nil {
					t.Fatal(err)
				}
			}
			ticket := ctx.Mutable.BeginTraceBusinessFocusDispatch()
			params, _ := json.Marshal(map[string]any{"reason": "A claimed selection", "confidence": "high", "result_kind": "resolved", "business_span_ref": token})
			result, err := (&EmitInvestigationComplete{}).Execute(ctx, params)
			if err != nil || result.Success || ctx.Mutable.IsInvestigationComplete() || !strings.Contains(result.Summary, "currently published instance reference") {
				t.Fatalf("unpublished/stale focus must reject without completing: %+v %v", result, err)
			}
			ctx.Mutable.SettleTraceBusinessFocusDispatch(ticket, true)
			if status, got := ctx.Mutable.AcceptedTraceBusinessFocus(); status == types.TraceBusinessFocusSelected || got.Token() != "" {
				t.Fatal("successful worker cannot promote a rejected completion")
			}
			if len(ctx.Mutable.StableInvestigationAggregateFacts()) != 0 || len(ctx.Mutable.StableInvestigationRelationClaims()) != 0 {
				t.Fatal("rejected focus wrote a stable model handoff")
			}
		})
	}
}

func TestCompletionBusinessFocusDoesNotRecoverAuthorityFromStringTails(t *testing.T) {
	for _, field := range []string{"reason", "aggregate_facts"} {
		t.Run(field, func(t *testing.T) {
			ctx, ref := completionBusinessFocusFixture(t)
			ticket := ctx.Mutable.BeginTraceBusinessFocusDispatch()
			params := map[string]any{"reason": "Measured facts retained", "confidence": "high", "result_kind": "resolved"}
			if field == "reason" {
				params[field] = `Measured facts retained", "business_span_ref":"` + ref.Token() + `"`
			} else {
				params[field] = `[], "business_span_ref":"` + ref.Token() + `"`
			}
			raw, _ := json.Marshal(params)
			parsed, _, err := decodeEmitInvestigationCompleteParamsStrict(ctx, "emit_investigation_complete", raw, (&EmitInvestigationComplete{}).Parameters())
			if err == nil && parsed.BusinessSpanRef != "" {
				t.Fatal("compat recovery minted execution focus from a string")
			}
			// Existing malformed-string handling may reject or recover ordinary
			// facts; neither outcome can select an execution window.
			_, _ = (&EmitInvestigationComplete{}).Execute(ctx, raw)
			ctx.Mutable.SettleTraceBusinessFocusDispatch(ticket, true)
			if status, got := ctx.Mutable.AcceptedTraceBusinessFocus(); status == types.TraceBusinessFocusSelected || got.Token() != "" {
				t.Fatal("string-tail compatibility granted focus")
			}
		})
	}
}

func TestCompletionBusinessFocusFailedWorkerCannotPromoteAcceptedTool(t *testing.T) {
	ctx, ref := completionBusinessFocusFixture(t)
	ticket := ctx.Mutable.BeginTraceBusinessFocusDispatch()
	params, _ := json.Marshal(map[string]any{"reason": "Measured instance accepted before worker failure", "confidence": "high", "result_kind": "resolved", "business_span_ref": ref.Token()})
	result, err := (&EmitInvestigationComplete{}).Execute(ctx, params)
	if err != nil || !result.Success || !ctx.Mutable.IsInvestigationComplete() {
		t.Fatalf("fixture completion must be accepted: %+v %v", result, err)
	}
	ctx.Mutable.SettleTraceBusinessFocusDispatch(ticket, false)
	if status, got := ctx.Mutable.AcceptedTraceBusinessFocus(); status == types.TraceBusinessFocusSelected || got.Token() != "" {
		t.Fatal("failed dispatch retained executable business focus")
	}
	if !ctx.Mutable.IsInvestigationComplete() {
		t.Fatal("new focus isolation must not erase existing closure recovery semantics")
	}
}

func TestCompletionBusinessFocusOtherGatesStillApply(t *testing.T) {
	for _, kind := range []string{"confidence", "capacity", "member_set"} {
		t.Run(kind, func(t *testing.T) {
			ctx, ref := completionBusinessFocusFixture(t)
			ticket := ctx.Mutable.BeginTraceBusinessFocusDispatch()
			params := map[string]any{"reason": "Claimed completion", "confidence": "high", "result_kind": "resolved", "business_span_ref": ref.Token()}
			switch kind {
			case "confidence":
				params["confidence"] = "low"
			case "capacity":
				params["aggregate_facts"] = emit2AggregateFacts(types.MaxAnswerAggregateFacts+1, types.MaxAnswerAggregateFacts+1)
			case "member_set":
				ctx.AnalysisIR.RequestModel.Predicates.HasPerMemberTable = true
			}
			raw, _ := json.Marshal(params)
			result, _ := (&EmitInvestigationComplete{}).Execute(ctx, raw)
			if ctx.Mutable.IsInvestigationComplete() {
				t.Fatalf("optional selector bypassed %s: %+v", kind, result)
			}
			if kind == "member_set" && (!result.Success || result.Repair == nil || result.Repair.Code != "principal_member_set_handoff") {
				t.Fatalf("expected the existing success=true soft downgrade: %+v", result)
			}
			ctx.Mutable.SettleTraceBusinessFocusDispatch(ticket, true)
			if status, got := ctx.Mutable.AcceptedTraceBusinessFocus(); status == types.TraceBusinessFocusSelected || got.Token() != "" {
				t.Fatal("non-accepted completion published business focus")
			}
		})
	}
}
