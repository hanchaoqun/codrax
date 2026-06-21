package orchestrator

import (
	"fmt"
	"hash/fnv"
	"strings"

	"github.com/hanchaoqun/codrax/internal/agent"
	"github.com/hanchaoqun/codrax/internal/types"
)

const (
	exploreNodeArtifactConsumerExtract = "extract"

	exploreNodeArtifactReasonEvidenceItem  = "explore_stage_output_evidence_item"
	exploreNodeArtifactReasonAnswerChain   = "explore_stage_output_answer_chain"
	exploreNodeArtifactReasonAggregateFact = "explore_accepted_aggregate_fact"
)

type exploreNodeArtifactProjectionSnapshot struct {
	evidenceIDs       map[string]bool
	answerChainIDs    map[string]bool
	aggregateFactIDs  map[string]bool
	aggregateFactList []types.AnswerAggregateFact
}

func captureExploreNodeArtifactProjectionSnapshot(bus *types.BusContext, mut *types.MutableState) exploreNodeArtifactProjectionSnapshot {
	snap := exploreNodeArtifactProjectionSnapshot{
		evidenceIDs:      map[string]bool{},
		answerChainIDs:   map[string]bool{},
		aggregateFactIDs: map[string]bool{},
	}
	if bus != nil {
		for _, item := range bus.EvidenceItems {
			if id := exploreEvidenceItemArtifactID(item); id != "" {
				snap.evidenceIDs[id] = true
			}
		}
		for _, chain := range bus.AnswerChains {
			if id := exploreAnswerChainArtifactID(chain); id != "" {
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
			if id := exploreAggregateFactIdentity(fact); id != "" {
				snap.aggregateFactIDs[id] = true
			}
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
		id := exploreEvidenceItemArtifactID(item)
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
		id := exploreAnswerChainArtifactID(chain)
		if id == "" || before.answerChainIDs[id] {
			continue
		}
		path := strings.TrimSpace(chain.Item.Source)
		evidenceID := exploreEvidenceItemArtifactID(chain.Item)
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
		identity := exploreAggregateFactIdentity(fact)
		if identity == "" || before.aggregateFactIDs[identity] {
			continue
		}
		id := exploreAggregateFactArtifactID(identity)
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
					ContentHash: exploreHashString(identity),
				},
			})
		}
	}
	return records
}

func exploreEvidenceItemArtifactID(item types.EvidenceItem) string {
	id := strings.TrimSpace(item.ID)
	if id == "" {
		id = types.StableEvidenceID(item)
	}
	return strings.TrimSpace(id)
}

func exploreAnswerChainArtifactID(chain types.AnswerChain) string {
	id := exploreEvidenceItemArtifactID(chain.Item)
	if id == "" {
		return ""
	}
	return "answer_chain:" + id
}

func exploreAggregateFactIdentity(fact types.AnswerAggregateFact) string {
	return strings.TrimSpace(types.AnswerAggregateFactIdentity(fact))
}

func exploreAggregateFactArtifactID(identity string) string {
	identity = strings.TrimSpace(identity)
	if identity == "" {
		return ""
	}
	return "aggregate_fact:" + exploreHashString(identity)
}

func exploreHashString(value string) string {
	h := fnv.New64a()
	_, _ = h.Write([]byte(value))
	return fmt.Sprintf("%016x", h.Sum64())
}
