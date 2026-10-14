package tool

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"strings"

	"github.com/hanchaoqun/codrax/internal/tracebundle"
	"github.com/hanchaoqun/codrax/internal/tracequery"
	"github.com/hanchaoqun/codrax/internal/types"
)

// This wire receipt is issued only from the native producer's private frozen
// source witness. Its tree digest binds the exact published (possibly bounded)
// tree, not arbitrary later JSON carrying the same member path. It is a
// display bridge, not a serialized replacement for preparation authority.
type traceQuerySamplingSourceBinding struct {
	Version          int    `json:"version"`
	QueryScopeID     string `json:"query_scope_id"`
	PayloadRef       string `json:"payload_ref"`
	TreeSHA256       string `json:"tree_sha256"`
	SourcePath       string `json:"source_path"`
	MemberPath       string `json:"member_path"`
	SourceGeneration string `json:"source_generation"`
	CaptureID        string `json:"capture_id"`
	MemberSHA256     string `json:"member_sha256"`
	MemberBytes      int64  `json:"member_bytes"`
}

func traceQuerySamplingTreeDigest(data []byte) string {
	digest := sha256.Sum256(data)
	return hex.EncodeToString(digest[:])
}

func traceQuerySamplingSourceBindingNote(result tracequery.Result, ref types.ObservationSourceRef, treeData []byte) string {
	source, ok := tracequery.PerfSamplingTreeBoundSource(result)
	if !ok || ref.QueryScopeID == "" || ref.PayloadRef == "" || result.SourcePath != ref.Path {
		return ""
	}
	binding := traceQuerySamplingSourceBinding{Version: 1, QueryScopeID: ref.QueryScopeID, PayloadRef: ref.PayloadRef,
		TreeSHA256: traceQuerySamplingTreeDigest(treeData), SourcePath: result.SourcePath, MemberPath: source.SourcePath,
		SourceGeneration: result.PerfSamplingTree.Cohorts[0].SourceGeneration,
		CaptureID:        source.CaptureID, MemberSHA256: source.SourceSHA256, MemberBytes: source.SourceBytes}
	data, err := json.Marshal(binding)
	if err != nil {
		return ""
	}
	return types.TraceNoteKeyPerfSamplingSourceBinding + "=" + string(data)
}

// Call only after DecodeTracePerfSamplingTree established producer, source,
// role, query, target and window binding. No display note can grant these.
func traceQuerySamplingSourceBindingMatches(record types.ObservationRecord, tree tracequery.PerfSamplingTreeResult) bool {
	var binding traceQuerySamplingSourceBinding
	count := 0
	for _, note := range record.RichNotes {
		if !strings.HasPrefix(note, types.TraceNoteKeyPerfSamplingSourceBinding+"=") {
			continue
		}
		count++
		raw := json.RawMessage(strings.TrimPrefix(note, types.TraceNoteKeyPerfSamplingSourceBinding+"="))
		decoder := json.NewDecoder(bytes.NewReader(raw))
		decoder.DisallowUnknownFields()
		if !traceRootCauseReportHasUniqueNestedKeys(raw) || decoder.Decode(&binding) != nil {
			return false
		}
	}
	if count != 1 || binding.Version != 1 || binding.SourcePath != tree.SourcePath ||
		binding.QueryScopeID != record.SourceRef.QueryScopeID || binding.PayloadRef != record.SourceRef.PayloadRef ||
		binding.MemberPath == "" || binding.MemberPath == tree.SourcePath || binding.SourceGeneration == "" ||
		binding.MemberBytes <= 0 || tracebundle.ValidateCaptureID(binding.CaptureID) != nil || tracebundle.ValidateSHA256(binding.MemberSHA256) != nil {
		return false
	}
	data, err := json.Marshal(tree)
	if err != nil || traceQuerySamplingTreeDigest(data) != binding.TreeSHA256 {
		return false
	}
	for _, cohort := range tree.Cohorts {
		if cohort.SourcePath != binding.MemberPath || cohort.SourceGeneration != binding.SourceGeneration {
			return false
		}
	}
	return len(tree.Cohorts) > 0
}
