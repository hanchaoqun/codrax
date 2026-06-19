package types

import (
	"fmt"
	"strings"
)

// WriteBehaviorContract is a typed observable that the write workflow should
// preserve or satisfy. The write_analyzer emits these atoms through
// emit_write_analysis; downstream validators only check IDs/enums/coverage
// relationships and never infer contract semantics from prose.
type WriteBehaviorContract struct {
	ID          string                      `json:"id"`
	Kind        WriteBehaviorContractKind   `json:"kind"`
	Polarity    WriteBehaviorPolarity       `json:"polarity,omitempty"`
	Subject     string                      `json:"subject,omitempty"`
	Operator    WriteBehaviorOperator       `json:"operator,omitempty"`
	Expected    string                      `json:"expected,omitempty"`
	Placement   *WriteRenderedTextPlacement `json:"placement,omitempty"`
	Comparator  *WriteBehaviorComparator    `json:"comparator,omitempty"`
	EvidenceRef string                      `json:"evidence_ref,omitempty"`
	Required    bool                        `json:"required,omitempty"`
	Source      string                      `json:"source,omitempty"`
}

// WriteRenderedTextPlacement describes a line-local rendered-output placement
// obligation. It is a typed contract surface shared by Python reprs, JS CLI
// lines, Go String() output, Java toString(), and UI snapshots. System hard
// gates read this struct only; they must not infer placement from issue prose,
// model rationale, terminal narratives, or prompt text.
type WriteRenderedTextPlacement struct {
	Surface     WriteRenderedTextSurface  `json:"surface,omitempty"`
	Anchor      string                    `json:"anchor,omitempty"`
	Expected    string                    `json:"expected,omitempty"`
	Relation    WriteRenderedTextRelation `json:"relation,omitempty"`
	Delimiter   string                    `json:"delimiter,omitempty"`
	EvidenceRef string                    `json:"evidence_ref,omitempty"`
}

// WriteBehaviorComparator ties an expected behavior contract to a grounded
// reference surface that is already known to work or intentionally contrasts
// with the failing surface. It is carried as typed context so later probes can
// assert the relationship without control flow parsing issue prose.
type WriteBehaviorComparator struct {
	Subject     string                          `json:"subject,omitempty"`
	Operator    WriteBehaviorOperator           `json:"operator,omitempty"`
	Expected    string                          `json:"expected,omitempty"`
	Relation    WriteBehaviorComparatorRelation `json:"relation,omitempty"`
	EvidenceRef string                          `json:"evidence_ref,omitempty"`
}

type WriteBehaviorContractKind string

const (
	WriteBehaviorObservable    WriteBehaviorContractKind = "observable"
	WriteBehaviorException     WriteBehaviorContractKind = "exception"
	WriteBehaviorOutputPath    WriteBehaviorContractKind = "output_path"
	WriteBehaviorStdout        WriteBehaviorContractKind = "stdout"
	WriteBehaviorStatusCode    WriteBehaviorContractKind = "status_code"
	WriteBehaviorFileLayout    WriteBehaviorContractKind = "file_layout"
	WriteBehaviorCommandResult WriteBehaviorContractKind = "command_result"
	WriteBehaviorInvariant     WriteBehaviorContractKind = "invariant"
)

type WriteBehaviorOperator string

const (
	WriteBehaviorOpSatisfies   WriteBehaviorOperator = "satisfies"
	WriteBehaviorOpEquals      WriteBehaviorOperator = "equals"
	WriteBehaviorOpNotEquals   WriteBehaviorOperator = "not_equals"
	WriteBehaviorOpContains    WriteBehaviorOperator = "contains"
	WriteBehaviorOpNotContains WriteBehaviorOperator = "not_contains"
	WriteBehaviorOpExists      WriteBehaviorOperator = "exists"
	WriteBehaviorOpNotExists   WriteBehaviorOperator = "not_exists"
	WriteBehaviorOpRaises      WriteBehaviorOperator = "raises"
	WriteBehaviorOpNotRaises   WriteBehaviorOperator = "not_raises"
	WriteBehaviorOpReturns     WriteBehaviorOperator = "returns"
)

type WriteBehaviorPolarity string

const (
	WriteBehaviorPolarityExpected  WriteBehaviorPolarity = "expected"
	WriteBehaviorPolarityForbidden WriteBehaviorPolarity = "forbidden"
	WriteBehaviorPolarityObserved  WriteBehaviorPolarity = "observed"
)

type WriteBehaviorComparatorRelation string

const (
	WriteBehaviorComparatorSameAs             WriteBehaviorComparatorRelation = "same_as"
	WriteBehaviorComparatorConsistentWith     WriteBehaviorComparatorRelation = "consistent_with"
	WriteBehaviorComparatorContrastsWith      WriteBehaviorComparatorRelation = "contrasts_with"
	WriteBehaviorComparatorRegressionBaseline WriteBehaviorComparatorRelation = "regression_baseline"
)

type WriteRenderedTextSurface string

const (
	WriteRenderedTextSurfaceRepr         WriteRenderedTextSurface = "repr"
	WriteRenderedTextSurfaceStdoutLine   WriteRenderedTextSurface = "stdout_line"
	WriteRenderedTextSurfaceCLILine      WriteRenderedTextSurface = "cli_line"
	WriteRenderedTextSurfaceStringer     WriteRenderedTextSurface = "stringer"
	WriteRenderedTextSurfaceUIText       WriteRenderedTextSurface = "ui_text"
	WriteRenderedTextSurfaceSnapshotText WriteRenderedTextSurface = "snapshot_text"
)

type WriteRenderedTextRelation string

const (
	WriteRenderedTextAfterAnchor               WriteRenderedTextRelation = "after_anchor"
	WriteRenderedTextBeforeAnchor              WriteRenderedTextRelation = "before_anchor"
	WriteRenderedTextSuffixBeforeDelimiter     WriteRenderedTextRelation = "suffix_before_delimiter"
	WriteRenderedTextPrefixAfterDelimiter      WriteRenderedTextRelation = "prefix_after_delimiter"
	WriteRenderedTextBetweenAnchorAndDelimiter WriteRenderedTextRelation = "between_anchor_and_delimiter"
	WriteRenderedTextSameLineContains          WriteRenderedTextRelation = "same_line_contains"
	WriteRenderedTextLineLocalNotContains      WriteRenderedTextRelation = "line_local_not_contains"
)

func IsKnownWriteBehaviorContractKind(v string) bool {
	switch WriteBehaviorContractKind(v) {
	case WriteBehaviorObservable, WriteBehaviorException, WriteBehaviorOutputPath,
		WriteBehaviorStdout, WriteBehaviorStatusCode, WriteBehaviorFileLayout,
		WriteBehaviorCommandResult, WriteBehaviorInvariant:
		return true
	default:
		return false
	}
}

func IsKnownWriteBehaviorOperator(v string) bool {
	switch WriteBehaviorOperator(v) {
	case WriteBehaviorOpSatisfies, WriteBehaviorOpEquals, WriteBehaviorOpNotEquals,
		WriteBehaviorOpContains, WriteBehaviorOpNotContains, WriteBehaviorOpExists,
		WriteBehaviorOpNotExists, WriteBehaviorOpRaises, WriteBehaviorOpNotRaises,
		WriteBehaviorOpReturns:
		return true
	default:
		return false
	}
}

func IsKnownWriteBehaviorPolarity(v string) bool {
	switch WriteBehaviorPolarity(v) {
	case WriteBehaviorPolarityExpected, WriteBehaviorPolarityForbidden, WriteBehaviorPolarityObserved:
		return true
	default:
		return false
	}
}

func IsKnownWriteBehaviorComparatorRelation(v string) bool {
	switch WriteBehaviorComparatorRelation(v) {
	case WriteBehaviorComparatorSameAs, WriteBehaviorComparatorConsistentWith,
		WriteBehaviorComparatorContrastsWith, WriteBehaviorComparatorRegressionBaseline:
		return true
	default:
		return false
	}
}

func IsKnownWriteRenderedTextSurface(v string) bool {
	switch WriteRenderedTextSurface(v) {
	case WriteRenderedTextSurfaceRepr,
		WriteRenderedTextSurfaceStdoutLine,
		WriteRenderedTextSurfaceCLILine,
		WriteRenderedTextSurfaceStringer,
		WriteRenderedTextSurfaceUIText,
		WriteRenderedTextSurfaceSnapshotText:
		return true
	default:
		return false
	}
}

func IsKnownWriteRenderedTextRelation(v string) bool {
	switch WriteRenderedTextRelation(v) {
	case WriteRenderedTextAfterAnchor,
		WriteRenderedTextBeforeAnchor,
		WriteRenderedTextSuffixBeforeDelimiter,
		WriteRenderedTextPrefixAfterDelimiter,
		WriteRenderedTextBetweenAnchorAndDelimiter,
		WriteRenderedTextSameLineContains,
		WriteRenderedTextLineLocalNotContains:
		return true
	default:
		return false
	}
}

// NormalizeWriteBehaviorContracts validates and normalizes analyzer-emitted
// contract atoms. When no structured atoms are emitted but expected_outcomes
// exist, it creates generic observable atoms that preserve the outcome text
// without attempting semantic parsing.
func NormalizeWriteBehaviorContracts(in []WriteBehaviorContract, expectedOutcomes []string) []WriteBehaviorContract {
	out := make([]WriteBehaviorContract, 0, len(in)+len(expectedOutcomes))
	seen := map[string]struct{}{}
	seenExpected := map[string]struct{}{}
	for i, c := range in {
		c.ID = strings.TrimSpace(c.ID)
		if c.ID == "" {
			c.ID = fmt.Sprintf("contract-%d", i+1)
		}
		c.Kind = WriteBehaviorContractKind(strings.ToLower(strings.TrimSpace(string(c.Kind))))
		if !IsKnownWriteBehaviorContractKind(string(c.Kind)) {
			c.Kind = WriteBehaviorObservable
		}
		c.Polarity = WriteBehaviorPolarity(strings.ToLower(strings.TrimSpace(string(c.Polarity))))
		if !IsKnownWriteBehaviorPolarity(string(c.Polarity)) {
			c.Polarity = WriteBehaviorPolarityExpected
		}
		if c.Polarity == WriteBehaviorPolarityObserved {
			c.Required = false
		} else {
			c.Required = true
		}
		c.Operator = WriteBehaviorOperator(strings.ToLower(strings.TrimSpace(string(c.Operator))))
		if !IsKnownWriteBehaviorOperator(string(c.Operator)) {
			c.Operator = WriteBehaviorOpSatisfies
		}
		c.Subject = strings.TrimSpace(c.Subject)
		c.Expected = strings.TrimSpace(c.Expected)
		c.Placement = NormalizeWriteRenderedTextPlacement(c.Placement, c.Expected)
		if c.Expected == "" && c.Placement != nil {
			c.Expected = c.Placement.Expected
		}
		c.Comparator = normalizeWriteBehaviorComparator(c.Comparator, c.Operator)
		c.EvidenceRef = strings.TrimSpace(c.EvidenceRef)
		c.Source = strings.TrimSpace(c.Source)
		if c.Source == "" {
			c.Source = "write_analyzer"
		}
		if c.Expected == "" && c.Subject == "" && c.Placement == nil {
			continue
		}
		if _, ok := seen[c.ID]; ok {
			continue
		}
		seen[c.ID] = struct{}{}
		if key := writeBehaviorContractExpectedKey(c.Expected); key != "" {
			seenExpected[key] = struct{}{}
		}
		out = append(out, c)
	}
	for i, outcome := range expectedOutcomes {
		outcome = strings.TrimSpace(outcome)
		if outcome == "" {
			continue
		}
		if key := writeBehaviorContractExpectedKey(outcome); key != "" {
			if _, ok := seenExpected[key]; ok {
				continue
			}
			seenExpected[key] = struct{}{}
		}
		id := uniqueWriteBehaviorContractID(fmt.Sprintf("outcome-%d", i+1), seen)
		seen[id] = struct{}{}
		out = append(out, WriteBehaviorContract{
			ID:       id,
			Kind:     WriteBehaviorObservable,
			Polarity: WriteBehaviorPolarityExpected,
			Operator: WriteBehaviorOpSatisfies,
			Expected: outcome,
			Required: true,
			Source:   "expected_outcome_fallback",
		})
	}
	const maxContracts = 12
	if len(out) > maxContracts {
		out = out[:maxContracts]
	}
	return out
}

func NormalizeWriteRenderedTextPlacement(in *WriteRenderedTextPlacement, fallbackExpected string) *WriteRenderedTextPlacement {
	if in == nil {
		return nil
	}
	p := *in
	p.Surface = WriteRenderedTextSurface(strings.ToLower(strings.TrimSpace(string(p.Surface))))
	if !IsKnownWriteRenderedTextSurface(string(p.Surface)) {
		p.Surface = ""
	}
	p.Anchor = strings.TrimSpace(p.Anchor)
	p.Expected = strings.TrimSpace(p.Expected)
	if p.Expected == "" {
		p.Expected = strings.TrimSpace(fallbackExpected)
	}
	p.Relation = WriteRenderedTextRelation(strings.ToLower(strings.TrimSpace(string(p.Relation))))
	if !IsKnownWriteRenderedTextRelation(string(p.Relation)) {
		p.Relation = ""
	}
	p.Delimiter = strings.TrimSpace(p.Delimiter)
	p.EvidenceRef = strings.TrimSpace(p.EvidenceRef)
	if p.Anchor == "" && p.Expected == "" && p.Relation == "" && p.Delimiter == "" && p.EvidenceRef == "" && p.Surface == "" {
		return nil
	}
	return &p
}

func normalizeWriteBehaviorComparator(in *WriteBehaviorComparator, fallbackOperator WriteBehaviorOperator) *WriteBehaviorComparator {
	if in == nil {
		return nil
	}
	c := *in
	c.Subject = strings.TrimSpace(c.Subject)
	c.Expected = strings.TrimSpace(c.Expected)
	c.EvidenceRef = strings.TrimSpace(c.EvidenceRef)
	c.Operator = WriteBehaviorOperator(strings.ToLower(strings.TrimSpace(string(c.Operator))))
	if !IsKnownWriteBehaviorOperator(string(c.Operator)) {
		if IsKnownWriteBehaviorOperator(string(fallbackOperator)) {
			c.Operator = fallbackOperator
		} else {
			c.Operator = WriteBehaviorOpSatisfies
		}
	}
	c.Relation = WriteBehaviorComparatorRelation(strings.ToLower(strings.TrimSpace(string(c.Relation))))
	if !IsKnownWriteBehaviorComparatorRelation(string(c.Relation)) {
		c.Relation = WriteBehaviorComparatorSameAs
	}
	if c.Subject == "" && c.Expected == "" && c.EvidenceRef == "" {
		return nil
	}
	return &c
}

func writeBehaviorContractExpectedKey(s string) string {
	return strings.ToLower(strings.Join(strings.Fields(strings.TrimSpace(s)), " "))
}

func uniqueWriteBehaviorContractID(base string, seen map[string]struct{}) string {
	base = strings.TrimSpace(base)
	if base == "" {
		base = "outcome"
	}
	if _, ok := seen[base]; !ok {
		return base
	}
	for i := 2; ; i++ {
		id := fmt.Sprintf("%s-%d", base, i)
		if _, ok := seen[id]; !ok {
			return id
		}
	}
}

func RequiredWriteBehaviorContractIDs(contracts []WriteBehaviorContract, includeFallback bool) map[string]struct{} {
	ids := make(map[string]struct{}, len(contracts))
	for _, c := range contracts {
		if !c.Required || strings.TrimSpace(c.ID) == "" {
			continue
		}
		if c.Polarity == WriteBehaviorPolarityObserved {
			continue
		}
		if !includeFallback && strings.TrimSpace(c.Source) == "expected_outcome_fallback" {
			continue
		}
		ids[c.ID] = struct{}{}
	}
	return ids
}

func IsHardRequiredWriteBehaviorContract(c WriteBehaviorContract) bool {
	if !c.Required || strings.TrimSpace(c.ID) == "" {
		return false
	}
	if c.Polarity == WriteBehaviorPolarityObserved {
		return false
	}
	if strings.TrimSpace(c.Source) == "expected_outcome_fallback" {
		return false
	}
	if c.Placement != nil {
		return true
	}
	switch c.Operator {
	case WriteBehaviorOpEquals, WriteBehaviorOpNotEquals,
		WriteBehaviorOpContains, WriteBehaviorOpNotContains,
		WriteBehaviorOpExists, WriteBehaviorOpNotExists,
		WriteBehaviorOpRaises, WriteBehaviorOpNotRaises,
		WriteBehaviorOpReturns:
		return true
	default:
		return false
	}
}

func IsPlacementRequiredWriteBehaviorContract(c WriteBehaviorContract) bool {
	if !IsHardRequiredWriteBehaviorContract(c) {
		return false
	}
	return c.Placement != nil
}

func HardRequiredWriteBehaviorContractIDs(contracts []WriteBehaviorContract) map[string]struct{} {
	ids := make(map[string]struct{}, len(contracts))
	for _, c := range contracts {
		if IsHardRequiredWriteBehaviorContract(c) {
			ids[c.ID] = struct{}{}
		}
	}
	return ids
}

func WriteBehaviorContractIDs(contracts []WriteBehaviorContract) map[string]struct{} {
	ids := make(map[string]struct{}, len(contracts))
	for _, c := range contracts {
		if id := strings.TrimSpace(c.ID); id != "" {
			ids[id] = struct{}{}
		}
	}
	return ids
}

func PlacementRequiredWriteBehaviorContractIDs(contracts []WriteBehaviorContract) map[string]struct{} {
	ids := make(map[string]struct{}, len(contracts))
	for _, c := range contracts {
		if IsPlacementRequiredWriteBehaviorContract(c) {
			ids[strings.TrimSpace(c.ID)] = struct{}{}
		}
	}
	return ids
}
