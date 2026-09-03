package tool

import (
	"path/filepath"
	"testing"

	"github.com/hanchaoqun/codrax/internal/types"
)

// run_tests_fold_in6_test.go — fold-in round six (colleague_merge_audit
// §40.36 六轮收编): the FOURTH member of the baseline reason-code family.
// When no immutable main snapshot exists to probe (MainRepoRoot unset,
// pointing at a missing directory, or equal to the active worktree), the
// producer mints the baseline_unavailable evidence row WITHOUT running the
// inner probe, and that row carries its own family code —
// verification_probe_baseline_snapshot_unavailable — with an EMPTY
// BaselineProbeReasonCode (no inner probe ran, so there is no inner reason
// to record). The round-five pins only drove the probed-but-unavailable
// path, so the snapshot-missing arm of the family was documented as three
// codes while the producer wrote a fourth.
func TestBaselineSnapshotUnavailableRowCarriesItsOwnFamilyCode(t *testing.T) {
	active := t.TempDir()
	for _, tc := range []struct {
		name     string
		mainRoot string
	}{
		{name: "main_root_unset", mainRoot: ""},
		{name: "main_root_missing", mainRoot: filepath.Join(active, "no-such-snapshot")},
		{name: "main_root_equals_active", mainRoot: active},
	} {
		t.Run(tc.name, func(t *testing.T) {
			mu := types.NewMutableState("snapshot unavailable baseline")
			plan := &types.ChangePlan{
				ID:     "plan-snapshot-unavailable",
				Status: types.PlanStatusPending,
				VerificationProbes: []types.VerificationProbe{{
					ID:                     "p1",
					Language:               "python",
					Code:                   "assert True\n",
					WorkingDir:             ".",
					ExpectsBaselineFailure: true,
				}},
			}
			mu.SetChangePlan(plan)
			ctx := &types.BusContext{
				Mutable:      mu,
				Mode:         types.ModeApply,
				RepoRoot:     active,
				MainRepoRoot: tc.mainRoot,
			}
			rows := runExpectedFailureVerificationProbeBaselines(ctx, verificationProbeBaselineSource)
			if len(rows) != 1 {
				t.Fatalf("want exactly one baseline evidence row, got %+v", rows)
			}
			row := rows[0]
			if row.Outcome != types.ExecutedCommandOutcomeBaselineUnavailable {
				t.Fatalf("Outcome = %q, want %q: %+v", row.Outcome, types.ExecutedCommandOutcomeBaselineUnavailable, row)
			}
			if row.ReasonCode != verificationProbeBaselineSnapshotUnavailableReasonCode {
				t.Fatalf("ReasonCode = %q, want the snapshot-unavailable family code %q: %+v", row.ReasonCode, verificationProbeBaselineSnapshotUnavailableReasonCode, row)
			}
			if row.BaselineProbeReasonCode != "" {
				t.Fatalf("BaselineProbeReasonCode = %q, want \"\" — no inner probe ran against a missing snapshot: %+v", row.BaselineProbeReasonCode, row)
			}
			if row.Source != verificationProbeBaselineSource || row.Command != "verification_probe_baseline:p1" {
				t.Fatalf("row identity = source %q command %q: %+v", row.Source, row.Command, row)
			}
			// The row classifies like every baseline evidence row: never a
			// failed command, never an unavailable verification reason.
			if types.ExecutedCommandFailed(row) {
				t.Fatalf("snapshot-unavailable baseline row must not be a failed command: %+v", row)
			}
			if code := types.ExecutedCommandUnavailableReasonCode(row); code != "" {
				t.Fatalf("ExecutedCommandUnavailableReasonCode = %q, want \"\": %+v", code, row)
			}
		})
	}
}
