package repl

import (
	"os"
	"regexp"
	"sort"
	"strings"
	"testing"
	"unicode/utf8"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/mattn/go-runewidth"

	"github.com/hanchaoqun/codrax/internal/types"
)

// TestSlashCommandsMatchCanonicalRegistry enforces the comment above
// the slashCommands literal ("Kept in sync by hand with
// replCommandAliases' target set"). The autocomplete panel is a
// separate hardcoded list from the dispatch aliases map — any
// /command added to one without the other produces a user-visible
// drift (see session 30 /chat rollout, which shipped the dispatch
// but missed the autocomplete surface and /help output twice).
//
// This test fires on every new or removed canonical command,
// catching the mismatch at `go test` time rather than user report.
func TestSlashCommandsMatchCanonicalRegistry(t *testing.T) {
	autocompleteSet := map[string]bool{}
	for _, c := range slashCommands {
		autocompleteSet[c.Name] = true
	}

	canonical := types.CanonicalREPLCommands()
	sort.Strings(canonical)

	var missing []string
	for _, cmd := range canonical {
		if !autocompleteSet[cmd] {
			missing = append(missing, cmd)
		}
	}
	if len(missing) > 0 {
		t.Errorf("slashCommands autocomplete list drifted from replCommandAliases target set.\n"+
			"Missing from internal/repl/input.go :: slashCommands: %v\n"+
			"Either add them (with a short help string) or — if a command is deliberately hidden from the panel — document the exclusion above this test.",
			missing)
	}
}

// TestHandleSlashDispatchMatchesRegistry is the stronger twin of the
// previous test. It guards against the far more dangerous drift
// where handleSlash grows a new `case "/foo":` branch but the author
// forgets to register /foo in replCommandAliases — which in the real
// REPL Loop means NormalizeREPLCommandAlias returns "" for /foo and
// the line falls through to r.dispatch(), sending the slash command
// to the pipeline as if it were a user question.
//
// Session 33 B1 shipped /mode, /plan, /approve, /reject without
// registering any of them; only tests calling handleXCmd directly
// kept working. Session 35 discovered this while rolling out /verify
// and /worktree. This test ensures no future addition repeats it.
//
// Implementation: parse repl.go's handleSlash function literal for
// `case "/xyz":` tokens and compare against the canonical registry.
// A minor parse failure (comment removed, syntax change) would also
// surface here — acceptable given the alternative is silent breakage.
func TestHandleSlashDispatchMatchesRegistry(t *testing.T) {
	// Read repl.go and extract every `case "/<name>":` between the
	// `func (r *REPL) handleSlash(` marker and the next closing brace
	// at column 0. Skip the fall-through "/exit"/"/quit" alias pairs
	// that share a case body.
	data, err := os.ReadFile("repl.go")
	if err != nil {
		t.Fatalf("read repl.go: %v", err)
	}
	source := strings.ReplaceAll(string(data), "\r\n", "\n")
	const startMarker = "func (r *REPL) handleSlash("
	start := strings.Index(source, startMarker)
	if start < 0 {
		t.Fatalf("could not locate %q in repl.go - refactor?", startMarker)
	}
	body := source[start:]
	end := strings.Index(body, "\n}\n")
	if end < 0 {
		t.Fatalf("could not locate end of handleSlash")
	}
	body = body[:end]

	caseRE := regexp.MustCompile(`case\s+"(/[a-zA-Z]+)"`)
	// Also catch multi-case lines like `case "/exit", "/quit":`.
	multiCaseRE := regexp.MustCompile(`"(/[a-zA-Z]+)"`)
	lines := strings.Split(body, "\n")
	dispatchCommands := map[string]bool{}
	for _, ln := range lines {
		trimmed := strings.TrimSpace(ln)
		if !strings.HasPrefix(trimmed, "case ") {
			continue
		}
		for _, m := range multiCaseRE.FindAllStringSubmatch(trimmed, -1) {
			dispatchCommands[m[1]] = true
		}
		_ = caseRE // retained for clarity; multiCaseRE subsumes it
	}
	if len(dispatchCommands) == 0 {
		t.Fatal("extracted zero /commands from handleSlash — parser broke")
	}

	canonicalSet := map[string]bool{}
	for _, c := range types.CanonicalREPLCommands() {
		canonicalSet[c] = true
	}

	var missing []string
	for cmd := range dispatchCommands {
		if !canonicalSet[cmd] {
			missing = append(missing, cmd)
		}
	}
	sort.Strings(missing)
	if len(missing) > 0 {
		t.Errorf("handleSlash dispatches these commands but replCommandAliases doesn't register them.\n"+
			"In the real REPL Loop, NormalizeREPLCommandAlias returns \"\" for an unregistered command,\n"+
			"so the line falls through to the analysis pipeline — the dispatch case is DEAD.\n"+
			"Missing from internal/types/conversation.go :: replCommandAliases: %v\n"+
			"Add both '/foo' and '\\\\foo' entries mapping to '/foo'.",
			missing)
	}
}

// These tests exercise the input model's pure logic (placeholder
// atomicity, history navigation, slash suggestion visibility,
// submit-blocking) without running a tea.Program. Running a program
// requires a TTY; the model's Update method is a normal function we
// can drive directly.

func newTestModel() *inputModel {
	return newInputModel("❯❯", nil, false, 80, 0, nil, "") // 0 → DefaultPasteFoldMinChars
}

func TestInputModelWidthUsesDisplayColumnsForWidePrompt(t *testing.T) {
	const termWidth = 40
	m := newInputModel("[focus:框架] ❯❯", nil, false, termWidth, 0, nil, "")
	want := termWidth - inputPromptDisplayWidth(m.ti.Prompt) - 2
	if m.ti.Width != want {
		t.Fatalf("initial textinput width=%d, want display-column width %d", m.ti.Width, want)
	}
	runeBased := termWidth - utf8.RuneCountInString(m.ti.Prompt) - 2
	if want == runeBased {
		t.Fatalf("test prompt must distinguish rune count from display width; both=%d", want)
	}

	m.Update(tea.WindowSizeMsg{Width: 48, Height: 24})
	want = 48 - inputPromptDisplayWidth(m.ti.Prompt) - 2
	if m.ti.Width != want {
		t.Fatalf("resized textinput width=%d, want display-column width %d", m.ti.Width, want)
	}
}

func TestNativeInputVisibleWindowUsesCJKDisplayWidth(t *testing.T) {
	value := []rune("abc你好def")
	visible, cursorCol := nativeInputVisibleWindow(value, 5, 7)
	if visible != "…c你好d" {
		t.Fatalf("visible window=%q, want %q", visible, "…c你好d")
	}
	if cursorCol != 6 {
		t.Fatalf("cursor display column=%d, want 6", cursorCol)
	}
}

func TestNativeInputVisibleWindowClipsLongCJKPrefix(t *testing.T) {
	value := []rune("一二三四五六")
	visible, cursorCol := nativeInputVisibleWindow(value, len(value), 6)
	if visible != "…五六" {
		t.Fatalf("visible window=%q, want ellipsis plus tail", visible)
	}
	if cursorCol != 5 {
		t.Fatalf("cursor display column=%d, want 5", cursorCol)
	}
}

func TestNativeClampDisplayWidthUsesDisplayColumns(t *testing.T) {
	if got := nativeClampDisplayWidth("中文abc", 5); got != "中文…" {
		t.Fatalf("nativeClampDisplayWidth=%q, want %q", got, "中文…")
	}
	clamped := nativeClampDisplayWidth("中文abc", 5)
	if width := runewidth.StringWidth(clamped); width > 5 {
		t.Fatalf("clamped string still wider than budget: %q width=%d", clamped, width)
	}
}

// Helper: send a rune-only key (e.g. ASCII char) through Update.
func sendRunes(m *inputModel, runes ...rune) {
	m.Update(tea.KeyMsg(tea.Key{Type: tea.KeyRunes, Runes: runes}))
}

// Helper: send a simple control key by type.
func sendType(m *inputModel, t tea.KeyType) {
	m.Update(tea.KeyMsg(tea.Key{Type: t}))
}

func TestHasPrintable(t *testing.T) {
	cases := []struct {
		in   string
		want bool
	}{
		{"", false},
		{"   ", false},
		{"\t\n ​", false}, // zero-width space is not printable
		{"hello", true},
		{"  你好  ", true},
		{"​ hi", true}, // "hi" is printable
	}
	for _, c := range cases {
		if got := hasPrintable(c.in); got != c.want {
			t.Errorf("hasPrintable(%q) = %v, want %v", c.in, got, c.want)
		}
	}
}

func TestPasteFolding_MultiLine_GetsToken(t *testing.T) {
	m := newTestModel()
	pasted := "line1\nline2\nline3"
	m.Update(tea.KeyMsg(tea.Key{Type: tea.KeyRunes, Paste: true, Runes: []rune(pasted)}))

	buf := m.ti.Value()
	if !placeholderRE.MatchString(buf) {
		t.Fatalf("expected placeholder in buffer, got %q", buf)
	}
	if len(m.pastes) != 1 || m.pastes[0] != pasted {
		t.Fatalf("paste not stored, got pastes=%v", m.pastes)
	}
	// Expand must restore the original content.
	if got := m.expand(buf); got != pasted {
		t.Errorf("expand() = %q, want %q", got, pasted)
	}
}

func TestPasteFolding_ShortSingleLine_Verbatim(t *testing.T) {
	m := newTestModel()
	pasted := "foo"
	m.Update(tea.KeyMsg(tea.Key{Type: tea.KeyRunes, Paste: true, Runes: []rune(pasted)}))

	if buf := m.ti.Value(); buf != "foo" {
		t.Errorf("short paste should be verbatim, got %q", buf)
	}
	if len(m.pastes) != 0 {
		t.Errorf("short paste should not register placeholder, got %v", m.pastes)
	}
}

func TestPasteFolding_SingleLineAtDefaultThreshold_Folds(t *testing.T) {
	// Default DefaultPasteFoldMinChars = 120. A 130-rune single-line
	// ASCII paste must fold; a 119-rune paste must not.
	m := newTestModel()
	long := strings.Repeat("a", 130)
	m.Update(tea.KeyMsg(tea.Key{Type: tea.KeyRunes, Paste: true, Runes: []rune(long)}))
	if !placeholderRE.MatchString(m.ti.Value()) {
		t.Errorf("130-char paste should fold at default threshold, buf=%q", m.ti.Value())
	}

	m2 := newTestModel()
	short := strings.Repeat("b", 119)
	m2.Update(tea.KeyMsg(tea.Key{Type: tea.KeyRunes, Paste: true, Runes: []rune(short)}))
	if placeholderRE.MatchString(m2.ti.Value()) {
		t.Errorf("119-char paste must not fold, buf=%q", m2.ti.Value())
	}
}

func TestPasteFolding_CJK_CountsRunesNotBytes(t *testing.T) {
	// 60 Chinese chars = 60 runes (180 bytes in UTF-8). Under the
	// 120-char floor, so must NOT fold.
	m := newTestModel()
	cjk := strings.Repeat("字", 60)
	m.Update(tea.KeyMsg(tea.Key{Type: tea.KeyRunes, Paste: true, Runes: []rune(cjk)}))
	if placeholderRE.MatchString(m.ti.Value()) {
		t.Errorf("60 CJK chars (< 120) must not fold, buf=%q", m.ti.Value())
	}

	// 125 chars → fold.
	m2 := newTestModel()
	big := strings.Repeat("字", 125)
	m2.Update(tea.KeyMsg(tea.Key{Type: tea.KeyRunes, Paste: true, Runes: []rune(big)}))
	if !placeholderRE.MatchString(m2.ti.Value()) {
		t.Errorf("125 CJK chars must fold, buf=%q", m2.ti.Value())
	}
}

func TestPasteFolding_CustomThreshold_Respected(t *testing.T) {
	// Caller-supplied threshold overrides the default.
	m := newInputModel("❯❯", nil, false, 80, 10, nil, "") // fold at ≥10 chars
	m.Update(tea.KeyMsg(tea.Key{Type: tea.KeyRunes, Paste: true, Runes: []rune("abcdefghij")}))
	if !placeholderRE.MatchString(m.ti.Value()) {
		t.Errorf("10-char paste at custom threshold=10 should fold, buf=%q", m.ti.Value())
	}

	m2 := newInputModel("❯❯", nil, false, 80, 10, nil, "")
	m2.Update(tea.KeyMsg(tea.Key{Type: tea.KeyRunes, Paste: true, Runes: []rune("abcdef")}))
	if placeholderRE.MatchString(m2.ti.Value()) {
		t.Errorf("6-char paste under threshold=10 must not fold, buf=%q", m2.ti.Value())
	}
}

func TestPasteFolding_BackspaceDeletesWholeToken(t *testing.T) {
	m := newTestModel()
	sendRunes(m, 'h', 'i', ' ')
	m.Update(tea.KeyMsg(tea.Key{Type: tea.KeyRunes, Paste: true, Runes: []rune("a\nb\nc")}))
	sendRunes(m, '!')
	buf := m.ti.Value()
	if !strings.Contains(buf, "[Pasted text #0") || !strings.HasPrefix(buf, "hi ") || !strings.HasSuffix(buf, "!") {
		t.Fatalf("unexpected buffer before delete: %q", buf)
	}

	// Cursor is right after the '!'. Backspace removes the '!'.
	sendType(m, tea.KeyBackspace)
	// Second backspace should remove the whole placeholder atomically.
	sendType(m, tea.KeyBackspace)

	if got := m.ti.Value(); got != "hi " {
		t.Errorf("after atomic delete got %q, want %q", got, "hi ")
	}
}

func TestCursorSnap_LeftInSideSpan_SnapsToStart(t *testing.T) {
	m := newTestModel()
	m.Update(tea.KeyMsg(tea.Key{Type: tea.KeyRunes, Paste: true, Runes: []rune("a\nb\nc")}))
	// Move left once: textinput lands inside the token; our snap
	// should pull the cursor to token.start.
	sendType(m, tea.KeyLeft)

	spans := m.spans()
	if len(spans) != 1 {
		t.Fatalf("expected one span, got %d", len(spans))
	}
	if got := m.ti.Position(); got != spans[0].start {
		t.Errorf("cursor = %d, expected snap to start %d", got, spans[0].start)
	}
}

func TestHistoryPrevNext_RestoresDraft(t *testing.T) {
	m := newInputModel("❯❯", []string{"oldest", "middle", "newest"}, false, 80, 0, nil, "")
	sendRunes(m, 'd', 'r', 'a', 'f', 't')

	// Up → newest
	sendType(m, tea.KeyUp)
	if got := m.ti.Value(); got != "newest" {
		t.Errorf("first up got %q, want 'newest'", got)
	}
	sendType(m, tea.KeyUp)
	if got := m.ti.Value(); got != "middle" {
		t.Errorf("second up got %q, want 'middle'", got)
	}
	sendType(m, tea.KeyUp)
	if got := m.ti.Value(); got != "oldest" {
		t.Errorf("third up got %q, want 'oldest'", got)
	}
	// One more Up should clamp (still oldest).
	sendType(m, tea.KeyUp)
	if got := m.ti.Value(); got != "oldest" {
		t.Errorf("clamp up got %q, want 'oldest'", got)
	}
	// Down restores intermediate entries and then the draft.
	sendType(m, tea.KeyDown)
	if got := m.ti.Value(); got != "middle" {
		t.Errorf("down 1 got %q, want 'middle'", got)
	}
	sendType(m, tea.KeyDown)
	if got := m.ti.Value(); got != "newest" {
		t.Errorf("down 2 got %q, want 'newest'", got)
	}
	sendType(m, tea.KeyDown)
	if got := m.ti.Value(); got != "draft" {
		t.Errorf("down 3 should restore draft, got %q", got)
	}
}

func TestHistory_MultiLineEntryBecomesPlaceholder(t *testing.T) {
	m := newInputModel("❯❯", []string{"single-line", "multi\nline\nentry"}, false, 80, 0, nil, "")
	// Up → newest is "multi\nline\nentry", should fold.
	sendType(m, tea.KeyUp)
	buf := m.ti.Value()
	if !placeholderRE.MatchString(buf) {
		t.Fatalf("multi-line history entry should fold to placeholder, got %q", buf)
	}
	if expanded := m.expand(buf); expanded != "multi\nline\nentry" {
		t.Errorf("expand = %q", expanded)
	}
}

func TestSlashSuggest_AppearsOnSlash_DisappearsOnSpace(t *testing.T) {
	m := newTestModel()
	sendRunes(m, '/')
	if !m.showSuggest {
		t.Fatal("expected suggestion panel after '/'")
	}
	sendRunes(m, 'h')
	if !m.showSuggest {
		t.Fatal("expected suggestion panel after '/h'")
	}
	sendRunes(m, ' ')
	if m.showSuggest {
		t.Fatal("space should hide the panel")
	}
}

func TestSlashSuggest_HtraceSubcommandsAfterSpace(t *testing.T) {
	m := newTestModel()
	for _, r := range "/htrace " {
		sendRunes(m, r)
	}
	if !m.showSuggest {
		t.Fatal("expected /htrace subcommand suggestions after a trailing space")
	}
	var found bool
	for _, suggestion := range m.filterSuggestions(m.ti.Value()) {
		if suggestion.display == "/htrace convert <binary> [out.systrace]" {
			found = true
		}
	}
	if !found {
		t.Fatalf("convert subcommand not suggested for %q", m.ti.Value())
	}
}

func TestSlashSuggest_WorkflowWriteRunSubcommands(t *testing.T) {
	m := newTestModel()
	for _, r := range "/workflow " {
		sendRunes(m, r)
	}
	if !m.showSuggest {
		t.Fatal("expected /workflow subcommand suggestions after a trailing space")
	}
	got := map[string]bool{}
	for _, suggestion := range m.filterSuggestions(m.ti.Value()) {
		got[suggestion.display] = true
	}
	for _, want := range []string{
		"/workflow show",
		"/workflow show <run-id>",
		"/workflow list",
		"/workflow resume [run-id]",
		"/workflow clear [run-id]",
		"/workflow cancel",
	} {
		if !got[want] {
			t.Fatalf("workflow subcommand %q not suggested; got %+v", want, got)
		}
	}
}

func TestSubmit_BlocksWhitespaceOnly(t *testing.T) {
	m := newTestModel()
	sendRunes(m, ' ', '\t', ' ')
	cmd := m.handleSubmit()
	if cmd != nil {
		t.Error("whitespace-only submit should not return a quit command")
	}
	if m.submitted {
		t.Error("model should not be marked submitted for whitespace-only input")
	}
}

func TestSubmit_TrailingBackslashContinues(t *testing.T) {
	m := newTestModel()
	sendRunes(m, 'a', 'b', 'c', '\\')
	_ = m.handleSubmit()
	if !m.continues {
		t.Error("trailing \\ should set continues=true")
	}
	if m.doneExpanded != "abc" {
		t.Errorf("doneExpanded = %q, want %q", m.doneExpanded, "abc")
	}
}

func TestSubmit_ExpandsPlaceholdersBeforeDispatch(t *testing.T) {
	m := newTestModel()
	sendRunes(m, 'p', 'r', 'e', ' ')
	m.Update(tea.KeyMsg(tea.Key{Type: tea.KeyRunes, Paste: true, Runes: []rune("X\nY\nZ")}))
	sendRunes(m, ' ', 'p', 'o', 's', 't')
	_ = m.handleSubmit()
	wantExpanded := "pre X\nY\nZ post"
	if m.doneExpanded != wantExpanded {
		t.Errorf("doneExpanded = %q, want %q", m.doneExpanded, wantExpanded)
	}
	if !strings.Contains(m.doneDisplay, "[Pasted text #0") {
		t.Errorf("doneDisplay should keep placeholder, got %q", m.doneDisplay)
	}
}

func TestSubmit_RejectsUnresolvedPastePlaceholder(t *testing.T) {
	m := newTestModel()
	sendRunes(m, []rune("[Pasted text #0 +1 lines +802 chars]")...)

	cmd := m.handleSubmit()
	if cmd != nil {
		t.Fatal("unresolved paste placeholder must not submit")
	}
	if m.submitted {
		t.Fatal("unresolved paste placeholder marked input as submitted")
	}
	if got := m.doneExpanded; got != "" {
		t.Fatalf("doneExpanded=%q, want empty", got)
	}
}

func TestNativeSubmit_RejectsUnresolvedPastePlaceholder(t *testing.T) {
	e := &nativeLineInput{
		value: []rune("[Pasted text #0 +1 lines +802 chars]"),
	}

	res, done := e.submit()
	if done {
		t.Fatalf("native submit should reject unresolved placeholder, got %+v", res)
	}
	if strings.TrimSpace(string(e.value)) != "" {
		t.Fatalf("native input should clear unresolved placeholder, got %q", string(e.value))
	}
}

func TestHistoryReversed_TrimsAndCaps(t *testing.T) {
	src := []string{"a", "b", "  ", "c", ""}
	got := historyReversed(src, 100)
	want := []string{"c", "b", "a"}
	if strings.Join(got, ",") != strings.Join(want, ",") {
		t.Errorf("historyReversed = %v, want %v", got, want)
	}

	cappedSrc := make([]string, 0, 200)
	for i := 0; i < 150; i++ {
		cappedSrc = append(cappedSrc, "x")
	}
	if got := historyReversed(cappedSrc, 50); len(got) != 50 {
		t.Errorf("cap not applied, got %d", len(got))
	}
}

func TestHistoryRecallMultilineEntryCreatesExpandablePlaceholder(t *testing.T) {
	expanded := "请分析下面这段长文本:\n第一行原文\n第二行原文"
	m := newInputModel("❯❯", []string{expanded}, false, 80, 0, nil, "")

	m.historyPrev()
	if got := m.ti.Value(); !strings.Contains(got, "[Pasted text #0") {
		t.Fatalf("history UI should fold multiline recall into a placeholder, got %q", got)
	}
	_ = m.handleSubmit()
	if m.doneExpanded != expanded {
		t.Fatalf("doneExpanded=%q, want original expanded history %q", m.doneExpanded, expanded)
	}
	if !strings.Contains(m.doneDisplay, "[Pasted text #0") {
		t.Fatalf("doneDisplay should keep folded placeholder, got %q", m.doneDisplay)
	}
}

func TestPasteSeed_InjectsAsFirstPlaceholder(t *testing.T) {
	content := "line A\nline B\nline C"
	m := newInputModel("❯❯", nil, false, 80, 0, &pasteSeed{content: content}, "")

	buf := m.ti.Value()
	if !placeholderRE.MatchString(buf) {
		t.Fatalf("seed should produce a placeholder token, buf=%q", buf)
	}
	if !strings.HasSuffix(buf, " ") {
		t.Errorf("seed should leave a trailing space for continued typing, buf=%q", buf)
	}
	if len(m.pastes) != 1 || m.pastes[0] != content {
		t.Errorf("seeded paste not stored in pastes[0], got %v", m.pastes)
	}
	if exp := m.expand(buf); strings.TrimRight(exp, " ") != content {
		t.Errorf("expand(buf) = %q, want content", exp)
	}
}

func TestSpans_CorrectRuneBoundaries(t *testing.T) {
	m := newTestModel()
	sendRunes(m, []rune("你好")...)
	m.Update(tea.KeyMsg(tea.Key{Type: tea.KeyRunes, Paste: true, Runes: []rune("p\nq")}))
	sendRunes(m, []rune("世界")...)
	spans := m.spans()
	if len(spans) != 1 {
		t.Fatalf("expected 1 span, got %d (buf=%q)", len(spans), m.ti.Value())
	}
	// 你好 = 2 runes; placeholder = N runes; 世界 = 2 runes.
	if spans[0].start != 2 {
		t.Errorf("start = %d, want 2", spans[0].start)
	}
	buf := m.ti.Value()
	if spans[0].end != len([]rune(buf))-2 {
		t.Errorf("end = %d, want %d", spans[0].end, len([]rune(buf))-2)
	}
}
