package tracequery

// Accessor of the HMC377 round-12 tests (R-377-12 ①: the publication's re-run
// returns the published board and lossless-detail rows, since a detail row can
// return to the board). The red run on fa2f01465 replaces this file through a
// build overlay with that commit's signature (no return value: the rows are
// published in place and a detail row never leaves the detail slice).
//
// HMC377 round 13 (R-377-13 ⑤): WithdrawRankShareHolders takes the published
// copy and writes its rows back; the accessor keeps the tests' shape. The red
// run on 532dc208d swaps in that commit's return-value signature.
func hmc377R12Withdraw(board, detail []RootCauseRankItem, withdrawnBoard, withdrawnDetail []bool) ([]RootCauseRankItem, []RootCauseRankItem) {
	published := RootCauseRankResult{Window: TimeWindow{StartTs: 10, EndTs: 11}, Items: board, AbsorbedItems: detail}
	WithdrawRankShareHolders(&published, withdrawnBoard, withdrawnDetail)
	return published.Items, published.AbsorbedItems
}

// The round-12 forms the tests read (the red run's shim spells the same
// bytes, since fa2f01465 has none of these composers).
func hmc377R12SharePartsSeeDetail() string { return rspaSharePartsSeeDetail }
func hmc377R12OwnedSeeDetail() string      { return rspaSummaryOwnedSeeDetail }
func hmc377R12TailReleased() string        { return rspaInversionDetailTailReleased }
func hmc377R12RepresentedSeeDetail(inside, full, held float64) string {
	return rspaRepresentedSeeDetail(inside, full, held)
}
func hmc377R12RepresentedPartial(inside, full, held, rest float64) string {
	return rspaRepresentedPartial(inside, full, held, rest)
}
