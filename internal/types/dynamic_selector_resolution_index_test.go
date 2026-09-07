package types

import (
	"bytes"
	"encoding/json"
	"fmt"
	"math/rand"
	"reflect"
	"testing"
)

// These tests compare complete public compiler results with the frozen scan,
// not with an expectation assembled from the new index's candidate buckets.
func TestB1580IndexedSelectorCompilerExactScanParity(t *testing.T) {
	tests := []struct {
		name   string
		build  func() []EvidenceItem
		entry  string
		reason DynamicSelectorResolutionRejectionReason
	}{
		{name: "complete", build: dynamicSelectorCompleteEvidence, entry: "run_pipeline"},
		{name: "unfiltered_entry", build: dynamicSelectorCompleteEvidence},
		{name: "qualified_separator_aliases", build: b1580IndexedAliasEvidence, entry: "Runner/run_pipeline"},
		{name: "multiple_selector_groups", build: b1580IndexedMultiGroupEvidence, entry: "run_pipeline"},
		{name: "callback_and_type_order", build: b1580IndexedOptionalEvidence, entry: "run_pipeline"},
	}
	conflicts := []struct {
		name   string
		index  int
		reason DynamicSelectorResolutionRejectionReason
	}{
		{"application", 0, DynamicSelectorRejectAmbiguousCandidate},
		{"binding", 1, DynamicSelectorRejectAmbiguousContainer},
		{"lookup", 2, DynamicSelectorRejectAmbiguousLookup},
		{"return", 3, DynamicSelectorRejectAmbiguousReturn},
		{"entry", 4, DynamicSelectorRejectAmbiguousEntry},
		{"argument", 5, DynamicSelectorRejectAmbiguousArgument},
	}
	for _, conflict := range conflicts {
		tests = append(tests, struct {
			name   string
			build  func() []EvidenceItem
			entry  string
			reason DynamicSelectorResolutionRejectionReason
		}{"distinct_source_occurrence_" + conflict.name, func() []EvidenceItem {
			rows := dynamicSelectorCompleteEvidence()
			other := rows[conflict.index]
			other.ID += "-other"
			// Argument joining deliberately keys source + line start only;
			// line end remains a distinct occurrence within that same callsite.
			other.LineEnd++
			return append(rows, other)
		}, "run_pipeline", conflict.reason})
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			rows := test.build()
			if test.reason != "" {
				want := b1580ScanReferenceCompileDynamicSelectorResolutionPaths(rows, test.entry)
				if len(want.Candidates) != 0 || len(want.Rejected) != 1 || want.Rejected[0].Reason != test.reason {
					t.Fatalf("test must exercise %s, got %+v", test.reason, want)
				}
			} else if got := b1580ScanReferenceCompileDynamicSelectorResolutionPaths(rows, test.entry); len(got.Candidates) == 0 {
				t.Fatalf("positive fixture did not reach complete compiler joins: %+v", got)
			}
			b1580AssertShuffledScanParity(t, rows, test.entry)
		})
	}
}

func TestB1580IndexedSelectorCompilerMissingUncitableAndIdentityBoundaries(t *testing.T) {
	for index, role := range []string{"application", "binding", "lookup", "return", "entry", "argument", "callback_call", "callback", "type"} {
		for _, mode := range []string{"missing", "uncitable", "empty_id", "empty_source"} {
			t.Run(role+"/"+mode, func(t *testing.T) {
				rows := dynamicSelectorCompleteEvidence()
				switch mode {
				case "missing":
					rows = append(rows[:index], rows[index+1:]...)
				case "uncitable":
					rows[index].GroundingStatus = GroundingUngrounded
				case "empty_id":
					rows[index].ID = ""
				case "empty_source":
					rows[index].Source = ""
				}
				b1580AssertShuffledScanParity(t, rows, "run_pipeline")
			})
		}
	}
	mutations := []struct {
		name  string
		apply func([]EvidenceItem)
		entry string
	}{
		{"short_owner_is_not_qualified_owner", func(rows []EvidenceItem) { rows[1].OwnerSymbol = "Registry.register" }, "run_pipeline"},
		{"binding_has_no_subject_fallback", func(rows []EvidenceItem) { rows[1].OwnerSymbol = "" }, "run_pipeline"},
		{"return_subject_fallback", func(rows []EvidenceItem) { rows[3].OwnerSymbol = "" }, "run_pipeline"},
		{"return_owner_beats_matching_subject", func(rows []EvidenceItem) { rows[3].OwnerSymbol = "other.resolve" }, "run_pipeline"},
		{"entry_subject_beats_qualified_owner", func(rows []EvidenceItem) { rows[4].OwnerSymbol = "pkg.run_pipeline" }, "run_pipeline"},
		{"entry_owner_fallback", func(rows []EvidenceItem) { rows[4].Subject = "" }, "run_pipeline"},
		{"invalid_nonempty_owner_not_fallback", func(rows []EvidenceItem) { rows[3].OwnerSymbol = "!!!" }, "run_pipeline"},
		{"invalid_lookup_owner", func(rows []EvidenceItem) { rows[2].OwnerSymbol = "!!!" }, "run_pipeline"},
		{"invalid_selector_owner", func(rows []EvidenceItem) { rows[0].SelectorApplication.Owner = "!!!" }, "run_pipeline"},
		{"invalid_candidate_identity", func(rows []EvidenceItem) { rows[0].Object = "!!!" }, "run_pipeline"},
		{"blank_binding_container", func(rows []EvidenceItem) { rows[1].Subject = "[name]" }, "run_pipeline"},
		{"argument_source_trim", func(rows []EvidenceItem) { rows[5].Source = "  " + rows[4].Source + "\t" }, "run_pipeline"},
		{"argument_source_separator_not_alias", func(rows []EvidenceItem) { rows[5].Source = "src\\registry.py" }, "run_pipeline"},
		{"argument_wrong_callsite", func(rows []EvidenceItem) { rows[5].LineStart++ }, "run_pipeline"},
		{"argument_empty_subject", func(rows []EvidenceItem) { rows[5].Subject = "" }, "run_pipeline"},
		{"lookup_snippet_endpoint_mismatch", func(rows []EvidenceItem) { rows[2].Object = "OTHER[name]" }, "run_pipeline"},
		{"indexed_assignment_not_registration", func(rows []EvidenceItem) { rows[1].Kind = EvidenceConcrete; rows[1].Predicate = "assigns" }, "run_pipeline"},
		{"ordinary_assignment_not_binding", func(rows []EvidenceItem) {
			rows[1].Kind, rows[1].Predicate = EvidenceConcrete, "assigns"
			rows[1].Subject, rows[1].Object, rows[1].Snippet = "cls.name", "name", "cls.name = name"
		}, "run_pipeline"},
		{"callback_owner_fallback", func(rows []EvidenceItem) { rows[6].OwnerSymbol = "" }, "run_pipeline"},
		{"callback_owner_beats_matching_subject", func(rows []EvidenceItem) { rows[6].OwnerSymbol = "other.run_pipeline" }, "run_pipeline"},
		{"callback_empty_receiver", func(rows []EvidenceItem) { rows[7].Subject = "" }, "run_pipeline"},
		{"type_short_is_not_qualified", func(rows []EvidenceItem) { rows[8].Subject = "plugins.JsonPlugin" }, "run_pipeline"},
		{"different_requested_entry", func([]EvidenceItem) {}, "other_entry"},
	}
	for _, mutation := range mutations {
		t.Run(mutation.name, func(t *testing.T) {
			rows := dynamicSelectorCompleteEvidence()
			mutation.apply(rows)
			b1580AssertShuffledScanParity(t, rows, mutation.entry)
		})
	}
	for _, rows := range [][]EvidenceItem{nil, {}, dynamicSelectorCompleteEvidence()[1:]} {
		b1580AssertShuffledScanParity(t, rows, "run_pipeline")
	}
}

func TestB1580IndexedSelectorCompilerKeepsSameOccurrenceAndOptionalOrder(t *testing.T) {
	rows := dynamicSelectorCompleteEvidence()
	assignment := rows[1]
	assignment.ID, assignment.Kind, assignment.Predicate = "E-assignment", EvidenceConcrete, "assigns"
	rows = append(rows, assignment)
	b1580AssertShuffledScanParity(t, rows, "run_pipeline")
	for seed := int64(0); seed < 8; seed++ {
		shuffled := b1580CloneSelectorEvidence(rows)
		rand.New(rand.NewSource(seed)).Shuffle(len(shuffled), func(i, j int) { shuffled[i], shuffled[j] = shuffled[j], shuffled[i] })
		got := CompileDynamicSelectorResolutionPaths(shuffled, "run_pipeline")
		if len(got.Candidates) != 1 || got.Candidates[0].Hops[3].EvidenceID != "E-bind" {
			t.Fatalf("same occurrence must prefer registration in either input order: %+v", got)
		}
	}
	optional := CompileDynamicSelectorResolutionPaths(b1580IndexedOptionalEvidence(), "run_pipeline")
	if len(optional.Candidates) != 1 {
		t.Fatalf("optional fixture must keep complete core path: %+v", optional)
	}
	var callbackIDs, typeIDs []string
	for _, hop := range optional.Candidates[0].CallbackHops {
		callbackIDs = append(callbackIDs, hop.EvidenceID)
	}
	for _, hop := range optional.Candidates[0].TypeRoster {
		typeIDs = append(typeIDs, hop.EvidenceID)
	}
	if !reflect.DeepEqual(callbackIDs, []string{"E-call-second", "E-handoff-second", "E-callback-call", "E-callback"}) ||
		!reflect.DeepEqual(typeIDs, []string{"E-type", "E-type-second", "E-type-no-ordinal"}) {
		t.Fatalf("handoff outer order / first full-census ID ordinal changed: callback=%v type=%v", callbackIDs, typeIDs)
	}
}

func b1580AssertShuffledScanParity(t *testing.T, rows []EvidenceItem, entry string) {
	t.Helper()
	for seed := int64(-1); seed < 8; seed++ {
		input := b1580CloneSelectorEvidence(rows)
		if seed >= 0 {
			rand.New(rand.NewSource(seed)).Shuffle(len(input), func(i, j int) { input[i], input[j] = input[j], input[i] })
		}
		before, err := json.Marshal(input)
		if err != nil {
			t.Fatal(err)
		}
		want := b1580ScanReferenceCompileDynamicSelectorResolutionPaths(input, entry)
		got := CompileDynamicSelectorResolutionPaths(input, entry)
		if !reflect.DeepEqual(got, want) {
			t.Fatalf("seed=%d indexed compiler differs from frozen full scan\ngot:  %+v\nwant: %+v", seed, got, want)
		}
		after, err := json.Marshal(input)
		if err != nil || !bytes.Equal(before, after) {
			t.Fatalf("compilation mutated caller-owned evidence, seed=%d err=%v", seed, err)
		}
	}
}

func b1580CloneSelectorEvidence(rows []EvidenceItem) []EvidenceItem {
	if rows == nil {
		return nil
	}
	out := append([]EvidenceItem{}, rows...)
	for i := range out {
		if out[i].SelectorApplication != nil {
			selector := *out[i].SelectorApplication
			out[i].SelectorApplication = &selector
		}
	}
	return out
}

func b1580IndexedAliasEvidence() []EvidenceItem {
	rows := dynamicSelectorCompleteEvidence()
	rows[0].SelectorApplication.Owner, rows[1].OwnerSymbol = "Inventory::register", "Inventory/register"
	rows[0].Object, rows[8].Subject = "plugins::JsonPlugin", "plugins/JsonPlugin"
	rows[1].Subject, rows[1].Snippet = "ns.REGISTRY[name]", "ns.REGISTRY[name] = cls"
	rows[2].Object, rows[2].Snippet = "ns.REGISTRY[name]", "cls = ns.REGISTRY[name]"
	rows[2].OwnerSymbol, rows[3].OwnerSymbol, rows[3].Subject = "Factory::resolve", "Factory.resolve", "wrong_subject"
	rows[4].Object, rows[5].Object = "Factory#resolve", "Factory/resolve"
	rows[4].Subject, rows[4].OwnerSymbol = "Runner->run_pipeline", "enclosing.run_pipeline"
	rows[6].Subject, rows[6].OwnerSymbol = "Runner.run_pipeline", "Runner::run_pipeline"
	rows[6].Object, rows[7].Subject = "executor::submit", "executor.submit"
	return rows
}

func b1580IndexedMultiGroupEvidence() []EvidenceItem {
	rows := dynamicSelectorCompleteEvidence()
	for i := 0; i < 6; i++ {
		application := b1580CloneSelectorEvidence(rows[:1])[0]
		application.ID = fmt.Sprintf("E-app-group-%d", i)
		application.Object = fmt.Sprintf("Plugin%d", i)
		application.SelectorApplication.Literal = fmt.Sprintf("selector-%d", 5-i)
		rows = append(rows, application)
	}
	// One group is ambiguous; all other complete groups must remain available.
	conflict := rows[len(rows)-1]
	conflict.ID, conflict.Object = "E-one-group-conflict", "OtherPlugin"
	return append(rows, conflict)
}

func b1580IndexedOptionalEvidence() []EvidenceItem {
	rows := dynamicSelectorCompleteEvidence()
	secondCall, secondHandoff, secondType, noOrdinal := rows[6], rows[7], rows[8], rows[8]
	secondCall.ID, secondCall.Object = "E-call-second", "another::executor"
	secondHandoff.ID, secondHandoff.Subject, secondHandoff.Object = "E-handoff-second", "another.executor", "plugin.close"
	secondType.ID, secondType.Object, secondType.RelationOrdinal = "E-type-second", "Mixin", 1
	noOrdinal.ID, noOrdinal.Object, noOrdinal.RelationOrdinal = "E-type-no-ordinal", "Marker", 0
	// The first same-ID row is not citable and is not a type relation, yet the
	// old stable ordering deliberately reads its ordinal from the full census.
	firstSameID := rows[4]
	firstSameID.ID, firstSameID.RelationOrdinal, firstSameID.GroundingStatus = "E-type", 1, GroundingUngrounded
	secondType.RelationOrdinal = 2
	rows[8].RelationOrdinal = 99
	wrongOwnerCall := secondCall
	wrongOwnerCall.ID, wrongOwnerCall.OwnerSymbol = "E-wrong-owner", "other_entry"
	uncitableHandoff := secondHandoff
	uncitableHandoff.ID, uncitableHandoff.GroundingStatus = "E-uncitable-handoff", GroundingUngrounded
	out := append([]EvidenceItem{secondHandoff, firstSameID, wrongOwnerCall}, rows...)
	out = append(out, secondCall, secondHandoff, rows[6], secondType, secondType, noOrdinal, uncitableHandoff)
	return out
}
