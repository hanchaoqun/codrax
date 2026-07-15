package hitraceconv

import (
	"errors"
	"fmt"
	"io"
	"os"
	"strings"
)

// standaloneHiperfPayloadView binds one scanner-authenticated HIPERF_DATA
// payload to the immutable container generation that supplied its header.
// Reads are clipped to the payload; neither the profiler header nor adjacent
// standalone records can enter format detection, raw parsing, or a child tool.
type standaloneHiperfPayloadView struct {
	parent      conversionInputView
	parentSize  int64
	segment     standaloneSegment
	payloadBase int64
	payloadSize int64
	displayPath string
}

func newStandaloneHiperfPayloadView(
	inventory standaloneSegmentInventory,
	segmentIndex int,
	displayPath string,
) (*standaloneHiperfPayloadView, error) {
	stage := conversionInputStageStandaloneExtract
	path := strings.TrimSpace(displayPath)
	fail := func(cause error) (*standaloneHiperfPayloadView, error) {
		return nil, conversionInputFailure(ConversionInputCodeInternalContract, stage, path, cause)
	}
	if inventory.input == nil || inventory.inputSize < 0 || inventory.input.Size() != inventory.inputSize {
		return fail(errors.New("standalone HIPERF payload has no immutable parent authority"))
	}
	if segmentIndex < 0 || segmentIndex >= len(inventory.segments) {
		return fail(fmt.Errorf("standalone HIPERF segment index is outside inventory: %d", segmentIndex))
	}
	if path == "" {
		return fail(errors.New("standalone HIPERF public sidecar path is empty"))
	}
	segment := inventory.segments[segmentIndex]
	if segment.DataType != profilerDataTypeHiperf || !standaloneSegmentRangeValid(segment, inventory.inputSize) ||
		segment.Length < profilerStandalonePayloadBase {
		return fail(fmt.Errorf("standalone HIPERF segment has an invalid typed range: offset=%d length=%d", segment.Offset, segment.Length))
	}
	verified, ok := readStandaloneSegmentAt(inventory.input, segment.Offset, inventory.inputSize)
	if !ok || verified != segment {
		return fail(errors.New("standalone HIPERF segment no longer matches its authority header"))
	}
	payloadBase := segment.Offset + profilerStandalonePayloadBase
	payloadSize := segment.Length - profilerStandalonePayloadBase
	if payloadBase < segment.Offset || payloadBase > inventory.inputSize || payloadSize > inventory.inputSize-payloadBase {
		return fail(errors.New("standalone HIPERF payload range overflowed its parent authority"))
	}
	return &standaloneHiperfPayloadView{
		parent: inventory.input, parentSize: inventory.inputSize, segment: segment,
		payloadBase: payloadBase, payloadSize: payloadSize, displayPath: path,
	}, nil
}

func (view *standaloneHiperfPayloadView) ReadAt(buffer []byte, offset int64) (int, error) {
	if view == nil || view.parent == nil {
		return 0, conversionInputFailure(ConversionInputCodeClosed, conversionInputStageStandaloneExtract, "", nil)
	}
	if offset < 0 {
		return 0, conversionInputFailure(ConversionInputCodeInvalidRange, conversionInputStageStandaloneExtract, view.displayPath, nil)
	}
	if len(buffer) == 0 {
		return 0, nil
	}
	if offset >= view.payloadSize {
		return 0, io.EOF
	}
	remaining := view.payloadSize - offset
	limited := buffer
	truncated := int64(len(buffer)) > remaining
	if truncated {
		limited = buffer[:int(remaining)]
	}
	n, err := view.parent.ReadAt(limited, view.payloadBase+offset)
	if err == nil && truncated {
		err = io.EOF
	}
	return n, err
}

func (view *standaloneHiperfPayloadView) Size() int64 {
	if view == nil {
		return 0
	}
	return view.payloadSize
}

func (view *standaloneHiperfPayloadView) DisplayPath() string {
	if view == nil {
		return ""
	}
	return view.displayPath
}

func (view *standaloneHiperfPayloadView) Validate(stage conversionInputStage) error {
	if view == nil || view.parent == nil || !stage.valid() || view.parentSize < 0 ||
		view.parent.Size() != view.parentSize || strings.TrimSpace(view.displayPath) == "" ||
		view.segment.DataType != profilerDataTypeHiperf ||
		view.payloadBase < 0 || view.payloadSize < 0 || view.payloadBase > view.parentSize ||
		view.payloadSize > view.parentSize-view.payloadBase {
		return conversionInputFailure(
			ConversionInputCodeInternalContract,
			stage,
			view.DisplayPath(),
			errors.New("standalone HIPERF payload view is invalid"),
		)
	}
	if err := view.parent.Validate(stage); err != nil {
		return err
	}
	verified, ok := readStandaloneSegmentAt(view.parent, view.segment.Offset, view.parentSize)
	if !ok || verified != view.segment {
		return conversionInputFailure(
			ConversionInputCodeGenerationChanged,
			stage,
			view.DisplayPath(),
			errors.New("standalone HIPERF segment header changed"),
		)
	}
	return nil
}

// withOpenFile is intentionally only a snapshot-backing capability. It lends
// the parent handle so Windows can prove the source file-system generation;
// the view does not implement externalToolWholeFileSource, so Linux can never
// pass the whole profiler container to a child as this payload's inherited FD.
func (view *standaloneHiperfPayloadView) withOpenFile(callback func(*os.File) error) error {
	if view == nil || view.parent == nil || callback == nil {
		return conversionInputFailure(
			ConversionInputCodeInternalContract,
			conversionInputStageExternalTool,
			view.DisplayPath(),
			errors.New("standalone HIPERF snapshot backing authority is incomplete"),
		)
	}
	backing, ok := view.parent.(externalToolInputFileSource)
	if !ok {
		return conversionInputFailure(
			ConversionInputCodeInternalContract,
			conversionInputStageExternalTool,
			view.DisplayPath(),
			errors.New("standalone HIPERF parent has no held snapshot backing file"),
		)
	}
	return backing.withOpenFile(callback)
}

var _ conversionInputView = (*standaloneHiperfPayloadView)(nil)
var _ externalToolInputFileSource = (*standaloneHiperfPayloadView)(nil)
