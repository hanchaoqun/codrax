package types

import (
	"sort"
	"strings"
)

// ConceptualTerminalResolutionConclusion is selected by the model after it
// compares one parser-grounded terminal operation with the conceptual
// destination in the current request. The system publishes exact operations
// and validates the selected pair; it never derives this conclusion from an
// identifier, request wording, or answer prose.
type ConceptualTerminalResolutionConclusion string

const (
	ConceptualTerminalResolutionUnknown                ConceptualTerminalResolutionConclusion = ""
	ConceptualTerminalResolutionDestinationSupported   ConceptualTerminalResolutionConclusion = "requested_destination_supported"
	ConceptualTerminalResolutionCurrentTerminalDiffers ConceptualTerminalResolutionConclusion = "current_terminal_differs"
	ConceptualTerminalResolutionDestinationUnproven    ConceptualTerminalResolutionConclusion = "destination_unproven"
)

func (c ConceptualTerminalResolutionConclusion) IsValid() bool {
	switch c {
	case ConceptualTerminalResolutionDestinationSupported,
		ConceptualTerminalResolutionCurrentTerminalDiffers,
		ConceptualTerminalResolutionDestinationUnproven:
		return true
	default:
		return false
	}
}

// SchemaDescription explains the model-owned conclusion choices without
// narrowing them from operation names or request prose. All exact operations
// keep all three choices; this text is guidance, never a semantic hard gate.
func (c ConceptualTerminalResolutionConclusion) SchemaDescription() string {
	switch c {
	case ConceptualTerminalResolutionDestinationSupported:
		return "Choose only when the selected exact operation itself supports the requested conceptual destination; names, comments, and layer labels are not sufficient."
	case ConceptualTerminalResolutionCurrentTerminalDiffers:
		return "Choose when the selected exact operation establishes a materially different current terminal behavior from the requested conceptual destination."
	case ConceptualTerminalResolutionDestinationUnproven:
		return "Choose when the selected exact operation does not establish either that the requested destination is supported or that a materially different terminal is proven."
	default:
		return ""
	}
}

// ConceptualTerminalResolutionRow is one exact parser-grounded operation from
// a grounded terminal candidate or an explicitly selected callable body. It
// is evidence for what that callable does, not a system classification of the
// operation's business meaning or a declaration that it is the graph leaf.
type ConceptualTerminalResolutionRow struct {
	EvidenceID         string
	TerminalCallable   string
	ExactOperation     string
	Source             string
	AllowedConclusions []ConceptualTerminalResolutionConclusion
}

// ConceptualTerminalResolutionContract is activated only by the typed
// discover_terminal endpoint profile. Rows may be empty: that
// shape still requires the model to report that the destination is unproven
// instead of silently treating a conceptual label as an implementation fact.
type ConceptualTerminalResolutionContract struct {
	Rows []ConceptualTerminalResolutionRow
}

func (c *ConceptualTerminalResolutionContract) Active() bool { return c != nil }

// AnswerConceptualTerminalResolutionReceipt is model-authored on one visible
// principal block. EvidenceID selects one schema-published operation when one
// exists; the empty-evidence form is exposed only when the contract has no
// grounded terminal operation and permits only destination_unproven.
type AnswerConceptualTerminalResolutionReceipt struct {
	EvidenceID string                                 `json:"evidence_id,omitempty"`
	Conclusion ConceptualTerminalResolutionConclusion `json:"conclusion"`
	BoundRow   ConceptualTerminalResolutionRow        `json:"-"`
	Bound      bool                                   `json:"-"`
}

func (r *AnswerConceptualTerminalResolutionReceipt) IsBound() bool {
	return r != nil && r.Bound
}

func BindConceptualTerminalResolutionReceipt(r *AnswerConceptualTerminalResolutionReceipt, contract *ConceptualTerminalResolutionContract) bool {
	if r == nil || !contract.Active() || !r.Conclusion.IsValid() {
		return false
	}
	id := strings.TrimSpace(r.EvidenceID)
	if len(contract.Rows) == 0 {
		if id != "" || r.Conclusion != ConceptualTerminalResolutionDestinationUnproven {
			return false
		}
		r.EvidenceID = ""
		r.BoundRow = ConceptualTerminalResolutionRow{}
		r.Bound = true
		return true
	}
	for _, row := range contract.Rows {
		if id == row.EvidenceID && conceptualTerminalConclusionAllowed(r.Conclusion, row.AllowedConclusions) {
			r.EvidenceID = id
			r.BoundRow = row
			r.Bound = true
			return true
		}
	}
	return false
}

// IsConceptualTerminalOperationEvidence is the single producer/caliber
// predicate for parser-grounded operations that the model may compare with a
// requested conceptual destination. Selected-callable body enrichment is as
// exact as terminal-body enrichment, but neither producer chooses the path,
// terminal, business meaning, or conclusion.
func IsConceptualTerminalOperationEvidence(item EvidenceItem) bool {
	switch strings.TrimSpace(item.Producer) {
	case EvidenceProducerRepoMapTerminalBodyCall,
		EvidenceProducerRepoMapSelectedCallableBodyCall:
		return ClaimFormOf(item) == ClaimCallEdge && item.IsCitable()
	default:
		return false
	}
}

// IsCallChainBodyEnrichmentEvidence reports parser body-call rows that enrich
// a candidate's behavior after the principal call graph has been established.
// These rows must not extend that graph or redefine which principal callable
// is a leaf.
func IsCallChainBodyEnrichmentEvidence(item EvidenceItem) bool {
	switch strings.TrimSpace(item.Producer) {
	case EvidenceProducerRepoMapTerminalBodyCall,
		EvidenceProducerRepoMapSelectedCallableBodyCall:
		return true
	default:
		return false
	}
}

// BuildConceptualTerminalResolutionRows constructs the canonical exact
// operation candidate universe shared by the prompt and receipt contract.
// It combines parser body enrichment with the incoming edge of every leaf in
// the principal typed call graph. The latter closes a legitimate timing lane:
// a deeper grounded call edge may arrive after parser body enrichment ran, but
// it is still the exact operation that made that principal branch terminal.
// Body enrichment is excluded from principal leaf calculation, so it cannot
// redefine topology. The construction reads typed evidence coordinates only
// and never interprets identifiers or business wording.
func BuildConceptualTerminalResolutionRows(evidence []EvidenceItem) []ConceptualTerminalResolutionRow {
	seen := make(map[string]bool)
	rows := make([]ConceptualTerminalResolutionRow, 0, 16)
	appendRow := func(item EvidenceItem) {
		id := strings.TrimSpace(item.ID)
		caller := strings.TrimSpace(item.Subject)
		operation := strings.TrimSpace(item.Object)
		source := strings.TrimSpace(item.DisplayLocation(true))
		coordinate := strings.ToLower(caller + "\x00" + operation + "\x00" + source)
		if id == "" || caller == "" || operation == "" || source == "" || seen[coordinate] {
			return
		}
		seen[coordinate] = true
		rows = append(rows, ConceptualTerminalResolutionRow{
			EvidenceID:       id,
			TerminalCallable: caller,
			ExactOperation:   operation,
			Source:           source,
			AllowedConclusions: []ConceptualTerminalResolutionConclusion{
				ConceptualTerminalResolutionDestinationSupported,
				ConceptualTerminalResolutionCurrentTerminalDiffers,
				ConceptualTerminalResolutionDestinationUnproven,
			},
		})
	}
	// Parser-authored body rows remain exact behavior candidates but never
	// contribute subject/object nodes to the principal topology below.
	for _, item := range evidence {
		if !IsConceptualTerminalOperationEvidence(item) {
			continue
		}
		appendRow(item)
	}
	principalSubjects := make(map[string]bool)
	for _, item := range evidence {
		if !isConceptualTerminalPrincipalCallEdge(item) {
			continue
		}
		if key := conceptualTerminalEndpointKey(item.Subject); key != "" {
			principalSubjects[key] = true
		}
	}
	for _, item := range evidence {
		if !isConceptualTerminalPrincipalCallEdge(item) {
			continue
		}
		objectKey := conceptualTerminalEndpointKey(item.Object)
		if objectKey == "" || principalSubjects[objectKey] {
			continue
		}
		appendRow(item)
	}
	sort.SliceStable(rows, func(i, j int) bool {
		if rows[i].Source != rows[j].Source {
			return rows[i].Source < rows[j].Source
		}
		if rows[i].TerminalCallable != rows[j].TerminalCallable {
			return rows[i].TerminalCallable < rows[j].TerminalCallable
		}
		if rows[i].ExactOperation != rows[j].ExactOperation {
			return rows[i].ExactOperation < rows[j].ExactOperation
		}
		return rows[i].EvidenceID < rows[j].EvidenceID
	})
	return roundRobinConceptualTerminalRows(rows, 16)
}

func isConceptualTerminalPrincipalCallEdge(item EvidenceItem) bool {
	return !IsCallChainBodyEnrichmentEvidence(item) &&
		ClaimFormOf(item) == ClaimCallEdge && item.IsCitable() &&
		strings.TrimSpace(item.ID) != "" && strings.TrimSpace(item.Subject) != "" &&
		strings.TrimSpace(item.Object) != "" && strings.TrimSpace(item.DisplayLocation(true)) != ""
}

func conceptualTerminalEndpointKey(raw string) string {
	return strings.ToLower(strings.Join(strings.Fields(strings.TrimSpace(raw)), " "))
}

// roundRobinConceptualTerminalRows prevents one utility-heavy callable from
// consuming the bounded schema before another principal leaf contributes one
// exact operation. Ordering inside each callable stays source-stable.
func roundRobinConceptualTerminalRows(rows []ConceptualTerminalResolutionRow, limit int) []ConceptualTerminalResolutionRow {
	if len(rows) <= limit || limit <= 0 {
		return rows
	}
	groups := make(map[string][]ConceptualTerminalResolutionRow)
	order := make([]string, 0)
	for _, row := range rows {
		key := conceptualTerminalEndpointKey(row.TerminalCallable)
		if _, ok := groups[key]; !ok {
			order = append(order, key)
		}
		groups[key] = append(groups[key], row)
	}
	out := make([]ConceptualTerminalResolutionRow, 0, limit)
	for depth := 0; len(out) < limit; depth++ {
		added := false
		for _, key := range order {
			if depth >= len(groups[key]) {
				continue
			}
			out = append(out, groups[key][depth])
			added = true
			if len(out) == limit {
				break
			}
		}
		if !added {
			break
		}
	}
	return out
}

// BuildConceptualTerminalResolutionContract consumes only schema-validated
// endpoint mode plus exact parser evidence. It does not read the current
// request, evidence summaries, model reasoning, or answer text.
func BuildConceptualTerminalResolutionContract(profile *CallChainEndpointProfile, evidence []EvidenceItem) *ConceptualTerminalResolutionContract {
	if profile == nil || !profile.DiscoverTerminalActive() {
		return nil
	}
	return &ConceptualTerminalResolutionContract{Rows: BuildConceptualTerminalResolutionRows(evidence)}
}

func conceptualTerminalConclusionAllowed(got ConceptualTerminalResolutionConclusion, allowed []ConceptualTerminalResolutionConclusion) bool {
	for _, candidate := range allowed {
		if got == candidate {
			return true
		}
	}
	return false
}
