package dataquery

import (
	"fmt"
	"strconv"
	"strings"
)

// actionRowIdentityField is the runner-owned row identity carrier (V9-1,
// colleague_merge_audit §40.15). It is the key for ledger dedupe and the
// cross-ledger consistency gate; `_source_locator` stays the lineage carrier.
// The two keys split roles: a 1:N action gives each derived sibling its own
// identity while every sibling keeps the parent's locator.
const actionRowIdentityField = "_row_identity"

// actionRecordLedgerIdentity returns one stable typed identity for a material
// row across filter/qualify/derived-artifact/contribution actions. An explicit
// item_id_field remains authoritative for itemID (B461). Otherwise the row
// identity wins: the `_row_identity` a 1:N/N:1 action minted, else the
// immutable source locator carried by the row (1:1 inherit lane); only
// genuinely raw rows fall back to source#index. It intentionally performs no
// fuzzy value matching or business-field inference, so ledger consistency
// checks join exact identities only.
func actionRecordLedgerIdentity(record actionRecord, rel, itemIDField string) (itemID, source, sourceLocator, rowIdentity string) {
	virtual := actionRecordVirtualFields(record, rel)
	source = strings.TrimSpace(virtual["_source_path"])
	sourceLocator = strings.TrimSpace(virtual["_source_locator"])
	rowIdentity = actionRecordRowIdentity(record, rel)
	if rowIdentity == "" {
		rowIdentity = fmt.Sprintf("%s#%d", firstNonEmptyString(source, strings.TrimSpace(rel)), record.Index)
	}
	itemID = strings.TrimSpace(recordField(record.Fields, itemIDField))
	if itemID == "" {
		itemID = rowIdentity
	}
	if sourceLocator == "" {
		sourceLocator = itemID
	}
	if source == "" {
		source = firstNonEmptyString(strings.TrimSpace(record.Path), strings.TrimSpace(rel))
	}
	return itemID, source, sourceLocator, rowIdentity
}

// actionRecordRowIdentity reads the identity a row carries: a minted
// `_row_identity`, else the inherited source locator (raw rows: identity ==
// locator). Empty only for rows without any source path.
func actionRecordRowIdentity(record actionRecord, rel string) string {
	if identity := strings.TrimSpace(recordField(record.Fields, actionRowIdentityField)); identity != "" {
		return identity
	}
	return strings.TrimSpace(actionRecordVirtualFields(record, rel)["_source_locator"])
}

// deriveActionRowIdentity is the single formatting rule for 1:N derivation:
// `<parent identity>#<ordinal>`. Nested derivations chain (items.csv#1#2#1),
// and rowIdentityAncestors peels the same rule back.
func deriveActionRowIdentity(record actionRecord, rel string, ordinal int) string {
	parent := actionRecordRowIdentity(record, rel)
	if parent == "" {
		parent = fmt.Sprintf("%s#%d", strings.TrimSpace(rel), record.Index)
	}
	return parent + "#" + strconv.Itoa(ordinal)
}

// stampDerivedRowIdentity stamps a 1:N derived row: parent lineage fields
// verbatim plus its own ordinal identity.
func stampDerivedRowIdentity(row map[string]any, record actionRecord, rel string, ordinal int) {
	if row == nil {
		return
	}
	stampActionRecordOriginFields(row, record, rel)
	row[actionRowIdentityField] = deriveActionRowIdentity(record, rel, ordinal)
}

// stampGroupRowIdentity stamps an N:1 group row with an artifact-local
// identity that cannot collide with any input row locator; lineage lives in
// the row's `_source_locators` list.
func stampGroupRowIdentity(row map[string]any, rel string, ordinal int) {
	if row == nil {
		return
	}
	row[actionRowIdentityField] = strings.TrimSpace(rel) + "#group#" + strconv.Itoa(ordinal)
}

// rowIdentityAncestors enumerates the derivation ancestors of a row identity,
// nearest first, stopping exactly at the lineage locator. It peels trailing
// `#<digits>` steps only while the remainder is still a strict extension of
// sourceLocator; an identity not formed from that locator has no enumerable
// ancestors (nil — fail-open, no fuzzy walk).
func rowIdentityAncestors(identity, sourceLocator string) []string {
	identity = strings.TrimSpace(identity)
	sourceLocator = strings.TrimSpace(sourceLocator)
	if identity == "" || sourceLocator == "" || identity == sourceLocator || !strings.HasPrefix(identity, sourceLocator+"#") {
		return nil
	}
	var out []string
	current := identity
	for current != sourceLocator {
		cut := strings.LastIndex(current, "#")
		if cut < 0 {
			return nil
		}
		if step := current[cut+1:]; step == "" || strings.Trim(step, "0123456789") != "" {
			return nil
		}
		current = current[:cut]
		if !strings.HasPrefix(current, sourceLocator) {
			return nil
		}
		out = append(out, current)
	}
	return out
}

// stampActionRecordOriginFields copies the immutable typed origin carrier into
// a derived row, including an inherited `_row_identity` when the record has
// one (1:1 inherit lane). The derived artifact alias remains available through
// the artifact itself and must not replace decision/contribution row identity.
func stampActionRecordOriginFields(row map[string]any, record actionRecord, rel string) {
	if row == nil {
		return
	}
	virtual := actionRecordVirtualFields(record, rel)
	for _, field := range []string{"_source", "_source_path", "_source_index", "_source_line", "_source_locator", actionRowIdentityField} {
		if value := strings.TrimSpace(virtual[field]); value != "" {
			row[field] = value
		}
	}
}
