package types

import "testing"

func TestSourceInventoryAdvisory_CloneAndMergePreservesAttributes(t *testing.T) {
	prior := SourceInventoryAdvisory{
		Active:       true,
		AdvisoryOnly: true,
		Complete:     true,
		Sets: []SourceInventoryAdvisorySet{{
			Role:     AnswerCandidateRolePackage,
			Complete: true,
			Candidates: []SourceInventoryAdvisoryCandidate{{
				Member: "alpha",
				Key:    "src/alpha",
				Role:   AnswerCandidateRolePackage,
				File:   "src/alpha",
				Attributes: []SourceInventoryAdvisoryAttribute{{
					Member: "run_alpha",
					Key:    "run_alpha",
					Role:   AnswerCandidateRoleFunction,
					File:   "src/alpha/a.py",
					Line:   7,
				}},
			}},
		}},
	}
	cloned := CloneSourceInventoryAdvisory(prior)
	cloned.Sets[0].Candidates[0].Attributes[0].Member = "mutated"
	if got := prior.Sets[0].Candidates[0].Attributes[0].Member; got != "run_alpha" {
		t.Fatalf("clone leaked attribute mutation into source: %q", got)
	}

	current := SourceInventoryAdvisory{
		Active:       true,
		AdvisoryOnly: true,
		Complete:     true,
		Sets: []SourceInventoryAdvisorySet{{
			Role:     AnswerCandidateRolePackage,
			Complete: true,
			Candidates: []SourceInventoryAdvisoryCandidate{{
				Member: "alpha",
				Key:    "src/alpha",
				Role:   AnswerCandidateRolePackage,
				File:   "src/alpha",
				Attributes: []SourceInventoryAdvisoryAttribute{{
					Member: "build_alpha",
					Key:    "build_alpha",
					Role:   AnswerCandidateRoleFunction,
					File:   "src/alpha/build.py",
					Line:   11,
				}},
			}},
		}},
	}
	merged := MergeSourceInventoryAdvisory(prior, current)
	if len(merged.Sets) != 1 || len(merged.Sets[0].Candidates) != 1 {
		t.Fatalf("merged candidate shape = %+v", merged)
	}
	attrs := merged.Sets[0].Candidates[0].Attributes
	if len(attrs) != 2 {
		t.Fatalf("merged attributes len = %d, want 2: %+v", len(attrs), attrs)
	}
	if attrs[0].Member != "run_alpha" || attrs[1].Member != "build_alpha" {
		t.Fatalf("merged attributes not preserved in order: %+v", attrs)
	}
}

func TestSourceInventoryAdvisoryRoleLabel_PackageIsScopeCarrier(t *testing.T) {
	if got := SourceInventoryAdvisoryRoleLabel(AnswerCandidateRolePackage); got != "package/directory/module scope" {
		t.Fatalf("package advisory label = %q", got)
	}
	if got := SourceInventoryAdvisoryRoleLabel(AnswerCandidateRoleFunction); got != "function" {
		t.Fatalf("function advisory label = %q", got)
	}
}

func TestMutableState_ClaimSourceInventoryAdvisoryHintOncePerLifecycle(t *testing.T) {
	mut := NewMutableState("source inventory")
	if mut.ClaimSourceInventoryAdvisoryHint() {
		t.Fatal("inactive advisory should not claim a hint")
	}
	active := SourceInventoryAdvisory{
		Active:       true,
		AdvisoryOnly: true,
		Sets: []SourceInventoryAdvisorySet{{
			Role: AnswerCandidateRolePackage,
			Candidates: []SourceInventoryAdvisoryCandidate{{
				Member: "alpha",
				Key:    "src/alpha",
			}},
		}},
	}
	mut.SetSourceInventoryAdvisory(active)
	if !mut.ClaimSourceInventoryAdvisoryHint() {
		t.Fatal("first active advisory hint claim should succeed")
	}
	if mut.ClaimSourceInventoryAdvisoryHint() {
		t.Fatal("second active advisory hint claim should be suppressed")
	}
	mut.SetSourceInventoryAdvisory(active)
	if mut.ClaimSourceInventoryAdvisoryHint() {
		t.Fatal("refreshing the same active advisory should preserve suppression")
	}
	mut.ClearSourceInventoryAdvisory()
	mut.SetSourceInventoryAdvisory(active)
	if !mut.ClaimSourceInventoryAdvisoryHint() {
		t.Fatal("fresh advisory lifecycle should allow one hint again")
	}
}
