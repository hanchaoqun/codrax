package tracequery

// Accessors of the HMC377 round-12 restriction-subset pins
// (rank_subset_seat_carve_r12_hmc377_test.go). The red run on fa2f01465
// replaces this file through a build overlay with zero accessors (the fields
// do not exist there); the test file itself is unchanged.

func hmc377R12Restricted(item RootCauseRankItem) (account, elsewhere, here float64) {
	return item.RestrictedReadyAccountMs, item.ShareCountedElsewhereMs, item.ShareCountedHereMs
}
