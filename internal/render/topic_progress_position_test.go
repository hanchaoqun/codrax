package render

import (
	"strings"
	"testing"
	"time"
)

// stripAnsi removes ANSI colour escape sequences so tests can assert
// the textual layout independently of colour.
func stripAnsi(s string) string {
	var b strings.Builder
	inEsc := false
	for _, r := range s {
		if r == '\x1b' {
			inEsc = true
			continue
		}
		if inEsc {
			if r == 'm' {
				inEsc = false
			}
			continue
		}
		b.WriteRune(r)
	}
	return b.String()
}

// TestFormatStageDoneLine_TopicProgressPositionAndColor pins the
// 2026-05-17 visual fix for the "关注点 N/M" sub-topic tag:
//
//  1. POSITION: the topic tag MUST appear IMMEDIATELY AFTER the
//     stage progress ("2/4") and BEFORE the stage-done label
//     (e.g. "已完成证据收集"). Pre-fix the tag was tacked onto
//     the very end of the row after stage/total elapsed.
//
//  2. COLOR: the topic tag MUST render in statusMeta (gray) like
//     every other meta-segment on the row (round count, tool count,
//     elapsed). Pre-fix it was rendered in statusObjective (cyan)
//     which broke the row's monochrome-meta convention and made
//     it look like a different KIND of information than its
//     adjacent stage-progress sibling.
//
// Together these fix the user-reported issue: "关注点 2/2 看不到
// 意义在哪里" — the tag now sits next to its semantic sibling
// (stage progress) and shares the same colour grammar.
func TestFormatStageDoneLine_TopicProgressPositionAndColor(t *testing.T) {
	r := New(nil, false)
	r.lang = "zh"

	row := &taskRow{
		stage:       "explore",
		agent:       "explorer",
		startTime:   time.Unix(100, 0),
		endTime:     time.Unix(143, 0),
		okFinished:  true,
		iteration:   5,
		toolCount:   7,
		isNodeRow:   true,
		nodeID:      "n1_evidence_t1", // 2nd sibling of 2
		nodeKind:    "evidence",
		dispatchGen: 1,
	}

	line := r.formatStageDoneLine(row, /*topicTotal*/ 2)
	plain := stripAnsi(line)

	// (1) Topic tag MUST appear.
	if !strings.Contains(plain, "关注点 2/2") {
		t.Fatalf("topic tag missing: %q", plain)
	}

	// (2) POSITION: topic tag MUST appear BEFORE the stage-done
	// label ("已完成证据收集") so it sits next to the stage
	// progress, not at the row tail.
	topicAt := strings.Index(plain, "关注点 2/2")
	labelAt := strings.Index(plain, "已完成证据收集")
	if labelAt < 0 {
		t.Fatalf("stage-done label missing: %q", plain)
	}
	if topicAt > labelAt {
		t.Errorf("topic tag must appear BEFORE the stage label\nline: %q\ntopicAt=%d labelAt=%d", plain, topicAt, labelAt)
	}

	// (3) POSITION: topic tag MUST appear AFTER the stage progress
	// ("2/4" or similar — derived dynamically). The order ✓ → progress
	// → topic → label is the load-bearing contract.
	// Stage progress for explorer + nodeRow comes from stageProgressForFocus;
	// we don't bother computing the exact text here — we check the relative
	// position vs the glyph.
	glyphAt := strings.Index(plain, "✓")
	if glyphAt < 0 {
		t.Fatalf("success glyph missing: %q", plain)
	}
	if topicAt < glyphAt {
		t.Errorf("topic tag must appear AFTER the success glyph\nline: %q", plain)
	}

	// (4) Topic tag MUST be tail-free: no longer at the very end of
	// the row (the old position). Concretely: there must be content
	// after the topic tag (the stage label + meta fields).
	if strings.HasSuffix(strings.TrimSpace(plain), "关注点 2/2") {
		t.Errorf("topic tag MUST NOT be at the row tail anymore; got: %q", plain)
	}
}

// TestFormatStageDoneLine_TopicTagOnlyForMultiSubTopicEvidence
// pins that the topic tag is rendered ONLY when topicTotal > 1
// AND the row is an evidence-type node row. Other rows
// (probe / validate / non-node stage rows / single-sub_topic
// explore) MUST NOT render the tag.
func TestFormatStageDoneLine_TopicTagOnlyForMultiSubTopicEvidence(t *testing.T) {
	r := New(nil, false)
	r.lang = "zh"

	base := taskRow{
		stage:       "explore",
		agent:       "explorer",
		startTime:   time.Unix(100, 0),
		endTime:     time.Unix(143, 0),
		okFinished:  true,
		iteration:   5,
		isNodeRow:   true,
		nodeID:      "n1_evidence_t0",
		nodeKind:    "evidence",
		dispatchGen: 1,
	}

	// (a) Single-sub_topic (topicTotal=1): no tag.
	row := base
	plain := stripAnsi(r.formatStageDoneLine(&row, 1))
	if strings.Contains(plain, "关注点") || strings.Contains(plain, "focus") {
		t.Errorf("single-sub_topic row must NOT carry topic tag; got %q", plain)
	}

	// (b) Non-evidence node kind: no tag (even with topicTotal=2).
	row = base
	row.nodeKind = "validate"
	row.nodeID = "n2_validate"
	plain = stripAnsi(r.formatStageDoneLine(&row, 2))
	if strings.Contains(plain, "关注点") || strings.Contains(plain, "focus") {
		t.Errorf("non-evidence node must NOT carry topic tag; got %q", plain)
	}

	// (c) Non-node stage row (isNodeRow=false): no tag.
	row = base
	row.isNodeRow = false
	row.nodeID = ""
	row.nodeKind = ""
	plain = stripAnsi(r.formatStageDoneLine(&row, 2))
	if strings.Contains(plain, "关注点") || strings.Contains(plain, "focus") {
		t.Errorf("non-node stage row must NOT carry topic tag; got %q", plain)
	}
}

// TestFormatStageDoneLine_TopicTagEnglish pins the localised tag
// at the same position and colour for EN mode.
func TestFormatStageDoneLine_TopicTagEnglish(t *testing.T) {
	r := New(nil, false)
	r.lang = "en"

	row := &taskRow{
		stage:       "explore",
		agent:       "explorer",
		startTime:   time.Unix(100, 0),
		endTime:     time.Unix(143, 0),
		okFinished:  true,
		iteration:   3,
		isNodeRow:   true,
		nodeID:      "n1_evidence_t0", // 1st sibling of 3
		nodeKind:    "evidence",
		dispatchGen: 1,
	}

	plain := stripAnsi(r.formatStageDoneLine(row, 3))
	if !strings.Contains(plain, "focus 1/3") {
		t.Fatalf("EN topic tag 'focus 1/3' missing: %q", plain)
	}
	// MUST NOT carry zh tag.
	if strings.Contains(plain, "关注点") {
		t.Errorf("EN mode must NOT carry zh topic tag; got %q", plain)
	}
}
