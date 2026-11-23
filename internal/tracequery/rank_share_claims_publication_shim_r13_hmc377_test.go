package tracequery

// Accessors of the HMC377 round-13 tests (R-377-13 ⑤: WithdrawRankShareHolders
// takes the publication and writes its rows back). The red run on 532dc208d
// replaces this file through a build overlay with that commit's return-value
// signature and the literal bytes of the round-13 phrase (the test files
// themselves are unchanged).

func hmc377R13Withdraw(published *RootCauseRankResult, withdrawnBoard, withdrawnDetail []bool) {
	WithdrawRankShareHolders(published, withdrawnBoard, withdrawnDetail)
}

func hmc377R13InversionHeadSeeDetail() string { return rspaInversionDetailHeadSeeDetail }
