package repl

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"os"
	"strings"
	"unicode/utf8"

	"github.com/hanchaoqun/codrax/internal/operation"
)

const (
	// Material paging is a prompt-context bridge, not an unbounded file
	// reader. The source and rendered-page ceilings keep memory/context costs
	// deterministic while covering ordinary manuals, logs, and captured
	// command output in one evaluator observation.
	commandOperationMaterialMaxSourceBytes = 2 * 1024 * 1024
	commandOperationMaterialPageRunes      = 6000
	commandOperationMaterialMaxPages       = 24
	commandOperationMaterialMaxSources     = 2
)

// commandOperationMaterialPage is system-owned evidence. Page and receipt
// refs are minted only from bytes read from an already-recorded command
// payload; a model cannot create an authoritative ref by mentioning one.
type commandOperationMaterialPage struct {
	Ref                string
	SourceRef          string
	SourceIdentity     string
	Representation     string
	Ordinal            int
	StartRune          int
	EndRune            int
	VisibleRunes       int
	SourceBytes        int64
	SourceTruncated    bool
	PagesTruncated     bool
	CoverageReceiptRef string
	Content            string
}

func commandOperationAttachMaterialPages(records []commandOperationResultRecord) []commandOperationResultRecord {
	if len(records) == 0 {
		return records
	}
	known := make(map[string]bool)
	for _, record := range records {
		for _, page := range record.MaterialPages {
			known[page.SourceIdentity+"\x00"+page.Representation] = true
		}
	}
	last := len(records) - 1
	addedSources := 0
	for _, ref := range commandOperationResultPayloadRefs(records[last].Result) {
		if addedSources >= commandOperationMaterialMaxSources {
			break
		}
		pages := commandOperationBuildMaterialPages(ref)
		if len(pages) == 0 {
			continue
		}
		key := pages[0].SourceIdentity + "\x00" + pages[0].Representation
		if known[key] {
			continue
		}
		known[key] = true
		records[last].MaterialPages = append(records[last].MaterialPages, pages...)
		addedSources++
	}
	return records
}

func commandOperationBuildMaterialPages(ref string) []commandOperationMaterialPage {
	ref = strings.TrimSpace(ref)
	if ref == "" {
		return nil
	}
	file, err := os.Open(ref)
	if err != nil {
		return nil
	}
	defer file.Close()
	info, err := file.Stat()
	if err != nil || !info.Mode().IsRegular() || info.Size() <= 0 {
		return nil
	}
	data, err := io.ReadAll(io.LimitReader(file, commandOperationMaterialMaxSourceBytes+1))
	if err != nil || len(data) == 0 || bytes.IndexByte(data, 0) >= 0 {
		return nil
	}
	sourceTruncated := len(data) > commandOperationMaterialMaxSourceBytes
	if sourceTruncated {
		data = data[:commandOperationMaterialMaxSourceBytes]
		var valid bool
		data, valid = trimCommandOperationMaterialUTF8Tail(data)
		if !valid {
			return nil
		}
	}
	if len(data) == 0 || !utf8.Valid(data) {
		return nil
	}
	raw := string(data)
	representation := "text"
	visible := raw
	if looksLikeHTML(raw) {
		representation = "html_text"
		visible = extractHTMLVisibleText(raw)
	}
	visible = compactMaterialText(visible)
	if visible == "" {
		return nil
	}
	digest := sha256.Sum256(data)
	identityKind := "sha256"
	if sourceTruncated {
		identityKind = "prefix_sha256"
	}
	sourceIdentity := fmt.Sprintf("%s:%s:bytes:%d", identityKind, hex.EncodeToString(digest[:]), info.Size())
	runes := []rune(visible)
	visibleRunes := len(runes)
	maxVisibleRunes := commandOperationMaterialPageRunes * commandOperationMaterialMaxPages
	pagesTruncated := visibleRunes > maxVisibleRunes
	pageRunes := visibleRunes
	if pageRunes > maxVisibleRunes {
		pageRunes = maxVisibleRunes
	}
	stableHash := hex.EncodeToString(digest[:])
	receipt := ""
	if !sourceTruncated && !pagesTruncated {
		receipt = fmt.Sprintf("material-coverage:v1:%s:%s", stableHash, representation)
	}
	pageCount := (pageRunes + commandOperationMaterialPageRunes - 1) / commandOperationMaterialPageRunes
	pages := make([]commandOperationMaterialPage, 0, pageCount)
	for ordinal, start := 0, 0; start < pageRunes; ordinal, start = ordinal+1, start+commandOperationMaterialPageRunes {
		end := start + commandOperationMaterialPageRunes
		if end > pageRunes {
			end = pageRunes
		}
		pages = append(pages, commandOperationMaterialPage{
			Ref:                fmt.Sprintf("material-page:v1:%s:%s:%d", stableHash, representation, ordinal+1),
			SourceRef:          ref,
			SourceIdentity:     sourceIdentity,
			Representation:     representation,
			Ordinal:            ordinal + 1,
			StartRune:          start,
			EndRune:            end,
			VisibleRunes:       visibleRunes,
			SourceBytes:        info.Size(),
			SourceTruncated:    sourceTruncated,
			PagesTruncated:     pagesTruncated,
			CoverageReceiptRef: receipt,
			Content:            string(runes[start:end]),
		})
	}
	return pages
}

// trimCommandOperationMaterialUTF8Tail accepts only a UTF-8 prefix whose
// invalidity can be explained by the byte ceiling cutting one final rune.
// UTF-8 uses at most four bytes, so at most three suffix bytes can be partial;
// bounding the candidates keeps validation O(n) even when an earlier byte is
// malformed instead of repeatedly rescanning a multi-megabyte prefix.
func trimCommandOperationMaterialUTF8Tail(data []byte) ([]byte, bool) {
	for trimmed := 0; trimmed < utf8.UTFMax && trimmed <= len(data); trimmed++ {
		candidate := data[:len(data)-trimmed]
		if utf8.Valid(candidate) {
			return candidate, true
		}
	}
	return nil, false
}

func renderCommandOperationMaterialPages(pages []commandOperationMaterialPage) string {
	if len(pages) == 0 {
		return ""
	}
	var b strings.Builder
	for i, page := range pages {
		if i == 0 || page.SourceIdentity != pages[i-1].SourceIdentity || page.Representation != pages[i-1].Representation {
			if b.Len() > 0 {
				b.WriteString("\n")
			}
			receipt := page.CoverageReceiptRef
			if receipt == "" {
				receipt = "unavailable"
			}
			fmt.Fprintf(&b, "material_coverage_ledger source_ref=%q source_identity=%q representation=%s source_bytes=%d visible_runes=%d source_truncated=%t pages_truncated=%t coverage_receipt_ref=%s\n",
				page.SourceRef, page.SourceIdentity, page.Representation, page.SourceBytes, page.VisibleRunes, page.SourceTruncated, page.PagesTruncated, receipt)
		}
		fmt.Fprintf(&b, "material_page ref=%s ordinal=%d range_runes=[%d,%d) visible_runes=%d\n%s\n",
			page.Ref, page.Ordinal, page.StartRune, page.EndRune, page.VisibleRunes, page.Content)
	}
	return strings.TrimRight(b.String(), "\n")
}

// renderCommandOperationMaterialCoverageForRepair keeps compact structured-tool
// repair on the same system-owned coverage authority as the full evaluator
// prompt. A source payload can have a truncated single-message preview while
// its normalized material pages cover the complete source; the receipt, not
// that preview flag, is the authority for this distinction.
func renderCommandOperationMaterialCoverageForRepair(records []commandOperationResultRecord) string {
	const maxSources = 16
	var b strings.Builder
	seen := make(map[string]bool)
	emitted := 0
	omitted := 0
	for _, record := range records {
		for _, page := range record.MaterialPages {
			key := page.SourceIdentity + "\x00" + page.Representation
			if seen[key] {
				continue
			}
			seen[key] = true
			if emitted >= maxSources {
				omitted++
				continue
			}
			receipt := strings.TrimSpace(page.CoverageReceiptRef)
			coverageStatus := "partial"
			if receipt != "" && !page.SourceTruncated && !page.PagesTruncated {
				coverageStatus = "complete"
			} else {
				receipt = "unavailable"
			}
			fmt.Fprintf(&b, "material_coverage_authority source_ref=%q source_identity=%q representation=%s source_truncated=%t pages_truncated=%t coverage_status=%s coverage_receipt_ref=%s\n",
				page.SourceRef, page.SourceIdentity, page.Representation, page.SourceTruncated, page.PagesTruncated, coverageStatus, receipt)
			emitted++
		}
	}
	if omitted > 0 {
		fmt.Fprintf(&b, "material_coverage_authority omitted_sources=%d\n", omitted)
	}
	return strings.TrimRight(b.String(), "\n")
}

func commandOperationMaterialCoverageAuthority(records []commandOperationResultRecord, extraResults ...operation.CommandOperationResult) (available, complete, incomplete map[string]bool) {
	available = make(map[string]bool)
	complete = make(map[string]bool)
	incomplete = make(map[string]bool)
	addResult := func(result operation.CommandOperationResult) {
		for _, ref := range commandOperationResultPayloadRefs(result) {
			ref = strings.TrimSpace(ref)
			if ref == "" {
				continue
			}
			available[ref] = true
			if commandPayloadMaterialExcerptFullyVisible(ref) {
				complete[ref] = true
			} else {
				incomplete[ref] = true
			}
		}
	}
	for _, record := range records {
		addResult(record.Result)
		for _, page := range record.MaterialPages {
			available[page.Ref] = true
			if page.CoverageReceiptRef != "" {
				available[page.CoverageReceiptRef] = true
				complete[page.CoverageReceiptRef] = true
			}
		}
	}
	for _, result := range extraResults {
		addResult(result)
	}
	return available, complete, incomplete
}

func commandOperationMaterialSourceFullyCovered(records []commandOperationResultRecord, ref string) bool {
	if commandPayloadMaterialExcerptFullyVisible(ref) {
		return true
	}
	for _, record := range records {
		for _, page := range record.MaterialPages {
			if page.SourceRef == ref && page.CoverageReceiptRef != "" {
				return true
			}
		}
	}
	return false
}
