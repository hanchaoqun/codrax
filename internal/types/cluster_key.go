package types

import "strings"

// ComposeClusterKey builds a stable typed cluster identity from
// alternating key/value pairs:
//
//	ComposeClusterKey("block", "summary", "root", "block_claim_use")
//	=> "block:summary|root:block_claim_use"
//
// Empty keys/values are dropped. When callers accidentally provide an
// odd number of strings, the trailing key is ignored so runtime code
// stays non-panicking.
func ComposeClusterKey(kv ...string) string {
	if len(kv) < 2 {
		return ""
	}
	parts := make([]string, 0, len(kv)/2)
	for i := 0; i+1 < len(kv); i += 2 {
		k := strings.TrimSpace(kv[i])
		v := strings.TrimSpace(kv[i+1])
		if k == "" || v == "" {
			continue
		}
		parts = append(parts, k+":"+v)
	}
	return strings.Join(parts, "|")
}

func RootClusterKey(rootField string) string {
	return ComposeClusterKey("root", rootField)
}

func BlockClusterKey(blockID string, rootField string) string {
	return ComposeClusterKey("block", blockID, "root", rootField)
}

func BlockKindClusterKey(kind AnswerBlockKind, rootField string) string {
	return ComposeClusterKey("block_kind", string(kind), "root", rootField)
}

func FacetClusterKey(kind string, rootField string) string {
	return ComposeClusterKey("facet", kind, "root", rootField)
}

func RelationClusterKey(kind DiagramRelationKind, rootField string) string {
	return ComposeClusterKey("relation", string(kind), "root", rootField)
}

func FamilyClusterKey(kind QuestionFamily, rootField string) string {
	return ComposeClusterKey("family", string(kind), "root", rootField)
}

func SymbolClusterKey(symbol string, rootField string) string {
	return ComposeClusterKey("symbol", symbol, "root", rootField)
}

func IdentityClusterKey(identity string, rootField string) string {
	identity = strings.TrimSpace(identity)
	rootField = strings.TrimSpace(rootField)
	switch {
	case identity != "" && rootField != "":
		return identity + "|root:" + rootField
	case identity != "":
		return identity
	case rootField != "":
		return "root:" + rootField
	default:
		return ""
	}
}

func TopicClusterKey(topic string, rootField string) string {
	return ComposeClusterKey("topic", topic, "root", rootField)
}
