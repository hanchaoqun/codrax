package types

import (
	"fmt"
	"strings"
)

// AnswerAggregateKind names a model-emitted aggregate fact that the
// finalizer should preserve as a structured exploration result. These
// facts are authored by the investigating model through
// emit_investigation_complete; the system stores and renders them but
// does not synthesize answer values from raw evidence on its own.
type AnswerAggregateKind string

const (
	AnswerAggregateUnknown      AnswerAggregateKind = ""
	AnswerAggregateTotalCount   AnswerAggregateKind = "total_count"
	AnswerAggregateUniqueCount  AnswerAggregateKind = "unique_count"
	AnswerAggregateGroupedCount AnswerAggregateKind = "grouped_count"
	AnswerAggregateBucketCount  AnswerAggregateKind = "bucket_count"
	AnswerAggregateExcluded     AnswerAggregateKind = "excluded_count"
	AnswerAggregateScalar       AnswerAggregateKind = "scalar_value"
)

var allAnswerAggregateKinds = []AnswerAggregateKind{
	AnswerAggregateTotalCount,
	AnswerAggregateUniqueCount,
	AnswerAggregateGroupedCount,
	AnswerAggregateBucketCount,
	AnswerAggregateExcluded,
	AnswerAggregateScalar,
}

// AllAnswerAggregateKinds returns the canonical non-empty aggregate
// kind list. Returned values are safe for callers to mutate.
func AllAnswerAggregateKinds() []AnswerAggregateKind {
	return append([]AnswerAggregateKind(nil), allAnswerAggregateKinds...)
}

func (k AnswerAggregateKind) IsValid() bool {
	for _, declared := range allAnswerAggregateKinds {
		if k == declared {
			return true
		}
	}
	return false
}

// AnswerAggregateDimension is one typed axis for an aggregate fact:
// scope=production, syntax=struct_literal, bucket=runtime, language=cpp,
// or any other model-authored dimension that was explicitly verified.
type AnswerAggregateDimension struct {
	Name  string `json:"name"`
	Value string `json:"value"`
}

// AnswerAggregateFact is the structured, model-emitted handoff for
// derived counts and grouped scalar facts. It intentionally keeps Value
// as a string so version numbers, ratios, hashes, and "4" all travel
// through the same exact-preservation path.
type AnswerAggregateFact struct {
	Kind        AnswerAggregateKind        `json:"kind"`
	Label       string                     `json:"label"`
	Value       string                     `json:"value"`
	Unit        string                     `json:"unit,omitempty"`
	Dimensions  []AnswerAggregateDimension `json:"dimensions,omitempty"`
	Members     []string                   `json:"members,omitempty"`
	Excluded    []string                   `json:"excluded,omitempty"`
	SupportRefs []string                   `json:"support_refs,omitempty"`
}

const (
	maxAnswerAggregateFacts      = 16
	maxAnswerAggregateDimensions = 8
	maxAnswerAggregateMembers    = 80
	maxAnswerAggregateTextLen    = 240
)

// NormalizeAnswerAggregateFacts validates and canonicalizes aggregate
// facts emitted by the model. The checks are structural only: closed
// kind enum, required label/value, bounded list sizes, and whitespace
// trimming. They do not infer or repair values from evidence.
func NormalizeAnswerAggregateFacts(in []AnswerAggregateFact) ([]AnswerAggregateFact, error) {
	if len(in) == 0 {
		return nil, nil
	}
	if len(in) > maxAnswerAggregateFacts {
		return nil, fmt.Errorf("aggregate_facts has %d entries; max %d", len(in), maxAnswerAggregateFacts)
	}
	out := make([]AnswerAggregateFact, 0, len(in))
	seen := map[string]bool{}
	for i, raw := range in {
		fact, err := normalizeAnswerAggregateFact(raw)
		if err != nil {
			return nil, fmt.Errorf("aggregate_facts[%d]: %w", i, err)
		}
		key := strings.ToLower(string(fact.Kind)) + "\x00" +
			strings.ToLower(fact.Label) + "\x00" +
			strings.ToLower(fact.Value) + "\x00" +
			strings.ToLower(renderAggregateDimensionsKey(fact.Dimensions))
		if seen[key] {
			continue
		}
		seen[key] = true
		out = append(out, fact)
	}
	if len(out) == 0 {
		return nil, nil
	}
	return out, nil
}

func normalizeAnswerAggregateFact(raw AnswerAggregateFact) (AnswerAggregateFact, error) {
	fact := AnswerAggregateFact{
		Kind:  raw.Kind,
		Label: trimAggregateText(raw.Label),
		Value: trimAggregateText(raw.Value),
		Unit:  trimAggregateText(raw.Unit),
	}
	if !fact.Kind.IsValid() {
		return AnswerAggregateFact{}, fmt.Errorf("kind %q is not accepted", raw.Kind)
	}
	if fact.Label == "" {
		return AnswerAggregateFact{}, fmt.Errorf("label is required")
	}
	if fact.Value == "" {
		return AnswerAggregateFact{}, fmt.Errorf("value is required")
	}
	dims, err := normalizeAnswerAggregateDimensions(raw.Dimensions)
	if err != nil {
		return AnswerAggregateFact{}, err
	}
	fact.Dimensions = dims
	fact.Members = normalizeAggregateStrings(raw.Members, maxAnswerAggregateMembers)
	fact.Excluded = normalizeAggregateStrings(raw.Excluded, maxAnswerAggregateMembers)
	fact.SupportRefs = normalizeAggregateStrings(raw.SupportRefs, maxAnswerAggregateMembers)
	return fact, nil
}

func normalizeAnswerAggregateDimensions(in []AnswerAggregateDimension) ([]AnswerAggregateDimension, error) {
	if len(in) == 0 {
		return nil, nil
	}
	if len(in) > maxAnswerAggregateDimensions {
		return nil, fmt.Errorf("dimensions has %d entries; max %d", len(in), maxAnswerAggregateDimensions)
	}
	out := make([]AnswerAggregateDimension, 0, len(in))
	seen := map[string]bool{}
	for _, raw := range in {
		name := trimAggregateText(raw.Name)
		value := trimAggregateText(raw.Value)
		if name == "" || value == "" {
			continue
		}
		key := strings.ToLower(name) + "\x00" + strings.ToLower(value)
		if seen[key] {
			continue
		}
		seen[key] = true
		out = append(out, AnswerAggregateDimension{Name: name, Value: value})
	}
	if len(out) == 0 {
		return nil, nil
	}
	return out, nil
}

func normalizeAggregateStrings(in []string, limit int) []string {
	if len(in) == 0 || limit <= 0 {
		return nil
	}
	out := make([]string, 0, len(in))
	seen := map[string]bool{}
	for _, raw := range in {
		item := trimAggregateText(raw)
		if item == "" {
			continue
		}
		key := strings.ToLower(item)
		if seen[key] {
			continue
		}
		seen[key] = true
		out = append(out, item)
		if len(out) >= limit {
			break
		}
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

func trimAggregateText(s string) string {
	s = strings.TrimSpace(s)
	if s == "" {
		return ""
	}
	s = strings.Join(strings.Fields(s), " ")
	if len(s) > maxAnswerAggregateTextLen {
		s = strings.TrimSpace(s[:maxAnswerAggregateTextLen])
	}
	return s
}

func renderAggregateDimensionsKey(dims []AnswerAggregateDimension) string {
	if len(dims) == 0 {
		return ""
	}
	parts := make([]string, 0, len(dims))
	for _, d := range dims {
		if d.Name == "" || d.Value == "" {
			continue
		}
		parts = append(parts, d.Name+"="+d.Value)
	}
	return strings.Join(parts, ";")
}

func cloneAnswerAggregateFacts(in []AnswerAggregateFact) []AnswerAggregateFact {
	if len(in) == 0 {
		return nil
	}
	out := make([]AnswerAggregateFact, len(in))
	for i, fact := range in {
		out[i] = fact
		if fact.Dimensions != nil {
			out[i].Dimensions = append([]AnswerAggregateDimension(nil), fact.Dimensions...)
		}
		if fact.Members != nil {
			out[i].Members = append([]string(nil), fact.Members...)
		}
		if fact.Excluded != nil {
			out[i].Excluded = append([]string(nil), fact.Excluded...)
		}
		if fact.SupportRefs != nil {
			out[i].SupportRefs = append([]string(nil), fact.SupportRefs...)
		}
	}
	return out
}
