package aggregator

import (
	"testing"

	"github.com/hanchaoqun/codrax/internal/types"
)

func buildClosure(events ...types.Violation) *types.EvidenceClosure {
	c := types.NewEvidenceClosure("")
	for _, v := range events {
		c.AppendViolation(v)
	}
	return c
}

func TestAggregate_GroupsByField(t *testing.T) {
	a := New(DefaultConfig())
	c := buildClosure(
		types.Violation{Kind: types.ViolFamilyMismatch, SuspectedRoot: types.SuspectedRoot{IRField: "question_kind", Confidence: 0.8}},
		types.Violation{Kind: types.ViolFamilyMismatch, SuspectedRoot: types.SuspectedRoot{IRField: "question_kind", Confidence: 0.9}},
		types.Violation{Kind: types.ViolFamilyMismatch, SuspectedRoot: types.SuspectedRoot{IRField: "question_kind", Confidence: 0.85}},
		types.Violation{Kind: types.ViolCitation, SuspectedRoot: types.SuspectedRoot{IRField: "ScannedSet", Confidence: 0.9}},
	)
	heats := a.Aggregate(c)
	if len(heats) != 2 {
		t.Fatalf("expected 2 field buckets, got %d", len(heats))
	}
	if heats[0].IRField != "question_kind" {
		t.Errorf("expected question_kind to be top by weighted score, got %s", heats[0].IRField)
	}
	if heats[0].EventCount != 3 {
		t.Errorf("question_kind EventCount = %d, want 3", heats[0].EventCount)
	}
}

func TestAggregate_FiltersLowConfidence(t *testing.T) {
	a := New(Config{MinConfidence: 0.7, PatchThreshold: 0.70, HintThreshold: 0.50, MinEventCount: 3})
	c := buildClosure(
		types.Violation{Kind: types.ViolFamilyMismatch, SuspectedRoot: types.SuspectedRoot{IRField: "X", Confidence: 0.5}},
		types.Violation{Kind: types.ViolFamilyMismatch, SuspectedRoot: types.SuspectedRoot{IRField: "X", Confidence: 0.6}},
		types.Violation{Kind: types.ViolFamilyMismatch, SuspectedRoot: types.SuspectedRoot{IRField: "Y", Confidence: 0.8}}, // keeps
	)
	heats := a.Aggregate(c)
	if len(heats) != 1 || heats[0].IRField != "Y" {
		t.Errorf("expected only Y kept (above threshold), got heats=%v", heats)
	}
}

func TestAggregate_SkipsEmptyIRField(t *testing.T) {
	a := New(DefaultConfig())
	c := buildClosure(
		types.Violation{Kind: types.ViolFamilyMismatch, Detail: "no root assigned"},
		types.Violation{Kind: types.ViolFamilyMismatch, SuspectedRoot: types.SuspectedRoot{IRField: "question_kind", Confidence: 0.9}},
	)
	heats := a.Aggregate(c)
	if len(heats) != 1 || heats[0].IRField != "question_kind" {
		t.Errorf("zero-root entry should be filtered, got heats=%v", heats)
	}
}

func TestClassify_ThresholdBoundaries(t *testing.T) {
	a := New(Config{
		MinConfidence: 0.5, PatchThreshold: 0.70,
		HintThreshold: 0.50, MinEventCount: 3,
	})
	cases := []struct {
		name   string
		heat   FieldHeat
		want   AggregationAction
	}{
		{"above patch", FieldHeat{WeightedScore: 0.80, EventCount: 3}, ActionRequestIRPatch},
		{"below events", FieldHeat{WeightedScore: 0.80, EventCount: 2}, ActionHintEnrich},
		{"hint tier", FieldHeat{WeightedScore: 0.55, EventCount: 5}, ActionHintEnrich},
		{"ignore", FieldHeat{WeightedScore: 0.30, EventCount: 5}, ActionIgnore},
	}
	for _, tc := range cases {
		if got := a.Classify(tc.heat); got != tc.want {
			t.Errorf("%s: Classify = %s, want %s", tc.name, got, tc.want)
		}
	}
}

func TestAggregate_SaturationCapsEventContribution(t *testing.T) {
	a := New(Config{MinConfidence: 0.1, PatchThreshold: 0.70, HintThreshold: 0.50, MinEventCount: 3})
	// 20 events at 0.60 confidence → avg=0.60, sat=1.0, weighted=0.60
	// WITHOUT saturation a naïve sum would explode; we want weighted
	// to stay ≤ avg.
	events := make([]types.Violation, 20)
	for i := range events {
		events[i] = types.Violation{
			Kind:          types.ViolFamilyMismatch,
			SuspectedRoot: types.SuspectedRoot{IRField: "Y", Confidence: 0.6},
		}
	}
	c := buildClosure(events...)
	heats := a.Aggregate(c)
	if len(heats) != 1 {
		t.Fatal("expected 1 heat entry")
	}
	if heats[0].WeightedScore > heats[0].AvgConfidence+0.0001 {
		t.Errorf("weighted %.4f should not exceed avg %.4f", heats[0].WeightedScore, heats[0].AvgConfidence)
	}
}

func TestAggregate_NilClosureSafe(t *testing.T) {
	a := New(DefaultConfig())
	if got := a.Aggregate(nil); got != nil {
		t.Errorf("nil closure must return nil, got %v", got)
	}
}
