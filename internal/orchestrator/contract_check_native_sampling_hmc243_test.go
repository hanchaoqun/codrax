package orchestrator

import (
	"encoding/json"
	"path/filepath"
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/tool"
	"github.com/hanchaoqun/codrax/internal/types"
)

func nativeSamplingReviewFixtureHMC243(t *testing.T) *types.AnswerRuntimeMeasurementReceipt {
	t.Helper()
	dir := t.TempDir()
	bus := &types.BusContext{RepoRoot: dir, WorkDir: dir, Mutable: types.NewMutableState("sampling facts")}
	path, err := filepath.Abs("../../eval/fixtures/hmosperf_lossless_sampling/paths.perftrace")
	if err != nil {
		t.Fatal(err)
	}
	args, _ := json.Marshal(map[string]any{"source": "path", "path": path, "view": "perf_stats", "pid": 11, "time_start": 1, "time_end": 2, "pattern": "symbol=work"})
	result, err := (&tool.TraceQuery{}).Execute(bus, args)
	if err != nil || !result.Success {
		t.Fatalf("public native query: %v %s", err, result.Summary)
	}
	contract := types.BuildRuntimeMeasurementContract(types.ObservationLedgerInput{ToolResults: []types.ToolResult{result}})
	for _, table := range contract.Choices() {
		if !table.HasNativeSamplingFacts() || table.View != types.RuntimeMeasurementSummary || !strings.Contains(table.Label, "/app/A.so") {
			continue
		}
		ref := &types.AnswerRuntimeMeasurementReceipt{ObservationID: table.ObservationID, View: table.View}
		if types.BindRuntimeMeasurementReceipt(ref, contract) {
			return ref
		}
	}
	t.Fatal("public native sampling facts unavailable")
	return nil
}

func TestNativeSamplingFactDiagramDoesNotHideOrFulfilRelationsHMC243(t *testing.T) {
	for _, family := range []string{"sampling", "event"} {
		for _, caliber := range []types.TraceCausalClaimCaliber{"", types.TraceCausalClaimNoConclusion} {
			t.Run(family+"/"+string(caliber), func(t *testing.T) {
				ref := nativeSamplingReviewFixtureHMC243(t)
				if family == "event" {
					ref = nativeEventReviewFixtureHMC236(t)
				}
				native := types.AnswerBlock{ID: "native-facts", Kind: types.BlockDiagram, RuntimeMeasurement: ref, TraceCausalClaimCaliber: caliber}
				if !isBoundNativeEventFactDiagram(native) {
					t.Error("pure current fact surface not recognized")
				}
				view := diagramRequiredView(types.DiagramSequence)
				doc := &types.AnswerDocumentV2{Blocks: []types.AnswerBlock{native}}
				if vs := validateDiagramEdgeSupport(doc, view); len(vs) != 1 || vs[0].Kind != types.ViolDiagramEdgeUnsupported {
					t.Errorf("edge-free native facts satisfied required relationship diagram: %+v", vs)
				}
				view.DiagramPlan.Required = false
				if vs := validateDiagramEdgeSupport(doc, view); len(vs) != 0 {
					t.Errorf("optional facts acquired relation obligations: %+v", vs)
				}
				view.DiagramPlan.Required = true
				doc.Blocks = append(doc.Blocks, types.AnswerBlock{ID: "ordinary", Kind: types.BlockDiagram, Diagram: &types.AnswerDiagramBlock{Kind: types.DiagramFlow, Body: "flowchart TB\nA-->B"}})
				if vs := validateDiagramEdgeSupport(doc, view); len(vs) != 1 || !strings.Contains(vs[0].Detail, "ordinary") {
					t.Errorf("native facts hid following ordinary diagram: %+v", vs)
				}
			})
		}
	}
}

func TestNativeSamplingFactDiagramExemptionRemainsExactHMC243(t *testing.T) {
	native := types.AnswerBlock{ID: "native", Kind: types.BlockDiagram, RuntimeMeasurement: nativeSamplingReviewFixtureHMC243(t)}
	for name, mutate := range map[string]func(*types.AnswerBlock){
		"unbound":               func(b *types.AnswerBlock) { b.RuntimeMeasurement.BoundTable = nil },
		"different receipt":     func(b *types.AnswerBlock) { b.RuntimeMeasurement.ObservationID += "-other" },
		"modified native facts": func(b *types.AnswerBlock) { b.RuntimeMeasurement.BoundTable.Rows[0][0] += " forged" },
		"event selector on sampling": func(b *types.AnswerBlock) {
			b.RuntimeMeasurement.EventSelection = &types.RuntimeEventSelection{RowIDs: []string{"event-1"}}
		},
		"model diagram": func(b *types.AnswerBlock) {
			b.Diagram = &types.AnswerDiagramBlock{Kind: types.DiagramSequence, Body: "sequenceDiagram\nA->>B: call"}
		},
		"model text":    func(b *types.AnswerBlock) { b.Text = "model cause" },
		"model rows":    func(b *types.AnswerBlock) { b.Items = []types.AnswerBlockItem{{Label: "model row"}} },
		"model columns": func(b *types.AnswerBlock) { b.Columns = []string{"model column"} },
		"work relation": func(b *types.AnswerBlock) { b.RuntimeWorkRelation = &types.AnswerRuntimeWorkRelationReceipt{} },
		"edges":         func(b *types.AnswerBlock) { b.EdgeAnchors = []types.DiagramEdgeAnchor{{FromNode: "A", ToNode: "B"}} },
		"participant": func(b *types.AnswerBlock) {
			b.ParticipantBoundaries = []types.DiagramParticipantBoundary{{Participant: "A"}}
		},
		"relation":       func(b *types.AnswerBlock) { b.RelationClaims = []types.AnswerRelationClaim{{}} },
		"claims":         func(b *types.AnswerBlock) { b.ClaimUses = []types.RenderedClaimUse{{}} },
		"causal caliber": func(b *types.AnswerBlock) { b.TraceCausalClaimCaliber = types.TraceCausalClaimTypedChain },
		"relation scope": func(b *types.AnswerBlock) { b.RequestedRelationScope = "unproven" },
		"json roundtrip": func(b *types.AnswerBlock) {
			raw, _ := json.Marshal(b.RuntimeMeasurement.BoundTable)
			var table types.RuntimeMeasurementTable
			_ = json.Unmarshal(raw, &table)
			b.RuntimeMeasurement.BoundTable = &table
		},
	} {
		t.Run(name, func(t *testing.T) {
			candidate := native
			candidate.RuntimeMeasurement = native.RuntimeMeasurement.Clone()
			mutate(&candidate)
			if isBoundNativeEventFactDiagram(candidate) {
				t.Fatal("non-native or mixed relation surface acquired pure-fact exemption")
			}
		})
	}
}
