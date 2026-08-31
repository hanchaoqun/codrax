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

// ConceptualTerminalResolutionRow is one exact parser-grounded operation from
// a grounded call-graph leaf. It is evidence for what the current endpoint
// does, not a system classification of the operation's business meaning.
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

// BuildConceptualTerminalResolutionContract consumes only schema-validated
// endpoint mode plus exact parser evidence. It does not read the current
// request, evidence summaries, model reasoning, or answer text.
func BuildConceptualTerminalResolutionContract(profile *CallChainEndpointProfile, evidence []EvidenceItem) *ConceptualTerminalResolutionContract {
	if profile == nil || !profile.DiscoverTerminalActive() {
		return nil
	}
	seen := make(map[string]bool)
	rows := make([]ConceptualTerminalResolutionRow, 0, 8)
	for _, item := range evidence {
		if item.Producer != EvidenceProducerRepoMapTerminalBodyCall ||
			ClaimFormOf(item) != ClaimCallEdge || !item.IsCitable() {
			continue
		}
		id := strings.TrimSpace(item.ID)
		caller := strings.TrimSpace(item.Subject)
		operation := strings.TrimSpace(item.Object)
		source := strings.TrimSpace(item.DisplayLocation(true))
		coordinate := strings.ToLower(caller + "\x00" + operation + "\x00" + source)
		if id == "" || caller == "" || operation == "" || source == "" || seen[coordinate] {
			continue
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
	if len(rows) > 16 {
		rows = rows[:16]
	}
	return &ConceptualTerminalResolutionContract{Rows: rows}
}

func conceptualTerminalConclusionAllowed(got ConceptualTerminalResolutionConclusion, allowed []ConceptualTerminalResolutionConclusion) bool {
	for _, candidate := range allowed {
		if got == candidate {
			return true
		}
	}
	return false
}
