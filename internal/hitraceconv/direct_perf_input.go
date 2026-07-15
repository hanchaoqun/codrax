package hitraceconv

import (
	"context"
	"errors"
	"strings"
)

// perfInputBinding ties a precise format decision to the one immutable input
// view that supplied the probe. The view can be either the complete direct
// input or a bounded standalone payload; built-in readers and external-tool
// leases must consume that view rather than reopening displayPath.
type perfInputBinding struct {
	input       conversionInputView
	inputSize   int64
	displayPath string
	inputFormat perfInputFormat
	stage       conversionInputStage
	kind        perfInputBindingKind
}

type perfInputBindingKind uint8

const (
	perfInputBindingDirect perfInputBindingKind = iota + 1
	perfInputBindingStandaloneHiperf
)

// directPerfInputBinding is a distinct route wrapper, not an alias. This keeps
// standalone payload bindings from being passed into direct provider APIs even
// though both share the same immutable binding core.
type directPerfInputBinding struct {
	perfInputBinding
}

// standaloneHiperfInputBinding is a distinct route wrapper. It prevents a
// bounded profiler payload from entering direct whole-file providers while
// still letting official and raw standalone arms share one exact view.
type standaloneHiperfInputBinding struct {
	perfInputBinding
}

func newDirectPerfInputBinding(input conversionInputView, inputFormat perfInputFormat) (directPerfInputBinding, error) {
	if !simpleperfDirectRequested(inputFormat) {
		path := ""
		if input != nil {
			path = input.DisplayPath()
		}
		return directPerfInputBinding{}, conversionInputFailure(
			ConversionInputCodeInternalContract,
			conversionInputStageDirectPerfRead,
			path,
			errors.New("direct perf input format is not a supported direct route"),
		)
	}
	binding, err := newPerfInputBinding(input, inputFormat, conversionInputStageDirectPerfRead, perfInputBindingDirect)
	if err != nil {
		return directPerfInputBinding{}, err
	}
	return directPerfInputBinding{perfInputBinding: binding}, nil
}

func newStandaloneHiperfInputBinding(input conversionInputView, inputFormat perfInputFormat) (standaloneHiperfInputBinding, error) {
	binding, err := newPerfInputBinding(input, inputFormat, conversionInputStageStandaloneExtract, perfInputBindingStandaloneHiperf)
	if err != nil {
		return standaloneHiperfInputBinding{}, err
	}
	return standaloneHiperfInputBinding{perfInputBinding: binding}, nil
}

func newPerfInputBinding(input conversionInputView, inputFormat perfInputFormat, stage conversionInputStage, kind perfInputBindingKind) (perfInputBinding, error) {
	path := ""
	if input != nil {
		path = input.DisplayPath()
	}
	fail := func(cause error) (perfInputBinding, error) {
		return perfInputBinding{}, conversionInputFailure(
			ConversionInputCodeInternalContract,
			stage,
			path,
			cause,
		)
	}
	if input == nil {
		return fail(errors.New("nil direct perf input view"))
	}
	if !stage.valid() {
		return fail(errors.New("perf input validation stage is invalid"))
	}
	if !inputFormat.valid() {
		return fail(errors.New("perf input format is outside the closed set"))
	}
	if !perfInputBindingContractValid(kind, stage, inputFormat) {
		return fail(errors.New("perf input kind, stage, and format are inconsistent"))
	}
	size := input.Size()
	if size < 0 {
		return fail(errors.New("negative direct perf input size"))
	}
	if strings.TrimSpace(path) == "" {
		return fail(errors.New("perf input display path is empty"))
	}
	return perfInputBinding{
		input:       input,
		inputSize:   size,
		displayPath: path,
		inputFormat: inputFormat,
		stage:       stage,
		kind:        kind,
	}, nil
}

func perfInputBindingContractValid(kind perfInputBindingKind, stage conversionInputStage, inputFormat perfInputFormat) bool {
	switch kind {
	case perfInputBindingDirect:
		return stage == conversionInputStageDirectPerfRead && simpleperfDirectRequested(inputFormat)
	case perfInputBindingStandaloneHiperf:
		return stage == conversionInputStageStandaloneExtract && inputFormat.valid()
	default:
		return false
	}
}

func (binding perfInputBinding) validate() error {
	stage := binding.stage
	if binding.input == nil || binding.inputSize < 0 || !stage.valid() || !binding.inputFormat.valid() ||
		!perfInputBindingContractValid(binding.kind, stage, binding.inputFormat) ||
		binding.input.Size() != binding.inputSize ||
		strings.TrimSpace(binding.displayPath) == "" {
		return conversionInputFailure(
			ConversionInputCodeInternalContract,
			stage,
			binding.displayPath,
			errors.New("invalid perf input binding"),
		)
	}
	return nil
}

func (binding directPerfInputBinding) validate() error {
	if binding.kind != perfInputBindingDirect || binding.stage != conversionInputStageDirectPerfRead || !simpleperfDirectRequested(binding.inputFormat) {
		return conversionInputFailure(
			ConversionInputCodeInternalContract,
			conversionInputStageDirectPerfRead,
			binding.displayPath,
			errors.New("invalid direct perf route binding"),
		)
	}
	return binding.perfInputBinding.validate()
}

func (binding standaloneHiperfInputBinding) validate() error {
	if binding.kind != perfInputBindingStandaloneHiperf || binding.stage != conversionInputStageStandaloneExtract {
		return conversionInputFailure(
			ConversionInputCodeInternalContract,
			conversionInputStageStandaloneExtract,
			binding.displayPath,
			errors.New("invalid standalone HIPERF route binding"),
		)
	}
	return binding.perfInputBinding.validate()
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
