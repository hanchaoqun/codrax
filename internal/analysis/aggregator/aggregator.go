// Package aggregator is the Session 11 F2 RootCauseAggregator. It
// consumes the F1 ViolationLedger (EvidenceClosure.Violations),
// groups events by SuspectedRoot.IRField, and produces a per-field
// heat score the F3 IRPatchEngine uses to decide whether to
// reconcile the upstream IR.
//
// The aggregator is deliberately stateless: every call walks the
// full ledger and recomputes from scratch. Operators call Aggregate
// once per explore window close; the result is ephemeral and not
// written back to the closure (the closure owns event history; the
// aggregator owns the *interpretation* of that history).
//
// Design notes (full-design §2.1):
//   - WeightedScore = AvgConfidence × saturation(EventCount/min).
//     Saturation caps contribution from runaway event streams so a
//     single field logging 20 low-confidence events does not beat a
//     field logging 3 high-confidence ones.
//   - Events with empty SuspectedRoot.IRField are filtered out (see
//     AppendViolation's backward-compat note in evidence_closure.go).
//   - The Classify action table drives the F3 patch decision:
//     RequestIRPatch when the score crosses the patch threshold,
//     HintEnrich for lower-but-non-trivial scores, Ignore otherwise.
package aggregator

import (
	"sort"

	"github.com/hanchaoqun/codrax/internal/types"
)

// FieldHeat describes the aggregated view of one IRField across
// the ledger. Consumers sort by WeightedScore to find the most
// likely root cause.
type FieldHeat struct {
	IRField       string
	EventCount    int
	AvgConfidence float64
	WeightedScore float64
	MaxConfidence float64
	TopReason     string            // highest-confidence Reason
	Events        []types.Violation // defensive copy
}

// AggregationAction is the classification result for a FieldHeat.
type AggregationAction int

const (
	// ActionIgnore — below every threshold; do nothing.
	ActionIgnore AggregationAction = iota

	// ActionHintEnrich — cross the hint threshold; the F4
	// HintComposer should mention the heat as a "same-field
	// violation seen N times" enrichment, but no IR patch yet.
	ActionHintEnrich

	// ActionRequestIRPatch — cross the patch threshold; the F3
	// IRPatchEngine should attempt to reconcile the named field.
	ActionRequestIRPatch
)

// String returns a stable label for logs.
func (a AggregationAction) String() string {
	switch a {
	case ActionIgnore:
		return "ignore"
	case ActionHintEnrich:
		return "hint_enrich"
	case ActionRequestIRPatch:
		return "request_ir_patch"
	}
	return "unknown"
}

// Config controls aggregator thresholds. Zero-value-safe:
// DefaultConfig() returns the full-design §2.1 values.
type Config struct {
	// MinConfidence filters ledger events before aggregation.
	// Events with SuspectedRoot.Confidence < this are excluded
	// from field heat computation. Default 0.5.
	MinConfidence float64

	// PatchThreshold: WeightedScore ≥ this → ActionRequestIRPatch.
	// Default 0.70.
	PatchThreshold float64

	// HintThreshold: WeightedScore ≥ this → ActionHintEnrich.
	// Default 0.50.
	HintThreshold float64

	// MinEventCount: events per field must meet this to be a
	// patch candidate. Default 3.
	MinEventCount int
}

// DefaultConfig returns the full-design §2.1 default thresholds.
func DefaultConfig() Config {
	return Config{
		MinConfidence:  0.5,
		PatchThreshold: 0.70,
		HintThreshold:  0.50,
		MinEventCount:  3,
	}
}

// Aggregator runs the grouping + scoring pipeline.
type Aggregator struct {
	cfg Config
}

// New constructs an Aggregator; missing cfg fields fall back to
// DefaultConfig values so a partial override is safe.
func New(cfg Config) *Aggregator {
	def := DefaultConfig()
	if cfg.MinConfidence <= 0 {
		cfg.MinConfidence = def.MinConfidence
	}
	if cfg.PatchThreshold <= 0 {
		cfg.PatchThreshold = def.PatchThreshold
	}
	if cfg.HintThreshold <= 0 {
		cfg.HintThreshold = def.HintThreshold
	}
	if cfg.MinEventCount <= 0 {
		cfg.MinEventCount = def.MinEventCount
	}
	return &Aggregator{cfg: cfg}
}

// Aggregate walks the ledger on closure and returns FieldHeat
// entries sorted by WeightedScore descending. Nil-safe: returns
// nil when closure is nil or empty.
func (a *Aggregator) Aggregate(closure *types.EvidenceClosure) []FieldHeat {
	if a == nil || closure == nil {
		return nil
	}
	violations := closure.Violations()
	if len(violations) == 0 {
		return nil
	}
	// Bucket events by IRField, skipping zero-field entries and
	// sub-threshold confidence.
	buckets := make(map[string][]types.Violation)
	for _, v := range violations {
		if v.SuspectedRoot.IRField == "" {
			continue
		}
		if v.SuspectedRoot.Confidence < a.cfg.MinConfidence {
			continue
		}
		buckets[v.SuspectedRoot.IRField] = append(buckets[v.SuspectedRoot.IRField], v)
	}
	var heats []FieldHeat
	for field, events := range buckets {
		sumConf := 0.0
		maxConf := 0.0
		topReason := ""
		for _, ev := range events {
			sumConf += ev.SuspectedRoot.Confidence
			if ev.SuspectedRoot.Confidence > maxConf {
				maxConf = ev.SuspectedRoot.Confidence
				topReason = ev.SuspectedRoot.Reason
			}
		}
		count := len(events)
		avg := sumConf / float64(count)
		// Saturate event-count contribution so one field with 20
		// events can't drown out a field with 5 — count caps at
		// MinEventCount×1.0 equivalent.
		sat := float64(count) / float64(a.cfg.MinEventCount)
		if sat > 1.0 {
			sat = 1.0
		}
		weighted := avg * sat
		heats = append(heats, FieldHeat{
			IRField:       field,
			EventCount:    count,
			AvgConfidence: avg,
			MaxConfidence: maxConf,
			WeightedScore: weighted,
			TopReason:     topReason,
			Events:        append([]types.Violation(nil), events...),
		})
	}
	// Sort by WeightedScore descending; ties broken by EventCount
	// then by IRField for stability.
	sort.Slice(heats, func(i, j int) bool {
		if heats[i].WeightedScore != heats[j].WeightedScore {
			return heats[i].WeightedScore > heats[j].WeightedScore
		}
		if heats[i].EventCount != heats[j].EventCount {
			return heats[i].EventCount > heats[j].EventCount
		}
		return heats[i].IRField < heats[j].IRField
	})
	return heats
}

// Classify returns the action to take for heat given the
// aggregator's thresholds. Separating this from Aggregate lets
// callers compute the full FieldHeat list once and then decide
// per-entry whether to emit patch requests vs hint enrichments.
func (a *Aggregator) Classify(heat FieldHeat) AggregationAction {
	if a == nil {
		return ActionIgnore
	}
	if heat.WeightedScore >= a.cfg.PatchThreshold && heat.EventCount >= a.cfg.MinEventCount {
		return ActionRequestIRPatch
	}
	if heat.WeightedScore >= a.cfg.HintThreshold {
		return ActionHintEnrich
	}
	return ActionIgnore
}
