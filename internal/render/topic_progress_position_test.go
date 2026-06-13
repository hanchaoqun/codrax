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

// TestFormatStageDoneLine_TopicOrdinalHidden pins the UX contract:
// ordinal investigation-unit labels ("第 N 个调查单元，共 M 个") are not stage
// progress.
// They belong in the topic-detail block, not in the top status or
// completion rows where users read them as "done N/M".
func TestFormatStageDoneLine_TopicOrdinalHidden(t *testing.T) {
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

	if strings.Contains(plain, "调查单元") || strings.Contains(plain, "关注点") || strings.Contains(plain, "focus") {
		t.Fatalf("topic ordinal must stay out of stage completion row: %q", plain)
	}
	if !strings.Contains(plain, "已完成证据收集") {
		t.Fatalf("stage-done label missing: %q", plain)
	}
	if !strings.Contains(plain, "✓") {
		t.Fatalf("success glyph missing: %q", plain)
	}
}

// TestComposeDockRow2_TopicOrdinalHidden pins the live dock row:
// even if legacy state.topicProgress is populated, the top row must
// not render it.
func TestComposeDockRow2_TopicOrdinalHidden(t *testing.T) {
	pterm.EnableColor()
	defer pterm.DisableColor()

	topicText := "第 2 个调查单元，共 2 个"
	row := composeDockRow2(dockRowState{
		stageProgress:         "2/4",
		topicProgress:         topicText,
		stageLabel:            "正在收集证据",
		modelID:               "MiniMax-M2.7-highspeed",
		contextTokensEstimate: 61000,
		contextWindowTokens:   200000,
		lang:                  "zh",
	})
	plain := stripAnsiEscapes(row)

	for _, want := range []string{"2/4", "正在收集证据", "模型 MiniMax-M2.7-highspeed", "约 61k/200k tok"} {
		if !strings.Contains(plain, want) {
			t.Fatalf("live dock row missing %q; got %q", want, plain)
		}
	}
	if strings.Contains(plain, topicText) || strings.Contains(plain, "调查单元") || strings.Contains(plain, "关注点") {
		t.Fatalf("topic ordinal must stay out of live dock row; got %q", plain)
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
	if strings.Contains(plain, "调查单元") || strings.Contains(plain, "关注点") || strings.Contains(plain, "focus") {
		t.Errorf("single-sub_topic row must NOT carry topic tag; got %q", plain)
	}

	// (b) Non-evidence node kind: no tag (even with topicTotal=2).
	row = base
	row.nodeKind = "validate"
	row.nodeID = "n2_validate"
	plain = stripAnsi(r.formatStageDoneLine(&row, 2))
	if strings.Contains(plain, "调查单元") || strings.Contains(plain, "关注点") || strings.Contains(plain, "focus") {
		t.Errorf("non-evidence node must NOT carry topic tag; got %q", plain)
	}

	// (c) Non-node stage row (isNodeRow=false): no tag.
	row = base
	row.isNodeRow = false
	row.nodeID = ""
	row.nodeKind = ""
	plain = stripAnsi(r.formatStageDoneLine(&row, 2))
	if strings.Contains(plain, "调查单元") || strings.Contains(plain, "关注点") || strings.Contains(plain, "focus") {
		t.Errorf("non-node stage row must NOT carry topic tag; got %q", plain)
	}
}

// TestFormatStageDoneLine_TopicTagEnglishHidden pins the same
// hidden-ordinal contract for EN mode.
func TestFormatStageDoneLine_TopicTagEnglishHidden(t *testing.T) {
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
	if strings.Contains(plain, "investigation unit") || strings.Contains(plain, "focus") || strings.Contains(plain, "关注点") {
		t.Fatalf("topic ordinal must stay out of EN completion row; got %q", plain)
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
