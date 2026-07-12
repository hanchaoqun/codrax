package hitraceconv

import (
	"encoding/binary"
	"go/ast"
	"go/parser"
	"go/token"
	"math"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/tracequery"
)

type directPairTestFixture struct {
	format  eventFormat
	content []byte
}

func TestDirectPairCanonicalProfileMatrix(t *testing.T) {
	tests := []struct {
		name    string
		fixture directPairTestFixture
		want    string
	}{
		{
			name:    "workqueue current start 32 bit keeps high pointer bits",
			fixture: directPairWorkqueueFixture("workqueue_execute_start", 4, true, 0xfedcba98, 0x87654321),
			want:    "work struct 0xfedcba98: function 0x87654321",
		},
		{
			name:    "workqueue current end 32 bit",
			fixture: directPairWorkqueueFixture("workqueue_execute_end", 4, true, 0xabcdef01, 0x12345678),
			want:    "work struct 0xabcdef01: function 0x12345678",
		},
		{
			name:    "workqueue current start 64 bit keeps high pointer bits",
			fixture: directPairWorkqueueFixture("workqueue_execute_start", 8, true, 0xfedcba9876543210, 0x876543210fedcba9),
			want:    "work struct 0xfedcba9876543210: function 0x876543210fedcba9",
		},
		{
			name:    "workqueue current end 64 bit",
			fixture: directPairWorkqueueFixture("workqueue_execute_end", 8, true, 0xabcdef0123456789, 0x123456789abcdef0),
			want:    "work struct 0xabcdef0123456789: function 0x123456789abcdef0",
		},
		{
			name:    "workqueue legacy end 32 bit has no invented function",
			fixture: directPairWorkqueueFixture("workqueue_execute_end", 4, false, 0xabcdef01, 0),
			want:    "work struct 0xabcdef01",
		},
		{
			name:    "workqueue legacy end 64 bit has no invented function",
			fixture: directPairWorkqueueFixture("workqueue_execute_end", 8, false, 0xabcdef0123456789, 0),
			want:    "work struct 0xabcdef0123456789",
		},
		{
			name:    "DMA unsigned locators preserve zero and max uint32",
			fixture: directPairDMAFixture("dma_fence_wait_start", []byte("display"), []byte("present"), 0, math.MaxUint32, false),
			want:    "driver=display timeline=present context=0 seqno=4294967295",
		},
		{
			name:    "DMA signed char locators are the same physical u32 locator profile",
			fixture: directPairDMAFixture("dma_fence_wait_end", []byte("gpu"), []byte("render"), math.MaxUint32, 0, true),
			want:    "driver=gpu timeline=render context=4294967295 seqno=0",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			body := directPairAdmittedBody(t, test.fixture)
			if body != test.want {
				t.Fatalf("canonical pair body=%q want=%q", body, test.want)
			}
			line, admission, reason, envelopeOK := renderEventLineDecision(
				renderContext{cmdlines: map[int]string{100: "pair-worker"}, tgids: map[int]int{100: 100}},
				1_000_000, 2, test.fixture.format, test.fixture.content,
			)
			if !envelopeOK || admission != bodyAdmitted || reason != "" ||
				!strings.HasSuffix(line, test.fixture.format.Name+": "+test.want) {
				t.Fatalf("wrapped pair drifted: envelope=%v admission=%d reason=%q line=%q", envelopeOK, admission, reason, line)
			}
			if len(line) > maxTraceDBSystraceLineBytes || !traceDBSinglePhysicalLine(line, false) {
				t.Fatalf("admitted pair escaped the shared physical-line budget: bytes=%d", len(line))
			}
		})
	}
}

func TestDirectPairWorkqueueMutationMatrix(t *testing.T) {
	assertRejected := func(t *testing.T, fixture directPairTestFixture) {
		t.Helper()
		directPairAssertRejected(t, fixture)
	}

	for _, eventName := range []string{"workqueue_execute_start", "workqueue_execute_end"} {
		for _, width := range []int{4, 8} {
			for _, fieldName := range []string{"work", "function"} {
				prefix := eventName + "/word" + string(rune('0'+width)) + "/" + fieldName
				t.Run(prefix+"/missing", func(t *testing.T) {
					fixture := directPairWorkqueueFixture(eventName, width, true, 0x12345678, 0x23456789)
					directPairRemoveField(&fixture, fieldName)
					assertRejected(t, fixture)
				})
				t.Run(prefix+"/wrong sign", func(t *testing.T) {
					fixture := directPairWorkqueueFixture(eventName, width, true, 0x12345678, 0x23456789)
					directPairField(t, &fixture, fieldName).Signed = true
					assertRejected(t, fixture)
				})
				t.Run(prefix+"/wrong type", func(t *testing.T) {
					fixture := directPairWorkqueueFixture(eventName, width, true, 0x12345678, 0x23456789)
					directPairField(t, &fixture, fieldName).Type = "unsigned long"
					assertRejected(t, fixture)
				})
				t.Run(prefix+"/wrong width", func(t *testing.T) {
					fixture := directPairWorkqueueFixture(eventName, width, true, 0x12345678, 0x23456789)
					field := directPairField(t, &fixture, fieldName)
					field.Size = 2
					assertRejected(t, fixture)
				})
				t.Run(prefix+"/truncated", func(t *testing.T) {
					fixture := directPairWorkqueueFixture(eventName, width, true, 0x12345678, 0x23456789)
					field := directPairField(t, &fixture, fieldName)
					fixture.content = fixture.content[:field.Offset+field.Size-1]
					assertRejected(t, fixture)
				})
				for _, conflict := range []bool{false, true} {
					label := "same duplicate"
					if conflict {
						label = "conflicting duplicate"
					}
					t.Run(prefix+"/"+label, func(t *testing.T) {
						fixture := directPairWorkqueueFixture(eventName, width, true, 0x12345678, 0x23456789)
						directPairDuplicateFixedField(t, &fixture, fieldName, conflict)
						assertRejected(t, fixture)
					})
				}
			}
		}
	}

	t.Run("legacy profile is end only", func(t *testing.T) {
		assertRejected(t, directPairWorkqueueFixture("workqueue_execute_start", 8, false, 0x12345678, 0))
	})

	for _, width := range []int{4, 8} {
		t.Run("legacy end exact work contract/word"+string(rune('0'+width)), func(t *testing.T) {
			base := func() directPairTestFixture {
				return directPairWorkqueueFixture("workqueue_execute_end", width, false, 0x12345678, 0)
			}
			missing := base()
			directPairRemoveField(&missing, "work")
			assertRejected(t, missing)

			signed := base()
			directPairField(t, &signed, "work").Signed = true
			assertRejected(t, signed)

			wrongType := base()
			directPairField(t, &wrongType, "work").Type = "unsigned long"
			assertRejected(t, wrongType)

			wrongWidth := base()
			directPairField(t, &wrongWidth, "work").Size = 2
			assertRejected(t, wrongWidth)

			truncated := base()
			truncated.content = truncated.content[:len(truncated.content)-1]
			assertRejected(t, truncated)

			for _, conflict := range []bool{false, true} {
				duplicate := base()
				directPairDuplicateFixedField(t, &duplicate, "work", conflict)
				assertRejected(t, duplicate)
			}

			for name, printFmt := range map[string]string{
				"missing print format": "",
				"garbage print format": `"work struct %p"`,
				"current print shape":  `"work struct %p: function %ps", REC->work, REC->function`,
			} {
				t.Run(name, func(t *testing.T) {
					fixture := base()
					fixture.format.PrintFmt = printFmt
					assertRejected(t, fixture)
				})
			}

			t.Run("extra payload field is not legacy", func(t *testing.T) {
				fixture := base()
				fixture.format.Fields = append(fixture.format.Fields,
					eventField{Type: "unsigned int", Name: "vendor_cookie", Offset: 8 + width, Size: 4})
				fixture.content = append(fixture.content, make([]byte, 4)...)
				assertRejected(t, fixture)
			})
		})
	}

	t.Run("current function cannot fail down to legacy end", func(t *testing.T) {
		for name, mutate := range map[string]func(*eventField){
			"signed":     func(field *eventField) { field.Signed = true },
			"wrong type": func(field *eventField) { field.Type = "unsigned long" },
			"wrong width": func(field *eventField) {
				field.Size = 4
			},
		} {
			t.Run(name, func(t *testing.T) {
				fixture := directPairWorkqueueFixture("workqueue_execute_end", 8, true, 0x12345678, 0x23456789)
				mutate(directPairField(t, &fixture, "function"))
				assertRejected(t, fixture)
			})
		}
		fixture := directPairWorkqueueFixture("workqueue_execute_end", 8, true, 0x12345678, 0x23456789)
		fixture.content = fixture.content[:len(fixture.content)-1]
		assertRejected(t, fixture)
	})

	t.Run("pointer widths cannot mix", func(t *testing.T) {
		fixture := directPairWorkqueueFixture("workqueue_execute_start", 8, true, 0x12345678, 0x23456789)
		directPairField(t, &fixture, "function").Size = 4
		assertRejected(t, fixture)
	})

	t.Run("pointer fields cannot overlap", func(t *testing.T) {
		fixture := directPairWorkqueueFixture("workqueue_execute_start", 8, true, 0x12345678, 0x23456789)
		directPairField(t, &fixture, "function").Offset = directPairField(t, &fixture, "work").Offset
		assertRejected(t, fixture)
	})

	t.Run("unrelated descriptor cannot reuse pointer bytes", func(t *testing.T) {
		fixture := directPairWorkqueueFixture("workqueue_execute_start", 8, true, 0x12345678, 0x23456789)
		fixture.format.Fields = append(fixture.format.Fields,
			eventField{Type: "unsigned int", Name: "vendor_alias", Offset: 8, Size: 4})
		assertRejected(t, fixture)
	})

	t.Run("unrelated payload field cannot expand current profile", func(t *testing.T) {
		fixture := directPairWorkqueueFixture("workqueue_execute_start", 8, true, 0x12345678, 0x23456789)
		fixture.format.Fields = append(fixture.format.Fields,
			eventField{Type: "unsigned int", Name: "vendor_cookie", Offset: len(fixture.content), Size: 4})
		fixture.content = append(fixture.content, make([]byte, 4)...)
		assertRejected(t, fixture)
	})

	t.Run("legacy clean-name aliases cannot mix with exact fields", func(t *testing.T) {
		for _, fieldName := range []string{"work", "function"} {
			aliasOnly := directPairWorkqueueFixture("workqueue_execute_start", 8, true, 0x12345678, 0x23456789)
			directPairField(t, &aliasOnly, fieldName).Name = fieldName + "[1]"
			assertRejected(t, aliasOnly)

			fixture := directPairWorkqueueFixture("workqueue_execute_start", 8, true, 0x12345678, 0x23456789)
			directPairDuplicateFixedField(t, &fixture, fieldName, false)
			fixture.format.Fields[len(fixture.format.Fields)-1].Name = fieldName + "[1]"
			assertRejected(t, fixture)
		}
	})

	t.Run("zero pointers are not identities or metadata", func(t *testing.T) {
		assertRejected(t, directPairWorkqueueFixture("workqueue_execute_start", 8, true, 0, 0x23456789))
		assertRejected(t, directPairWorkqueueFixture("workqueue_execute_start", 8, true, 0x12345678, 0))
		assertRejected(t, directPairWorkqueueFixture("workqueue_execute_end", 8, false, 0, 0))
	})
}

func TestDirectPairDMAFieldMutationMatrix(t *testing.T) {
	for _, fieldName := range []string{"driver", "timeline", "context", "seqno"} {
		t.Run(fieldName+"/missing", func(t *testing.T) {
			fixture := directPairDMAFixture("dma_fence_wait_start", []byte("gpu"), []byte("render"), 7, 9, false)
			directPairRemoveField(&fixture, fieldName)
			directPairAssertRejected(t, fixture)
		})
		t.Run(fieldName+"/wrong type", func(t *testing.T) {
			fixture := directPairDMAFixture("dma_fence_wait_start", []byte("gpu"), []byte("render"), 7, 9, false)
			field := directPairField(t, &fixture, fieldName)
			if fieldName == "driver" || fieldName == "timeline" {
				field.Type = "__rel_loc char[]"
			} else {
				field.Type = "unsigned long"
			}
			directPairAssertRejected(t, fixture)
		})
		t.Run(fieldName+"/wrong width", func(t *testing.T) {
			fixture := directPairDMAFixture("dma_fence_wait_start", []byte("gpu"), []byte("render"), 7, 9, false)
			directPairField(t, &fixture, fieldName).Size = 8
			directPairAssertRejected(t, fixture)
		})
		t.Run(fieldName+"/truncated", func(t *testing.T) {
			fixture := directPairDMAFixture("dma_fence_wait_start", []byte("gpu"), []byte("render"), 7, 9, false)
			field := directPairField(t, &fixture, fieldName)
			fixture.content = fixture.content[:field.Offset+field.Size-1]
			directPairAssertRejected(t, fixture)
		})
		if fieldName == "context" || fieldName == "seqno" {
			t.Run(fieldName+"/wrong sign", func(t *testing.T) {
				fixture := directPairDMAFixture("dma_fence_wait_start", []byte("gpu"), []byte("render"), 7, 9, false)
				directPairField(t, &fixture, fieldName).Signed = true
				directPairAssertRejected(t, fixture)
			})
		}
		for _, conflict := range []bool{false, true} {
			label := "same duplicate"
			if conflict {
				label = "conflicting duplicate"
			}
			t.Run(fieldName+"/"+label, func(t *testing.T) {
				fixture := directPairDMADuplicateFixture(fieldName, conflict)
				directPairAssertRejected(t, fixture)
			})
		}
	}

	t.Run("dynamic locator signed flag is char element metadata", func(t *testing.T) {
		fixture := directPairDMAFixture("dma_fence_wait_start", []byte("gpu"), []byte("render"), 7, 9, true)
		if got := directPairAdmittedBody(t, fixture); got != "driver=gpu timeline=render context=7 seqno=9" {
			t.Fatalf("signed dynamic-char descriptor changed locator semantics: %q", got)
		}
	})

	t.Run("fixed arrays and relative locators are not the wait endpoint profile", func(t *testing.T) {
		for _, fieldName := range []string{"driver", "timeline"} {
			fixed := directPairDMAFixture("dma_fence_wait_start", []byte("gpu"), []byte("render"), 7, 9, false)
			field := directPairField(t, &fixed, fieldName)
			field.Type = "char"
			field.Name += "[4]"
			directPairAssertRejected(t, fixed)

			relative := directPairDMAFixture("dma_fence_wait_start", []byte("gpu"), []byte("render"), 7, 9, false)
			directPairField(t, &relative, fieldName).Type = "__rel_loc char[]"
			directPairAssertRejected(t, relative)
		}
	})

	t.Run("fixed fields cannot overlap", func(t *testing.T) {
		fixture := directPairDMAFixture("dma_fence_wait_start", []byte("gpu"), []byte("render"), 7, 9, false)
		directPairField(t, &fixture, "timeline").Offset = directPairField(t, &fixture, "driver").Offset
		directPairAssertRejected(t, fixture)
	})

	t.Run("unrelated descriptor cannot reuse hard tuple bytes", func(t *testing.T) {
		fixture := directPairDMAFixture("dma_fence_wait_start", []byte("gpu"), []byte("render"), 7, 9, false)
		fixture.format.Fields = append(fixture.format.Fields,
			eventField{Type: "unsigned int", Name: "vendor_alias", Offset: 16, Size: 4})
		directPairAssertRejected(t, fixture)
	})

	t.Run("legacy clean-name aliases cannot mix with exact fields", func(t *testing.T) {
		aliases := map[string]string{
			"driver":   "__data_loc_driver",
			"timeline": "__data_loc_timeline",
			"context":  "context[1]",
			"seqno":    "seqno[1]",
		}
		for fieldName, alias := range aliases {
			t.Run(fieldName, func(t *testing.T) {
				aliasOnly := directPairDMAFixture("dma_fence_wait_start", []byte("gpu"), []byte("render"), 7, 9, false)
				directPairField(t, &aliasOnly, fieldName).Name = alias
				directPairAssertRejected(t, aliasOnly)

				fixture := directPairDMADuplicateFixture(fieldName, false)
				fixture.format.Fields[len(fixture.format.Fields)-1].Name = alias
				directPairAssertRejected(t, fixture)
			})
		}
	})
}

func TestDirectPairDMADynamicRangeAndScalarSafety(t *testing.T) {
	t.Run("dynamic range mutations", func(t *testing.T) {
		for name, mutate := range map[string]func(*directPairTestFixture){
			"offset before fixed tail": func(fixture *directPairTestFixture) {
				loc := directPairDMALocator(t, fixture, "driver")
				binary.LittleEndian.PutUint32(fixture.content[8:12], uint32((loc.length<<16)|20))
			},
			"offset outside record": func(fixture *directPairTestFixture) {
				loc := directPairDMALocator(t, fixture, "driver")
				binary.LittleEndian.PutUint32(fixture.content[8:12], uint32((loc.length<<16)|(len(fixture.content)+1)))
			},
			"zero length": func(fixture *directPairTestFixture) {
				loc := directPairDMALocator(t, fixture, "driver")
				binary.LittleEndian.PutUint32(fixture.content[8:12], uint32(loc.offset))
			},
			"early NUL": func(fixture *directPairTestFixture) {
				loc := directPairDMALocator(t, fixture, "driver")
				fixture.content[loc.offset+1] = 0
			},
			"missing terminal NUL": func(fixture *directPairTestFixture) {
				loc := directPairDMALocator(t, fixture, "driver")
				fixture.content[loc.offset+loc.length-1] = 'x'
			},
			"two dynamic ranges overlap": func(fixture *directPairTestFixture) {
				binary.LittleEndian.PutUint32(fixture.content[12:16], binary.LittleEndian.Uint32(fixture.content[8:12]))
			},
		} {
			t.Run(name, func(t *testing.T) {
				fixture := directPairDMAFixture("dma_fence_wait_start", []byte("gpu"), []byte("render"), 7, 9, false)
				mutate(&fixture)
				directPairAssertRejected(t, fixture)
			})
		}
	})

	t.Run("overlap never preserves a manufactured exact key", func(t *testing.T) {
		fixture := directPairDMAFixture("dma_fence_wait_start", []byte("gpu"), []byte("render"), 7, 9, false)
		binary.LittleEndian.PutUint32(fixture.content[12:16], binary.LittleEndian.Uint32(fixture.content[8:12]))
		payload, admission, reason := decodeDirectPairPayload(decodeEvent(fixture.format, fixture.content), fixture.content)
		if admission != bodyRejected || reason != "overlapping_dma_fence_strings" {
			t.Fatalf("overlap verdict admission=%d reason=%q", admission, reason)
		}
		verdict := fingerprintPairingEndpoint(pairPayloadTypedInput(payload, 100))
		if verdict.KeyKnown {
			t.Fatalf("overlapping dynamic bytes manufactured an exact DMA lane: %+v", verdict)
		}
	})

	t.Run("hard identity text is exact and injection safe", func(t *testing.T) {
		badValues := map[string][]byte{
			"empty":             {},
			"leading blank":     []byte(" gpu"),
			"trailing blank":    []byte("gpu "),
			"internal blank":    []byte("gpu render"),
			"tab":               []byte("gpu\trender"),
			"newline":           []byte("gpu\nrender"),
			"quote":             []byte(`gpu"render`),
			"apostrophe":        []byte("gpu'render"),
			"key injection":     []byte("gpu=context=8"),
			"wire punctuation":  []byte("gpu,"),
			"invalid UTF-8":     {0xff, 0xfe},
			"identity byte 257": []byte(strings.Repeat("x", 257)),
		}
		for name, value := range badValues {
			t.Run("driver/"+name, func(t *testing.T) {
				directPairAssertRejected(t, directPairDMAFixture("dma_fence_wait_start", value, []byte("render"), 7, 9, false))
			})
			t.Run("timeline/"+name, func(t *testing.T) {
				directPairAssertRejected(t, directPairDMAFixture("dma_fence_wait_start", []byte("gpu"), value, 7, 9, false))
			})
		}
	})

	t.Run("256 byte identities are admitted without trimming", func(t *testing.T) {
		driver := strings.Repeat("d", 256)
		timeline := strings.Repeat("t", 256)
		fixture := directPairDMAFixture("dma_fence_wait_end", []byte(driver), []byte(timeline), 0, math.MaxUint32, false)
		want := "driver=" + driver + " timeline=" + timeline + " context=0 seqno=4294967295"
		if body := directPairAdmittedBody(t, fixture); body != want {
			t.Fatalf("256-byte hard identity changed: got bytes=%d want bytes=%d", len(body), len(want))
		}
	})
}

func TestDirectPairCanonicalRoundTrip(t *testing.T) {
	workStart := directPairWorkqueueFixture("workqueue_execute_start", 8, true, 0xfedcba9876543210, 0x1111111111111111)
	workEnd := directPairWorkqueueFixture("workqueue_execute_end", 8, true, 0xfedcba9876543210, 0x2222222222222222)
	workLegacyEnd := directPairWorkqueueFixture("workqueue_execute_end", 8, false, 0xfedcba9876543210, 0)

	startVerdict := directPairRoundTripVerdict(t, workStart)
	endVerdict := directPairRoundTripVerdict(t, workEnd)
	legacyVerdict := directPairRoundTripVerdict(t, workLegacyEnd)
	if startVerdict.SemanticKey == "" || startVerdict.SemanticKey != endVerdict.SemanticKey || startVerdict.SemanticKey != legacyVerdict.SemanticKey {
		t.Fatalf("function metadata or legacy profile split one work lane: start=%+v end=%+v legacy=%+v", startVerdict, endVerdict, legacyVerdict)
	}

	dmaStart := directPairDMAFixture("dma_fence_wait_start", []byte("display"), []byte("present"), 0, math.MaxUint32, false)
	dmaEnd := directPairDMAFixture("dma_fence_wait_end", []byte("display"), []byte("present"), 0, math.MaxUint32, true)
	dmaStartVerdict := directPairRoundTripVerdict(t, dmaStart)
	dmaEndVerdict := directPairRoundTripVerdict(t, dmaEnd)
	if dmaStartVerdict.SemanticKey == "" || dmaStartVerdict.SemanticKey != dmaEndVerdict.SemanticKey {
		t.Fatalf("canonical DMA wire did not preserve one exact lane: start=%+v end=%+v", dmaStartVerdict, dmaEndVerdict)
	}
}

func TestDirectPairCanonicalRendererRejectsImpossibleTypedPayload(t *testing.T) {
	fixture := directPairDMAFixture("dma_fence_wait_start", []byte("gpu"), []byte("render"), 7, 9, false)
	payload, admission, reason := decodeDirectPairPayload(decodeEvent(fixture.format, fixture.content), fixture.content)
	if admission != bodyAdmitted || reason != "" || payload.DMAFence == nil {
		t.Fatalf("seed payload admission=%d reason=%q payload=%+v", admission, reason, payload)
	}

	payload.DMAFence.Context = uint64(math.MaxUint32) + 1
	if body, ok := renderCanonicalPairPayload(payload); ok || body != "" {
		t.Fatalf("canonical renderer published impossible uint32 context: ok=%v body=%q", ok, body)
	}
	payload.DMAFence.Context = 7
	payload.DMAFence.Seqno = uint64(math.MaxUint32) + 1
	if body, ok := renderCanonicalPairPayload(payload); ok || body != "" {
		t.Fatalf("canonical renderer published impossible uint32 seqno: ok=%v body=%q", ok, body)
	}

	workFixture := directPairWorkqueueFixture("workqueue_execute_start", 8, true, 0x12345678, 0x23456789)
	workPayload, workAdmission, workReason := decodeDirectPairPayload(decodeEvent(workFixture.format, workFixture.content), workFixture.content)
	if workAdmission != bodyAdmitted || workReason != "" || workPayload.Workqueue == nil {
		t.Fatalf("work seed admission=%d reason=%q payload=%+v", workAdmission, workReason, workPayload)
	}
	workPayload.Workqueue.FunctionKnown = false
	if body, ok := renderCanonicalPairPayload(workPayload); ok || body != "" {
		t.Fatalf("canonical renderer published current start without function metadata: ok=%v body=%q", ok, body)
	}

}

func TestDirectPairExactRegistryAndSingleAuthority(t *testing.T) {
	exact := []string{
		"workqueue_execute_start",
		"workqueue_execute_end",
		"dma_fence_wait_start",
		"dma_fence_wait_end",
	}
	for _, name := range exact {
		if !directPairNameGoverned(name) {
			t.Fatalf("exact pair endpoint missing from governed registry: %q", name)
		}
	}
	for _, name := range []string{
		"Workqueue_execute_start",
		"workqueue_execute_start ",
		"workqueue_execute_start_vendor",
		"workqueue_execute",
		"dma_fence_wait",
		"dma_fence_wait_start ",
		"dma_fence_wait_start_vendor",
		"dma_fence_signaled",
		"dma_fence_init",
	} {
		if directPairNameGoverned(name) {
			t.Fatalf("inventory/near-name entered the exact pair registry: %q", name)
		}
		fixture := directPairWorkqueueFixture(name, 8, true, 0x12345678, 0x23456789)
		if strings.HasPrefix(name, "dma_fence") {
			fixture = directPairDMAFixture(name, []byte("gpu"), []byte("render"), 7, 9, false)
		}
		if _, admission, reason := decodeDirectPairPayload(decodeEvent(fixture.format, fixture.content), fixture.content); admission != bodyUnsupported || reason != "" {
			t.Fatalf("near-name decoder verdict: name=%q admission=%d reason=%q", name, admission, reason)
		}
	}

	counts := map[string]int{
		"decodeDirectPairPayload":    0,
		"renderCanonicalPairPayload": 0,
		"directPairNameGoverned":     0,
	}
	files, err := filepath.Glob("*.go")
	if err != nil {
		t.Fatal(err)
	}
	for _, path := range files {
		if strings.HasSuffix(path, "_test.go") {
			continue
		}
		file, parseErr := parser.ParseFile(token.NewFileSet(), path, nil, 0)
		if parseErr != nil {
			t.Fatalf("parse %s: %v", path, parseErr)
		}
		ast.Inspect(file, func(node ast.Node) bool {
			decl, ok := node.(*ast.FuncDecl)
			if ok {
				if _, tracked := counts[decl.Name.Name]; tracked {
					counts[decl.Name.Name]++
				}
			}
			return true
		})
	}
	for name, count := range counts {
		if count != 1 {
			t.Fatalf("pair authority %s declaration count=%d want=1", name, count)
		}
	}

	renderSource, err := os.ReadFile("render.go")
	if err != nil {
		t.Fatal(err)
	}
	render := string(renderSource)
	decoderAt := strings.Index(render, "decodeDirectPairPayload(ev, content)")
	legacyAt := strings.Index(render, "renderLegacyEventBody(ev, content, cpu)")
	if decoderAt < 0 || legacyAt < 0 || decoderAt >= legacyAt {
		t.Fatalf("pair decoder must dominate legacy rendering: decoder=%d legacy=%d", decoderAt, legacyAt)
	}
	// The 256-byte hard-identity bounds make today's admitted pair body much
	// smaller than 1 MiB. Keep the shared final line gate structurally pinned
	// anyway so a later display-only extension cannot bypass that invariant.
	if !strings.Contains(render, "directPairNameGoverned(format.Name)") ||
		!strings.Contains(render, "traceDBSinglePhysicalLine(line, false)") {
		t.Fatal("pair endpoints are not pinned into the shared single-line/1MiB publication gate")
	}
}

func directPairAdmittedBody(t *testing.T, fixture directPairTestFixture) string {
	t.Helper()
	payload, admission, reason := decodeDirectPairPayload(decodeEvent(fixture.format, fixture.content), fixture.content)
	if admission != bodyAdmitted || reason != "" {
		t.Fatalf("pair admission=%d reason=%q payload=%+v", admission, reason, payload)
	}
	body, ok := renderCanonicalPairPayload(payload)
	if !ok || body == "" {
		t.Fatalf("canonical pair renderer rejected admitted payload: ok=%v body=%q payload=%+v", ok, body, payload)
	}
	return body
}

func directPairAssertRejected(t *testing.T, fixture directPairTestFixture) {
	t.Helper()
	_, admission, reason := decodeDirectPairPayload(decodeEvent(fixture.format, fixture.content), fixture.content)
	if admission != bodyRejected || reason == "" {
		t.Fatalf("malformed governed pair was not rejected: name=%q admission=%d reason=%q", fixture.format.Name, admission, reason)
	}
	if line, known := renderEventLine(
		renderContext{cmdlines: map[int]string{100: "pair-worker"}, tgids: map[int]int{100: 100}},
		1_000_000, 2, fixture.format, fixture.content,
	); known || line != "" {
		t.Fatalf("governed reject fell through to legacy/header rendering: known=%v line=%q", known, line)
	}
}

func directPairRoundTripVerdict(t *testing.T, fixture directPairTestFixture) tracequery.PairingEndpointVerdict {
	t.Helper()
	body := directPairAdmittedBody(t, fixture)
	verdict := tracequery.DecodePairingEndpoint(fixture.format.Name, body, 100)
	if !verdict.Recognized || !verdict.KeyKnown || !verdict.PayloadAdmitted || !verdict.EmitterKnown || !verdict.EmitterAdmitted {
		t.Fatalf("canonical pair body lost typed fingerprint on round-trip: body=%q verdict=%+v", body, verdict)
	}
	return verdict
}

func directPairCommonFields() []eventField {
	return []eventField{
		{Type: "unsigned short", Name: "common_type", Offset: 0, Size: 2},
		{Type: "unsigned char", Name: "common_flags", Offset: 2, Size: 1},
		{Type: "unsigned char", Name: "common_preempt_count", Offset: 3, Size: 1},
		{Type: "int", Name: "common_pid", Offset: 4, Size: 4, Signed: true},
	}
}

func directPairFillEnvelope(content []byte) {
	if len(content) < 8 {
		return
	}
	binary.LittleEndian.PutUint16(content[0:2], 900)
	binary.LittleEndian.PutUint32(content[4:8], 100)
}

func directPairWorkqueueFixture(name string, width int, withFunction bool, work, function uint64) directPairTestFixture {
	fields := directPairCommonFields()
	fields = append(fields, eventField{Type: "void *", Name: "work", Offset: 8, Size: width})
	end := 8 + width
	if withFunction {
		fields = append(fields, eventField{Type: "void *", Name: "function", Offset: end, Size: width})
		end += width
	}
	printFmt := `"work struct %p", REC->work`
	if withFunction {
		printFmt = `"work struct %p: function %ps", REC->work, REC->function`
	}
	content := make([]byte, end)
	directPairFillEnvelope(content)
	directPairPutWord(content[8:8+width], work)
	if withFunction {
		directPairPutWord(content[8+width:8+2*width], function)
	}
	return directPairTestFixture{format: eventFormat{Name: name, Fields: fields, PrintFmt: printFmt}, content: content}
}

func directPairPutWord(dst []byte, value uint64) {
	switch len(dst) {
	case 4:
		binary.LittleEndian.PutUint32(dst, uint32(value))
	case 8:
		binary.LittleEndian.PutUint64(dst, value)
	}
}

func directPairDMAFixture(name string, driver, timeline []byte, context, seqno uint32, signedLocator bool) directPairTestFixture {
	fields := directPairCommonFields()
	fields = append(fields,
		eventField{Type: "__data_loc char[]", Name: "driver", Offset: 8, Size: 4, Signed: signedLocator},
		eventField{Type: "__data_loc char[]", Name: "timeline", Offset: 12, Size: 4, Signed: signedLocator},
		eventField{Type: "unsigned int", Name: "context", Offset: 16, Size: 4},
		eventField{Type: "unsigned int", Name: "seqno", Offset: 20, Size: 4},
	)
	content := directPairDMAContent(24, driver, timeline, context, seqno)
	return directPairTestFixture{format: eventFormat{Name: name, Fields: fields}, content: content}
}

func directPairDMAContent(fixedTail int, driver, timeline []byte, context, seqno uint32) []byte {
	driverOffset := fixedTail
	timelineOffset := driverOffset + len(driver) + 1
	content := make([]byte, timelineOffset+len(timeline)+1)
	directPairFillEnvelope(content)
	binary.LittleEndian.PutUint32(content[8:12], uint32(((len(driver)+1)<<16)|driverOffset))
	binary.LittleEndian.PutUint32(content[12:16], uint32(((len(timeline)+1)<<16)|timelineOffset))
	binary.LittleEndian.PutUint32(content[16:20], context)
	binary.LittleEndian.PutUint32(content[20:24], seqno)
	copy(content[driverOffset:], driver)
	copy(content[timelineOffset:], timeline)
	return content
}

func directPairDMADuplicateFixture(fieldName string, conflict bool) directPairTestFixture {
	fixture := directPairDMAFixture("dma_fence_wait_start", []byte("gpu"), []byte("render"), 7, 9, false)
	original := *directPairField(nil, &fixture, fieldName)
	duplicate := original
	duplicate.Offset = 24
	fixture.format.Fields = append(fixture.format.Fields, duplicate)
	fixture.content = directPairDMAContent(28, []byte("gpu"), []byte("render"), 7, 9)

	switch fieldName {
	case "driver", "timeline":
		locField := directPairField(nil, &fixture, fieldName)
		loc := binary.LittleEndian.Uint32(fixture.content[locField.Offset : locField.Offset+4])
		if conflict {
			value := []byte("other\x00")
			offset := len(fixture.content)
			fixture.content = append(fixture.content, value...)
			loc = uint32((len(value) << 16) | offset)
		}
		binary.LittleEndian.PutUint32(fixture.content[24:28], loc)
	case "context":
		value := uint32(7)
		if conflict {
			value++
		}
		binary.LittleEndian.PutUint32(fixture.content[24:28], value)
	case "seqno":
		value := uint32(9)
		if conflict {
			value++
		}
		binary.LittleEndian.PutUint32(fixture.content[24:28], value)
	}
	return fixture
}

func directPairField(t *testing.T, fixture *directPairTestFixture, name string) *eventField {
	if t != nil {
		t.Helper()
	}
	for index := range fixture.format.Fields {
		if directCoreFieldBaseName(fixture.format.Fields[index].Name) == name {
			return &fixture.format.Fields[index]
		}
	}
	if t != nil {
		t.Fatalf("fixture field %q not found", name)
	}
	panic("direct pair fixture field not found: " + name)
}

func directPairRemoveField(fixture *directPairTestFixture, name string) {
	for index := range fixture.format.Fields {
		if directCoreFieldBaseName(fixture.format.Fields[index].Name) == name {
			fixture.format.Fields = append(fixture.format.Fields[:index], fixture.format.Fields[index+1:]...)
			return
		}
	}
}

func directPairDuplicateFixedField(t *testing.T, fixture *directPairTestFixture, name string, conflict bool) {
	t.Helper()
	original := *directPairField(t, fixture, name)
	raw := append([]byte(nil), fixture.content[original.Offset:original.Offset+original.Size]...)
	if conflict {
		raw[0]++
	}
	original.Offset = len(fixture.content)
	fixture.format.Fields = append(fixture.format.Fields, original)
	fixture.content = append(fixture.content, raw...)
}

type directPairDataLocator struct {
	offset int
	length int
}

func directPairDMALocator(t *testing.T, fixture *directPairTestFixture, name string) directPairDataLocator {
	t.Helper()
	field := directPairField(t, fixture, name)
	raw := binary.LittleEndian.Uint32(fixture.content[field.Offset : field.Offset+4])
	return directPairDataLocator{offset: int(raw & 0xffff), length: int(raw >> 16)}
}
