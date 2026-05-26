package repl

import (
	"bufio"
	"errors"
	"fmt"
	"io"
	"os"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/charmbracelet/lipgloss"
	"github.com/mattn/go-runewidth"
	"golang.org/x/sys/unix"
	"golang.org/x/term"
)

var errNativeInputUnavailable = errors.New("native repl input unavailable")

const (
	ansiShowCursor          = "\x1b[?25h"
	ansiEnableBracketed     = "\x1b[?2004h"
	ansiDisableBracketed    = "\x1b[?2004l"
	ansiEraseEntireLine     = "\x1b[2K"
	ansiEraseLineRight      = "\x1b[K"
	nativeEscapeReadTimeout = 25 * time.Millisecond
)

type nativeLineInput struct {
	out               io.Writer
	reader            *bufio.Reader
	fd                int
	prompt            string
	echoTag           string
	history           []string
	histIdx           int
	draft             string
	pastes            []string
	isContinue        bool
	pasteFoldMinChars int
	lang              string
	termWidth         int
	renderedRows      int
	value             []rune
	cursor            int
	showSuggest       bool
	slashSel          int
}

func (r *REPL) readInputNative(prompt string, isContinue bool) (inputResult, error) {
	fd := int(os.Stdin.Fd())
	if !term.IsTerminal(fd) {
		return inputResult{}, errNativeInputUnavailable
	}
	oldState, err := term.MakeRaw(fd)
	if err != nil {
		return inputResult{}, errNativeInputUnavailable
	}
	defer func() {
		_ = term.Restore(fd, oldState)
	}()

	w, _, _ := term.GetSize(fd)
	if w <= 0 {
		w = 80
	}
	foldMin := r.pasteFoldMinChars
	if foldMin <= 0 {
		foldMin = DefaultPasteFoldMinChars
	}
	editor := &nativeLineInput{
		out:               r.out,
		reader:            bufio.NewReader(os.Stdin),
		fd:                fd,
		prompt:            prompt + " ",
		echoTag:           r.echoTag,
		history:           historyReversed(r.historyStrings(), maxHistoryItems),
		histIdx:           -1,
		isContinue:        isContinue,
		pasteFoldMinChars: foldMin,
		lang:              r.language,
		termWidth:         w,
	}
	if !isContinue && r.pendingPaste != "" {
		editor.injectPlaceholder(r.pendingPaste)
		r.pendingPaste = ""
	}
	fmt.Fprint(editor.out, ansiShowCursor, ansiEnableBracketed)
	defer fmt.Fprint(editor.out, ansiDisableBracketed, ansiShowCursor)
	return editor.run()
}

func (e *nativeLineInput) run() (inputResult, error) {
	e.refreshSuggest()
	e.render()
	for {
		b, err := e.reader.ReadByte()
		if err != nil {
			return inputResult{aborted: true}, err
		}
		switch b {
		case 0x03: // Ctrl+C
			e.clearRendered()
			return inputResult{aborted: true}, io.EOF
		case 0x04: // Ctrl+D / delete-forward
			if len(e.value) == 0 {
				e.clearRendered()
				return inputResult{aborted: true}, io.EOF
			}
			e.deleteForward()
		case '\r', '\n':
			if e.showSuggest {
				e.acceptSuggestion(true)
			}
			res, done := e.submit()
			if done {
				return res, nil
			}
		case '\t':
			e.acceptSuggestion(false)
		case 0x7f, 0x08:
			e.backspace()
		case 0x0e: // Ctrl+N
			e.historyNext()
		case 0x10: // Ctrl+P
			e.historyPrev()
		case 0x1b:
			if err := e.handleEscape(); err != nil {
				return inputResult{aborted: true}, err
			}
		default:
			if b < 0x20 {
				// Ignore other controls; raw tabs are handled above.
				break
			}
			r, err := e.readRuneFromFirstByte(b)
			if err != nil {
				return inputResult{aborted: true}, err
			}
			e.insertRunes([]rune{r})
		}
		e.refreshSuggest()
		e.render()
	}
}

func (e *nativeLineInput) readRuneFromFirstByte(first byte) (rune, error) {
	if first < utf8.RuneSelf {
		return rune(first), nil
	}
	buf := []byte{first}
	for !utf8.FullRune(buf) && len(buf) < utf8.UTFMax {
		b, err := e.reader.ReadByte()
		if err != nil {
			return utf8.RuneError, err
		}
		buf = append(buf, b)
	}
	r, _ := utf8.DecodeRune(buf)
	return r, nil
}

func (e *nativeLineInput) handleEscape() error {
	next, ok, err := e.readByteWithTimeout(nativeEscapeReadTimeout)
	if err != nil {
		return err
	}
	if !ok {
		if len(e.value) == 0 {
			e.clearRendered()
			return io.EOF
		}
		e.value = nil
		e.cursor = 0
		e.pastes = nil
		e.showSuggest = false
		return nil
	}
	if next != '[' {
		return nil
	}
	seq, err := e.readCSISequence()
	if err != nil {
		return err
	}
	switch seq {
	case "A":
		if e.showSuggest {
			e.suggestionUp()
		} else {
			e.historyPrev()
		}
	case "B":
		if e.showSuggest {
			e.suggestionDown()
		} else {
			e.historyNext()
		}
	case "C":
		e.moveRight()
	case "D":
		e.moveLeft()
	case "3~":
		e.deleteForward()
	case "200~":
		paste, err := e.readBracketedPaste()
		if err != nil {
			return err
		}
		e.handlePaste(paste)
	}
	return nil
}

func (e *nativeLineInput) readByteWithTimeout(d time.Duration) (byte, bool, error) {
	if e.reader.Buffered() > 0 {
		b, err := e.reader.ReadByte()
		return b, err == nil, err
	}
	var set unix.FdSet
	set.Zero()
	set.Set(e.fd)
	tv := unix.NsecToTimeval(d.Nanoseconds())
	n, err := unix.Select(e.fd+1, &set, nil, nil, &tv)
	if err != nil {
		return 0, false, err
	}
	if n <= 0 || !set.IsSet(e.fd) {
		return 0, false, nil
	}
	b, err := e.reader.ReadByte()
	return b, err == nil, err
}

func (e *nativeLineInput) readCSISequence() (string, error) {
	var b strings.Builder
	for i := 0; i < 16; i++ {
		ch, ok, err := e.readByteWithTimeout(nativeEscapeReadTimeout)
		if err != nil {
			return "", err
		}
		if !ok {
			return b.String(), nil
		}
		b.WriteByte(ch)
		if (ch >= '@' && ch <= '~') || ch == '~' {
			return b.String(), nil
		}
	}
	return b.String(), nil
}

func (e *nativeLineInput) readBracketedPaste() (string, error) {
	var b strings.Builder
	const end = "\x1b[201~"
	for {
		ch, err := e.reader.ReadByte()
		if err != nil {
			return "", err
		}
		b.WriteByte(ch)
		s := b.String()
		if strings.HasSuffix(s, end) {
			return strings.TrimSuffix(s, end), nil
		}
	}
}

func (e *nativeLineInput) submit() (inputResult, bool) {
	raw := string(e.value)
	expanded := e.expand(raw)
	if !hasPrintable(expanded) {
		e.value = nil
		e.cursor = 0
		e.pastes = nil
		e.showSuggest = false
		return inputResult{}, false
	}
	display := raw
	continues := false
	if strings.HasSuffix(strings.TrimRight(display, " \t"), "\\") {
		continues = true
		display = strings.TrimRight(display, " \t")
		display = strings.TrimSuffix(display, "\\")
		expanded = e.expand(display)
	}
	echoBody := expanded
	if echoBody == "" {
		echoBody = display
	}
	e.clearRendered()
	e.printEcho(echoTagLine{
		body:      echoBody,
		continues: continues,
	})
	return inputResult{
		expanded:  expanded,
		display:   display,
		continues: continues,
	}, true
}

type echoTagLine struct {
	body      string
	continues bool
}

func (e *nativeLineInput) printEcho(line echoTagLine) {
	echoStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("51"))
	tagStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("214")).Bold(true)
	divStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("240"))
	if !line.continues {
		fmt.Fprintf(e.out, "  %s\r\n", divStyle.Render("─────────────────────────────────────"))
	}
	echoLines := strings.Split(line.body, "\n")
	for i, ln := range echoLines {
		var rendered string
		if i == 0 {
			rendered = echoStyle.Render("> " + ln)
			if e.echoTag != "" {
				rendered = tagStyle.Render(e.echoTag) + " " + rendered
			}
		} else {
			rendered = echoStyle.Render("  " + ln)
		}
		fmt.Fprintf(e.out, "  %s\r\n", rendered)
	}
}

func (e *nativeLineInput) render() {
	e.clearRendered()
	if w, _, err := term.GetSize(e.fd); err == nil && w > 0 {
		e.termWidth = w
	}
	lines, cursorCol := e.viewLines()
	for i, line := range lines {
		if i > 0 {
			fmt.Fprint(e.out, "\r\n")
		}
		fmt.Fprint(e.out, line, ansiEraseLineRight)
	}
	if len(lines) > 1 {
		fmt.Fprintf(e.out, "\x1b[%dA", len(lines)-1)
	}
	fmt.Fprint(e.out, "\r")
	if cursorCol > 0 {
		fmt.Fprintf(e.out, "\x1b[%dC", cursorCol)
	}
	e.renderedRows = len(lines)
}

func (e *nativeLineInput) clearRendered() {
	if e.renderedRows <= 0 {
		return
	}
	fmt.Fprint(e.out, "\r")
	if e.renderedRows > 1 {
		fmt.Fprintf(e.out, "\x1b[%dA", e.renderedRows-1)
	}
	for i := 0; i < e.renderedRows; i++ {
		fmt.Fprint(e.out, ansiEraseEntireLine, "\r")
		if i < e.renderedRows-1 {
			fmt.Fprint(e.out, "\x1b[1B")
		}
	}
	if e.renderedRows > 1 {
		fmt.Fprintf(e.out, "\x1b[%dA", e.renderedRows-1)
	}
	fmt.Fprint(e.out, "\r")
	e.renderedRows = 0
}

func (e *nativeLineInput) viewLines() ([]string, int) {
	promptStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("51")).Bold(true)
	prompt := promptStyle.Render(e.prompt)
	inputWidth := e.termWidth - inputPromptDisplayWidth(e.prompt)
	if inputWidth < 8 {
		inputWidth = 8
	}
	visible, cursorOffset := nativeInputVisibleWindow(e.value, e.cursor, inputWidth)
	firstLine := prompt + visible
	cursorCol := inputPromptDisplayWidth(e.prompt) + cursorOffset
	if e.showSuggest {
		return append([]string{firstLine}, e.renderSuggestLines()...), cursorCol
	}
	return []string{firstLine}, cursorCol
}

func nativeInputVisibleWindow(value []rune, cursor, maxWidth int) (string, int) {
	if cursor < 0 {
		cursor = 0
	}
	if cursor > len(value) {
		cursor = len(value)
	}
	if maxWidth <= 0 {
		maxWidth = 1
	}
	if runewidth.StringWidth(string(value)) <= maxWidth {
		return string(value), runewidth.StringWidth(string(value[:cursor]))
	}
	start := cursor
	widthBeforeCursor := 0
	for start > 0 {
		w := nativeRuneWidth(value[start-1])
		if widthBeforeCursor+w > maxWidth-1 {
			break
		}
		start--
		widthBeforeCursor += w
	}
	prefix := ""
	if start > 0 {
		prefix = "…"
		widthBeforeCursor++
		for widthBeforeCursor > maxWidth-1 && start < cursor {
			widthBeforeCursor -= nativeRuneWidth(value[start])
			start++
		}
	}
	end := cursor
	used := widthBeforeCursor
	for end < len(value) {
		w := nativeRuneWidth(value[end])
		if used+w > maxWidth {
			break
		}
		used += w
		end++
	}
	return prefix + string(value[start:end]), widthBeforeCursor
}

func nativeRuneWidth(r rune) int {
	w := runewidth.RuneWidth(r)
	if w < 0 {
		return 0
	}
	return w
}

func (e *nativeLineInput) renderSuggestLines() []string {
	matches := e.filterSuggestions()
	if len(matches) == 0 {
		return nil
	}
	selStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("51")).Bold(true)
	nameStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("252"))
	helpStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("240"))
	out := make([]string, 0, len(matches))
	for i, mi := range matches {
		c := slashCommands[mi]
		prefix := "  "
		plainPrefix := "  "
		nm := nameStyle.Render(c.Name)
		if i == e.slashSel {
			prefix = selStyle.Render("▸ ")
			plainPrefix = "▸ "
			nm = selStyle.Render(c.Name)
		}
		plainHead := "  " + plainPrefix + c.Name + "  "
		helpBudget := e.termWidth - runewidth.StringWidth(plainHead)
		out = append(out, "  "+prefix+nm+"  "+helpStyle.Render(nativeClampDisplayWidth(c.Help(e.lang), helpBudget)))
	}
	return out
}

func nativeClampDisplayWidth(s string, maxWidth int) string {
	s = strings.TrimRight(s, " \t")
	if maxWidth <= 0 || runewidth.StringWidth(s) <= maxWidth {
		return s
	}
	if maxWidth <= 1 {
		return "…"
	}
	var b strings.Builder
	used := 0
	for _, r := range s {
		w := nativeRuneWidth(r)
		if used+w > maxWidth-1 {
			break
		}
		b.WriteRune(r)
		used += w
	}
	b.WriteRune('…')
	return b.String()
}

func (e *nativeLineInput) insertRunes(ins []rune) {
	if e.cursor > len(e.value) {
		e.cursor = len(e.value)
	}
	next := make([]rune, 0, len(e.value)+len(ins))
	next = append(next, e.value[:e.cursor]...)
	next = append(next, ins...)
	next = append(next, e.value[e.cursor:]...)
	e.value = next
	e.cursor += len(ins)
}

func (e *nativeLineInput) backspace() {
	if sp, ok := nativeSpanEndingAt(string(e.value), e.cursor); ok {
		e.deleteSpan(sp)
		return
	}
	if e.cursor <= 0 || len(e.value) == 0 {
		return
	}
	e.value = append(e.value[:e.cursor-1], e.value[e.cursor:]...)
	e.cursor--
}

func (e *nativeLineInput) deleteForward() {
	if sp, ok := nativeSpanStartingAt(string(e.value), e.cursor); ok {
		e.deleteSpan(sp)
		return
	}
	if e.cursor < 0 || e.cursor >= len(e.value) {
		return
	}
	e.value = append(e.value[:e.cursor], e.value[e.cursor+1:]...)
}

func (e *nativeLineInput) deleteSpan(sp span) {
	if sp.end > len(e.value) {
		return
	}
	e.value = append(e.value[:sp.start], e.value[sp.end:]...)
	e.cursor = sp.start
}

func (e *nativeLineInput) moveLeft() {
	if sp, ok := nativeSpanAt(string(e.value), e.cursor-1); ok {
		e.cursor = sp.start
		return
	}
	if e.cursor > 0 {
		e.cursor--
	}
}

func (e *nativeLineInput) moveRight() {
	if sp, ok := nativeSpanAt(string(e.value), e.cursor+1); ok {
		e.cursor = sp.end
		return
	}
	if e.cursor < len(e.value) {
		e.cursor++
	}
}

func (e *nativeLineInput) handlePaste(pasted string) {
	chars := utf8.RuneCountInString(pasted)
	shouldFold := strings.Contains(pasted, "\n") || chars >= e.pasteFoldMinChars
	if !shouldFold {
		e.insertRunes([]rune(pasted))
		return
	}
	id := len(e.pastes)
	e.pastes = append(e.pastes, pasted)
	lines := strings.Count(pasted, "\n") + 1
	token := fmt.Sprintf("[Pasted text #%d +%d lines +%d chars]", id, lines, chars)
	e.insertRunes([]rune(token))
}

func (e *nativeLineInput) injectPlaceholder(content string) {
	id := len(e.pastes)
	e.pastes = append(e.pastes, content)
	lines := strings.Count(content, "\n") + 1
	chars := utf8.RuneCountInString(content)
	token := fmt.Sprintf("[Pasted text #%d +%d lines +%d chars] ", id, lines, chars)
	e.value = []rune(token)
	e.cursor = len(e.value)
}

func (e *nativeLineInput) expand(s string) string {
	return placeholderRE.ReplaceAllStringFunc(s, func(tok string) string {
		sub := placeholderRE.FindStringSubmatch(tok)
		if len(sub) < 2 {
			return tok
		}
		id, err := strconv.Atoi(sub[1])
		if err != nil || id < 0 || id >= len(e.pastes) {
			return tok
		}
		return e.pastes[id]
	})
}

func (e *nativeLineInput) historyPrev() {
	if e.isContinue || len(e.history) == 0 {
		return
	}
	if e.histIdx == -1 {
		e.draft = string(e.value)
	}
	if e.histIdx+1 >= len(e.history) {
		return
	}
	e.histIdx++
	e.loadHistoryEntry(e.history[e.histIdx])
}

func (e *nativeLineInput) historyNext() {
	if e.isContinue || e.histIdx == -1 {
		return
	}
	e.histIdx--
	if e.histIdx == -1 {
		e.value = []rune(e.draft)
		e.cursor = len(e.value)
		return
	}
	e.loadHistoryEntry(e.history[e.histIdx])
}

func (e *nativeLineInput) loadHistoryEntry(entry string) {
	if strings.Contains(entry, "\n") {
		e.pastes = nil
		e.value = nil
		e.cursor = 0
		e.injectPlaceholder(entry)
		return
	}
	e.value = []rune(entry)
	e.cursor = len(e.value)
	e.showSuggest = false
}

func (e *nativeLineInput) refreshSuggest() {
	if e.isContinue {
		e.showSuggest = false
		return
	}
	v := string(e.value)
	if !strings.HasPrefix(v, "/") || strings.ContainsAny(v, " \t") {
		e.showSuggest = false
		return
	}
	matches := e.filterSuggestions()
	e.showSuggest = len(matches) > 0
	if e.slashSel >= len(matches) {
		e.slashSel = 0
	}
}

func (e *nativeLineInput) filterSuggestions() []int {
	prefix := string(e.value)
	var out []int
	for i, c := range slashCommands {
		if strings.HasPrefix(c.Name, prefix) {
			out = append(out, i)
		}
	}
	return out
}

func (e *nativeLineInput) suggestionUp() {
	matches := e.filterSuggestions()
	if len(matches) == 0 {
		return
	}
	e.slashSel = (e.slashSel - 1 + len(matches)) % len(matches)
}

func (e *nativeLineInput) suggestionDown() {
	matches := e.filterSuggestions()
	if len(matches) == 0 {
		return
	}
	e.slashSel = (e.slashSel + 1) % len(matches)
}

func (e *nativeLineInput) acceptSuggestion(submit bool) {
	if !e.showSuggest {
		return
	}
	matches := e.filterSuggestions()
	if len(matches) == 0 || e.slashSel >= len(matches) {
		return
	}
	chosen := slashCommands[matches[e.slashSel]].Name
	next := chosen
	if !submit && needsArg(chosen) {
		next += " "
	}
	e.value = []rune(next)
	e.cursor = len(e.value)
	e.showSuggest = false
}

func nativeSpanAt(v string, pos int) (span, bool) {
	for _, sp := range nativeSpans(v) {
		if pos > sp.start && pos < sp.end {
			return sp, true
		}
	}
	return span{}, false
}

func nativeSpanEndingAt(v string, pos int) (span, bool) {
	for _, sp := range nativeSpans(v) {
		if pos == sp.end {
			return sp, true
		}
	}
	return span{}, false
}

func nativeSpanStartingAt(v string, pos int) (span, bool) {
	for _, sp := range nativeSpans(v) {
		if pos == sp.start {
			return sp, true
		}
	}
	return span{}, false
}

func nativeSpans(v string) []span {
	var out []span
	for _, loc := range placeholderRE.FindAllStringIndex(v, -1) {
		startRune := utf8.RuneCountInString(v[:loc[0]])
		endRune := startRune + utf8.RuneCountInString(v[loc[0]:loc[1]])
		out = append(out, span{start: startRune, end: endRune})
	}
	return out
}
