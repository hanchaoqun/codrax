package orchestrator

import (
	"strings"

	"github.com/hanchaoqun/codrax/internal/agent"
	"github.com/hanchaoqun/codrax/internal/types"
)

const (
	exploreNodeArtifactConsumerExtract = "extract"

	exploreNodeArtifactReasonEvidenceItem              = "explore_stage_output_evidence_item"
	exploreNodeArtifactReasonAnswerChain               = "explore_stage_output_answer_chain"
	exploreNodeArtifactReasonAggregateFact             = "explore_accepted_aggregate_fact"
	exploreNodeArtifactReasonAnalysisRefinementHandoff = "explore_analysis_refinement_handoff"
)

type exploreNodeArtifactProjectionSnapshot struct {
	evidenceIDs       map[string]bool
	answerChainIDs    map[string]bool
	aggregateFactIDs  map[string]bool
	aggregateFactList []types.AnswerAggregateFact
	progressDecision  types.ProgressDecision
}

func captureExploreNodeArtifactProjectionSnapshot(bus *types.BusContext, mut *types.MutableState) exploreNodeArtifactProjectionSnapshot {
	snap := exploreNodeArtifactProjectionSnapshot{
		evidenceIDs:      map[string]bool{},
		answerChainIDs:   map[string]bool{},
		aggregateFactIDs: map[string]bool{},
	}
	if bus != nil {
		for _, item := range bus.EvidenceItems {
			if id := types.RuntimeArtifactIDForEvidenceItem(item); id != "" {
				snap.evidenceIDs[id] = true
			}
		}
		for _, chain := range bus.AnswerChains {
			if id := types.RuntimeArtifactIDForAnswerChain(chain); id != "" {
				snap.answerChainIDs[id] = true
			}
		}
	}
	if mut != nil {
		var facts []types.AnswerAggregateFact
		if turnA := mut.TurnAArtifacts(); turnA != nil {
			facts = types.MergeAnswerAggregateFacts(facts, turnA.AcceptedAggregateFacts)
		}
		facts = types.MergeAnswerAggregateFacts(facts, mut.StableInvestigationAggregateFacts())
		snap.aggregateFactList = facts
		for _, fact := range facts {
			if id := types.RuntimeArtifactIdentityForAggregateFact(fact); id != "" {
				snap.aggregateFactIDs[id] = true
			}
		}
		if closure := mut.EvidenceClosure(); closure != nil {
			snap.progressDecision = closure.LatestProgressDecision()
		}
	}
	return snap
}

func (o *Orchestrator) ingestExploreNodeArtifactsForWindow(
	window []*types.TaskNode,
	output *agent.StageOutput,
	before exploreNodeArtifactProjectionSnapshot,
	after exploreNodeArtifactProjectionSnapshot,
) {
	if o == nil || o.busCtx == nil || o.busCtx.Mutable == nil {
		return
	}
	producers := exploreNodeArtifactProducerIDs(window)
	if len(producers) == 0 {
		return
	}
	records := exploreNodeArtifactRecordsForOutput(producers, output, before)
	records = append(records, exploreNodeArtifactRecordsForAggregateFacts(producers, before, after)...)
	records = append(records, exploreNodeArtifactRecordsForAnalysisRefinementHandoff(window, after.progressDecision)...)
	if len(records) == 0 {
		return
	}
	o.busCtx.Mutable.EvidenceClosure().IngestEvidenceReducerInput(types.EvidenceReducerInput{
		Class:         types.EvidenceReducerInputNodeArtifactDelta,
		NodeArtifacts: records,
	}, o.busCtx.RepoRoot)
}

func exploreNodeArtifactProducerIDs(window []*types.TaskNode) []string {
	seen := map[string]bool{}
	out := make([]string, 0, len(window))
	for _, node := range window {
		if node == nil {
			continue
		}
		id := strings.TrimSpace(node.ID)
		if id == "" || seen[id] {
			continue
		}
		seen[id] = true
		out = append(out, id)
	}
	return out
}

func exploreNodeArtifactRecordsForOutput(
	producers []string,
	output *agent.StageOutput,
	before exploreNodeArtifactProjectionSnapshot,
) []types.NodeArtifactRecord {
	if output == nil || len(producers) == 0 {
		return nil
	}
	var records []types.NodeArtifactRecord
	for _, item := range output.EvidenceItems {
		id := types.RuntimeArtifactIDForEvidenceItem(item)
		if id == "" || before.evidenceIDs[id] {
			continue
		}
		path := strings.TrimSpace(item.Source)
		for _, producer := range producers {
			records = append(records, types.NodeArtifactRecord{
				ProducerNodeID: producer,
				ProducerStage:  types.StageExplore,
				SourceStage:    types.StageExplore,
				Consumer:       exploreNodeArtifactConsumerExtract,
				EvidenceID:     id,
				ReasonCode:     exploreNodeArtifactReasonEvidenceItem,
				Artifact: types.RuntimeArtifactRef{
					Kind:      types.RuntimeArtifactEvidenceItem,
					ID:        id,
					Path:      path,
					LineStart: item.LineStart,
					LineEnd:   item.LineEnd,
				},
			})
		}
	}
	for _, chain := range output.AnswerChains {
		id := types.RuntimeArtifactIDForAnswerChain(chain)
		if id == "" || before.answerChainIDs[id] {
			continue
		}
		path := strings.TrimSpace(chain.Item.Source)
		evidenceID := types.RuntimeArtifactIDForEvidenceItem(chain.Item)
		for _, producer := range producers {
			records = append(records, types.NodeArtifactRecord{
				ProducerNodeID: producer,
				ProducerStage:  types.StageExplore,
				SourceStage:    types.StageExplore,
				Consumer:       exploreNodeArtifactConsumerExtract,
				EvidenceID:     evidenceID,
				ReasonCode:     exploreNodeArtifactReasonAnswerChain,
				Artifact: types.RuntimeArtifactRef{
					Kind:      types.RuntimeArtifactAnswerChain,
					ID:        id,
					Path:      path,
					LineStart: chain.Item.LineStart,
					LineEnd:   chain.Item.LineEnd,
				},
			})
		}
	}
	return records
}

func exploreNodeArtifactRecordsForAnalysisRefinementHandoff(
	window []*types.TaskNode,
	decision types.ProgressDecision,
) []types.NodeArtifactRecord {
	if len(window) == 0 {
		return nil
	}
	var records []types.NodeArtifactRecord
	for _, node := range window {
		if node == nil || !exploreNodeDeclaresOutput(node, "analysis_refinement_handoff") {
			continue
		}
		producer := strings.TrimSpace(node.ID)
		id := types.RuntimeArtifactIDForAnalysisRefinementHandoff(producer, decision)
		if producer == "" || id == "" {
			continue
		}
		records = append(records, types.NodeArtifactRecord{
			ProducerNodeID: producer,
			ProducerStage:  types.StageExplore,
			SourceStage:    types.StageExplore,
			Consumer:       types.RuntimeArtifactConsumerAnalysisRefinement,
			ReasonCode:     exploreNodeArtifactReasonAnalysisRefinementHandoff,
			Artifact: types.RuntimeArtifactRef{
				Kind:        types.RuntimeArtifactAnalysisRefinementHandoff,
				ID:          id,
				ContentHash: types.RuntimeArtifactHashForProgressDecision(decision),
			},
		})
	}
	return records
}

func exploreNodeDeclaresOutput(node *types.TaskNode, output string) bool {
	if node == nil {
		return false
	}
	output = strings.TrimSpace(output)
	if output == "" {
		return false
	}
	for _, raw := range node.Outputs {
		if strings.TrimSpace(raw) == output {
			return true
		}
	}
	return false
}

func exploreNodeArtifactRecordsForAggregateFacts(
	producers []string,
	before exploreNodeArtifactProjectionSnapshot,
	after exploreNodeArtifactProjectionSnapshot,
) []types.NodeArtifactRecord {
	if len(producers) == 0 || len(after.aggregateFactList) == 0 {
		return nil
	}
	var records []types.NodeArtifactRecord
	for _, fact := range after.aggregateFactList {
		identity := types.RuntimeArtifactIdentityForAggregateFact(fact)
		if identity == "" || before.aggregateFactIDs[identity] {
			continue
		}
		id := types.RuntimeArtifactIDForAggregateFactIdentity(identity)
		for _, producer := range producers {
			records = append(records, types.NodeArtifactRecord{
				ProducerNodeID: producer,
				ProducerStage:  types.StageExplore,
				SourceStage:    types.StageExplore,
				Consumer:       exploreNodeArtifactConsumerExtract,
				EvidenceID:     id,
				ReasonCode:     exploreNodeArtifactReasonAggregateFact,
				Artifact: types.RuntimeArtifactRef{
					Kind:        types.RuntimeArtifactAggregateFact,
					ID:          id,
					ContentHash: types.RuntimeArtifactHashString(identity),
				},
			})
		}
	}
	return records
}
