package tracequery

import (
	"strings"
	"testing"
)

func TestBlockRemapOfficialGrammarAndRQClassification(t *testing.T) {
	tests := []struct {
		name          string
		event         string
		wantDev       string
		wantOp        string
		wantBios      int64
		wantBiosKnown bool
	}{
		{
			name:          "rq remap with operation and bios",
			event:         "block_rq_remap: 12,80 RCVHS 66637568 + 8 <- (260,84) 14962432 3",
			wantDev:       "12,80",
			wantOp:        "RCVHS",
			wantBios:      3,
			wantBiosKnown: true,
		},
		{
			name:          "rq remap preserves exact zero bios",
			event:         "block_rq_remap: 12,80 R 66637568 + 8 <- (260,84) 14962432 0",
			wantDev:       "12,80",
			wantOp:        "R",
			wantBiosKnown: true,
		},
		{
			name:    "bio remap official operation",
			event:   "block_bio_remap: 12,80 R 66637568 + 8 <- (260,84) 14962432",
			wantDev: "12,80",
			wantOp:  "R",
		},
		{
			name:    "legacy bio remap without operation",
			event:   "block_bio_remap: 12,48 66637568 + 8 <- (260,84) 14962432",
			wantDev: "12,48",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			line := "worker-100 (100) [002] .... 1.000000: " + tt.event
			ev, ok := ParseLine(1, line, newStringInterner())
			if !ok || ev.Type != EventBlockRemap || ev.BlockIOFields == nil {
				t.Fatalf("official remap was not typed: ok=%v event=%+v", ok, ev)
			}
			blk := ev.BlockIOFields
			if !blk.IdentityValid || blk.Dev != tt.wantDev || blk.Op != tt.wantOp ||
				blk.Sector != 66637568 || blk.Len != 8 || blk.SrcDev != "260,84" || blk.SrcSector != 14962432 ||
				blk.RemapBios != tt.wantBios || blk.RemapBiosPresent != tt.wantBiosKnown {
				t.Fatalf("remap fields mismatch: %+v", blk)
			}
			if _, _, endpoint := blockLatencyEndpoint(ev); endpoint {
				t.Fatalf("remap inventory row entered the elapsed-latency endpoint lane: %+v", ev)
			}
		})
	}
}

func TestBlockRemapProfilesCannotCrossAcceptEachOther(t *testing.T) {
	for _, event := range []string{
		// RQ remap must carry both the operation and nr_bios.
		"block_rq_remap: 12,80 R 10 + 8 <- (8,1) 20",
		"block_rq_remap: 12,80 10 + 8 <- (8,1) 20",
		// BIO remap has no nr_bios, so an RQ-shaped tail is not BIO evidence.
		"block_bio_remap: 12,80 R 10 + 8 <- (8,1) 20 3",
		"block_bio_remap: 12,80 R 10 + 4294967296 <- (8,1) 20",
	} {
		line := "worker-100 (100) [002] .... 1.000000: " + event
		ev, ok := ParseLine(1, line, newStringInterner())
		if !ok || ev.Type != EventBlockRemap || ev.BlockIOFields == nil {
			t.Fatalf("remap inventory row disappeared: ok=%v event=%+v", ok, ev)
		}
		if ev.BlockIOFields.IdentityValid || ev.BlockIOFields.RemapBiosPresent ||
			ev.BlockIOFields.Dev != "" || ev.BlockIOFields.Op != "" {
			t.Fatalf("cross-profile remap published typed provenance: %q %+v", event, ev.BlockIOFields)
		}
	}
}

func TestInvalidBlockRemapCannotPublishExactZeroPresence(t *testing.T) {
	line := "worker-100 (100) [002] .... 1.000000: block_rq_remap: 12,80 R 10 + 8 <- (8,1) 20 4294967296"
	ev, ok := ParseLine(1, line, newStringInterner())
	if !ok || ev.BlockIOFields == nil {
		t.Fatalf("invalid remap inventory row disappeared: ok=%v event=%+v", ok, ev)
	}
	blk := ev.BlockIOFields
	if blk.IdentityValid || blk.RemapBiosPresent || blk.RemapBios != 0 || blk.Dev != "" || blk.SrcDev != "" {
		t.Fatalf("invalid nr_bios masqueraded as an exact typed zero: %+v", blk)
	}
}

func TestBlockDeviceGrammarCanonicalizesAndRejectsNonIdentityValues(t *testing.T) {
	tests := []struct {
		raw      string
		want     string
		parsed   bool
		identity bool
	}{
		{raw: "12,80", want: "12,80", parsed: true, identity: true},
		{raw: "0012:00080", want: "12,80", parsed: true, identity: true},
		{raw: "4095,1048575", want: "4095,1048575", parsed: true, identity: true},
		{raw: "0,0", want: "0,0", parsed: true, identity: false},
		{raw: "4096,0"},
		{raw: "1,1048576"},
		{raw: "-1,0"},
		{raw: "+1,0"},
		{raw: "dev0"},
		{raw: "1,2,3"},
	}
	for _, tt := range tests {
		t.Run(tt.raw, func(t *testing.T) {
			got, ok := canonicalBlockDevice(tt.raw)
			if ok != tt.parsed || got != tt.want || (ok && blockDeviceIdentifiesRequest(got) != tt.identity) {
				t.Fatalf("canonicalBlockDevice(%q)=(%q,%v identity=%v), want (%q,%v identity=%v)",
					tt.raw, got, ok, blockDeviceIdentifiesRequest(got), tt.want, tt.parsed, tt.identity)
			}
		})
	}
}

func TestBlockNullDeviceAndNonFlushZeroRemainInventoryButNeverPair(t *testing.T) {
	idx := buildTraceIndex(t, "block-null-and-zero.systrace", `
 io-40 (40) [003] .... 1.000000: block_rq_issue: 0,0 R 4096 () 123 + 8 [io]
irq-2 (2) [003] .... 1.001000: block_rq_complete: 0,0 R () 123 + 8 [0]
 io-40 (40) [003] .... 2.000000: block_rq_issue: 12,80 R 0 () 0 + 0 [io]
irq-2 (2) [003] .... 2.001000: block_rq_complete: 12,80 R () 0 + 0 [0]
`)
	if len(idx.Events) != 4 {
		t.Fatalf("exact kernel inventory rows were discarded: %+v", idx.Events)
	}
	stats := ComputeWindowStats(idx, Query{TimeStart: 0, TimeEnd: 3})
	if len(stats.IOLatencies) != 0 {
		t.Fatalf("null-device or non-flush zero request entered latency pairing: %+v", stats.IOLatencies)
	}
	if !containsSubstring(stats.Caveats, "block_io_pairing_identity_invalid=true") {
		t.Fatalf("rejected physical identities lack disclosure: %+v", stats.Caveats)
	}
}

func TestHandBuiltColonNullDeviceCannotBypassPairingGate(t *testing.T) {
	for _, fields := range []*BlockIOFields{
		{Dev: "0:0", Op: "R", Sector: 1, Len: 1},
		{Dev: "0:0", Op: "R", Sector: 1, Len: 1, IdentityParsed: true, IdentityValid: true},
		{Dev: "garbage", Op: "R", Sector: 1, Len: 1, IdentityParsed: true, IdentityValid: true},
	} {
		ev := Event{
			Type:          EventBlockIssue,
			Name:          "block_rq_issue",
			BlockIOFields: fields,
		}
		if blockRequestIdentityValid(ev) {
			t.Fatalf("hand-built invalid device bypassed canonical gate: %+v", fields)
		}
	}
}

func TestBlockRequestGrammarIsFullyAnchored(t *testing.T) {
	valid := "12,80 R 4096 () 123 + 8 [io]"
	dev, op, sector, length, ok := parseBlockRequestValidated("block_rq_issue", valid)
	if !ok || dev != "12,80" || op != "R" || sector != 123 || length != 8 {
		t.Fatalf("canonical request rejected: dev=%q op=%q sector=%d len=%d ok=%v", dev, op, sector, length, ok)
	}
	for _, malformed := range []string{
		valid + " forged=1",
		"garbage R 4096 () 123 + 8 [io]",
		"12,80 R|W 4096 () 123 + 8 [io]",
		"12,80 123 4096 () 123 + 8 [io]",
		"12,80 R 4096 () 123 + 8 [io] trailing",
	} {
		if _, _, _, _, ok := parseBlockRequestValidated("block_rq_issue", malformed); ok {
			t.Fatalf("malformed request prefix escaped the anchored grammar: %q", malformed)
		}
	}
	if _, _, _, _, ok := parseBlockRequestValidated("block_rq_issue", "12,80 FS 0 () 0 + 0 [io]"); !ok {
		t.Fatal("exact zero-length flush must remain a valid latency identity")
	}
	if got := strings.TrimSpace(parseBlockError(valid)); got != "io" {
		t.Fatalf("request suffix remained unparsable after anchoring: %q", got)
	}
}

func TestBlockEndpointProfilesAndByteDomainAreClosed(t *testing.T) {
	for _, tt := range []struct {
		name, rawType, fields string
		valid                 bool
	}{
		{name: "rq issue max bytes", rawType: "block_rq_issue", fields: "12,80 R 4294967295 () 123 + 8 [io]", valid: true},
		{name: "rq issue byte overflow", rawType: "block_rq_issue", fields: "12,80 R 4294967296 () 123 + 8 [io]"},
		{name: "rq issue max sectors", rawType: "block_rq_issue", fields: "12,80 R 4096 () 123 + 4294967295 [io]", valid: true},
		{name: "rq issue sector-count overflow", rawType: "block_rq_issue", fields: "12,80 R 4096 () 123 + 4294967296 [io]"},
		{name: "rq issue missing bytes", rawType: "block_rq_issue", fields: "12,80 R () 123 + 8 [io]"},
		{name: "rq issue rejects bio shape", rawType: "block_rq_issue", fields: "12,80 R 123 + 8 [io]"},
		{name: "rq complete", rawType: "block_rq_complete", fields: "12,80 R () 123 + 8 [-5]", valid: true},
		{name: "rq complete rejects issue shape", rawType: "block_rq_complete", fields: "12,80 R 4096 () 123 + 8 [0]"},
		{name: "rq complete error overflow", rawType: "block_rq_complete", fields: "12,80 R () 123 + 8 [2147483648]"},
		{name: "bio queue", rawType: "block_bio_queue", fields: "12,80 R 123 + 8 [io]", valid: true},
		{name: "bio queue rejects rq shape", rawType: "block_bio_queue", fields: "12,80 R 4096 () 123 + 8 [io]"},
		{name: "bio complete", rawType: "block_bio_complete", fields: "12,80 R 123 + 8 [0]", valid: true},
		{name: "bio complete error overflow", rawType: "block_bio_complete", fields: "12,80 R 123 + 8 [-2147483649]"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			_, _, _, _, valid := parseBlockRequestValidated(tt.rawType, tt.fields)
			if valid != tt.valid {
				t.Fatalf("profile verdict=%v want=%v for %s: %q", valid, tt.valid, tt.rawType, tt.fields)
			}
		})
	}
}

func TestMalformedRQIssueCannotMintLatencyAgainstValidComplete(t *testing.T) {
	idx := buildTraceIndex(t, "block-byte-overflow.systrace", `
 io-40 (40) [003] .... 1.000000: block_rq_issue: 12,80 R 4294967296 () 123 + 8 [io]
irq-2 (2) [003] .... 1.001000: block_rq_complete: 12,80 R () 123 + 8 [0]
`)
	stats := ComputeWindowStats(idx, Query{TimeStart: 0, TimeEnd: 2})
	if len(stats.IOLatencies) != 0 {
		t.Fatalf("malformed issue bytes minted elapsed latency: %+v", stats.IOLatencies)
	}
	if !containsSubstring(stats.Caveats, "block_io_pairing_identity_invalid=true") {
		t.Fatalf("malformed endpoint rejection lacks disclosure: %+v", stats.Caveats)
	}
}
