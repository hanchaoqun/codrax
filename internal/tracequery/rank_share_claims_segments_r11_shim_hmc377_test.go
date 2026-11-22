package tracequery

// Accessors of the HMC377 round-11 segment tests
// (rank_share_claims_segments_r11_hmc377_test.go). The red run on b2325eba6
// replaces this file through a build overlay with zero accessors for the
// fields round 11 adds; the test file itself is unchanged.

func hmc377R11Returned(item RootCauseRankItem) float64  { return item.ShareReturnedMs }
func hmc377R11ResolvedMs(c RootCauseShareClaim) float64 { return c.ResolvedMs }
func hmc377R11ReturnedMs(c RootCauseShareClaim) float64 { return c.ReturnedMs }
func hmc377R11Successor(c RootCauseShareClaim) bool     { return c.Successor }
func hmc377R11IsGatedShareClaim(c RootCauseShareClaim) bool {
	return c.Part == RootCauseShareClaimGatedShare
}
func hmc377R11HoldsGatedShare(item RootCauseRankItem) bool {
	return rspaHolds(item, RootCauseShareHolderGatedShareSeat, false)
}
