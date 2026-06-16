//go:build windows

package repl

import "errors"

var errNativeInputUnavailable = errors.New("native repl input unavailable")

func (r *REPL) readInputNative(prompt string, isContinue bool) (inputResult, error) {
	return inputResult{}, errNativeInputUnavailable
}
