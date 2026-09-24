package types

import "strings"

// RuntimeMeasurementView selects a producer-owned factual projection. None of
// these views proves a dependency, target wait, or root cause.
type RuntimeMeasurementView string

const (
	RuntimeMeasurementSummary  RuntimeMeasurementView = "summary"
	RuntimeMeasurementMembers  RuntimeMeasurementView = "members"
	RuntimeMeasurementTimeline RuntimeMeasurementView = "timeline"
)

func (v RuntimeMeasurementView) IsValid() bool {
	return v == RuntimeMeasurementSummary || v == RuntimeMeasurementMembers || v == RuntimeMeasurementTimeline
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
}

func (t RuntimeMeasurementTable) IsValid() bool {
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
	for i, table := range c.Tables {
		out.Tables[i] = table.Clone()
	}
	return out
}

// AnswerRuntimeMeasurementReceipt is an optional pure selector. The system
// binds the exact published table; neither values nor a causal conclusion are
// model inputs. BoundTable stays outside the model JSON surface.
type AnswerRuntimeMeasurementReceipt struct {
	ObservationID string                   `json:"observation_id"`
	View          RuntimeMeasurementView   `json:"view"`
	BoundTable    *RuntimeMeasurementTable `json:"-"`
}

func (r *AnswerRuntimeMeasurementReceipt) IsBound() bool {
	return r != nil && r.BoundTable != nil && r.BoundTable.IsValid() &&
		r.ObservationID == r.BoundTable.ObservationID && r.View == r.BoundTable.View
}

func (r *AnswerRuntimeMeasurementReceipt) Clone() *AnswerRuntimeMeasurementReceipt {
	if r == nil {
		return nil
	}
	out := *r
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
			r.BoundTable = &table
			return true
		}
	}
	return false
}
