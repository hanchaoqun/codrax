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
// TraceShareClaimPartGatedShare (HMC377 round 12) is the claim part of the
// LEVELMERGE-1 split's carved share — the one source of the token the engine
// mints (tracequery.RootCauseShareClaimGatedShare) and the projection merges
// read.
const TraceShareClaimPartGatedShare = "gated_share"

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
	// ResolvedMS / ReturnedMS / Successor (HMC377 round 11, R-377-11 ①): the
	// claim resolved per held segment — the part its holders
	// (HolderEvidenceIDs) hold, the part that passed back to the claimant
	// because the publication gate withdrew its holder, and whether a holder
	// is that withdrawn seat's published successor (another row of the
	// thread, not a seat of the claim's role). Carried only by a claim that
	// resolves in part, or by one the successor rule touched; a legacy entry
	// carries none of them (0 / false; Resolved says whether the whole share
	// is held).
	ResolvedMS float64
	ReturnedMS float64
	Successor  bool
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
	// ResolvedMS / ReturnedMS / Successor (HMC377 round 11): see
	// TraceCausalProjectionShareClaim. Extended marks an entry that carries
	// them (the eighth field); a legacy entry has seven fields.
	ResolvedMS float64
	ReturnedMS float64
	Successor  bool
	Extended   bool
}

const traceShareClaimRecordPositionMarker = "#root_cause_rank:"

// EncodeTraceShareClaimsNote renders the note value: one entry per claim,
// "part/role/resolved/exact/represented/share/pos+pos", entries joined by
// ";". "" for no
// claim (the note is then zero-dropped).
//
// HMC377 round 11 (R-377-11 ①): an Extended entry adds an eighth field,
// "r=<resolved ms>,t=<returned ms>,s=<0|1>" — the per-segment resolution of
// a claim that resolves in part or that the successor rule touched. Every
// other entry keeps the seven-field bytes.
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
		if c.Extended {
			entry += fmt.Sprintf("/r=%.3f,t=%.3f,s=%s", c.ResolvedMS, c.ReturnedMS, shareClaimBit(c.Successor))
		}
		parts = append(parts, entry)
	}
	return strings.Join(parts, ";")
}

// parseTraceShareClaimExtension decodes the eighth field of an Extended
// entry (HMC377 round 11). ok=false on any malformed key or value.
func parseTraceShareClaimExtension(field string, claim *TraceCausalProjectionShareClaim) bool {
	seen := map[string]bool{}
	for _, kv := range strings.Split(field, ",") {
		key, value, found := strings.Cut(kv, "=")
		if !found || seen[key] {
			return false
		}
		seen[key] = true
		switch key {
		case "r", "t":
			v, err := strconv.ParseFloat(value, 64)
			if err != nil || v < 0 {
				return false
			}
			if key == "r" {
				claim.ResolvedMS = v
			} else {
				claim.ReturnedMS = v
			}
		case "s":
			if !shareClaimBitValid(value) {
				return false
			}
			claim.Successor = value == "1"
		default:
			return false
		}
	}
	return seen["r"] && seen["t"] && seen["s"]
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
		if (len(fields) != 7 && len(fields) != 8) || fields[0] == "" || fields[1] == "" {
			continue
		}
		share, err := strconv.ParseFloat(fields[5], 64)
		if err != nil || !shareClaimBitValid(fields[2]) || !shareClaimBitValid(fields[3]) || !shareClaimBitValid(fields[4]) {
			continue
		}
		claim := TraceCausalProjectionShareClaim{Part: fields[0], Role: fields[1], ShareMS: share,
			Resolved: fields[2] == "1", Exact: fields[3] == "1", Represented: fields[4] == "1"}
		if len(fields) == 8 && !parseTraceShareClaimExtension(fields[7], &claim) {
			continue
		}
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
