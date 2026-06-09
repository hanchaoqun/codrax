package types

import (
	"encoding/json"
	"fmt"
	"sort"
	"strconv"
	"strings"
)

// ObservationSourceKind names the physical namespace of an observation's source
// reference. It is intentionally finer than AnswerEvidenceOrigin: an MCP
// resource and a connector response may both be external-resource evidence, but
// their address shapes and safety rules differ.
type ObservationSourceKind string

const (
	ObservationSourceUnknown          ObservationSourceKind = ""
	ObservationSourceCurrentSource    ObservationSourceKind = "current_source"
	ObservationSourceVCSMetadata      ObservationSourceKind = "vcs_metadata"
	ObservationSourceVCSDiff          ObservationSourceKind = "vcs_diff"
	ObservationSourceRuntimeArtifact  ObservationSourceKind = "runtime_artifact"
	ObservationSourceCommand          ObservationSourceKind = "command"
	ObservationSourceCrossRepoIndex   ObservationSourceKind = "cross_repo_index"
	ObservationSourceExternalDocument ObservationSourceKind = "external_document"
	ObservationSourceWebPage          ObservationSourceKind = "web_page"
	ObservationSourceMCPResource      ObservationSourceKind = "mcp_resource"
	ObservationSourceConnector        ObservationSourceKind = "connector_resource"
)

// ObservationSourceRef is the origin-specific address of the thing that was
// observed. Only the fields that apply to SourceKind should be populated.
type ObservationSourceRef struct {
	Kind         ObservationSourceKind `json:"kind,omitempty"`
	Repo         string                `json:"repo,omitempty"`
	Path         string                `json:"path,omitempty"`
	Commit       string                `json:"commit,omitempty"`
	Range        string                `json:"range,omitempty"`
	Pathspec     string                `json:"pathspec,omitempty"`
	Command      string                `json:"command,omitempty"`
	ToolCallID   string                `json:"tool_call_id,omitempty"`
	RawRef       string                `json:"raw_ref,omitempty"`
	PayloadRef   string                `json:"payload_ref,omitempty"`
	RowSetRef    string                `json:"row_set_ref,omitempty"`
	PageRef      string                `json:"page_ref,omitempty"`
	ArtifactID   string                `json:"artifact_id,omitempty"`
	ArtifactKind string                `json:"artifact_kind,omitempty"`
	URL          string                `json:"url,omitempty"`
	FetchedAt    string                `json:"fetched_at,omitempty"`
	Server       string                `json:"server,omitempty"`
	ResourceURI  string                `json:"resource_uri,omitempty"`
	MIMEType     string                `json:"mime_type,omitempty"`
	Connector    string                `json:"connector,omitempty"`
}

// ObservationSpan locates the observation inside SourceRef when that source has
// an addressable interior. It supports current source lines, artifact-local
// lines, VCS hunk coordinates, web/resource selectors, JSON pointers, table
// rows, and trace time ranges without overloading repo citations.
type ObservationSpan struct {
	LineStart   int     `json:"line_start,omitempty"`
	LineEnd     int     `json:"line_end,omitempty"`
	OldLine     int     `json:"old_line,omitempty"`
	NewLine     int     `json:"new_line,omitempty"`
	HunkHeader  string  `json:"hunk_header,omitempty"`
	Paragraph   int     `json:"paragraph,omitempty"`
	Selector    string  `json:"selector,omitempty"`
	JSONPointer string  `json:"json_pointer,omitempty"`
	Row         int     `json:"row,omitempty"`
	TextStart   int     `json:"text_start,omitempty"`
	TextEnd     int     `json:"text_end,omitempty"`
	StartTs     float64 `json:"start_ts,omitempty"`
	EndTs       float64 `json:"end_ts,omitempty"`
	StartTsMs   float64 `json:"start_ts_ms,omitempty"`
	EndTsMs     float64 `json:"end_ts_ms,omitempty"`
}

// ObservationProvenanceLane names how a non-current-source observation should
// be interpreted. It is intentionally a small typed enum: callers may use it to
// separate direct artifact facts from artifact-local coordinates and bounded
// upstream possibilities without inspecting user prose or model free text.
type ObservationProvenanceLane string

const (
	ObservationProvenanceUnknown                     ObservationProvenanceLane = ""
	ObservationProvenanceObservedDirectCause         ObservationProvenanceLane = "observed_direct_cause"
	ObservationProvenanceArtifactSpan                ObservationProvenanceLane = "artifact_span"
	ObservationProvenanceInferredUpstreamPossibility ObservationProvenanceLane = "inferred_upstream_possibility"
)

func (l ObservationProvenanceLane) IsValid() bool {
	switch l {
	case ObservationProvenanceObservedDirectCause,
		ObservationProvenanceArtifactSpan,
		ObservationProvenanceInferredUpstreamPossibility:
		return true
	default:
		return false
	}
}

func NormalizeObservationProvenanceLane(raw string) ObservationProvenanceLane {
	switch strings.ToLower(strings.TrimSpace(raw)) {
	case string(ObservationProvenanceObservedDirectCause), "direct_cause", "direct_runtime_cause", "observed_failure":
		return ObservationProvenanceObservedDirectCause
	case string(ObservationProvenanceArtifactSpan), "artifact_coordinate", "artifact_line", "artifact_measurement":
		return ObservationProvenanceArtifactSpan
	case string(ObservationProvenanceInferredUpstreamPossibility), "upstream_possibility", "possible_upstream_cause", "bounded_inference":
		return ObservationProvenanceInferredUpstreamPossibility
	default:
		return ObservationProvenanceUnknown
	}
}

// ObservationRecord is the normalized, read-only fact surface compiled from
// accepted producer outputs. It is not a replacement for EvidenceItem; source
// evidence remains the only lane that may become repo citations. The ledger
// gives non-code facts an equally typed route to finalizer/reviewer consumers.
type ObservationRecord struct {
	ID              string                    `json:"id"`
	Origin          AnswerEvidenceOrigin      `json:"origin"`
	Producer        string                    `json:"producer,omitempty"`
	Role            AnswerAggregateRole       `json:"role,omitempty"`
	GroundingPolicy ClaimGroundingPolicy      `json:"grounding_policy,omitempty"`
	ProvenanceLane  ObservationProvenanceLane `json:"provenance_lane,omitempty"`
	SourceRef       ObservationSourceRef      `json:"source_ref,omitempty"`
	Span            ObservationSpan           `json:"span,omitempty"`
	EvidenceKind    EvidenceKind              `json:"evidence_kind,omitempty"`
	AnchorKind      AnchorKind                `json:"anchor_kind,omitempty"`
	EvidenceScope   EvidenceScope             `json:"evidence_scope,omitempty"`
	GroundingStatus GroundingStatus           `json:"grounding_status,omitempty"`
	ClaimKey        string                    `json:"claim_key,omitempty"`
	Subject         string                    `json:"subject,omitempty"`
	Predicate       string                    `json:"predicate,omitempty"`
	Object          string                    `json:"object,omitempty"`
	Value           string                    `json:"value,omitempty"`
	Unit            string                    `json:"unit,omitempty"`
	Negative        bool                      `json:"negative,omitempty"`
	ResultCount     *int                      `json:"result_count,omitempty"`
	Summary         string                    `json:"summary,omitempty"`
	RawExcerpt      string                    `json:"raw_excerpt,omitempty"`
	RichNotes       []string                  `json:"rich_notes,omitempty"`
	SupportRefs     []string                  `json:"support_refs,omitempty"`
	ObservedAt      string                    `json:"observed_at,omitempty"`
	Scope           string                    `json:"scope,omitempty"`
	Confidence      float64                   `json:"confidence,omitempty"`
}

// ObservationLedger is the deterministic, compiled fact ledger for a run. Later
// batches will populate it from the existing carriers and make finalizer/reviewer
// consume it first. Keeping the type in internal/types lets all producers depend
// on the same contract without importing tool or agent packages.
type ObservationLedger struct {
	Records []ObservationRecord `json:"records,omitempty"`
}

func (l ObservationLedger) Empty() bool {
	return len(l.Records) == 0
}

// PrioritizeObservationRecords returns a stable, budgeted view of ledger records
// for prompt/reviewer consumers. It uses the typed request contract to avoid two
// bad extremes: source-code evidence should not be crowded out in mixed
// "external fact + current code" questions, and incidental source reads should
// not outrank VCS/log/trace observations in external-only questions.
func PrioritizeObservationRecords(records []ObservationRecord, rm *RequestModel, contract *AnswerContract, limit int) []ObservationRecord {
	if limit <= 0 || len(records) == 0 {
		return nil
	}
	out := append([]ObservationRecord(nil), records...)
	var intent *AnswerIntentContract
	if rm != nil {
		compiled := CompileAnswerIntentContract(*rm, contract)
		intent = &compiled
	}
	sort.SliceStable(out, func(i, j int) bool {
		return observationRecordRank(out[i], intent) < observationRecordRank(out[j], intent)
	})
	out = budgetSourceInventoryObservationRecords(out, limit)
	if len(out) > limit {
		out = budgetObservationRecordsByOrigin(out, intent, limit)
	}
	return out
}

func budgetSourceInventoryObservationRecords(sorted []ObservationRecord, limit int) []ObservationRecord {
	if limit <= 0 || len(sorted) <= limit {
		return sorted
	}
	sourceInventoryTotal := 0
	nonSourceInventoryTotal := 0
	for _, record := range sorted {
		if observationRecordIsSourceInventory(record) {
			sourceInventoryTotal++
		} else {
			nonSourceInventoryTotal++
		}
	}
	if sourceInventoryTotal == 0 || nonSourceInventoryTotal == 0 {
		return sorted
	}
	budget := sourceInventoryObservationPromptBudget(limit)
	if sourceInventoryTotal <= budget {
		return sorted
	}
	out := make([]ObservationRecord, 0, len(sorted)-sourceInventoryTotal+budget)
	keptSourceInventory := 0
	for _, record := range sorted {
		if !observationRecordIsSourceInventory(record) {
			out = append(out, record)
			continue
		}
		if keptSourceInventory >= budget {
			continue
		}
		keptSourceInventory++
		out = append(out, record)
	}
	return out
}

func sourceInventoryObservationPromptBudget(limit int) int {
	switch {
	case limit <= 4:
		return 1
	case limit <= 8:
		return 2
	case limit <= 14:
		return 4
	default:
		return 6
	}
}

func budgetObservationRecordsByOrigin(sorted []ObservationRecord, intent *AnswerIntentContract, limit int) []ObservationRecord {
	if limit <= 0 || len(sorted) <= limit {
		return sorted
	}
	selected := make([]bool, len(sorted))
	out := make([]ObservationRecord, 0, limit)
	if intent != nil {
		for _, origin := range intent.Origins {
			if len(out) >= limit {
				break
			}
			idx := firstObservationRecordIndexForOrigin(sorted, selected, origin)
			if idx < 0 {
				continue
			}
			selected[idx] = true
			out = append(out, sorted[idx])
		}
	}
	for i, record := range sorted {
		if len(out) >= limit {
			break
		}
		if selected[i] {
			continue
		}
		selected[i] = true
		out = append(out, record)
	}
	return out
}

func firstObservationRecordIndexForOrigin(records []ObservationRecord, selected []bool, origin AnswerEvidenceOrigin) int {
	if origin == AnswerEvidenceOriginUnknown {
		return -1
	}
	for i, record := range records {
		if selected[i] || record.Origin != origin {
			continue
		}
		return i
	}
	return -1
}

// ObservationLedgerInput carries existing accepted producer outputs into the
// ledger compiler. The compiler is intentionally side-effect free and must not
// inspect raw user prose or model free text to classify facts.
type ObservationLedgerInput struct {
	EvidenceItems              []EvidenceItem
	AggregateFacts             []AnswerAggregateFact
	SourceInventoryObservation SourceInventoryObservation
	ToolResults                []ToolResult
	LogBundle                  *LogBundle
	PerfBundle                 *PerfBundle
	MCPResponses               []MCPResponse
	RequestModel               *RequestModel
	AnswerContract             *AnswerContract
	RowSetWriter               ObservationRowSetWriter
}

// ObservationRowSetWriter stores a large, already-structured row set and
// returns a page-able reference. It is optional: nil keeps ledger compilation
// pure and side-effect-free. Callers with a WorkDir can wire this to the blob
// subsystem without making the types package depend on tool storage.
type ObservationRowSetWriter func(name, content string) string

// CompileObservationLedger projects accepted producer outputs into one read-only
// ledger. It deliberately consumes structured fields and system-authored tool
// origin banners only; raw user prose and model free text must not classify
// records.
func CompileObservationLedger(input ObservationLedgerInput) ObservationLedger {
	var out []ObservationRecord
	add := func(record ObservationRecord) {
		if record.Origin == AnswerEvidenceOriginUnknown || !record.Origin.IsValid() {
			return
		}
		if record.ID == "" {
			record.ID = fmt.Sprintf("obs:%03d", len(out)+1)
		}
		if record.SourceRef.Kind == ObservationSourceUnknown {
			record.SourceRef.Kind = ObservationSourceKindForOrigin(record.Origin)
		}
		if record.GroundingPolicy == ClaimGroundingUnknown {
			record.GroundingPolicy = AnswerClaimBindingGroundingPolicy(record.Origin, record.Role)
		}
		if record.Origin == AnswerEvidenceOriginCurrentSource &&
			record.GroundingPolicy == ClaimGroundingHard &&
			!ObservationRecordHasCurrentSourceLineSpan(record) {
			record.GroundingPolicy = ClaimGroundingRepairable
		}
		out = append(out, record)
	}
	compileEvidenceItemObservations(input.EvidenceItems, add)
	compileAggregateFactObservations(input.AggregateFacts, input.RequestModel, input.RowSetWriter, add)
	compileSourceInventoryObservationObservations(input.SourceInventoryObservation, input.RowSetWriter, add)
	compileToolResultObservations(input.ToolResults, add)
	compileLogBundleObservations(input.LogBundle, add)
	compilePerfBundleObservations(input.PerfBundle, add)
	compileMCPResponseObservations(input.MCPResponses, add)
	out = dedupeObservationRecords(out)
	return ObservationLedger{Records: out}
}

type observationRecordMergeKey struct {
	Origin          AnswerEvidenceOrigin
	SourceRef       ObservationSourceRef
	Span            ObservationSpan
	EvidenceKind    EvidenceKind
	AnchorKind      AnchorKind
	EvidenceScope   EvidenceScope
	ClaimKey        string
	Subject         string
	Predicate       string
	Object          string
	Value           string
	Negative        bool
	ResultCount     string
	Scope           string
	GroundingStatus GroundingStatus
}

func dedupeObservationRecords(in []ObservationRecord) []ObservationRecord {
	if len(in) < 2 {
		return in
	}
	out := make([]ObservationRecord, 0, len(in))
	seen := map[observationRecordMergeKey]int{}
	for _, record := range in {
		key, ok := observationRecordMergeKeyFor(record)
		if !ok {
			out = append(out, record)
			continue
		}
		if idx, exists := seen[key]; exists {
			out[idx] = mergeObservationRecord(out[idx], record)
			continue
		}
		seen[key] = len(out)
		out = append(out, record)
	}
	return out
}

func observationRecordMergeKeyFor(record ObservationRecord) (observationRecordMergeKey, bool) {
	if !observationRecordHasMergeAnchor(record) {
		return observationRecordMergeKey{}, false
	}
	resultCount := ""
	if record.ResultCount != nil {
		resultCount = strconv.Itoa(*record.ResultCount)
	}
	return observationRecordMergeKey{
		Origin:          record.Origin,
		SourceRef:       record.SourceRef,
		Span:            record.Span,
		EvidenceKind:    record.EvidenceKind,
		AnchorKind:      record.AnchorKind,
		EvidenceScope:   record.EvidenceScope,
		ClaimKey:        strings.TrimSpace(record.ClaimKey),
		Subject:         strings.TrimSpace(record.Subject),
		Predicate:       strings.TrimSpace(record.Predicate),
		Object:          strings.TrimSpace(record.Object),
		Value:           strings.TrimSpace(record.Value),
		Negative:        record.Negative,
		ResultCount:     resultCount,
		Scope:           strings.TrimSpace(record.Scope),
		GroundingStatus: record.GroundingStatus,
	}, true
}

func observationRecordHasMergeAnchor(record ObservationRecord) bool {
	if ObservationRecordHasCurrentSourceLineSpan(record) {
		return true
	}
	ref := record.SourceRef
	if strings.TrimSpace(ref.PayloadRef) != "" ||
		strings.TrimSpace(ref.RowSetRef) != "" ||
		strings.TrimSpace(ref.PageRef) != "" ||
		strings.TrimSpace(ref.RawRef) != "" ||
		strings.TrimSpace(ref.ArtifactID) != "" ||
		strings.TrimSpace(ref.URL) != "" ||
		strings.TrimSpace(ref.ResourceURI) != "" ||
		strings.TrimSpace(ref.Commit) != "" ||
		strings.TrimSpace(ref.Command) != "" ||
		strings.TrimSpace(ref.Path) != "" {
		return true
	}
	if strings.TrimSpace(record.ClaimKey) == "" {
		return false
	}
	return strings.TrimSpace(record.Value) != "" || record.ResultCount != nil || record.Negative
}

func mergeObservationRecord(dst, src ObservationRecord) ObservationRecord {
	if AnswerAggregateRolePriority(src.Role) > AnswerAggregateRolePriority(dst.Role) {
		dst.Role = src.Role
	}
	if observationGroundingPolicyPriority(src.GroundingPolicy) > observationGroundingPolicyPriority(dst.GroundingPolicy) {
		dst.GroundingPolicy = src.GroundingPolicy
	}
	if observationProvenanceLanePriority(src.ProvenanceLane) > observationProvenanceLanePriority(dst.ProvenanceLane) {
		dst.ProvenanceLane = src.ProvenanceLane
	}
	if dst.Summary == "" {
		dst.Summary = strings.TrimSpace(src.Summary)
	}
	dst.RichNotes = mergeObservationRecordRichNotes(dst, src)
	dst.SupportRefs = appendUniqueObservationStrings(dst.SupportRefs, src.SupportRefs...)
	if dst.RawExcerpt == "" {
		dst.RawExcerpt = strings.TrimSpace(src.RawExcerpt)
	}
	if dst.ObservedAt == "" {
		dst.ObservedAt = strings.TrimSpace(src.ObservedAt)
	}
	if dst.ResultCount == nil && src.ResultCount != nil {
		n := *src.ResultCount
		dst.ResultCount = &n
	}
	if src.Confidence > dst.Confidence {
		dst.Confidence = src.Confidence
	}
	return dst
}

func mergeObservationRecordRichNotes(dst, src ObservationRecord) []string {
	var out []string
	out = appendUniqueObservationString(out, dst.Summary)
	out = appendUniqueObservationStrings(out, dst.RichNotes...)
	if strings.TrimSpace(src.Summary) != strings.TrimSpace(dst.Summary) {
		out = appendUniqueObservationString(out, src.Summary)
	}
	out = appendUniqueObservationStrings(out, src.RichNotes...)
	return out
}

func observationProvenanceLanePriority(lane ObservationProvenanceLane) int {
	switch lane {
	case ObservationProvenanceObservedDirectCause:
		return 3
	case ObservationProvenanceInferredUpstreamPossibility:
		return 2
	case ObservationProvenanceArtifactSpan:
		return 1
	default:
		return 0
	}
}

func observationGroundingPolicyPriority(policy ClaimGroundingPolicy) int {
	switch policy {
	case ClaimGroundingHard:
		return 3
	case ClaimGroundingRepairable:
		return 2
	case ClaimGroundingSoft:
		return 1
	default:
		return 0
	}
}

func appendUniqueObservationStrings(dst []string, values ...string) []string {
	for _, value := range values {
		dst = appendUniqueObservationString(dst, value)
	}
	return dst
}

func appendUniqueObservationString(dst []string, value string) []string {
	value = strings.Join(strings.Fields(strings.TrimSpace(value)), " ")
	if value == "" {
		return dst
	}
	for _, existing := range dst {
		if strings.Join(strings.Fields(strings.TrimSpace(existing)), " ") == value {
			return dst
		}
	}
	return append(dst, value)
}

func ObservationSourceKindForOrigin(origin AnswerEvidenceOrigin) ObservationSourceKind {
	switch origin {
	case AnswerEvidenceOriginCurrentSource:
		return ObservationSourceCurrentSource
	case AnswerEvidenceOriginVCSMetadata:
		return ObservationSourceVCSMetadata
	case AnswerEvidenceOriginVCSDiff:
		return ObservationSourceVCSDiff
	case AnswerEvidenceOriginRuntimeArtifact:
		return ObservationSourceRuntimeArtifact
	case AnswerEvidenceOriginCommandMeasurement:
		return ObservationSourceCommand
	case AnswerEvidenceOriginRepoNegativeSearch:
		return ObservationSourceCurrentSource
	case AnswerEvidenceOriginCrossRepoIndex:
		return ObservationSourceCrossRepoIndex
	case AnswerEvidenceOriginExternalDocument:
		return ObservationSourceExternalDocument
	case AnswerEvidenceOriginWebPage:
		return ObservationSourceWebPage
	case AnswerEvidenceOriginMCPResource:
		return ObservationSourceMCPResource
	case AnswerEvidenceOriginConnectorResource:
		return ObservationSourceConnector
	default:
		return ObservationSourceUnknown
	}
}

// ObservationRecordHasCurrentSourceLineSpan reports whether a ledger record has
// an exact current-checkout file span. This is the shared predicate for
// source-line citation eligibility; external artifact lines, VCS hunks, command
// output rows, and ungrounded current-source leads deliberately return false.
func ObservationRecordHasCurrentSourceLineSpan(record ObservationRecord) bool {
	if record.Origin != AnswerEvidenceOriginCurrentSource {
		return false
	}
	if record.SourceRef.Kind != ObservationSourceCurrentSource && record.SourceRef.Kind != ObservationSourceUnknown {
		return false
	}
	if strings.TrimSpace(record.SourceRef.Path) == "" || record.Span.LineStart <= 0 {
		return false
	}
	if record.GroundingStatus == GroundingUngrounded {
		return false
	}
	switch record.EvidenceScope {
	case ScopeLine, ScopeLineRange, ScopeSection, ScopeFile, "":
		return true
	default:
		return false
	}
}

// ObservationRecordHasStrongCurrentSourceAnchor is the prompt-budget priority
// predicate for current-source records whose anchor shape is specific enough to
// explain current behavior. It is stricter than "has a line span" but still
// supports mechanisms, config/literal facts, and definitions across languages.
func ObservationRecordHasStrongCurrentSourceAnchor(record ObservationRecord) bool {
	if !ObservationRecordHasCurrentSourceLineSpan(record) {
		return false
	}
	switch record.AnchorKind {
	case AnchorDefinition,
		AnchorCall,
		AnchorCondition,
		AnchorReturn,
		AnchorAssignment,
		AnchorInitializer,
		AnchorImport,
		AnchorStringLiteral:
		return true
	default:
		return false
	}
}

func observationRecordRank(record ObservationRecord, intent *AnswerIntentContract) int {
	rank := 1000
	requestedOrigin := intent == nil || intent.HasOrigin(record.Origin)
	if requestedOrigin {
		rank -= 300
	} else {
		rank += 120
	}
	if record.Origin == AnswerEvidenceOriginCurrentSource && requestedOrigin {
		rank -= 120
		if ObservationRecordHasStrongCurrentSourceAnchor(record) {
			rank -= 160
		}
	}
	if NormalizeAnswerAggregateRole(record.Role).IsPrincipal() {
		rank -= 90
	}
	if observationRecordIsPrincipalAggregate(record) {
		rank -= 120
	}
	switch record.Origin {
	case AnswerEvidenceOriginCurrentSource:
		rank -= 40
	case AnswerEvidenceOriginVCSMetadata, AnswerEvidenceOriginVCSDiff:
		rank -= 35
	case AnswerEvidenceOriginRuntimeArtifact, AnswerEvidenceOriginCommandMeasurement:
		rank -= 30
	case AnswerEvidenceOriginRepoNegativeSearch:
		rank -= 25
	}
	if record.Negative {
		rank -= 20
	}
	if strings.TrimSpace(record.Summary) != "" {
		rank -= 15
	}
	if strings.TrimSpace(record.RawExcerpt) != "" || len(record.RichNotes) > 0 {
		rank -= 5
	}
	if observationRecordIsSourceInventorySet(record) {
		rank -= 180
	} else if observationRecordIsSourceInventoryAttribute(record) {
		rank += 30
	}
	return rank
}

func observationRecordIsPrincipalAggregate(record ObservationRecord) bool {
	return strings.HasPrefix(strings.TrimSpace(record.ID), "aggregate:") &&
		NormalizeAnswerAggregateRole(record.Role).IsPrincipal()
}

func observationRecordIsSourceInventory(record ObservationRecord) bool {
	return strings.HasPrefix(strings.TrimSpace(record.ID), "source_inventory:") ||
		strings.Contains(strings.ToLower(strings.TrimSpace(record.Producer)), "source_inventory")
}

func observationRecordIsSourceInventorySet(record ObservationRecord) bool {
	id := strings.TrimSpace(record.ID)
	return strings.HasPrefix(id, "source_inventory:set:")
}

func observationRecordIsSourceInventoryAttribute(record ObservationRecord) bool {
	id := strings.TrimSpace(record.ID)
	return strings.Contains(id, ":attr:")
}

func compileEvidenceItemObservations(items []EvidenceItem, add func(ObservationRecord)) {
	for i, ev := range items {
		if strings.TrimSpace(ev.Source) == "" && strings.TrimSpace(ev.Summary) == "" {
			continue
		}
		role := AnswerAggregateRoleSupportingCoverage
		if ev.Salience == SalienceLoadBearing || ev.Salience == SalienceExhaustListed {
			role = AnswerAggregateRolePrincipalAnswer
		}
		id := strings.TrimSpace(ev.ID)
		if id == "" {
			id = fmt.Sprintf("evidence:%d", i)
		} else {
			id = "evidence:" + id
		}
		add(ObservationRecord{
			ID:              id,
			Origin:          AnswerEvidenceOriginCurrentSource,
			Producer:        firstNonEmptyString(ev.Producer, "evidence_item"),
			Role:            role,
			GroundingPolicy: AnswerClaimBindingGroundingPolicy(AnswerEvidenceOriginCurrentSource, role),
			SourceRef: ObservationSourceRef{
				Kind: ObservationSourceCurrentSource,
				Path: strings.TrimSpace(ev.Source),
			},
			Span: ObservationSpan{
				LineStart: ev.LineStart,
				LineEnd:   ev.LineEnd,
			},
			ClaimKey:        firstNonEmptyString(ev.AnchorSymbol, ev.Subject, ev.ID),
			EvidenceKind:    ev.Kind,
			AnchorKind:      ev.AnchorKind,
			EvidenceScope:   ev.Scope,
			GroundingStatus: ev.GroundingStatus,
			Subject:         strings.TrimSpace(ev.Subject),
			Predicate:       strings.TrimSpace(ev.Predicate),
			Object:          strings.TrimSpace(ev.Object),
			Summary:         strings.TrimSpace(ev.Summary),
			RawExcerpt:      strings.TrimSpace(ev.Snippet),
			SupportRefs:     cloneStringSlice(ev.SurfaceTerms),
			Confidence:      ev.Confidence,
		})
	}
}

func compileAggregateFactObservations(facts []AnswerAggregateFact, rm *RequestModel, rowSetWriter ObservationRowSetWriter, add func(ObservationRecord)) {
	for i, fact := range facts {
		role := AnswerAggregateFactRoleForRequest(fact, rm)
		origins := AnswerAggregateFactEvidenceOrigins(fact, rm)
		if len(origins) == 0 {
			origins = []AnswerEvidenceOrigin{AnswerEvidenceOriginCurrentSource}
		}
		dims := aggregateDimensionMap(fact.Dimensions)
		for _, origin := range origins {
			resultCount := aggregateFactResultCount(fact, dims)
			sourceRef := sourceRefForAggregateFact(origin, dims)
			if sourceRef.RowSetRef == "" {
				sourceRef.RowSetRef = observationRowSetRefForAggregateFact(rowSetWriter, i, origin, fact)
			}
			add(ObservationRecord{
				ID:              fmt.Sprintf("aggregate:%d#%s", i, origin),
				Origin:          origin,
				Producer:        firstNonEmptyString(fact.Provenance, "aggregate_facts"),
				Role:            role,
				GroundingPolicy: AnswerClaimBindingGroundingPolicy(origin, role),
				ProvenanceLane:  observationProvenanceLaneForAggregateFact(dims),
				SourceRef:       sourceRef,
				Span:            observationSpanForAggregateFact(dims),
				ClaimKey:        firstNonEmptyString(dims["target"], dims["query"], dims["pattern"], dims["predicate"], fact.Label),
				Subject:         firstNonEmptyString(dims["target"], dims["query"], dims["pattern"], fact.Label),
				Predicate:       dims["predicate"],
				Value:           strings.TrimSpace(fact.Value),
				Unit:            strings.TrimSpace(fact.Unit),
				Negative:        fact.Kind == AnswerAggregateNegativeSearch || fact.Kind == AnswerAggregateNegativeObservation,
				ResultCount:     resultCount,
				Summary:         strings.TrimSpace(fact.Label),
				RichNotes:       aggregateFactObservationRichNotes(fact),
				SupportRefs:     cloneStringSlice(fact.SupportRefs),
				ObservedAt:      dims["searched_at"],
				Scope:           dims["scope"],
			})
		}
	}
}

func observationProvenanceLaneForAggregateFact(dims map[string]string) ObservationProvenanceLane {
	if len(dims) == 0 {
		return ObservationProvenanceUnknown
	}
	return NormalizeObservationProvenanceLane(firstNonEmptyString(
		dims["observation_lane"],
		dims["runtime_lane"],
		dims["provenance_lane"],
	))
}

const observationRowSetMinRows = 24

type observationRowSetLine struct {
	Index      int    `json:"index"`
	Member     string `json:"member,omitempty"`
	SupportRef string `json:"support_ref,omitempty"`
	Note       string `json:"note,omitempty"`
	Label      string `json:"label,omitempty"`
	Kind       string `json:"kind,omitempty"`
	Role       string `json:"role,omitempty"`
}

func observationRowSetRefForAggregateFact(writer ObservationRowSetWriter, factIndex int, origin AnswerEvidenceOrigin, fact AnswerAggregateFact) string {
	if writer == nil || observationRowSetStructuredRowCount(fact) < observationRowSetMinRows {
		return ""
	}
	content := observationRowSetJSONLForAggregateFact(fact)
	if strings.TrimSpace(content) == "" {
		return ""
	}
	name := fmt.Sprintf("aggregate-%03d-%s-row-set.jsonl", factIndex, origin)
	return strings.TrimSpace(writer(name, content))
}

func observationRowSetJSONLForAggregateFact(fact AnswerAggregateFact) string {
	rows := observationRowSetMembersForAggregateFact(fact)
	if len(rows) == 0 {
		return ""
	}
	var b strings.Builder
	for i, member := range rows {
		line := observationRowSetLine{
			Index:  i,
			Member: strings.TrimSpace(member),
			Label:  strings.TrimSpace(fact.Label),
			Kind:   string(fact.Kind),
			Role:   string(NormalizeAnswerAggregateRole(fact.Role)),
		}
		line.SupportRef = observationRowSetSupportRefAt(fact, i, member)
		if i < len(fact.MemberNotes) {
			line.Note = strings.Join(strings.Fields(strings.TrimSpace(fact.MemberNotes[i])), " ")
		}
		if line.Member == "" && line.SupportRef == "" && line.Note == "" {
			continue
		}
		raw, err := json.Marshal(line)
		if err != nil {
			continue
		}
		b.Write(raw)
		b.WriteByte('\n')
	}
	return b.String()
}

func observationRowSetStructuredRowCount(fact AnswerAggregateFact) int {
	if AnswerAggregateFactCarriesCompleteMemberSet(fact) {
		return len(fact.Members)
	}
	if observationRowSetCarriesCompleteExcludedSet(fact) {
		return len(fact.Excluded)
	}
	return 0
}

func observationRowSetMembersForAggregateFact(fact AnswerAggregateFact) []string {
	if observationRowSetCarriesCompleteExcludedSet(fact) {
		return cloneStringSlice(fact.Excluded)
	}
	if AnswerAggregateFactCarriesCompleteMemberSet(fact) {
		return cloneStringSlice(fact.Members)
	}
	return nil
}

func observationRowSetCarriesCompleteExcludedSet(fact AnswerAggregateFact) bool {
	if fact.Kind != AnswerAggregateExcluded || len(fact.Excluded) == 0 {
		return false
	}
	want, ok, err := parseAggregateCountValue(fact)
	return err == nil && ok && want == len(fact.Excluded)
}

func observationRowSetSupportRefAt(fact AnswerAggregateFact, idx int, member string) string {
	if idx < 0 || len(fact.SupportRefs) == 0 {
		return ""
	}
	rowCount := observationRowSetStructuredRowCount(fact)
	if len(fact.SupportRefs) == rowCount && idx < len(fact.SupportRefs) {
		return strings.TrimSpace(fact.SupportRefs[idx])
	}
	memberKey := AnswerAggregateMemberSurfaceKey(member)
	if memberKey == "" {
		return ""
	}
	for _, ref := range fact.SupportRefs {
		label, _, ok := strings.Cut(ref, ":")
		if !ok {
			continue
		}
		if AnswerAggregateMemberSurfaceKey(label) == memberKey {
			return strings.TrimSpace(ref)
		}
	}
	return ""
}

func compileSourceInventoryObservationObservations(observation SourceInventoryObservation, rowSetWriter ObservationRowSetWriter, add func(ObservationRecord)) {
	if !observation.IsActive() {
		return
	}
	for setIdx, set := range observation.Sets {
		if len(set.Members) == 0 {
			continue
		}
		count := len(set.Members)
		sourceRef := ObservationSourceRef{
			Kind:      ObservationSourceCurrentSource,
			Path:      firstNonEmptyString(firstSourceInventoryObservationScope(observation), firstSourceInventoryObservationMemberPath(set.Members)),
			RowSetRef: observationRowSetRefForSourceInventoryObservation(rowSetWriter, setIdx, set),
		}
		add(ObservationRecord{
			ID:              fmt.Sprintf("source_inventory:set:%d", setIdx),
			Origin:          AnswerEvidenceOriginCurrentSource,
			Producer:        firstNonEmptyString(strings.Join(observation.Provenance, ","), "source_inventory_observation"),
			Role:            AnswerAggregateRoleSupportingCoverage,
			GroundingPolicy: ClaimGroundingRepairable,
			SourceRef:       sourceRef,
			EvidenceKind:    EvidenceDirect,
			AnchorKind:      AnchorTextReference,
			EvidenceScope:   ScopeFile,
			GroundingStatus: GroundingRecovered,
			ClaimKey:        "source_inventory:" + string(set.Role),
			Subject:         string(set.Role),
			Predicate:       "source_inventory_count",
			ResultCount:     &count,
			Summary:         fmt.Sprintf("source-inventory %s count=%d complete=%t", SourceInventoryAdvisoryRoleLabel(set.Role), count, set.Complete),
			SupportRefs:     sourceInventoryObservationSupportRefs(set.Members),
			Scope:           strings.Join(observation.Scopes, ","),
			Confidence:      0.9,
		})
		for memberIdx, member := range set.Members {
			if strings.TrimSpace(member.Name) == "" {
				continue
			}
			record := sourceInventoryObservationMemberRecord(setIdx, memberIdx, member)
			add(record)
			for attrIdx, attr := range member.Attributes {
				if strings.TrimSpace(attr.Name) == "" {
					continue
				}
				add(sourceInventoryObservationAttributeRecord(setIdx, memberIdx, attrIdx, member, attr))
			}
		}
	}
}

func sourceInventoryObservationMemberRecord(setIdx, memberIdx int, member SourceInventoryObservationMember) ObservationRecord {
	scope := ScopeFile
	anchor := AnchorTextReference
	span := ObservationSpan{}
	if member.Line > 0 {
		scope = ScopeLine
		anchor = sourceInventoryObservationAnchorForRole(member.Role)
		span.LineStart = member.Line
	}
	return ObservationRecord{
		ID:              fmt.Sprintf("source_inventory:%d:%d", setIdx, memberIdx),
		Origin:          AnswerEvidenceOriginCurrentSource,
		Producer:        "source_inventory_observation",
		Role:            AnswerAggregateRoleSupportingCoverage,
		GroundingPolicy: ClaimGroundingRepairable,
		SourceRef: ObservationSourceRef{
			Kind: ObservationSourceCurrentSource,
			Path: strings.TrimSpace(member.File),
		},
		Span:            span,
		EvidenceKind:    EvidenceDirect,
		AnchorKind:      anchor,
		EvidenceScope:   scope,
		GroundingStatus: sourceInventoryObservationGrounding(member.CoverageState),
		ClaimKey:        firstNonEmptyString(member.Key, member.Name),
		Subject:         strings.TrimSpace(member.Name),
		Predicate:       "source_inventory_member",
		Summary:         strings.TrimSpace(member.Note),
		SupportRefs:     sourceInventoryObservationSingleSupportRef(member.SupportRef),
		Scope:           strings.TrimSpace(member.File),
		Confidence:      0.9,
	}
}

func sourceInventoryObservationAttributeRecord(setIdx, memberIdx, attrIdx int, member SourceInventoryObservationMember, attr SourceInventoryObservationAttribute) ObservationRecord {
	scope := ScopeFile
	anchor := AnchorTextReference
	span := ObservationSpan{}
	if attr.Line > 0 {
		scope = ScopeLine
		anchor = sourceInventoryObservationAnchorForRole(attr.Role)
		span.LineStart = attr.Line
	}
	notes := []string{}
	if strings.TrimSpace(attr.Ambiguity) != "" {
		notes = append(notes, "ambiguity="+strings.TrimSpace(attr.Ambiguity))
	}
	if strings.TrimSpace(attr.Reason) != "" {
		notes = append(notes, strings.TrimSpace(attr.Reason))
	}
	return ObservationRecord{
		ID:              fmt.Sprintf("source_inventory:%d:%d:attr:%d", setIdx, memberIdx, attrIdx),
		Origin:          AnswerEvidenceOriginCurrentSource,
		Producer:        "source_inventory_observation",
		Role:            AnswerAggregateRoleSupportingCoverage,
		GroundingPolicy: ClaimGroundingRepairable,
		SourceRef: ObservationSourceRef{
			Kind: ObservationSourceCurrentSource,
			Path: strings.TrimSpace(attr.File),
		},
		Span:            span,
		EvidenceKind:    EvidenceDirect,
		AnchorKind:      anchor,
		EvidenceScope:   scope,
		GroundingStatus: sourceInventoryObservationGrounding(attr.CoverageState),
		ClaimKey:        firstNonEmptyString(attr.Key, attr.Name),
		Subject:         strings.TrimSpace(member.Name),
		Predicate:       "source_inventory_attribute",
		Object:          strings.TrimSpace(attr.Name),
		Summary:         strings.TrimSpace(attr.Note),
		RichNotes:       notes,
		SupportRefs:     sourceInventoryObservationSingleSupportRef(attr.SupportRef),
		Scope:           strings.TrimSpace(member.File),
		Confidence:      0.85,
	}
}

func sourceInventoryObservationSingleSupportRef(ref string) []string {
	ref = strings.TrimSpace(ref)
	if ref == "" {
		return nil
	}
	return []string{ref}
}

func sourceInventoryObservationGrounding(state SourceInventoryCoverageState) GroundingStatus {
	switch state {
	case SourceInventoryCoverageObserved, SourceInventoryCoverageAmbiguous:
		return GroundingRecovered
	default:
		return GroundingUngrounded
	}
}

func sourceInventoryObservationAnchorForRole(role AnswerCandidateRole) AnchorKind {
	switch role {
	case AnswerCandidateRoleFunction,
		AnswerCandidateRoleMethod,
		AnswerCandidateRoleType,
		AnswerCandidateRoleConstant,
		AnswerCandidateRoleVariable,
		AnswerCandidateRoleField,
		AnswerCandidateRolePackage,
		AnswerCandidateRoleConfigKey,
		AnswerCandidateRoleRoute,
		AnswerCandidateRoleImportPath,
		AnswerCandidateRoleLiteralValue:
		return AnchorDefinition
	default:
		return AnchorTextReference
	}
}

func firstSourceInventoryObservationScope(observation SourceInventoryObservation) string {
	for _, scope := range observation.Scopes {
		if strings.TrimSpace(scope) != "" {
			return strings.TrimSpace(scope)
		}
	}
	return ""
}

func firstSourceInventoryObservationMemberPath(members []SourceInventoryObservationMember) string {
	for _, member := range members {
		if strings.TrimSpace(member.File) != "" {
			return strings.TrimSpace(member.File)
		}
	}
	return ""
}

func sourceInventoryObservationSupportRefs(members []SourceInventoryObservationMember) []string {
	out := make([]string, 0, len(members))
	for _, member := range members {
		if ref := strings.TrimSpace(member.SupportRef); ref != "" {
			out = appendUniqueObservationString(out, ref)
		}
	}
	return out
}

func observationRowSetRefForSourceInventoryObservation(writer ObservationRowSetWriter, setIndex int, set SourceInventoryObservationSet) string {
	if writer == nil || len(set.Members) < observationRowSetMinRows {
		return ""
	}
	content := observationRowSetJSONLForSourceInventoryObservation(set)
	if strings.TrimSpace(content) == "" {
		return ""
	}
	name := fmt.Sprintf("source-inventory-%03d-%s-row-set.jsonl", setIndex, set.Role)
	return strings.TrimSpace(writer(name, content))
}

func observationRowSetJSONLForSourceInventoryObservation(set SourceInventoryObservationSet) string {
	var b strings.Builder
	for i, member := range set.Members {
		line := observationRowSetLine{
			Index:      i,
			Member:     strings.TrimSpace(member.Name),
			SupportRef: strings.TrimSpace(member.SupportRef),
			Note:       strings.Join(strings.Fields(strings.TrimSpace(member.Note)), " "),
			Role:       string(set.Role),
		}
		if line.Member == "" && line.SupportRef == "" && line.Note == "" {
			continue
		}
		raw, err := json.Marshal(line)
		if err != nil {
			continue
		}
		b.Write(raw)
		b.WriteByte('\n')
	}
	return b.String()
}

func compileToolResultObservations(results []ToolResult, add func(ObservationRecord)) {
	for i, result := range results {
		if !result.Success {
			continue
		}
		banners := toolResultBanners(result.Summary)
		command := toolResultCommandLine(result.Summary)
		origins := toolResultEvidenceOrigins(result)
		if strings.EqualFold(strings.TrimSpace(result.ToolName), "trace_query") {
			compileTraceQueryToolResultObservations(i, result, banners, add)
		}
		for _, origin := range origins {
			role := AnswerAggregateRoleSupportingCoverage
			add(ObservationRecord{
				ID:              fmt.Sprintf("tool:%d#%s", i, origin),
				Origin:          origin,
				Producer:        strings.TrimSpace(result.ToolName),
				Role:            role,
				GroundingPolicy: AnswerClaimBindingGroundingPolicy(origin, role),
				SourceRef:       sourceRefForToolResult(origin, result, i, banners, command),
				Span:            spanForToolResult(banners),
				ClaimKey:        toolResultClaimKey(result, banners),
				ResultCount:     toolResultResultCount(result.Summary, banners),
				Summary:         firstNonBannerLine(result.Summary),
				RawExcerpt:      clippedObservationExcerpt(result.Summary),
				RichNotes:       toolResultRichNotes(result.Summary),
				ObservedAt:      result.Timestamp.Format("2006-01-02T15:04:05Z07:00"),
			})
		}
	}
}

func compileTraceQueryToolResultObservations(index int, result ToolResult, banners []map[string]string, add func(ObservationRecord)) {
	ref := sourceRefForToolResult(AnswerEvidenceOriginRuntimeArtifact, result, index, banners, "")
	observedAt := result.Timestamp.Format("2006-01-02T15:04:05Z07:00")
	rootCauseOrdinal := 0
	rootEvidenceOrdinal := 0
	blockingOrdinal := 0
	stateChurnOrdinal := 0
	fileIOOrdinal := 0
	pageCacheOrdinal := 0
	storageLatencyOrdinal := 0
	ioPressureOrdinal := 0
	runtimeResourceOrdinals := map[string]int{}
	pluginEventOrdinal := 0
	lines := strings.Split(result.Summary, "\n")
	for lineIdx := 0; lineIdx < len(lines); lineIdx++ {
		rawLine := lines[lineIdx]
		line := strings.TrimSpace(rawLine)
		switch {
		case strings.HasPrefix(line, "- rank="):
			rootCauseOrdinal++
			if record, ok := traceQueryRootCauseRankRecord(index, rootCauseOrdinal, line, ref, observedAt); ok {
				add(record)
			}
		case strings.HasPrefix(line, "- root_evidence="):
			rootEvidenceOrdinal++
			if record, ok := traceQueryRootEvidenceRecord(index, rootEvidenceOrdinal, line, ref, observedAt); ok {
				add(record)
			}
		case strings.HasPrefix(line, "- blocking "):
			blockingOrdinal++
			if record, ok := traceQueryBlockingRecord(index, blockingOrdinal, line, ref, observedAt); ok {
				add(record)
			}
		case strings.HasPrefix(line, "- state_churn "):
			stateChurnOrdinal++
			if record, ok := traceQueryStateChurnRecord(index, stateChurnOrdinal, line, ref, observedAt); ok {
				add(record)
			}
		case strings.HasPrefix(line, "- file_io "):
			fileIOOrdinal++
			if record, ok := traceQueryFileIORecord(index, fileIOOrdinal, line, ref, observedAt); ok {
				add(record)
			}
		case strings.HasPrefix(line, "- page_cache "):
			pageCacheOrdinal++
			if record, ok := traceQueryPageCacheRecord(index, pageCacheOrdinal, line, ref, observedAt); ok {
				add(record)
			}
		case strings.HasPrefix(line, "- storage_latency "):
			storageLatencyOrdinal++
			if record, ok := traceQueryStorageLatencyRecord(index, storageLatencyOrdinal, line, ref, observedAt); ok {
				add(record)
			}
		case strings.HasPrefix(line, "- io_pressure "):
			ioPressureOrdinal++
			if record, ok := traceQueryIOPressureRecord(index, ioPressureOrdinal, line, ref, observedAt); ok {
				add(record)
			}
		case traceQueryRuntimeResourceLabel(line) != "":
			label := traceQueryRuntimeResourceLabel(line)
			runtimeResourceOrdinals[label]++
			continuation := traceQueryIndentedContinuation(lines, lineIdx+1, "callstack=")
			if record, ok := traceQueryRuntimeResourceRecord(index, runtimeResourceOrdinals[label], label, line, continuation, ref, observedAt); ok {
				add(record)
			}
		case strings.HasPrefix(line, "- plugin_event "):
			pluginEventOrdinal++
			if record, ok := traceQueryPluginEventRecord(index, pluginEventOrdinal, line, ref, observedAt); ok {
				add(record)
			}
		}
	}
}

func traceQueryRootCauseRankRecord(index, ordinal int, line string, ref ObservationSourceRef, observedAt string) (ObservationRecord, bool) {
	fields, summary := traceQuerySummaryLineFields(line, "- ")
	rank := traceQueryFieldInt(fields, "rank")
	tier := strings.TrimSpace(fields["tier"])
	typ := strings.TrimSpace(fields["type"])
	thread := strings.TrimSpace(fields["thread"])
	impact := traceQueryFieldMS(fields, "impact")
	score := traceQueryFieldFloat(fields, "score")
	conf := traceQueryFieldFloat(fields, "confidence")
	lineStart, lineEnd := traceQueryFieldLineSpan(fields["lines"])
	if rank <= 0 {
		rank = ordinal
	}
	if tier == "" {
		tier = rootCauseTierName(rank)
	}
	if typ == "" && summary == "" {
		return ObservationRecord{}, false
	}
	value := ""
	if impact > 0 {
		value = fmt.Sprintf("%.3f", impact)
	}
	return ObservationRecord{
		ID:              fmt.Sprintf("tool:%d#trace_query:root_cause_rank:%d", index, rank),
		Origin:          AnswerEvidenceOriginRuntimeArtifact,
		Producer:        "trace_query",
		Role:            AnswerAggregateRolePrincipalAnswer,
		GroundingPolicy: ClaimGroundingHard,
		ProvenanceLane:  ObservationProvenanceObservedDirectCause,
		SourceRef:       ref,
		Span:            ObservationSpan{LineStart: lineStart, LineEnd: lineEnd},
		ClaimKey:        firstNonEmptyString("root_cause_"+tier, typ),
		Subject:         thread,
		Predicate:       "root_cause_" + tier,
		Object:          typ,
		Value:           value,
		Unit:            "ms",
		Summary:         firstNonEmptyString(summary, fmt.Sprintf("%s cause #%d (%s)", tier, rank, typ)),
		RichNotes:       traceQueryPriorityRichNotes(rank, tier, typ, fields["source"], score, impact),
		SupportRefs:     traceQuerySupportRefs(ref, lineStart, lineEnd),
		ObservedAt:      observedAt,
		Confidence:      conf,
	}, true
}

func traceQueryRootEvidenceRecord(index, ordinal int, line string, ref ObservationSourceRef, observedAt string) (ObservationRecord, bool) {
	fields, summary := traceQuerySummaryLineFields(line, "- ")
	typ := strings.TrimSpace(fields["root_evidence"])
	thread := strings.TrimSpace(fields["thread"])
	duration := traceQueryFieldMS(fields, "duration")
	conf := traceQueryFieldFloat(fields, "confidence")
	lineStart, lineEnd := traceQueryFieldLineSpan(fields["lines"])
	if typ == "" && summary == "" {
		return ObservationRecord{}, false
	}
	value := ""
	if duration > 0 {
		value = fmt.Sprintf("%.3f", duration)
	}
	return ObservationRecord{
		ID:              fmt.Sprintf("tool:%d#trace_query:root_evidence:%d", index, ordinal),
		Origin:          AnswerEvidenceOriginRuntimeArtifact,
		Producer:        "trace_query",
		Role:            AnswerAggregateRoleSupportingCoverage,
		GroundingPolicy: ClaimGroundingHard,
		ProvenanceLane:  ObservationProvenanceObservedDirectCause,
		SourceRef:       ref,
		Span:            ObservationSpan{LineStart: lineStart, LineEnd: lineEnd},
		ClaimKey:        "root_evidence:" + typ,
		Subject:         thread,
		Predicate:       typ,
		Value:           value,
		Unit:            "ms",
		Summary:         summary,
		SupportRefs:     traceQuerySupportRefs(ref, lineStart, lineEnd),
		ObservedAt:      observedAt,
		Confidence:      conf,
	}, true
}

func traceQueryBlockingRecord(index, ordinal int, line string, ref ObservationSourceRef, observedAt string) (ObservationRecord, bool) {
	fields, summary := traceQuerySummaryLineFields(line, "- blocking ")
	typ := strings.TrimSpace(fields["type"])
	thread := strings.TrimSpace(fields["thread"])
	peer := strings.TrimSpace(fields["peer"])
	duration := traceQueryFieldMS(fields, "duration")
	conf := traceQueryFieldFloat(fields, "confidence")
	lineStart, lineEnd := traceQueryFieldLineSpan(fields["lines"])
	if typ == "" && summary == "" {
		return ObservationRecord{}, false
	}
	value := ""
	if duration > 0 {
		value = fmt.Sprintf("%.3f", duration)
	}
	return ObservationRecord{
		ID:              fmt.Sprintf("tool:%d#trace_query:critical_blocking:%d", index, ordinal),
		Origin:          AnswerEvidenceOriginRuntimeArtifact,
		Producer:        "trace_query",
		Role:            AnswerAggregateRoleSupportingCoverage,
		GroundingPolicy: ClaimGroundingHard,
		ProvenanceLane:  ObservationProvenanceObservedDirectCause,
		SourceRef:       ref,
		Span:            ObservationSpan{LineStart: lineStart, LineEnd: lineEnd},
		ClaimKey:        "critical_blocking:" + typ,
		Subject:         thread,
		Predicate:       "critical_blocking",
		Object:          firstNonEmptyString(typ, peer),
		Value:           value,
		Unit:            "ms",
		Summary:         summary,
		SupportRefs:     traceQuerySupportRefs(ref, lineStart, lineEnd),
		ObservedAt:      observedAt,
		Confidence:      conf,
	}, true
}

func traceQueryStateChurnRecord(index, ordinal int, line string, ref ObservationSourceRef, observedAt string) (ObservationRecord, bool) {
	thread, fields, summary := traceQueryStateChurnLineFields(line)
	dominant := strings.TrimSpace(fields["dominant_state"])
	impact := traceQueryFieldMS(fields, "impact")
	total := traceQueryFieldMS(fields, "total")
	conf := traceQueryFieldFloat(fields, "confidence")
	lineStart, lineEnd := traceQueryFieldLineSpan(fields["lines"])
	if dominant == "" && summary == "" {
		return ObservationRecord{}, false
	}
	value := ""
	if impact > 0 {
		value = fmt.Sprintf("%.3f", impact)
	}
	return ObservationRecord{
		ID:              fmt.Sprintf("tool:%d#trace_query:state_churn:%d", index, ordinal),
		Origin:          AnswerEvidenceOriginRuntimeArtifact,
		Producer:        "trace_query",
		Role:            AnswerAggregateRoleSupportingCoverage,
		GroundingPolicy: ClaimGroundingHard,
		ProvenanceLane:  ObservationProvenanceObservedDirectCause,
		SourceRef:       ref,
		Span:            ObservationSpan{LineStart: lineStart, LineEnd: lineEnd},
		ClaimKey:        "state_churn:" + dominant,
		Subject:         thread,
		Predicate:       "state_churn",
		Object:          dominant,
		Value:           value,
		Unit:            "ms",
		Summary:         summary,
		RichNotes:       traceQueryStateChurnRichNotes(fields, total),
		SupportRefs:     traceQuerySupportRefs(ref, lineStart, lineEnd),
		ObservedAt:      observedAt,
		Confidence:      conf,
	}, true
}

func traceQueryStateChurnLineFields(line string) (string, map[string]string, string) {
	fields := map[string]string{}
	body := strings.TrimSpace(strings.TrimPrefix(strings.TrimSpace(line), "- state_churn "))
	head, summary, ok := strings.Cut(body, " — ")
	if !ok {
		head = body
	}
	thread := ""
	for _, token := range strings.Fields(head) {
		key, value, ok := strings.Cut(token, "=")
		if !ok {
			if thread == "" {
				thread = strings.TrimSpace(token)
			}
			continue
		}
		fields[strings.ToLower(strings.TrimSpace(key))] = strings.TrimSpace(value)
	}
	return thread, fields, strings.TrimSpace(summary)
}

func traceQueryStateChurnRichNotes(fields map[string]string, total float64) []string {
	var notes []string
	for _, key := range []string{"dominant_state", "fragments", "switches", "max_segment", "p95_segment", "running", "runnable", "sleep", "d_state", "io_wait"} {
		if value := strings.TrimSpace(fields[key]); value != "" {
			notes = append(notes, key+"="+value)
		}
	}
	if total > 0 {
		notes = append(notes, fmt.Sprintf("total=%.3fms", total))
	}
	return notes
}

func traceQueryFileIORecord(index, ordinal int, line string, ref ObservationSourceRef, observedAt string) (ObservationRecord, bool) {
	fields, summary := traceQuerySummaryLineFields(line, "- file_io ")
	inode := strings.TrimSpace(fields["inode"])
	op := strings.TrimSpace(fields["op"])
	name := strings.TrimSpace(fields["name"])
	bytes := strings.TrimSpace(fields["bytes"])
	lineStart, lineEnd := traceQueryFieldLineSpan(fields["lines"])
	if inode == "" && summary == "" {
		return ObservationRecord{}, false
	}
	return ObservationRecord{
		ID:              fmt.Sprintf("tool:%d#trace_query:file_io:%d", index, ordinal),
		Origin:          AnswerEvidenceOriginRuntimeArtifact,
		Producer:        "trace_query",
		Role:            AnswerAggregateRoleSupportingCoverage,
		GroundingPolicy: ClaimGroundingHard,
		ProvenanceLane:  ObservationProvenanceObservedDirectCause,
		SourceRef:       ref,
		Span:            ObservationSpan{LineStart: lineStart, LineEnd: lineEnd},
		ClaimKey:        "file_io:" + firstNonEmptyString(inode, name, op),
		Subject:         firstNonEmptyString(name, "inode="+inode),
		Predicate:       "file_io_by_inode",
		Object:          op,
		Value:           bytes,
		Unit:            "bytes",
		Summary:         summary,
		RichNotes:       traceQuerySelectedRichNotes(fields, []string{"inode", "dev", "name", "op", "thread", "count", "bytes", "total_latency", "max_latency", "offsets"}),
		SupportRefs:     traceQuerySupportRefs(ref, lineStart, lineEnd),
		ObservedAt:      observedAt,
		Confidence:      0.74,
	}, true
}

func traceQueryPageCacheRecord(index, ordinal int, line string, ref ObservationSourceRef, observedAt string) (ObservationRecord, bool) {
	fields, summary := traceQuerySummaryLineFields(line, "- page_cache ")
	inode := strings.TrimSpace(fields["inode"])
	dev := strings.TrimSpace(fields["dev"])
	churn := strings.TrimSpace(fields["churn"])
	lineStart, lineEnd := traceQueryFieldLineSpan(fields["lines"])
	if inode == "" && summary == "" {
		return ObservationRecord{}, false
	}
	return ObservationRecord{
		ID:              fmt.Sprintf("tool:%d#trace_query:page_cache:%d", index, ordinal),
		Origin:          AnswerEvidenceOriginRuntimeArtifact,
		Producer:        "trace_query",
		Role:            AnswerAggregateRoleSupportingCoverage,
		GroundingPolicy: ClaimGroundingHard,
		ProvenanceLane:  ObservationProvenanceObservedDirectCause,
		SourceRef:       ref,
		Span:            ObservationSpan{LineStart: lineStart, LineEnd: lineEnd},
		ClaimKey:        "page_cache:" + firstNonEmptyString(inode, dev),
		Subject:         "inode=" + inode,
		Predicate:       "page_cache_by_inode",
		Object:          dev,
		Value:           churn,
		Unit:            "events",
		Summary:         summary,
		RichNotes:       traceQuerySelectedRichNotes(fields, []string{"inode", "dev", "thread", "adds", "deletes", "churn", "bytes", "offsets"}),
		SupportRefs:     traceQuerySupportRefs(ref, lineStart, lineEnd),
		ObservedAt:      observedAt,
		Confidence:      0.70,
	}, true
}

func traceQueryStorageLatencyRecord(index, ordinal int, line string, ref ObservationSourceRef, observedAt string) (ObservationRecord, bool) {
	fields, summary := traceQuerySummaryLineFields(line, "- storage_latency ")
	layer := strings.TrimSpace(fields["layer"])
	event := strings.TrimSpace(fields["event"])
	maxLatency := traceQueryFieldMS(fields, "max_latency")
	lineStart, lineEnd := traceQueryFieldLineSpan(fields["lines"])
	if layer == "" && summary == "" {
		return ObservationRecord{}, false
	}
	value := ""
	if maxLatency > 0 {
		value = fmt.Sprintf("%.3f", maxLatency)
	}
	return ObservationRecord{
		ID:              fmt.Sprintf("tool:%d#trace_query:storage_latency:%d", index, ordinal),
		Origin:          AnswerEvidenceOriginRuntimeArtifact,
		Producer:        "trace_query",
		Role:            AnswerAggregateRoleSupportingCoverage,
		GroundingPolicy: ClaimGroundingHard,
		ProvenanceLane:  ObservationProvenanceObservedDirectCause,
		SourceRef:       ref,
		Span:            ObservationSpan{LineStart: lineStart, LineEnd: lineEnd},
		ClaimKey:        "storage_latency:" + firstNonEmptyString(layer, event),
		Subject:         layer,
		Predicate:       "storage_latency_by_layer",
		Object:          event,
		Value:           value,
		Unit:            "ms",
		Summary:         summary,
		RichNotes:       traceQuerySelectedRichNotes(fields, []string{"layer", "event", "dev", "op", "thread", "count", "paired", "unpaired_start", "unpaired_done", "max_latency", "avg_latency", "bytes"}),
		SupportRefs:     traceQuerySupportRefs(ref, lineStart, lineEnd),
		ObservedAt:      observedAt,
		Confidence:      0.72,
	}, true
}

func traceQueryIOPressureRecord(index, ordinal int, line string, ref ObservationSourceRef, observedAt string) (ObservationRecord, bool) {
	fields, summary := traceQuerySummaryLineFields(line, "- io_pressure ")
	signal := strings.TrimSpace(fields["signal"])
	score := traceQueryFieldFloat(fields, "score")
	lineStart, lineEnd := traceQueryFieldLineSpan(fields["lines"])
	if signal == "" && summary == "" {
		return ObservationRecord{}, false
	}
	value := ""
	if score > 0 {
		value = fmt.Sprintf("%.3f", score)
	}
	return ObservationRecord{
		ID:              fmt.Sprintf("tool:%d#trace_query:io_pressure:%d", index, ordinal),
		Origin:          AnswerEvidenceOriginRuntimeArtifact,
		Producer:        "trace_query",
		Role:            AnswerAggregateRoleSupportingCoverage,
		GroundingPolicy: ClaimGroundingHard,
		ProvenanceLane:  ObservationProvenanceObservedDirectCause,
		SourceRef:       ref,
		Span:            ObservationSpan{LineStart: lineStart, LineEnd: lineEnd},
		ClaimKey:        "io_pressure:" + signal,
		Subject:         "io_pressure",
		Predicate:       signal,
		Object:          strings.TrimSpace(fields["top_inode"]),
		Value:           value,
		Unit:            "score",
		Summary:         summary,
		RichNotes:       traceQuerySelectedRichNotes(fields, []string{"signal", "score", "block_max", "storage_max", "file_bytes", "file_events", "page_cache_churn", "iowait_blocked", "d_state", "top_inode", "top_dev", "top_name"}),
		SupportRefs:     traceQuerySupportRefs(ref, lineStart, lineEnd),
		ObservedAt:      observedAt,
		Confidence:      0.70,
	}, true
}

func traceQueryRuntimeResourceLabel(line string) string {
	switch {
	case strings.HasPrefix(line, "- bio_resource "):
		return "bio"
	case strings.HasPrefix(line, "- filesystem_resource "):
		return "filesystem"
	case strings.HasPrefix(line, "- page_fault_resource "):
		return "page_fault"
	default:
		return ""
	}
}

func traceQueryIndentedContinuation(lines []string, index int, prefix string) string {
	if index < 0 || index >= len(lines) {
		return ""
	}
	line := strings.TrimSpace(lines[index])
	if !strings.HasPrefix(line, prefix) {
		return ""
	}
	return line
}

func traceQueryRuntimeResourceRecord(index, ordinal int, label, line, continuation string, ref ObservationSourceRef, observedAt string) (ObservationRecord, bool) {
	fields, summary := traceQuerySummaryLineFields(line, "- "+label+"_resource ")
	path := strings.TrimSpace(fields["path"])
	op := strings.TrimSpace(fields["op"])
	totalLatency := traceQueryFieldMS(fields, "total_latency")
	lineStart, lineEnd := traceQueryFieldLineSpan(fields["line"])
	if path == "" && op == "" && summary == "" {
		return ObservationRecord{}, false
	}
	value := ""
	if totalLatency > 0 {
		value = fmt.Sprintf("%.3f", totalLatency)
	}
	notes := traceQuerySelectedRichNotes(fields, []string{"op", "path", "thread", "count", "total_latency", "max_latency", "bytes", "line", "example"})
	if continuation != "" {
		notes = append(notes, continuation)
	}
	return ObservationRecord{
		ID:              fmt.Sprintf("tool:%d#trace_query:%s_resource:%d", index, label, ordinal),
		Origin:          AnswerEvidenceOriginRuntimeArtifact,
		Producer:        "trace_query",
		Role:            AnswerAggregateRoleSupportingCoverage,
		GroundingPolicy: ClaimGroundingSoft,
		ProvenanceLane:  ObservationProvenanceArtifactSpan,
		SourceRef:       ref,
		Span:            ObservationSpan{LineStart: lineStart, LineEnd: lineEnd},
		ClaimKey:        label + "_resource:" + firstNonEmptyString(path, op),
		Subject:         firstNonEmptyString(path, label+"_resource"),
		Predicate:       label + "_resource",
		Object:          op,
		Value:           value,
		Unit:            "ms",
		Summary:         firstNonEmptyString(summary, traceQueryRuntimeResourceSummary(label, fields)),
		RichNotes:       notes,
		SupportRefs:     traceQuerySupportRefs(ref, lineStart, lineEnd),
		ObservedAt:      observedAt,
		Confidence:      0.68,
	}, true
}

func traceQueryRuntimeResourceSummary(label string, fields map[string]string) string {
	parts := []string{label + "_resource"}
	for _, key := range []string{"op", "path", "total_latency", "max_latency", "bytes", "count"} {
		if value := strings.TrimSpace(fields[key]); value != "" {
			parts = append(parts, key+"="+value)
		}
	}
	return strings.Join(parts, " ")
}

func traceQueryPluginEventRecord(index, ordinal int, line string, ref ObservationSourceRef, observedAt string) (ObservationRecord, bool) {
	fields, summary := traceQuerySummaryLineFields(line, "- plugin_event ")
	kind := strings.TrimSpace(fields["kind"])
	domain := strings.TrimSpace(fields["domain"])
	event := strings.TrimSpace(fields["event"])
	metric := strings.TrimSpace(fields["metric"])
	value := strings.TrimSpace(fields["value"])
	lineStart, lineEnd := traceQueryFieldLineSpan(fields["line"])
	if kind == "" && event == "" && summary == "" {
		return ObservationRecord{}, false
	}
	return ObservationRecord{
		ID:              fmt.Sprintf("tool:%d#trace_query:plugin_event:%d", index, ordinal),
		Origin:          AnswerEvidenceOriginRuntimeArtifact,
		Producer:        "trace_query",
		Role:            AnswerAggregateRoleSupportingCoverage,
		GroundingPolicy: ClaimGroundingSoft,
		ProvenanceLane:  ObservationProvenanceArtifactSpan,
		SourceRef:       ref,
		Span:            ObservationSpan{LineStart: lineStart, LineEnd: lineEnd},
		ClaimKey:        "plugin_event:" + firstNonEmptyString(kind, domain, event, metric),
		Subject:         firstNonEmptyString(domain, kind, "plugin_event"),
		Predicate:       firstNonEmptyString(kind, "plugin_event"),
		Object:          firstNonEmptyString(event, metric),
		Value:           value,
		Summary:         firstNonEmptyString(summary, traceQueryPluginEventSummary(fields)),
		RichNotes:       traceQuerySelectedRichNotes(fields, []string{"kind", "domain", "event", "metric", "value", "category", "thread", "count", "line", "example"}),
		SupportRefs:     traceQuerySupportRefs(ref, lineStart, lineEnd),
		ObservedAt:      observedAt,
		Confidence:      0.68,
	}, true
}

func traceQueryPluginEventSummary(fields map[string]string) string {
	parts := []string{"plugin_event"}
	for _, key := range []string{"kind", "domain", "event", "metric", "value", "category", "count"} {
		if value := strings.TrimSpace(fields[key]); value != "" {
			parts = append(parts, key+"="+value)
		}
	}
	return strings.Join(parts, " ")
}

func traceQuerySelectedRichNotes(fields map[string]string, keys []string) []string {
	var notes []string
	for _, key := range keys {
		if value := strings.TrimSpace(fields[key]); value != "" {
			notes = append(notes, key+"="+value)
		}
	}
	return notes
}

func traceQuerySummaryLineFields(line, prefix string) (map[string]string, string) {
	fields := map[string]string{}
	body := strings.TrimSpace(strings.TrimPrefix(strings.TrimSpace(line), prefix))
	head, summary, ok := strings.Cut(body, " — ")
	if !ok {
		head = body
	}
	for _, token := range strings.Fields(head) {
		key, value, ok := strings.Cut(token, "=")
		if !ok {
			continue
		}
		fields[strings.ToLower(strings.TrimSpace(key))] = strings.TrimSpace(value)
	}
	return fields, strings.TrimSpace(summary)
}

func traceQueryFieldInt(fields map[string]string, key string) int {
	raw := strings.TrimSpace(fields[key])
	if raw == "" {
		return 0
	}
	n, err := strconv.Atoi(raw)
	if err != nil {
		return 0
	}
	return n
}

func traceQueryFieldFloat(fields map[string]string, key string) float64 {
	raw := strings.TrimSpace(fields[key])
	if raw == "" {
		return 0
	}
	raw = strings.TrimSuffix(raw, ",")
	n, err := strconv.ParseFloat(raw, 64)
	if err != nil {
		return 0
	}
	return n
}

func traceQueryFieldMS(fields map[string]string, key string) float64 {
	raw := strings.TrimSpace(fields[key])
	raw = strings.TrimSuffix(raw, "ms")
	if raw == "" {
		return 0
	}
	n, err := strconv.ParseFloat(raw, 64)
	if err != nil {
		return 0
	}
	return n
}

func traceQueryFieldLineSpan(raw string) (int, int) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return 0, 0
	}
	startRaw, endRaw, ok := strings.Cut(raw, "-")
	start, err := strconv.Atoi(strings.TrimSpace(startRaw))
	if err != nil {
		return 0, 0
	}
	if !ok {
		return start, start
	}
	end, err := strconv.Atoi(strings.TrimSpace(endRaw))
	if err != nil {
		return start, start
	}
	return start, end
}

func traceQueryPriorityRichNotes(rank int, tier, typ, source string, score, impact float64) []string {
	var notes []string
	if rank > 0 {
		notes = append(notes, fmt.Sprintf("rank=%d", rank))
	}
	if tier != "" {
		notes = append(notes, "tier="+tier)
	}
	if typ != "" {
		notes = append(notes, "type="+typ)
	}
	if impact > 0 {
		notes = append(notes, fmt.Sprintf("impact_ms=%.3f", impact))
	}
	if score > 0 {
		notes = append(notes, fmt.Sprintf("score=%.3f", score))
	}
	if source != "" {
		notes = append(notes, "source="+source)
	}
	return notes
}

func traceQuerySupportRefs(ref ObservationSourceRef, lineStart, lineEnd int) []string {
	if lineStart <= 0 {
		return nil
	}
	path := firstNonEmptyString(ref.Path, ref.ArtifactID, "runtime_artifact")
	if lineEnd <= 0 || lineEnd == lineStart {
		return []string{fmt.Sprintf("%s:%d", path, lineStart)}
	}
	return []string{fmt.Sprintf("%s:%d-%d", path, lineStart, lineEnd)}
}

func rootCauseTierName(rank int) string {
	switch rank {
	case 1:
		return "primary"
	case 2:
		return "secondary"
	case 3:
		return "tertiary"
	default:
		if rank > 0 {
			return fmt.Sprintf("rank_%d", rank)
		}
		return "ranked"
	}
}

func compileLogBundleObservations(bundle *LogBundle, add func(ObservationRecord)) {
	if bundle == nil {
		return
	}
	var errIndex int
	var walkErr func(LogError)
	walkErr = func(err LogError) {
		target := firstNonEmptyString(err.Type, err.Message)
		if target != "" {
			add(ObservationRecord{
				ID:              fmt.Sprintf("log:error:%d", errIndex),
				Origin:          AnswerEvidenceOriginRuntimeArtifact,
				Producer:        "log_triage",
				Role:            AnswerAggregateRolePrincipalAnswer,
				GroundingPolicy: AnswerClaimBindingGroundingPolicy(AnswerEvidenceOriginRuntimeArtifact, AnswerAggregateRolePrincipalAnswer),
				ProvenanceLane:  ObservationProvenanceObservedDirectCause,
				SourceRef: ObservationSourceRef{
					Kind:         ObservationSourceRuntimeArtifact,
					ArtifactID:   "attached_log",
					ArtifactKind: "log",
				},
				ClaimKey:    target,
				Subject:     target,
				Summary:     firstNonEmptyString(err.Message, err.Type),
				RichNotes:   logErrorObservationRichNotes(err),
				SupportRefs: logFrameRawRefs(err.Frames),
			})
			errIndex++
		}
		if err.Cause != nil {
			walkErr(*err.Cause)
		}
	}
	for _, err := range bundle.Errors {
		walkErr(err)
	}
	for i, obs := range bundle.Observations {
		role := logObservationRecordRole(bundle, obs)
		add(ObservationRecord{
			ID:              fmt.Sprintf("log:observation:%d", i),
			Origin:          AnswerEvidenceOriginRuntimeArtifact,
			Producer:        "log_triage",
			Role:            role,
			GroundingPolicy: AnswerClaimBindingGroundingPolicy(AnswerEvidenceOriginRuntimeArtifact, role),
			ProvenanceLane:  observationProvenanceLaneForLogObservation(obs),
			SourceRef: ObservationSourceRef{
				Kind:         ObservationSourceRuntimeArtifact,
				ArtifactID:   "attached_log",
				ArtifactKind: "log",
			},
			Span: ObservationSpan{
				LineStart: obs.LineStart,
				LineEnd:   obs.LineEnd,
			},
			ClaimKey:   firstNonEmptyString(obs.Subject, string(obs.Kind)),
			Subject:    strings.TrimSpace(obs.Subject),
			Predicate:  string(obs.Kind),
			Summary:    strings.TrimSpace(obs.Summary),
			RawExcerpt: strings.TrimSpace(obs.Evidence),
			Confidence: obs.Confidence,
		})
	}
}

func logErrorObservationRichNotes(err LogError) []string {
	if len(err.Frames) == 0 {
		return nil
	}
	return []string{"Stack frames are artifact-local runtime support; they are not current-source root-cause proof unless separate current-source evidence grounds them."}
}

func observationProvenanceLaneForLogObservation(obs LogObservation) ObservationProvenanceLane {
	if obs.Diagnostic || obs.Severity == LogObservationFailure || obs.Severity == LogObservationWarning {
		return ObservationProvenanceObservedDirectCause
	}
	if obs.LineStart > 0 || obs.LineEnd > 0 || strings.TrimSpace(obs.Evidence) != "" {
		return ObservationProvenanceArtifactSpan
	}
	return ObservationProvenanceUnknown
}

func logObservationRecordRole(bundle *LogBundle, obs LogObservation) AnswerAggregateRole {
	if bundle == nil {
		return AnswerAggregateRolePrincipalAnswer
	}
	if bundle.IsExternalSource() &&
		len(bundle.Errors) > 0 &&
		!obs.Diagnostic &&
		(obs.Severity == "" || obs.Severity == LogObservationInfo) {
		return AnswerAggregateRoleSupportingCoverage
	}
	return AnswerAggregateRolePrincipalAnswer
}

func compilePerfBundleObservations(bundle *PerfBundle, add func(ObservationRecord)) {
	if bundle == nil {
		return
	}
	for i, frame := range bundle.Frames {
		label := perfFrameObservationLabel(frame)
		add(ObservationRecord{
			ID:              fmt.Sprintf("perf:frame:%d", i),
			Origin:          AnswerEvidenceOriginRuntimeArtifact,
			Producer:        "perf_trace",
			Role:            AnswerAggregateRolePrincipalAnswer,
			GroundingPolicy: AnswerClaimBindingGroundingPolicy(AnswerEvidenceOriginRuntimeArtifact, AnswerAggregateRolePrincipalAnswer),
			ProvenanceLane:  ObservationProvenanceArtifactSpan,
			SourceRef:       perfObservationSourceRef(bundle, ""),
			Span: ObservationSpan{
				StartTsMs: frame.TsMs,
				EndTsMs:   perfFrameEndTs(frame),
			},
			ClaimKey:   firstNonEmptyString(label.ClaimKey, "frame_sample"),
			Subject:    firstNonEmptyString(label.Subject, "frame sample"),
			Value:      strconv.FormatFloat(frame.DurationMs, 'f', -1, 64),
			Unit:       "ms",
			Summary:    fmt.Sprintf("%s duration %gms", firstNonEmptyString(label.Subject, "frame sample"), frame.DurationMs),
			ObservedAt: strconv.FormatFloat(frame.TsMs, 'f', -1, 64),
		})
	}
	for i, jank := range bundle.Janks {
		add(ObservationRecord{
			ID:              fmt.Sprintf("perf:jank:%d", i),
			Origin:          AnswerEvidenceOriginRuntimeArtifact,
			Producer:        "perf_trace",
			Role:            AnswerAggregateRolePrincipalAnswer,
			GroundingPolicy: AnswerClaimBindingGroundingPolicy(AnswerEvidenceOriginRuntimeArtifact, AnswerAggregateRolePrincipalAnswer),
			ProvenanceLane:  observationProvenanceLaneForPerfJank(jank),
			SourceRef:       perfObservationSourceRef(bundle, ""),
			Span: ObservationSpan{
				StartTsMs: jank.StartTsMs,
				EndTsMs:   jank.StartTsMs + jank.DurationMs,
			},
			ClaimKey:  firstNonEmptyString(jank.TriggerSpan, "jank"),
			Subject:   firstNonEmptyString(jank.TriggerSpan, "jank"),
			Predicate: "duration",
			Value:     strconv.FormatFloat(jank.DurationMs, 'f', -1, 64),
			Unit:      "ms",
			Summary:   firstNonEmptyString(jank.Reason, "observed jank"),
			RichNotes: cloneStringSlice(jank.Tags),
		})
	}
	for i, stall := range bundle.Stalls {
		add(ObservationRecord{
			ID:              fmt.Sprintf("perf:stall:%d", i),
			Origin:          AnswerEvidenceOriginRuntimeArtifact,
			Producer:        "perf_trace",
			Role:            AnswerAggregateRolePrincipalAnswer,
			GroundingPolicy: AnswerClaimBindingGroundingPolicy(AnswerEvidenceOriginRuntimeArtifact, AnswerAggregateRolePrincipalAnswer),
			ProvenanceLane:  observationProvenanceLaneForPerfStall(stall),
			SourceRef:       perfObservationSourceRef(bundle, stall.File),
			Span: ObservationSpan{
				LineStart: stall.Line,
				StartTsMs: stall.StartTsMs,
				EndTsMs:   stall.StartTsMs + stall.DurationMs,
			},
			ClaimKey:  firstNonEmptyString(stall.Symbol, stall.Kind, "stall"),
			Subject:   firstNonEmptyString(stall.Symbol, stall.Kind, "stall"),
			Predicate: "duration",
			Value:     strconv.FormatFloat(stall.DurationMs, 'f', -1, 64),
			Unit:      "ms",
			Summary:   firstNonEmptyString(stall.Kind, "observed stall"),
		})
	}
	if bundle.Startup != nil {
		add(ObservationRecord{
			ID:              "perf:startup",
			Origin:          AnswerEvidenceOriginRuntimeArtifact,
			Producer:        "perf_trace",
			Role:            AnswerAggregateRolePrincipalAnswer,
			GroundingPolicy: AnswerClaimBindingGroundingPolicy(AnswerEvidenceOriginRuntimeArtifact, AnswerAggregateRolePrincipalAnswer),
			ProvenanceLane:  ObservationProvenanceArtifactSpan,
			SourceRef:       perfObservationSourceRef(bundle, ""),
			ClaimKey:        firstNonEmptyString(bundle.Startup.Mode, "startup"),
			Subject:         firstNonEmptyString(bundle.Startup.Mode, "startup"),
			Predicate:       "launch_duration",
			Value:           strconv.FormatFloat(bundle.Startup.AppLaunchMs, 'f', -1, 64),
			Unit:            "ms",
			Summary:         firstNonEmptyString(bundle.Meta.Summary, "startup timing"),
		})
	}
	for i, obs := range bundle.Observations {
		add(ObservationRecord{
			ID:              fmt.Sprintf("perf:observation:%d", i),
			Origin:          AnswerEvidenceOriginRuntimeArtifact,
			Producer:        "perf_trace",
			Role:            AnswerAggregateRolePrincipalAnswer,
			GroundingPolicy: AnswerClaimBindingGroundingPolicy(AnswerEvidenceOriginRuntimeArtifact, AnswerAggregateRolePrincipalAnswer),
			ProvenanceLane:  observationProvenanceLaneForPerfObservation(obs),
			SourceRef:       perfObservationSourceRef(bundle, ""),
			Span: ObservationSpan{
				LineStart: obs.LineStart,
				LineEnd:   obs.LineEnd,
				StartTsMs: obs.StartTsMs,
				EndTsMs:   obs.EndTsMs,
			},
			ClaimKey:   firstNonEmptyString(obs.Subject, obs.Kind),
			Subject:    strings.TrimSpace(obs.Subject),
			Predicate:  strings.TrimSpace(obs.Kind),
			Value:      strconv.FormatFloat(obs.DurationMs, 'f', -1, 64),
			Unit:       "ms",
			Summary:    strings.TrimSpace(obs.Summary),
			RawExcerpt: strings.TrimSpace(obs.Evidence),
			RichNotes:  cloneStringSlice(obs.Tags),
			Confidence: obs.Confidence,
		})
	}
}

type perfFrameObservationLabelResult struct {
	ClaimKey string
	Subject  string
}

func perfFrameObservationLabel(frame PerfFrame) perfFrameObservationLabelResult {
	if frame.FrameNo > 0 {
		return perfFrameObservationLabelResult{
			ClaimKey: fmt.Sprintf("frame:%d", frame.FrameNo),
			Subject:  fmt.Sprintf("frame %d", frame.FrameNo),
		}
	}
	return perfFrameObservationLabelResult{
		ClaimKey: "frame_sample",
		Subject:  "frame sample",
	}
}

func perfFrameEndTs(frame PerfFrame) float64 {
	if frame.TsMs <= 0 || frame.DurationMs <= 0 {
		return 0
	}
	return frame.TsMs + frame.DurationMs
}

func observationProvenanceLaneForPerfJank(jank PerfJank) ObservationProvenanceLane {
	if strings.TrimSpace(jank.Reason) != "" || strings.TrimSpace(jank.TriggerSpan) != "" {
		return ObservationProvenanceObservedDirectCause
	}
	return ObservationProvenanceArtifactSpan
}

func observationProvenanceLaneForPerfStall(stall PerfStall) ObservationProvenanceLane {
	if strings.TrimSpace(stall.Kind) != "" || strings.TrimSpace(stall.Symbol) != "" {
		return ObservationProvenanceObservedDirectCause
	}
	return ObservationProvenanceArtifactSpan
}

func observationProvenanceLaneForPerfObservation(obs PerfObservation) ObservationProvenanceLane {
	if obs.LineStart > 0 || obs.LineEnd > 0 || obs.StartTsMs > 0 || obs.EndTsMs > 0 || obs.DurationMs > 0 {
		return ObservationProvenanceArtifactSpan
	}
	return ObservationProvenanceUnknown
}

func perfObservationSourceRef(bundle *PerfBundle, path string) ObservationSourceRef {
	artifactKind := "trace"
	if bundle != nil {
		artifactKind = firstNonEmptyString(bundle.Meta.Source, artifactKind)
	}
	return ObservationSourceRef{
		Kind:         ObservationSourceRuntimeArtifact,
		Path:         strings.TrimSpace(path),
		ArtifactID:   "attached_trace",
		ArtifactKind: artifactKind,
	}
}

func compileMCPResponseObservations(responses []MCPResponse, add func(ObservationRecord)) {
	for i, response := range responses {
		if !response.Success {
			continue
		}
		if len(response.Observations) == 0 {
			add(mcpObservationRecordFromResponse(fmt.Sprintf("mcp:%d", i), response, MCPTypedObservation{}))
			continue
		}
		for j, obs := range response.Observations {
			add(mcpObservationRecordFromResponse(fmt.Sprintf("mcp:%d:%d", i, j), response, obs))
		}
	}
}

func mcpObservationRecordFromResponse(id string, response MCPResponse, obs MCPTypedObservation) ObservationRecord {
	summary := firstNonEmptyString(obs.Summary, response.Summary)
	rawExcerpt := firstNonEmptyString(obs.Summary, response.Summary)
	return ObservationRecord{
		ID:              id,
		Origin:          AnswerEvidenceOriginMCPResource,
		Producer:        firstNonEmptyString(response.Method, "mcp"),
		Role:            AnswerAggregateRoleSupportingCoverage,
		GroundingPolicy: AnswerClaimBindingGroundingPolicy(AnswerEvidenceOriginMCPResource, AnswerAggregateRoleSupportingCoverage),
		SourceRef: ObservationSourceRef{
			Kind:        ObservationSourceMCPResource,
			Server:      strings.TrimSpace(response.ServerName),
			RawRef:      strings.TrimSpace(firstNonEmptyString(obs.RawRef, response.RawRef)),
			PayloadRef:  strings.TrimSpace(firstNonEmptyString(obs.PayloadRef, response.PayloadRef)),
			RowSetRef:   strings.TrimSpace(firstNonEmptyString(obs.RowSetRef, response.RowSetRef)),
			PageRef:     strings.TrimSpace(firstNonEmptyString(obs.PageRef, response.PageRef)),
			ResourceURI: strings.TrimSpace(firstNonEmptyString(obs.ResourceURI, response.ResourceURI)),
			MIMEType:    strings.TrimSpace(firstNonEmptyString(obs.MIMEType, response.MIMEType)),
		},
		Span: ObservationSpan{
			LineStart:   firstPositiveInt(obs.LineStart, response.LineStart),
			LineEnd:     firstPositiveInt(obs.LineEnd, response.LineEnd),
			Selector:    strings.TrimSpace(firstNonEmptyString(obs.Selector, response.Selector)),
			JSONPointer: strings.TrimSpace(firstNonEmptyString(obs.JSONPointer, response.JSONPointer)),
			Row:         firstPositiveInt(obs.Row, response.Row),
		},
		ClaimKey:   firstNonEmptyString(response.Method, response.ServerName),
		Summary:    firstLine(strings.TrimSpace(summary)),
		RawExcerpt: clippedObservationExcerpt(rawExcerpt),
		ObservedAt: response.Timestamp.Format("2006-01-02T15:04:05Z07:00"),
	}
}

func aggregateFactResultCount(fact AnswerAggregateFact, dims map[string]string) *int {
	for _, raw := range []string{dims["result_count"], fact.Value} {
		n, err := strconv.Atoi(strings.TrimSpace(raw))
		if err == nil && n >= 0 {
			return &n
		}
	}
	return nil
}

func sourceRefForAggregateFact(origin AnswerEvidenceOrigin, dims map[string]string) ObservationSourceRef {
	ref := ObservationSourceRef{Kind: ObservationSourceKindForOrigin(origin)}
	ref.PayloadRef = firstNonEmptyString(dims["payload_ref"], dims["blob_ref"], dims["source_blob"], dims["tool_raw_ref"], dims["raw_ref"])
	ref.RowSetRef = dims["row_set_ref"]
	ref.PageRef = dims["page_ref"]
	ref.RawRef = firstNonEmptyString(dims["raw_ref"], ref.PayloadRef, ref.RowSetRef, ref.PageRef)
	ref.ToolCallID = firstNonEmptyString(dims["tool_call_id"], dims["tool_result"])
	ref.Command = strings.TrimSpace(dims["command"])
	ref.MIMEType = strings.TrimSpace(dims["mime_type"])
	switch origin {
	case AnswerEvidenceOriginCurrentSource, AnswerEvidenceOriginRepoNegativeSearch:
		ref.Repo = dims["repo"]
		ref.Path = firstNonEmptyString(dims["path"], dims["scope"])
	case AnswerEvidenceOriginVCSMetadata, AnswerEvidenceOriginVCSDiff:
		ref.Repo = dims["repo"]
		ref.Commit = dims["commit"]
		ref.Range = firstNonEmptyString(dims["commit_range"], dims["range"])
		ref.Pathspec = firstNonEmptyString(dims["pathspec"], dims["diff_path"], dims["window_path"])
	case AnswerEvidenceOriginRuntimeArtifact:
		ref.ArtifactID = firstNonEmptyString(dims["artifact_id"], dims["trace_window"], dims["scope"])
		ref.ArtifactKind = firstNonEmptyString(dims["artifact_kind"], dims["origin"], "runtime_artifact")
		ref.Path = firstNonEmptyString(dims["path"], dims["artifact_path"])
	case AnswerEvidenceOriginCommandMeasurement:
		ref.Command = dims["command"]
	case AnswerEvidenceOriginCrossRepoIndex:
		ref.Repo = dims["repo"]
		ref.Path = firstNonEmptyString(dims["path"], dims["scope"])
	case AnswerEvidenceOriginExternalDocument:
		ref.ResourceURI = firstNonEmptyString(dims["source_ref"], dims["resource_uri"], dims["scope"])
	case AnswerEvidenceOriginWebPage:
		ref.URL = firstNonEmptyString(dims["url"], dims["source_ref"], dims["scope"])
		ref.FetchedAt = dims["fetched_at"]
	case AnswerEvidenceOriginMCPResource:
		ref.Server = dims["server"]
		ref.ResourceURI = firstNonEmptyString(dims["resource_uri"], dims["source_ref"], dims["scope"])
	case AnswerEvidenceOriginConnectorResource:
		ref.Connector = dims["connector"]
		ref.ResourceURI = firstNonEmptyString(dims["resource_uri"], dims["source_ref"], dims["scope"])
	}
	return ref
}

func observationSpanForAggregateFact(dims map[string]string) ObservationSpan {
	if len(dims) == 0 {
		return ObservationSpan{}
	}
	span := ObservationSpan{
		LineStart:   firstNonNegativeIntDim(dims, "line_start", "line"),
		LineEnd:     firstNonNegativeIntDim(dims, "line_end"),
		OldLine:     firstNonNegativeIntDim(dims, "old_line"),
		NewLine:     firstNonNegativeIntDim(dims, "new_line"),
		Paragraph:   firstNonNegativeIntDim(dims, "paragraph"),
		Row:         firstNonNegativeIntDim(dims, "row"),
		TextStart:   firstNonNegativeIntDim(dims, "text_start"),
		TextEnd:     firstNonNegativeIntDim(dims, "text_end"),
		StartTs:     firstFloatDim(dims, "start_ts", "time_start"),
		EndTs:       firstFloatDim(dims, "end_ts", "time_end"),
		StartTsMs:   firstFloatDim(dims, "start_ts_ms", "start_ms"),
		EndTsMs:     firstFloatDim(dims, "end_ts_ms", "end_ms"),
		HunkHeader:  strings.TrimSpace(dims["hunk_header"]),
		Selector:    strings.TrimSpace(dims["selector"]),
		JSONPointer: strings.TrimSpace(dims["json_pointer"]),
	}
	if span.LineEnd == 0 && span.LineStart > 0 {
		span.LineEnd = span.LineStart
	}
	return span
}

func firstNonNegativeIntDim(dims map[string]string, keys ...string) int {
	for _, key := range keys {
		raw := strings.TrimSpace(dims[key])
		if raw == "" {
			continue
		}
		n, err := strconv.Atoi(raw)
		if err == nil && n >= 0 {
			return n
		}
	}
	return 0
}

func firstFloatDim(dims map[string]string, keys ...string) float64 {
	for _, key := range keys {
		raw := strings.TrimSpace(dims[key])
		if raw == "" {
			continue
		}
		n, err := strconv.ParseFloat(raw, 64)
		if err == nil {
			return n
		}
	}
	return 0
}

func sourceRefForToolResult(origin AnswerEvidenceOrigin, result ToolResult, index int, banners []map[string]string, command string) ObservationSourceRef {
	ref := ObservationSourceRef{
		Kind:       ObservationSourceKindForOrigin(origin),
		ToolCallID: fmt.Sprintf("%s[%d]", strings.TrimSpace(result.ToolName), index),
		RawRef:     strings.TrimSpace(result.RawRef),
		PayloadRef: strings.TrimSpace(result.RawRef),
	}
	switch origin {
	case AnswerEvidenceOriginVCSMetadata, AnswerEvidenceOriginVCSDiff:
		ref.Repo = firstBannerValue(banners, "repo", "repo_path", "path")
		ref.Commit = firstBannerValue(banners, "commit")
		ref.Range = firstBannerValue(banners, "commit_range", "range", "ref")
		ref.Pathspec = firstBannerValue(banners, "pathspec", "diff_path", "window_path")
		if ref.Pathspec == "" && result.ToolName == "git_show" {
			ref.Pathspec = firstBannerValue(banners, "path")
		}
		if result.ToolName == "git_show" && ref.Commit == "" {
			ref.Commit = firstBannerValue(banners, "ref")
		}
		if result.ToolName == "git_history_search" {
			ref.Range = compactToolResultRange(
				firstBannerValue(banners, "order"),
				firstBannerValue(banners, "window_count"),
			)
		}
		if result.ToolName == "git_log" {
			ref.Range = compactGitLogToolResultRange(
				firstBannerValue(banners, "ref"),
				firstBannerValue(banners, "count"),
				firstBannerValue(banners, "first_parent"),
				firstBannerValue(banners, "merges_only"),
				firstBannerValue(banners, "no_merges"),
			)
		}
	case AnswerEvidenceOriginCommandMeasurement:
		ref.Command = command
	case AnswerEvidenceOriginRuntimeArtifact:
		ref.Path = firstBannerValue(banners, "path", "artifact_path")
		ref.ArtifactID = firstNonEmptyString(firstBannerValue(banners, "artifact_id"), firstBannerValue(banners, "source"), "trace_query")
		ref.ArtifactKind = firstNonEmptyString(firstBannerValue(banners, "artifact_kind"), "runtime_artifact")
		ref.PayloadRef = firstNonEmptyString(firstBannerValue(banners, "payload_ref", "blob_ref", "raw_ref"), ref.PayloadRef)
		ref.RowSetRef = firstBannerValue(banners, "row_set_ref")
		ref.PageRef = firstBannerValue(banners, "page_ref")
		ref.RawRef = firstNonEmptyString(firstBannerValue(banners, "raw_ref"), ref.RawRef, ref.PayloadRef, ref.RowSetRef)
	}
	if ref.Command == "" && result.ToolName == "exec_command" {
		ref.Command = command
	}
	return ref
}

func spanForToolResult(banners []map[string]string) ObservationSpan {
	return ObservationSpan{
		LineStart: parseFirstBannerInt(banners, "line_start", "start_line"),
		LineEnd:   parseFirstBannerInt(banners, "line_end", "end_line"),
		StartTs:   parseFirstBannerFloat(banners, "start_ts", "time_start"),
		EndTs:     parseFirstBannerFloat(banners, "end_ts", "time_end"),
		StartTsMs: parseFirstBannerFloat(banners, "start_ts_ms", "time_start_ms"),
		EndTsMs:   parseFirstBannerFloat(banners, "end_ts_ms", "time_end_ms"),
	}
}

func parseFirstBannerInt(banners []map[string]string, keys ...string) int {
	raw := firstBannerValue(banners, keys...)
	if raw == "" {
		return 0
	}
	n, err := strconv.Atoi(strings.TrimSpace(raw))
	if err != nil {
		return 0
	}
	return n
}

func parseFirstBannerFloat(banners []map[string]string, keys ...string) float64 {
	raw := firstBannerValue(banners, keys...)
	if raw == "" {
		return 0
	}
	n, err := strconv.ParseFloat(strings.TrimSpace(raw), 64)
	if err != nil {
		return 0
	}
	return n
}

func compactGitLogToolResultRange(ref, count, firstParent, mergesOnly, noMerges string) string {
	var parts []string
	appendPart := func(key, value string) {
		value = strings.TrimSpace(value)
		if value == "" {
			return
		}
		parts = append(parts, key+"="+value)
	}
	appendBool := func(key, value string) {
		if strings.EqualFold(strings.TrimSpace(value), "true") {
			parts = append(parts, key+"=true")
		}
	}
	appendPart("ref", ref)
	appendPart("count", count)
	appendBool("first_parent", firstParent)
	appendBool("merges_only", mergesOnly)
	appendBool("no_merges", noMerges)
	return strings.Join(parts, " ")
}

// FormatObservationSourceRef renders an origin-specific source address for
// downstream prompts. It deliberately labels every field so external payloads
// are not mistaken for current-source file:line citations.
func FormatObservationSourceRef(ref ObservationSourceRef, maxValueLen int) string {
	parts := make([]string, 0, 8)
	if ref.Kind != ObservationSourceUnknown {
		parts = append(parts, "kind="+string(ref.Kind))
	}
	appendPart := func(key, value string) {
		if value = strings.TrimSpace(value); value != "" {
			parts = append(parts, key+"="+clampObservationSourceRefValue(value, maxValueLen))
		}
	}
	appendPart("repo", ref.Repo)
	appendPart("path", ref.Path)
	appendPart("commit", ref.Commit)
	appendPart("range", ref.Range)
	appendPart("pathspec", ref.Pathspec)
	appendPart("command", ref.Command)
	appendPart("tool_call_id", ref.ToolCallID)
	appendPart("payload_ref", ref.PayloadRef)
	appendPart("row_set_ref", ref.RowSetRef)
	appendPart("page_ref", ref.PageRef)
	if raw := strings.TrimSpace(ref.RawRef); raw != "" &&
		raw != strings.TrimSpace(ref.PayloadRef) &&
		raw != strings.TrimSpace(ref.RowSetRef) &&
		raw != strings.TrimSpace(ref.PageRef) {
		appendPart("raw_ref", raw)
	}
	appendPart("artifact_id", ref.ArtifactID)
	appendPart("artifact_kind", ref.ArtifactKind)
	appendPart("url", ref.URL)
	appendPart("fetched_at", ref.FetchedAt)
	appendPart("server", ref.Server)
	appendPart("resource_uri", ref.ResourceURI)
	appendPart("mime_type", ref.MIMEType)
	appendPart("connector", ref.Connector)
	if len(parts) == 0 {
		return "unknown"
	}
	return strings.Join(parts, " | ")
}

func clampObservationSourceRefValue(value string, max int) string {
	value = strings.TrimSpace(value)
	if max <= 0 {
		return value
	}
	runes := []rune(value)
	if len(runes) <= max {
		return value
	}
	return strings.TrimSpace(string(runes[:max])) + "...[truncated]"
}

func compactToolResultRange(order, count string) string {
	order = strings.TrimSpace(order)
	count = strings.TrimSpace(count)
	switch {
	case order != "" && count != "":
		return fmt.Sprintf("order=%s window_count=%s", order, count)
	case order != "":
		return "order=" + order
	case count != "":
		return "window_count=" + count
	default:
		return ""
	}
}

func toolResultClaimKey(result ToolResult, banners []map[string]string) string {
	return firstNonEmptyString(
		firstBannerValue(banners, "target", "query", "contains", "ref", "pathspec", "diff_path", "window_path"),
		strings.TrimSpace(result.ToolName),
	)
}

func toolResultResultCount(summary string, banners []map[string]string) *int {
	for _, raw := range []string{
		firstKeyValueLine(summary, "answer_count"),
		firstKeyValueLine(summary, "result_count"),
		firstBannerValue(banners, "answer_count", "result_count"),
	} {
		n, err := strconv.Atoi(strings.TrimSpace(raw))
		if err == nil && n >= 0 {
			return &n
		}
	}
	return nil
}

func firstKeyValueLine(summary, key string) string {
	prefix := strings.ToLower(strings.TrimSpace(key)) + "="
	for _, line := range strings.Split(summary, "\n") {
		trimmed := strings.TrimSpace(line)
		lower := strings.ToLower(trimmed)
		if strings.HasPrefix(lower, prefix) {
			return strings.TrimSpace(trimmed[len(prefix):])
		}
	}
	return ""
}

func firstBannerValue(banners []map[string]string, keys ...string) string {
	for _, banner := range banners {
		for _, key := range keys {
			if value := strings.TrimSpace(banner[strings.ToLower(strings.TrimSpace(key))]); value != "" {
				return value
			}
		}
	}
	return ""
}

func toolResultEvidenceOrigins(result ToolResult) []AnswerEvidenceOrigin {
	seen := map[AnswerEvidenceOrigin]bool{}
	var out []AnswerEvidenceOrigin
	add := func(origin AnswerEvidenceOrigin) {
		if origin == AnswerEvidenceOriginUnknown || !origin.IsValid() || seen[origin] {
			return
		}
		seen[origin] = true
		out = append(out, origin)
	}
	for _, kv := range toolResultBanners(result.Summary) {
		for _, key := range []string{"origin", "evidence_origin", "secondary_origin", "diff_origin", "proof_source", "tool", "source", "measurement_kind", "measurement_origin"} {
			answerEvidenceOriginFromStructuredToken(kv[key], add)
		}
	}
	return out
}

func toolResultCommandLine(summary string) string {
	for _, line := range strings.Split(summary, "\n") {
		line = strings.TrimSpace(line)
		const prefix = "[exec_command: $ "
		if strings.HasPrefix(line, prefix) && strings.HasSuffix(line, "]") {
			return strings.TrimSpace(strings.TrimSuffix(strings.TrimPrefix(line, prefix), "]"))
		}
	}
	return ""
}

func toolResultBanners(summary string) []map[string]string {
	var out []map[string]string
	for _, line := range strings.Split(summary, "\n") {
		line = strings.TrimSpace(line)
		if !strings.HasPrefix(line, "[") || !strings.HasSuffix(line, "]") {
			continue
		}
		body := strings.TrimSuffix(strings.TrimPrefix(line, "["), "]")
		colon := strings.Index(body, ":")
		if colon < 0 {
			continue
		}
		fields := strings.Fields(strings.TrimSpace(body[colon+1:]))
		kv := map[string]string{}
		for _, field := range fields {
			k, v, ok := strings.Cut(field, "=")
			if !ok {
				continue
			}
			kv[strings.ToLower(strings.TrimSpace(k))] = strings.TrimSpace(v)
		}
		if len(kv) > 0 {
			out = append(out, kv)
		}
	}
	return out
}

func logFrameRawRefs(frames []LogFrame) []string {
	out := make([]string, 0, len(frames))
	for _, frame := range frames {
		if raw := strings.TrimSpace(frame.Raw); raw != "" {
			out = append(out, raw)
			continue
		}
		if frame.File != "" && frame.Line > 0 {
			out = append(out, fmt.Sprintf("%s:%d", frame.File, frame.Line))
		}
	}
	return out
}

func firstNonEmptyString(values ...string) string {
	for _, value := range values {
		if trimmed := strings.TrimSpace(value); trimmed != "" {
			return trimmed
		}
	}
	return ""
}

func firstPositiveInt(values ...int) int {
	for _, value := range values {
		if value > 0 {
			return value
		}
	}
	return 0
}

func firstLine(s string) string {
	if idx := strings.IndexByte(s, '\n'); idx >= 0 {
		return strings.TrimSpace(s[:idx])
	}
	return strings.TrimSpace(s)
}

func firstNonBannerLine(s string) string {
	for _, line := range strings.Split(s, "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		if strings.HasPrefix(line, "[") && strings.HasSuffix(line, "]") {
			continue
		}
		if toolResultExactDetailLine(line) {
			continue
		}
		return line
	}
	return firstLine(s)
}

func toolResultRichNotes(summary string) []string {
	const maxNotes = 16
	var out []string
	for _, line := range strings.Split(summary, "\n") {
		line = strings.TrimSpace(line)
		if !toolResultExactDetailLine(line) {
			continue
		}
		out = appendUniqueObservationString(out, line)
		if len(out) >= maxNotes {
			break
		}
	}
	return out
}

func toolResultExactDetailLine(line string) bool {
	line = strings.TrimSpace(line)
	return strings.HasPrefix(line, "exact_changed_paths:") ||
		strings.HasPrefix(line, "exact_path ") ||
		strings.HasPrefix(line, "exact_path_")
}

func clippedObservationExcerpt(s string) string {
	const max = 600
	s = strings.TrimSpace(s)
	if len([]rune(s)) <= max {
		return s
	}
	runes := []rune(s)
	return string(runes[:max]) + "...[truncated]"
}

func aggregateFactObservationRichNotes(fact AnswerAggregateFact) []string {
	if len(fact.MemberNotes) == 0 {
		return cloneStringSlice(fact.Members)
	}
	out := make([]string, 0, len(fact.MemberNotes)+len(fact.Members))
	seen := make(map[string]struct{}, len(fact.MemberNotes)+len(fact.Members))
	appendOne := func(raw string) {
		trimmed := strings.TrimSpace(raw)
		if trimmed == "" {
			return
		}
		key := strings.Join(strings.Fields(trimmed), " ")
		if _, ok := seen[key]; ok {
			return
		}
		seen[key] = struct{}{}
		out = append(out, trimmed)
	}
	for _, note := range fact.MemberNotes {
		appendOne(note)
	}
	for _, member := range fact.Members {
		appendOne(member)
	}
	return out
}

func cloneStringSlice(in []string) []string {
	if len(in) == 0 {
		return nil
	}
	out := make([]string, len(in))
	copy(out, in)
	return out
}
