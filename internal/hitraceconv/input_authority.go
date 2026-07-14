package hitraceconv

import (
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"sync"

	"github.com/hanchaoqun/codrax/internal/filegeneration"
)

type ConversionInputErrorCode string

const (
	ConversionInputCodeInvalidPath               ConversionInputErrorCode = "invalid_input_path"
	ConversionInputCodeOpenFailed                ConversionInputErrorCode = "input_open_failed"
	ConversionInputCodeNotRegular                ConversionInputErrorCode = "input_not_regular"
	ConversionInputCodeStrongIdentityUnavailable ConversionInputErrorCode = "strong_identity_unavailable"
	ConversionInputCodePathBindingFailed         ConversionInputErrorCode = "input_path_binding_failed"
	ConversionInputCodeGenerationChanged         ConversionInputErrorCode = "source_generation_changed"
	ConversionInputCodeInvalidRange              ConversionInputErrorCode = "input_range_invalid"
	ConversionInputCodeClosed                    ConversionInputErrorCode = "input_authority_closed"
	ConversionInputCodeInternalContract          ConversionInputErrorCode = "input_contract_invalid"
)

func (code ConversionInputErrorCode) valid() bool {
	switch code {
	case ConversionInputCodeInvalidPath,
		ConversionInputCodeOpenFailed,
		ConversionInputCodeNotRegular,
		ConversionInputCodeStrongIdentityUnavailable,
		ConversionInputCodePathBindingFailed,
		ConversionInputCodeGenerationChanged,
		ConversionInputCodeInvalidRange,
		ConversionInputCodeClosed,
		ConversionInputCodeInternalContract:
		return true
	default:
		return false
	}
}

const conversionInputProbeSize = 64

type conversionInputStage uint8

const (
	conversionInputStageOpen conversionInputStage = iota + 1
	conversionInputStageProbe
	conversionInputStageRoute
	conversionInputStageStandaloneScan
	conversionInputStageStandaloneExtract
	conversionInputStageProfilerHeader
	conversionInputStageProfilerBody
	conversionInputStageBuiltinMetadata
	conversionInputStageBuiltinRender
	conversionInputStageDirectPerfRead
	conversionInputStageExternalTool
	conversionInputStagePreCommit
)

func (stage conversionInputStage) valid() bool {
	return stage >= conversionInputStageOpen && stage <= conversionInputStagePreCommit
}

func (stage conversionInputStage) String() string {
	switch stage {
	case conversionInputStageOpen:
		return "open"
	case conversionInputStageProbe:
		return "probe"
	case conversionInputStageRoute:
		return "route"
	case conversionInputStageStandaloneScan:
		return "standalone_scan"
	case conversionInputStageStandaloneExtract:
		return "standalone_extract"
	case conversionInputStageProfilerHeader:
		return "profiler_header"
	case conversionInputStageProfilerBody:
		return "profiler_body"
	case conversionInputStageBuiltinMetadata:
		return "builtin_metadata"
	case conversionInputStageBuiltinRender:
		return "builtin_render"
	case conversionInputStageDirectPerfRead:
		return "direct_perf_read"
	case conversionInputStageExternalTool:
		return "external_tool"
	case conversionInputStagePreCommit:
		return "pre_commit"
	default:
		return "invalid"
	}
}

// ConversionInputError is the stable typed failure surface for the immutable
// input transaction. Code and Stage are closed values; Cause retains the
// underlying filesystem error for errors.Is/errors.As callers.
type ConversionInputError struct {
	Code  ConversionInputErrorCode
	Stage string
	Path  string
	Cause error
}

func (err *ConversionInputError) Error() string {
	if err == nil {
		return "trace conversion input rejected"
	}
	message := "trace conversion input rejected: code=" + firstNonEmpty(strings.TrimSpace(string(err.Code)), "unknown")
	if stage := strings.TrimSpace(err.Stage); stage != "" {
		message += " stage=" + stage
	}
	if path := strings.TrimSpace(err.Path); path != "" {
		message += " path=" + strconv.Quote(path)
	}
	if err.Cause != nil {
		message += ": " + err.Cause.Error()
	}
	return message
}

func (err *ConversionInputError) Unwrap() error {
	if err == nil {
		return nil
	}
	return err.Cause
}

// conversionInputAuthority is the sole physical-file content authority for a
// conversion. Every parser receives bounded ReaderAt views backed by this one
// read-only handle. Path opens in Validate are metadata-only generation checks
// and are never exposed to a parser.
type conversionInputAuthority struct {
	mu            sync.RWMutex
	file          *os.File
	displayPath   string
	requestedPath string
	canonicalPath string
	size          int64
	fileInfo      os.FileInfo
	identity      filegeneration.Identity
	closed        bool
}

func openConversionInputAuthority(path string) (*conversionInputAuthority, error) {
	requested := strings.TrimSpace(path)
	if requested == "" {
		return nil, conversionInputFailure(ConversionInputCodeInvalidPath, conversionInputStageOpen, requested, nil)
	}
	abs, err := filepath.Abs(filepath.Clean(requested))
	if err != nil {
		return nil, conversionInputFailure(ConversionInputCodeInvalidPath, conversionInputStageOpen, requested, err)
	}
	abs = filepath.Clean(abs)
	file, err := openConversionInputFile(abs)
	if err != nil {
		return nil, conversionInputFailure(ConversionInputCodeOpenFailed, conversionInputStageOpen, abs, err)
	}
	closeOnError := true
	defer func() {
		if closeOnError {
			_ = file.Close()
		}
	}()
	info, err := file.Stat()
	if err != nil {
		return nil, conversionInputFailure(ConversionInputCodeOpenFailed, conversionInputStageOpen, abs, err)
	}
	if !info.Mode().IsRegular() {
		return nil, conversionInputFailure(ConversionInputCodeNotRegular, conversionInputStageOpen, abs, nil)
	}
	identity, err := filegeneration.FromFile(file)
	if err != nil {
		return nil, conversionInputFailure(ConversionInputCodeOpenFailed, conversionInputStageOpen, abs, err)
	}
	if !identity.Strong() {
		return nil, conversionInputFailure(ConversionInputCodeStrongIdentityUnavailable, conversionInputStageOpen, abs, nil)
	}
	canonical, err := filepath.EvalSymlinks(abs)
	if err != nil {
		return nil, conversionInputFailure(ConversionInputCodePathBindingFailed, conversionInputStageOpen, abs, err)
	}
	authority := &conversionInputAuthority{
		file: file, displayPath: requested, requestedPath: abs, canonicalPath: filepath.Clean(canonical),
		size: identity.Size(), fileInfo: info, identity: identity,
	}
	if err := authority.validateLocked(conversionInputStageOpen, true); err != nil {
		return nil, err
	}
	closeOnError = false
	return authority, nil
}

func conversionInputFailure(code ConversionInputErrorCode, stage conversionInputStage, path string, cause error) error {
	if !code.valid() {
		cause = errors.Join(cause, fmt.Errorf("invalid conversion input error code %q", code))
		code = ConversionInputCodeInternalContract
	}
	stageName := stage.String()
	if !stage.valid() {
		cause = errors.Join(cause, fmt.Errorf("invalid conversion input stage %d", stage))
		code = ConversionInputCodeInternalContract
		stageName = "invalid"
	}
	return &ConversionInputError{Code: code, Stage: stageName, Path: path, Cause: cause}
}

func (authority *conversionInputAuthority) Size() int64 {
	if authority == nil {
		return 0
	}
	return authority.size
}

func (authority *conversionInputAuthority) DisplayPath() string {
	if authority == nil {
		return ""
	}
	return authority.displayPath
}

func (authority *conversionInputAuthority) CanonicalPath() string {
	if authority == nil {
		return ""
	}
	return authority.canonicalPath
}

// canonicalIdentity returns the immutable input identity in the same closed
// shape used by output-collision and file-ledger checks. This is controller
// authority only: parser views intentionally do not expose canonical paths.
func (authority *conversionInputAuthority) canonicalIdentity() (traceCanonicalPath, error) {
	if authority == nil {
		return traceCanonicalPath{}, conversionInputFailure(ConversionInputCodeClosed, conversionInputStageRoute, "", nil)
	}
	authority.mu.RLock()
	defer authority.mu.RUnlock()
	if authority.closed || authority.file == nil || authority.fileInfo == nil {
		return traceCanonicalPath{}, conversionInputFailure(ConversionInputCodeClosed, conversionInputStageRoute, authority.requestedPath, nil)
	}
	return traceCanonicalPath{path: authority.canonicalPath, info: authority.fileInfo}, nil
}

func (authority *conversionInputAuthority) ReadAt(buffer []byte, offset int64) (int, error) {
	if authority == nil {
		return 0, conversionInputFailure(ConversionInputCodeClosed, conversionInputStageOpen, "", nil)
	}
	authority.mu.RLock()
	defer authority.mu.RUnlock()
	if authority.closed || authority.file == nil {
		return 0, conversionInputFailure(ConversionInputCodeClosed, conversionInputStageOpen, authority.requestedPath, nil)
	}
	if offset < 0 {
		return 0, conversionInputFailure(ConversionInputCodeInvalidRange, conversionInputStageOpen, authority.requestedPath, nil)
	}
	if len(buffer) == 0 {
		return 0, nil
	}
	if offset >= authority.size {
		return 0, io.EOF
	}
	remaining := authority.size - offset
	limited := buffer
	truncated := int64(len(buffer)) > remaining
	if truncated {
		limited = buffer[:int(remaining)]
	}
	n, err := authority.file.ReadAt(limited, offset)
	if err == nil && truncated {
		err = io.EOF
	}
	return n, err
}

func (authority *conversionInputAuthority) Section(offset, length int64) (*io.SectionReader, error) {
	if authority == nil {
		return nil, conversionInputFailure(ConversionInputCodeClosed, conversionInputStageOpen, "", nil)
	}
	authority.mu.RLock()
	defer authority.mu.RUnlock()
	if authority.closed || authority.file == nil {
		return nil, conversionInputFailure(ConversionInputCodeClosed, conversionInputStageOpen, authority.requestedPath, nil)
	}
	if offset < 0 || length < 0 || offset > authority.size || length > authority.size-offset {
		return nil, conversionInputFailure(ConversionInputCodeInvalidRange, conversionInputStageOpen, authority.requestedPath, nil)
	}
	return io.NewSectionReader(authority, offset, length), nil
}

func (authority *conversionInputAuthority) WholeSection() (*io.SectionReader, error) {
	if authority == nil {
		return nil, conversionInputFailure(ConversionInputCodeClosed, conversionInputStageOpen, "", nil)
	}
	return authority.Section(0, authority.size)
}

func (authority *conversionInputAuthority) Probe() ([]byte, error) {
	if authority == nil {
		return nil, conversionInputFailure(ConversionInputCodeClosed, conversionInputStageProbe, "", nil)
	}
	if authority.size == 0 {
		return nil, authority.Validate(conversionInputStageProbe)
	}
	length := int64(conversionInputProbeSize)
	if length > authority.size {
		length = authority.size
	}
	buffer := make([]byte, int(length))
	_, readErr := io.ReadFull(io.NewSectionReader(authority, 0, length), buffer)
	if validateErr := authority.Validate(conversionInputStageProbe); validateErr != nil {
		return nil, validateErr
	}
	if readErr != nil {
		return nil, readErr
	}
	return buffer, nil
}

func (authority *conversionInputAuthority) Validate(stage conversionInputStage) error {
	if authority == nil {
		return conversionInputFailure(ConversionInputCodeClosed, stage, "", nil)
	}
	authority.mu.RLock()
	defer authority.mu.RUnlock()
	return authority.validateLocked(stage, false)
}

func (authority *conversionInputAuthority) validateLocked(stage conversionInputStage, opening bool) error {
	if !stage.valid() {
		return conversionInputFailure(ConversionInputCodePathBindingFailed, stage, authority.requestedPath, errors.New("invalid conversion input stage"))
	}
	if authority.closed || authority.file == nil || !authority.identity.Initialized() {
		return conversionInputFailure(ConversionInputCodeClosed, stage, authority.requestedPath, nil)
	}
	current, err := filegeneration.FromFile(authority.file)
	if err != nil {
		return conversionInputFailure(ConversionInputCodeGenerationChanged, stage, authority.requestedPath, err)
	}
	if !current.Strong() {
		return conversionInputFailure(ConversionInputCodeStrongIdentityUnavailable, stage, authority.requestedPath, nil)
	}
	if !authority.identity.SameVersion(current) {
		return conversionInputFailure(ConversionInputCodeGenerationChanged, stage, authority.requestedPath, nil)
	}
	requestedIdentity, err := filegeneration.FromPath(authority.requestedPath)
	if err != nil {
		code := ConversionInputCodeGenerationChanged
		if opening {
			code = ConversionInputCodePathBindingFailed
		}
		return conversionInputFailure(code, stage, authority.requestedPath, err)
	}
	if !requestedIdentity.Strong() {
		return conversionInputFailure(ConversionInputCodeStrongIdentityUnavailable, stage, authority.requestedPath, nil)
	}
	if !authority.identity.SameVersion(requestedIdentity) {
		code := ConversionInputCodeGenerationChanged
		if opening {
			code = ConversionInputCodePathBindingFailed
		}
		return conversionInputFailure(code, stage, authority.requestedPath, nil)
	}
	canonical, err := filepath.EvalSymlinks(authority.requestedPath)
	if err != nil || !sameConversionCanonicalPath(authority.canonicalPath, filepath.Clean(canonical)) {
		code := ConversionInputCodeGenerationChanged
		if opening {
			code = ConversionInputCodePathBindingFailed
		}
		return conversionInputFailure(code, stage, authority.requestedPath, err)
	}
	canonicalIdentity, err := filegeneration.FromPath(authority.canonicalPath)
	if err != nil {
		code := ConversionInputCodeGenerationChanged
		if opening {
			code = ConversionInputCodePathBindingFailed
		}
		return conversionInputFailure(code, stage, authority.canonicalPath, err)
	}
	if !canonicalIdentity.Strong() {
		return conversionInputFailure(ConversionInputCodeStrongIdentityUnavailable, stage, authority.canonicalPath, nil)
	}
	if !authority.identity.SameVersion(canonicalIdentity) {
		code := ConversionInputCodeGenerationChanged
		if opening {
			code = ConversionInputCodePathBindingFailed
		}
		return conversionInputFailure(code, stage, authority.canonicalPath, nil)
	}
	return nil
}

func (authority *conversionInputAuthority) Close() error {
	if authority == nil {
		return nil
	}
	authority.mu.Lock()
	defer authority.mu.Unlock()
	if authority.closed {
		return nil
	}
	authority.closed = true
	if authority.file == nil {
		return nil
	}
	err := authority.file.Close()
	authority.file = nil
	return err
}

func sameConversionCanonicalPath(left, right string) bool {
	if runtime.GOOS == "windows" {
		return strings.EqualFold(filepath.Clean(left), filepath.Clean(right))
	}
	return filepath.Clean(left) == filepath.Clean(right)
}

type conversionInputView interface {
	io.ReaderAt
	Size() int64
	DisplayPath() string
	Validate(conversionInputStage) error
}

var _ conversionInputView = (*conversionInputAuthority)(nil)

func (authority *conversionInputAuthority) String() string {
	if authority == nil {
		return "conversionInputAuthority<nil>"
	}
	return fmt.Sprintf("conversionInputAuthority{path:%q,size:%d}", authority.requestedPath, authority.size)
}
