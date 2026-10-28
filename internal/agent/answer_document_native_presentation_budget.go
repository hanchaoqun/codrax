package agent

import "strings"

const nativePresentationNavigationByteLimit = 16 << 10

// Sampling detail blocks share a dispatch-local byte budget. This is not a
// whole-prompt truncator: independent log/MCP, source and causal lanes retain
// their existing budgets. Call only for an atomic string immediately before
// writing it; publication receipts are recorded after that write succeeds.
func (p *answerDocNativePresentationReceipt) takePresentationBytes(n int) bool {
	if p == nil {
		return true
	}
	limit := p.presentationLimit
	if limit <= 0 {
		limit = 96 << 10
	}
	if n < 0 || n > limit-p.presentationBytes {
		return false
	}
	p.presentationBytes += n
	return true
}

// Navigation has a separate small allowance, so one large native tree cannot
// hide every later selector. Whole records survive or are counted as omitted;
// paths, IDs and names are never clipped into plausible different references.
func (p *answerDocNativePresentationReceipt) writePresentationNavigation(b *strings.Builder, text string) bool {
	if p != nil && len(text) > nativePresentationNavigationByteLimit-p.navigationBytes {
		p.navigationOmitted++
		return false
	}
	b.WriteString(text)
	if p != nil {
		p.navigationBytes += len(text)
	}
	return true
}

func (p *answerDocNativePresentationReceipt) writeDetailNavigation(b *strings.Builder, text string, localLimit int) bool {
	if b.Len()+len(text)+512 > localLimit {
		return false
	}
	if p != nil && len(text) > nativePresentationNavigationByteLimit-p.navigationBytes {
		p.navigationOmitted++
		return false
	}
	if !p.takePresentationBytes(len(text)) {
		return false
	}
	return p.writePresentationNavigation(b, text)
}
