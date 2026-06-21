package types

import (
	"fmt"
	"sort"
	"strings"
)

const (
	RuntimeArtifactConsumerExtract = "extract"

	RuntimeArtifactReasonReadinessConsumed = "artifact_readiness_resolved_ref_consumed"

	taskArtifactInputEvidenceItems  = "evidence_items"
	taskArtifactInputAnswerChains   = "answer_chains"
	taskArtifactInputAggregateFacts = "aggregate_facts"
)

// ArtifactReadinessReasonCode is a closed structural reason family for
// artifact readiness. It is consumed by hard gates; do not route on prompt text,
// model rationale, or rendered final-answer prose.
type ArtifactReadinessReasonCode string

const (
	ArtifactReadinessReady                ArtifactReadinessReasonCode = "ready"
	ArtifactReadinessNoTypedPayload       ArtifactReadinessReasonCode = "no_typed_payload"
	ArtifactReadinessPayloadMissing       ArtifactReadinessReasonCode = "artifact_payload_missing"
	ArtifactReadinessKindUnsupported      ArtifactReadinessReasonCode = "artifact_kind_unsupported"
	ArtifactReadinessRequiredInputMissing ArtifactReadinessReasonCode = "contract_required_input_missing"
)

// ArtifactReadinessInput is the immutable input used to derive an artifact
// readiness view. It references existing typed payload carriers only; raw tool
// output, prompt text, model reasoning, and rendered answers must not enter.
type ArtifactReadinessInput struct {
	Contracts      []TaskArtifactContract
	Ledger         NodeArtifactLedger
	EvidenceItems  []EvidenceItem
	AnswerChains   []AnswerChain
	AggregateFacts []AnswerAggregateFact
	Consumer       string
}

// ArtifactReadinessView is a derived read-only projection used by criterion
// gates and final-audit surfaces. The view may explain why lineage is
// unresolved, but the payload authority remains the existing typed carriers.
type ArtifactReadinessView struct {
	Active                bool                        `json:"active"`
	Ready                 bool                        `json:"ready"`
	ReasonCode            ArtifactReadinessReasonCode `json:"reason_code,omitempty"`
	Consumer              string                      `json:"consumer,omitempty"`
	PayloadCount          int                         `json:"payload_count,omitempty"`
	EvidenceItemCount     int                         `json:"evidence_item_count,omitempty"`
	AnswerChainCount      int                         `json:"answer_chain_count,omitempty"`
	AggregateFactCount    int                         `json:"aggregate_fact_count,omitempty"`
	ResolvedRefCount      int                         `json:"resolved_ref_count,omitempty"`
	UnresolvedRefCount    int                         `json:"unresolved_ref_count,omitempty"`
	UnsupportedRefCount   int                         `json:"unsupported_ref_count,omitempty"`
	MissingRequiredInputs []string                    `json:"missing_required_inputs,omitempty"`
	ResolvedRefs          []NodeArtifactRecord        `json:"resolved_refs,omitempty"`
	UnresolvedRefs        []NodeArtifactRecord        `json:"unresolved_refs,omitempty"`
	UnsupportedRefs       []NodeArtifactRecord        `json:"unsupported_refs,omitempty"`
}

// HasBlockingLedgerIssue reports whether an active ledger contains precise
// structural problems that must block readiness before fallback evidence floors.
func (v ArtifactReadinessView) HasBlockingLedgerIssue() bool {
	return v.UnresolvedRefCount > 0 || v.UnsupportedRefCount > 0
}

func (v ArtifactReadinessView) Detail() string {
	reason := strings.TrimSpace(string(v.ReasonCode))
	if reason == "" {
		reason = string(ArtifactReadinessNoTypedPayload)
	}
	return fmt.Sprintf("artifact_readiness: reason=%s payloads=%d evidence_items=%d answer_chains=%d aggregate_facts=%d resolved_refs=%d unresolved_refs=%d unsupported_refs=%d",
		reason,
		v.PayloadCount,
		v.EvidenceItemCount,
		v.AnswerChainCount,
		v.AggregateFactCount,
		v.ResolvedRefCount,
		v.UnresolvedRefCount,
		v.UnsupportedRefCount,
	)
}

type runtimeArtifactPayloadIndex struct {
	evidenceItemIDs  map[string]struct{}
	answerChainIDs   map[string]struct{}
	aggregateFactIDs map[string]struct{}
}

// BuildArtifactReadinessView derives readiness from artifact declarations,
// node-scoped artifact lineage, and current typed payload carriers.
func BuildArtifactReadinessView(input ArtifactReadinessInput) ArtifactReadinessView {
	consumer := strings.TrimSpace(input.Consumer)
	index := buildRuntimeArtifactPayloadIndex(input.EvidenceItems, input.AnswerChains, input.AggregateFacts)
	view := ArtifactReadinessView{
		Active:             len(input.Contracts) > 0,
		Consumer:           consumer,
		EvidenceItemCount:  len(index.evidenceItemIDs),
		AnswerChainCount:   len(index.answerChainIDs),
		AggregateFactCount: len(index.aggregateFactIDs),
	}
	view.PayloadCount = view.EvidenceItemCount + view.AnswerChainCount + view.AggregateFactCount
	view.MissingRequiredInputs = artifactReadinessMissingInputs(input.Contracts, index)

	records := artifactReadinessRelevantRecords(input.Ledger, consumer)
	if len(records) > 0 {
		view.Active = true
	}
	for _, record := range records {
		kind := NormalizeRuntimeArtifactKind(record.Artifact.Kind)
		if !artifactReadinessSupportedKind(kind) {
			view.UnsupportedRefs = append(view.UnsupportedRefs, record)
			continue
		}
		if !index.has(kind, record.Artifact.ID) {
			view.UnresolvedRefs = append(view.UnresolvedRefs, record)
			continue
		}
		view.ResolvedRefs = append(view.ResolvedRefs, record)
	}
	view.ResolvedRefCount = len(view.ResolvedRefs)
	view.UnresolvedRefCount = len(view.UnresolvedRefs)
	view.UnsupportedRefCount = len(view.UnsupportedRefs)

	switch {
	case view.UnsupportedRefCount > 0:
		view.Ready = false
		view.ReasonCode = ArtifactReadinessKindUnsupported
	case view.UnresolvedRefCount > 0:
		view.Ready = false
		view.ReasonCode = ArtifactReadinessPayloadMissing
	case view.PayloadCount > 0:
		view.Ready = true
		view.ReasonCode = ArtifactReadinessReady
	case len(view.MissingRequiredInputs) > 0:
		view.Ready = false
		view.ReasonCode = ArtifactReadinessRequiredInputMissing
	default:
		view.Ready = false
		view.ReasonCode = ArtifactReadinessNoTypedPayload
	}
	return view
}

func buildRuntimeArtifactPayloadIndex(
	evidence []EvidenceItem,
	chains []AnswerChain,
	facts []AnswerAggregateFact,
) runtimeArtifactPayloadIndex {
	idx := runtimeArtifactPayloadIndex{
		evidenceItemIDs:  map[string]struct{}{},
		answerChainIDs:   map[string]struct{}{},
		aggregateFactIDs: map[string]struct{}{},
	}
	for _, item := range evidence {
		addRuntimeArtifactID(idx.evidenceItemIDs, RuntimeArtifactIDForEvidenceItem(item))
		addRuntimeArtifactID(idx.evidenceItemIDs, strings.TrimSpace(item.ID))
	}
	for _, chain := range chains {
		addRuntimeArtifactID(idx.answerChainIDs, RuntimeArtifactIDForAnswerChain(chain))
		addRuntimeArtifactID(idx.evidenceItemIDs, RuntimeArtifactIDForEvidenceItem(chain.Item))
	}
	for _, fact := range facts {
		addRuntimeArtifactID(idx.aggregateFactIDs, RuntimeArtifactIDForAggregateFact(fact))
	}
	return idx
}

func addRuntimeArtifactID(set map[string]struct{}, id string) {
	id = strings.TrimSpace(id)
	if id == "" {
		return
	}
	set[id] = struct{}{}
}

func (idx runtimeArtifactPayloadIndex) has(kind RuntimeArtifactKind, id string) bool {
	id = strings.TrimSpace(id)
	if id == "" {
		return false
	}
	switch NormalizeRuntimeArtifactKind(kind) {
	case RuntimeArtifactEvidenceItem:
		_, ok := idx.evidenceItemIDs[id]
		return ok
	case RuntimeArtifactAnswerChain:
		_, ok := idx.answerChainIDs[id]
		return ok
	case RuntimeArtifactAggregateFact:
		_, ok := idx.aggregateFactIDs[id]
		return ok
	default:
		return false
	}
}

func artifactReadinessRelevantRecords(ledger NodeArtifactLedger, consumer string) []NodeArtifactRecord {
	records := NormalizeNodeArtifactRecords(ledger.Records)
	if len(records) == 0 {
		return nil
	}
	var out []NodeArtifactRecord
	for _, record := range records {
		if record.Direction == RuntimeArtifactConsumed {
			continue
		}
		recordConsumer := strings.TrimSpace(record.Consumer)
		if consumer != "" && recordConsumer != "" && recordConsumer != consumer {
			continue
		}
		out = append(out, record)
	}
	return out
}

func ConsumedNodeArtifactRecordsFromReadiness(consumerNodeID string, view ArtifactReadinessView) []NodeArtifactRecord {
	consumerNodeID = strings.TrimSpace(consumerNodeID)
	if consumerNodeID == "" || !view.Ready || len(view.ResolvedRefs) == 0 {
		return nil
	}
	records := make([]NodeArtifactRecord, 0, len(view.ResolvedRefs))
	for _, record := range view.ResolvedRefs {
		record = NormalizeNodeArtifactRecord(record)
		if !record.Valid || record.Direction == RuntimeArtifactConsumed {
			continue
		}
		record.Direction = RuntimeArtifactConsumed
		record.ConsumerNodeID = consumerNodeID
		if strings.TrimSpace(record.Consumer) == "" {
			record.Consumer = strings.TrimSpace(view.Consumer)
		}
		record.ReasonCode = RuntimeArtifactReasonReadinessConsumed
		records = append(records, record)
	}
	return NormalizeNodeArtifactRecords(records)
}

func artifactReadinessSupportedKind(kind RuntimeArtifactKind) bool {
	switch NormalizeRuntimeArtifactKind(kind) {
	case RuntimeArtifactEvidenceItem, RuntimeArtifactAnswerChain, RuntimeArtifactAggregateFact:
		return true
	default:
		return false
	}
}

func artifactReadinessMissingInputs(contracts []TaskArtifactContract, index runtimeArtifactPayloadIndex) []string {
	required := map[string]bool{}
	for _, contract := range contracts {
		for _, input := range contract.Inputs {
			input = strings.TrimSpace(input)
			switch input {
			case taskArtifactInputEvidenceItems, taskArtifactInputAnswerChains, taskArtifactInputAggregateFacts:
				required[input] = true
			}
		}
	}
	if len(required) == 0 || len(index.evidenceItemIDs)+len(index.answerChainIDs)+len(index.aggregateFactIDs) > 0 {
		return nil
	}
	out := make([]string, 0, len(required))
	for input := range required {
		out = append(out, input)
	}
	sort.Strings(out)
	return out
}
