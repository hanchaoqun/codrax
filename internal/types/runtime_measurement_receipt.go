package types

import "strings"

// RuntimeMeasurementView selects a producer-owned factual projection. None of
// these views proves a dependency, target wait, or root cause.
type RuntimeMeasurementView string

const (
	RuntimeMeasurementSummary      RuntimeMeasurementView = "summary"
	RuntimeMeasurementMembers      RuntimeMeasurementView = "members"
	RuntimeMeasurementTimeline     RuntimeMeasurementView = "timeline"
	RuntimeMeasurementDistribution RuntimeMeasurementView = "distribution"
)

func (v RuntimeMeasurementView) IsValid() bool {
	return v == RuntimeMeasurementSummary || v == RuntimeMeasurementMembers || v == RuntimeMeasurementTimeline || v == RuntimeMeasurementDistribution
}

// RuntimeMeasurementTable is a lossless display projection supplied by a typed
// measurement provider, never decoded from model-authored answer JSON. The
// provider owns units, unknown values, membership, source/window boundaries,
// and coverage/omission notes. Rows are already formatted, not re-computed by
// the answer renderer. ObservationID must identify the complete measurement
// scope rather than merely a display name or device name.
type RuntimeMeasurementTable struct {
	ObservationID string
	View          RuntimeMeasurementView
	Label         string
	Columns       []string
	Rows          [][]string
	Notes         []string
	// DefaultPresentation selects the producer's minimal lossless fact view.
	// It is display intent only, not completeness, source or causal authority.
	DefaultPresentation bool                         `json:"default_presentation,omitempty"`
	MemberSet           *RuntimeMeasurementMemberSet `json:"member_set,omitempty"`
	// DisplayPopulation describes the native rows matched by this one query.
	// It is not a capture roster or investigation-completion certificate.
	DisplayPopulation *RuntimeMeasurementDisplayPopulation `json:"display_population,omitempty"`
	// EventData is a native event query's field pool. Public transport alone
	// cannot grant selection authority; the decoder rechecks the inventory.
	EventData                 *RuntimeEventData `json:"event_data,omitempty"`
	nativeEventData           bool
	nativeEventSelectionKey   string
	nativeEventScopeNoteIndex int
	// Request-window exclusions are display selection, not source omissions.
	nativeEventRequestExcluded int
	coverageScope              *runtimeMeasurementCoverageScope
	// Native log query population is fully retained and every source read is
	// healthy. This private display credential never grants completion.
	nativeLogDisplayPopulation   bool
	nativeTraceDisplayPopulation bool
	// Exact whole-publication display identity, qualified by a current private
	// source receipt. Never serialized or used as member/completion authority.
	nativeFactIdentity string
}

func (t RuntimeMeasurementTable) HasCompleteNativeDisplayPopulation() bool {
	return t.IsValid() && t.View == RuntimeMeasurementMembers && len(t.Rows) > 0 &&
		(t.nativeLogDisplayPopulation || t.nativeTraceDisplayPopulation && t.DisplayPopulation.valid(len(t.Rows)))
}

// RuntimeMeasurementDisplayPopulation never supplies MemberSet authority.
// Unknown-time rows are outside this timed query population; their separate
// tables and omission notes remain visible without inventing coordinates.
type RuntimeMeasurementDisplayPopulation struct {
	QueryScopeID string   `json:"query_scope_id"`
	RowIDs       []string `json:"row_ids"`
	MatchedRows  int      `json:"matched_rows"`
	OmittedRows  int      `json:"omitted_rows"`
}

func (p *RuntimeMeasurementDisplayPopulation) valid(rows int) bool {
	if p == nil || strings.TrimSpace(p.QueryScopeID) == "" || rows <= 0 || p.MatchedRows != rows || p.OmittedRows != 0 || len(p.RowIDs) != rows {
		return false
	}
	seen := make(map[string]bool, rows)
	for _, id := range p.RowIDs {
		if strings.TrimSpace(id) == "" || seen[id] {
			return false
		}
		seen[id] = true
	}
	return true
}

func (t RuntimeMeasurementTable) IsValid() bool {
	if t.EventData != nil && !t.nativeEventData {
		return false
	}
	if strings.TrimSpace(t.ObservationID) == "" || !t.View.IsValid() || len(t.Columns) == 0 {
		return false
	}
	for _, column := range t.Columns {
		if strings.TrimSpace(column) == "" {
			return false
		}
	}
	for _, row := range t.Rows {
		if len(row) != len(t.Columns) {
			return false
		}
	}
	return true
}

func (t RuntimeMeasurementTable) Clone() RuntimeMeasurementTable {
	out := t
	out.EventData = t.EventData.Clone()
	out.MemberSet = t.MemberSet.clone()
	if t.DisplayPopulation != nil {
		population := *t.DisplayPopulation
		population.RowIDs = append([]string(nil), t.DisplayPopulation.RowIDs...)
		out.DisplayPopulation = &population
	}
	if t.coverageScope != nil {
		scope := *t.coverageScope
		out.coverageScope = &scope
	}
	out.Columns = append([]string(nil), t.Columns...)
	out.Notes = append([]string(nil), t.Notes...)
	if t.Rows != nil {
		out.Rows = make([][]string, len(t.Rows))
		for i := range t.Rows {
			out.Rows[i] = append([]string(nil), t.Rows[i]...)
		}
	}
	return out
}

type RuntimeMeasurementContract struct {
	Tables []RuntimeMeasurementTable
	// Only current request-owned native display dimensions. Not persisted in
	// provider publications and never used by investigation completion.
	displayDimensionIndices []int
	displayPopulationKeys   map[string]bool
	// Exact current-source interval annotations, never recovered from JSON.
	nativeBusinessDiagramAnnotations []NativeBusinessDiagramAnnotation
}

// Choices excludes invalid or ambiguous identifiers, including identical
// duplicate publications. Binding must never silently choose one of two rows.
func (c *RuntimeMeasurementContract) Choices() []RuntimeMeasurementTable {
	if c == nil {
		return nil
	}
	type key struct {
		id   string
		view RuntimeMeasurementView
	}
	counts := make(map[key]int, len(c.Tables))
	for _, table := range c.Tables {
		counts[key{table.ObservationID, table.View}]++
	}
	var out []RuntimeMeasurementTable
	for _, table := range c.Tables {
		if table.IsValid() && counts[key{table.ObservationID, table.View}] == 1 {
			out = append(out, table.Clone())
		}
	}
	return out
}

func (c *RuntimeMeasurementContract) Active() bool { return len(c.Choices()) > 0 }

func (c *RuntimeMeasurementContract) Clone() *RuntimeMeasurementContract {
	if c == nil {
		return nil
	}
	out := &RuntimeMeasurementContract{Tables: make([]RuntimeMeasurementTable, len(c.Tables))}
	out.displayDimensionIndices = append([]int(nil), c.displayDimensionIndices...)
	out.nativeBusinessDiagramAnnotations = cloneNativeBusinessDiagramAnnotations(c.nativeBusinessDiagramAnnotations)
	if c.displayPopulationKeys != nil {
		out.displayPopulationKeys = make(map[string]bool, len(c.displayPopulationKeys))
		for key, value := range c.displayPopulationKeys {
			out.displayPopulationKeys[key] = value
		}
	}
	for i, table := range c.Tables {
		out.Tables[i] = table.Clone()
	}
	return out
}

// AnswerRuntimeMeasurementReceipt is an optional pure selector. The system
// binds the exact published table; neither values nor a causal conclusion are
// model inputs. BoundTable stays outside the model JSON surface.
type AnswerRuntimeMeasurementReceipt struct {
	ObservationID  string                   `json:"observation_id"`
	View           RuntimeMeasurementView   `json:"view"`
	EventSelection *RuntimeEventSelection   `json:"event_selection,omitempty"`
	BoundTable     *RuntimeMeasurementTable `json:"-"`
}

func (r *AnswerRuntimeMeasurementReceipt) IsBound() bool {
	return r != nil && r.BoundTable != nil && r.BoundTable.IsValid() &&
		r.ObservationID == r.BoundTable.ObservationID && r.View == r.BoundTable.View &&
		(r.BoundTable.EventData != nil && r.BoundTable.nativeEventSelectionKey == runtimeEventSelectionKey(r.EventSelection) || r.BoundTable.EventData == nil && r.EventSelection == nil)
}

func (r *AnswerRuntimeMeasurementReceipt) Clone() *AnswerRuntimeMeasurementReceipt {
	if r == nil {
		return nil
	}
	out := *r
	out.EventSelection = r.EventSelection.Clone()
	if r.BoundTable != nil {
		table := r.BoundTable.Clone()
		out.BoundTable = &table
	}
	return &out
}

func BindRuntimeMeasurementReceipt(r *AnswerRuntimeMeasurementReceipt, c *RuntimeMeasurementContract) bool {
	if r == nil {
		return false
	}
	r.BoundTable = nil // A stale successful binding cannot survive a failed rebind.
	for _, table := range c.Choices() {
		if r.ObservationID == table.ObservationID && r.View == table.View {
			if table.EventData != nil {
				selected, ok := selectRuntimeEventTable(table, r.EventSelection)
				if !ok {
					return false
				}
				table = selected
			} else if r.EventSelection != nil {
				return false
			}
			r.BoundTable = &table
			return true
		}
	}
	return false
}
