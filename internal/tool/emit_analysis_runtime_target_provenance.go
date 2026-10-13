package tool

import "github.com/hanchaoqun/codrax/internal/types"

// The roster is already structurally normalized. Reuse the lookup lane's exact
// token/selector provenance check, not its lookup role or routing. A positive
// PID owns identity even when a paired display name happens to be quoted; that
// name cannot authorize an invented PID. Do not reparse a normalized comm such
// as worker-3 as another thread selector. Process kind remains unchanged.
func runtimeTargetIdentityInQuote(target types.RuntimeTarget, quote string) bool {
	identity := types.RuntimeThreadLookup{PID: target.PID, SourceQuote: quote}
	if target.PID <= 0 {
		identity.Thread = target.Thread
	}
	return runtimeThreadLookupIdentityInQuote(identity)
}
