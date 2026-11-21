package types

// trace_share_claims.go — HMC377 fix round 5: the one codec of the
// share_claims observation note (TraceNoteKeyShareClaims). A rank row's
// sentences may say that a share of its account is held by another seat of
// the same board; the engine resolves every such claim once against the
// published board, and the tool writes the result here: the part handed over,
// the holder role, the share, whether it resolved, whether one holder
// publishes exactly that share, and the holder rows' board positions. The
// projection turns the positions into the holder records' ids on the same
// board; the renderer names the holder through them and nothing else.
//
// HMC377 fix round 7 (R7-2): a claim the published board does not hold may
// carry the positions of the wakeup causal impacts whose own seats hold the
// share on the engine's pre-truncation pool; the projection turns them into
// the ids of those impacts' chain-view records on the same publication
// (wakeup_causal_impact:<position>) — the same causal impact under another
// record identity, never a value or name match.

import (
	"fmt"
	"strconv"
	"strings"
)

// TraceCausalProjectionShareClaim is one resolved share claim of a node.
type TraceCausalProjectionShareClaim struct {
	// Part: anchored / unwalked / remainder / chain_account.
	Part string
	// Role: chain_seat / clipped_seat / unwalked_seat / residual_seat /
	// remainder_seat.
	Role    string
	ShareMS float64
	// Resolved: the engine found the share held by seats on the published
	// board (HolderEvidenceIDs).
	Resolved bool
	// Exact: one holder publishes exactly this share.
	Exact bool
	// Represented: a "represented by" sentence makes the claim.
	Represented       bool
	HolderEvidenceIDs []string
	// HolderImpactEvidenceIDs (HMC377 fix round 7): on a claim the published
	// rank board does not hold (Resolved false), the ids of the chain-view
	// records of the causal impacts whose own seats hold the share — the
	// holder is drawn where one of those records is drawn.
	HolderImpactEvidenceIDs []string
}

// TraceShareClaimNote is one claim on the note wire (holder board positions,
// 1-based, the rank record identity's position suffix).
type TraceShareClaimNote struct {
	Part            string
	Role            string
	ShareMS         float64
	Resolved        bool
	Exact           bool
	Represented     bool
	HolderPositions []int
	// HolderImpactPositions (HMC377 fix round 7): the 1-based positions of the
	// causal impacts whose chain-view records publish the holders' accounts
	// (an unresolved claim only; the wire's optional eighth field).
	HolderImpactPositions []int
}

const (
	traceShareClaimRecordPositionMarker = "#root_cause_rank:"
	// TraceShareClaimCausalImpactRecordMarker is the record-identity suffix of
	// a causal impact's chain-view record on a trace_query publication
	// ("<scope>#wakeup_causal_impact:<position>").
	TraceShareClaimCausalImpactRecordMarker = "#wakeup_causal_impact:"
)

// EncodeTraceShareClaimsNote renders the note value: one entry per claim,
// "part/role/resolved/exact/represented/share/pos+pos", entries joined by
// ";" — an unresolved claim naming its holders' causal impacts appends
// "/impact+impact" (HMC377 fix round 7). "" for no claim (the note is then
// zero-dropped).
func EncodeTraceShareClaimsNote(claims []TraceShareClaimNote) string {
	parts := make([]string, 0, len(claims))
	for _, c := range claims {
		if strings.TrimSpace(c.Part) == "" || strings.TrimSpace(c.Role) == "" {
			continue
		}
		positions := make([]string, 0, len(c.HolderPositions))
		for _, p := range c.HolderPositions {
			if p > 0 {
				positions = append(positions, strconv.Itoa(p))
			}
		}
		entry := fmt.Sprintf("%s/%s/%s/%s/%s/%.3f/%s", c.Part, c.Role, shareClaimBit(c.Resolved), shareClaimBit(c.Exact),
			shareClaimBit(c.Represented), c.ShareMS, strings.Join(positions, "+"))
		if impacts := shareClaimPositionList(c.HolderImpactPositions); impacts != "" && !c.Resolved {
			// HMC377 fix round 7: the optional eighth field (absent on every
			// other claim, so their bytes do not move).
			entry += "/" + impacts
		}
		parts = append(parts, entry)
	}
	return strings.Join(parts, ";")
}

func shareClaimPositionList(list []int) string {
	out := make([]string, 0, len(list))
	for _, p := range list {
		if p > 0 {
			out = append(out, strconv.Itoa(p))
		}
	}
	return strings.Join(out, "+")
}

func shareClaimBitValid(v string) bool { return v == "0" || v == "1" }

func shareClaimBit(b bool) string {
	if b {
		return "1"
	}
	return "0"
}

// ParseTraceShareClaimsNote decodes the note value of the record recordID:
// holder positions become the ids of the records at those positions on the
// same rank board. A malformed entry is dropped whole (never guessed); a
// position without a rank record identity to anchor it leaves the claim
// without holders (it then reads as compacted).
func ParseTraceShareClaimsNote(value, recordID string) []TraceCausalProjectionShareClaim {
	value = strings.TrimSpace(value)
	if value == "" {
		return nil
	}
	prefix, scope := "", ""
	if at := strings.LastIndex(recordID, traceShareClaimRecordPositionMarker); at >= 0 {
		prefix = recordID[:at+len(traceShareClaimRecordPositionMarker)]
		scope = recordID[:at]
	}
	var out []TraceCausalProjectionShareClaim
	for _, entry := range strings.Split(value, ";") {
		fields := strings.Split(strings.TrimSpace(entry), "/")
		if (len(fields) != 7 && len(fields) != 8) || fields[0] == "" || fields[1] == "" {
			continue
		}
		share, err := strconv.ParseFloat(fields[5], 64)
		if err != nil || !shareClaimBitValid(fields[2]) || !shareClaimBitValid(fields[3]) || !shareClaimBitValid(fields[4]) {
			continue
		}
		claim := TraceCausalProjectionShareClaim{Part: fields[0], Role: fields[1], ShareMS: share,
			Resolved: fields[2] == "1", Exact: fields[3] == "1", Represented: fields[4] == "1"}
		if fields[6] != "" && prefix != "" {
			for _, raw := range strings.Split(fields[6], "+") {
				if pos, err := strconv.Atoi(raw); err == nil && pos > 0 {
					claim.HolderEvidenceIDs = append(claim.HolderEvidenceIDs, prefix+strconv.Itoa(pos))
				}
			}
		}
		if len(fields) == 8 && fields[7] != "" && scope != "" && !claim.Resolved {
			// HMC377 fix round 7: the holders' causal impacts' chain-view
			// records on the same publication.
			for _, raw := range strings.Split(fields[7], "+") {
				if pos, err := strconv.Atoi(raw); err == nil && pos > 0 {
					claim.HolderImpactEvidenceIDs = append(claim.HolderImpactEvidenceIDs, scope+TraceShareClaimCausalImpactRecordMarker+strconv.Itoa(pos))
				}
			}
		}
		out = append(out, claim)
	}
	return out
}
