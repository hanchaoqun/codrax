package types

import (
	"reflect"
	"strings"
	"testing"
)

func TestClassifyRepairDirective(t *testing.T) {
	tests := []struct {
		name string
		in   RepairDirective
		want RepairDebtClass
	}{
		{
			name: "advisory flag wins",
			in:   RepairDirective{Kind: RepairEmitEvidence, Advisory: true},
			want: RepairDebtAdvisory,
		},
		{
			name: "expand search is advisory debt",
			in:   RepairDirective{Kind: RepairExpandSearch, Keywords: []string{"token"}},
			want: RepairDebtAdvisory,
		},
		{
			name: "emit evidence is surgical grounding",
			in:   RepairDirective{Kind: RepairEmitEvidence, Files: []string{"a.go"}},
			want: RepairDebtSurgicalGrounding,
		},
		{
			name: "structured handoff is principal blocking",
			in:   RepairDirective{Kind: RepairStructuredHandoff, Subject: "aggregate_facts"},
			want: RepairDebtPrincipalBlocking,
		},
		{
			name: "completion form handoff is advisory",
			in: RepairDirective{
				Kind:   RepairStructuredHandoff,
				Origin: RepairOriginCompletionFormPrefix + "aggregate_facts",
			},
			want: RepairDebtAdvisory,
		},
		{
			name: "surgical read is surgical grounding",
			in:   RepairDirective{Kind: RepairReadFile, Files: []string{"a.go"}, LineRanges: []LineRange{{Start: 10, End: 12}}},
			want: RepairDebtSurgicalGrounding,
		},
		{
			name: "soft support read is advisory",
			in:   RepairDirective{Kind: RepairReadFile, Files: []string{"support.go"}, Origin: "phase1_unread"},
			want: RepairDebtAdvisory,
		},
		{
			name: "chain-promotion surgical read is advisory after accepted closure",
			in:   RepairDirective{Kind: RepairReadFile, Files: []string{"support.go"}, Origin: "chain_promotion.bridge_literal", LineRanges: []LineRange{{Start: 10, End: 12}}},
			want: RepairDebtAdvisory,
		},
		{
			name: "required read is principal blocking",
			in:   RepairDirective{Kind: RepairReadFile, Files: []string{"required.go"}, Origin: "required_file_hint_unread"},
			want: RepairDebtPrincipalBlocking,
		},
		{
			name: "subject rebind is principal blocking by default",
			in:   RepairDirective{Kind: RepairRebindSubject, Subject: string(SubjectFunctionName)},
			want: RepairDebtPrincipalBlocking,
		},
		{
			name: "empty directive is advisory for reporting",
			in:   RepairDirective{},
			want: RepairDebtAdvisory,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := ClassifyRepairDirective(tt.in); got != tt.want {
				t.Fatalf("ClassifyRepairDirective()=%q, want %q", got, tt.want)
			}
		})
	}
}

func TestRepairDirectiveRenderExpandSearchCarriesTypedTargetsWithoutBlankStem(t *testing.T) {
	withTargets := RepairDirective{
		Kind:      RepairExpandSearch,
		Files:     []string{"internal/agent/analyzer.go"},
		Keywords:  []string{"Analyzer", "AnalysisIR"},
		Rationale: "inspect the exact operation site",
	}.Render()
	for _, want := range []string{"Analyzer, AnalysisIR", "internal/agent/analyzer.go", "inspect the exact operation site"} {
		if !strings.Contains(withTargets, want) {
			t.Fatalf("expand-search rendering missing %q:\n%s", want, withTargets)
		}
	}

	filesOnly := RepairDirective{
		Kind:      RepairExpandSearch,
		Files:     []string{"internal/agent/analyzer.go"},
		Rationale: "inspect the exact operation site",
	}.Render()
	if strings.Contains(filesOnly, "stems:") || !strings.Contains(filesOnly, "internal/agent/analyzer.go") {
		t.Fatalf("files-only expand-search must not emit a blank keyword command:\n%s", filesOnly)
	}
}

func TestRepairDirectiveRenderExpandSearchPrefersTypedRepoMapBeforeBroadGrep(t *testing.T) {
	withTypedNavigation := RepairDirective{
		Kind:     RepairExpandSearch,
		Files:    []string{"internal/orchestrator/orchestrator.go"},
		Keywords: []string{"appendStageOutputEvidenceToMutable", "MutableState"},
		Tools:    []string{"repo_map", "grep", "read_file", "emit_evidence"},
	}.Render()
	for _, want := range []string{
		"Use repo_map first to locate exact definitions or structural relations",
		"then read the selected operation bodies",
		"use a narrowed grep only when no exact symbol or relation resolves",
		"appendStageOutputEvidenceToMutable, MutableState",
	} {
		if !strings.Contains(withTypedNavigation, want) {
			t.Fatalf("typed structural repair rendering missing %q:\n%s", want, withTypedNavigation)
		}
	}
	if strings.Contains(withTypedNavigation, "Run grep over these typed navigation stems") {
		t.Fatalf("typed structural repair must not demote an exact symbol target to grep-first:\n%s", withTypedNavigation)
	}

	grepOnly := RepairDirective{
		Kind:     RepairExpandSearch,
		Keywords: []string{"opaque-text-token"},
	}.Render()
	if !strings.Contains(grepOnly, "Run grep over these typed navigation stems: opaque-text-token") ||
		strings.Contains(grepOnly, "Use repo_map first") {
		t.Fatalf("grep-only repair must preserve its legacy navigation contract:\n%s", grepOnly)
	}
}

func TestClassifyPendingReadRepair(t *testing.T) {
	tests := []struct {
		name string
		in   PendingRead
		want RepairDebtClass
	}{
		{
			name: "phase1 unread support is advisory",
			in:   PendingRead{File: "support.go", Origin: "phase1_unread"},
			want: RepairDebtAdvisory,
		},
		{
			name: "line-ranged read is surgical",
			in:   PendingRead{File: "anchor.go", Origin: "pre_complete.multi_path_anchor", LineRanges: []LineRange{{Start: 4, End: 4}}},
			want: RepairDebtSurgicalGrounding,
		},
		{
			name: "line-ranged chain promotion is advisory",
			in:   PendingRead{File: "support.go", Origin: "chain_promotion.bridge_literal", LineRanges: []LineRange{{Start: 4, End: 4}}},
			want: RepairDebtAdvisory,
		},
		{
			name: "required file remains principal blocking",
			in:   PendingRead{File: "required.go", Origin: "required_file_hint_unread"},
			want: RepairDebtPrincipalBlocking,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := ClassifyPendingReadRepair(tt.in); got != tt.want {
				t.Fatalf("ClassifyPendingReadRepair()=%q, want %q", got, tt.want)
			}
		})
	}
}

func TestRepairBlocksAcceptedClosure_CompletionFormDebtDoesNotBlock(t *testing.T) {
	r := RepairDirective{
		Kind:   RepairStructuredHandoff,
		Origin: RepairOriginCompletionFormPrefix + "aggregate_facts",
	}
	if RepairBlocksAcceptedClosure(r) {
		t.Fatal("completion-form repair debt must not reopen exploration after accepted closure")
	}
}

func TestRepairBlocksAcceptedClosure_NoisySubjectRebindCannotReopenCompletion(t *testing.T) {
	r := RepairDirective{
		Kind:      RepairRebindSubject,
		Subject:   string(SubjectFunctionName),
		Origin:    "chain_ranker",
		Rationale: "ranked candidates stayed below the heuristic subject-match floor",
	}
	if got := ClassifyRepairDirective(r); got != RepairDebtPrincipalBlocking {
		t.Fatalf("ClassifyRepairDirective(rebind)=%q, want principal guidance while exploration is active", got)
	}
	if RepairBlocksAcceptedClosure(r) {
		t.Fatal("noisy subject-ranker guidance must not reopen an accepted typed closure")
	}
}

func TestMergeRepairs_MergesAcceptedEvidenceCarrier(t *testing.T) {
	in := []RepairDirective{
		{
			Kind:  RepairEmitEvidence,
			Files: []string{"internal/a.go"},
			AcceptedEvidence: []AcceptedEvidenceRef{
				{ID: "ev-a", Source: "internal/a.go", LineStart: 10, Subject: "A"},
			},
		},
		{
			Kind:  RepairEmitEvidence,
			Files: []string{"internal/a.go"},
			AcceptedEvidence: []AcceptedEvidenceRef{
				{ID: "ev-a", Source: "internal/a.go", LineStart: 10, Subject: "duplicate"},
				{ID: "ev-b", Source: "internal/b.go", LineStart: 22, Subject: "B"},
			},
		},
	}
	got := MergeRepairs(in)
	if len(got) != 1 {
		t.Fatalf("MergeRepairs len=%d, want 1", len(got))
	}
	if len(got[0].AcceptedEvidence) != 2 {
		t.Fatalf("accepted evidence len=%d, want 2: %+v", len(got[0].AcceptedEvidence), got[0].AcceptedEvidence)
	}
	if got[0].AcceptedEvidence[0].ID != "ev-a" || got[0].AcceptedEvidence[1].ID != "ev-b" {
		t.Fatalf("accepted evidence order/ids = %+v, want ev-a then ev-b", got[0].AcceptedEvidence)
	}
}

func TestMergeRepairs_MonotonicallyMergesProgressCarriers(t *testing.T) {
	got := MergeRepairs([]RepairDirective{
		{
			Kind:     RepairEmitEvidence,
			Keywords: []string{"source.Start"},
			Tools:    []string{"emit_evidence"},
		},
		{
			Kind:     RepairEmitEvidence,
			Keywords: []string{"source.Start", "sink.Run"},
			Tools:    []string{"repo_map", "grep", "read_file", "emit_evidence"},
		},
	})
	if len(got) != 1 {
		t.Fatalf("MergeRepairs len=%d, want 1", len(got))
	}
	if want := []string{"emit_evidence", "repo_map", "grep", "read_file"}; !reflect.DeepEqual(got[0].Tools, want) {
		t.Fatalf("merged tools=%v, want %v", got[0].Tools, want)
	}
	if want := []string{"source.Start", "sink.Run"}; !reflect.DeepEqual(got[0].Keywords, want) {
		t.Fatalf("merged keywords=%v, want %v", got[0].Keywords, want)
	}
}

func TestRepairDirectiveRender_DoesNotRenderAcceptedEvidenceCarrier(t *testing.T) {
	r := RepairDirective{
		Kind:      RepairEmitEvidence,
		Files:     []string{"internal/a.go"},
		Rationale: "materialize accepted evidence",
		AcceptedEvidence: []AcceptedEvidenceRef{
			{ID: "ev-secret", Source: "internal/hidden.go", LineStart: 99, Subject: "hidden"},
		},
	}
	rendered := r.Render()
	for _, forbidden := range []string{"ev-secret", "internal/hidden.go", "hidden"} {
		if strings.Contains(rendered, forbidden) {
			t.Fatalf("Render leaked accepted evidence carrier %q in:\n%s", forbidden, rendered)
		}
	}
}

func TestRepairDirective_SourceInventoryCompletionCarrierSurvivesNormalizeAndRender(t *testing.T) {
	r := RepairDirective{
		Kind: RepairStructuredHandoff,
		SourceInventoryCompletionAuthority: SourceInventoryCompletionAuthority{
			Active:     true,
			Blocking:   true,
			ReasonCode: SourceInventoryCompletionReasonFollowupDebt,
			FollowupDebt: SourceInventoryFollowupDebt{
				Active:     true,
				ReasonCode: SourceInventoryFollowupDebtPagination,
				Query: SourceInventoryLensQuery{
					Path:              ".",
					Scopes:            []string{"src"},
					Roles:             []AnswerCandidateRole{AnswerCandidateRoleFunction},
					IncludeCounts:     true,
					IncludeAttributes: false,
					TopN:              12,
					Cursor:            "24",
				},
			},
		},
	}

	normalized := NormalizeRepairDirective(r)
	if !normalized.SourceInventoryCompletionAuthority.IsBlocking() {
		t.Fatalf("source-inventory completion authority was not preserved: %+v", normalized)
	}
	if tools := RepairDirectiveRequiredTools(normalized); len(tools) != 1 || tools[0] != "repo_map" {
		t.Fatalf("required tools = %+v, want repo_map", tools)
	}
	rendered := normalized.Render()
	for _, want := range []string{
		"Source-inventory completion is blocked",
		"repo_map",
		"source_inventory",
		"cursor: \"24\"",
		"roles: [function]",
	} {
		if !strings.Contains(rendered, want) {
			t.Fatalf("rendered repair missing %q:\n%s", want, rendered)
		}
	}
}

func TestMergeRepairs_DoesNotConflateDifferentSourceInventoryDebt(t *testing.T) {
	base := RepairDirective{
		Kind:    RepairStructuredHandoff,
		Subject: "continue source inventory",
		SourceInventoryCompletionAuthority: SourceInventoryCompletionAuthority{
			Active:     true,
			Blocking:   true,
			ReasonCode: SourceInventoryCompletionReasonFollowupDebt,
			FollowupDebt: SourceInventoryFollowupDebt{
				Active:     true,
				ReasonCode: SourceInventoryFollowupDebtPagination,
				Query: SourceInventoryLensQuery{
					Path:              ".",
					Scopes:            []string{"src"},
					Roles:             []AnswerCandidateRole{AnswerCandidateRoleFunction},
					IncludeCounts:     true,
					IncludeAttributes: false,
					Cursor:            "24",
				},
			},
		},
	}
	next := base
	next.SourceInventoryCompletionAuthority.FollowupDebt.Query.Cursor = "48"

	got := MergeRepairs([]RepairDirective{base, next})
	if len(got) != 2 {
		t.Fatalf("MergeRepairs conflated distinct source-inventory cursors: %+v", got)
	}
}
