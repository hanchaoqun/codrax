package orchestrator

import (
	"testing"

	"github.com/hanchaoqun/codrax/internal/types"
)

func TestBuildArtifactReadinessViewUsesClosureLedgerAndTypedPayloads(t *testing.T) {
	ev := types.EvidenceItem{ID: "ev-ready", Source: "a.go", LineStart: 1}
	ir := artifactReadinessIRFixture()
	mut := types.NewMutableState("artifact readiness")
	mut.EvidenceClosure().AppendNodeArtifactRecords([]types.NodeArtifactRecord{{
		ProducerNodeID: "explore",
		Consumer:       types.RuntimeArtifactConsumerExtract,
		Artifact:       types.RuntimeArtifactRef{Kind: types.RuntimeArtifactEvidenceItem, ID: "ev-ready"},
	}})
	o := &Orchestrator{busCtx: &types.BusContext{
		AnalysisIR:    ir,
		Mutable:       mut,
		EvidenceItems: []types.EvidenceItem{ev},
	}}

	view := o.buildArtifactReadinessView(ir)
	if view == nil || !view.Ready || view.ResolvedRefCount != 1 {
		t.Fatalf("artifact readiness view = %+v, want resolved ready view", view)
	}
}

func TestBuildArtifactReadinessViewCarriesDanglingLedgerBlocker(t *testing.T) {
	ir := artifactReadinessIRFixture()
	mut := types.NewMutableState("artifact readiness dangling")
	mut.EvidenceClosure().AppendNodeArtifactRecords([]types.NodeArtifactRecord{{
		ProducerNodeID: "explore",
		Consumer:       types.RuntimeArtifactConsumerExtract,
		Artifact:       types.RuntimeArtifactRef{Kind: types.RuntimeArtifactEvidenceItem, ID: "ev-missing"},
	}})
	o := &Orchestrator{busCtx: &types.BusContext{
		AnalysisIR:    ir,
		Mutable:       mut,
		EvidenceItems: []types.EvidenceItem{{ID: "ev-present"}},
	}}

	view := o.buildArtifactReadinessView(ir)
	if view == nil || view.Ready || !view.HasBlockingLedgerIssue() || view.ReasonCode != types.ArtifactReadinessPayloadMissing {
		t.Fatalf("artifact readiness view = %+v, want dangling blocker", view)
	}
}

func TestIngestExtractConsumedArtifactsRecordsResolvedRefs(t *testing.T) {
	ev := types.EvidenceItem{ID: "ev-ready", Source: "a.go", LineStart: 1}
	ir := artifactReadinessIRFixture()
	mut := types.NewMutableState("artifact readiness consume")
	mut.EvidenceClosure().AppendNodeArtifactRecords([]types.NodeArtifactRecord{{
		ProducerNodeID: "explore",
		Consumer:       types.RuntimeArtifactConsumerExtract,
		Artifact:       types.RuntimeArtifactRef{Kind: types.RuntimeArtifactEvidenceItem, ID: "ev-ready"},
	}})
	o := &Orchestrator{busCtx: &types.BusContext{
		AnalysisIR:    ir,
		Mutable:       mut,
		EvidenceItems: []types.EvidenceItem{ev},
	}}

	view := o.buildArtifactReadinessView(ir)
	if view == nil || !view.Ready || view.ResolvedRefCount != 1 {
		t.Fatalf("artifact readiness view = %+v, want resolved ready view", view)
	}
	o.ingestExtractConsumedArtifacts(&types.TaskNode{ID: "extract", Type: types.NodeExtract}, view)
	consumed := mut.EvidenceClosure().NodeArtifactLedger().RecordsByConsumerNode("extract")
	if len(consumed) != 1 {
		t.Fatalf("consumed records = %+v, want 1", consumed)
	}
	if consumed[0].Direction != types.RuntimeArtifactConsumed ||
		consumed[0].ProducerNodeID != "explore" ||
		consumed[0].Artifact.ID != "ev-ready" {
		t.Fatalf("consumed record = %+v", consumed[0])
	}
}

func artifactReadinessIRFixture() *types.AnalysisIR {
	return &types.AnalysisIR{
		Version:      types.AnalysisIRVersion,
		RequestModel: types.RequestModel{Language: "en", Intent: types.IntentExplain},
		TaskGraph: types.TaskGraph{Nodes: []types.TaskNode{{
			ID:   "extract",
			Type: types.NodeExtract,
			Inputs: []string{
				"evidence_items",
				"answer_chains",
				"aggregate_facts",
			},
		}}},
	}
}
