package tool

import "github.com/hanchaoqun/codrax/internal/types"

// Setters of the HMC377 round-12 restriction-subset face pins
// (trace_query_subset_seat_r12_hmc377_test.go). The red run on fa2f01465
// replaces this file through a build overlay with no-op setters (the fields
// do not exist there); the test file itself is unchanged.

func hmc377R12SetRestricted(n *types.TraceCausalProjectionNode, account, elsewhere, here float64) {
	n.RestrictedReadyAccountMS, n.ShareCountedElsewhereMS, n.ShareCountedHereMS = account, elsewhere, here
}
