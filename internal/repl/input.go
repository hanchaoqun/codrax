// Bubble Tea–driven interactive input for the REPL.
//
// Responsibilities beyond what bubbles/textinput gives us out of the box:
//
//  1. Up/Down history navigation (seeded from memory.Store).
//  2. Reject Enter on whitespace-only / non-printable-only buffers.
//  3. Slash-command suggestion panel when the buffer starts with "/".
//  4. Bracketed-paste folding: a multi-line or long single-line paste
//     (≥ DefaultPasteFoldMinChars runes, configurable) is replaced
//     inline by a `[Pasted text #N +L lines +C chars]` placeholder
//     token, and the textinput widget treats the whole token as one
//     atomic unit (cursor moves past it, Backspace deletes it whole).
//     On submit, tokens expand back to the original pastes.
//
// Cross-platform: bubbletea handles Windows console, Unix termios, and
// CJK widths; no OS-specific imports belong here.
package repl

import (
	"fmt"
	"io"
	"regexp"
	"strconv"
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/charmbracelet/bubbles/key"
	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"golang.org/x/term"
)

// slashCommands is the canonical autocomplete surface. Kept in sync by
// hand with replCommandAliases' target set in internal/types. Shown
// only when the buffer looks like a bare slash token.
var slashCommands = []struct {
	Name string
	Help string
}{
	{"/help", "show available commands"},
	{"/history", "show recent turns"},
	{"/compact", "compact memory"},
	{"/clear", "wipe conversation memory"},
	{"/log", "attach/show/clear a runtime log"},
	{"/paste", "capture a paste when bracketed paste is stripped (SSH / tmux)"},
	{"/exit", "leave the REPL"},
	{"/quit", "leave the REPL"},
}

// placeholderRE matches a folded-paste token. Group 1 is the id.
var placeholderRE = regexp.MustCompile(`\[Pasted text #(\d+) \+\d+ lines \+\d+ chars\]`)

// DefaultPasteFoldMinChars is the fallback threshold (in Unicode
// runes, not bytes) above which a single-line paste gets folded into
// a placeholder. Multi-line pastes fold unconditionally. Kept as a
// public constant so cmd/root.go can surface the same default in its
// help text, and so tests stay insensitive to yaml plumbing.
//
// Runes, not bytes, because the user-facing setting is in characters
// (pay attention, CJK users: "你好" is 2 chars, 6 bytes). Keeping the
// internal comparison in runes makes the knob's unit match the UI.
const DefaultPasteFoldMinChars = 60

// maxHistoryItems caps how far Up/Down scrolls. Memory.Store.Recent
// is already bounded, but this keeps the model honest even if the
// caller passes a huge slice.
const maxHistoryItems = 100

// inputResult is what the interactive input session returns.
type inputResult struct {
	// expanded is the final text to feed the orchestrator: placeholders
	// are replaced with their original pasted content.
	expanded string
	// display is what the user last saw on screen: placeholders kept
	// as tokens. Used for the "  > …" echo above the answer block.
	display string
	// continues is true when the raw buffer ended with "\" — the
	// outer loop should re-invoke with a continuation prompt.
	continues bool
	// aborted signals Ctrl+C / Ctrl+D / Esc-on-empty.
	aborted bool
}

// inputKeyMap gathers the bindings we intercept before delegating to
// textinput. Keeping them in one place makes cross-platform fiddling
// (e.g. macOS option-arrow quirks) a local edit.
type inputKeyMap struct {
	Submit         key.Binding
	HistoryPrev    key.Binding
	HistoryNext    key.Binding
	Backspace      key.Binding
	DeleteForward  key.Binding
	Tab            key.Binding
	Escape         key.Binding
	Quit           key.Binding
	SuggestionUp   key.Binding
	SuggestionDown key.Binding
	SuggestionTake key.Binding
}

func defaultInputKeys() inputKeyMap {
	return inputKeyMap{
		Submit:         key.NewBinding(key.WithKeys("enter")),
		HistoryPrev:    key.NewBinding(key.WithKeys("up", "ctrl+p")),
		HistoryNext:    key.NewBinding(key.WithKeys("down", "ctrl+n")),
		Backspace:      key.NewBinding(key.WithKeys("backspace", "ctrl+h")),
		DeleteForward:  key.NewBinding(key.WithKeys("delete", "ctrl+d")),
		Tab:            key.NewBinding(key.WithKeys("tab")),
		Escape:         key.NewBinding(key.WithKeys("esc")),
		Quit:           key.NewBinding(key.WithKeys("ctrl+c")),
		SuggestionUp:   key.NewBinding(key.WithKeys("up", "ctrl+p")),
		SuggestionDown: key.NewBinding(key.WithKeys("down", "ctrl+n")),
		SuggestionTake: key.NewBinding(key.WithKeys("tab", "enter")),
	}
}

// inputModel is the Bubble Tea model that backs one readInput call.
type inputModel struct {
	ti                textinput.Model
	keys              inputKeyMap
	history           []string
	histIdx           int      // -1 = fresh draft; 0..len-1 = browsing (0 = newest)
	draft             string   // saved when first entering history
	pastes            []string // pastes[id] → original content
	isContinue        bool     // set for the "…" continuation prompt
	slashSel          int      // selected index in filtered suggestions
	showSuggest       bool
	doneDisplay       string // captured at Submit for echo
	doneExpanded      string
	submitted         bool
	aborted           bool
	continues         bool
	termWidth         int
	pasteFoldMinChars int // per-instance threshold, in runes
}

// pasteSeed is a single pre-captured paste the caller injects into a
// freshly constructed inputModel. Used by the /paste fallback: each
// seeded turn gets the pasted content as the model's first
// placeholder (id=0) with the cursor positioned after the token so
// the user can keep typing around it. Nil means "normal start".
type pasteSeed struct {
	content string
}

func newInputModel(prompt string, history []string, isContinue bool, w, foldMinChars int, seed *pasteSeed) *inputModel {
	ti := textinput.New()
	ti.Prompt = prompt + " "
	ti.PromptStyle = lipgloss.NewStyle().
		Foreground(lipgloss.Color("51")).Bold(true)
	ti.Cursor.Style = lipgloss.NewStyle().
		Foreground(lipgloss.Color("51"))
	// Disable textinput's own suggestion path — we run our own.
	ti.ShowSuggestions = false
	// Remove Up/Down from textinput's keymap so they propagate to us
	// as history nav instead of scrolling its built-in suggestion ring.
	ti.KeyMap.PrevSuggestion = key.NewBinding(key.WithDisabled())
	ti.KeyMap.NextSuggestion = key.NewBinding(key.WithDisabled())
	ti.KeyMap.AcceptSuggestion = key.NewBinding(key.WithDisabled())
	// Let textinput scroll horizontally if the line exceeds width.
	promptWidth := utf8.RuneCountInString(prompt) + 1
	if w > promptWidth+8 {
		ti.Width = w - promptWidth - 2
	}
	ti.Focus()

	// Freshest turn first → pressing Up once yields the most recent.
	hist := historyReversed(history, maxHistoryItems)

	if foldMinChars <= 0 {
		foldMinChars = DefaultPasteFoldMinChars
	}

	m := &inputModel{
		ti:                ti,
		keys:              defaultInputKeys(),
		history:           hist,
		histIdx:           -1,
		isContinue:        isContinue,
		termWidth:         w,
		pasteFoldMinChars: foldMinChars,
	}
	if seed != nil && seed.content != "" {
		m.injectPlaceholder(seed.content)
	}
	return m
}

// injectPlaceholder folds `content` into placeholder id 0 and seeds
// the textinput with that token followed by a single space so the
// caller lands with the cursor ready for additional prose.
func (m *inputModel) injectPlaceholder(content string) {
	id := len(m.pastes)
	m.pastes = append(m.pastes, content)
	lines := strings.Count(content, "\n") + 1
	chars := utf8.RuneCountInString(content)
	token := fmt.Sprintf("[Pasted text #%d +%d lines +%d chars]", id, lines, chars)
	val := token + " "
	m.ti.SetValue(val)
	m.ti.SetCursor(utf8.RuneCountInString(val))
}

// historyReversed returns up to cap entries from newest to oldest.
func historyReversed(src []string, cap int) []string {
	out := make([]string, 0, len(src))
	for i := len(src) - 1; i >= 0; i-- {
		s := strings.TrimSpace(src[i])
		if s == "" {
			continue
		}
		out = append(out, src[i])
		if len(out) >= cap {
			break
		}
	}
	return out
}

func (m *inputModel) Init() tea.Cmd {
	return tea.Batch(
		tea.EnableBracketedPaste,
		textinput.Blink,
	)
}

// Update dispatches on key events. We intercept submit, history,
// suggestion navigation, and placeholder-atomic editing before
// delegating the rest to the underlying textinput.
func (m *inputModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.termWidth = msg.Width
		promptWidth := utf8.RuneCountInString(m.ti.Prompt)
		if msg.Width > promptWidth+8 {
			m.ti.Width = msg.Width - promptWidth - 2
		}
		return m, nil

	case tea.KeyMsg:
		// Bracketed paste is delivered as a KeyRunes msg with Paste=true.
		if msg.Type == tea.KeyRunes && msg.Paste {
			return m, m.handlePaste(string(msg.Runes))
		}

		// When the suggestion panel is visible, it owns Up/Down/Tab/Enter/Esc.
		if m.showSuggest {
			if cmd, handled := m.handleSuggestKey(msg); handled {
				return m, cmd
			}
		}

		// Global keys.
		switch {
		case key.Matches(msg, m.keys.Quit):
			m.aborted = true
			return m, tea.Quit

		case key.Matches(msg, m.keys.Escape):
			if m.ti.Value() == "" {
				m.aborted = true
				return m, tea.Quit
			}
			m.ti.SetValue("")
			m.pastes = nil
			m.showSuggest = false
			m.refreshSuggest()
			return m, nil

		case key.Matches(msg, m.keys.Submit):
			return m, m.handleSubmit()

		case key.Matches(msg, m.keys.HistoryPrev):
			if !m.isContinue && len(m.history) > 0 {
				m.historyPrev()
				return m, nil
			}

		case key.Matches(msg, m.keys.HistoryNext):
			if !m.isContinue && len(m.history) > 0 {
				m.historyNext()
				return m, nil
			}

		case key.Matches(msg, m.keys.Backspace):
			if sp, ok := m.spanEndingAt(m.ti.Position()); ok {
				m.deleteSpan(sp)
				m.refreshSuggest()
				return m, nil
			}

		case key.Matches(msg, m.keys.DeleteForward):
			if sp, ok := m.spanStartingAt(m.ti.Position()); ok {
				m.deleteSpan(sp)
				m.refreshSuggest()
				return m, nil
			}

		case key.Matches(msg, m.keys.Tab):
			// Tab outside a visible suggest panel is a no-op (avoid
			// inserting raw \t which trips many terminals).
			return m, nil
		}

		// Delegate to textinput, then repair cursor position if it
		// landed inside a placeholder span.
		var cmd tea.Cmd
		m.ti, cmd = m.ti.Update(msg)
		m.snapCursorOutOfSpan(msg)
		m.refreshSuggest()
		return m, cmd
	}

	var cmd tea.Cmd
	m.ti, cmd = m.ti.Update(msg)
	return m, cmd
}

func (m *inputModel) View() string {
	if m.submitted || m.aborted {
		return ""
	}
	out := m.ti.View()
	if m.showSuggest {
		out += "\n" + m.renderSuggestPanel()
	}
	return out
}

// handleSubmit finalises the buffer or silently refuses to submit on
// whitespace/non-printable-only content.
func (m *inputModel) handleSubmit() tea.Cmd {
	raw := m.ti.Value()
	expanded := m.expand(raw)
	if !hasPrintable(expanded) {
		// Normalise: drop the empty whitespace so the cursor is clean.
		if raw != "" {
			m.ti.SetValue("")
			m.pastes = nil
			m.showSuggest = false
		}
		return nil
	}
	// Detect trailing "\" continuation (on the display buffer, so a
	// backslash inside a folded paste doesn't trigger it).
	display := raw
	continues := false
	if strings.HasSuffix(strings.TrimRight(display, " \t"), "\\") {
		continues = true
		display = strings.TrimRight(display, " \t")
		display = strings.TrimSuffix(display, "\\")
		// Re-expand without the trailing "\" so it doesn't reach dispatch.
		expanded = m.expand(display)
	}
	m.submitted = true
	m.continues = continues
	m.doneDisplay = display
	m.doneExpanded = expanded
	m.ti.Blur()

	// Persist the echo + divider above the inline viewport, then quit.
	// Skip divider on continuation — the next line's prompt already
	// gives visual separation.
	echoStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("51"))
	divStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("240"))
	cmds := []tea.Cmd{
		tea.Printf("  %s", echoStyle.Render("> "+singleLine(display))),
	}
	if !continues {
		cmds = append(cmds, tea.Printf("  %s", divStyle.Render("─────────────────────────────────────")))
	}
	cmds = append(cmds, tea.Quit)
	return tea.Sequence(cmds...)
}

// handlePaste decides verbatim-vs-folded and mutates the buffer.
// Unit for the threshold is Unicode runes (characters), not bytes,
// so a 60-char Chinese paste and a 60-char ASCII paste both fold.
func (m *inputModel) handlePaste(pasted string) tea.Cmd {
	chars := utf8.RuneCountInString(pasted)
	shouldFold := strings.Contains(pasted, "\n") || chars >= m.pasteFoldMinChars
	if !shouldFold {
		return m.insertAtCursor(pasted)
	}
	id := len(m.pastes)
	m.pastes = append(m.pastes, pasted)
	lines := strings.Count(pasted, "\n") + 1
	token := fmt.Sprintf("[Pasted text #%d +%d lines +%d chars]", id, lines, chars)
	return m.insertAtCursor(token)
}

// insertAtCursor writes text at the current textinput cursor and keeps
// the cursor at the end of the inserted region.
func (m *inputModel) insertAtCursor(text string) tea.Cmd {
	v := []rune(m.ti.Value())
	pos := m.ti.Position()
	if pos > len(v) {
		pos = len(v)
	}
	ins := []rune(text)
	newV := make([]rune, 0, len(v)+len(ins))
	newV = append(newV, v[:pos]...)
	newV = append(newV, ins...)
	newV = append(newV, v[pos:]...)
	m.ti.SetValue(string(newV))
	m.ti.SetCursor(pos + len(ins))
	m.refreshSuggest()
	return nil
}

// expand replaces every placeholder token with its recorded content.
func (m *inputModel) expand(s string) string {
	return placeholderRE.ReplaceAllStringFunc(s, func(tok string) string {
		sub := placeholderRE.FindStringSubmatch(tok)
		if len(sub) < 2 {
			return tok
		}
		id, err := strconv.Atoi(sub[1])
		if err != nil || id < 0 || id >= len(m.pastes) {
			return tok
		}
		return m.pastes[id]
	})
}

// span is a rune-indexed range [start, end) for one placeholder token.
type span struct{ start, end int }

func (m *inputModel) spans() []span {
	v := m.ti.Value()
	var out []span
	for _, loc := range placeholderRE.FindAllStringIndex(v, -1) {
		startRune := utf8.RuneCountInString(v[:loc[0]])
		endRune := startRune + utf8.RuneCountInString(v[loc[0]:loc[1]])
		out = append(out, span{startRune, endRune})
	}
	return out
}

func (m *inputModel) spanAt(pos int) (span, bool) {
	for _, sp := range m.spans() {
		if pos > sp.start && pos < sp.end {
			return sp, true
		}
	}
	return span{}, false
}

func (m *inputModel) spanEndingAt(pos int) (span, bool) {
	for _, sp := range m.spans() {
		if pos == sp.end {
			return sp, true
		}
	}
	return span{}, false
}

func (m *inputModel) spanStartingAt(pos int) (span, bool) {
	for _, sp := range m.spans() {
		if pos == sp.start {
			return sp, true
		}
	}
	return span{}, false
}

func (m *inputModel) deleteSpan(sp span) {
	v := []rune(m.ti.Value())
	if sp.end > len(v) {
		return
	}
	newV := make([]rune, 0, len(v)-(sp.end-sp.start))
	newV = append(newV, v[:sp.start]...)
	newV = append(newV, v[sp.end:]...)
	m.ti.SetValue(string(newV))
	m.ti.SetCursor(sp.start)
}

// snapCursorOutOfSpan is called after textinput processes a key: if
// cursor motion parked us strictly inside a placeholder, nudge to the
// side the motion came from.
func (m *inputModel) snapCursorOutOfSpan(k tea.KeyMsg) {
	sp, ok := m.spanAt(m.ti.Position())
	if !ok {
		return
	}
	switch k.Type {
	case tea.KeyLeft, tea.KeyCtrlB, tea.KeyHome, tea.KeyCtrlA:
		m.ti.SetCursor(sp.start)
	default:
		m.ti.SetCursor(sp.end)
	}
}

// historyPrev and historyNext walk the buffer of prior submissions.
// If a recalled entry contains newlines, fold it into one placeholder
// so the single-line textinput doesn't explode.
func (m *inputModel) historyPrev() {
	if m.histIdx == -1 {
		m.draft = m.ti.Value()
	}
	if m.histIdx+1 >= len(m.history) {
		return
	}
	m.histIdx++
	m.loadHistoryEntry(m.history[m.histIdx])
}

func (m *inputModel) historyNext() {
	if m.histIdx == -1 {
		return
	}
	m.histIdx--
	if m.histIdx == -1 {
		m.ti.SetValue(m.draft)
		m.ti.SetCursor(utf8.RuneCountInString(m.draft))
		return
	}
	m.loadHistoryEntry(m.history[m.histIdx])
}

func (m *inputModel) loadHistoryEntry(entry string) {
	// Replace placeholder state with a fresh slot if the entry is
	// multi-line (a past /log paste or trailing-\ composition).
	if strings.Contains(entry, "\n") {
		id := len(m.pastes)
		m.pastes = append(m.pastes, entry)
		lines := strings.Count(entry, "\n") + 1
		token := fmt.Sprintf("[Pasted text #%d +%d lines +%d chars]",
			id, lines, utf8.RuneCountInString(entry))
		m.ti.SetValue(token)
		m.ti.SetCursor(utf8.RuneCountInString(token))
	} else {
		m.ti.SetValue(entry)
		m.ti.SetCursor(utf8.RuneCountInString(entry))
	}
	m.showSuggest = false
}

// refreshSuggest recomputes the visibility of the slash-command panel.
func (m *inputModel) refreshSuggest() {
	if m.isContinue {
		m.showSuggest = false
		return
	}
	v := m.ti.Value()
	if !strings.HasPrefix(v, "/") {
		m.showSuggest = false
		return
	}
	if strings.ContainsAny(v, " \t") {
		// User has started arguments — no more command-name completion.
		m.showSuggest = false
		return
	}
	matches := m.filterSuggestions(v)
	if len(matches) == 0 {
		m.showSuggest = false
		return
	}
	m.showSuggest = true
	if m.slashSel >= len(matches) {
		m.slashSel = 0
	}
}

func (m *inputModel) filterSuggestions(prefix string) []int {
	var out []int
	for i, c := range slashCommands {
		if strings.HasPrefix(c.Name, prefix) {
			out = append(out, i)
		}
	}
	return out
}

// handleSuggestKey consumes Up/Down/Tab/Enter/Esc while the panel is
// visible. Returns (cmd, true) when it took the event.
func (m *inputModel) handleSuggestKey(msg tea.KeyMsg) (tea.Cmd, bool) {
	matches := m.filterSuggestions(m.ti.Value())
	switch {
	case key.Matches(msg, m.keys.SuggestionUp):
		if len(matches) > 0 {
			m.slashSel = (m.slashSel - 1 + len(matches)) % len(matches)
		}
		return nil, true
	case key.Matches(msg, m.keys.SuggestionDown):
		if len(matches) > 0 {
			m.slashSel = (m.slashSel + 1) % len(matches)
		}
		return nil, true
	case key.Matches(msg, m.keys.SuggestionTake):
		if len(matches) > 0 && m.slashSel < len(matches) {
			chosen := slashCommands[matches[m.slashSel]].Name
			// Tab → accept and keep editing (allow adding args).
			// Enter → accept and submit on the next Enter (matches what
			// users expect from /clear, /exit etc. which take no args).
			if msg.Type == tea.KeyEnter {
				m.ti.SetValue(chosen)
				m.ti.SetCursor(utf8.RuneCountInString(chosen))
				m.showSuggest = false
				return m.handleSubmit(), true
			}
			next := chosen
			if needsArg(chosen) {
				next = chosen + " "
			}
			m.ti.SetValue(next)
			m.ti.SetCursor(utf8.RuneCountInString(next))
			m.showSuggest = false
			return nil, true
		}
	case key.Matches(msg, m.keys.Escape):
		m.showSuggest = false
		return nil, true
	}
	return nil, false
}

func needsArg(cmd string) bool {
	return cmd == "/log"
}

// renderSuggestPanel lays the filtered list below the input.
func (m *inputModel) renderSuggestPanel() string {
	matches := m.filterSuggestions(m.ti.Value())
	if len(matches) == 0 {
		return ""
	}
	selStyle := lipgloss.NewStyle().
		Foreground(lipgloss.Color("51")).Bold(true)
	nameStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("252"))
	helpStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("240"))
	var b strings.Builder
	for i, mi := range matches {
		c := slashCommands[mi]
		var prefix string
		var nm string
		if i == m.slashSel {
			prefix = selStyle.Render("▸ ")
			nm = selStyle.Render(c.Name)
		} else {
			prefix = "  "
			nm = nameStyle.Render(c.Name)
		}
		b.WriteString("  ")
		b.WriteString(prefix)
		b.WriteString(nm)
		b.WriteString("  ")
		b.WriteString(helpStyle.Render(c.Help))
		if i < len(matches)-1 {
			b.WriteByte('\n')
		}
	}
	return b.String()
}

// readInputBubble runs one Bubble Tea session. The outer caller uses
// its return (expanded, continues) to build the multi-line buffer.
func (r *REPL) readInputBubble(prompt string, isContinue bool) (inputResult, error) {
	w, _, _ := term.GetSize(0)
	if w <= 0 {
		w = 80
	}
	hist := r.historyStrings()
	var seed *pasteSeed
	if !isContinue && r.pendingPaste != "" {
		seed = &pasteSeed{content: r.pendingPaste}
		r.pendingPaste = "" // single-use; consumed on inject
	}
	model := newInputModel(prompt, hist, isContinue, w, r.pasteFoldMinChars, seed)

	p := tea.NewProgram(model,
		tea.WithOutput(r.out),
	)
	finalM, err := p.Run()
	if err != nil {
		return inputResult{aborted: true}, err
	}
	fm, _ := finalM.(*inputModel)
	if fm == nil {
		return inputResult{aborted: true}, io.EOF
	}
	if fm.aborted {
		return inputResult{aborted: true}, io.EOF
	}
	return inputResult{
		expanded:  fm.doneExpanded,
		display:   fm.doneDisplay,
		continues: fm.continues,
	}, nil
}

// historyStrings returns past user Requests, oldest first, drawn from
// the memory store when present.
func (r *REPL) historyStrings() []string {
	if r.store == nil {
		return nil
	}
	turns := r.store.Recent()
	out := make([]string, 0, len(turns))
	for _, t := range turns {
		if strings.TrimSpace(t.Request) == "" {
			continue
		}
		out = append(out, t.Request)
	}
	return out
}

// hasPrintable returns true iff the string contains at least one rune
// that is not whitespace and not a control/format/zero-width codepoint.
func hasPrintable(s string) bool {
	for _, r := range s {
		if unicode.IsSpace(r) {
			continue
		}
		if !unicode.IsPrint(r) {
			continue
		}
		return true
	}
	return false
}

// singleLine flattens any stray newlines in the display echo. The
// fold logic already replaces multi-line pastes with a placeholder,
// but a trailing "\" continuation line that we just submitted can
// still contain intermediate \n when accumulated by the outer loop.
func singleLine(s string) string {
	s = strings.ReplaceAll(s, "\r\n", " ")
	s = strings.ReplaceAll(s, "\n", " ")
	return s
}
