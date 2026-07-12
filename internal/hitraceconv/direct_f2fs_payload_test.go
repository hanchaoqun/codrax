package hitraceconv

import (
	"encoding/binary"
	"fmt"
	"math"
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/tracequery"
)

type directF2FSTestFixture struct {
	profile directF2FSProfile
	width   int
	format  eventFormat
	content []byte
}

func TestDirectF2FSExactProfileABIAndWireParityMatrix(t *testing.T) {
	for profile := directF2FSProfileSyncEnter; profile <= directF2FSProfileWriteEnd; profile++ {
		for _, width := range []int{4, 8} {
			profile, width := profile, width
			t.Run(fmt.Sprintf("profile%d/arm%d", profile, width*8), func(t *testing.T) {
				fixture := directF2FSTestFixtureFor(profile, width)
				payload, admission, reason := decodeDirectF2FSPayload(decodeEvent(fixture.format, fixture.content))
				if admission != bodyAdmitted || reason != "" || !payload.HeaderOwnerKnown || payload.HeaderTID != 100 {
					t.Fatalf("valid direct F2FS rejected: admission=%d reason=%q payload=%+v", admission, reason, payload)
				}
				body, ok := renderCanonicalF2FSPayload(payload)
				if !ok || !directF2FSWireParity(payload, body, fingerprintPairingEndpoint(f2fsPayloadTypedInput(payload))) {
					t.Fatalf("valid direct F2FS lost typed/wire parity: body=%q payload=%+v", body, payload)
				}
				if want := directF2FSExpectedBody(profile); body != want {
					t.Fatalf("profile canonical body drifted:\n got: %q\nwant: %q", body, want)
				}
				for _, fragment := range []string{"dev=8:0", "ino=0x9"} {
					if !strings.Contains(body, fragment) {
						t.Fatalf("canonical F2FS body lost %q: %q", fragment, body)
					}
				}
				line, lineAdmission, lineReason, envelopeOK := renderEventLineDecision(
					renderContext{cmdlines: map[int]string{100: "f2fs-worker"}, tgids: map[int]int{100: 100}},
					1_000_000, 2, fixture.format, fixture.content,
				)
				if !envelopeOK || lineAdmission != bodyAdmitted || lineReason != "" ||
					!strings.HasSuffix(line, fixture.format.Name+": "+body) || !traceDBSinglePhysicalLine(line, false) {
					t.Fatalf("wrapped F2FS drifted: envelope=%t admission=%d reason=%q line=%q", envelopeOK, lineAdmission, lineReason, line)
				}
			})
		}
	}
}

func TestDirectF2FSDescriptorMutationMatrix(t *testing.T) {
	for profile := directF2FSProfileSyncEnter; profile <= directF2FSProfileWriteEnd; profile++ {
		for _, width := range []int{4, 8} {
			base := directF2FSTestFixtureFor(profile, width)
			for fieldIndex, field := range base.format.Fields {
				fieldIndex, field := fieldIndex, field
				prefix := fmt.Sprintf("profile%d/arm%d/%s", profile, width*8, cleanFieldName(field.Name))
				t.Run(prefix+"/missing", func(t *testing.T) {
					fixture := directF2FSCloneFixture(base)
					fixture.format.Fields = append(fixture.format.Fields[:fieldIndex], fixture.format.Fields[fieldIndex+1:]...)
					directF2FSAssertRejected(t, fixture)
				})
				t.Run(prefix+"/wrong_type", func(t *testing.T) {
					fixture := directF2FSCloneFixture(base)
					fixture.format.Fields[fieldIndex].Type = "unsigned long long"
					directF2FSAssertRejected(t, fixture)
				})
				t.Run(prefix+"/type_case_drift", func(t *testing.T) {
					fixture := directF2FSCloneFixture(base)
					fixture.format.Fields[fieldIndex].Type = strings.ToUpper(fixture.format.Fields[fieldIndex].Type)
					directF2FSAssertRejected(t, fixture)
				})
				t.Run(prefix+"/wrong_sign", func(t *testing.T) {
					fixture := directF2FSCloneFixture(base)
					fixture.format.Fields[fieldIndex].Signed = !fixture.format.Fields[fieldIndex].Signed
					directF2FSAssertRejected(t, fixture)
				})
				t.Run(prefix+"/wrong_offset", func(t *testing.T) {
					fixture := directF2FSCloneFixture(base)
					fixture.format.Fields[fieldIndex].Offset++
					directF2FSAssertRejected(t, fixture)
				})
				t.Run(prefix+"/wrong_size", func(t *testing.T) {
					fixture := directF2FSCloneFixture(base)
					fixture.format.Fields[fieldIndex].Size++
					directF2FSAssertRejected(t, fixture)
				})
				t.Run(prefix+"/truncated_content", func(t *testing.T) {
					fixture := directF2FSCloneFixture(base)
					cut := field.Offset + field.Size - 1
					if cut < 0 {
						cut = 0
					}
					fixture.content = fixture.content[:cut]
					directF2FSAssertRejected(t, fixture)
				})
				t.Run(prefix+"/alias", func(t *testing.T) {
					fixture := directF2FSCloneFixture(base)
					fixture.format.Fields[fieldIndex].Name = cleanFieldName(field.Name) + "_alias"
					directF2FSAssertRejected(t, fixture)
				})
				t.Run(prefix+"/duplicate", func(t *testing.T) {
					fixture := directF2FSCloneFixture(base)
					fixture.format.Fields = append(fixture.format.Fields, fixture.format.Fields[fieldIndex])
					directF2FSAssertRejected(t, fixture)
				})
			}
			fixture := directF2FSCloneFixture(base)
			fixture.format.Fields = append(fixture.format.Fields, eventField{Type: "unsigned int", Name: "vendor", Offset: len(fixture.content), Size: 4})
			fixture.content = append(fixture.content, make([]byte, 4)...)
			directF2FSAssertRejected(t, fixture)
			for _, printFmt := range []string{"", `"synthetic"`, directF2FSPrintFmtLiteral(profile) + " drift"} {
				fixture := directF2FSCloneFixture(base)
				fixture.format.PrintFmt = printFmt
				directF2FSAssertRejected(t, fixture)
			}
		}
	}
}

func TestDirectF2FSScalarAndSharedFinalizerBoundaries(t *testing.T) {
	blocks := directF2FSTestFixtureFor(directF2FSProfileSyncEnter, 8)
	directF2FSPutUint(directF2FSFixtureField(t, &blocks, "blocks"), blocks.content, math.MaxUint64)
	payload, admission, reason := decodeDirectF2FSPayload(decodeEvent(blocks.format, blocks.content))
	body, rendered := renderCanonicalF2FSPayload(payload)
	if admission != bodyAdmitted || reason != "" || !rendered || !strings.Contains(body, "i_blocks=18446744073709551615") ||
		!directF2FSWireParity(payload, body, fingerprintPairingEndpoint(f2fsPayloadTypedInput(payload))) {
		t.Fatalf("blkcnt_t MaxUint64 lost: admission=%d reason=%q body=%q payload=%+v", admission, reason, body, payload)
	}

	for _, tc := range []struct {
		name   string
		mutate func(*directF2FSTestFixture)
	}{
		{name: "zero dev", mutate: func(f *directF2FSTestFixture) { directF2FSPutUint(directF2FSFixtureField(t, f, "dev"), f.content, 0) }},
		{name: "zero inode", mutate: func(f *directF2FSTestFixture) { directF2FSPutUint(directF2FSFixtureField(t, f, "ino"), f.content, 0) }},
		{name: "negative pos", mutate: func(f *directF2FSTestFixture) {
			directF2FSPutUint(directF2FSFixtureField(t, f, "pos"), f.content, math.MaxUint64)
		}},
		{name: "invalid rw", mutate: func(f *directF2FSTestFixture) { directF2FSPutUint(directF2FSFixtureField(t, f, "rw"), f.content, 2) }},
		{name: "length above int64", mutate: func(f *directF2FSTestFixture) {
			directF2FSPutUint(directF2FSFixtureField(t, f, "len"), f.content, uint64(math.MaxInt64)+1)
		}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			fixture := directF2FSTestFixtureFor(directF2FSProfileDirectIOExit, 8)
			tc.mutate(&fixture)
			directF2FSAssertRejected(t, fixture)
		})
	}

	valid := f2fsPayload{Kind: f2fsPayloadDirectIOEnter, Name: "f2fs_direct_IO_enter", HeaderTID: 40, HeaderOwnerKnown: true, Dev: 1, Ino: 2, Len: 1, RW: 0}
	finalizeF2FSPayloadAdmission(&valid)
	if !valid.IdentityKnown || !valid.PayloadAdmitted {
		t.Fatalf("valid shared payload rejected: %+v", valid)
	}
	invalid := []f2fsPayload{
		{Kind: f2fsPayloadWriteBegin, Name: "f2fs_write_end", Dev: 1, Ino: 2, Len: 1},
		{Kind: f2fsPayloadDirectIOEnter, Name: "f2fs_direct_IO_enter", Dev: 1, Ino: 2, Len: 1, RW: 0, KIFieldsPresent: true, KIFlags: math.MaxInt32 + 1},
		{Kind: f2fsPayloadDirectIOEnter, Name: "f2fs_direct_IO_enter", Dev: 1, Ino: 2, Len: 1, RW: 0, KIFieldsPresent: true, KIIoprio: math.MaxUint16 + 1},
		{Kind: f2fsPayloadWriteEnd, Name: "f2fs_write_end", Dev: 1, Ino: 2, Len: 1, FlagsPresent: true},
		{Kind: f2fsPayloadSyncExit, Name: "f2fs_sync_file_exit", Dev: 1, Ino: 2, Ret: math.MaxInt32 + 1},
	}
	for index := range invalid {
		finalizeF2FSPayloadAdmission(&invalid[index])
		if invalid[index].PayloadAdmitted {
			t.Fatalf("invalid shared payload %d escaped finalizer: %+v", index, invalid[index])
		}
	}
}

func TestDirectF2FSSourceWidthBoundaryMatrix(t *testing.T) {
	t.Run("sync-enter-arm32", func(t *testing.T) {
		fixture := directF2FSTestFixtureFor(directF2FSProfileSyncEnter, 4)
		for field, value := range map[string]uint64{
			"pino": math.MaxUint32, "mode": math.MaxUint16, "size": math.MaxInt64,
			"nlink": math.MaxUint32, "blocks": math.MaxUint64, "advise": math.MaxUint8,
		} {
			directF2FSPutUint(directF2FSFixtureField(t, &fixture, field), fixture.content, value)
		}
		body := directF2FSAssertAdmittedBody(t, fixture)
		for _, fragment := range []string{"pino=0xffffffff", "i_mode=0xffff", "i_size=9223372036854775807", "i_nlink=4294967295", "i_blocks=18446744073709551615", "i_advise=0xff"} {
			if !strings.Contains(body, fragment) {
				t.Fatalf("ARM32 sync boundary lost %q: %q", fragment, body)
			}
		}
	})

	t.Run("sync-enter-arm64-inode", func(t *testing.T) {
		fixture := directF2FSTestFixtureFor(directF2FSProfileSyncEnter, 8)
		directF2FSPutUint(directF2FSFixtureField(t, &fixture, "pino"), fixture.content, math.MaxUint64)
		if body := directF2FSAssertAdmittedBody(t, fixture); !strings.Contains(body, "pino=0xffffffffffffffff") {
			t.Fatalf("ARM64 ino_t boundary lost: %q", body)
		}
	})

	for _, width := range []int{4, 8} {
		width := width
		t.Run(fmt.Sprintf("sync-exit-arm%d-signed", width*8), func(t *testing.T) {
			fixture := directF2FSTestFixtureFor(directF2FSProfileSyncExit, width)
			directF2FSPutUint(directF2FSFixtureField(t, &fixture, "cp_reason"), fixture.content, uint64(uint32(1<<31)))
			directF2FSPutUint(directF2FSFixtureField(t, &fixture, "datasync"), fixture.content, math.MaxInt32)
			directF2FSPutUint(directF2FSFixtureField(t, &fixture, "ret"), fixture.content, uint64(uint32(1<<31)))
			body := directF2FSAssertAdmittedBody(t, fixture)
			for _, fragment := range []string{"cp_reason=-2147483648", "datasync=2147483647", "ret=-2147483648"} {
				if !strings.Contains(body, fragment) {
					t.Fatalf("signed int boundary lost %q: %q", fragment, body)
				}
			}
		})

		t.Run(fmt.Sprintf("dio66-arm%d", width*8), func(t *testing.T) {
			fixture := directF2FSTestFixtureFor(directF2FSProfileDirectIOEnter66, width)
			directF2FSPutUint(directF2FSFixtureField(t, &fixture, "ki_pos"), fixture.content, math.MaxInt64)
			directF2FSPutUint(directF2FSFixtureField(t, &fixture, "ki_flags"), fixture.content, uint64(uint32(1<<31)))
			directF2FSPutUint(directF2FSFixtureField(t, &fixture, "ki_ioprio"), fixture.content, math.MaxUint16)
			length := uint64(math.MaxUint32)
			if width == 8 {
				length = math.MaxInt64
			}
			directF2FSPutUint(directF2FSFixtureField(t, &fixture, "len"), fixture.content, length)
			directF2FSPutUint(directF2FSFixtureField(t, &fixture, "rw"), fixture.content, 1)
			body := directF2FSAssertAdmittedBody(t, fixture)
			for _, fragment := range []string{"pos=9223372036854775807", fmt.Sprintf("len=%d", length), "ki_flags=0x80000000", "ki_ioprio=0xffff", "rw=write"} {
				if !strings.Contains(body, fragment) {
					t.Fatalf("DIO source boundary lost %q: %q", fragment, body)
				}
			}
		})
	}

	t.Run("write-u32", func(t *testing.T) {
		begin := directF2FSTestFixtureFor(directF2FSProfileWriteBegin510, 8)
		for _, field := range []string{"len", "flags"} {
			directF2FSPutUint(directF2FSFixtureField(t, &begin, field), begin.content, math.MaxUint32)
		}
		directF2FSPutUint(directF2FSFixtureField(t, &begin, "pos"), begin.content, math.MaxInt64)
		body := directF2FSAssertAdmittedBody(t, begin)
		if !strings.Contains(body, "pos=9223372036854775807 len=4294967295 flags=4294967295") {
			t.Fatalf("write-begin u32/loff_t boundary lost: %q", body)
		}

		end := directF2FSTestFixtureFor(directF2FSProfileWriteEnd, 4)
		directF2FSPutUint(directF2FSFixtureField(t, &end, "copied"), end.content, math.MaxUint32)
		if body := directF2FSAssertAdmittedBody(t, end); !strings.Contains(body, "copied=4294967295") {
			t.Fatalf("write-end copied boundary lost: %q", body)
		}
	})

	t.Run("negative-sync-size", func(t *testing.T) {
		fixture := directF2FSTestFixtureFor(directF2FSProfileSyncEnter, 8)
		directF2FSPutUint(directF2FSFixtureField(t, &fixture, "size"), fixture.content, math.MaxUint64)
		directF2FSAssertRejected(t, fixture)
	})
}

func TestF2FSExactNameAuthorityParityAcrossConverterAndConsumer(t *testing.T) {
	exact := []struct {
		name string
		body string
	}{
		{"f2fs_sync_file_enter", directF2FSExpectedBody(directF2FSProfileSyncEnter)},
		{"f2fs_sync_file_exit", directF2FSExpectedBody(directF2FSProfileSyncExit)},
		{"f2fs_direct_IO_enter", directF2FSExpectedBody(directF2FSProfileDirectIOEnter510)},
		{"f2fs_direct_IO_exit", directF2FSExpectedBody(directF2FSProfileDirectIOExit)},
		{"f2fs_write_begin", directF2FSExpectedBody(directF2FSProfileWriteBegin66)},
		{"f2fs_write_end", directF2FSExpectedBody(directF2FSProfileWriteEnd)},
	}
	for _, tc := range exact {
		if !directF2FSNameGoverned(tc.name) || pairCriticalFormatFamilyForName(tc.name) != pairCriticalFormatFamilyF2FS {
			t.Fatalf("direct/descriptor authority disagrees for exact name %q", tc.name)
		}
		if kind, governed := profilerPairKindForExactName(tc.name); !governed || kind != pairRenderF2FS {
			t.Fatalf("profiler authority disagrees for exact name %q: kind=%d governed=%t", tc.name, kind, governed)
		}
		if verdict := tracequery.DecodePairingEndpoint(tc.name, tc.body, 100); !verdict.Recognized || !verdict.KeyKnown || !verdict.PayloadAdmitted {
			t.Fatalf("tracequery authority disagrees for exact name %q: %+v", tc.name, verdict)
		}
	}

	for _, name := range []string{
		"F2FS_sync_file_enter", "f2fs_sync_file_enter ", "f2fs_sync_file_enter_vendor",
		"f2fs_direct_io_enter", "f2fs_directIO_enter", "f2fs_write_enter",
	} {
		if directF2FSNameGoverned(name) || pairCriticalFormatFamilyForName(name) != 0 {
			t.Fatalf("near name entered direct/descriptor exact registry: %q", name)
		}
		if kind, governed := profilerPairKindForExactName(name); governed || kind != pairRenderUnknown {
			t.Fatalf("near name entered profiler exact registry: %q kind=%d", name, kind)
		}
		if verdict := tracequery.DecodePairingEndpoint(name, directF2FSExpectedBody(directF2FSProfileDirectIOEnter510), 100); verdict.Recognized || verdict.KeyKnown || verdict.PayloadAdmitted {
			t.Fatalf("near name entered tracequery exact registry: %q verdict=%+v", name, verdict)
		}
	}
}

func TestDirectF2FSClosedEndpointCandidateParityAndCompactNearNames(t *testing.T) {
	tests := []struct {
		name      string
		candidate bool
		exact     bool
	}{
		{name: "f2fs_sync_file_enter", candidate: true, exact: true},
		{name: "f2fs_sync_file_exit", candidate: true, exact: true},
		{name: "f2fs_direct_IO_enter", candidate: true, exact: true},
		{name: "f2fs_direct_IO_exit", candidate: true, exact: true},
		{name: "f2fs_write_begin", candidate: true, exact: true},
		{name: "f2fs_write_end", candidate: true, exact: true},

		// Removing every separator must not escape the negative-only endpoint
		// gate. Suffix drift is inventory too, never a legacy body authority.
		{name: "f2fs_syncfileenter", candidate: true},
		{name: " f2fs_syncfileexit_vendor ", candidate: true},
		{name: "f2fs_directIOenter", candidate: true},
		{name: "F2FS_directioexitVendor", candidate: true},
		{name: "f2fs_writebegin", candidate: true},
		{name: "f2fs_writeend_vendor", candidate: true},
		{name: "f2fs_write_start_vendor", candidate: true},
		{name: "f2fs_direct_io_done_vendor", candidate: true},
		{name: "f2fs_sync_file_complete_vendor", candidate: true},

		// These are independent F2FS observation families. In particular,
		// dataread/datawrite carry start/end phases but are not members of the
		// six-endpoint closed registry and must not be hidden by this gate.
		{name: "f2fs_dataread_start"},
		{name: "f2fs_dataread_end"},
		{name: "f2fs_datawrite_start"},
		{name: "f2fs_datawrite_end"},
		{name: "f2fs_readpage"},
		{name: "f2fs_readpages"},
		{name: "f2fs_writepage"},
		{name: "f2fs_writepages"},
		{name: "f2fs_writeback_end"},
		{name: "f2fs_submit_write_bio"},
		{name: "f2fs_sync_file"},
		{name: "f2fs_direct_io"},
		{name: "f2fs_write"},
	}

	for _, tc := range tests {
		t.Run(strings.TrimSpace(tc.name), func(t *testing.T) {
			direct := directF2FSNameCandidate(tc.name)
			consumer := tracequery.F2FSClosedEndpointNameCandidate(tc.name)
			if direct != consumer || direct != tc.candidate {
				t.Fatalf("direct/consumer F2FS candidate drift for %q: direct=%t consumer=%t want=%t", tc.name, direct, consumer, tc.candidate)
			}
			if governed := directF2FSNameGoverned(tc.name); governed != tc.exact {
				t.Fatalf("exact F2FS authority drift for %q: governed=%t want=%t", tc.name, governed, tc.exact)
			}
			if !tc.candidate || tc.exact {
				return
			}

			fixture := directF2FSTestFixtureFor(directF2FSProfileDirectIOEnter510, 8)
			fixture.format.Name = tc.name
			_, admission, reason := decodeDirectF2FSPayload(decodeEvent(fixture.format, fixture.content))
			if admission != bodyUnsupported || reason != "" {
				t.Fatalf("compact near-name decoder verdict: admission=%d reason=%q", admission, reason)
			}
			body, wrappedAdmission, wrappedReason := renderEventBodyDecision(coreDecodeContext{}, decodeEvent(fixture.format, fixture.content), fixture.content, 0)
			if wrappedAdmission != bodyUnsupported || wrappedReason != "" || body != "" {
				t.Fatalf("compact near-name reached legacy body authority: admission=%d reason=%q body=%q", wrappedAdmission, wrappedReason, body)
			}
		})
	}

	// The gate is a family/phase cross-product, not a list of the six exact
	// spellings. Pin every compact combination with suffix drift so a later
	// refactor cannot reopen one separator-loss lane while preserving the
	// handful of canonical examples above.
	for _, family := range []string{"syncfile", "directio", "write"} {
		for _, phase := range []string{"enter", "exit", "start", "done", "begin", "end", "complete"} {
			name := "f2fs_" + family + phase + "_vendor"
			t.Run("compact-matrix/"+family+"/"+phase, func(t *testing.T) {
				if !directF2FSNameCandidate(name) || !tracequery.F2FSClosedEndpointNameCandidate(name) {
					t.Fatalf("compact F2FS family/phase escaped negative gate: %q", name)
				}
				fixture := directF2FSTestFixtureFor(directF2FSProfileDirectIOEnter510, 8)
				fixture.format.Name = name
				body, admission, reason := renderEventBodyDecision(coreDecodeContext{}, decodeEvent(fixture.format, fixture.content), fixture.content, 0)
				if admission != bodyUnsupported || reason != "" || body != "" {
					t.Fatalf("compact F2FS family/phase reached legacy body authority: admission=%d reason=%q body=%q", admission, reason, body)
				}
			})
		}
	}
}

func TestDirectF2FSClosedEndpointCandidateHasOneProductionAuthority(t *testing.T) {
	body := conversionProductionFunction(t, "direct_f2fs_payload.go", "directF2FSNameCandidate")
	want := "return tracequery.F2FSClosedEndpointNameCandidate(name)"
	if strings.Count(body, want) != 1 {
		t.Fatalf("direct F2FS candidate must be a single tracequery authority handoff:\n%s", body)
	}
	for _, forbidden := range []string{`"`, "strings.", "switch ", "for ", "HasPrefix", "Contains"} {
		if strings.Contains(body, forbidden) {
			t.Fatalf("direct F2FS candidate regained a local spelling authority %q:\n%s", forbidden, body)
		}
	}
}

func TestDirectF2FSExactNamesStayBeforeLegacy(t *testing.T) {
	for _, name := range []string{"f2fs_sync_file_enter", "f2fs_sync_file_exit", "f2fs_direct_IO_enter", "f2fs_direct_IO_exit", "f2fs_write_begin", "f2fs_write_end"} {
		if !directF2FSNameGoverned(name) {
			t.Fatalf("exact F2FS name is not governed: %q", name)
		}
	}
	for _, name := range []string{
		"f2fs_direct_io_enter", "F2FS_direct_IO_enter", "f2fs_direct_IO_enter ", "f2fs_direct_IO_enter_vendor",
		"f2fs_directIO_enter", "f2fs_syncfile_exit", "f2fs_write_enter", "f2fs_write_exit",
		"f2fs_directIOenter", "f2fs_syncfileenter", "f2fs_writebegin", "f2fs_writeend_vendor",
	} {
		fixture := directF2FSTestFixtureFor(directF2FSProfileDirectIOEnter510, 8)
		fixture.format.Name = name
		if directF2FSNameGoverned(name) {
			t.Fatalf("near-name entered exact F2FS registry: %q", name)
		}
		_, admission, reason := decodeDirectF2FSPayload(decodeEvent(fixture.format, fixture.content))
		if admission != bodyUnsupported || reason != "" {
			t.Fatalf("near-name decoder verdict: admission=%d reason=%q", admission, reason)
		}
		body, wrappedAdmission, wrappedReason := renderEventBodyDecision(coreDecodeContext{}, decodeEvent(fixture.format, fixture.content), fixture.content, 0)
		if wrappedAdmission != bodyUnsupported || wrappedReason != "" || body != "" {
			t.Fatalf("near-name reached publishing renderer: admission=%d reason=%q body=%q", wrappedAdmission, wrappedReason, body)
		}
	}
}

func directF2FSTestFixtureFor(profile directF2FSProfile, width int) directF2FSTestFixture {
	name := directF2FSNameForProfile(profile)
	specs := directF2FSSpecs(profile, width)
	tail := 8
	for _, item := range specs {
		if end := item.Offset + item.Size; end > tail {
			tail = end
		}
	}
	content := make([]byte, tail)
	fields := []eventField{
		{Type: "unsigned short", Name: "common_type", Offset: 0, Size: 2},
		{Type: "unsigned char", Name: "common_flags", Offset: 2, Size: 1},
		{Type: "unsigned char", Name: "common_preempt_count", Offset: 3, Size: 1},
		{Type: "int", Name: "common_pid", Offset: 4, Size: 4, Signed: true},
	}
	binary.LittleEndian.PutUint16(content[0:2], 7)
	binary.LittleEndian.PutUint32(content[4:8], 100)
	for _, item := range specs {
		field := eventField{Type: item.Type, Name: item.Name, Offset: item.Offset, Size: item.Size, Signed: item.Signed}
		fields = append(fields, field)
		value := uint64(0)
		switch item.Name {
		case "dev":
			value = uint64(syntheticDev(8, 0))
		case "ino":
			value = 9
		case "pino":
			value = 1
		case "mode":
			value = 0x81a4
		case "size", "len", "copied":
			value = 4096
		case "nlink":
			value = 1
		case "blocks":
			value = 8
		case "datasync":
			value = 1
		case "ki_flags":
			value = 0x12
		case "ki_ioprio":
			value = 0x34
		}
		directF2FSPutUint(&field, content, value)
	}
	return directF2FSTestFixture{profile: profile, width: width, format: eventFormat{ID: 7, Name: name, Fields: fields, PrintFmt: directF2FSPrintFmtLiteral(profile)}, content: content}
}

func directF2FSNameForProfile(profile directF2FSProfile) string {
	switch profile {
	case directF2FSProfileSyncEnter:
		return "f2fs_sync_file_enter"
	case directF2FSProfileSyncExit:
		return "f2fs_sync_file_exit"
	case directF2FSProfileDirectIOEnter510, directF2FSProfileDirectIOEnter66:
		return "f2fs_direct_IO_enter"
	case directF2FSProfileDirectIOExit:
		return "f2fs_direct_IO_exit"
	case directF2FSProfileWriteBegin510, directF2FSProfileWriteBegin66:
		return "f2fs_write_begin"
	case directF2FSProfileWriteEnd:
		return "f2fs_write_end"
	default:
		panic("unknown direct F2FS profile")
	}
}

func directF2FSExpectedBody(profile directF2FSProfile) string {
	switch profile {
	case directF2FSProfileSyncEnter:
		return "dev=8:0 ino=0x9 pino=0x1 i_mode=0x81a4 i_size=4096 i_nlink=1 i_blocks=8 i_advise=0x0"
	case directF2FSProfileSyncExit:
		return "dev=8:0 ino=0x9 cp_reason=0 datasync=1 ret=0"
	case directF2FSProfileDirectIOEnter510:
		return "dev=8:0 ino=0x9 pos=0 len=4096 rw=read"
	case directF2FSProfileDirectIOEnter66:
		return "dev=8:0 ino=0x9 pos=0 len=4096 ki_flags=0x12 ki_ioprio=0x34 rw=read"
	case directF2FSProfileDirectIOExit:
		return "dev=8:0 ino=0x9 pos=0 len=4096 rw=read ret=0"
	case directF2FSProfileWriteBegin510:
		return "dev=8:0 ino=0x9 pos=0 len=4096 flags=0"
	case directF2FSProfileWriteBegin66:
		return "dev=8:0 ino=0x9 pos=0 len=4096"
	case directF2FSProfileWriteEnd:
		return "dev=8:0 ino=0x9 pos=0 len=4096 copied=4096"
	default:
		panic("unknown direct F2FS profile")
	}
}

func directF2FSPutUint(field *eventField, content []byte, value uint64) {
	raw := content[field.Offset : field.Offset+field.Size]
	switch field.Size {
	case 1:
		raw[0] = byte(value)
	case 2:
		binary.LittleEndian.PutUint16(raw, uint16(value))
	case 4:
		binary.LittleEndian.PutUint32(raw, uint32(value))
	case 8:
		binary.LittleEndian.PutUint64(raw, value)
	default:
		panic("unsupported direct F2FS scalar width")
	}
}

func directF2FSCloneFixture(in directF2FSTestFixture) directF2FSTestFixture {
	out := in
	out.content = append([]byte(nil), in.content...)
	out.format.Fields = append([]eventField(nil), in.format.Fields...)
	return out
}

func directF2FSFixtureField(t *testing.T, fixture *directF2FSTestFixture, name string) *eventField {
	t.Helper()
	for index := range fixture.format.Fields {
		if cleanFieldName(fixture.format.Fields[index].Name) == name {
			return &fixture.format.Fields[index]
		}
	}
	t.Fatalf("missing fixture field %q", name)
	return nil
}

func directF2FSAssertRejected(t *testing.T, fixture directF2FSTestFixture) {
	t.Helper()
	payload, admission, reason := decodeDirectF2FSPayload(decodeEvent(fixture.format, fixture.content))
	if admission != bodyRejected || reason == "" {
		t.Fatalf("malformed direct F2FS escaped: admission=%d reason=%q payload=%+v", admission, reason, payload)
	}
	body, wrappedAdmission, wrappedReason := renderEventBodyDecision(coreDecodeContext{}, decodeEvent(fixture.format, fixture.content), fixture.content, 0)
	if wrappedAdmission != bodyRejected || wrappedReason == "" || body != "" {
		t.Fatalf("malformed direct F2FS reached renderer: admission=%d reason=%q body=%q", wrappedAdmission, wrappedReason, body)
	}
}

func directF2FSAssertAdmittedBody(t *testing.T, fixture directF2FSTestFixture) string {
	t.Helper()
	payload, admission, reason := decodeDirectF2FSPayload(decodeEvent(fixture.format, fixture.content))
	if admission != bodyAdmitted || reason != "" {
		t.Fatalf("valid boundary fixture rejected: admission=%d reason=%q payload=%+v", admission, reason, payload)
	}
	body, ok := renderCanonicalF2FSPayload(payload)
	if !ok {
		t.Fatalf("admitted boundary payload did not render: %+v", payload)
	}
	return body
}
