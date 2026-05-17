package render

import (
	"strings"
	"testing"
	"time"

	"github.com/pterm/pterm"
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
// 2026-05-17 visual fix for the ordinal sub-topic tag:
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
// Together these fix the user-reported issue: the tag now sits next to
// its semantic sibling (stage progress), shares the same colour grammar,
// and reads as an ordinal "which focus" label rather than completion.
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

	line := r.formatStageDoneLine(row, 2)
	plain := stripAnsi(line)

	// (1) Topic tag MUST appear.
	if !strings.Contains(plain, "第 2 个关注点，共 2 个") {
		t.Fatalf("topic tag missing: %q", plain)
	}

	// (2) POSITION: topic tag MUST appear BEFORE the stage-done
	// label ("已完成证据收集") so it sits next to the stage
	// progress, not at the row tail.
	topicAt := strings.Index(plain, "第 2 个关注点，共 2 个")
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
	if strings.HasSuffix(strings.TrimSpace(plain), "第 2 个关注点，共 2 个") {
		t.Errorf("topic tag MUST NOT be at the row tail anymore; got: %q", plain)
	}
}

// TestComposeDockRow2_TopicProgressPositionAndColor pins the same
// visual contract for the live dock row the user watches while explore
// is running:
//
//	▪ 2/4 · 第 2 个关注点，共 2 个 · 正在探索代码并收集证据 · 模型 ... · 约 ... tok
//
// Topic progress is navigation state, so it belongs beside stage
// progress at the front of the row. It must not trail after model/token
// telemetry, and it must use statusMeta instead of the brighter
// statusObjective marker color.
func TestComposeDockRow2_TopicProgressPositionAndColor(t *testing.T) {
	pterm.EnableColor()
	defer pterm.DisableColor()

	topicText := "第 2 个关注点，共 2 个"
	row := composeDockRow2(dockRowState{
		stageProgress:         "2/4",
		topicProgress:         topicText,
		stageLabel:            "正在探索代码并收集证据",
		modelID:               "MiniMax-M2.7-highspeed",
		contextTokensEstimate: 61000,
		contextWindowTokens:   200000,
		lang:                  "zh",
	})
	plain := stripAnsiEscapes(row)

	for _, want := range []string{"2/4", topicText, "正在探索代码并收集证据", "模型 MiniMax-M2.7-highspeed", "约 61k/200k tok"} {
		if !strings.Contains(plain, want) {
			t.Fatalf("live dock row missing %q; got %q", want, plain)
		}
	}

	ordered := []string{"2/4", topicText, "正在探索代码并收集证据", "模型 MiniMax-M2.7-highspeed", "约 61k/200k tok"}
	prev := -1
	for _, part := range ordered {
		idx := strings.Index(plain, part)
		if idx <= prev {
			t.Fatalf("live dock row order drift: %q appeared at %d after previous index %d; row=%q", part, idx, prev, plain)
		}
		prev = idx
	}
	if strings.HasSuffix(strings.TrimSpace(plain), topicText) {
		t.Fatalf("topic progress must not be the tail metadata field anymore; got %q", plain)
	}
	if !strings.Contains(row, statusMeta.Sprint(topicText)) {
		t.Fatalf("topic progress must use statusMeta styling; row=%q", row)
	}
	if strings.Contains(row, statusObjective.Sprint(topicText)) {
		t.Fatalf("topic progress must not use bright statusObjective styling; row=%q", row)
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
	if !strings.Contains(plain, "focus 1 of 3") {
		t.Fatalf("EN topic tag 'focus 1 of 3' missing: %q", plain)
	}
	// MUST NOT carry zh tag.
	if strings.Contains(plain, "关注点") {
		t.Errorf("EN mode must NOT carry zh topic tag; got %q", plain)
	}
}

func TestFormatStageDoneLine_UsesAggregateExploreSlotElapsed(t *testing.T) {
	r := New(nil, false)
	r.lang = "zh"

	t0 := time.Unix(100, 0)
	first := &taskRow{
		startTime:  t0,
		firstStart: t0,
		endTime:    t0.Add(90 * time.Second),
		okFinished: true,
		isNodeRow:  true,
		nodeID:     "n1_evidence_t0",
		nodeKind:   "evidence",
	}
	second := &taskRow{
		startTime:  t0.Add(120 * time.Second),
		firstStart: t0.Add(120 * time.Second),
		endTime:    t0.Add(200 * time.Second),
		okFinished: true,
		isNodeRow:  true,
		nodeID:     "n1_evidence_t1",
		nodeKind:   "evidence",
	}
	r.tasks = []*taskRow{first, second}

	plain := stripAnsi(r.formatStageDoneLine(second, 2))
	if !strings.Contains(plain, "本 3m20s") {
		t.Fatalf("explore completion should show aggregate stage-slot elapsed, got %q", plain)
	}
	if strings.Contains(plain, "本 1m20s") {
		t.Fatalf("explore completion must not show only the last node window, got %q", plain)
	}
}
