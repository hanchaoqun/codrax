package hitraceconv

import (
	"strings"
	"testing"
)

func TestTraceDBBlockRendererUsesCanonicalTypedPayloads(t *testing.T) {
	integer := func(value string) traceDBValue { return traceDBValue{Valid: true, Text: value, Datatype: 0} }
	text := func(value string) traceDBValue { return traceDBValue{Valid: true, Text: value, Datatype: 1} }
	base := func() map[string]traceDBValue {
		return map[string]traceDBValue{
			"dev": integer("12582992"), "sector": integer("123"), "nr_sector": integer("8"), "rwbs": text("R"),
		}
	}
	tests := []struct {
		name string
		args map[string]traceDBValue
		want string
	}{
		{name: "block_bio_complete", args: mergeTraceDBBlockArgs(base(), map[string]traceDBValue{"error": integer("-5")}), want: "block_bio_complete: 12,80 R 123 + 8 [-5]"},
		{name: "block_bio_queue", args: mergeTraceDBBlockArgs(base(), map[string]traceDBValue{"comm": text("io-worker")}), want: "block_bio_queue: 12,80 R 123 + 8 [io-worker]"},
		{name: "block_bio_remap", args: mergeTraceDBBlockArgs(base(), map[string]traceDBValue{"old_dev": text("8:1"), "old_sector": integer("77")}), want: "block_bio_remap: 12,80 R 123 + 8 <- (8,1) 77"},
		{name: "block_rq_complete", args: mergeTraceDBBlockArgs(base(), map[string]traceDBValue{"error": integer("-5"), "cmd": text("READ")}), want: "block_rq_complete: 12,80 R (READ) 123 + 8 [-5]"},
		{name: "block_rq_insert", args: mergeTraceDBBlockArgs(base(), map[string]traceDBValue{"bytes": integer("4096"), "cmd": text("READ"), "comm": text("io-worker")}), want: "block_rq_insert: 12,80 R 4096 (READ) 123 + 8 [io-worker]"},
		{name: "block_rq_issue", args: mergeTraceDBBlockArgs(base(), map[string]traceDBValue{"bytes": integer("4096"), "cmd": text("READ"), "comm": text("io-worker")}), want: "block_rq_issue: 12,80 R 4096 (READ) 123 + 8 [io-worker]"},
		{name: "block_rq_remap", args: mergeTraceDBBlockArgs(base(), map[string]traceDBValue{"old_dev": text("8,1"), "old_sector": integer("77"), "nr_bios": integer("2")}), want: "block_rq_remap: 12,80 R 123 + 8 <- (8,1) 77 2"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if traceDBRawFtraceClass(tt.name) != "block_storage" {
				t.Fatal("SQL raw classifier omitted canonical block event")
			}
			got, ok := renderTraceDBBlockEvent(tt.name, tt.args, nil)
			if !ok || got != tt.want {
				t.Fatalf("SQL block canonical mismatch: ok=%v got=%q want=%q", ok, got, tt.want)
			}
			if !traceDBRawRequiredArgs(tt.name, tt.args, nil) {
				t.Fatal("required-args gate diverged from shared SQL decoder")
			}
		})
	}

	zero := map[string]traceDBValue{
		"dev": integer("0"), "sector": integer("0"), "nr_sector": integer("0"), "bytes": integer("0"), "rwbs": text("R"),
	}
	if got, ok := renderTraceDBBlockEvent("block_rq_issue", zero, nil); !ok || got != "block_rq_issue: 0,0 R 0 () 0 + 0 []" {
		t.Fatalf("typed SQL exact-zero inventory was not preserved: ok=%v got=%q", ok, got)
	}
}

func TestTraceDBBlockDecoderFailsClosedWithoutCrossFieldFallback(t *testing.T) {
	integer := func(value string) traceDBValue { return traceDBValue{Valid: true, Text: value, Datatype: 0} }
	text := func(value string) traceDBValue { return traceDBValue{Valid: true, Text: value, Datatype: 1} }
	valid := func() map[string]traceDBValue {
		return map[string]traceDBValue{
			"dev": text("8,0"), "sector": integer("128"), "nr_sector": integer("8"), "bytes": integer("4096"),
			"rwbs": text("R"), "cmd": text("READ"), "comm": text("io-worker"),
		}
	}
	tests := []struct {
		name        string
		mutate      func(map[string]traceDBValue)
		invalidKeys map[string]bool
	}{
		{name: "missing dev", mutate: func(args map[string]traceDBValue) { delete(args, "dev") }},
		{name: "missing rwbs cannot use cmd", mutate: func(args map[string]traceDBValue) { delete(args, "rwbs") }},
		{name: "missing nr sector cannot use bytes", mutate: func(args map[string]traceDBValue) { delete(args, "nr_sector") }},
		{name: "missing bytes cannot use nr sector", mutate: func(args map[string]traceDBValue) { delete(args, "bytes") }},
		{name: "numeric text wrong wire", mutate: func(args map[string]traceDBValue) { args["sector"] = text("128") }},
		{name: "noncanonical integer spelling", mutate: func(args map[string]traceDBValue) { args["sector"] = integer("+128") }},
		{name: "rwbs integer wrong wire", mutate: func(args map[string]traceDBValue) { args["rwbs"] = integer("1") }},
		{name: "cross alias ambiguity", mutate: func(args map[string]traceDBValue) { args["op"] = text("R") }},
		{name: "poisoned alias", mutate: func(map[string]traceDBValue) {}, invalidKeys: map[string]bool{"rw": true}},
		{name: "device major out of range", mutate: func(args map[string]traceDBValue) { args["dev"] = text("4096,0") }},
		{name: "device minor out of range", mutate: func(args map[string]traceDBValue) { args["dev"] = text("8,1048576") }},
		{name: "device sign forbidden", mutate: func(args map[string]traceDBValue) { args["dev"] = text("+8,0") }},
		{name: "sector signed forbidden", mutate: func(args map[string]traceDBValue) { args["sector"] = integer("-1") }},
		{name: "nr sector uint32 bound", mutate: func(args map[string]traceDBValue) { args["nr_sector"] = integer("4294967296") }},
		{name: "bytes uint32 bound", mutate: func(args map[string]traceDBValue) { args["bytes"] = integer("4294967296") }},
		{name: "unsafe cmd grammar", mutate: func(args map[string]traceDBValue) { args["cmd"] = text("READ) 0 + 1") }},
		{name: "unsafe comm grammar", mutate: func(args map[string]traceDBValue) { args["comm"] = text("io] forged") }},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			args := valid()
			tt.mutate(args)
			if payload, ok := decodeTraceDBBlockPayload("block_rq_issue", args, tt.invalidKeys); ok {
				t.Fatalf("malformed SQL args escaped strict decoder: %+v", payload)
			}
			if traceDBRawRequiredArgs("block_rq_issue", args, tt.invalidKeys) {
				t.Fatal("required-args gate diverged from strict decoder")
			}
		})
	}

	complete := map[string]traceDBValue{
		"dev": text("8,0"), "sector": integer("1"), "nr_sector": integer("1"), "error": integer("2147483648"), "rwbs": text("R"),
	}
	if _, ok := decodeTraceDBBlockPayload("block_rq_complete", complete, nil); ok {
		t.Fatal("SQL error outside int32 escaped strict decoder")
	}
	complete["error"] = integer("-2147483648")
	if got, ok := renderTraceDBBlockEvent("block_rq_complete", complete, nil); !ok || !strings.HasSuffix(got, "[-2147483648]") {
		t.Fatalf("canonical SQL int32 minimum rejected: ok=%v got=%q", ok, got)
	}
	complete["sector"] = integer("9223372036854775807")
	if _, ok := decodeTraceDBBlockPayload("block_rq_complete", complete, nil); !ok {
		t.Fatal("SQL MaxInt64 sector rejected")
	}
	complete["sector"] = integer("9223372036854775808")
	if _, ok := decodeTraceDBBlockPayload("block_rq_complete", complete, nil); ok {
		t.Fatal("SQL sector above MaxInt64 escaped strict decoder")
	}
}

func mergeTraceDBBlockArgs(base map[string]traceDBValue, extra map[string]traceDBValue) map[string]traceDBValue {
	for key, value := range extra {
		base[key] = value
	}
	return base
}
