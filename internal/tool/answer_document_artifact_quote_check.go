// Package tool — answer_document_artifact_quote_check.go (XGAP-FIX ⑤
// independent arm, §29.104.8, witness 20260715-202022.323-89609).
//
// Runtime-artifact citations (attached trace/log blob refs) had NO quote
// verification on any path: normalizeCurrentSourceCitationQuotes
// deliberately skips artifact files (CPD #58 size/semantics guard), and the
// V2 answer-document lane never wires ground.GroundCitation. The witness
// shipped citations whose "quote" was model-authored prose summary while
// the cited artifact lines were raw ftrace events. This arm READS the
// cited artifact line and, when the published quote does not match the
// line's actual text, DISCLOSES the mismatch through the standard caveat
// lane. Detection → disclosure only: it never rejects an emit, never
// rewrites the quote, and runs on the healthy persist path AND the
// degraded recovery lane.
package tool

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/hanchaoqun/codrax/internal/logging"
	"github.com/hanchaoqun/codrax/internal/types"
)

// artifactQuoteCheckMaxScanBytes bounds the streamed line scan of one
// artifact file. Beyond the cap the arm goes inert for the remaining
// citations of that file (fail-open: no positive line text → no mismatch
// verdict → no disclosure). Aligned with the read_file whole-read wall so
// the check never out-reads the tool that produced the citation.
const artifactQuoteCheckMaxScanBytes = 64 << 20

// artifactQuoteCheckScanByteCap backs the stream wall above. It is a var
// (defaulting to the const) purely as a size-probe injection point: the
// negative fail-open pin (修补轮 件F, 2026-07-16) shrinks it to prove that a
// citation line beyond the cap yields ZERO disclosure, without synthesizing
// a real 64MiB artifact. Production code never mutates it.
var artifactQuoteCheckScanByteCap int64 = artifactQuoteCheckMaxScanBytes

// artifactQuoteMismatchCaveatPrefixZH / EN are the reconcile keys of the
// disclosure caveat (same upsert pattern as the trace-supplement
// disclosure lane: remove-by-prefix then append once, so repeated persists
// stay idempotent).
const (
	artifactQuoteMismatchCaveatPrefixZH = "运行时工件引用核对："
	artifactQuoteMismatchCaveatPrefixEN = "Runtime-artifact citation check:"
)

// artifactQuoteMismatchDisclosureCap bounds how many mismatched citations
// the caveat enumerates verbatim; the remainder is honestly counted.
const artifactQuoteMismatchDisclosureCap = 6

// verifyRuntimeArtifactCitationQuotes checks every single-line
// runtime-artifact citation whose Quote is non-empty against the artifact's
// actual line text and appends ONE disclosure caveat naming the mismatches.
// Returns the number of mismatched citations disclosed. Mutates only
// doc.Caveats.
func verifyRuntimeArtifactCitationQuotes(doc *types.AnswerDocumentV2, ctx *types.BusContext) int {
	if doc == nil || ctx == nil || len(doc.Citations) == 0 {
		return 0
	}
	artifactPaths := runtimeArtifactCitationPathSet(ctx)
	type pending struct {
		citationIdx int
		line        int
	}
	byPath := map[string][]pending{}
	for idx, cit := range doc.Citations {
		if cit.Line <= 0 || strings.TrimSpace(cit.NegativePattern) != "" {
			continue
		}
		if cit.LineEnd > cit.Line {
			// Multi-line range quotes have no single authoritative line;
			// out of this arm's precise scope.
			continue
		}
		if strings.TrimSpace(cit.Quote) == "" {
			continue
		}
		if !citationFileIsRuntimeArtifact(artifactPaths, cit.File) &&
			!types.LooksLikeRuntimeArtifactPath(cit.File) {
			continue
		}
		path := resolveRuntimeArtifactCitationReadPath(ctx, cit.File)
		if path == "" {
			continue
		}
		byPath[path] = append(byPath[path], pending{citationIdx: idx, line: cit.Line})
	}
	if len(byPath) == 0 {
		return 0
	}
	type mismatch struct {
		citationIdx int
		file        string
		line        int
	}
	var mismatches []mismatch
	for path, refs := range byPath {
		wanted := map[int]bool{}
		maxLine := 0
		for _, ref := range refs {
			wanted[ref.line] = true
			if ref.line > maxLine {
				maxLine = ref.line
			}
		}
		lines, err := readArtifactLines(path, wanted, maxLine)
		if err != nil {
			logging.Debug("[answer_document/artifact_quote_check] artifact %s unreadable, arm inert: %v", path, err)
			continue
		}
		for _, ref := range refs {
			text, ok := lines[ref.line]
			if !ok {
				// Line beyond the scan cap or past EOF — no positive
				// witness, no verdict (fail-open).
				continue
			}
			quote := doc.Citations[ref.citationIdx].Quote
			if artifactQuoteMatchesLine(quote, text) {
				continue
			}
			mismatches = append(mismatches, mismatch{
				citationIdx: ref.citationIdx,
				file:        doc.Citations[ref.citationIdx].File,
				line:        ref.line,
			})
		}
	}
	if len(mismatches) == 0 {
		return 0
	}
	sort.Slice(mismatches, func(i, j int) bool { return mismatches[i].citationIdx < mismatches[j].citationIdx })
	zh := runtimeTraceCausalProjectionUseChinese(requestedAnswerDocumentLanguage(ctx))
	kept := doc.Caveats[:0]
	for _, caveat := range doc.Caveats {
		trimmed := strings.TrimSpace(caveat)
		if strings.HasPrefix(trimmed, artifactQuoteMismatchCaveatPrefixZH) ||
			strings.HasPrefix(trimmed, artifactQuoteMismatchCaveatPrefixEN) {
			continue
		}
		kept = append(kept, caveat)
	}
	doc.Caveats = kept
	refs := make([]string, 0, len(mismatches))
	for i, m := range mismatches {
		if i >= artifactQuoteMismatchDisclosureCap {
			break
		}
		refs = append(refs, fmt.Sprintf("%s:%d", filepath.Base(strings.TrimSpace(m.file)), m.line))
	}
	extra := len(mismatches) - len(refs)
	var caveat string
	if zh {
		caveat = artifactQuoteMismatchCaveatPrefixZH +
			fmt.Sprintf("%d 处引用的摘录与运行时工件对应行的原文不符（%s", len(mismatches), strings.Join(refs, "、"))
		if extra > 0 {
			caveat += fmt.Sprintf("，另有 %d 处", extra)
		}
		// 修补轮 件D (2026-07-16): the proven fact is ONLY "quote ≠ cited
		// line text" — the quote may be verbatim from a DIFFERENT line
		// (line-number drift) just as well as a paraphrase, so the
		// disclosure must not over-claim "model paraphrase".
		caveat += "）。该摘录与所引工件行原文不符（可能为转述或行号错位）；请以工件行原文为准。"
	} else {
		caveat = artifactQuoteMismatchCaveatPrefixEN +
			fmt.Sprintf(" %d citation quote(s) do not match the cited runtime-artifact line text (%s", len(mismatches), strings.Join(refs, ", "))
		if extra > 0 {
			caveat += fmt.Sprintf(", +%d more", extra)
		}
		caveat += "). Those quotes differ from the cited artifact lines (they may be paraphrased or cite a shifted line number); treat the artifact line text as authoritative."
	}
	doc.Caveats = append(doc.Caveats, caveat)
	logging.Warning("[answer_document/artifact_quote_check] disclosed %d runtime-artifact citation quote mismatch(es)", len(mismatches))
	return len(mismatches)
}

// resolveRuntimeArtifactCitationReadPath maps a runtime-artifact citation
// File to a readable on-disk path: the spelled path itself when it exists
// AND its basename is a reserved artifact blob basename or the attached
// trace spelling (never an arbitrary absolute file — this arm must not
// become a generic file reader), else the session blob materialization of
// the same basename under the work dir.
func resolveRuntimeArtifactCitationReadPath(ctx *types.BusContext, file string) string {
	file = strings.TrimSpace(file)
	if file == "" {
		return ""
	}
	base := filepath.Base(file)
	reserved := false
	for _, name := range types.ReservedRuntimeArtifactBlobBasenames() {
		if strings.EqualFold(base, name) {
			reserved = true
			break
		}
	}
	if !reserved && ctx != nil {
		if spelled := strings.TrimSpace(ctx.AttachedHitraceSource); spelled != "" &&
			strings.EqualFold(filepath.Base(spelled), base) {
			reserved = true
		}
	}
	if !reserved {
		return ""
	}
	if info, err := os.Stat(file); err == nil && !info.IsDir() {
		return file
	}
	if ctx != nil && strings.TrimSpace(ctx.WorkDir) != "" {
		if path := attachedArtifactBlobPath(ctx.WorkDir, base); path != "" {
			return path
		}
	}
	return ""
}

// readArtifactLines streams the artifact once and returns the requested
// 1-based line numbers' text. Memory stays bounded (line-at-a-time); the
// scan aborts at artifactQuoteCheckMaxScanBytes with whatever it has.
func readArtifactLines(path string, wanted map[int]bool, maxLine int) (map[int]string, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	out := make(map[int]string, len(wanted))
	reader := bufio.NewReaderSize(f, 1<<20)
	lineNo := 0
	var scanned int64
	for lineNo < maxLine && len(out) < len(wanted) {
		line, err := reader.ReadString('\n')
		scanned += int64(len(line))
		if len(line) > 0 {
			lineNo++
			if wanted[lineNo] {
				out[lineNo] = strings.TrimRight(line, "\r\n")
			}
		}
		if err != nil {
			break
		}
		if scanned > artifactQuoteCheckScanByteCap {
			logging.Debug("[answer_document/artifact_quote_check] scan cap reached for %s at line %d; remaining citations stay unverified", path, lineNo)
			break
		}
	}
	return out, nil
}

// artifactQuoteMatchesLine reports whether the published quote is a
// verbatim (whitespace-folded) fragment of the artifact line or vice versa.
// Only a POSITIVE line read with no containment either way is a mismatch —
// truncated quotes (display cap) and quotes wider than the trimmed line
// both stay matched.
func artifactQuoteMatchesLine(quote, line string) bool {
	q := foldArtifactQuoteWhitespace(quote)
	l := foldArtifactQuoteWhitespace(line)
	if q == "" || l == "" {
		return true
	}
	// The citation display cap may have appended an ellipsis; strip a
	// trailing ellipsis before containment so truncation never reads as
	// fabrication.
	q = strings.TrimSuffix(strings.TrimSuffix(q, "…"), "...")
	q = strings.TrimSpace(q)
	if q == "" {
		return true
	}
	return strings.Contains(l, q) || strings.Contains(q, l)
}

func foldArtifactQuoteWhitespace(s string) string {
	return strings.Join(strings.Fields(strings.TrimSpace(s)), " ")
}
