package types

// NormalizeSourceInventoryAuthoritySnapshot refreshes the derived fields after a
// caller overlays package-local authority data such as requested-universe
// follow-up debt. Keep this helper in types so every consumer recomputes the
// same snapshot flags instead of locally deciding which reason codes matter.
func NormalizeSourceInventoryAuthoritySnapshot(s SourceInventoryAuthoritySnapshot) SourceInventoryAuthoritySnapshot {
	s.CompletionAuthority = NormalizeSourceInventoryCompletionAuthority(s.CompletionAuthority)
	s.FollowupDebt = NormalizeSourceInventoryFollowupDebt(s.FollowupDebt)
	completionBlocking := s.CompletionAuthority.IsBlocking()
	if !completionBlocking {
		s.FollowupDebt = SourceInventoryFollowupDebt{}
	} else if s.CompletionAuthority.FollowupDebt.IsActive() {
		s.FollowupDebt = s.CompletionAuthority.FollowupDebt
	}
	s.NeedsFollowup = completionBlocking
	if s.NeedsFollowup {
		s.CanUseMechanicalRowsForCite = false
		s.CanEnterMechanicalLanding = false
	}
	s.ReasonCodes = sourceInventorySnapshotReasonCodes(s)
	return s
}
