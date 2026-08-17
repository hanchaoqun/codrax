package hitraceconv

import (
	"archive/zip"
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

type traceArchiveTestMember struct {
	name    string
	body    []byte
	method  uint16
	mode    os.FileMode
	nonUTF8 bool
}

func traceArchiveTestBuiltinBody() []byte {
	var capture bytes.Buffer
	writeFileHeader(&capture, 1)
	writeSegment(&capture, segmentEventsFormat, []byte(syntheticEventFormat()))
	writeSegment(&capture, segmentCmdlines, []byte("36379 archive-app\n"))
	writeSegment(&capture, segmentTGIDs, []byte("36379 36379\n"))
	writeSegment(&capture, segmentRawTrace, syntheticRawPage())
	return capture.Bytes()
}

func traceArchiveTestZIP(t *testing.T, path string, members ...traceArchiveTestMember) []byte {
	t.Helper()
	var body bytes.Buffer
	writer := zip.NewWriter(&body)
	for _, member := range members {
		header := &zip.FileHeader{Name: member.name, Method: member.method}
		header.NonUTF8 = member.nonUTF8
		if header.Method == 0 {
			header.Method = zip.Store
		}
		if member.mode != 0 {
			header.SetMode(member.mode)
		}
		entry, err := writer.CreateHeader(header)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := entry.Write(member.body); err != nil {
			t.Fatal(err)
		}
	}
	if err := writer.Close(); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, body.Bytes(), 0o600); err != nil {
		t.Fatal(err)
	}
	return append([]byte(nil), body.Bytes()...)
}

func traceArchiveTestForceZIP64(t *testing.T, body []byte) []byte {
	t.Helper()
	eocdOffset := bytes.LastIndex(body, []byte("PK\x05\x06"))
	if eocdOffset < 0 || len(body)-eocdOffset < traceArchiveZIPEndRecordFixedBytes {
		t.Fatal("normal ZIP has no EOCD")
	}
	eocd := append([]byte(nil), body[eocdOffset:]...)
	entries := binary.LittleEndian.Uint16(eocd[10:12])
	centralSize := binary.LittleEndian.Uint32(eocd[12:16])
	centralOffset := binary.LittleEndian.Uint32(eocd[16:20])
	binary.LittleEndian.PutUint16(eocd[8:10], 0xffff)
	binary.LittleEndian.PutUint16(eocd[10:12], 0xffff)
	binary.LittleEndian.PutUint32(eocd[12:16], 0xffffffff)
	binary.LittleEndian.PutUint32(eocd[16:20], 0xffffffff)

	record := make([]byte, traceArchiveZIP64EndRecordMinimumBytes)
	binary.LittleEndian.PutUint32(record[0:4], 0x06064b50)
	binary.LittleEndian.PutUint64(record[4:12], 44)
	binary.LittleEndian.PutUint16(record[12:14], 45)
	binary.LittleEndian.PutUint16(record[14:16], 45)
	binary.LittleEndian.PutUint64(record[24:32], uint64(entries))
	binary.LittleEndian.PutUint64(record[32:40], uint64(entries))
	binary.LittleEndian.PutUint64(record[40:48], uint64(centralSize))
	binary.LittleEndian.PutUint64(record[48:56], uint64(centralOffset))
	locator := make([]byte, traceArchiveZIP64LocatorBytes)
	binary.LittleEndian.PutUint32(locator[0:4], 0x07064b50)
	binary.LittleEndian.PutUint64(locator[8:16], uint64(eocdOffset))
	binary.LittleEndian.PutUint32(locator[16:20], 1)

	out := append([]byte(nil), body[:eocdOffset]...)
	out = append(out, record...)
	out = append(out, locator...)
	out = append(out, eocd...)
	return out
}

func traceArchiveTestSHA(body []byte) string {
	sum := sha256.Sum256(body)
	return hex.EncodeToString(sum[:])
}

func assertTraceArchiveCode(t *testing.T, err error, code string) {
	t.Helper()
	var typed *TraceArchiveError
	if !errors.As(err, &typed) || typed.Code != code {
		t.Fatalf("archive error=%T %v; want code=%q", err, err, code)
	}
}

func assertNoTraceArchivePublication(t *testing.T, output string) {
	t.Helper()
	for _, path := range []string{output, traceSidecarBase("", output) + ".tracebundle.json"} {
		if _, err := os.Lstat(path); !os.IsNotExist(err) {
			t.Fatalf("failed archive conversion leaked %q: %v", path, err)
		}
	}
	matches, err := filepath.Glob(filepath.Join(filepath.Dir(output), "."+filepath.Base(output)+".*.archive"))
	if err != nil {
		t.Fatal(err)
	}
	if len(matches) != 0 {
		t.Fatalf("failed archive conversion leaked private roots: %v", matches)
	}
}

func TestTraceArchiveZIPBuiltinConversionPublishesOuterAndMemberProvenance(t *testing.T) {
	dir := t.TempDir()
	input := filepath.Join(dir, "customer.sys.zip")
	output := filepath.Join(dir, "converted.systrace")
	memberBody := traceArchiveTestBuiltinBody()
	archiveBody := traceArchiveTestZIP(t, input, traceArchiveTestMember{
		name: "capture/session.htrace", body: memberBody, method: zip.Deflate,
	})

	result, err := ConvertFile(context.Background(), Options{
		InputPath: input, OutputPath: output, TraceEngine: traceEngineBuiltin,
	})
	if err != nil {
		t.Fatal(err)
	}
	if result.InputPath != input || result.InputBytes != int64(len(archiveBody)) || result.OutputPath != output {
		t.Fatalf("public archive result drifted: %+v", result)
	}
	want := &TraceArchiveProvenance{
		Format: "zip", ArchiveBytes: int64(len(archiveBody)), ArchiveSHA256: traceArchiveTestSHA(archiveBody),
		Member: "capture/session.htrace", MemberBytes: int64(len(memberBody)), MemberSHA256: traceArchiveTestSHA(memberBody),
		Selection: "unique_candidate",
	}
	if result.ArchiveProvenance == nil || *result.ArchiveProvenance != *want {
		t.Fatalf("archive provenance=%+v want=%+v", result.ArchiveProvenance, want)
	}
	manifestBody, err := os.ReadFile(result.BundlePath)
	if err != nil {
		t.Fatal(err)
	}
	var metadata traceBundleMetadata
	if err := json.Unmarshal(manifestBody, &metadata); err != nil {
		t.Fatal(err)
	}
	if metadata.ArchiveProvenance == nil || *metadata.ArchiveProvenance != *want {
		t.Fatalf("bundle archive provenance=%+v want=%+v", metadata.ArchiveProvenance, want)
	}
	if strings.Contains(string(manifestBody), ".archive/") || strings.Contains(string(manifestBody), traceArchiveMemberSnapshotLeaf) {
		t.Fatalf("bundle leaked private archive staging path: %s", manifestBody)
	}
	matches, err := filepath.Glob(filepath.Join(dir, ".converted.systrace.*.archive"))
	if err != nil || len(matches) != 0 {
		t.Fatalf("successful archive conversion retained private roots=%v err=%v", matches, err)
	}
}

func TestTraceArchiveZIPExplicitSelectionAndFakeExtension(t *testing.T) {
	body := traceArchiveTestBuiltinBody()
	t.Run("multiple requires exact explicit member", func(t *testing.T) {
		dir := t.TempDir()
		input := filepath.Join(dir, "two.zip")
		output := filepath.Join(dir, "out.systrace")
		traceArchiveTestZIP(t, input,
			traceArchiveTestMember{name: "a.sys", body: body},
			traceArchiveTestMember{name: "nested/b.htrace", body: body},
		)
		_, err := ConvertFile(context.Background(), Options{InputPath: input, OutputPath: output, TraceEngine: traceEngineBuiltin})
		assertTraceArchiveCode(t, err, traceArchiveCodeMultipleCandidates)
		assertNoTraceArchivePublication(t, output)

		result, err := ConvertFile(context.Background(), Options{
			InputPath: input, OutputPath: output, TraceEngine: traceEngineBuiltin, ArchiveMember: "nested/b.htrace",
		})
		if err != nil {
			t.Fatal(err)
		}
		if result.ArchiveProvenance == nil || result.ArchiveProvenance.Member != "nested/b.htrace" || result.ArchiveProvenance.Selection != "explicit_member" {
			t.Fatalf("explicit member provenance=%+v", result.ArchiveProvenance)
		}
	})

	t.Run("suffix alone is not an archive gate", func(t *testing.T) {
		dir := t.TempDir()
		input := filepath.Join(dir, "plain.zip")
		output := filepath.Join(dir, "plain.systrace")
		if err := os.WriteFile(input, body, 0o600); err != nil {
			t.Fatal(err)
		}
		result, err := ConvertFile(context.Background(), Options{InputPath: input, OutputPath: output, TraceEngine: traceEngineBuiltin})
		if err != nil {
			t.Fatal(err)
		}
		if result.ArchiveProvenance != nil {
			t.Fatalf("extension-only input gained archive provenance: %+v", result.ArchiveProvenance)
		}
		badOutput := filepath.Join(dir, "bad.systrace")
		_, err = ConvertFile(context.Background(), Options{InputPath: input, OutputPath: badOutput, TraceEngine: traceEngineBuiltin, ArchiveMember: "x.sys"})
		assertTraceArchiveCode(t, err, traceArchiveCodeExplicitMember)
		assertNoTraceArchivePublication(t, badOutput)
	})

	t.Run("unknown explicit member fails closed", func(t *testing.T) {
		dir := t.TempDir()
		input := filepath.Join(dir, "one.zip")
		output := filepath.Join(dir, "out.systrace")
		traceArchiveTestZIP(t, input, traceArchiveTestMember{name: "only.sys", body: body})
		_, err := ConvertFile(context.Background(), Options{
			InputPath: input, OutputPath: output, TraceEngine: traceEngineBuiltin, ArchiveMember: "missing.sys",
		})
		assertTraceArchiveCode(t, err, traceArchiveCodeExplicitMember)
		assertNoTraceArchivePublication(t, output)
	})

	t.Run("padded explicit member is not normalized", func(t *testing.T) {
		dir := t.TempDir()
		input := filepath.Join(dir, "one.zip")
		output := filepath.Join(dir, "out.systrace")
		traceArchiveTestZIP(t, input, traceArchiveTestMember{name: "only.sys", body: body})
		_, err := ConvertFile(context.Background(), Options{
			InputPath: input, OutputPath: output, TraceEngine: traceEngineBuiltin, ArchiveMember: " only.sys ",
		})
		assertTraceArchiveCode(t, err, traceArchiveCodeExplicitMember)
		assertNoTraceArchivePublication(t, output)
	})
}

func TestTraceArchiveZIPAcceptsCanonicalZIP64(t *testing.T) {
	dir := t.TempDir()
	input := filepath.Join(dir, "zip64.zip")
	output := filepath.Join(dir, "zip64.systrace")
	memberBody := traceArchiveTestBuiltinBody()
	normal := traceArchiveTestZIP(t, input, traceArchiveTestMember{name: "capture.sys", body: memberBody})
	zip64 := traceArchiveTestForceZIP64(t, normal)
	if err := os.WriteFile(input, zip64, 0o600); err != nil {
		t.Fatal(err)
	}
	result, err := ConvertFile(context.Background(), Options{InputPath: input, OutputPath: output, TraceEngine: traceEngineBuiltin})
	if err != nil {
		t.Fatal(err)
	}
	if result.ArchiveProvenance == nil || result.ArchiveProvenance.ArchiveSHA256 != traceArchiveTestSHA(zip64) || result.ArchiveProvenance.Member != "capture.sys" {
		t.Fatalf("ZIP64 provenance=%+v", result.ArchiveProvenance)
	}
}

func TestTraceArchiveZIP64RejectsLegacyTupleDisagreement(t *testing.T) {
	dir := t.TempDir()
	input := filepath.Join(dir, "bad-zip64.zip")
	output := filepath.Join(dir, "out.systrace")
	normal := traceArchiveTestZIP(t, input, traceArchiveTestMember{name: "capture.sys", body: traceArchiveTestBuiltinBody()})
	zip64 := traceArchiveTestForceZIP64(t, normal)
	eocd := bytes.LastIndex(zip64, []byte("PK\x05\x06"))
	binary.LittleEndian.PutUint16(zip64[eocd+8:eocd+10], 2)
	if err := os.WriteFile(input, zip64, 0o600); err != nil {
		t.Fatal(err)
	}
	_, err := ConvertFile(context.Background(), Options{InputPath: input, OutputPath: output, TraceEngine: traceEngineBuiltin})
	assertTraceArchiveCode(t, err, traceArchiveCodeInvalidZIP)
	assertNoTraceArchivePublication(t, output)
}

func TestTraceArchiveZIPRejectsUnsafeCentralDirectory(t *testing.T) {
	body := traceArchiveTestBuiltinBody()
	tests := []struct {
		name    string
		members []traceArchiveTestMember
		code    string
	}{
		{name: "empty archive", code: traceArchiveCodeNoCandidate},
		{name: "zero candidate", members: []traceArchiveTestMember{{name: "readme.txt", body: []byte("x")}}, code: traceArchiveCodeNoCandidate},
		{name: "duplicate", members: []traceArchiveTestMember{{name: "x.sys", body: body}, {name: "x.sys", body: body}}, code: traceArchiveCodeDuplicateMember},
		{name: "parent", members: []traceArchiveTestMember{{name: "../x.sys", body: body}}, code: traceArchiveCodeInvalidMember},
		{name: "absolute", members: []traceArchiveTestMember{{name: "/x.sys", body: body}}, code: traceArchiveCodeInvalidMember},
		{name: "drive", members: []traceArchiveTestMember{{name: "C:/x.sys", body: body}}, code: traceArchiveCodeInvalidMember},
		{name: "backslash", members: []traceArchiveTestMember{{name: `dir\x.sys`, body: body}}, code: traceArchiveCodeInvalidMember},
		{name: "non utf8", members: []traceArchiveTestMember{{name: string([]byte{0xff}) + ".sys", body: body, nonUTF8: true}}, code: traceArchiveCodeInvalidMember},
		{name: "symlink", members: []traceArchiveTestMember{{name: "link.sys", body: []byte("target"), mode: os.ModeSymlink | 0o777}}, code: traceArchiveCodeSpecialMember},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			dir := t.TempDir()
			input := filepath.Join(dir, "input.zip")
			output := filepath.Join(dir, "out.systrace")
			traceArchiveTestZIP(t, input, test.members...)
			_, err := ConvertFile(context.Background(), Options{InputPath: input, OutputPath: output, TraceEngine: traceEngineBuiltin})
			assertTraceArchiveCode(t, err, test.code)
			assertNoTraceArchivePublication(t, output)
		})
	}
}

func TestTraceArchiveZIPMemberAndOuterGenerationRemainBound(t *testing.T) {
	prepare := func(t *testing.T) (*traceConversionInput, string) {
		t.Helper()
		dir := t.TempDir()
		input := filepath.Join(dir, "input.zip")
		traceArchiveTestZIP(t, input, traceArchiveTestMember{name: "capture.sys", body: traceArchiveTestBuiltinBody()})
		authority, err := openConversionInputAuthority(input)
		if err != nil {
			t.Fatal(err)
		}
		probe, err := authority.Probe()
		if err != nil {
			_ = authority.Close()
			t.Fatal(err)
		}
		route, err := prepareTraceConversionInput(context.Background(), Options{
			InputPath: input, OutputPath: filepath.Join(dir, "out.systrace"),
		}, authority, probe)
		if err != nil {
			_ = authority.Close()
			t.Fatal(err)
		}
		return route, input
	}

	t.Run("outer archive replacement", func(t *testing.T) {
		route, input := prepare(t)
		defer route.Close()
		old := input + ".old"
		if err := os.Rename(input, old); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(input, []byte("replacement"), 0o600); err != nil {
			t.Fatal(err)
		}
		err := route.input.Validate(conversionInputStageBuiltinMetadata)
		var typed *ConversionInputError
		if !errors.As(err, &typed) || typed.Code != ConversionInputCodeGenerationChanged {
			t.Fatalf("outer generation error=%T %v", err, err)
		}
	})

	t.Run("sealed member path replacement", func(t *testing.T) {
		route, _ := prepare(t)
		defer route.Close()
		memberPath, err := route.staging.ChildPath(traceArchiveMemberSnapshotLeaf)
		if err != nil {
			t.Fatal(err)
		}
		if err := os.Rename(memberPath, memberPath+".old"); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(memberPath, traceArchiveTestBuiltinBody(), 0o600); err != nil {
			t.Fatal(err)
		}
		err = route.input.Validate(conversionInputStageBuiltinMetadata)
		var typed *ConversionInputError
		if !errors.As(err, &typed) || typed.Code != ConversionInputCodeGenerationChanged {
			t.Fatalf("member generation error=%T %v", err, err)
		}
	})
}

func TestTraceArchiveZIPRejectsEncryptedAndResourceLimitInputs(t *testing.T) {
	body := traceArchiveTestBuiltinBody()
	t.Run("encrypted flag", func(t *testing.T) {
		dir := t.TempDir()
		input := filepath.Join(dir, "encrypted.zip")
		output := filepath.Join(dir, "out.systrace")
		archive := traceArchiveTestZIP(t, input, traceArchiveTestMember{name: "x.sys", body: body})
		central := bytes.Index(archive, []byte("PK\x01\x02"))
		if central < 0 {
			t.Fatal("central header missing")
		}
		binary.LittleEndian.PutUint16(archive[6:8], binary.LittleEndian.Uint16(archive[6:8])|1)
		binary.LittleEndian.PutUint16(archive[central+8:central+10], binary.LittleEndian.Uint16(archive[central+8:central+10])|1)
		if err := os.WriteFile(input, archive, 0o600); err != nil {
			t.Fatal(err)
		}
		_, err := ConvertFile(context.Background(), Options{InputPath: input, OutputPath: output, TraceEngine: traceEngineBuiltin})
		assertTraceArchiveCode(t, err, traceArchiveCodeEncryptedMember)
		assertNoTraceArchivePublication(t, output)
	})

	t.Run("entry budget", func(t *testing.T) {
		dir := t.TempDir()
		input := filepath.Join(dir, "entries.zip")
		output := filepath.Join(dir, "out.systrace")
		members := make([]traceArchiveTestMember, traceArchiveZIPMaxEntries+1)
		for index := range members {
			members[index].name = "metadata/entry-" + leftPadDecimal(index, 4) + ".txt"
			members[index].body = []byte("x")
		}
		traceArchiveTestZIP(t, input, members...)
		_, err := ConvertFile(context.Background(), Options{InputPath: input, OutputPath: output, TraceEngine: traceEngineBuiltin})
		assertTraceArchiveCode(t, err, traceArchiveCodeResourceLimit)
		assertNoTraceArchivePublication(t, output)
	})

	t.Run("central directory budget", func(t *testing.T) {
		dir := t.TempDir()
		input := filepath.Join(dir, "central.zip")
		output := filepath.Join(dir, "out.systrace")
		archive := traceArchiveTestZIP(t, input, traceArchiveTestMember{name: "x.sys", body: body})
		eocd := bytes.LastIndex(archive, []byte("PK\x05\x06"))
		binary.LittleEndian.PutUint32(archive[eocd+12:eocd+16], traceArchiveZIPMaxCentralDirectory+1)
		if err := os.WriteFile(input, archive, 0o600); err != nil {
			t.Fatal(err)
		}
		_, err := ConvertFile(context.Background(), Options{InputPath: input, OutputPath: output, TraceEngine: traceEngineBuiltin})
		assertTraceArchiveCode(t, err, traceArchiveCodeResourceLimit)
		assertNoTraceArchivePublication(t, output)
	})

	t.Run("compression ratio", func(t *testing.T) {
		dir := t.TempDir()
		input := filepath.Join(dir, "ratio.zip")
		output := filepath.Join(dir, "out.systrace")
		traceArchiveTestZIP(t, input, traceArchiveTestMember{name: "bomb.sys", body: make([]byte, 16<<20), method: zip.Deflate})
		_, err := ConvertFile(context.Background(), Options{InputPath: input, OutputPath: output, TraceEngine: traceEngineBuiltin})
		assertTraceArchiveCode(t, err, traceArchiveCodeResourceLimit)
		assertNoTraceArchivePublication(t, output)
	})
}

func TestTraceArchiveZIPPhysicalSizeLimitBoundsWholeArchiveHashing(t *testing.T) {
	dir := t.TempDir()
	input := filepath.Join(dir, "oversized.zip")
	file, err := os.OpenFile(input, os.O_CREATE|os.O_RDWR|os.O_TRUNC, 0o600)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := file.Write([]byte("PK\x03\x04")); err != nil {
		_ = file.Close()
		t.Fatal(err)
	}
	if err := file.Truncate(traceArchiveZIPMaxArchiveBytes + 1); err != nil {
		_ = file.Close()
		t.Skipf("filesystem cannot create a sparse archive limit fixture: %v", err)
	}
	if err := file.Close(); err != nil {
		t.Fatal(err)
	}
	output := filepath.Join(dir, "out.systrace")
	_, err = ConvertFile(context.Background(), Options{InputPath: input, OutputPath: output, TraceEngine: traceEngineBuiltin})
	assertTraceArchiveCode(t, err, traceArchiveCodeResourceLimit)
	assertNoTraceArchivePublication(t, output)
}

func leftPadDecimal(value, width int) string {
	text := ""
	for value > 0 {
		text = string(rune('0'+value%10)) + text
		value /= 10
	}
	if text == "" {
		text = "0"
	}
	for len(text) < width {
		text = "0" + text
	}
	return text
}

func TestTraceArchiveZIPRejectsContainerAndMemberIntegrityFailures(t *testing.T) {
	body := traceArchiveTestBuiltinBody()
	t.Run("truncated end record", func(t *testing.T) {
		dir := t.TempDir()
		input := filepath.Join(dir, "bad.zip")
		output := filepath.Join(dir, "out.systrace")
		archive := traceArchiveTestZIP(t, input, traceArchiveTestMember{name: "x.sys", body: body})
		if err := os.WriteFile(input, archive[:len(archive)-8], 0o600); err != nil {
			t.Fatal(err)
		}
		_, err := ConvertFile(context.Background(), Options{InputPath: input, OutputPath: output, TraceEngine: traceEngineBuiltin})
		assertTraceArchiveCode(t, err, traceArchiveCodeInvalidZIP)
		assertNoTraceArchivePublication(t, output)
	})

	t.Run("multi disk", func(t *testing.T) {
		dir := t.TempDir()
		input := filepath.Join(dir, "multi.zip")
		output := filepath.Join(dir, "out.systrace")
		archive := traceArchiveTestZIP(t, input, traceArchiveTestMember{name: "x.sys", body: body})
		eocd := bytes.LastIndex(archive, []byte("PK\x05\x06"))
		binary.LittleEndian.PutUint16(archive[eocd+4:eocd+6], 1)
		if err := os.WriteFile(input, archive, 0o600); err != nil {
			t.Fatal(err)
		}
		_, err := ConvertFile(context.Background(), Options{InputPath: input, OutputPath: output, TraceEngine: traceEngineBuiltin})
		assertTraceArchiveCode(t, err, traceArchiveCodeMultiDisk)
		assertNoTraceArchivePublication(t, output)
	})

	t.Run("crc mismatch", func(t *testing.T) {
		dir := t.TempDir()
		input := filepath.Join(dir, "crc.zip")
		output := filepath.Join(dir, "out.systrace")
		archive := traceArchiveTestZIP(t, input, traceArchiveTestMember{name: "x.sys", body: body, method: zip.Store})
		nameLen := int(binary.LittleEndian.Uint16(archive[26:28]))
		extraLen := int(binary.LittleEndian.Uint16(archive[28:30]))
		archive[30+nameLen+extraLen] ^= 0xff
		if err := os.WriteFile(input, archive, 0o600); err != nil {
			t.Fatal(err)
		}
		_, err := ConvertFile(context.Background(), Options{InputPath: input, OutputPath: output, TraceEngine: traceEngineBuiltin})
		assertTraceArchiveCode(t, err, traceArchiveCodeMemberIntegrity)
		assertNoTraceArchivePublication(t, output)
	})

	t.Run("nested zip", func(t *testing.T) {
		dir := t.TempDir()
		innerPath := filepath.Join(dir, "inner.zip")
		inner := traceArchiveTestZIP(t, innerPath, traceArchiveTestMember{name: "real.sys", body: body})
		input := filepath.Join(dir, "outer.zip")
		output := filepath.Join(dir, "out.systrace")
		traceArchiveTestZIP(t, input, traceArchiveTestMember{name: "nested.sys", body: inner})
		_, err := ConvertFile(context.Background(), Options{InputPath: input, OutputPath: output, TraceEngine: traceEngineBuiltin})
		assertTraceArchiveCode(t, err, traceArchiveCodeNestedArchive)
		assertNoTraceArchivePublication(t, output)
	})
}

func TestTraceArchiveZIPTraceStreamerUsesSelectedMemberBasename(t *testing.T) {
	dir := t.TempDir()
	input := filepath.Join(dir, "archive.zip")
	traceArchiveTestZIP(t, input, traceArchiveTestMember{name: "nested/capture.sys", body: traceArchiveTestBuiltinBody()})
	authority, err := openConversionInputAuthority(input)
	if err != nil {
		t.Fatal(err)
	}
	route := newTraceConversionInput(authority)
	defer func() { _ = route.Close() }()
	probe, err := authority.Probe()
	if err != nil {
		t.Fatal(err)
	}
	route, err = prepareTraceConversionInput(context.Background(), Options{InputPath: input, OutputPath: filepath.Join(dir, "out.systrace")}, authority, probe)
	if err != nil {
		t.Fatal(err)
	}
	leaf, err := traceStreamerInputSnapshotLeafForView(route.input)
	if err != nil {
		t.Fatal(err)
	}
	if leaf != "capture.sys" {
		t.Fatalf("trace_streamer snapshot leaf=%q", leaf)
	}
}

func TestTraceArchiveZIPDefaultOutputPreservesBuiltinBytes(t *testing.T) {
	dir := t.TempDir()
	body := traceArchiveTestBuiltinBody()
	rawInput := filepath.Join(dir, "capture.sys")
	if err := os.WriteFile(rawInput, body, 0o600); err != nil {
		t.Fatal(err)
	}
	rawOutput := filepath.Join(dir, "raw.systrace")
	if _, err := ConvertFile(context.Background(), Options{
		InputPath: rawInput, OutputPath: rawOutput, TraceEngine: traceEngineBuiltin,
	}); err != nil {
		t.Fatalf("convert direct builtin input: %v", err)
	}

	archiveInput := filepath.Join(dir, "capture.zip")
	traceArchiveTestZIP(t, archiveInput, traceArchiveTestMember{
		name: "nested/capture.sys", body: body, method: zip.Deflate,
	})
	result, err := ConvertFile(context.Background(), Options{
		InputPath: archiveInput, TraceEngine: traceEngineBuiltin,
	})
	if err != nil {
		t.Fatalf("convert archived builtin input: %v", err)
	}
	if result.OutputPath != DefaultOutputPath(archiveInput) || result.BundlePath == "" || result.ArchiveProvenance == nil {
		t.Fatalf("archive default publication drifted: %+v", result)
	}
	rawBytes, err := os.ReadFile(rawOutput)
	if err != nil {
		t.Fatal(err)
	}
	archiveBytes, err := os.ReadFile(result.OutputPath)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(archiveBytes, rawBytes) {
		t.Fatal("archive intake changed builtin systrace bytes")
	}
}

func TestTraceArchiveZIPDirectRawPerfPublishesArchiveProvenance(t *testing.T) {
	dir := t.TempDir()
	input := filepath.Join(dir, "perf-capture.zip")
	perfBody := syntheticRawPerfData()
	traceArchiveTestZIP(t, input, traceArchiveTestMember{
		name: "perf/session.sys", body: perfBody, method: zip.Deflate,
	})
	ignoredOutput := filepath.Join(dir, "ignored.systrace")
	result, err := ConvertFile(context.Background(), Options{
		InputPath: input, OutputPath: ignoredOutput, PerfParser: "raw",
	})
	if err != nil {
		t.Fatalf("convert archived direct perf input: %v", err)
	}
	if result.OutputPath != "" || result.EventsWritten != 0 || result.BundlePath == "" {
		t.Fatalf("archived direct perf route drifted: %+v", result)
	}
	if result.ArchiveProvenance == nil || result.ArchiveProvenance.Member != "perf/session.sys" ||
		result.ArchiveProvenance.MemberSHA256 != traceArchiveTestSHA(perfBody) {
		t.Fatalf("archived direct perf provenance=%+v", result.ArchiveProvenance)
	}
	var perfTrace Artifact
	for _, artifact := range result.Artifacts {
		if artifact.Type == ArtifactPerfTrace {
			perfTrace = artifact
		}
	}
	if perfTrace.Path == "" || perfTrace.Perf == nil || perfTrace.Perf.ProviderName != perfProviderNameRawFallback {
		t.Fatalf("archived direct perf artifact=%+v artifacts=%+v", perfTrace, result.Artifacts)
	}
	if _, err := os.Lstat(ignoredOutput); !os.IsNotExist(err) {
		t.Fatalf("direct perf archive created a systrace output: %v", err)
	}
	manifest, err := os.ReadFile(result.BundlePath)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Contains(manifest, []byte(`"archive_provenance"`)) ||
		bytes.Contains(manifest, []byte(traceArchiveMemberSnapshotLeaf)) || bytes.Contains(manifest, []byte(".archive/")) {
		t.Fatalf("direct perf archive bundle provenance/private-path mismatch: %s", manifest)
	}
}

func TestTraceArchiveZIPTraceStreamerConsumesSelectedMemberInExplicitAndAutoModes(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("fake trace_streamer shell fixture uses /bin/sh")
	}
	for _, engine := range []string{traceEngineTraceStreamer, traceEngineAuto} {
		t.Run(engine, func(t *testing.T) {
			dir := t.TempDir()
			input := filepath.Join(dir, "customer.zip")
			memberBody := []byte("selected archive member payload\n")
			traceArchiveTestZIP(t, input, traceArchiveTestMember{
				name: "nested/customer-capture.htrace", body: memberBody, method: zip.Deflate,
			})
			fixtureDB := createTraceDBFixture(t, traceStreamerIntegrationDBStatements())
			tool := writeFakeTraceStreamer(t, dir, 0)
			argsLog := filepath.Join(dir, "args.log")
			consumed := filepath.Join(dir, "consumed.bin")
			t.Setenv("TRACE_STREAMER_FIXTURE_DB", fixtureDB)
			t.Setenv("TRACE_STREAMER_ARGS_LOG", argsLog)
			t.Setenv("TRACE_STREAMER_CONSUMED_INPUT", consumed)
			output := filepath.Join(dir, "converted.systrace")
			result, err := ConvertFile(context.Background(), Options{
				InputPath: input, OutputPath: output, TraceEngine: engine, TraceStreamerPath: tool,
			})
			if err != nil {
				t.Fatalf("convert archived trace_streamer input: %v", err)
			}
			got, err := os.ReadFile(consumed)
			if err != nil {
				t.Fatal(err)
			}
			if !bytes.Equal(got, memberBody) {
				t.Fatalf("trace_streamer consumed wrong bytes: got=%q want=%q", got, memberBody)
			}
			args, err := os.ReadFile(argsLog)
			if err != nil {
				t.Fatal(err)
			}
			firstArg := strings.Split(strings.TrimSpace(string(args)), "\n")[0]
			if runtime.GOOS == "linux" {
				if !strings.HasPrefix(firstArg, "/proc/self/fd/") {
					t.Fatalf("Linux trace_streamer did not receive the verified FD transport: %q", firstArg)
				}
			} else if filepath.Base(firstArg) != "customer-capture.htrace" {
				t.Fatalf("snapshot transport lost selected member basename: %q", firstArg)
			}
			manifest, err := os.ReadFile(result.BundlePath)
			if err != nil {
				t.Fatal(err)
			}
			for _, private := range []string{traceArchiveMemberSnapshotLeaf, ".archive/", strings.TrimSuffix(traceStreamerPrivateDirPattern, "*")} {
				if bytes.Contains(manifest, []byte(private)) {
					t.Fatalf("trace_streamer archive bundle leaked private token %q: %s", private, manifest)
				}
			}
		})
	}
}

func TestTraceArchiveZIPExtractionCancellationRollsBackPrivateMember(t *testing.T) {
	dir := t.TempDir()
	input := filepath.Join(dir, "cancel.zip")
	traceArchiveTestZIP(t, input, traceArchiveTestMember{
		name: "capture.sys", body: bytes.Repeat([]byte("trace-payload-"), 16<<10), method: zip.Deflate,
	})
	authority, err := openConversionInputAuthority(input)
	if err != nil {
		t.Fatal(err)
	}
	defer authority.Close()
	directory, err := preflightTraceArchiveZIP(context.Background(), authority)
	if err != nil {
		t.Fatal(err)
	}
	reader, err := zip.NewReader(authority, authority.Size())
	if err != nil {
		t.Fatal(err)
	}
	candidate, _, err := selectTraceArchiveZIPMember(reader, directory, "")
	if err != nil {
		t.Fatal(err)
	}
	staging, err := newPrivateConversionDir(dir, ".cancel.*.archive")
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, _, err = extractSelectedTraceArchiveMember(ctx, input, candidate, staging)
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("archive extraction lost cancellation identity: %T %v", err, err)
	}
	if cleanupErr := staging.FinalizeCleanup(); cleanupErr != nil {
		t.Fatalf("cleanup canceled archive staging: %v", cleanupErr)
	}
	matches, globErr := filepath.Glob(filepath.Join(dir, ".cancel.*.archive"))
	if globErr != nil || len(matches) != 0 {
		t.Fatalf("canceled archive extraction leaked private roots=%v err=%v", matches, globErr)
	}
}
