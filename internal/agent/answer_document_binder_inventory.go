package agent

import (
	"encoding/json"
	"fmt"
	"math"
	"strconv"
	"strings"

	"github.com/hanchaoqun/codrax/internal/types"
)

// This is a writing reference, not a second proof builder. Only the query's
// verified inventory can enter it. Its summary is data, never a gate on prose.
func traceBinderInventoryRecords(ledger types.ObservationLedger, rm *types.RequestModel) []types.ObservationRecord {
	var out []types.ObservationRecord
	seen := map[string]bool{}
	for _, r := range ledger.Records {
		if r.Predicate != "target_binder_wait_inventory" || r.Object != "indexed_target_verified_closed_waits" ||
			!traceBinderInventoryRecordEligible(r, rm) {
			continue
		}
		// Fold only identical measurements; different facts for the same scope
		// remain visible. Never elect a larger value or the latest publication.
		key, _ := json.Marshal([]any{traceBinderInventoryArtifact(r), r.Subject, r.Span.StartTs, r.Span.EndTs, r.Value, r.ResultCount, r.Summary})
		if !seen[string(key)] {
			seen[string(key)] = true
			out = append(out, r)
		}
	}
	return out
}

func traceBinderInventoryRecordEligible(r types.ObservationRecord, rm *types.RequestModel) bool {
	v, err := strconv.ParseFloat(r.Value, 64)
	return types.RuntimeObservationProducerIsDeterministicQuery(r.Producer) && r.Origin == types.AnswerEvidenceOriginRuntimeArtifact &&
		r.Role == types.AnswerAggregateRoleSupportingCoverage && r.GroundingPolicy == types.ClaimGroundingHard &&
		types.ObservationRecordMatchesUserRuntimeTarget(r, rm) && traceBinderInventoryArtifact(r) != "" &&
		r.Unit == "ms" && err == nil && v >= 0 && !math.IsNaN(v) && !math.IsInf(v, 0) &&
		r.Span.EndTs > r.Span.StartTs && !math.IsInf(r.Span.StartTs, 0) && !math.IsInf(r.Span.EndTs, 0)
}

func traceBinderInventoryArtifact(r types.ObservationRecord) string {
	// The real producer's ArtifactID may be the generic "attached_trace".
	// A physical capture path therefore outranks that display/source label.
	if path := types.RuntimeArtifactCaptureIdentityPath(r.SourceRef); path != "" {
		return path
	}
	if id := strings.TrimSpace(r.SourceRef.ArtifactID); id != "" {
		return id
	}
	return types.RuntimeArtifactCaptureIdentityPath(r.SourceRef)
}

func traceBinderInventoryMatchesBlocking(records []types.ObservationRecord, block types.TraceBlockingWallClockAuthority, ledger types.ObservationLedger) bool {
	if block.Type != "binder_wait" {
		return false
	}
	// Legacy blocking groups expose a label, not a physical capture key.
	// Resolve every contributing record instead of borrowing that label.
	// A mixed/unknown-source legacy account cannot acquire a new same-scope
	// relationship here; this writing helper does not re-group its numbers.
	capture := ""
	for _, occurrence := range block.Occurrences {
		if len(occurrence.RecordIDs) == 0 {
			return false
		}
		for _, id := range occurrence.RecordIDs {
			found := false
			for _, source := range ledger.Records {
				if source.ID != id {
					continue
				}
				key := traceBinderInventoryArtifact(source)
				if key == "" || capture != "" && capture != key {
					return false
				}
				capture, found = key, true
			}
			if !found {
				return false
			}
		}
	}
	if capture == "" {
		return false
	}
	for _, r := range records {
		if traceBinderInventoryArtifact(r) == capture && r.Subject == block.Subject &&
			fmt.Sprintf("%.6f..%.6f", r.Span.StartTs, r.Span.EndTs) == block.SelectedWindow {
			return true
		}
	}
	return false
}

func traceBinderInventoryScopeNote(zh bool) string {
	if zh {
		return "独立闭合 Binder 等待清单与已选因果链候选使用不同的筛选和证明范围。分别引用各自的次数、墙钟与范围；不要用链候选的下界替换独立清单，也不要相加或假定二者互为子集。扫描完成只覆盖已保留索引中构建的目标区间，不证明所有等待机制均已识别；未关联的 S/D/IO 区间不能因此判成非 Binder 或主动休眠。清单提供等待事实，不授予链上根因资格，业务诊断与优化结论仍由模型结合链证据作出。"
	}
	return "The independent closed Binder wait inventory and selected causal-chain candidates have different selection and proof scopes. Cite each count, wall clock, and scope separately; do not replace the inventory with the chain candidate lower bound, add them, or assume either is a subset. A complete scan covers constructed target intervals in the retained index, not every possible wait mechanism. Unassociated S/D/IO intervals do not prove non-Binder or voluntary sleep. Inventory facts grant no chain-root seat; the model owns business diagnosis and optimization conclusions using chain evidence."
}

func renderAnswerDocBinderInventory(ledger types.ObservationLedger, rm *types.RequestModel, lang string) string {
	records := traceBinderInventoryRecords(ledger, rm)
	if len(records) == 0 {
		return ""
	}
	var b strings.Builder
	b.WriteString("### Independently verified target Binder waits\n\n")
	fmt.Fprintf(&b, "- %s\n", traceBinderInventoryScopeNote(strings.HasPrefix(strings.ToLower(lang), "zh")))
	const groupLimit, rowLimit = 8, 8
	for i, r := range records {
		if i == groupLimit {
			fmt.Fprintf(&b, "- Inventory recap shows %d/%d scope measurements; additional measurements remain in tool evidence, not evaluated in this compact recap.\n", i, len(records))
			break
		}
		window := fmt.Sprintf("%.6f..%.6f", r.Span.StartTs, r.Span.EndTs)
		fmt.Fprintf(&b, "- [%s] artifact=`%s`; target=`%s`; window=`%s`; verified_wait_union=%sms. %s\n", r.ID, traceBinderInventoryArtifact(r), r.Subject, window, r.Value, r.Summary)
		n := 0
		seen := map[string]bool{}
		for _, row := range ledger.Records {
			if row.Predicate != "target_binder_wait_interval" || row.Object != "verified_reply_wakeup" ||
				!traceBinderInventoryRecordEligible(row, rm) || row.Subject != r.Subject ||
				traceBinderInventoryArtifact(row) != traceBinderInventoryArtifact(r) {
				continue
			}
			matchedWindow := false
			for _, note := range row.RichNotes {
				if note == types.TraceNoteKeySelectedWindow+"="+window {
					matchedWindow = true
				}
			}
			key, _ := json.Marshal([]any{row.Span, row.Value, row.Summary})
			if !matchedWindow || seen[string(key)] {
				continue
			}
			seen[string(key)] = true
			if n < rowLimit {
				fmt.Fprintf(&b, "  - [%s] %s\n", row.ID, row.Summary)
			}
			n++
		}
		if n > rowLimit {
			fmt.Fprintf(&b, "  - Interval recap shows %d/%d available evidence rows; the inventory union is computed before all display caps.\n", rowLimit, n)
		}
	}
	b.WriteByte('\n')
	return b.String()
}
