package types

// DispatchRepositoryReadMessageReceipt binds a producer-observed byte version
// to one actual tool message. Its fields are private and JSON cannot recreate
// it. Binding is not delivery: BaseAgent commits it only after a successful
// request containing the exact message. It is never test-execution proof.
type DispatchRepositoryReadMessageReceipt struct {
	owner                 *MutableState
	generation            uint64
	key                   dispatchRepositoryFileReadKey
	version               dispatchRepositoryFileReadVersion
	callID, messageSHA256 string
}

type dispatchRepositoryDeliveredReadKey struct {
	key     dispatchRepositoryFileReadKey
	version dispatchRepositoryFileReadVersion
}

// MatchesMessage uses exact transport identity and bytes, not text semantics.
func (r DispatchRepositoryReadMessageReceipt) MatchesMessage(callID, content string) bool {
	return r.owner != nil && callID != "" && callID == r.callID && dispatchRepositoryReadDigest(content) == r.messageSHA256
}

// BindDispatchRepositoryReadMessage requires an appended current-dispatch
// result whose body preserves the original producer rendering. Agent-added
// advisory tails may follow its existing trim-newlines/line-break convention;
// changed source text, copied coverage alone, and historical JSON cannot bind.
func (m *MutableState) BindDispatchRepositoryReadMessage(generation uint64, callID string, result ToolResult) DispatchRepositoryReadMessageReceipt {
	if m == nil || callID == "" || !result.Success || result.ToolName != "read_file" || result.RuntimeArtifactRead != nil || result.ReadCoverage == nil {
		return DispatchRepositoryReadMessageReceipt{}
	}
	m.mu.RLock()
	defer m.mu.RUnlock()
	if generation != m.dispatchRepositoryFileReadGeneration || !m.dispatchRepositoryReadExactResultLocked(result) {
		return DispatchRepositoryReadMessageReceipt{}
	}
	var receipt DispatchRepositoryReadMessageReceipt
	for key, versions := range m.dispatchRepositoryFileReadVersions {
		if key.rawRef != result.RawRef || !dispatchRepositoryReadCoverageMatches(result, key) {
			continue
		}
		for _, version := range versions {
			c := result.ReadCoverage
			if c.LineStart != version.lineStart || c.LineEnd != version.lineEnd || c.TotalLines != version.totalLines ||
				!dispatchRepositoryReadMessagePreservesProducer(version, result.Summary) || !m.dispatchRepositoryReadVersionHasResultLocked(key, version) {
				continue
			}
			if receipt.owner != nil && (receipt.key != key || receipt.version != version) {
				return DispatchRepositoryReadMessageReceipt{} // ambiguous producer identity
			}
			receipt = DispatchRepositoryReadMessageReceipt{m, generation, key, version, callID, dispatchRepositoryReadDigest(result.Summary)}
		}
	}
	return receipt
}

func dispatchRepositoryReadCoverageMatches(r ToolResult, key dispatchRepositoryFileReadKey) bool {
	return r.ReadCoverage != nil && r.RawRef == key.rawRef && r.ReadCoverage.RawRef == key.rawRef && r.ReadCoverage.Path == key.path
}

func (m *MutableState) dispatchRepositoryReadExactResultLocked(want ToolResult) bool {
	for _, got := range m.dispatchToolResults {
		if got.Success && got.ToolName == "read_file" && got.RuntimeArtifactRead == nil && got.ReadCoverage != nil &&
			got.RawRef == want.RawRef && got.Summary == want.Summary && *got.ReadCoverage == *want.ReadCoverage {
			return true
		}
	}
	return false
}

func dispatchRepositoryReadMessagePreservesProducer(v dispatchRepositoryFileReadVersion, message string) bool {
	if v.producerSummarySHA256 == "" {
		return false
	}
	if len(message) == v.producerSummaryBytes && dispatchRepositoryReadDigest(message) == v.producerSummarySHA256 {
		return true
	}
	n := v.trimmedSummaryBytes
	return n > 0 && len(message) > n && message[n] == '\n' && dispatchRepositoryReadDigest(message[:n]) == v.trimmedSummarySHA256
}

// RecordDispatchRepositoryReadDelivery atomically commits a successful request
// snapshot. A stale dispatch or even one invalid ticket leaves all unchanged.
// The caller must supply only tickets matched to that request's actual messages.
func (m *MutableState) RecordDispatchRepositoryReadDelivery(generation uint64, receipts []DispatchRepositoryReadMessageReceipt) bool {
	if m == nil || len(receipts) == 0 {
		return false
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	if generation != m.dispatchRepositoryFileReadGeneration {
		return false
	}
	seen := map[string]bool{}
	for _, r := range receipts {
		if r.owner != m || r.generation != generation || r.callID == "" || seen[r.callID] || !m.dispatchRepositoryReadVersionHasResultLocked(r.key, r.version) {
			return false
		}
		seen[r.callID] = true
	}
	if m.dispatchRepositoryDeliveredReadVersions == nil {
		m.dispatchRepositoryDeliveredReadVersions = make(map[dispatchRepositoryDeliveredReadKey]bool)
	}
	for _, r := range receipts {
		m.dispatchRepositoryDeliveredReadVersions[dispatchRepositoryDeliveredReadKey{r.key, r.version}] = true
	}
	return true
}

// CompleteDispatchDeliveredRepositoryFileReadVersion requires gap-free pages
// of the exact version delivered by successful requests in this dispatch. It
// does not re-read files, authorize registration, or claim any test executed.
func (m *MutableState) CompleteDispatchDeliveredRepositoryFileReadVersion(root, path, sha256 string) bool {
	return m.completeDispatchRepositoryFileReadVersion(root, path, sha256, true)
}
