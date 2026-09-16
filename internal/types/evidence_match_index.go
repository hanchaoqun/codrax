package types

import "strings"

// EvidenceMatchIndex separates exact fact identity from source-anchor revision
// candidates. A coordinate may hold several independent claims; a revision may
// amend a row only when its typed carrier is compatible with exactly one row.
type EvidenceMatchIndex struct {
	items      map[int]EvidenceItem
	byStable   map[string]int
	byRevision map[string]map[int]struct{}
}

func NewEvidenceMatchIndex(items []EvidenceItem) *EvidenceMatchIndex {
	idx := &EvidenceMatchIndex{
		items: make(map[int]EvidenceItem, len(items)), byStable: make(map[string]int, len(items)),
		byRevision: make(map[string]map[int]struct{}, len(items)),
	}
	for i, item := range items {
		idx.Set(i, item)
	}
	return idx
}

// Find prefers the complete stable merge key (not a bare ID). Ambiguous sparse
// revisions are kept independent instead of choosing the first or last sibling.
func (idx *EvidenceMatchIndex) Find(item EvidenceItem) (int, bool) {
	if i, ok := idx.byStable[EvidenceStableMergeKey(item)]; ok {
		return i, true
	}
	key := EvidenceRevisionKey(item)
	if key == "" {
		return 0, false
	}
	found, matched := 0, false
	for i := range idx.byRevision[key] {
		if !EvidenceRevisionCompatible(idx.items[i], item) {
			continue
		}
		if matched {
			return 0, false
		}
		found, matched = i, true
	}
	return found, matched
}

func (idx *EvidenceMatchIndex) Item(index int) EvidenceItem { return idx.items[index] }

// Set replaces a slot's indexes as well as its current carrier. Old aliases
// must not let later claims match a source/endpoint that this row no longer has.
func (idx *EvidenceMatchIndex) Set(index int, item EvidenceItem) {
	if old, ok := idx.items[index]; ok {
		key := EvidenceStableMergeKey(old)
		if idx.byStable[key] == index {
			delete(idx.byStable, key)
		}
		key = EvidenceRevisionKey(old)
		delete(idx.byRevision[key], index)
		if len(idx.byRevision[key]) == 0 {
			delete(idx.byRevision, key)
		}
	}
	idx.items[index] = item
	idx.byStable[EvidenceStableMergeKey(item)] = index
	if key := EvidenceRevisionKey(item); key != "" {
		if idx.byRevision[key] == nil {
			idx.byRevision[key] = make(map[int]struct{})
		}
		idx.byRevision[key][index] = struct{}{}
	}
}

// EvidenceRevisionCompatible compares typed identities, never prose or scores.
// It is only the cross-ID fallback: callers must prefer exact stable identity,
// which also supports explicit corrections to a previously accepted record.
func EvidenceRevisionCompatible(existing, incoming EvidenceItem) bool {
	key := EvidenceRevisionKey(existing)
	if key == "" || key != EvidenceRevisionKey(incoming) {
		return false
	}
	if existing.Origin != ClaimOriginUnknown && incoming.Origin != ClaimOriginUnknown && existing.Origin != incoming.Origin {
		return false
	}
	if existing.Authority != AuthorityUnknown && incoming.Authority != AuthorityUnknown && existing.Authority != incoming.Authority {
		return false
	}
	for _, pair := range [][2]string{{existing.OwnerSymbol, incoming.OwnerSymbol}, {existing.Condition, incoming.Condition}} {
		left, right := strings.TrimSpace(pair[0]), strings.TrimSpace(pair[1])
		if left != "" && right != "" && left != right {
			return false
		}
	}
	if a, b := existing.SelectorApplication, incoming.SelectorApplication; a != nil && b != nil &&
		(a.Owner != b.Owner || a.Literal != b.Literal) {
		return false
	}
	if existing.Kind == incoming.Kind &&
		strings.TrimSpace(existing.Subject) == strings.TrimSpace(incoming.Subject) &&
		strings.TrimSpace(existing.Predicate) == strings.TrimSpace(incoming.Predicate) &&
		strings.TrimSpace(existing.Object) == strings.TrimSpace(incoming.Object) &&
		strings.TrimSpace(existing.Condition) == strings.TrimSpace(incoming.Condition) {
		return true
	}
	// Reuse the narrow endpoint-promotion contract of the atomic carrier merger.
	// Reverse order must identify the same fact without downgrading its carrier.
	return evidenceTypedRelationEndpointPromotion(existing, incoming) || evidenceTypedRelationEndpointPromotion(incoming, existing)
}
