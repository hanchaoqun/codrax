package cmd

import (
	"io"
	"os"

	"golang.org/x/term"
)

// replInputForStdin gives redirected input to the REPL's existing script
// owner. TTY input keeps Config.In nil, preserving the native interactive
// editor and its single stdin owner. This borrows the descriptor without
// reading, buffering, changing its terminal mode, or taking close ownership.
func replInputForStdin(stdin *os.File) io.Reader {
	return replInputForStdinWithTerminalCheck(stdin, term.IsTerminal)
}

func replInputForStdinWithTerminalCheck(stdin *os.File, isTerminal func(int) bool) io.Reader {
	if stdin == nil || isTerminal(int(stdin.Fd())) {
		return nil
	}
	return stdin
}
