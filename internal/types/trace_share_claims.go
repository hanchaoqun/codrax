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
}

const traceShareClaimRecordPositionMarker = "#root_cause_rank:"

// EncodeTraceShareClaimsNote renders the note value: one entry per claim,
// "part/role/resolved/exact/represented/share/pos+pos", entries joined by
// ";". "" for no
// claim (the note is then zero-dropped).
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
		parts = append(parts, fmt.Sprintf("%s/%s/%s/%s/%s/%.3f/%s", c.Part, c.Role, shareClaimBit(c.Resolved), shareClaimBit(c.Exact),
			shareClaimBit(c.Represented), c.ShareMS, strings.Join(positions, "+")))
	}
	return strings.Join(parts, ";")
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
	prefix := ""
	if at := strings.LastIndex(recordID, traceShareClaimRecordPositionMarker); at >= 0 {
		prefix = recordID[:at+len(traceShareClaimRecordPositionMarker)]
	}
	var out []TraceCausalProjectionShareClaim
	for _, entry := range strings.Split(value, ";") {
		fields := strings.Split(strings.TrimSpace(entry), "/")
		if len(fields) != 7 || fields[0] == "" || fields[1] == "" {
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
		out = append(out, claim)
	}
	return out
}
