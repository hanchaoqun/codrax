package orchestrator

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

// TestNoOrchestratorAgentReasoningEmits is a structural regression
// test that pins the EventOrchestratorNotice migration: NO emit site
// in the orchestrator package may construct
// `Kind: render.EventAgentReasoning` with `Agent: "orchestrator"`.
// Genuine LLM reasoning (agent.go) keeps EventAgentReasoning, but
// orchestrator-class soft messages MUST go through the new
// EventOrchestratorNotice event so the dock can render them without
// the `⋯ <stage> · Round N …` LLM-thinking signature.
//
// The check scans every .go file in the orchestrator package and
// fails if any line containing `EventAgentReasoning` is followed
// (within a small window) by `Agent: "orchestrator"`. Comments are
// allowed (we explicitly grep raw `Kind:` lines). Test files are
// excluded so this test does not gate itself.
func TestNoOrchestratorAgentReasoningEmits(t *testing.T) {
	files, err := filepath.Glob("*.go")
	if err != nil {
		t.Fatalf("glob: %v", err)
	}
	// Match the literal `Kind: render.EventAgentReasoning,` line
	// (allow varying whitespace) so a comment that mentions the type
	// name is not flagged.
	kindLine := regexp.MustCompile(`(?m)^\s*Kind:\s*render\.EventAgentReasoning,\s*$`)
	agentLine := regexp.MustCompile(`(?m)^\s*Agent:\s*"orchestrator",\s*$`)

	for _, f := range files {
		if strings.HasSuffix(f, "_test.go") {
			continue
		}
		raw, err := os.ReadFile(f)
		if err != nil {
			t.Fatalf("read %s: %v", f, err)
		}
		src := string(raw)
		// Find every Kind: render.EventAgentReasoning, line then look
		// at the surrounding ~10 lines for an Agent: "orchestrator"
		// line — capturing the Event{ ... } struct literal as a unit.
		for _, m := range kindLine.FindAllStringIndex(src, -1) {
			start := m[0]
			end := m[1]
			// Walk forward up to the next `})` (struct close) or 12
			// lines, whichever comes first; that bounds the Event
			// struct literal even on multi-line Reasoning helpers.
			window := src[end:]
			closeIdx := strings.Index(window, "})")
			if closeIdx < 0 || closeIdx > 600 {
				closeIdx = 600
			}
			body := window[:closeIdx]
			if agentLine.MatchString(body) {
				lineNum := strings.Count(src[:start], "\n") + 1
				t.Errorf("%s:%d emits EventAgentReasoning with Agent=\"orchestrator\" — orchestrator-class soft messages must use EventOrchestratorNotice + NoticeKind", f, lineNum)
			}
		}
	}
}
