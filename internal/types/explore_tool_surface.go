package types

// ExploreToolSurface is a scheduler-owned tool-surface restriction for a
// single explorer dispatch. It is more specific than ExploreDispatchKind: the
// latter is render/evaluator-key metadata, while this field is allowed to drive
// tool schema filtering and runtime tool-call boundary checks.
type ExploreToolSurface string

const (
	ExploreToolSurfaceDefault                 ExploreToolSurface = ""
	ExploreToolSurfaceSourceInventoryLens     ExploreToolSurface = "source_inventory_lens"
	ExploreToolSurfaceSourceInventoryFollowup ExploreToolSurface = "source_inventory_followup"
)

func (s ExploreToolSurface) IsSourceInventoryLens() bool {
	return s == ExploreToolSurfaceSourceInventoryLens
}

func (s ExploreToolSurface) IsSourceInventory() bool {
	return s == ExploreToolSurfaceSourceInventoryLens || s == ExploreToolSurfaceSourceInventoryFollowup
}

func (s ExploreToolSurface) IsSourceInventoryFollowup() bool {
	return s == ExploreToolSurfaceSourceInventoryFollowup
}
