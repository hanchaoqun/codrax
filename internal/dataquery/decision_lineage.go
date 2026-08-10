package dataquery

import (
	"fmt"
	"strings"
)

// actionRecordLedgerIdentity returns one stable typed identity for a material
// row across filter/qualify/derived-artifact/contribution actions. An explicit
// item_id_field remains authoritative. Otherwise the immutable source locator
// carried by the row wins; only genuinely raw rows fall back to source#index.
// It intentionally performs no fuzzy value matching or business-field
// inference, so ledger consistency checks join exact identities only.
func actionRecordLedgerIdentity(record actionRecord, rel, itemIDField string) (itemID, source, sourceLocator string) {
	virtual := actionRecordVirtualFields(record, rel)
	source = strings.TrimSpace(virtual["_source_path"])
	sourceLocator = strings.TrimSpace(virtual["_source_locator"])
	itemID = strings.TrimSpace(recordField(record.Fields, itemIDField))
	if itemID == "" {
		itemID = sourceLocator
	}
	if itemID == "" {
		itemID = fmt.Sprintf("%s#%d", firstNonEmptyString(source, strings.TrimSpace(rel)), record.Index)
	}
	if sourceLocator == "" {
		sourceLocator = itemID
	}
	if source == "" {
		source = firstNonEmptyString(strings.TrimSpace(record.Path), strings.TrimSpace(rel))
	}
	return itemID, source, sourceLocator
}

// stampActionRecordOriginFields copies the immutable typed origin carrier into
// a derived row. The derived artifact alias remains available through the
// artifact itself and must not replace decision/contribution row identity.
func stampActionRecordOriginFields(row map[string]any, record actionRecord, rel string) {
	if row == nil {
		return
	}
	virtual := actionRecordVirtualFields(record, rel)
	for _, field := range []string{"_source", "_source_path", "_source_index", "_source_line", "_source_locator"} {
		if value := strings.TrimSpace(virtual[field]); value != "" {
			row[field] = value
		}
	}
}
