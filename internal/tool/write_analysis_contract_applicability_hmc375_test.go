package tool

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/types"
)

func TestHMC375WriteAnalysisSourceOutlineDoesNotMintProofDebt(t *testing.T) {
	for _, tc := range []struct {
		name, atom       string
		reject, required bool
	}{
		{"incompatible required outline", `{"id":"c","kind":"file_layout","operator":"satisfies","subject":"pkg/other.py","expected":"结果采用另一种计算规则","evidence_ref":"pkg/other.py:1-2","required":true}`, true, false},
		{"optional outline stays useful", `{"id":"c","kind":"file_layout","operator":"satisfies","subject":"pkg/other.py","expected":"Outline for the planner","evidence_ref":"pkg/other.py:1-2","required":false}`, false, false},
		{"missing operator with empty comparator is still an outline", `{"id":"c","kind":"FILE_LAYOUT","subject":"pkg/else.go","expected":"not an assertion","comparator":{},"required":true}`, true, false},
		{"exact layout requirement remains", `{"id":"c","kind":"file_layout","operator":"contains","subject":"pkg/other.py","expected":"type Example","evidence_ref":"pkg/other.py:4","required":true}`, false, true},
		{"model cannot stamp planning-only authority", `{"id":"c","kind":"file_layout","operator":"contains","subject":"pkg/other.py","expected":"type Example","evidence_ref":"pkg/other.py:4","source":"write_analyzer:planning_only_source_outline","required":true}`, false, true},
		{"runtime with a producing-line reference remains", `{"id":"c","kind":"observable","operator":"returns","subject":"choose","expected":"target","evidence_ref":"pkg/other.py:1-2","required":true}`, false, true},
		{"executable comparator remains", `{"id":"c","kind":"file_layout","operator":"satisfies","subject":"new document layout","expected":"existing rendered shape","comparator":{"subject":"baseline render","operator":"equals","expected":"same number of sections","relation":"same_as","evidence_ref":"test_render.py:14"},"required":true}`, false, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			bus := newTestBusForWriteAnalysis()
			res, err := (&EmitWriteAnalysis{}).Execute(bus, json.RawMessage(`{"task":{"kind":"bugfix","scope":"micro","summary":"change behavior"},"risk":{"overall":"low"},"behavior_contracts":[`+tc.atom+`]}`))
			if err != nil {
				t.Fatal(err)
			}
			if tc.reject {
				if res.Success || bus.Mutable.WriteAnalysisIR() != nil || !strings.Contains(res.Summary, "behavior_contracts[0]") || !strings.Contains(res.Summary, "required=false") {
					t.Fatalf("non-checkable required outline entered durable IR: %+v", res)
				}
				return
			}
			if !res.Success {
				t.Fatalf("compatible declaration rejected: %+v", res)
			}
			contracts := bus.Mutable.WriteAnalysisIR().Request.BehaviorContracts
			// Normalization is reused by planning and proof generation. An
			// admitted outline must not acquire authority on that second pass.
			for _, round := range [][]types.WriteBehaviorContract{contracts, types.NormalizeWriteBehaviorContracts(contracts, nil)} {
				if len(round) != 1 || round[0].Required != tc.required || (len(types.RequiredWriteBehaviorContractIDs(round, true)) == 1) != tc.required || round[0].EvidenceRef != contracts[0].EvidenceRef {
					t.Fatalf("declaration changed provenance or obligation at handoff: %+v", round)
				}
			}
		})
	}
}
