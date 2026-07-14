package hitraceconv

import (
	"context"
	"errors"
	"strings"
)

// directPerfInputBinding ties the route's precise format decision to the one
// immutable input view that supplied the probe. DisplayPath is retained only
// for diagnostics and the still-open SOURCE-GEN-B external-tool lane; built-in
// readers must consume input directly.
type directPerfInputBinding struct {
	input       conversionInputView
	inputSize   int64
	displayPath string
	inputFormat perfInputFormat
}

func newDirectPerfInputBinding(input conversionInputView, inputFormat perfInputFormat) (directPerfInputBinding, error) {
	path := ""
	if input != nil {
		path = input.DisplayPath()
	}
	fail := func(cause error) (directPerfInputBinding, error) {
		return directPerfInputBinding{}, conversionInputFailure(
			ConversionInputCodeInternalContract,
			conversionInputStageDirectPerfRead,
			path,
			cause,
		)
	}
	if input == nil {
		return fail(errors.New("nil direct perf input view"))
	}
	size := input.Size()
	if size < 0 {
		return fail(errors.New("negative direct perf input size"))
	}
	if !simpleperfDirectRequested(inputFormat) {
		return fail(errors.New("direct perf input format is not a supported direct route"))
	}
	if strings.TrimSpace(path) == "" {
		return fail(errors.New("direct perf input display path is empty"))
	}
	return directPerfInputBinding{
		input:       input,
		inputSize:   size,
		displayPath: path,
		inputFormat: inputFormat,
	}, nil
}

func (binding directPerfInputBinding) validate() error {
	if binding.input == nil || binding.inputSize < 0 ||
		binding.input.Size() != binding.inputSize ||
		strings.TrimSpace(binding.displayPath) == "" ||
		!simpleperfDirectRequested(binding.inputFormat) {
		return conversionInputFailure(
			ConversionInputCodeInternalContract,
			conversionInputStageDirectPerfRead,
			binding.displayPath,
			errors.New("invalid direct perf input binding"),
		)
	}
	return nil
}

func directPerfInputBoundaryError(err error) bool {
	if err == nil {
		return false
	}
	if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
		return true
	}
	var inputErr *ConversionInputError
	return errors.As(err, &inputErr)
}
