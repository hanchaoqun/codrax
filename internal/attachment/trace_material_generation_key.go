package attachment

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"sort"
)

// GenerationCacheKey identifies all already-bound file versions and selected
// material coordinates. It is not a source/evidence receipt, does not inspect
// the current filesystem, and cannot replace Validate or sourceCheck.
func (m *TraceMaterial) GenerationCacheKey() string {
	if m == nil {
		return ""
	}
	type boundFile struct{ Path, Version string }
	identity := struct {
		Source, Query, ArchiveMember string
		ExplicitSelection            bool
		QueryDomain                  TraceQueryDomain
		Files                        []boundFile
	}{Source: m.sourcePath, Query: m.queryPath, ArchiveMember: m.archiveMember,
		ExplicitSelection: m.explicitArchiveSelection, QueryDomain: m.queryDomain}
	for path, version := range m.bindings {
		identity.Files = append(identity.Files, boundFile{path, version.CacheToken()})
	}
	sort.Slice(identity.Files, func(i, j int) bool { return identity.Files[i].Path < identity.Files[j].Path })
	data, _ := json.Marshal(identity)
	digest := sha256.Sum256(data)
	return hex.EncodeToString(digest[:])
}
