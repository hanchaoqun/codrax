package context

import (
	"fmt"
	"sort"
	"strconv"
	"strings"

	"github.com/hanchaoqun/codrax/internal/types"
)

type traceBoardDomain struct {
	identity                  types.TraceRankBoardDisplayIdentity
	requestedScopeText        string
	parentWindowSeen          bool
	parentWindowKnown         bool
	parentWindowStart         float64
	parentWindowEnd           float64
	sourceRecord, sortKey     string
	chain, adjacent           []traceBoardRow
	shownChain, shownAdjacent int
	seenRows                  map[string]bool
}

func traceBoardAppendDomainRow(domains *[]*traceBoardDomain, byKey map[string]*traceBoardDomain, record types.ObservationRecord, row traceBoardRow) {
	identity := types.TraceRankBoardDisplayIdentityFromRecord(record)
	var domain *traceBoardDomain
	if identity.Complete {
		domain = byKey[identity.Key]
	}
	if domain == nil {
		domain = &traceBoardDomain{identity: identity, sourceRecord: record.ID, seenRows: map[string]bool{}}
		if identity.Complete {
			domain.sortKey = identity.Key
			byKey[identity.Key] = domain
		} else {
			// A sort tie-breaker is not an identity grant. Unknown records get
			// separate domains even if every visible label and source ID match.
			domain.sortKey = strings.Join([]string{
				"unknown", identity.ArtifactKey, identity.ArtifactPath, identity.ArtifactLabel,
				identity.BoardTarget, identity.BoardParamsFingerprint,
				strconv.FormatFloat(identity.WindowStartTs, 'g', -1, 64),
				strconv.FormatFloat(identity.WindowEndTs, 'g', -1, 64),
				record.ID, traceBoardRowIdentity(row),
			}, "\x00")
		}
		*domains = append(*domains, domain)
	}
	// All contributing publications participate, even a duplicate row or one
	// outside the display budget. Native board coordinates are not the parent
	// query that selected a requested member.
	domain.observeParentWindow(record.SourceRef)
	rowKey := traceBoardRowIdentity(row)
	if identity.Complete && domain.seenRows[rowKey] {
		return
	}
	domain.seenRows[rowKey] = true
	if row.channel == "adjacent" {
		domain.adjacent = append(domain.adjacent, row)
	} else {
		domain.chain = append(domain.chain, row)
	}
}

func (domain *traceBoardDomain) observeParentWindow(ref types.ObservationSourceRef) {
	known := ref.Kind == types.ObservationSourceRuntimeArtifact && ref.QueryWindowKnown &&
		types.ResolveTraceQueryWindowScope(nil, ref.QueryWindowStartTs, ref.QueryWindowEndTs).Role != types.TraceQueryWindowScopeUnknownQueryWindow
	if !domain.parentWindowSeen {
		domain.parentWindowSeen, domain.parentWindowKnown = true, known
		if known {
			domain.parentWindowStart, domain.parentWindowEnd = ref.QueryWindowStartTs, ref.QueryWindowEndTs
		}
		return
	}
	// Exact producer endpoint agreement is order-independent. Tolerant request
	// matching happens later, only after this source tuple is unanimous; a
	// near-but-different parent must not elect whichever row arrived first.
	if !known || !domain.parentWindowKnown || domain.parentWindowStart != ref.QueryWindowStartTs || domain.parentWindowEnd != ref.QueryWindowEndTs {
		domain.parentWindowKnown = false
		domain.parentWindowStart, domain.parentWindowEnd = 0, 0
	}
}

func traceBoardSelectDomainRows(domains []*traceBoardDomain) {
	sort.SliceStable(domains, func(i, j int) bool { return domains[i].sortKey < domains[j].sortKey })
	for _, domain := range domains {
		for _, rows := range [][]traceBoardRow{domain.chain, domain.adjacent} {
			sort.SliceStable(rows, func(i, j int) bool {
				if rows[i].rank != rows[j].rank {
					return rows[i].rank < rows[j].rank
				}
				// Conflicting same-rank publications remain visible; no larger
				// value is chosen as a replacement or a preferred board.
				return traceBoardRowIdentity(rows[i]) < traceBoardRowIdentity(rows[j])
			})
		}
	}
	// The historical 8+4 budget is global, not multiplied by board count.
	// Round-robin only affects which rows fit the prompt; displayed rows are
	// then emitted together under their own board with original ordinals.
	for _, adjacent := range []bool{false, true} {
		remaining := traceBoardChainRowCap
		if adjacent {
			remaining = traceBoardAdjacentRowCap
		}
		for remaining > 0 {
			progress := false
			for _, domain := range domains {
				shown, total := &domain.shownChain, len(domain.chain)
				if adjacent {
					shown, total = &domain.shownAdjacent, len(domain.adjacent)
				}
				if *shown < total {
					*shown++
					remaining--
					progress = true
				}
				if remaining == 0 {
					break
				}
			}
			if !progress {
				break
			}
		}
	}
}

func traceBoardWriteDomains(b *strings.Builder, domains []*traceBoardDomain, writeRow func(traceBoardRow, string)) {
	omittedDomains, omittedChain, omittedAdjacent := 0, 0, 0
	for i, domain := range domains {
		if domain.shownChain+domain.shownAdjacent == 0 {
			omittedDomains++
			omittedChain += len(domain.chain)
			omittedAdjacent += len(domain.adjacent)
			continue
		}
		identity := domain.identity
		window := "unavailable"
		if types.TraceCausalProjectionWindowPresent(identity.WindowStartTs, identity.WindowEndTs) {
			window = fmt.Sprintf("%.6f..%.6f", identity.WindowStartTs, identity.WindowEndTs)
		}
		fmt.Fprintf(b, "### Rank board %d\n- capture=`%s`; target=`%s`; query_window=`%s`; params=`%s`; identity_complete=%t\n",
			i+1, sanitizeForInlineCode(firstNonEmptyBoardField(identity.ArtifactPath, identity.ArtifactLabel, "unavailable")),
			sanitizeForInlineCode(firstNonEmptyBoardField(identity.BoardTarget, "unavailable")), window,
			sanitizeForInlineCode(firstNonEmptyBoardField(identity.BoardParamsFingerprint, "unavailable")), identity.Complete)
		if domain.requestedScopeText != "" {
			fmt.Fprintf(b, "- %s\n", domain.requestedScopeText)
		}
		if !identity.Complete {
			fmt.Fprintf(b, "- source_record=`%s`; incomplete board identity; retained separately, not merged by labels.\n", sanitizeForInlineCode(domain.sourceRecord))
		}
		if len(domain.chain) > 0 {
			b.WriteString("On-chain seats (root-cause order within this board):\n")
			for _, row := range domain.chain[:domain.shownChain] {
				writeRow(row, "root-cause seat")
			}
			if hidden := len(domain.chain) - domain.shownChain; hidden > 0 {
				fmt.Fprintf(b, "- (+%d more seated rows in this board; see the measured observations)\n", hidden)
			}
		}
		if len(domain.adjacent) > 0 {
			b.WriteString("Adjacent-impact seats (time-adjacent to the chain, not on it — a distinct ordinal space, never merged with the on-chain order):\n")
			for _, row := range domain.adjacent[:domain.shownAdjacent] {
				writeRow(row, "adjacent seat")
			}
			if hidden := len(domain.adjacent) - domain.shownAdjacent; hidden > 0 {
				fmt.Fprintf(b, "- (+%d more adjacent rows in this board; see the measured observations)\n", hidden)
			}
		}
	}
	if omittedDomains > 0 {
		fmt.Fprintf(b, "- (+%d additional board domains omitted by the global display budget, containing %d seated and %d adjacent rows; all remain in the measured observations)\n", omittedDomains, omittedChain, omittedAdjacent)
	}
}
