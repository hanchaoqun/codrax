package hitraceconv

import (
	"archive/zip"
	"context"
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"os"
	"path"
	"path/filepath"
	"strconv"
	"strings"
	"unicode/utf8"

	"github.com/hanchaoqun/codrax/internal/filegeneration"
	"github.com/hanchaoqun/codrax/internal/tracebundle"
)

const (
	traceArchiveZIPMaxEntries              = 1024
	traceArchiveZIPMaxCentralDirectory     = 64 << 20
	traceArchiveZIPMaxArchiveBytes         = int64(65 << 30)
	traceArchiveZIPMaxMemberCompressed     = uint64(64 << 30)
	traceArchiveZIPMaxMemberUncompressed   = uint64(64 << 30)
	traceArchiveZIPMaxCompressionRatio     = uint64(1000)
	traceArchiveZIPEndRecordFixedBytes     = 22
	traceArchiveZIPMaxCommentBytes         = 1<<16 - 1
	traceArchiveZIP64LocatorBytes          = 20
	traceArchiveZIP64EndRecordMinimumBytes = 56
	traceArchiveZIP64EndRecordMaxBytes     = 1 << 20
	traceArchiveMemberSnapshotLeaf         = "archive_member_input.bin"
)

const (
	traceArchiveCodeInvalidZIP         = "invalid_zip"
	traceArchiveCodeMultiDisk          = "multi_disk_unsupported"
	traceArchiveCodeResourceLimit      = "archive_resource_limit"
	traceArchiveCodeInvalidMember      = "invalid_archive_member"
	traceArchiveCodeEncryptedMember    = "encrypted_member_unsupported"
	traceArchiveCodeSpecialMember      = "special_member_unsupported"
	traceArchiveCodeDuplicateMember    = "duplicate_member"
	traceArchiveCodeNoCandidate        = "trace_member_not_found"
	traceArchiveCodeMultipleCandidates = "multiple_trace_members"
	traceArchiveCodeExplicitMember     = "explicit_trace_member_invalid"
	traceArchiveCodeMemberIntegrity    = "trace_member_integrity_failed"
	traceArchiveCodeNestedArchive      = "nested_archive_unsupported"
)

// TraceArchiveError is the stable fail-closed surface for archive intake.
// Code identifies the rejected contract dimension; Member is populated only
// after one canonical central-directory member has been identified.
type TraceArchiveError struct {
	Code   string
	Member string
	Cause  error
}

func (err *TraceArchiveError) Error() string {
	if err == nil {
		return "trace archive rejected"
	}
	message := "trace archive rejected: code=" + firstNonEmpty(err.Code, traceArchiveCodeInvalidZIP)
	if err.Member != "" {
		message += " member=" + strconv.Quote(boundedTraceProviderErrorText(err.Member, 512))
	}
	if err.Cause != nil {
		message += ": " + err.Cause.Error()
	}
	return message
}

func (err *TraceArchiveError) Unwrap() error {
	if err == nil {
		return nil
	}
	return err.Cause
}

func traceArchiveFailure(code, member string, cause error) error {
	return &TraceArchiveError{Code: code, Member: member, Cause: cause}
}

type traceConversionInput struct {
	archive    *conversionInputAuthority
	input      conversionInputView
	member     *traceArchiveMemberInput
	staging    *privateConversionDir
	provenance *TraceArchiveProvenance
	namespace  string
}

func newTraceConversionInput(authority *conversionInputAuthority) *traceConversionInput {
	namespace := ""
	if authority != nil {
		namespace = authority.CanonicalPath()
	}
	return &traceConversionInput{archive: authority, input: authority, namespace: namespace}
}

func (route *traceConversionInput) Close() error {
	if route == nil {
		return nil
	}
	var err error
	if route.member != nil {
		err = traceDBJoinPreservingSingle(err, route.member.Close())
		route.member = nil
	}
	if route.staging != nil {
		err = traceDBJoinPreservingSingle(err, route.staging.FinalizeCleanup())
		route.staging = nil
	}
	if route.archive != nil {
		err = traceDBJoinPreservingSingle(err, route.archive.Close())
	}
	return err
}

func (route *traceConversionInput) finalize(ctx context.Context) error {
	if route == nil || route.archive == nil || route.input == nil {
		return conversionInputFailure(ConversionInputCodeInternalContract, conversionInputStagePreCommit, "", errors.New("trace conversion route input is incomplete"))
	}
	if err := completeConversionInputStage(ctx, route.input, conversionInputStagePreCommit, nil); err != nil {
		return err
	}
	if route.member != nil {
		if err := route.member.Close(); err != nil {
			return err
		}
		route.member = nil
	}
	if route.staging != nil {
		if err := route.staging.FinalizeCleanup(); err != nil {
			return err
		}
		route.staging = nil
	}
	if err := route.archive.Validate(conversionInputStagePreCommit); err != nil {
		return err
	}
	return route.archive.Close()
}

func (route *traceConversionInput) decorate(result *Result) {
	if route == nil || route.archive == nil || result == nil {
		return
	}
	result.InputPath = route.archive.DisplayPath()
	result.InputBytes = route.archive.Size()
	result.ArchiveProvenance = cloneTraceArchiveProvenance(route.provenance)
}

func (route *traceConversionInput) bindLedger(ledger *conversionFileLedger) error {
	if route == nil || ledger == nil {
		return conversionInputFailure(ConversionInputCodeInternalContract, conversionInputStageRoute, "", errors.New("trace archive ledger binding is incomplete"))
	}
	if route.provenance == nil {
		return nil
	}
	if err := validateTraceArchiveProvenance(route.provenance); err != nil {
		return err
	}
	ledger.archive = cloneTraceArchiveProvenance(route.provenance)
	return nil
}

func prepareTraceConversionInput(ctx context.Context, opts Options, authority *conversionInputAuthority, outerProbe []byte) (*traceConversionInput, error) {
	route := newTraceConversionInput(authority)
	if !traceArchiveZIPMagic(outerProbe) {
		if opts.ArchiveMember != "" {
			return route, traceArchiveFailure(traceArchiveCodeExplicitMember, "", errors.New("--archive-member is valid only for a ZIP input selected by content magic"))
		}
		return route, nil
	}
	if len(outerProbe) >= 4 && string(outerProbe[:4]) == "PK\x07\x08" {
		return route, traceArchiveFailure(traceArchiveCodeMultiDisk, "", errors.New("spanned ZIP marker is unsupported"))
	}
	if authority.Size() > traceArchiveZIPMaxArchiveBytes {
		return route, traceArchiveFailure(traceArchiveCodeResourceLimit, "", fmt.Errorf(
			"ZIP archive exceeds physical size limit: bytes=%d/%d", authority.Size(), traceArchiveZIPMaxArchiveBytes,
		))
	}
	output := strings.TrimSpace(opts.OutputPath)
	if output == "" {
		output = DefaultOutputPath(authority.DisplayPath())
	}
	stagingRoot, err := resolveConversionRuntimeAnchor(opts.RuntimeAnchor, output)
	if err != nil {
		return route, err
	}
	staging, err := newRuntimePrivateConversionDir(stagingRoot, "."+filepath.Base(output)+".*.archive")
	if err != nil {
		return route, err
	}
	member, provenance, namespace, err := extractTraceArchiveZIPMember(ctx, opts, authority, staging)
	if err != nil {
		return route, traceDBJoinPreservingSingle(err, staging.FinalizeCleanup())
	}
	route.input = member
	route.member = member
	route.staging = staging
	route.provenance = provenance
	route.namespace = namespace
	return route, nil
}

func traceArchiveZIPMagic(probe []byte) bool {
	if len(probe) < 4 {
		return false
	}
	switch string(probe[:4]) {
	case "PK\x03\x04", "PK\x05\x06", "PK\x07\x08":
		return true
	default:
		return false
	}
}

type traceArchiveZIPDirectory struct {
	entries uint64
	size    uint64
	offset  uint64
}

func preflightTraceArchiveZIP(ctx context.Context, input conversionInputView) (traceArchiveZIPDirectory, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	if input == nil || input.Size() < traceArchiveZIPEndRecordFixedBytes {
		return traceArchiveZIPDirectory{}, traceArchiveFailure(traceArchiveCodeInvalidZIP, "", errors.New("ZIP is too short for an end-of-central-directory record"))
	}
	if err := completeConversionInputStage(ctx, input, conversionInputStageArchiveIntake, nil); err != nil {
		return traceArchiveZIPDirectory{}, err
	}
	size := input.Size()
	tailSize := int64(traceArchiveZIPEndRecordFixedBytes + traceArchiveZIPMaxCommentBytes)
	if tailSize > size {
		tailSize = size
	}
	tail := make([]byte, int(tailSize))
	if _, err := input.ReadAt(tail, size-tailSize); err != nil {
		return traceArchiveZIPDirectory{}, traceArchiveFailure(traceArchiveCodeInvalidZIP, "", fmt.Errorf("read ZIP end record: %w", err))
	}
	if err := ctx.Err(); err != nil {
		return traceArchiveZIPDirectory{}, err
	}
	eocdIndex := -1
	for index := len(tail) - traceArchiveZIPEndRecordFixedBytes; index >= 0; index-- {
		if binary.LittleEndian.Uint32(tail[index:index+4]) != 0x06054b50 {
			continue
		}
		comment := int(binary.LittleEndian.Uint16(tail[index+20 : index+22]))
		if index+traceArchiveZIPEndRecordFixedBytes+comment == len(tail) {
			eocdIndex = index
			break
		}
	}
	if eocdIndex < 0 {
		return traceArchiveZIPDirectory{}, traceArchiveFailure(traceArchiveCodeInvalidZIP, "", errors.New("canonical ZIP end record was not found"))
	}
	eocd := tail[eocdIndex : eocdIndex+traceArchiveZIPEndRecordFixedBytes]
	eocdOffset := uint64(size-tailSize) + uint64(eocdIndex)
	disk := binary.LittleEndian.Uint16(eocd[4:6])
	centralDisk := binary.LittleEndian.Uint16(eocd[6:8])
	recordsDisk := binary.LittleEndian.Uint16(eocd[8:10])
	recordsTotal := binary.LittleEndian.Uint16(eocd[10:12])
	centralSize := binary.LittleEndian.Uint32(eocd[12:16])
	centralOffset := binary.LittleEndian.Uint32(eocd[16:20])
	zip64 := disk == 0xffff || centralDisk == 0xffff || recordsDisk == 0xffff || recordsTotal == 0xffff || centralSize == 0xffffffff || centralOffset == 0xffffffff
	var directory traceArchiveZIPDirectory
	if !zip64 {
		if disk != 0 || centralDisk != 0 || recordsDisk != recordsTotal {
			return traceArchiveZIPDirectory{}, traceArchiveFailure(traceArchiveCodeMultiDisk, "", errors.New("ZIP spans multiple disks or has inconsistent per-disk counts"))
		}
		directory = traceArchiveZIPDirectory{entries: uint64(recordsTotal), size: uint64(centralSize), offset: uint64(centralOffset)}
	} else {
		if eocdOffset < traceArchiveZIP64LocatorBytes {
			return traceArchiveZIPDirectory{}, traceArchiveFailure(traceArchiveCodeInvalidZIP, "", errors.New("ZIP64 locator is missing"))
		}
		if disk != 0 || centralDisk != 0 {
			return traceArchiveZIPDirectory{}, traceArchiveFailure(traceArchiveCodeMultiDisk, "", errors.New("ZIP64 end record declares a nonzero disk"))
		}
		var locator [traceArchiveZIP64LocatorBytes]byte
		if _, err := input.ReadAt(locator[:], int64(eocdOffset-traceArchiveZIP64LocatorBytes)); err != nil || binary.LittleEndian.Uint32(locator[0:4]) != 0x07064b50 {
			return traceArchiveZIPDirectory{}, traceArchiveFailure(traceArchiveCodeInvalidZIP, "", fmt.Errorf("read ZIP64 locator: %w", firstNonNilError(err, errors.New("invalid ZIP64 locator signature"))))
		}
		if binary.LittleEndian.Uint32(locator[4:8]) != 0 || binary.LittleEndian.Uint32(locator[16:20]) != 1 {
			return traceArchiveZIPDirectory{}, traceArchiveFailure(traceArchiveCodeMultiDisk, "", errors.New("ZIP64 spans multiple disks"))
		}
		zip64Offset := binary.LittleEndian.Uint64(locator[8:16])
		if zip64Offset > uint64(size) || uint64(size)-zip64Offset < traceArchiveZIP64EndRecordMinimumBytes {
			return traceArchiveZIPDirectory{}, traceArchiveFailure(traceArchiveCodeInvalidZIP, "", errors.New("ZIP64 end record offset exceeds fixed input"))
		}
		var record [traceArchiveZIP64EndRecordMinimumBytes]byte
		if _, err := input.ReadAt(record[:], int64(zip64Offset)); err != nil || binary.LittleEndian.Uint32(record[0:4]) != 0x06064b50 {
			return traceArchiveZIPDirectory{}, traceArchiveFailure(traceArchiveCodeInvalidZIP, "", fmt.Errorf("read ZIP64 end record: %w", firstNonNilError(err, errors.New("invalid ZIP64 end record signature"))))
		}
		recordSize := binary.LittleEndian.Uint64(record[4:12])
		if recordSize < 44 || recordSize > traceArchiveZIP64EndRecordMaxBytes || recordSize > uint64(size)-zip64Offset-12 || zip64Offset+recordSize+12 > eocdOffset-traceArchiveZIP64LocatorBytes {
			return traceArchiveZIPDirectory{}, traceArchiveFailure(traceArchiveCodeInvalidZIP, "", errors.New("ZIP64 end record range is invalid"))
		}
		recordsOnDisk64 := binary.LittleEndian.Uint64(record[24:32])
		recordsTotal64 := binary.LittleEndian.Uint64(record[32:40])
		centralSize64 := binary.LittleEndian.Uint64(record[40:48])
		centralOffset64 := binary.LittleEndian.Uint64(record[48:56])
		if binary.LittleEndian.Uint32(record[16:20]) != 0 || binary.LittleEndian.Uint32(record[20:24]) != 0 ||
			recordsOnDisk64 != recordsTotal64 {
			return traceArchiveZIPDirectory{}, traceArchiveFailure(traceArchiveCodeMultiDisk, "", errors.New("ZIP64 spans multiple disks or has inconsistent counts"))
		}
		if (recordsDisk != 0xffff && uint64(recordsDisk) != recordsOnDisk64) ||
			(recordsTotal != 0xffff && uint64(recordsTotal) != recordsTotal64) ||
			(centralSize != 0xffffffff && uint64(centralSize) != centralSize64) ||
			(centralOffset != 0xffffffff && uint64(centralOffset) != centralOffset64) {
			return traceArchiveZIPDirectory{}, traceArchiveFailure(traceArchiveCodeInvalidZIP, "", errors.New("ZIP64 and legacy end-record values disagree"))
		}
		directory = traceArchiveZIPDirectory{
			entries: recordsTotal64,
			size:    centralSize64,
			offset:  centralOffset64,
		}
	}
	if directory.entries > traceArchiveZIPMaxEntries || directory.size > traceArchiveZIPMaxCentralDirectory {
		return traceArchiveZIPDirectory{}, traceArchiveFailure(traceArchiveCodeResourceLimit, "", fmt.Errorf("ZIP central directory exceeds limits: entries=%d/%d bytes=%d/%d", directory.entries, traceArchiveZIPMaxEntries, directory.size, traceArchiveZIPMaxCentralDirectory))
	}
	if directory.offset > uint64(size) || directory.size > uint64(size)-directory.offset || directory.offset+directory.size > eocdOffset {
		return traceArchiveZIPDirectory{}, traceArchiveFailure(traceArchiveCodeInvalidZIP, "", errors.New("ZIP central directory range exceeds fixed input"))
	}
	if err := completeConversionInputStage(ctx, input, conversionInputStageArchiveIntake, nil); err != nil {
		return traceArchiveZIPDirectory{}, err
	}
	return directory, nil
}

func firstNonNilError(primary, fallback error) error {
	if primary != nil {
		return primary
	}
	return fallback
}

type traceArchiveZIPCandidate struct {
	file *zip.File
	name string
}

func selectTraceArchiveZIPMember(reader *zip.Reader, directory traceArchiveZIPDirectory, explicit string) (traceArchiveZIPCandidate, string, error) {
	if reader == nil || uint64(len(reader.File)) != directory.entries {
		return traceArchiveZIPCandidate{}, "", traceArchiveFailure(traceArchiveCodeInvalidZIP, "", fmt.Errorf("ZIP central-directory count mismatch: parsed=%d declared=%d", len(reader.File), directory.entries))
	}
	names := make(map[string]struct{}, len(reader.File))
	var candidates []traceArchiveZIPCandidate
	for _, file := range reader.File {
		name, isDir, err := validateTraceArchiveZIPName(file)
		if err != nil {
			return traceArchiveZIPCandidate{}, "", err
		}
		if _, duplicate := names[name]; duplicate {
			return traceArchiveZIPCandidate{}, "", traceArchiveFailure(traceArchiveCodeDuplicateMember, name, errors.New("canonical member name appears more than once"))
		}
		names[name] = struct{}{}
		if file.Flags&1 != 0 {
			return traceArchiveZIPCandidate{}, "", traceArchiveFailure(traceArchiveCodeEncryptedMember, name, errors.New("encrypted ZIP members are unsupported"))
		}
		mode := file.Mode()
		if isDir {
			if !mode.IsDir() && mode.Type() != 0 {
				return traceArchiveZIPCandidate{}, "", traceArchiveFailure(traceArchiveCodeSpecialMember, name, fmt.Errorf("directory member has special mode %s", mode))
			}
			continue
		}
		if mode.Type() != 0 {
			return traceArchiveZIPCandidate{}, "", traceArchiveFailure(traceArchiveCodeSpecialMember, name, fmt.Errorf("member mode %s is not regular", mode))
		}
		ext := strings.ToLower(path.Ext(name))
		if ext == ".sys" || ext == ".htrace" {
			candidates = append(candidates, traceArchiveZIPCandidate{file: file, name: name})
		}
	}
	if explicit != "" {
		canonical, err := validateExplicitTraceArchiveMember(explicit)
		if err != nil {
			return traceArchiveZIPCandidate{}, "", err
		}
		for _, candidate := range candidates {
			if candidate.name == canonical {
				return candidate, "explicit_member", nil
			}
		}
		return traceArchiveZIPCandidate{}, "", traceArchiveFailure(traceArchiveCodeExplicitMember, canonical, errors.New("explicit member is absent or is not a regular .sys/.htrace candidate"))
	}
	switch len(candidates) {
	case 0:
		return traceArchiveZIPCandidate{}, "", traceArchiveFailure(traceArchiveCodeNoCandidate, "", errors.New("ZIP contains no regular .sys/.htrace member"))
	case 1:
		return candidates[0], "unique_candidate", nil
	default:
		return traceArchiveZIPCandidate{}, "", traceArchiveFailure(traceArchiveCodeMultipleCandidates, "", fmt.Errorf(
			"ZIP contains %d trace candidates (%s); select one with --archive-member",
			len(candidates), traceArchiveCandidateSummary(candidates),
		))
	}
}

func traceArchiveCandidateSummary(candidates []traceArchiveZIPCandidate) string {
	const maxShown = 8
	shown := len(candidates)
	if shown > maxShown {
		shown = maxShown
	}
	parts := make([]string, 0, shown+1)
	for index := 0; index < shown; index++ {
		parts = append(parts, strconv.Quote(boundedTraceProviderErrorText(candidates[index].name, 160)))
	}
	if omitted := len(candidates) - shown; omitted > 0 {
		parts = append(parts, fmt.Sprintf("... %d omitted", omitted))
	}
	return strings.Join(parts, ", ")
}

func validateTraceArchiveZIPName(file *zip.File) (string, bool, error) {
	if file == nil {
		return "", false, traceArchiveFailure(traceArchiveCodeInvalidMember, "", errors.New("nil ZIP member"))
	}
	raw := file.Name
	if raw == "" || file.NonUTF8 || !utf8.ValidString(raw) || strings.ContainsRune(raw, 0) || strings.Contains(raw, "\\") {
		return "", false, traceArchiveFailure(traceArchiveCodeInvalidMember, raw, errors.New("member name must be non-empty canonical UTF-8 with slash separators"))
	}
	isDir := strings.HasSuffix(raw, "/")
	trimmed := strings.TrimSuffix(raw, "/")
	if trimmed == "" || strings.HasPrefix(trimmed, "/") || path.IsAbs(trimmed) || path.Clean(trimmed) != trimmed ||
		trimmed == "." || trimmed == ".." || strings.HasPrefix(trimmed, "../") || traceArchiveVolumePrefix(trimmed) {
		return "", false, traceArchiveFailure(traceArchiveCodeInvalidMember, raw, errors.New("member name is not canonical bundle-relative syntax"))
	}
	if file.Mode().IsDir() != isDir && file.Mode().Type() != 0 {
		return "", false, traceArchiveFailure(traceArchiveCodeSpecialMember, trimmed, errors.New("member directory marker and mode disagree"))
	}
	return trimmed, isDir, nil
}

func validateExplicitTraceArchiveMember(raw string) (string, error) {
	if raw == "" || strings.TrimSpace(raw) != raw || !utf8.ValidString(raw) || strings.ContainsRune(raw, 0) || strings.Contains(raw, "\\") ||
		strings.HasSuffix(raw, "/") || strings.HasPrefix(raw, "/") || path.IsAbs(raw) || path.Clean(raw) != raw ||
		raw == "." || raw == ".." || strings.HasPrefix(raw, "../") || traceArchiveVolumePrefix(raw) {
		return "", traceArchiveFailure(traceArchiveCodeExplicitMember, raw, errors.New("--archive-member must be one exact canonical slash-separated member name"))
	}
	return raw, nil
}

func traceArchiveVolumePrefix(value string) bool {
	return len(value) >= 2 && ((value[0] >= 'a' && value[0] <= 'z') || (value[0] >= 'A' && value[0] <= 'Z')) && value[1] == ':'
}

func extractTraceArchiveZIPMember(ctx context.Context, opts Options, authority *conversionInputAuthority, staging *privateConversionDir) (_ *traceArchiveMemberInput, _ *TraceArchiveProvenance, _ string, err error) {
	if ctx == nil {
		ctx = context.Background()
	}
	if authority == nil || staging == nil {
		return nil, nil, "", conversionInputFailure(ConversionInputCodeInternalContract, conversionInputStageArchiveIntake, "", errors.New("ZIP intake authority is incomplete"))
	}
	if err := completeConversionInputStage(ctx, authority, conversionInputStageArchiveIntake, nil); err != nil {
		return nil, nil, "", err
	}
	defer func() {
		err = completeConversionInputStage(ctx, authority, conversionInputStageArchiveIntake, err)
	}()
	directory, err := preflightTraceArchiveZIP(ctx, authority)
	if err != nil {
		return nil, nil, "", err
	}
	reader, err := zip.NewReader(authority, authority.Size())
	if err != nil {
		return nil, nil, "", traceArchiveFailure(traceArchiveCodeInvalidZIP, "", err)
	}
	candidate, selection, err := selectTraceArchiveZIPMember(reader, directory, opts.ArchiveMember)
	if err != nil {
		return nil, nil, "", err
	}
	memberSnapshot, memberSHA, err := extractSelectedTraceArchiveMember(ctx, authority.DisplayPath(), candidate, staging)
	if err != nil {
		return nil, nil, "", err
	}
	member := &traceArchiveMemberInput{
		archive: authority, snapshot: memberSnapshot, display: authority.DisplayPath(), leaf: path.Base(candidate.name), member: candidate.name,
	}
	closeMember := true
	defer func() {
		if closeMember {
			err = traceDBJoinPreservingSingle(err, member.Close())
		}
	}()
	archiveSHA, err := hashTraceArchiveInput(ctx, authority)
	if err != nil {
		return nil, nil, "", err
	}
	provenance := &TraceArchiveProvenance{
		Format: "zip", ArchiveBytes: authority.Size(), ArchiveSHA256: archiveSHA,
		Member: candidate.name, MemberBytes: member.Size(), MemberSHA256: memberSHA, Selection: selection,
	}
	if err := validateTraceArchiveProvenance(provenance); err != nil {
		return nil, nil, "", err
	}
	if err := member.Validate(conversionInputStageArchiveIntake); err != nil {
		return nil, nil, "", err
	}
	closeMember = false
	return member, provenance, authority.CanonicalPath() + "!/" + candidate.name, nil
}

func extractSelectedTraceArchiveMember(ctx context.Context, display string, candidate traceArchiveZIPCandidate, staging *privateConversionDir) (_ *externalToolInputSnapshot, memberSHA string, resultErr error) {
	if candidate.file == nil {
		return nil, "", traceArchiveFailure(traceArchiveCodeInvalidMember, candidate.name, errors.New("selected ZIP member is missing"))
	}
	compressed := candidate.file.CompressedSize64
	uncompressed := candidate.file.UncompressedSize64
	if uncompressed == 0 || compressed > traceArchiveZIPMaxMemberCompressed || uncompressed > traceArchiveZIPMaxMemberUncompressed ||
		(compressed == 0 && uncompressed != 0) {
		return nil, "", traceArchiveFailure(traceArchiveCodeResourceLimit, candidate.name, fmt.Errorf("ZIP member exceeds size/ratio limits: compressed=%d uncompressed=%d", compressed, uncompressed))
	}
	// Both declared sizes are already capped at 64 GiB, so the ratio product is
	// strictly inside uint64 and needs no architecture-dependent conversion.
	if compressed != 0 && uncompressed > compressed*traceArchiveZIPMaxCompressionRatio {
		return nil, "", traceArchiveFailure(traceArchiveCodeResourceLimit, candidate.name, fmt.Errorf("ZIP member compression ratio exceeds %d", traceArchiveZIPMaxCompressionRatio))
	}
	reader, err := candidate.file.Open()
	if err != nil {
		return nil, "", traceArchiveFailure(traceArchiveCodeMemberIntegrity, candidate.name, err)
	}
	readerClosed := false
	defer func() {
		if !readerClosed {
			resultErr = traceDBJoinPreservingSingle(resultErr, reader.Close())
		}
	}()
	writer, err := createExternalToolInputSnapshotFile(staging, traceArchiveMemberSnapshotLeaf)
	if err != nil {
		return nil, "", err
	}
	writerOwned := true
	defer func() {
		if writerOwned {
			resultErr = traceDBJoinPreservingSingle(resultErr, writer.Close())
		}
	}()
	hasher := sha256.New()
	destination := io.MultiWriter(writer, hasher)
	prefixSize := int64(4)
	if int64(uncompressed) < prefixSize {
		prefixSize = int64(uncompressed)
	}
	prefix := make([]byte, int(prefixSize))
	if _, err := io.ReadFull(reader, prefix); err != nil {
		return nil, "", traceArchiveFailure(traceArchiveCodeMemberIntegrity, candidate.name, err)
	}
	if traceArchiveZIPMagic(prefix) {
		return nil, "", traceArchiveFailure(traceArchiveCodeNestedArchive, candidate.name, errors.New("selected trace member is another ZIP archive"))
	}
	written, err := destination.Write(prefix)
	if err != nil || written != len(prefix) {
		if err == nil {
			err = io.ErrShortWrite
		}
		return nil, "", traceArchiveFailure(traceArchiveCodeMemberIntegrity, candidate.name, err)
	}
	remaining := int64(uncompressed) - prefixSize
	limited := &io.LimitedReader{R: reader, N: remaining + 1}
	copied, err := copyCancellableRange(ctx, destination, limited, nil)
	if err != nil {
		return nil, "", traceArchiveFailure(traceArchiveCodeMemberIntegrity, candidate.name, err)
	}
	actual := prefixSize + copied
	if actual != int64(uncompressed) {
		return nil, "", traceArchiveFailure(traceArchiveCodeMemberIntegrity, candidate.name, fmt.Errorf("decompressed size mismatch: got=%d want=%d", actual, uncompressed))
	}
	if err := reader.Close(); err != nil {
		readerClosed = true
		return nil, "", traceArchiveFailure(traceArchiveCodeMemberIntegrity, candidate.name, err)
	}
	readerClosed = true
	if err := writer.Sync(); err != nil {
		return nil, "", traceArchiveFailure(traceArchiveCodeMemberIntegrity, candidate.name, fmt.Errorf("sync member snapshot: %w", err))
	}
	created, err := writer.Stat()
	if err != nil || !created.Mode().IsRegular() || created.Size() != int64(uncompressed) {
		if err == nil {
			err = fmt.Errorf("member snapshot mode/size mismatch")
		}
		return nil, "", traceArchiveFailure(traceArchiveCodeMemberIntegrity, candidate.name, err)
	}
	createdIdentity, err := filegeneration.FromFile(writer)
	if err != nil || !createdIdentity.Strong() {
		return nil, "", traceArchiveFailure(traceArchiveCodeMemberIntegrity, candidate.name, firstNonNilError(err, errors.New("member snapshot has no strong identity")))
	}
	held, heldInfo, err := freezeExternalToolInputSnapshotFile(staging, traceArchiveMemberSnapshotLeaf, writer, created)
	writerOwned = false
	writer = nil
	if err != nil {
		return nil, "", err
	}
	closeHeld := true
	defer func() {
		if closeHeld {
			resultErr = traceDBJoinPreservingSingle(resultErr, held.Close())
		}
	}()
	heldIdentity, err := filegeneration.FromFile(held)
	if err != nil || !heldIdentity.Strong() || !createdIdentity.SameVersion(heldIdentity) || heldInfo == nil || heldInfo.Size() != int64(uncompressed) {
		return nil, "", traceArchiveFailure(traceArchiveCodeMemberIntegrity, candidate.name, firstNonNilError(err, errors.New("member snapshot generation changed while freezing")))
	}
	snapshotPath, err := staging.ChildPath(traceArchiveMemberSnapshotLeaf)
	if err != nil {
		return nil, "", err
	}
	snapshot := &externalToolInputSnapshot{
		dir: staging, name: traceArchiveMemberSnapshotLeaf, path: snapshotPath, display: display,
		file: held, identity: heldIdentity, size: heldIdentity.Size(),
	}
	if err := snapshot.Validate(); err != nil {
		return nil, "", err
	}
	closeHeld = false
	return snapshot, hex.EncodeToString(hasher.Sum(nil)), nil
}

func hashTraceArchiveInput(ctx context.Context, input conversionInputView) (string, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	hasher := sha256.New()
	buffer := make([]byte, 256<<10)
	for offset := int64(0); offset < input.Size(); {
		if err := ctx.Err(); err != nil {
			return "", err
		}
		want := int64(len(buffer))
		if remaining := input.Size() - offset; remaining < want {
			want = remaining
		}
		n, err := input.ReadAt(buffer[:int(want)], offset)
		if n != int(want) {
			return "", traceArchiveFailure(traceArchiveCodeMemberIntegrity, "", fmt.Errorf("hash archive short read at %d: got=%d want=%d: %w", offset, n, want, firstNonNilError(err, io.ErrUnexpectedEOF)))
		}
		if err != nil && err != io.EOF {
			return "", err
		}
		_, _ = hasher.Write(buffer[:n])
		offset += int64(n)
	}
	return hex.EncodeToString(hasher.Sum(nil)), nil
}

type traceArchiveMemberInput struct {
	archive  *conversionInputAuthority
	snapshot *externalToolInputSnapshot
	display  string
	leaf     string
	member   string
}

func (input *traceArchiveMemberInput) Size() int64 {
	if input == nil || input.snapshot == nil {
		return 0
	}
	return input.snapshot.size
}

func (input *traceArchiveMemberInput) DisplayPath() string {
	if input == nil {
		return ""
	}
	return input.display
}

func (input *traceArchiveMemberInput) ReadAt(buffer []byte, offset int64) (int, error) {
	if input == nil || input.snapshot == nil {
		return 0, conversionInputFailure(ConversionInputCodeClosed, conversionInputStageArchiveIntake, input.DisplayPath(), errors.New("archive member input is closed"))
	}
	input.snapshot.mu.RLock()
	defer input.snapshot.mu.RUnlock()
	if input.snapshot.closed || input.snapshot.file == nil {
		return 0, conversionInputFailure(ConversionInputCodeClosed, conversionInputStageArchiveIntake, input.display, errors.New("archive member input is closed"))
	}
	if offset < 0 {
		return 0, conversionInputFailure(ConversionInputCodeInvalidRange, conversionInputStageArchiveIntake, input.display, nil)
	}
	if len(buffer) == 0 {
		return 0, nil
	}
	if offset >= input.snapshot.size {
		return 0, io.EOF
	}
	remaining := input.snapshot.size - offset
	limited := buffer
	truncated := int64(len(buffer)) > remaining
	if truncated {
		limited = buffer[:int(remaining)]
	}
	n, err := input.snapshot.file.ReadAt(limited, offset)
	if err == nil && truncated {
		err = io.EOF
	}
	return n, err
}

func (input *traceArchiveMemberInput) Validate(stage conversionInputStage) error {
	if input == nil || input.archive == nil || input.snapshot == nil || !stage.valid() {
		return conversionInputFailure(ConversionInputCodeInternalContract, stage, input.DisplayPath(), errors.New("archive member input contract is incomplete"))
	}
	if err := input.archive.Validate(stage); err != nil {
		return err
	}
	if err := input.snapshot.Validate(); err != nil {
		return conversionInputFailure(ConversionInputCodeGenerationChanged, stage, input.display, err)
	}
	return input.archive.Validate(stage)
}

func (input *traceArchiveMemberInput) withOpenFile(fn func(*os.File) error) error {
	if input == nil || input.snapshot == nil || fn == nil {
		return conversionInputFailure(ConversionInputCodeInternalContract, conversionInputStageExternalTool, input.DisplayPath(), errors.New("archive member file callback is incomplete"))
	}
	input.snapshot.mu.RLock()
	defer input.snapshot.mu.RUnlock()
	if input.snapshot.closed || input.snapshot.file == nil {
		return conversionInputFailure(ConversionInputCodeClosed, conversionInputStageExternalTool, input.display, errors.New("archive member input is closed"))
	}
	return fn(input.snapshot.file)
}

func (*traceArchiveMemberInput) externalToolWholeFileSource() {}

func (input *traceArchiveMemberInput) traceStreamerSnapshotLeaf() string {
	if input == nil {
		return ""
	}
	return input.leaf
}

func (input *traceArchiveMemberInput) Close() error {
	if input == nil || input.snapshot == nil {
		return nil
	}
	return input.snapshot.Close()
}

func validateTraceArchiveProvenance(provenance *TraceArchiveProvenance) error {
	if provenance == nil || provenance.Format != "zip" || provenance.ArchiveBytes <= 0 || provenance.MemberBytes <= 0 ||
		(provenance.Selection != "unique_candidate" && provenance.Selection != "explicit_member") {
		return traceArchiveFailure(traceArchiveCodeMemberIntegrity, "", errors.New("archive provenance tuple is incomplete"))
	}
	if _, err := validateExplicitTraceArchiveMember(provenance.Member); err != nil {
		return err
	}
	extension := strings.ToLower(path.Ext(provenance.Member))
	if extension != ".sys" && extension != ".htrace" {
		return traceArchiveFailure(traceArchiveCodeMemberIntegrity, provenance.Member, errors.New("archive provenance member extension is outside the closed candidate set"))
	}
	if err := tracebundle.ValidateSHA256(provenance.ArchiveSHA256); err != nil {
		return traceArchiveFailure(traceArchiveCodeMemberIntegrity, provenance.Member, fmt.Errorf("archive sha256: %w", err))
	}
	if err := tracebundle.ValidateSHA256(provenance.MemberSHA256); err != nil {
		return traceArchiveFailure(traceArchiveCodeMemberIntegrity, provenance.Member, fmt.Errorf("member sha256: %w", err))
	}
	return nil
}

func cloneTraceArchiveProvenance(provenance *TraceArchiveProvenance) *TraceArchiveProvenance {
	if provenance == nil {
		return nil
	}
	clone := *provenance
	return &clone
}

var _ conversionInputView = (*traceArchiveMemberInput)(nil)
var _ externalToolWholeFileSource = (*traceArchiveMemberInput)(nil)
