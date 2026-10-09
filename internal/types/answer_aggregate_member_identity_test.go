package types

import (
	"reflect"
	"strconv"
	"strings"
	"testing"
)

func TestAggregateMemberIdentityRequiresPositiveSharedIdentity(t *testing.T) {
	for _, tc := range []struct {
		name          string
		members, refs []string
		want          int
	}{
		{"unknown_qualifiers", []string{"Worker (background)", "Worker (supporting)"}, nil, 2},
		{"tenants_same_citation", []string{"Cache (tenant alpha)", "Cache (tenant beta)"}, []string{"catalog.txt:7", "catalog.txt:7"}, 2},
		{"versions_same_citation", []string{"Protocol (v1)", "Protocol (v2)"}, []string{"catalog.txt:7", "catalog.txt:7"}, 2},
		{"case_sensitive", []string{"Cache", "cache"}, []string{"catalog.txt:7", "catalog.txt:7"}, 2},
		{"conflicting_namespace", []string{"left::Load", "right::Load"}, []string{"src/a.cc:7", "src/a.cc:7"}, 2},
		{"one_sided_location", []string{"Load", "Load"}, []string{"src/a.cc:7"}, 2},
		{"basename_not_identity", []string{"Load", "Load"}, []string{"a.cc:7", "src/a.cc:7"}, 2},
		{"source_case_sensitive", []string{"Load", "Load"}, []string{"src/A.cc:7", "src/a.cc:7"}, 2},
		{"distinct_artifacts_equal_timestamp", []string{"poll (ts=1.250)", "poll (ts=1.250)"}, []string{"a.trace:9", "b.trace:9"}, 2},
		{"opaque_revisions", []string{"Load", "Load"}, []string{"blob:a#g1", "blob:a#g2"}, 2},
		{"inline_location_opaque_revisions", []string{"Load @ src/a.cc:7", "Load @ src/a.cc:7"}, []string{"blob:a#g1", "blob:a#g2"}, 2},
		{"opaque_revision_distinct_inline_locations", []string{"Load @ src/a.cc:7", "Load @ src/b.cc:7"}, []string{"blob:a#g1", "blob:a#g1"}, 2},
		{"inline_location_one_opaque_ref", []string{"Load @ src/a.cc:7", "Load @ src/a.cc:7"}, []string{"blob:a#g1", "Load @ src/a.cc:7"}, 2},
		{"qualified_unproven_relation", []string{"module → Load", "module → Class.Load"}, nil, 2},
		{"exact_duplicate", []string{"Load", "Load"}, []string{"src/a.cc:7", "src/a.cc:7"}, 1},
		{"same_full_coordinate_alias", []string{"module → Load", "module → Class.Load"}, []string{"src/a.cc:7", "src/a.cc:7"}, 1},
		{"same_literal_format", []string{"`Load`", "Load"}, nil, 1},
		{"same_relation_format", []string{"module → Load", "module/Load"}, nil, 1},
	} {
		t.Run(tc.name, func(t *testing.T) {
			fact := AnswerAggregateFact{Kind: AnswerAggregateMemberSet, Label: "objects", Value: "2", Members: tc.members, SupportRefs: tc.refs, MemberNotes: []string{"first note", "second note"}}
			got, err := NormalizeAnswerAggregateFacts([]AnswerAggregateFact{fact})
			if err != nil || len(got) != 1 || len(got[0].Members) != tc.want || got[0].Value != strconv.Itoa(tc.want) {
				t.Fatalf("normalize=%+v err=%v, want %d", got, err, tc.want)
			}
			if tc.want == 2 {
				if !reflect.DeepEqual(got[0].SupportRefs, tc.refs) || !reflect.DeepEqual(got[0].MemberNotes, fact.MemberNotes) {
					t.Fatalf("metadata misaligned: %+v", got[0])
				}
			} else if !strings.Contains(got[0].MemberNotes[0], "first note") || !strings.Contains(got[0].MemberNotes[0], "second note") {
				t.Fatalf("duplicate notes lost: %+v", got[0])
			}
			if next := NormalizeAnswerAggregateMemberSetSurfaces(got); !reflect.DeepEqual(next, got) {
				t.Fatalf("not idempotent: %+v -> %+v", got, next)
			}
			left, right := fact, fact
			left.Value, right.Value = "1", "1"
			left.Members, right.Members = tc.members[:1], tc.members[1:]
			left.MemberNotes, right.MemberNotes = fact.MemberNotes[:1], fact.MemberNotes[1:]
			left.SupportRefs, right.SupportRefs = nil, nil
			if len(tc.refs) > 0 {
				left.SupportRefs = tc.refs[:1]
			}
			if len(tc.refs) > 1 {
				right.SupportRefs = tc.refs[1:]
			}
			merged := MergeAnswerAggregateFacts([]AnswerAggregateFact{left}, []AnswerAggregateFact{right})
			if len(merged) != 1 || len(merged[0].Members) != tc.want || merged[0].Value != strconv.Itoa(tc.want) {
				t.Fatalf("merge=%+v, want %d", merged, tc.want)
			}
			if tc.want == 1 && (len(merged[0].MemberNotes) != 1 || !strings.Contains(merged[0].MemberNotes[0], "first note") || !strings.Contains(merged[0].MemberNotes[0], "second note")) {
				t.Fatalf("cross-completion merge lost duplicate's aligned note: %+v", merged)
			}
		})
	}
}

func TestAggregateMemberIdentityShortAliasCannotBridgeDistinctQualifiedObjects(t *testing.T) {
	identities := []string{"module → Left.Load", "module → Load", "module → Right.Load"}
	for _, order := range [][3]int{{0, 1, 2}, {0, 2, 1}, {1, 0, 2}, {1, 2, 0}, {2, 0, 1}, {2, 1, 0}} {
		members := []string{identities[order[0]], identities[order[1]], identities[order[2]]}
		fact := AnswerAggregateFact{Kind: AnswerAggregateMemberSet, Label: "objects", Value: "3", Members: members,
			SupportRefs: []string{"src/a.cc:7", "src/a.cc:7", "src/a.cc:7"}, MemberNotes: []string{"first note", "second note", "third note"}}
		check := func(stage string, got []AnswerAggregateFact) {
			t.Helper()
			if len(got) != 1 || got[0].Value != "2" || len(got[0].Members) != 2 || !stringSliceContains(got[0].Members, identities[0]) || !stringSliceContains(got[0].Members, identities[2]) {
				t.Fatalf("%s %v: short alias bridged distinct full identities: %+v", stage, order, got)
			}
			for _, note := range fact.MemberNotes {
				if !strings.Contains(strings.Join(got[0].MemberNotes, "\n"), note) {
					t.Fatalf("%s: note lost: %+v", stage, got)
				}
			}
		}
		got, err := NormalizeAnswerAggregateFacts([]AnswerAggregateFact{fact})
		if err != nil {
			t.Fatal(err)
		}
		check("normalize", got)
		check("repeat", NormalizeAnswerAggregateMemberSetSurfaces(got))
		var merged []AnswerAggregateFact
		for i, member := range members {
			one := AnswerAggregateFact{Kind: AnswerAggregateMemberSet, Label: fact.Label, Value: "1", Members: []string{member}, SupportRefs: fact.SupportRefs[i : i+1], MemberNotes: fact.MemberNotes[i : i+1]}
			merged = MergeAnswerAggregateFacts(merged, []AnswerAggregateFact{one})
		}
		check("cross completion", merged)
	}
}

func TestAggregateMemberFactIdentityIncludesSourceAndQualifiedObject(t *testing.T) {
	base := AnswerAggregateFact{Kind: AnswerAggregateMemberSet, Label: "objects", Value: "1", Members: []string{"Load"}, SupportRefs: []string{"src/a.cc:7"}}
	for _, other := range []AnswerAggregateFact{
		{Kind: AnswerAggregateMemberSet, Label: "objects", Value: "1", Members: []string{"Load"}, SupportRefs: []string{"src/b.cc:7"}},
		{Kind: AnswerAggregateMemberSet, Label: "objects", Value: "1", Members: []string{"Load (tenant alpha)"}, SupportRefs: []string{"src/a.cc:7"}},
		{Kind: AnswerAggregateMemberSet, Label: "objects", Value: "1", Members: []string{"Load"}},
	} {
		if AnswerAggregateFactIdentity(base) == AnswerAggregateFactIdentity(other) {
			t.Fatalf("distinct identities collide: %+v %+v", base, other)
		}
		got, err := NormalizeAnswerAggregateFacts([]AnswerAggregateFact{base, other})
		if err != nil || len(got) != 2 {
			t.Fatalf("whole-fact dedupe erased source/object: %+v %v", got, err)
		}
	}
}

func TestAggregateMemberIdentityDoesNotPromoteSuffixAcrossNormalization(t *testing.T) {
	for _, memberPath := range []string{"a.cc", "src/A.cc"} {
		facts := []AnswerAggregateFact{{Kind: AnswerAggregateMemberSet, Label: "objects", Value: "2",
			Members:     []string{"Load @ " + memberPath + ":7", "Load"},
			SupportRefs: []string{"src/a.cc:7", "src/a.cc:7"},
			MemberNotes: []string{"member-owned coordinate", "full coordinate"},
		}}
		got, err := NormalizeAnswerAggregateFacts(facts)
		if err != nil || len(got) != 1 || len(got[0].Members) != 2 {
			t.Fatalf("first boundary: %+v %v", got, err)
		}
		next := NormalizeAnswerAggregateMemberSetSurfaces(got)
		if !reflect.DeepEqual(next, got) {
			t.Fatalf("normalization promoted ambiguous source path: %+v -> %+v", got, next)
		}
	}
}
