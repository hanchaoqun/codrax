package tool

import "github.com/hanchaoqun/codrax/internal/types"

// defaultTraceFindingShadowMode is process-scoped: when true, newly created
// MutableState contracts can be installed by orchestrator helpers. The
// authoritative per-run switch remains MutableState.TraceFindingContract.
var defaultTraceFindingShadowMode bool

// EnableTraceFindingShadowModeDefault turns on P0 shadow defaults for this process.
func EnableTraceFindingShadowModeDefault() {
	defaultTraceFindingShadowMode = true
}

// ApplyDefaultTraceFindingContract installs the shadow-optional contract when
// the process default is enabled and no contract is set yet.
func ApplyDefaultTraceFindingContract(mutable *types.MutableState) {
	if mutable == nil || !defaultTraceFindingShadowMode {
		return
	}
	if mutable.TraceFindingContract() != nil {
		return
	}
	mutable.SetTraceFindingContract(&types.TraceFindingContract{
		ShadowOptional:       true,
		FindingSchemaVersion: types.TraceFindingSchemaVersion,
	})
}
