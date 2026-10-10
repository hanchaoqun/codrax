package agent

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	ctxbuilder "github.com/hanchaoqun/codrax/internal/context"
	"github.com/hanchaoqun/codrax/internal/loginput"
	"github.com/hanchaoqun/codrax/internal/tool"
	"github.com/hanchaoqun/codrax/internal/types"
)

// A real source receipt proves the excerpt, not the triager's interpretation
// of it. Exercise the public producer and all answer-grade consumers, rather
// than checking answer words or a hand-constructed authority flag.
func TestHMC223LogObservationPublicFactsDoNotPromoteInterpretation(t *testing.T) {
	for _, captured := range []string{
		"request=opaque-29 status=aborted",
		"token=z7 lookup status=not_found",
		"business=CatalogRefresh reason=explicitly_printed_failure",
	} {
		t.Run(captured, func(t *testing.T) {
			const label = "unverified business culprit"
			const summary = "unsupported relation between independently captured events"
			catalog, err := loginput.Prepare(context.Background(), []loginput.Input{{Name: "capture.log", Data: []byte(captured + "\n")}}, loginput.Options{})
			if err != nil {
				t.Fatal(err)
			}
			bus := &types.BusContext{AttachedLogCatalog: catalog, AttachedLog: catalog.Preview(4096), Mutable: types.NewMutableState("inspect attached records")}
			params, _ := json.Marshal(map[string]any{
				"meta": map[string]any{"lang": "unknown", "signals": []string{}}, "errors": []any{},
				"observations": []any{map[string]any{"kind": "runtime_event", "subject": label, "summary": summary, "evidence": captured, "diagnostic": true, "confidence": 1}},
			})
			registry := tool.NewRegistry()
			registry.Register(&tool.EmitLogTriage{})
			result, err := registry.Execute(bus, "emit_log_triage", params)
			if err != nil || !result.Success {
				t.Fatalf("emit: %+v %v", result, err)
			}
			bundle := bus.Mutable.LogTriage()
			if len(bundle.Observations) != 1 || !bundle.Observations[0].SourceBinding.IsVerified() {
				t.Fatalf("public source proof missing: %+v", bundle)
			}
			rm := types.RequestModel{Intent: types.IntentRootCause, Scenario: types.ScenarioRootCause, LogTriage: bundle}
			bus.AnalysisIR = &types.AnalysisIR{RequestModel: rm}
			bus.Mutable.SetRequestModel(rm)
			ledger := types.CompileObservationLedger(types.ObservationLedgerInput{LogBundle: bundle})
			for _, row := range ledger.Records {
				if row.ProvenanceLane == types.ObservationProvenanceObservedDirectCause || row.SourceRef.ClockCalibrated {
					t.Fatalf("observation received cross-event/clock authority: %+v", row)
				}
			}
			for _, surface := range []struct {
				name string
				data any
			}{
				{"ledger", ledger},
				{"claim bindings", types.CompileRuntimeArtifactClaimBindings(&rm, nil)},
				{"profile", types.BuildArtifactObservationProfile(bundle, nil)},
				{"seeds", types.CollectExternalObservationSeeds(bundle, nil)},
			} {
				wire, _ := json.Marshal(surface.data)
				for _, forbidden := range []string{label, summary} {
					if strings.Contains(string(wire), forbidden) {
						t.Errorf("%s promoted triager interpretation: %s", surface.name, wire)
					}
				}
				if !strings.Contains(string(wire), captured) {
					t.Errorf("%s lost literal business evidence: %s", surface.name, wire)
				}
			}
			ac := ctxbuilder.BuildAgentContext(bus, types.AgentFinalizer, types.StageFinalize)
			prompt := (&answerDocumentEvaluator{}).BuildInitialInstruction(ac, nil)
			for _, forbidden := range []string{label, summary} {
				if strings.Contains(prompt, forbidden) {
					t.Errorf("finalizer answer-grade support promoted %q", forbidden)
				}
			}
			if !strings.Contains(prompt, captured) {
				t.Error("finalizer lost literal observation")
			}
			if obs := bundle.Observations[0]; obs.Subject != label || obs.Summary != summary || obs.Evidence != captured {
				t.Fatalf("projection rewrote the audit bundle: %+v", obs)
			}
		})
	}
}

func TestHMC223LogObservationFactsPreserveExplicitCauseAndStack(t *testing.T) {
	const marker = "Caused by: CatalogUnavailable: missing catalog"
	const frame = "at BusinessCatalog.refresh(BusinessCatalog.java:19)"
	bus := emitHMC222Log(t, "StartupFailure: start failed\n"+marker+"\n"+frame+"\n", []map[string]any{{
		"type": "StartupFailure", "message": "start failed",
		"cause": map[string]any{"type": "CatalogUnavailable", "message": "missing catalog", "frames": []any{
			map[string]any{"raw": frame, "func": "BusinessCatalog.refresh", "confidence": 1},
		}},
		"cause_relation": map[string]any{"authority": "explicit_artifact_marker", "marker": marker},
	}})
	bundle := bus.Mutable.LogTriage()
	ledger := types.CompileObservationLedger(types.ObservationLedgerInput{LogBundle: bundle})
	var found bool
	for _, row := range ledger.Records {
		if row.Subject == "CatalogUnavailable" && row.ProvenanceLane == types.ObservationProvenanceObservedDirectCause {
			found = true
			if !strings.Contains(strings.Join(row.SupportRefs, "\n"), frame) {
				t.Fatalf("explicit cause lost its business frame: %+v", row)
			}
		}
	}
	if !found {
		t.Fatalf("source-proven explicit cause was demoted: %+v", ledger.Records)
	}
	seeds := types.CollectExternalObservationSeeds(bundle, nil)
	wire, _ := json.Marshal(seeds)
	if !strings.Contains(string(wire), "BusinessCatalog.refresh") || !strings.Contains(string(wire), "CatalogUnavailable") {
		t.Fatalf("source business/causal clues disappeared: %s", wire)
	}
}
