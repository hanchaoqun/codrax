package types

import (
	"sort"
	"strconv"
	"strings"
)

const (
	TraceCausalRolePrimaryRootCause = "primary_root_cause"
	TraceCausalRoleCausalHop        = "causal_hop"
)

type TraceCausalProjection struct {
	PrimaryRootCause *TraceCausalProjectionNode  `json:"primary_root_cause,omitempty"`
	WakeupPath       []string                    `json:"wakeup_path,omitempty"`
	SupportingHops   []TraceCausalProjectionNode `json:"supporting_hops,omitempty"`
}

func (p TraceCausalProjection) Active() bool {
	return p.PrimaryRootCause != nil || len(p.WakeupPath) > 0 || len(p.SupportingHops) > 0
}

type TraceCausalProjectionNode struct {
	Role               string   `json:"role,omitempty"`
	EvidenceID         string   `json:"evidence_id,omitempty"`
	Subject            string   `json:"subject,omitempty"`
	Predicate          string   `json:"predicate,omitempty"`
	Object             string   `json:"object,omitempty"`
	Value              string   `json:"value,omitempty"`
	Unit               string   `json:"unit,omitempty"`
	Summary            string   `json:"summary,omitempty"`
	SupportRefs        []string `json:"support_refs,omitempty"`
	LineStart          int      `json:"line_start,omitempty"`
	LineEnd            int      `json:"line_end,omitempty"`
	Rank               int      `json:"rank,omitempty"`
	Tier               string   `json:"tier,omitempty"`
	Causality          string   `json:"causality,omitempty"`
	ChainDepth         int      `json:"chain_depth,omitempty"`
	ImpactMS           float64  `json:"impact_ms,omitempty"`
	CumulativeImpactMS float64  `json:"cumulative_impact_ms,omitempty"`
	Confidence         float64  `json:"confidence,omitempty"`
}

func CompileTraceCausalProjection(ledger ObservationLedger) TraceCausalProjection {
	return TraceCausalProjectionFromObservationRecords(ledger.Records)
}

func TraceCausalProjectionFromObservationRecords(records []ObservationRecord) TraceCausalProjection {
	if len(records) == 0 {
		return TraceCausalProjection{}
	}
	var primary []TraceCausalProjectionNode
	var hops []TraceCausalProjectionNode
	var wakeupPath []string
	for _, record := range records {
		if !traceCausalProjectionTraceQueryRecord(record) {
			continue
		}
		switch {
		case traceCausalProjectionIsPrimaryRootCause(record):
			primary = append(primary, traceCausalProjectionNodeFromRecord(TraceCausalRolePrimaryRootCause, record))
		case strings.TrimSpace(record.Predicate) == "wakeup_chain" && len(wakeupPath) == 0:
			wakeupPath = traceCausalProjectionPath(record.Object)
		case traceCausalProjectionIsCausalHop(record):
			hops = append(hops, traceCausalProjectionNodeFromRecord(TraceCausalRoleCausalHop, record))
		}
	}
	pathIndex := traceCausalProjectionPathIndex(wakeupPath)
	sort.SliceStable(primary, func(i, j int) bool {
		return traceCausalProjectionPrimaryLess(primary[i], primary[j], pathIndex)
	})
	primary = traceCausalProjectionDedupeNodes(primary)
	sort.SliceStable(hops, func(i, j int) bool {
		return traceCausalProjectionHopLess(hops[i], hops[j], pathIndex)
	})
	hops = traceCausalProjectionDedupeNodes(hops)
	if len(hops) > 4 {
		hops = hops[:4]
	}
	out := TraceCausalProjection{
		WakeupPath:     wakeupPath,
		SupportingHops: hops,
	}
	if len(primary) > 0 {
		node := primary[0]
		out.PrimaryRootCause = &node
	}
	if !out.Active() {
		return TraceCausalProjection{}
	}
	return out
}

func traceCausalProjectionTraceQueryRecord(record ObservationRecord) bool {
	return record.Origin == AnswerEvidenceOriginRuntimeArtifact &&
		strings.TrimSpace(record.Producer) == "trace_query" &&
		record.GroundingPolicy == ClaimGroundingHard
}

func traceCausalProjectionIsPrimaryRootCause(record ObservationRecord) bool {
	return strings.TrimSpace(record.Predicate) == "root_cause_primary" ||
		strings.HasPrefix(strings.TrimSpace(record.ClaimKey), "root_cause_primary")
}

func traceCausalProjectionIsCausalHop(record ObservationRecord) bool {
	switch strings.TrimSpace(record.Predicate) {
	case "wakeup_causal_impact", "wakeup_causal_aggregate", "critical_blocking":
		return true
	default:
		return strings.HasPrefix(strings.TrimSpace(record.ClaimKey), "root_evidence:")
	}
}

func traceCausalProjectionNodeFromRecord(role string, record ObservationRecord) TraceCausalProjectionNode {
	node := TraceCausalProjectionNode{
		Role:        role,
		EvidenceID:  strings.TrimSpace(record.ID),
		Subject:     strings.TrimSpace(record.Subject),
		Predicate:   strings.TrimSpace(record.Predicate),
		Object:      strings.TrimSpace(record.Object),
		Value:       strings.TrimSpace(record.Value),
		Unit:        strings.TrimSpace(record.Unit),
		Summary:     strings.TrimSpace(record.Summary),
		SupportRefs: cloneStringSlice(record.SupportRefs),
		LineStart:   record.Span.LineStart,
		LineEnd:     record.Span.LineEnd,
		Rank:        traceCausalProjectionRichNoteInt(record.RichNotes, "rank"),
		Tier:        traceCausalProjectionRichNoteValue(record.RichNotes, "tier"),
		Causality:   traceCausalProjectionRichNoteValue(record.RichNotes, "causality"),
		ChainDepth:  traceCausalProjectionRichNoteInt(record.RichNotes, "chain_depth"),
		ImpactMS:    traceCausalProjectionImpact(record),
		Confidence:  record.Confidence,
	}
	node.CumulativeImpactMS = traceCausalProjectionRichNoteFloat(record.RichNotes, "cumulative_impact_ms")
	if node.CumulativeImpactMS <= 0 {
		node.CumulativeImpactMS = node.ImpactMS
	}
	return node
}

func traceCausalProjectionNodeLess(a, b TraceCausalProjectionNode) bool {
	if a.CumulativeImpactMS != b.CumulativeImpactMS {
		return a.CumulativeImpactMS > b.CumulativeImpactMS
	}
	if a.ImpactMS != b.ImpactMS {
		return a.ImpactMS > b.ImpactMS
	}
	if a.Confidence != b.Confidence {
		return a.Confidence > b.Confidence
	}
	if a.Rank > 0 && b.Rank > 0 && a.Rank != b.Rank {
		return a.Rank < b.Rank
	}
	return a.EvidenceID < b.EvidenceID
}

func traceCausalProjectionPrimaryLess(a, b TraceCausalProjectionNode, pathIndex map[string]int) bool {
	aClass := traceCausalProjectionPrimaryClass(a, pathIndex)
	bClass := traceCausalProjectionPrimaryClass(b, pathIndex)
	if aClass != bClass {
		return aClass < bClass
	}
	return traceCausalProjectionNodeLess(a, b)
}

func traceCausalProjectionPrimaryClass(node TraceCausalProjectionNode, pathIndex map[string]int) int {
	onChain := strings.TrimSpace(node.Causality) == "on_wakeup_chain"
	inPath := traceCausalProjectionNodeInPath(node, pathIndex)
	known := traceCausalProjectionKnownSubject(node.Subject)
	switch {
	case onChain && inPath && known:
		return 0
	case inPath && known:
		return 1
	case onChain && known:
		return 2
	case known:
		return 3
	default:
		return 4
	}
}

func traceCausalProjectionHopLess(a, b TraceCausalProjectionNode, pathIndex map[string]int) bool {
	aOnChain := strings.TrimSpace(a.Causality) == "on_wakeup_chain"
	bOnChain := strings.TrimSpace(b.Causality) == "on_wakeup_chain"
	if aOnChain != bOnChain {
		return aOnChain
	}
	aInPath := traceCausalProjectionNodeInPath(a, pathIndex)
	bInPath := traceCausalProjectionNodeInPath(b, pathIndex)
	if aInPath != bInPath {
		return aInPath
	}
	if a.ChainDepth > 0 && b.ChainDepth > 0 && a.ChainDepth != b.ChainDepth {
		return a.ChainDepth < b.ChainDepth
	}
	return traceCausalProjectionNodeLess(a, b)
}

func traceCausalProjectionPathIndex(path []string) map[string]int {
	if len(path) == 0 {
		return nil
	}
	out := make(map[string]int, len(path))
	for i, item := range path {
		item = traceCausalProjectionCanonicalNode(item)
		if item != "" {
			out[item] = i + 1
		}
	}
	return out
}

func traceCausalProjectionNodeInPath(node TraceCausalProjectionNode, pathIndex map[string]int) bool {
	if len(pathIndex) == 0 {
		return false
	}
	_, ok := pathIndex[traceCausalProjectionCanonicalNode(node.Subject)]
	return ok
}

func traceCausalProjectionKnownSubject(subject string) bool {
	subject = traceCausalProjectionCanonicalNode(subject)
	return subject != "" && subject != "unknown-thread" && subject != "unknown"
}

func traceCausalProjectionCanonicalNode(raw string) string {
	return strings.ToLower(strings.TrimSpace(raw))
}

func traceCausalProjectionDedupeNodes(nodes []TraceCausalProjectionNode) []TraceCausalProjectionNode {
	if len(nodes) < 2 {
		return nodes
	}
	seen := make(map[string]bool, len(nodes))
	out := make([]TraceCausalProjectionNode, 0, len(nodes))
	for _, node := range nodes {
		key := strings.Join([]string{
			traceCausalProjectionCanonicalNode(node.Role),
			traceCausalProjectionCanonicalNode(node.Subject),
			traceCausalProjectionCanonicalNode(node.Predicate),
			traceCausalProjectionCanonicalNode(node.Object),
			traceCausalProjectionCanonicalNode(strings.Join(node.SupportRefs, "|")),
		}, "\x00")
		if seen[key] {
			continue
		}
		seen[key] = true
		out = append(out, node)
	}
	return out
}

func traceCausalProjectionPath(raw string) []string {
	parts := strings.Split(raw, "->")
	out := make([]string, 0, len(parts))
	for _, part := range parts {
		part = strings.TrimSpace(part)
		if part != "" {
			out = append(out, part)
		}
	}
	return out
}

func traceCausalProjectionImpact(record ObservationRecord) float64 {
	if value := traceCausalProjectionRichNoteFloat(record.RichNotes, "impact_ms"); value > 0 {
		return value
	}
	if value := traceCausalProjectionRichNoteFloat(record.RichNotes, "impact"); value > 0 {
		return value
	}
	if strings.TrimSpace(record.Unit) == "ms" {
		return traceCausalProjectionFloat(record.Value)
	}
	return 0
}

func traceCausalProjectionRichNoteValue(notes []string, key string) string {
	prefix := strings.TrimSpace(key) + "="
	if prefix == "=" {
		return ""
	}
	for _, note := range notes {
		note = strings.TrimSpace(note)
		if strings.HasPrefix(note, prefix) {
			return strings.TrimSpace(strings.TrimPrefix(note, prefix))
		}
	}
	return ""
}

func traceCausalProjectionRichNoteFloat(notes []string, key string) float64 {
	return traceCausalProjectionFloat(traceCausalProjectionRichNoteValue(notes, key))
}

func traceCausalProjectionRichNoteInt(notes []string, key string) int {
	value := traceCausalProjectionRichNoteValue(notes, key)
	value = strings.TrimSpace(strings.TrimSuffix(strings.ToLower(value), "ms"))
	n, err := strconv.Atoi(value)
	if err != nil {
		return 0
	}
	return n
}

func traceCausalProjectionFloat(raw string) float64 {
	value := strings.TrimSpace(raw)
	value = strings.TrimSuffix(strings.ToLower(value), "ms")
	value = strings.TrimSpace(strings.TrimSuffix(value, "毫秒"))
	value = strings.TrimSpace(value)
	if value == "" {
		return 0
	}
	f, err := strconv.ParseFloat(value, 64)
	if err != nil {
		return 0
	}
	return f
}
