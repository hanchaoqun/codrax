package tool

import (
	"fmt"
	"strings"

	"github.com/hanchaoqun/codrax/internal/tracequery"
	"github.com/hanchaoqun/codrax/internal/types"
)

// Reuse the already published business observations. A navigation hint does
// not add a measurement or join a marker's wall time to another state account.
func traceFrameBundleBusinessPreview(result tracequery.Result, records []types.ObservationRecord, payloadRef string) string {
	stats := result.WindowStats
	if stats == nil || len(result.TraceArtifacts) != 1 || result.TraceArtifacts[0].VirtualLineBase != 0 {
		return ""
	}
	type spanKey struct {
		subject, name string
		start, end    int
	}
	published := make(map[spanKey]types.ObservationRecord)
	for _, record := range records {
		if record.Predicate == types.TraceBusinessSpanPredicate {
			published[spanKey{record.Subject, record.Object, record.Span.LineStart, record.Span.LineEnd}] = record
		}
	}
	target := result.FrameRootCauseBundle.Target
	var out strings.Builder
	out.WriteString("### Business markers for the frame\n")
	var missing []string
	for _, name := range []string{"layout", "measure", "traversal", "Choreographer#doFrame", "draw", "DrawFrames"} {
		matches := 0
		for _, span := range stats.TraceSpans {
			exact := strings.EqualFold(span.Name, name)
			numbered := (name == "Choreographer#doFrame" || name == "DrawFrames") &&
				strings.HasPrefix(span.Name, name+" ") && len(span.Name) > len(name)+1 &&
				span.Name[len(name)+1] >= '0' && span.Name[len(name)+1] <= '9'
			renderMarker := name == "draw" || name == "DrawFrames"
			sameProcess := target.TGID > 0 && span.Thread.TGID == target.TGID
			if (!exact && !numbered) || (span.Thread.PID != target.PID && !(renderMarker && sameProcess)) {
				continue
			}
			record, ok := published[spanKey{traceThreadLabel(span.Thread), span.Name, span.StartLine, span.EndLine}]
			if !ok {
				continue
			}
			matches++
			if matches > 4 {
				continue
			}
			fmt.Fprintf(&out, "- %s interval=%.6f..%.6f lines=%d-%d evidence=%s\n",
				sanitizeForBanner(record.Summary), record.Span.StartTs, record.Span.EndTs,
				record.Span.LineStart, record.Span.LineEnd, record.ID)
			if states := traceQueryBusinessSpanSchedulerSummary(span); states != "" {
				fmt.Fprintf(&out, "  %s\n", states)
			}
		}
		if matches == 0 {
			missing = append(missing, name)
		} else if matches > 4 {
			fmt.Fprintf(&out, "- %s: showing 4 of %d published instances; retrieve a specific omitted instance only if needed.\n", name, matches)
		}
	}
	if len(missing) > 0 {
		fmt.Fprintf(&out, "- No matching published marker in this preview for %s. This is not absence evidence.\n", strings.Join(missing, ", "))
	}
	fmt.Fprintf(&out, "Marker wall time and marker-local states have separate rulers from attached preanalysis and full-frame states. Other returned markers: payload_ref=%s; inspect only the missing marker.\n\n", sanitizeForBanner(payloadRef))
	return out.String()
}

// Present existing observations at the head of the model response. The
// stored result, observation IDs, and evidence supplied to reports stay intact.
func traceFrameBundleEvidencePreview(records []types.ObservationRecord) string {
	var out strings.Builder
	for _, section := range []struct {
		title, fragment string
		limit           int
	}{
		{"Target thread state", "#target_window_states", 1},
		{"On-chain ranked candidates", "#root_cause_rank:", 8},
		{"Wakeup edges (individual edges, not a causal ranking)", "#wakeup_chain_edge:", 6},
	} {
		fmt.Fprintf(&out, "### %s\n", section.title)
		count := 0
		for _, record := range records {
			_, fragment, ok := strings.Cut(record.ID, "#")
			if !ok || !strings.HasPrefix("#"+fragment, section.fragment) ||
				section.limit == 1 && fragment != "target_window_states" {
				continue
			}
			var notes []string
			if section.fragment == "#root_cause_rank:" {
				onChain, ranked := false, false
				for _, note := range record.RichNotes {
					key, value, _ := strings.Cut(note, "=")
					switch key {
					case types.TraceNoteKeyChainRelevance:
						onChain = value == "on_chain"
					case types.TraceNoteKeyRank:
						ranked = value != "" && value != "0"
						notes = append(notes, note)
					case types.TraceNoteKeyEffectiveImpactMS, types.TraceNoteKeyImpactMS,
						types.TraceNoteKeySpanName, types.TraceNoteKeyBlockedReasonCaller, types.TraceNoteKeyMemberCount,
						types.TraceNoteKeyMemberFoldCaliber, types.TraceNoteKeyCausality,
						types.TraceNoteKeyDominantState, types.TraceNoteKeyDState, types.TraceNoteKeyIOWait:
						notes = append(notes, sanitizeForBanner(note))
					}
				}
				if !onChain || !ranked {
					continue
				}
			}
			count++
			if count > section.limit {
				continue
			}
			summary := sanitizeForBanner(record.Summary)
			if section.fragment == "#root_cause_rank:" {
				summary = fmt.Sprintf("%s %s %s", record.Subject, record.Object, strings.Join(notes, " "))
			}
			fmt.Fprintf(&out, "- %s lines=%d-%d evidence=%s\n", summary, record.Span.LineStart, record.Span.LineEnd, record.ID)
		}
		switch {
		case count == 0:
			out.WriteString("- No observation published for this section. Follow up only if needed.\n")
		case count > section.limit:
			fmt.Fprintf(&out, "- Showing %d of %d published observations. Inspect the existing result for an omitted fact if needed.\n", section.limit, count)
		}
	}
	out.WriteString("Cite these published values and IDs directly for the executed thread/window. Batch independent gaps in one tool-call response. A line range identifies the evidence location, not continuous blocking.\n\n")
	return out.String()
}
