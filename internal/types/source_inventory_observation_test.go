package types

import "testing"

func TestSourceInventoryObservationFromAdvisory_NormalizesCountAndAmbiguity(t *testing.T) {
	advisory := SourceInventoryAdvisory{
		Active:       true,
		AdvisoryOnly: true,
		Complete:     true,
		Scopes:       []string{"src"},
		Provenance:   []string{"repomap_graph"},
		Sets: []SourceInventoryAdvisorySet{{
			Role:     AnswerCandidateRolePackage,
			Complete: true,
			Candidates: []SourceInventoryAdvisoryCandidate{{
				Member:     "alpha",
				Key:        "src/alpha",
				SupportRef: "src/alpha",
				File:       "src/alpha",
				Language:   "python",
				Attributes: []SourceInventoryAdvisoryAttribute{
					{Member: "run_alpha", Key: "run_alpha", SupportRef: "run_alpha: src/alpha/a.py:7", Role: AnswerCandidateRoleFunction, File: "src/alpha/a.py", Line: 7},
					{Member: "build_alpha", Key: "build_alpha", SupportRef: "build_alpha: src/alpha/build.py:11", Role: AnswerCandidateRoleFunction, File: "src/alpha/build.py", Line: 11},
				},
			}},
		}},
	}

	got := SourceInventoryObservationFromAdvisory(advisory)
	if !got.IsActive() || !got.AdvisoryOnly || !got.Complete {
		t.Fatalf("observation flags = %+v", got)
	}
	if len(got.Sets) != 1 || got.Sets[0].Count != 1 || len(got.Sets[0].Members) != 1 {
		t.Fatalf("observation set/count = %+v", got.Sets)
	}
	member := got.Sets[0].Members[0]
	if member.CoverageState != SourceInventoryCoverageObserved {
		t.Fatalf("member coverage = %q", member.CoverageState)
	}
	if len(member.Attributes) != 2 {
		t.Fatalf("attributes = %+v", member.Attributes)
	}
	for _, attr := range member.Attributes {
		if attr.CoverageState != SourceInventoryCoverageAmbiguous ||
			attr.Ambiguity != "one_of_many_candidate_attributes" {
			t.Fatalf("attribute ambiguity not preserved: %+v", attr)
		}
	}
}

func TestSourceInventoryObservation_CloneAndMergePreservesCountInvariant(t *testing.T) {
	prior := SourceInventoryObservation{
		Active:       true,
		AdvisoryOnly: true,
		Complete:     true,
		Lens:         []string{"members"},
		Sets: []SourceInventoryObservationSet{{
			Role:     AnswerCandidateRoleFunction,
			Complete: true,
			Count:    99,
			Members: []SourceInventoryObservationMember{{
				Name: "Run",
				Key:  "Run",
				Role: AnswerCandidateRoleFunction,
				File: "src/run.py",
				Line: 7,
			}},
		}},
	}
	current := SourceInventoryObservation{
		Active:       true,
		AdvisoryOnly: true,
		Complete:     true,
		Lens:         []string{"count"},
		Sets: []SourceInventoryObservationSet{{
			Role:     AnswerCandidateRoleFunction,
			Complete: true,
			Members: []SourceInventoryObservationMember{{
				Name: "Build",
				Key:  "Build",
				Role: AnswerCandidateRoleFunction,
				File: "src/build.py",
				Line: 11,
			}},
		}},
	}

	cloned := CloneSourceInventoryObservation(prior)
	if cloned.Sets[0].Count != 1 {
		t.Fatalf("clone did not normalize count: %+v", cloned.Sets[0])
	}
	merged := MergeSourceInventoryObservation(prior, current)
	if len(merged.Sets) != 1 || merged.Sets[0].Count != 2 || len(merged.Sets[0].Members) != 2 {
		t.Fatalf("merge set/count = %+v", merged.Sets)
	}
	if len(merged.Lens) != 2 || merged.Lens[0] != "members" || merged.Lens[1] != "count" {
		t.Fatalf("merge lens order = %+v", merged.Lens)
	}
}

func TestMutableState_SourceInventoryAdvisoryMaintainsObservation(t *testing.T) {
	mut := NewMutableState("source inventory")
	mut.SetSourceInventoryAdvisory(SourceInventoryAdvisory{
		Active: true,
		Sets: []SourceInventoryAdvisorySet{{
			Role: AnswerCandidateRoleFunction,
			Candidates: []SourceInventoryAdvisoryCandidate{{
				Member:     "Run",
				Key:        "Run",
				SupportRef: "Run: src/run.py:7",
				Role:       AnswerCandidateRoleFunction,
				File:       "src/run.py",
				Line:       7,
			}},
		}},
	})
	if got := mut.SourceInventoryObservation(); !got.IsActive() ||
		len(got.Sets) != 1 || got.Sets[0].Count != 1 ||
		got.Sets[0].Members[0].Name != "Run" {
		t.Fatalf("observation not maintained with advisory: %+v", got)
	}
	mut.ClearSourceInventoryAdvisory()
	if got := mut.SourceInventoryObservation(); got.IsActive() {
		t.Fatalf("clear advisory should clear observation: %+v", got)
	}
}

func TestTurnAArtifacts_SourceInventoryObservationBackfilledFromAdvisory(t *testing.T) {
	mut := NewMutableState("source inventory")
	mut.SetTurnAArtifacts(TurnAArtifacts{
		SourceInventoryAdvisory: SourceInventoryAdvisory{
			Active: true,
			Sets: []SourceInventoryAdvisorySet{{
				Role: AnswerCandidateRoleFile,
				Candidates: []SourceInventoryAdvisoryCandidate{{
					Member:     "src/run.py",
					Key:        "src/run.py",
					SupportRef: "src/run.py",
					Role:       AnswerCandidateRoleFile,
					File:       "src/run.py",
					Language:   "python",
				}},
			}},
		},
	})
	got := mut.TurnAArtifacts()
	if got == nil || !got.SourceInventoryObservation.IsActive() {
		t.Fatalf("turn A observation not backfilled: %+v", got)
	}
	if got.SourceInventoryObservation.Sets[0].Count != 1 ||
		got.SourceInventoryObservation.Sets[0].Members[0].Name != "src/run.py" {
		t.Fatalf("turn A observation = %+v", got.SourceInventoryObservation)
	}
}
