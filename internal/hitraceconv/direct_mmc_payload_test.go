package hitraceconv

import (
	"encoding/binary"
	"fmt"
	"strings"
	"testing"
)

type directMMCTestFixture struct {
	format  eventFormat
	content []byte
}

func TestDirectMMCCanonicalABIAndResponseWordMatrix(t *testing.T) {
	for _, name := range []string{"mmc_request_start", "mmc_request_done"} {
		for _, width := range []int{4, 8} {
			name, width := name, width
			t.Run(fmt.Sprintf("%s/word%d", name, width), func(t *testing.T) {
				fixture := directMMCTestFixtureFor(name, width)
				payload, admission, reason := decodeDirectMMCPayload(decodeEvent(fixture.format, fixture.content), fixture.content)
				if admission != bodyAdmitted || reason != "" || !payload.HeaderOwnerKnown || payload.HeaderTID != 100 {
					t.Fatalf("valid direct MMC rejected: admission=%d reason=%q payload=%+v", admission, reason, payload)
				}
				body, ok := renderCanonicalMMCPayload(payload)
				if !ok {
					t.Fatal("valid direct MMC payload did not render")
				}
				pointer := "0xfedcba98"
				if width == 8 {
					pointer = "0xfedcba9876543210"
				}
				for _, fragment := range []string{"mmc0:", "struct mmc_request[" + pointer + "]", " tag=-1 "} {
					if !strings.Contains(body, fragment) {
						t.Fatalf("canonical MMC body lost %q: %q", fragment, body)
					}
				}
				if name == "mmc_request_done" {
					for _, fragment := range []string{
						"cmd_err=-5", "cmd_resp=0x1020304 0x11223344 0x89abcdef 0xfedcba98",
						"stop_err=-6", "stop_resp=0xa0b0c0d 0x10203040 0x55667788 0x99aabbcc",
						"sbc_err=-7", "sbc_resp=0xdeadbeef 0xcafebabe 0x76543210 0xf0e0d0c", "data_err=-8",
					} {
						if !strings.Contains(body, fragment) {
							t.Fatalf("u32 response/error fidelity lost %q: %q", fragment, body)
						}
					}
					if strings.Contains(body, "cmd_resp=0x4 0x3 0x2 0x1") {
						t.Fatalf("u32[4] regressed to four bytes: %q", body)
					}
				}
				line, lineAdmission, lineReason, envelopeOK := renderEventLineDecision(
					renderContext{cmdlines: map[int]string{100: "mmc-worker"}, tgids: map[int]int{100: 100}},
					1_000_000, 2, fixture.format, fixture.content,
				)
				if !envelopeOK || lineAdmission != bodyAdmitted || lineReason != "" ||
					!strings.HasSuffix(line, name+": "+body) || !traceDBSinglePhysicalLine(line, false) {
					t.Fatalf("wrapped MMC drifted: envelope=%t admission=%d reason=%q line=%q", envelopeOK, lineAdmission, lineReason, line)
				}
			})
		}
	}
}

func TestDirectMMCDescriptorMutationMatrix(t *testing.T) {
	for _, name := range []string{"mmc_request_start", "mmc_request_done"} {
		for _, width := range []int{4, 8} {
			base := directMMCTestFixtureFor(name, width)
			for fieldIndex, field := range base.format.Fields {
				fieldIndex, field := fieldIndex, field
				prefix := fmt.Sprintf("%s/word%d/%s", name, width, cleanFieldName(field.Name))
				t.Run(prefix+"/missing", func(t *testing.T) {
					fixture := directMMCCloneFixture(base)
					fixture.format.Fields = append(fixture.format.Fields[:fieldIndex], fixture.format.Fields[fieldIndex+1:]...)
					directMMCAssertRejected(t, fixture)
				})
				t.Run(prefix+"/wrong_type", func(t *testing.T) {
					fixture := directMMCCloneFixture(base)
					fixture.format.Fields[fieldIndex].Type = "unsigned long long"
					directMMCAssertRejected(t, fixture)
				})
				t.Run(prefix+"/type_case_drift", func(t *testing.T) {
					fixture := directMMCCloneFixture(base)
					fixture.format.Fields[fieldIndex].Type = strings.ToUpper(fixture.format.Fields[fieldIndex].Type)
					directMMCAssertRejected(t, fixture)
				})
				t.Run(prefix+"/wrong_sign", func(t *testing.T) {
					fixture := directMMCCloneFixture(base)
					fixture.format.Fields[fieldIndex].Signed = !fixture.format.Fields[fieldIndex].Signed
					directMMCAssertRejected(t, fixture)
				})
				t.Run(prefix+"/wrong_offset", func(t *testing.T) {
					fixture := directMMCCloneFixture(base)
					fixture.format.Fields[fieldIndex].Offset++
					directMMCAssertRejected(t, fixture)
				})
				t.Run(prefix+"/wrong_size", func(t *testing.T) {
					fixture := directMMCCloneFixture(base)
					fixture.format.Fields[fieldIndex].Size++
					directMMCAssertRejected(t, fixture)
				})
				t.Run(prefix+"/truncated_content", func(t *testing.T) {
					fixture := directMMCCloneFixture(base)
					cut := field.Offset + field.Size - 1
					if cut < 0 {
						cut = 0
					}
					fixture.content = fixture.content[:cut]
					directMMCAssertRejected(t, fixture)
				})
				t.Run(prefix+"/alias", func(t *testing.T) {
					fixture := directMMCCloneFixture(base)
					fixture.format.Fields[fieldIndex].Name = cleanFieldName(field.Name) + "_alias"
					directMMCAssertRejected(t, fixture)
				})
				t.Run(prefix+"/duplicate", func(t *testing.T) {
					fixture := directMMCCloneFixture(base)
					fixture.format.Fields = append(fixture.format.Fields, fixture.format.Fields[fieldIndex])
					directMMCAssertRejected(t, fixture)
				})
			}
		}
	}

	for _, name := range []string{"mmc_request_start", "mmc_request_done"} {
		fixture := directMMCTestFixtureFor(name, 8)
		fixture.format.Fields = append(fixture.format.Fields, eventField{
			Type: "unsigned int", Name: "vendor", Offset: len(fixture.content), Size: 4,
		})
		fixture.content = append(fixture.content, make([]byte, 4)...)
		directMMCAssertRejected(t, fixture)
	}
}

func TestDirectMMCZeroAndSignedScalarBoundariesRemainValues(t *testing.T) {
	for _, name := range []string{"mmc_request_start", "mmc_request_done"} {
		fixture := directMMCTestFixtureFor(name, 8)
		for _, field := range fixture.format.Fields {
			clean := cleanFieldName(field.Name)
			if strings.HasPrefix(clean, "common_") || clean == "mrq" || clean == "name" || field.Size == 16 {
				continue
			}
			for index := 0; index < field.Size; index++ {
				fixture.content[field.Offset+index] = 0
			}
		}
		payload, admission, reason := decodeDirectMMCPayload(decodeEvent(fixture.format, fixture.content), fixture.content)
		if admission != bodyAdmitted || reason != "" {
			t.Fatalf("real zero scalar became missing for %s: admission=%d reason=%q", name, admission, reason)
		}
		if _, ok := renderCanonicalMMCPayload(payload); !ok {
			t.Fatalf("real zero scalar payload did not render for %s", name)
		}
	}

	fixture := directMMCTestFixtureFor("mmc_request_done", 8)
	tag := directMMCFixtureField(t, &fixture, "tag")
	hold := directMMCFixtureField(t, &fixture, "hold_retune")
	binary.LittleEndian.PutUint32(fixture.content[tag.Offset:tag.Offset+4], 0x80000000)
	binary.LittleEndian.PutUint32(fixture.content[hold.Offset:hold.Offset+4], 0x7fffffff)
	payload, admission, reason := decodeDirectMMCPayload(decodeEvent(fixture.format, fixture.content), fixture.content)
	if admission != bodyAdmitted || reason != "" || payload.Done == nil ||
		payload.Done.Tag != -2147483648 || payload.Done.HoldRetune != 2147483647 {
		t.Fatalf("signed int32 boundaries drifted: admission=%d reason=%q payload=%+v", admission, reason, payload)
	}
}

func TestDirectMMCNameLocatorAndPointerFailClosed(t *testing.T) {
	for _, width := range []int{4, 8} {
		base := directMMCTestFixtureFor("mmc_request_start", width)
		tail, ok := directDescriptorFixedTail(decodeEvent(base.format, base.content))
		if !ok {
			t.Fatal("valid fixture has no fixed tail")
		}
		tests := []struct {
			name   string
			raw    []byte
			offset int
			length int
		}{
			{name: "before fixed tail", raw: []byte{'m', 'm', 'c', '0', 0}, offset: tail - 1},
			{name: "empty", raw: []byte{0}, offset: tail},
			{name: "no nul", raw: []byte("mmc0"), offset: tail},
			{name: "interior nul", raw: []byte{'m', 0, 'x', 0}, offset: tail},
			{name: "space", raw: []byte("mmc 0\x00"), offset: tail},
			{name: "colon", raw: []byte("mmc0:\x00"), offset: tail},
			{name: "comma", raw: []byte("mmc0,\x00"), offset: tail},
			{name: "quote", raw: []byte("\"mmc0\"\x00"), offset: tail},
			{name: "newline", raw: []byte("mmc0\nnext\x00"), offset: tail},
			{name: "invalid utf8", raw: []byte{0xff, 0}, offset: tail},
			{name: "over limit", raw: append([]byte(strings.Repeat("x", 257)), 0), offset: tail},
			{name: "zero locator length", raw: []byte("mmc0\x00"), offset: tail, length: -1},
			{name: "range overflow", raw: []byte("mmc0\x00"), offset: tail + 100, length: 20},
		}
		for _, test := range tests {
			t.Run(fmt.Sprintf("word%d/%s", width, test.name), func(t *testing.T) {
				fixture := directMMCCloneFixture(base)
				directMMCSetName(&fixture, test.raw, test.offset, test.length)
				directMMCAssertRejected(t, fixture)
			})
		}

		fixture := directMMCCloneFixture(base)
		mrq := directMMCFixtureField(t, &fixture, "mrq")
		for i := 0; i < mrq.Size; i++ {
			fixture.content[mrq.Offset+i] = 0
		}
		directMMCAssertRejected(t, fixture)
	}
}

func TestDirectMMCExactNamesAndStructuredResponseBoundary(t *testing.T) {
	for _, name := range []string{"mmc_request_start", "mmc_request_done"} {
		if !directMMCNameGoverned(name) {
			t.Fatalf("exact MMC name is not governed: %q", name)
		}
	}
	for _, name := range []string{
		"MMC_request_start", "mmc_request_start ", "mmc_request_start_vendor",
		"mmc_request_done_start", "vendor_mmc_request_done",
	} {
		fixture := directMMCTestFixtureFor("mmc_request_start", 8)
		fixture.format.Name = name
		if directMMCNameGoverned(name) {
			t.Fatalf("near-name entered exact MMC registry: %q", name)
		}
		payload, admission, reason := decodeDirectMMCPayload(decodeEvent(fixture.format, fixture.content), fixture.content)
		if admission != bodyUnsupported || reason != "" {
			t.Fatalf("near-name decoder verdict: admission=%d reason=%q payload=%+v", admission, reason, payload)
		}
		body, wrappedAdmission, wrappedReason := renderEventBodyDecision(
			coreDecodeContext{}, decodeEvent(fixture.format, fixture.content), fixture.content, 0,
		)
		if wrappedAdmission != bodyUnsupported || wrappedReason != "" || body != "" {
			t.Fatalf("near-name reached a publishing renderer: admission=%d reason=%q body=%q", wrappedAdmission, wrappedReason, body)
		}
	}

	item := &mmcDonePayload{Name: "mmc0", MRQ: 1, CmdOpcode: 17, CmdErr: -5, DataErr: -8}
	structured, ok := renderCanonicalMMCPayload(mmcPayload{Kind: mmcPayloadDone, Name: "mmc_request_done", Done: item})
	if !ok || strings.Contains(structured, "_resp=") {
		t.Fatalf("structured response non-authority regressed: ok=%t body=%q", ok, structured)
	}
	item.CmdResponseKnown = true
	if body, ok := renderCanonicalMMCPayload(mmcPayload{Kind: mmcPayloadDone, Done: item}); ok || body != "" {
		t.Fatalf("partial response authority rendered: ok=%t body=%q", ok, body)
	}
}

func TestDirectMMCSingleAuthorityBeforeLegacy(t *testing.T) {
	render := mustReadRendererSource(t, "render.go")
	official := mustReadRendererSource(t, "official_render.go")
	decision := sourceBetween(t, render, "func renderEventBodyDecisionWithPair(", "func renderLegacyEventBody(")
	decoderAt := strings.Index(decision, "decodeDirectMMCPayload(ev, content)")
	legacyAt := strings.Index(decision, "renderLegacyEventBody(ev, content, cpu)")
	if decoderAt < 0 || legacyAt < 0 || decoderAt >= legacyAt || !strings.Contains(render, "directMMCNameGoverned(format.Name)") {
		t.Fatalf("direct MMC authority/order drifted: decoder=%d legacy=%d", decoderAt, legacyAt)
	}
	for _, forbidden := range []string{
		`strings.HasPrefix(name, "mmc_request_start")`, `strings.HasPrefix(name, "mmc_request_done")`,
		"func renderMMCRequestStart(", "func renderMMCRequestDone(", "byteAt(cmdResp",
	} {
		if strings.Contains(official, forbidden) {
			t.Fatalf("legacy MMC authority survived: %q", forbidden)
		}
	}
}

func directMMCTestFixtureFor(name string, width int) directMMCTestFixture {
	kind := mmcPayloadStart
	if name == "mmc_request_done" {
		kind = mmcPayloadDone
	}
	specs := directMMCSpecs(kind, width)
	tail := 0
	for _, item := range specs {
		if end := item.Offset + item.Size; end > tail {
			tail = end
		}
	}
	host := []byte("mmc0\x00")
	content := make([]byte, tail+len(host))
	fields := []eventField{
		{Type: "unsigned short", Name: "common_type", Offset: 0, Size: 2},
		{Type: "unsigned char", Name: "common_flags", Offset: 2, Size: 1},
		{Type: "unsigned char", Name: "common_preempt_count", Offset: 3, Size: 1},
		{Type: "int", Name: "common_pid", Offset: 4, Size: 4, Signed: true},
	}
	binary.LittleEndian.PutUint16(content[0:2], 7)
	binary.LittleEndian.PutUint32(content[4:8], 100)
	for _, item := range specs {
		fields = append(fields, eventField{Type: item.Type, Name: item.DeclaredName, Offset: item.Offset, Size: item.Size, Signed: item.Signed})
		raw := content[item.Offset : item.Offset+item.Size]
		switch item.Name {
		case "name":
			binary.LittleEndian.PutUint32(raw, uint32(len(host))<<16|uint32(tail))
			copy(content[tail:], host)
		case "mrq":
			if width == 4 {
				binary.LittleEndian.PutUint32(raw, 0xfedcba98)
			} else {
				binary.LittleEndian.PutUint64(raw, 0xfedcba9876543210)
			}
		case "tag":
			binary.LittleEndian.PutUint32(raw, 0xffffffff)
		case "cmd_err":
			binary.LittleEndian.PutUint32(raw, 0xfffffffb)
		case "stop_err":
			binary.LittleEndian.PutUint32(raw, 0xfffffffa)
		case "sbc_err":
			binary.LittleEndian.PutUint32(raw, 0xfffffff9)
		case "data_err":
			binary.LittleEndian.PutUint32(raw, 0xfffffff8)
		case "need_retune":
			binary.LittleEndian.PutUint32(raw, 0xfffffffe)
		case "hold_retune":
			binary.LittleEndian.PutUint32(raw, 3)
		case "cmd_resp":
			directMMCPutResponse(raw, [4]uint32{0x01020304, 0x11223344, 0x89abcdef, 0xfedcba98})
		case "stop_resp":
			directMMCPutResponse(raw, [4]uint32{0x0a0b0c0d, 0x10203040, 0x55667788, 0x99aabbcc})
		case "sbc_resp":
			directMMCPutResponse(raw, [4]uint32{0xdeadbeef, 0xcafebabe, 0x76543210, 0x0f0e0d0c})
		case "cmd_opcode":
			binary.LittleEndian.PutUint32(raw, 17)
		case "bytes_xfered":
			binary.LittleEndian.PutUint32(raw, 4096)
		default:
			binary.LittleEndian.PutUint32(raw, uint32(item.Offset+1))
		}
	}
	return directMMCTestFixture{format: eventFormat{ID: 7, Name: name, Fields: fields}, content: content}
}

func directMMCPutResponse(raw []byte, words [4]uint32) {
	for index, word := range words {
		binary.LittleEndian.PutUint32(raw[index*4:index*4+4], word)
	}
}

func directMMCCloneFixture(in directMMCTestFixture) directMMCTestFixture {
	out := directMMCTestFixture{format: in.format, content: append([]byte(nil), in.content...)}
	out.format.Fields = append([]eventField(nil), in.format.Fields...)
	return out
}

func directMMCFixtureField(t *testing.T, fixture *directMMCTestFixture, name string) *eventField {
	t.Helper()
	for index := range fixture.format.Fields {
		if cleanFieldName(fixture.format.Fields[index].Name) == name {
			return &fixture.format.Fields[index]
		}
	}
	t.Fatalf("missing fixture field %q", name)
	return nil
}

func directMMCSetName(fixture *directMMCTestFixture, raw []byte, offset, lengthOverride int) {
	var field *eventField
	for index := range fixture.format.Fields {
		if cleanFieldName(fixture.format.Fields[index].Name) == "name" {
			field = &fixture.format.Fields[index]
			break
		}
	}
	if field == nil {
		panic("direct MMC fixture has no name field")
	}
	need := offset + len(raw)
	if need > len(fixture.content) && offset < 1<<16 {
		fixture.content = append(fixture.content, make([]byte, need-len(fixture.content))...)
	}
	if offset >= 0 && offset <= len(fixture.content) && len(raw) <= len(fixture.content)-offset {
		copy(fixture.content[offset:], raw)
	}
	length := len(raw)
	if lengthOverride < 0 {
		length = 0
	} else if lengthOverride > 0 {
		length = lengthOverride
	}
	binary.LittleEndian.PutUint32(fixture.content[field.Offset:field.Offset+4], uint32(length)<<16|uint32(offset&0xffff))
}

func directMMCAssertRejected(t *testing.T, fixture directMMCTestFixture) {
	t.Helper()
	payload, admission, reason := decodeDirectMMCPayload(decodeEvent(fixture.format, fixture.content), fixture.content)
	if admission != bodyRejected || reason == "" {
		t.Fatalf("malformed direct MMC escaped: admission=%d reason=%q payload=%+v", admission, reason, payload)
	}
	body, wrappedAdmission, wrappedReason := renderEventBodyDecision(
		coreDecodeContext{}, decodeEvent(fixture.format, fixture.content), fixture.content, 0,
	)
	if wrappedAdmission != bodyRejected || wrappedReason == "" || body != "" {
		t.Fatalf("malformed direct MMC reached renderer: admission=%d reason=%q body=%q", wrappedAdmission, wrappedReason, body)
	}
}
