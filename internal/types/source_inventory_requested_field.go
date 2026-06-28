package types

// SourceInventoryRequestedField captures the answer fields the user asked the
// inventory to include. This is a typed request-shape carrier, not a rendering
// instruction; downstream may use it to avoid surfacing enum values when the
// user asked only for names and definition locations.
type SourceInventoryRequestedField string

const (
	SourceInventoryFieldName      SourceInventoryRequestedField = "name"
	SourceInventoryFieldLocation  SourceInventoryRequestedField = "location"
	SourceInventoryFieldSummary   SourceInventoryRequestedField = "summary"
	SourceInventoryFieldValues    SourceInventoryRequestedField = "values"
	SourceInventoryFieldCount     SourceInventoryRequestedField = "count"
	SourceInventoryFieldPackage   SourceInventoryRequestedField = "package"
	SourceInventoryFieldModule    SourceInventoryRequestedField = "module"
	SourceInventoryFieldNamespace SourceInventoryRequestedField = "namespace"
)

func AllSourceInventoryRequestedFields() []SourceInventoryRequestedField {
	return []SourceInventoryRequestedField{
		SourceInventoryFieldName,
		SourceInventoryFieldLocation,
		SourceInventoryFieldSummary,
		SourceInventoryFieldValues,
		SourceInventoryFieldCount,
		SourceInventoryFieldPackage,
		SourceInventoryFieldModule,
		SourceInventoryFieldNamespace,
	}
}

func (f SourceInventoryRequestedField) IsValid() bool {
	for _, declared := range AllSourceInventoryRequestedFields() {
		if f == declared {
			return true
		}
	}
	return false
}
