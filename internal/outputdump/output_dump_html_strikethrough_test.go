package outputdump

import (
	"os"
	"strings"
	"testing"
	"time"
)

// End-to-end regression for the customer-visible HTML dump face
// (specimen: .codrax/output/20260705-181128.544-80798.html): answer
// prose with two single-tilde range connectors in one paragraph must
// not come out struck through in the .html sibling. Ruling 2026-07-05:
// a single "~" never triggers strikethrough; "~~" keeps GFM semantics.
func TestWriteResultHTMLSingleTildeRangesNotStruckThrough(t *testing.T) {
	dir := t.TempDir()
	now := time.Date(2026, 7, 5, 18, 11, 28, 0, time.UTC)
	answer := "主线程在 34579.472865s~34579.587805s 的帧窗口内出现卡顿，唤醒链每层延迟 6~11ms，而 ~~全帧丢失~~ 的旧结论已修正。"
	result := WriteResult(Args{
		Dir:     dir,
		Max:     10,
		Request: "分析 trace",
		Answer:  answer,
		Now:     now,
		PID:     80798,
	})
	if result.HTMLPath == "" {
		t.Fatalf("expected html sibling, got %#v", result)
	}
	htmlBytes, err := os.ReadFile(result.HTMLPath)
	if err != nil {
		t.Fatalf("read html sibling: %v", err)
	}
	html := string(htmlBytes)
	if !strings.Contains(html, "34579.472865s~34579.587805s") ||
		!strings.Contains(html, "6~11ms") {
		t.Fatalf("tilde range connectors must stay literal in html dump")
	}
	// The embedded mermaid bundle contains "<del>" in minified JS, so
	// assert on the prose: no strikethrough may open on range content.
	if strings.Contains(html, "<del>34579") || strings.Contains(html, "<del>6") {
		t.Fatalf("html dump struck through prose between single-tilde ranges")
	}
	if !strings.Contains(html, "<del>全帧丢失</del>") {
		t.Fatalf("double-tilde strikethrough must keep standard GFM semantics in html dump")
	}
}
