//go:build windows

package repl

import (
	"bufio"
	"errors"
	"fmt"
	"io"
	"os"
	"strings"

	"golang.org/x/term"
)

var errNativeInputUnavailable = errors.New("native repl input unavailable")

func (r *REPL) readInputNative(prompt string, isContinue bool) (inputResult, error) {
	fd := int(os.Stdin.Fd())
	if !term.IsTerminal(fd) {
		return inputResult{}, errNativeInputUnavailable
	}
	if !isContinue && r.pendingPaste != "" {
		return inputResult{}, errNativeInputUnavailable
	}
	// Windows IMEs anchor their candidate/preedit UI to the console's real
	// cursor. Bubble Tea's virtual cursor can render correctly while leaving
	// the real cursor at the renderer's line-reset position, which makes CJK
	// composition jump to the left edge. Use the Windows console's cooked line
	// editor for ordinary input so the OS owns cursor placement during IME
	// composition. Paste-placeholder turns still fall back to Bubble Tea above
	// because they need an editable seeded token.
	reader := bufio.NewReader(os.Stdin)
	for {
		if prompt != "" {
			fmt.Fprint(r.out, prompt+" ")
		}
		line, err := reader.ReadString('\n')
		if err != nil && len(line) == 0 {
			if errors.Is(err, io.EOF) {
				return inputResult{aborted: true}, io.EOF
			}
			return inputResult{aborted: true}, err
		}
		display := strings.TrimRight(line, "\r\n")
		expanded := display
		if hasUnresolvedPastePlaceholder(expanded) {
			continue
		}
		if !hasPrintable(expanded) {
			continue
		}
		continues := false
		if strings.HasSuffix(strings.TrimRight(display, " \t"), "\\") {
			continues = true
			display = strings.TrimRight(display, " \t")
			display = strings.TrimSuffix(display, "\\")
			expanded = display
			if hasUnresolvedPastePlaceholder(expanded) || !hasPrintable(expanded) {
				continue
			}
		}
		return inputResult{
			expanded:  expanded,
			display:   display,
			continues: continues,
		}, nil
	}
}
