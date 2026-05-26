package types

import "testing"

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
