package types

// AUTOREPAIR-1 (§29.175) types-layer pins:
//
//   件3① T2-DIMS-DEDUP-BEFORE-CAP — the dimensions cap applies to the
//     canonical (deduped, empty-dropped) set; the cap on DISTINCT dims stays
//     a hard reject with the unchanged message format.
//   件3③ T2-KIND-LEXICAL-FOLD — fixed lexical fold before the enum gate,
//     static distinctness of the folded enum, semantic aliases stay invalid,
//     and the kind reject lists the closed enum.
//   T2-VALUE-FROM-MEMBERS pin (1) — idempotence sweep: normalizing an
//     already-normalized payload is a no-op (replay / carry-forward safety).

import (
	"reflect"
	"strings"
	"testing"
)

// 件3① positive arm: 9 raw dimensions with 2 case-folded duplicates fit the
// cap after dedup — accepted with the 7 distinct dims.
func TestAggregateDimensionsDedupBeforeCapPositive(t *testing.T) {
	fact := AnswerAggregateFact{
		Kind:  AnswerAggregateScalar,
		Label: "main-thread blocked span",
		Value: "281.9",
		Unit:  "ms",
		Dimensions: []AnswerAggregateDimension{
			{Name: "scope", Value: "window"},
			{Name: "Scope", Value: "WINDOW"}, // case-folded duplicate
			{Name: "thread", Value: "UI"},
			{Name: "cpu", Value: "3"},
			{Name: "phase", Value: "doFrame"},
			{Name: "window", Value: "10.0..10.2"},
			{Name: "lane", Value: "supply"},
			{Name: "lane", Value: "supply"}, // exact duplicate
			{Name: "band", Value: "mid"},
		},
	}
	out, err := NormalizeAnswerAggregateFacts([]AnswerAggregateFact{fact})
	if err != nil {
		t.Fatalf("deduped payload within cap must accept, got: %v", err)
	}
	if len(out) != 1 || len(out[0].Dimensions) != 7 {
		t.Fatalf("expected 7 canonical dimensions, got %d", len(out[0].Dimensions))
	}
}

// 件3① mutation arm: 9 DISTINCT dimensions keep the hard reject — choosing
// which distinct dim to discard would be content authorship (Tier3) — with
// the legacy message format reporting the canonical count.
func TestAggregateDimensionsDistinctOverCapStillRejects(t *testing.T) {
	dims := make([]AnswerAggregateDimension, 0, 9)
	for _, name := range []string{"a", "b", "c", "d", "e", "f", "g", "h", "i"} {
		dims = append(dims, AnswerAggregateDimension{Name: name, Value: "v-" + name})
	}
	_, err := NormalizeAnswerAggregateFacts([]AnswerAggregateFact{{
		Kind:       AnswerAggregateScalar,
		Label:      "x",
		Value:      "1",
		Dimensions: dims,
	}})
	if err == nil {
		t.Fatalf("9 distinct dimensions must reject")
	}
	if !strings.Contains(err.Error(), "dimensions has 9 entries; max 8") {
		t.Fatalf("cap reject must keep the legacy message format, got: %v", err)
	}
}

// 件3③ fold table: representative case/separator variants land, canonical
// kinds are fold fixed-points, semantic aliases stay invalid.
func TestFoldAnswerAggregateKindLexical(t *testing.T) {
	cases := []struct {
		in   AnswerAggregateKind
		want AnswerAggregateKind
		ok   bool
	}{
		{"Member_Set", AnswerAggregateMemberSet, true},
		{"member set", AnswerAggregateMemberSet, true},
		{"negative-observation", AnswerAggregateNegativeObservation, true},
		{"TOTAL_COUNT", AnswerAggregateTotalCount, true},
		{" scalar_value ", AnswerAggregateScalar, true},
		{"count", "count", false},           // semantic alias — ambiguous, Tier3
		{"members", "members", false},       // not a kind
		{"", "", false},                     // empty stays invalid
		{"membre_set", "membre_set", false}, // edit distance is NOT a fold
	}
	for _, tc := range cases {
		got, ok := FoldAnswerAggregateKindLexical(tc.in)
		if ok != tc.ok || got != tc.want {
			t.Fatalf("fold(%q) = (%q, %v), want (%q, %v)", tc.in, got, ok, tc.want, tc.ok)
		}
	}
	for _, kind := range AllAnswerAggregateKinds() {
		got, ok := FoldAnswerAggregateKindLexical(kind)
		if !ok || got != kind {
			t.Fatalf("canonical kind %q must be a fold fixed-point, got (%q, %v)", kind, got, ok)
		}
	}
}

// 件3③ static distinctness pin: the fold is injective over the closed enum —
// no two canonical kinds collapse to the same folded token, so a fold-hit
// can never be ambiguous.
func TestFoldedAggregateKindEnumRemainsDistinct(t *testing.T) {
	seen := map[AnswerAggregateKind]AnswerAggregateKind{}
	for _, kind := range AllAnswerAggregateKinds() {
		folded, ok := FoldAnswerAggregateKindLexical(kind)
		if !ok {
			t.Fatalf("canonical kind %q must fold to itself", kind)
		}
		if prev, dup := seen[folded]; dup {
			t.Fatalf("folded enum collision: %q and %q both fold to %q", prev, kind, folded)
		}
		seen[folded] = kind
	}
}

// 件3③ mutation arm: an unfoldable kind rejects and the reject lists the
// closed enum (reflected from the declaration list, never hand-copied).
func TestAggregateKindSemanticAliasRejectListsEnum(t *testing.T) {
	_, err := NormalizeAnswerAggregateFacts([]AnswerAggregateFact{{
		Kind:  "count",
		Label: "call sites",
		Value: "4",
	}})
	if err == nil {
		t.Fatalf("semantic alias kind must reject")
	}
	msg := err.Error()
	if !strings.Contains(msg, `kind "count" is not accepted; valid kinds: `) {
		t.Fatalf("kind reject must teach the enum, got: %v", err)
	}
	for _, kind := range AllAnswerAggregateKinds() {
		if !strings.Contains(msg, string(kind)) {
			t.Fatalf("kind reject must list %q, got: %v", kind, err)
		}
	}
}

// 件3③ positive arm at the validator level: a folded kind normalizes and the
// fact is accepted with the canonical kind.
func TestAggregateKindLexicalFoldAcceptsVariant(t *testing.T) {
	out, err := NormalizeAnswerAggregateFacts([]AnswerAggregateFact{{
		Kind:    "Member_Set",
		Label:   "blocking chain principal threads",
		Value:   "2",
		Members: []string{"ThreadA-1", "ThreadB-2"},
	}})
	if err != nil {
		t.Fatalf("folded kind variant must accept, got: %v", err)
	}
	if len(out) != 1 || out[0].Kind != AnswerAggregateMemberSet {
		t.Fatalf("kind must canonicalize to member_set, got %+v", out)
	}
}

// T2-VALUE-FROM-MEMBERS pin (1) + 件4 fixed-point discipline: the validator
// is idempotent over a fixture set covering the repair arms — member_set
// value recompute, partial count member drop, dims dedup, kind fold, and the
// negative-kind backfills.
func TestNormalizeAnswerAggregateFactsIdempotent(t *testing.T) {
	fixtures := [][]AnswerAggregateFact{
		{{ // member_set value recompute ("1+" style)
			Kind:    "Member_Set",
			Label:   "handlers",
			Value:   "1+",
			Members: []string{"A", "B", "C"},
		}},
		{{ // partial count member list drop
			Kind:    AnswerAggregateTotalCount,
			Label:   "call sites",
			Value:   "5",
			Members: []string{"one", "two"},
		}},
		{{ // dims dedup
			Kind:  AnswerAggregateScalar,
			Label: "span",
			Value: "1.5",
			Unit:  "ms",
			Dimensions: []AnswerAggregateDimension{
				{Name: "scope", Value: "w"},
				{Name: "Scope", Value: "W"},
				{Name: "thread", Value: "UI"},
			},
		}},
		{{ // negative_observation backfills (value zero, result_count, scope, searched_at)
			Kind:  AnswerAggregateNegativeObservation,
			Label: "no wakeup in window",
			Dimensions: []AnswerAggregateDimension{
				{Name: "origin", Value: "runtime_artifact"},
				{Name: "target", Value: "wakeup"},
				{Name: "trace_window", Value: "10.0..10.2"},
			},
		}},
		{{ // negative_search backfills (scope from repo, searched_at)
			Kind:  AnswerAggregateNegativeSearch,
			Label: "no Activity Resume marker",
			Value: "0",
			Dimensions: []AnswerAggregateDimension{
				{Name: "repo", Value: "codrax"},
				{Name: "pattern", Value: "onResume"},
			},
		}},
	}
	for i, fixture := range fixtures {
		once, err := NormalizeAnswerAggregateFacts(fixture)
		if err != nil {
			t.Fatalf("fixture %d must normalize, got: %v", i, err)
		}
		twice, err := NormalizeAnswerAggregateFacts(once)
		if err != nil {
			t.Fatalf("fixture %d must re-normalize, got: %v", i, err)
		}
		if !reflect.DeepEqual(once, twice) {
			t.Fatalf("fixture %d must be idempotent:\n once: %+v\ntwice: %+v", i, once, twice)
		}
	}
}
