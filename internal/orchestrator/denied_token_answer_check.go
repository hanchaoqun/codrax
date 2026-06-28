package orchestrator

import (
	"fmt"
	"sort"
	"strings"

	"github.com/hanchaoqun/codrax/internal/types"
)

// runDeniedTokenAnswerCheck is the L3 negative-knowledge answer-side
// validator. Walks every prose surface of the V2 answer document
// (block.Title / block.Summary / block.Text / list-item.Text /
// list-item.Label) and flags any block whose prose contains a
// verbatim mention of a TypedDenials token WITHOUT an accompanying
// "unverified" / "external" / "not in this repository" caveat.
//
// Behaviour contract:
//   - Skips when TypedDenials is empty (the dominant case for runs
//     without attached input or with all input fully corroborated).
//   - Whitelists answer prose that already discloses the denial:
//     the same line / surrounding sentence contains any of the
//     accepted disclosure phrases (English + Chinese), OR the token
//     appears inside a "<unverified-...>" marker (already redacted
//     by L2 Sanitise — passes through to answer if any path).
//   - Emits one Violation per (block id, denial token) pair so the
//     finalize-stage rewrite has actionable per-block feedback.
//
// SOFT-by-default — the answer ships; the closure ledger records the
// violation so operators can promote via pipeline_contract_strict_kinds
// or use telemetry to assess prompt drift.
func runDeniedTokenAnswerCheck(doc *types.AnswerDocumentV2, denials *types.TypedDenialSet) []types.Violation {
	if doc == nil || denials == nil || denials.Len() == 0 {
		return nil
	}
	if len(doc.Blocks) == 0 {
		return nil
	}

	// Precompute the deny set + per-token metadata. Sort tokens
	// longest-first so a substring match doesn't trip on a shorter
	// token when a longer one ALSO appears (e.g. "internal/foo.go"
	// vs "foo.go").
	type denialEntry struct {
		Token  string
		Class  types.TypedDenialClass
		Reason string
		IsPath bool
	}
	entries := make([]denialEntry, 0, denials.Len())
	for _, d := range denials.Snapshot() {
		if d.Class == types.TypedDenialAnswerSurfaceSymbolUnverified {
			// This class is owned by the current answer-surface validators
			// (label / diagram / inline-code checks). Those validators
			// return precise current-result violations and can be
			// deterministically repaired. Re-reading the append-only run
			// stamp here would turn a repaired draft into a stale user-visible
			// caveat whenever the identifier remains as ordinary prose.
			continue
		}
		isPath := d.Class == types.TypedDenialExternalLogFrameUnresolved ||
			d.Class == types.TypedDenialExternalPerfStallUnresolved ||
			d.Class == types.TypedDenialDriftFrameRelocated ||
			d.Class == types.TypedDenialAttachedExtractedUnscoped
		entries = append(entries, denialEntry{Token: d.Token, Class: d.Class, Reason: d.Reason, IsPath: isPath})
	}
	sort.Slice(entries, func(i, j int) bool { return len(entries[i].Token) > len(entries[j].Token) })

	var out []types.Violation
	for _, blk := range doc.Blocks {
		prose := proseTextOfBlock(blk)
		if prose == "" {
			continue
		}
		for _, e := range entries {
			if !mentionsToken(prose, e.Token) {
				continue
			}
			if proseDisclosesUnverified(prose, e.Token) {
				continue
			}
			detail := fmt.Sprintf("answer block %q names token %q without disclosing it as unverified / external", blk.ID, e.Token)
			repair := proseRepairHint(e.Token, e.IsPath)
			out = append(out, types.Violation{
				Kind:       types.ViolDeniedTokenUndeclared,
				Detail:     detail,
				Repair:     repair,
				Stage:      "finalize",
				ClusterKey: "denied_token_undeclared:" + blk.ID,
				SuspectedRoot: types.SuspectedRoot{
					IRField:    "answer_document.blocks." + blk.ID + ".prose",
					Reason:     fmt.Sprintf("verbatim mention of denied token %q (%s)", e.Token, e.Class),
					Confidence: 0.7,
				},
			})
		}
	}
	return out
}

// proseTextOfBlock collects every user-visible prose snippet on a
// block: Title / Text plus any list-item Text + Label. Concatenated
// with newlines so a multi-token detection sees the surrounding
// context for proseDisclosesUnverified.
func proseTextOfBlock(blk types.AnswerBlock) string {
	var parts []string
	if s := strings.TrimSpace(blk.Title); s != "" {
		parts = append(parts, s)
	}
	if s := strings.TrimSpace(blk.Text); s != "" {
		parts = append(parts, s)
	}
	for _, it := range blk.Items {
		if s := strings.TrimSpace(it.Label); s != "" {
			parts = append(parts, s)
		}
		if s := strings.TrimSpace(it.Text); s != "" {
			parts = append(parts, s)
		}
	}
	return strings.Join(parts, "\n")
}

// mentionsToken reports whether prose contains tok as a verbatim
// substring. Cross-OS path normalisation (POSIX ↔ Windows separator)
// matches the IsPathDenied / Sanitise convention.
func mentionsToken(prose, tok string) bool {
	if tok == "" {
		return false
	}
	if strings.Contains(prose, tok) {
		return true
	}
	// Try alternate path separator form (cross-OS coverage).
	if strings.ContainsAny(tok, "/\\") {
		alt := tok
		if strings.Contains(tok, "/") {
			alt = strings.ReplaceAll(tok, "/", "\\")
		} else if strings.Contains(tok, "\\") {
			alt = strings.ReplaceAll(tok, "\\", "/")
		}
		if alt != tok && strings.Contains(prose, alt) {
			return true
		}
	}
	return false
}

// proseDisclosesUnverified reports whether the prose around the
// token (or anywhere in the block's prose, scoped check) carries
// any of the canonical disclosure phrases. Bilingual coverage —
// the LLM may write the answer in zh or en (or both, for repos
// with --lang=zh defaults).
func proseDisclosesUnverified(prose, _ string) bool {
	lower := strings.ToLower(prose)
	disclosures := []string{
		// English
		"unverified", "external-source", "external source", "not in this repository",
		"not present in this repo", "could not be verified", "cannot be verified",
		"not defined in", "absent from this repo", "outside the repository",
		"runtime log", "attached log", "attached trace", "external log",
		"<unverified-",
		// Chinese
		"未验证", "未证实", "无法验证", "外部来源", "外部日志", "外部 trace",
		"不在此仓库", "本仓库未", "外部代码", "无法在仓库中找到",
	}
	for _, phrase := range disclosures {
		if strings.Contains(lower, strings.ToLower(phrase)) {
			return true
		}
	}
	return false
}

// proseRepairHint generates the Violation.Repair string the hint
// composer pairs with Detail. Names the offending token + the
// minimal-disclosure phrasing the LLM should add. No internal
// pipeline terminology (R6).
func proseRepairHint(token string, isPath bool) string {
	var what string
	if isPath {
		what = "this file path"
	} else {
		what = "this identifier"
	}
	return fmt.Sprintf("Re-emit this answer block with an explicit disclosure that %q comes from external runtime input (an attached log frame / trace tag) and is NOT present in the current repository — e.g., \"%s (from the attached input; %s could not be verified against repository sources)\". Do not silently cite or paraphrase %s as if it were a real repository symbol.",
		token, token, what, token)
}
