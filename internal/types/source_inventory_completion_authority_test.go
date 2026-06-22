package types

import "testing"

func TestSourceInventoryCompletionAuthority_BlocksWithFollowupDebt(t *testing.T) {
	obs := SourceInventoryObservation{
		Active:   true,
		Complete: false,
		Scopes:   []string{"."},
		Lens:     []string{"members", "count"},
		SourceClasses: []SourceInventorySourceClassCount{{
			Role: SourcePathRoleProduction, Count: 2, Complete: true, Samples: []string{"src/a.go"},
		}, {
			Role: SourcePathRoleThirdParty, Count: 1, Complete: true, Samples: []string{"thirdparty/corpus/a.ets"},
		}},
		Execution: &SourceInventoryExecutionState{Budgeted: true, CandidateBudgetTruncated: true},
		Sets: []SourceInventoryObservationSet{{
			Role:     AnswerCandidateRoleType,
			Complete: false,
			Members: []SourceInventoryObservationMember{{
				Name: "A",
				Role: AnswerCandidateRoleType,
				File: "src/a.go",
			}},
		}},
	}
	rm := RequestModel{SourceInventoryProfile: &SourceInventoryProfile{
		IsSourceInventory: true,
		TargetRoles:       []AnswerCandidateRole{AnswerCandidateRoleType},
	}}

	auth := BuildSourceInventoryCompletionAuthority(obs, rm, false)
	if !auth.Blocking || auth.ReasonCode != SourceInventoryCompletionReasonFollowupDebt {
		t.Fatalf("authority = %+v", auth)
	}
	if !auth.FollowupDebt.IsActive() || auth.FollowupDebt.ReasonCode != SourceInventoryFollowupDebtMissingSourceClass {
		t.Fatalf("follow-up debt = %+v", auth.FollowupDebt)
	}
	if len(auth.FollowupDebt.Query.Scopes) != 1 || auth.FollowupDebt.Query.Scopes[0] != "thirdparty/corpus" {
		t.Fatalf("follow-up scopes = %+v", auth.FollowupDebt.Query.Scopes)
	}
}

func TestSourceInventoryCompletionAuthority_AcceptedExactUniverseCanCloseBoundedScope(t *testing.T) {
	obs := SourceInventoryObservation{
		Active:   true,
		Complete: false,
		Scopes:   []string{"pkg"},
		Lens:     []string{"members"},
		Sets: []SourceInventoryObservationSet{{
			Role:     AnswerCandidateRoleFunction,
			Complete: false,
			Members:  []SourceInventoryObservationMember{{Name: "Run", Role: AnswerCandidateRoleFunction, File: "pkg/a.go"}},
		}},
	}
	rm := RequestModel{
		SourceInventoryProfile: &SourceInventoryProfile{
			IsSourceInventory: true,
			TargetRoles:       []AnswerCandidateRole{AnswerCandidateRoleFunction},
		},
		SourceScopeProfile: &SourceScopeProfile{RequestedScope: SourceScopeProduction},
	}

	auth := BuildSourceInventoryCompletionAuthority(obs, rm, true)
	if auth.Blocking || auth.ReasonCode != SourceInventoryCompletionReasonAcceptedExactUniverse {
		t.Fatalf("authority = %+v", auth)
	}
}

func TestSourceInventoryCompletionAuthority_RepoWideStillBlocksAcceptedPartial(t *testing.T) {
	obs := SourceInventoryObservation{
		Active:   true,
		Complete: false,
		Scopes:   []string{"pkg"},
		Lens:     []string{"members"},
		Sets: []SourceInventoryObservationSet{{
			Role:     AnswerCandidateRoleFunction,
			Complete: false,
			Members:  []SourceInventoryObservationMember{{Name: "Run", Role: AnswerCandidateRoleFunction, File: "pkg/a.go"}},
		}},
	}
	rm := RequestModel{SourceInventoryProfile: &SourceInventoryProfile{
		IsSourceInventory: true,
		TargetRoles:       []AnswerCandidateRole{AnswerCandidateRoleFunction},
	}}

	auth := BuildSourceInventoryCompletionAuthority(obs, rm, true)
	if !auth.Blocking || auth.ReasonCode != SourceInventoryCompletionReasonIncompleteObservation {
		t.Fatalf("repo-wide incomplete inventory must remain blocking, got %+v", auth)
	}
	if len(auth.Scopes) != 1 || auth.Scopes[0] != "." {
		t.Fatalf("repo-wide repair scope = %+v, want root", auth.Scopes)
	}
}

func TestSourceInventoryCompletionAuthority_RequestedUniverseCanCloseRepoWideDebt(t *testing.T) {
	obs := SourceInventoryObservation{
		Active:   true,
		Complete: false,
		Scopes:   []string{"."},
		Lens:     []string{"members"},
		Execution: &SourceInventoryExecutionState{
			Budgeted:                 true,
			CandidateBudgetTruncated: true,
		},
		Sets: []SourceInventoryObservationSet{{
			Role:     AnswerCandidateRoleFunction,
			Complete: false,
			Members:  []SourceInventoryObservationMember{{Name: "Run", Role: AnswerCandidateRoleFunction, File: "pkg/a.go"}},
		}},
	}
	rm := RequestModel{SourceInventoryProfile: &SourceInventoryProfile{
		IsSourceInventory: true,
		TargetRoles:       []AnswerCandidateRole{AnswerCandidateRoleFunction},
	}}

	auth := BuildSourceInventoryCompletionAuthorityWithOptions(obs, rm, SourceInventoryCompletionAuthorityOptions{
		AcceptedExactUniverse:     true,
		AcceptedRequestedUniverse: true,
	})
	if auth.Blocking || auth.ReasonCode != SourceInventoryCompletionReasonAcceptedRequestedUniverse {
		t.Fatalf("requested universe should discharge repo-wide generic debt, got %+v", auth)
	}
	if !auth.AcceptedRequestedUniverse || !auth.RepoWideRequired {
		t.Fatalf("authority should preserve requested-universe/repo-wide flags, got %+v", auth)
	}
}

func TestSourceInventoryCompletionAuthority_ValueLookupSupportProfileIsInactive(t *testing.T) {
	obs := SourceInventoryObservation{
		Active:   true,
		Complete: false,
		Scopes:   []string{"."},
		Lens:     []string{"members"},
		Sets: []SourceInventoryObservationSet{{
			Role:     AnswerCandidateRoleFunction,
			Complete: false,
			Members:  []SourceInventoryObservationMember{{Name: "Name", Role: AnswerCandidateRoleFunction, File: "internal/agent/sub_explorer.go"}},
		}},
	}
	rm := RequestModel{SourceInventoryProfile: &SourceInventoryProfile{
		IsSourceInventory: true,
		TargetRoles:       []AnswerCandidateRole{AnswerCandidateRoleFunction, AnswerCandidateRoleType},
		RequestedFields:   []SourceInventoryRequestedField{SourceInventoryFieldName, SourceInventoryFieldLocation, SourceInventoryFieldValues},
	}}

	auth := BuildSourceInventoryCompletionAuthority(obs, rm, false)
	if auth.Blocking || auth.ReasonCode != SourceInventoryCompletionReasonInactive {
		t.Fatalf("value lookup support profile should not hard-block source-inventory completion, got %+v", auth)
	}
}

func TestSourceInventoryCompletionAuthority_EnumValueInventoryStillBlocks(t *testing.T) {
	obs := SourceInventoryObservation{
		Active:   true,
		Complete: false,
		Scopes:   []string{"."},
		Lens:     []string{"members"},
		Sets: []SourceInventoryObservationSet{{
			Role:     AnswerCandidateRoleType,
			Complete: false,
			Members:  []SourceInventoryObservationMember{{Name: "Mode", Role: AnswerCandidateRoleType, File: "pkg/mode.go"}},
		}},
	}
	rm := RequestModel{SourceInventoryProfile: &SourceInventoryProfile{
		IsSourceInventory: true,
		TargetRoles:       []AnswerCandidateRole{AnswerCandidateRoleType},
		TypeUnderlying:    SourceInventoryTypeUnderlyingString,
		RequiresConstSet:  true,
		RequestedFields:   []SourceInventoryRequestedField{SourceInventoryFieldName, SourceInventoryFieldLocation, SourceInventoryFieldValues},
	}}

	auth := BuildSourceInventoryCompletionAuthority(obs, rm, false)
	if !auth.Blocking || auth.ReasonCode != SourceInventoryCompletionReasonIncompleteObservation {
		t.Fatalf("enum-like value inventory should remain blocking until complete, got %+v", auth)
	}
}

func TestSourceInventoryCompletionAuthority_InactiveWithoutExecutableLens(t *testing.T) {
	obs := SourceInventoryObservation{
		Active:        true,
		Complete:      false,
		SourceClasses: []SourceInventorySourceClassCount{{Role: SourcePathRoleProduction, Count: 3, Complete: true}},
		Lens:          []string{"source_class_universe", "count"},
	}
	rm := RequestModel{SourceInventoryProfile: &SourceInventoryProfile{
		IsSourceInventory: true,
		TargetRoles:       []AnswerCandidateRole{AnswerCandidateRoleType},
	}}

	auth := BuildSourceInventoryCompletionAuthority(obs, rm, false)
	if auth.Blocking || auth.ReasonCode != SourceInventoryCompletionReasonInactive {
		t.Fatalf("class-universe seed must not become executable completion debt: %+v", auth)
	}
}
