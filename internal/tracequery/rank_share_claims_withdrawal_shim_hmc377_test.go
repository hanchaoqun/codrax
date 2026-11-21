package tracequery

// Accessors of the HMC377 round-10 withdrawal tests
// (rank_share_claims_withdrawal_r10_hmc377_test.go). The red run on b71f82049
// replaces this file through a build overlay with that commit's signature
// (WithdrawRankShareHolders(board, detail, withdrawn)) and zero accessors for
// the fields round 10 adds; the test file itself is unchanged.

func hmc377R10Withdraw(board, detail []RootCauseRankItem, withdrawnBoard, withdrawnDetail []bool) {
	WithdrawRankShareHolders(board, detail, withdrawnBoard, withdrawnDetail)
}

func hmc377R10Held(item RootCauseRankItem) float64 { return item.ChainAnchorRepresentedHeldMs }

func hmc377R10ClaimWithdrawn(item RootCauseRankItem) bool { return item.GatedShareClaimWithdrawn }
