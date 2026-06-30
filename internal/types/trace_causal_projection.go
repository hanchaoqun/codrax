package types

import (
	"sort"
	"strconv"
	"strings"
)

const (
	TraceCausalRolePrimaryRootCause = "primary_root_cause"
	TraceCausalRoleCausalHop        = "causal_hop"
	TraceCausalRoleRootCauseContext = "root_cause_context"
)

const (
	traceCausalProjectionPrimaryLimit       = 4
	traceCausalProjectionOnChainLimit       = 10
	traceCausalProjectionContextBucketLimit = 4
	traceCausalProjectionSupportingHopLimit = 10
)

type TraceCausalProjection struct {
	PrimaryRootCause  *TraceCausalProjectionNode  `json:"primary_root_cause,omitempty"`
	PrimaryRootCauses []TraceCausalProjectionNode `json:"primary_root_causes,omitempty"`
	OnChainCauses     []TraceCausalProjectionNode `json:"on_chain_causes,omitempty"`
	AdjacentCauses    []TraceCausalProjectionNode `json:"adjacent_causes,omitempty"`
	BackgroundCauses  []TraceCausalProjectionNode `json:"background_causes,omitempty"`
	WakeupPath        []string                    `json:"wakeup_path,omitempty"`
	SupportingHops    []TraceCausalProjectionNode `json:"supporting_hops,omitempty"`
}

func (p TraceCausalProjection) Active() bool {
	return p.PrimaryRootCause != nil ||
		len(p.PrimaryRootCauses) > 0 ||
		len(p.OnChainCauses) > 0 ||
		len(p.AdjacentCauses) > 0 ||
		len(p.BackgroundCauses) > 0 ||
		len(p.WakeupPath) > 0 ||
		len(p.SupportingHops) > 0
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
	ChainRelevance     string   `json:"chain_relevance,omitempty"`
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
	var classified []TraceCausalProjectionNode
	var hops []TraceCausalProjectionNode
	var wakeupPath []string
	for _, record := range records {
		if !traceCausalProjectionTraceQueryRecord(record) {
			continue
		}
		switch {
		case traceCausalProjectionIsPrimaryRootCause(record):
			node := traceCausalProjectionNodeFromRecord(TraceCausalRolePrimaryRootCause, record)
			primary = append(primary, node)
			classified = append(classified, node)
		case traceCausalProjectionIsRootCauseContext(record):
			classified = append(classified, traceCausalProjectionNodeFromRecord(TraceCausalRoleRootCauseContext, record))
		case strings.TrimSpace(record.Predicate) == "wakeup_chain" && len(wakeupPath) == 0:
			wakeupPath = traceCausalProjectionPath(record.Object)
		case traceCausalProjectionIsCausalHop(record):
			node := traceCausalProjectionNodeFromRecord(TraceCausalRoleCausalHop, record)
			hops = append(hops, node)
			classified = append(classified, node)
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
	if len(hops) > traceCausalProjectionSupportingHopLimit {
		hops = hops[:traceCausalProjectionSupportingHopLimit]
	}
	sort.SliceStable(classified, func(i, j int) bool {
		return traceCausalProjectionClassifiedLess(classified[i], classified[j], pathIndex)
	})
	classified = traceCausalProjectionDedupeNodes(classified)
	out := TraceCausalProjection{
		PrimaryRootCauses: traceCausalProjectionLimitNodes(primary, traceCausalProjectionPrimaryLimit),
		OnChainCauses:     traceCausalProjectionLimitNodes(traceCausalProjectionSelectChainRelevance(classified, "on_chain"), traceCausalProjectionOnChainLimit),
		AdjacentCauses:    traceCausalProjectionLimitNodes(traceCausalProjectionSelectChainRelevance(classified, "adjacent"), traceCausalProjectionContextBucketLimit),
		BackgroundCauses:  traceCausalProjectionLimitNodes(traceCausalProjectionSelectChainRelevance(classified, "background"), traceCausalProjectionContextBucketLimit),
		WakeupPath:        wakeupPath,
		SupportingHops:    hops,
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
		runtimeObservationProducerIsDeterministicQuery(record.Producer) &&
		record.GroundingPolicy == ClaimGroundingHard
}

func traceCausalProjectionIsPrimaryRootCause(record ObservationRecord) bool {
	return strings.TrimSpace(record.Predicate) == "root_cause_primary" ||
		strings.HasPrefix(strings.TrimSpace(record.ClaimKey), "root_cause_primary")
}

func traceCausalProjectionIsRootCauseContext(record ObservationRecord) bool {
	predicate := strings.TrimSpace(record.Predicate)
	claimKey := strings.TrimSpace(record.ClaimKey)
	if predicate == "" && claimKey == "" {
		return false
	}
	return strings.HasPrefix(predicate, "root_cause_") ||
		strings.HasPrefix(claimKey, "root_cause_")
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
		Role:           role,
		EvidenceID:     strings.TrimSpace(record.ID),
		Subject:        strings.TrimSpace(record.Subject),
		Predicate:      strings.TrimSpace(record.Predicate),
		Object:         strings.TrimSpace(record.Object),
		Value:          strings.TrimSpace(record.Value),
		Unit:           strings.TrimSpace(record.Unit),
		Summary:        strings.TrimSpace(record.Summary),
		SupportRefs:    cloneStringSlice(record.SupportRefs),
		LineStart:      record.Span.LineStart,
		LineEnd:        record.Span.LineEnd,
		Rank:           traceCausalProjectionRichNoteInt(record.RichNotes, "rank"),
		Tier:           traceCausalProjectionRichNoteValue(record.RichNotes, "tier"),
		Causality:      traceCausalProjectionRichNoteValue(record.RichNotes, "causality"),
		ChainRelevance: traceCausalProjectionChainRelevance(record.RichNotes),
		ChainDepth:     traceCausalProjectionRichNoteFirstInt(record.RichNotes, "chain_depth", "depth"),
		ImpactMS:       traceCausalProjectionImpact(record),
		Confidence:     record.Confidence,
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
	onChain := traceCausalProjectionNodeOnChain(node)
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
	aOnChain := traceCausalProjectionNodeOnChain(a)
	bOnChain := traceCausalProjectionNodeOnChain(b)
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

func traceCausalProjectionClassifiedLess(a, b TraceCausalProjectionNode, pathIndex map[string]int) bool {
	aRelevance := traceCausalProjectionChainRelevanceRank(a.ChainRelevance)
	bRelevance := traceCausalProjectionChainRelevanceRank(b.ChainRelevance)
	if aRelevance != bRelevance {
		return aRelevance < bRelevance
	}
	if a.Role != b.Role {
		return traceCausalProjectionRoleRank(a.Role) < traceCausalProjectionRoleRank(b.Role)
	}
	return traceCausalProjectionHopLess(a, b, pathIndex)
}

func traceCausalProjectionRoleRank(role string) int {
	switch strings.TrimSpace(role) {
	case TraceCausalRolePrimaryRootCause:
		return 0
	case TraceCausalRoleCausalHop:
		return 1
	default:
		return 2
	}
}

func traceCausalProjectionNodeOnChain(node TraceCausalProjectionNode) bool {
	return strings.TrimSpace(node.ChainRelevance) == "on_chain" ||
		strings.TrimSpace(node.Causality) == "on_wakeup_chain" ||
		strings.TrimSpace(node.Causality) == "on_dependency_chain"
}

func traceCausalProjectionChainRelevanceRank(relevance string) int {
	switch strings.TrimSpace(relevance) {
	case "on_chain":
		return 0
	case "adjacent":
		return 1
	case "background":
		return 2
	default:
		return 3
	}
}

func traceCausalProjectionLimitNodes(nodes []TraceCausalProjectionNode, limit int) []TraceCausalProjectionNode {
	if limit <= 0 || len(nodes) == 0 {
		return nil
	}
	if len(nodes) > limit {
		nodes = nodes[:limit]
	}
	return append([]TraceCausalProjectionNode(nil), nodes...)
}

func traceCausalProjectionSelectChainRelevance(nodes []TraceCausalProjectionNode, relevance string) []TraceCausalProjectionNode {
	relevance = strings.TrimSpace(relevance)
	if relevance == "" {
		return nil
	}
	var out []TraceCausalProjectionNode
	for _, node := range nodes {
		if strings.TrimSpace(node.ChainRelevance) == relevance {
			out = append(out, node)
		}
	}
	return traceCausalProjectionDedupeNodes(out)
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

func traceCausalProjectionChainRelevance(notes []string) string {
	relevance := traceCausalProjectionRichNoteValue(notes, "chain_relevance")
	switch strings.TrimSpace(relevance) {
	case "on_chain", "adjacent", "background":
		return strings.TrimSpace(relevance)
	}
	switch strings.TrimSpace(traceCausalProjectionRichNoteValue(notes, "causality")) {
	case "on_wakeup_chain", "on_dependency_chain":
		return "on_chain"
	case "adjacent_to_wakeup_chain", "adjacent_to_dependency_chain":
		return "adjacent"
	case "background", "off_chain":
		return "background"
	default:
		return ""
	}
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

func traceCausalProjectionRichNoteFirstInt(notes []string, keys ...string) int {
	for _, key := range keys {
		if value := traceCausalProjectionRichNoteInt(notes, key); value > 0 {
			return value
		}
	}
	return 0
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
