package orchestrator

import (
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/types"
)

func TestFormatCGECViolationLedgerSummarySplitsStrictAndAdvisory(t *testing.T) {
	t.Cleanup(func() { SetSoftViolationKinds(nil, nil) })
	SetSoftViolationKinds(nil, nil)

	closure := types.NewEvidenceClosure("")
	closure.AppendViolation(types.Violation{
		Kind: types.ViolBlockCoverageMissing,
		SuspectedRoot: types.SuspectedRoot{
			IRField: "answer_block_coverage",
		},
	})
	closure.AppendViolation(types.Violation{
		Kind: types.ViolWriteCrossSubRepoForbidden,
		SuspectedRoot: types.SuspectedRoot{
			IRField: "write_scope",
		},
	})

	got := formatCGECViolationLedgerSummary(closure, closure.Stats())
	for _, want := range []string{
		"ledger_events=2",
		"strict_findings=0",
		"advisory_events=2",
		"advisory_by_field={answer_block_coverage:1,write_scope:1}",
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("summary missing %q:\n%s", want, got)
		}
	}
	if strings.Contains(got, " violations=") || strings.Contains(got, " by_field=") {
		t.Fatalf("summary must not expose advisory debt as generic violations/by_field:\n%s", got)
	}
}

func TestFormatCGECViolationLedgerSummaryHonorsStrictPromotion(t *testing.T) {
	t.Cleanup(func() { SetSoftViolationKinds(nil, nil) })
	SetSoftViolationKinds(nil, []string{string(types.ViolBlockCoverageMissing)})

	closure := types.NewEvidenceClosure("")
	closure.AppendViolation(types.Violation{
		Kind: types.ViolBlockCoverageMissing,
		SuspectedRoot: types.SuspectedRoot{
			IRField: "answer_block_coverage",
		},
	})

	got := formatCGECViolationLedgerSummary(closure, closure.Stats())
	for _, want := range []string{
		"ledger_events=1",
		"strict_findings=1",
		"advisory_events=0",
		"strict_by_field={answer_block_coverage:1}",
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("summary missing %q:\n%s", want, got)
		}
	}
}

func TestFormatCGECViolationLedgerSummaryKeepsLegacyUnclassifiedEventsSeparate(t *testing.T) {
	closure := types.NewEvidenceClosure("")
	closure.BumpViolationsLogged(2)

	got := formatCGECViolationLedgerSummary(closure, closure.Stats())
	for _, want := range []string{
		"ledger_events=2",
		"strict_findings=0",
		"advisory_events=0",
		"legacy_unclassified_events=2",
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("summary missing %q:\n%s", want, got)
		}
	}
}
