package types

import (
	"strings"
	"testing"
)

// write_context_pack_fold_in5_test.go — fold-in round five
// (colleague_merge_audit §40.36 五轮收编, findings II / JJ): typed customer
// disclosures are must-carry on every surface, and the two typed row kinds
// keep their text byte-for-byte.
//
// II: the verification_lockfile_fixed_point item survives every cap — the
// 96-item pack bound and every consumer view limit / budget — even when
// refused rows crowd the front of the pack. Before the fix the item was an
// ordinary P1 row: with enough effect rows ahead of the lockfile row the
// 16-item controller view (and the pack bound itself) dropped the only
// carrier of the lockfile_fixed_point token.
//
// JJ: the normalisation exemption for verification_worktree_effect /
// verification_lockfile_fixed_point rows used to collapse whitespace inside
// the path ("my  dir/Cargo.lock" → "my dir/Cargo.lock"), so the pack row was
// not byte-identical to the controller line. Exempt rows keep their text
// byte-for-byte — no Fields join.

// refusedEffectForPath builds the crowding shape: an unclassified refused
// tracked row, the kind that precedes the lockfile row in a mixed refused
// run.
func refusedEffectForPath(path string) VerificationWorktreeEffect {
	return VerificationWorktreeEffect{
		Path: path, Kind: VerificationWorktreeEffectTrackedChanged, Ownership: "git_tracked",
		Action: "verification_failed_retained_for_review", DriftClass: VerificationWorktreeDriftUnclassified,
		Disposition: VerificationWorktreeEffectRefused,
	}
}

func packWithRefusedRowsAheadOfLockfile(refused int) (WriteContextPack, VerificationWorktreeEffect) {
	lockfile := lockfileEffectForPath("Cargo.lock", VerificationLockfileFixedPointUnprovenRunRefused)
	effects := make([]VerificationWorktreeEffect, 0, refused+1)
	for i := 0; i < refused; i++ {
		effects = append(effects, refusedEffectForPath("src/refused_"+strings.Repeat("x", i%7)+"_"+string(rune('a'+i%26))+".rs"))
	}
	effects = append(effects, lockfile)
	report := &ChangeReport{PlanID: "plan-ii", Passed: false, FailureKind: FailureKindVerificationSideEffect,
		WorktreeAudit: &VerificationWorktreeAudit{Status: VerificationWorktreeAuditTrackedDrift,
			TrackedEffectCount: refused + 1, Effects: effects}}
	return WriteContextPackFromChangeReport(report), lockfile
}

func viewCarriesLockfileDisclosure(items []WriteContextItem, want string) bool {
	for _, item := range items {
		if item.Kind == "verification_lockfile_fixed_point" && item.Text == want {
			return true
		}
	}
	return false
}

func TestWriteContextLockfileFixedPointItemIsMustCarryOnEveryCap(t *testing.T) {
	want := WriteContextLockfileFixedPointDisclosureText("Cargo.lock", VerificationLockfileFixedPointUnprovenRunRefused)
	if want == "" {
		t.Fatal("run-refused disclosure text must be non-empty")
	}
	t.Run("controller_view_limit_16_with_refused_rows_ahead", func(t *testing.T) {
		pack, _ := packWithRefusedRowsAheadOfLockfile(17)
		for _, consumer := range []WriteContextConsumer{WriteConsumerController, WriteConsumerPlanner, WriteConsumerVerifier} {
			view := pack.View(consumer, 16)
			if len(view.Items) > 16 {
				t.Fatalf("%s: the limit still applies, got %d items", consumer, len(view.Items))
			}
			if !viewCarriesLockfileDisclosure(view.Items, want) {
				t.Fatalf("%s: the 16-item view dropped the verification_lockfile_fixed_point item: %+v", consumer, view.Items)
			}
		}
	})
	t.Run("budgeted_view_keeps_the_item", func(t *testing.T) {
		pack, _ := packWithRefusedRowsAheadOfLockfile(17)
		view := pack.ViewForScopeWithBudget(WriteConsumerController, 16, "", "", 8)
		if !viewCarriesLockfileDisclosure(view.Items, want) {
			t.Fatalf("budgeted view dropped the verification_lockfile_fixed_point item: %+v", view.Items)
		}
	})
	t.Run("pack_bound_96_keeps_the_item", func(t *testing.T) {
		pack, _ := packWithRefusedRowsAheadOfLockfile(120)
		if len(pack.Items) > writeContextPackMaxItems {
			t.Fatalf("the pack bound still applies, got %d items", len(pack.Items))
		}
		if !viewCarriesLockfileDisclosure(pack.Items, want) {
			t.Fatalf("the %d-item pack bound dropped the verification_lockfile_fixed_point item", writeContextPackMaxItems)
		}
	})
}

// JJ: exempt rows keep their text byte-for-byte through normalisation — a
// path with a double space and a tab survives on the pack row exactly as the
// shared helpers render it, so the pack row and the controller line (which
// prints the helper output directly) are byte-identical.
func TestWriteContextWorktreeRowsKeepPathWhitespaceByteForByte(t *testing.T) {
	// Each path carries a double space and/or a tab, and fits the 240-rune
	// row bound so the fit helper keeps it whole.
	for _, path := range []string{"my  dir/Cargo.lock", "a\tb/Cargo.lock", "a  b/c\td/Cargo.lock"} {
		effect := lockfileEffectForPath(path, VerificationLockfileFixedPointUnprovenRunRefused)
		wantRow := WriteContextWorktreeEffectText(effect)
		wantItem := WriteContextLockfileFixedPointDisclosureText(path, VerificationLockfileFixedPointUnprovenRunRefused)
		if !strings.Contains(wantRow, path) || !strings.Contains(wantItem, path) {
			t.Fatalf("%q: the helpers must render the path verbatim: %q / %q", path, wantRow, wantItem)
		}
		pack := WriteContextPackFromChangeReport(&ChangeReport{PlanID: "plan-jj", Passed: true,
			WorktreeAudit: &VerificationWorktreeAudit{Status: VerificationWorktreeAuditTrackedDriftDisclosed,
				Effects: []VerificationWorktreeEffect{effect}}})
		var row, item string
		for _, it := range pack.Items {
			switch it.Kind {
			case "verification_worktree_effect":
				row = it.Text
			case "verification_lockfile_fixed_point":
				item = it.Text
			}
		}
		if row != wantRow {
			t.Fatalf("%q: pack effect row is not byte-identical to the controller line:\n pack: %q\n line: %q", path, row, wantRow)
		}
		if item != wantItem {
			t.Fatalf("%q: pack disclosure item is not byte-identical to the controller line:\n pack: %q\n line: %q", path, item, wantItem)
		}
	}
}
