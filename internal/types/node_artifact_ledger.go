package types

import (
	"sort"
	"strconv"
	"strings"
)

// RuntimeArtifactKind is the closed runtime artifact reference family shared by
// read/write execution ledgers. It is intentionally small and typed; hard gates
// must not accept arbitrary prompt/model prose as a new artifact kind.
type RuntimeArtifactKind string

const (
	RuntimeArtifactUnknown                   RuntimeArtifactKind = "unknown"
	RuntimeArtifactEvidenceItem              RuntimeArtifactKind = "evidence_item"
	RuntimeArtifactAnswerChain               RuntimeArtifactKind = "answer_chain"
	RuntimeArtifactAggregateFact             RuntimeArtifactKind = "aggregate_fact"
	RuntimeArtifactSource                    RuntimeArtifactKind = "source"
	RuntimeArtifactToolInvocation            RuntimeArtifactKind = "tool_invocation"
	RuntimeArtifactRepoMap                   RuntimeArtifactKind = "repo_map"
	RuntimeArtifactProof                     RuntimeArtifactKind = "proof"
	RuntimeArtifactAnalysisRefinementHandoff RuntimeArtifactKind = "analysis_refinement_handoff"
)

// RuntimeArtifactDirection is the closed lineage edge direction for one
// artifact record. Produced records say which node created a typed ref;
// consumed records say which downstream node consumed that resolved ref.
type RuntimeArtifactDirection string

const (
	RuntimeArtifactProduced RuntimeArtifactDirection = "produced"
	RuntimeArtifactConsumed RuntimeArtifactDirection = "consumed"
)

// RuntimeArtifactRef identifies an artifact payload that already lives in an
// existing typed carrier. The ledger stores references only; raw payloads,
// rendered answers, prompt text, and model rationale stay out of this surface.
type RuntimeArtifactRef struct {
	Kind        RuntimeArtifactKind `json:"kind,omitempty"`
	ID          string              `json:"id,omitempty"`
	Path        string              `json:"path,omitempty"`
	LineStart   int                 `json:"line_start,omitempty"`
	LineEnd     int                 `json:"line_end,omitempty"`
	ContentHash string              `json:"content_hash,omitempty"`
	Version     int                 `json:"version,omitempty"`
}

// NodeArtifactRecord records one producer/consumer edge for a runtime artifact.
// ProducerNodeID is required for produced records. Consumed records additionally
// require ConsumerNodeID so replay/final audit can prove which node consumed the
// resolved ref without parsing model prose or rendered answers.
type NodeArtifactRecord struct {
	Direction      RuntimeArtifactDirection `json:"direction,omitempty"`
	Artifact       RuntimeArtifactRef       `json:"artifact,omitempty"`
	ProducerNodeID string                   `json:"producer_node_id,omitempty"`
	ConsumerNodeID string                   `json:"consumer_node_id,omitempty"`
	ProducerStage  PipelineStage            `json:"producer_stage,omitempty"`
	SourceStage    PipelineStage            `json:"source_stage,omitempty"`
	Consumer       string                   `json:"consumer,omitempty"`
	EvidenceID     string                   `json:"evidence_id,omitempty"`
	Valid          bool                     `json:"valid,omitempty"`
	ReasonCode     string                   `json:"reason_code,omitempty"`
}

// NodeArtifactLedger is the run-local artifact lineage carrier. It is outside
// AnalysisIR so immutable TaskNode Inputs/Outputs remain declarations, not
// mutable runtime state.
type NodeArtifactLedger struct {
	Records []NodeArtifactRecord `json:"records,omitempty"`
}

func NormalizeRuntimeArtifactKind(kind RuntimeArtifactKind) RuntimeArtifactKind {
	switch RuntimeArtifactKind(strings.ToLower(strings.TrimSpace(string(kind)))) {
	case RuntimeArtifactEvidenceItem:
		return RuntimeArtifactEvidenceItem
	case RuntimeArtifactAnswerChain:
		return RuntimeArtifactAnswerChain
	case RuntimeArtifactAggregateFact:
		return RuntimeArtifactAggregateFact
	case RuntimeArtifactSource:
		return RuntimeArtifactSource
	case RuntimeArtifactToolInvocation:
		return RuntimeArtifactToolInvocation
	case RuntimeArtifactRepoMap:
		return RuntimeArtifactRepoMap
	case RuntimeArtifactProof:
		return RuntimeArtifactProof
	case RuntimeArtifactAnalysisRefinementHandoff:
		return RuntimeArtifactAnalysisRefinementHandoff
	case RuntimeArtifactUnknown:
		return RuntimeArtifactUnknown
	default:
		return RuntimeArtifactUnknown
	}
}

func NormalizeRuntimeArtifactDirection(direction RuntimeArtifactDirection) RuntimeArtifactDirection {
	switch RuntimeArtifactDirection(strings.ToLower(strings.TrimSpace(string(direction)))) {
	case RuntimeArtifactConsumed:
		return RuntimeArtifactConsumed
	case RuntimeArtifactProduced, "":
		return RuntimeArtifactProduced
	default:
		return RuntimeArtifactProduced
	}
}

func NormalizeRuntimeArtifactRef(ref RuntimeArtifactRef) RuntimeArtifactRef {
	ref.Kind = NormalizeRuntimeArtifactKind(ref.Kind)
	ref.ID = strings.TrimSpace(ref.ID)
	ref.Path = strings.TrimSpace(ref.Path)
	ref.ContentHash = strings.TrimSpace(ref.ContentHash)
	if ref.LineStart <= 0 {
		ref.LineStart = 0
		ref.LineEnd = 0
	} else if ref.LineEnd <= 0 || ref.LineEnd < ref.LineStart {
		ref.LineEnd = ref.LineStart
	}
	if ref.Version < 0 {
		ref.Version = 0
	}
	if ref.ID == "" {
		ref.ID = runtimeArtifactIDFromPath(ref.Path, ref.LineStart, ref.LineEnd)
	}
	return ref
}

func NormalizeNodeArtifactRecord(record NodeArtifactRecord) NodeArtifactRecord {
	record.Direction = NormalizeRuntimeArtifactDirection(record.Direction)
	record.Artifact = NormalizeRuntimeArtifactRef(record.Artifact)
	record.ProducerNodeID = strings.TrimSpace(record.ProducerNodeID)
	record.ConsumerNodeID = strings.TrimSpace(record.ConsumerNodeID)
	record.ProducerStage = PipelineStage(strings.TrimSpace(string(record.ProducerStage)))
	record.SourceStage = PipelineStage(strings.TrimSpace(string(record.SourceStage)))
	record.Consumer = strings.TrimSpace(record.Consumer)
	record.EvidenceID = strings.TrimSpace(record.EvidenceID)
	record.ReasonCode = strings.TrimSpace(record.ReasonCode)
	if record.Artifact.ID == "" && record.EvidenceID != "" {
		record.Artifact.ID = record.EvidenceID
	}
	record.Valid = ValidNodeArtifactRecord(record)
	if !record.Valid {
		return NodeArtifactRecord{}
	}
	return record
}

func ValidNodeArtifactRecord(record NodeArtifactRecord) bool {
	record.Direction = NormalizeRuntimeArtifactDirection(record.Direction)
	record.Artifact = NormalizeRuntimeArtifactRef(record.Artifact)
	if strings.TrimSpace(record.Artifact.ID) == "" || strings.TrimSpace(record.ProducerNodeID) == "" {
		return false
	}
	if record.Direction == RuntimeArtifactConsumed && strings.TrimSpace(record.ConsumerNodeID) == "" {
		return false
	}
	return true
}

func NormalizeNodeArtifactRecords(records []NodeArtifactRecord) []NodeArtifactRecord {
	if len(records) == 0 {
		return nil
	}
	seen := make(map[string]bool, len(records))
	out := make([]NodeArtifactRecord, 0, len(records))
	for _, record := range records {
		record = NormalizeNodeArtifactRecord(record)
		if !record.Valid {
			continue
		}
		key := NodeArtifactRecordKey(record)
		if key == "" || seen[key] {
			continue
		}
		seen[key] = true
		out = append(out, record)
	}
	sort.SliceStable(out, func(i, j int) bool {
		return NodeArtifactRecordKey(out[i]) < NodeArtifactRecordKey(out[j])
	})
	if len(out) == 0 {
		return nil
	}
	return out
}

func NormalizeNodeArtifactLedger(ledger NodeArtifactLedger) NodeArtifactLedger {
	return NodeArtifactLedger{Records: NormalizeNodeArtifactRecords(ledger.Records)}
}

func CloneNodeArtifactLedger(ledger NodeArtifactLedger) NodeArtifactLedger {
	return NormalizeNodeArtifactLedger(ledger)
}

func MergeNodeArtifactLedgers(a, b NodeArtifactLedger) NodeArtifactLedger {
	records := append(NormalizeNodeArtifactRecords(a.Records), NormalizeNodeArtifactRecords(b.Records)...)
	return NormalizeNodeArtifactLedger(NodeArtifactLedger{Records: records})
}

func (l NodeArtifactLedger) Empty() bool {
	return len(NormalizeNodeArtifactRecords(l.Records)) == 0
}

func (l NodeArtifactLedger) RecordsByKind(kind RuntimeArtifactKind) []NodeArtifactRecord {
	kind = NormalizeRuntimeArtifactKind(kind)
	var out []NodeArtifactRecord
	for _, record := range NormalizeNodeArtifactRecords(l.Records) {
		if record.Artifact.Kind == kind {
			out = append(out, record)
		}
	}
	return out
}

func (l NodeArtifactLedger) RecordsByProducer(nodeID string) []NodeArtifactRecord {
	nodeID = strings.TrimSpace(nodeID)
	if nodeID == "" {
		return nil
	}
	var out []NodeArtifactRecord
	for _, record := range NormalizeNodeArtifactRecords(l.Records) {
		if record.ProducerNodeID == nodeID {
			out = append(out, record)
		}
	}
	return out
}

func (l NodeArtifactLedger) RecordsByConsumerNode(nodeID string) []NodeArtifactRecord {
	nodeID = strings.TrimSpace(nodeID)
	if nodeID == "" {
		return nil
	}
	var out []NodeArtifactRecord
	for _, record := range NormalizeNodeArtifactRecords(l.Records) {
		if record.ConsumerNodeID == nodeID {
			out = append(out, record)
		}
	}
	return out
}

func NodeArtifactRecordKey(record NodeArtifactRecord) string {
	record = NormalizeNodeArtifactRecord(record)
	if !record.Valid {
		return ""
	}
	parts := []string{
		string(record.Direction),
		string(record.Artifact.Kind),
		record.Artifact.ID,
		record.ProducerNodeID,
		record.ConsumerNodeID,
		string(record.ProducerStage),
		string(record.SourceStage),
		record.Consumer,
		record.Artifact.Path,
		strconv.Itoa(record.Artifact.LineStart),
		strconv.Itoa(record.Artifact.LineEnd),
		record.Artifact.ContentHash,
		strconv.Itoa(record.Artifact.Version),
		record.EvidenceID,
	}
	return strings.Join(parts, "\x00")
}

func runtimeArtifactIDFromPath(path string, lineStart, lineEnd int) string {
	path = strings.TrimSpace(path)
	if path == "" {
		return ""
	}
	if lineStart <= 0 {
		return path
	}
	if lineEnd <= 0 || lineEnd == lineStart {
		return path + ":" + strconv.Itoa(lineStart)
	}
	return path + ":" + strconv.Itoa(lineStart) + "-" + strconv.Itoa(lineEnd)
}
