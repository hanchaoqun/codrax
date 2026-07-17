package hitraceconv

import (
	"context"
	"encoding/binary"
	"math"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/tracequery"
)

func TestTraceDBRawBinderDomainClosure(t *testing.T) {
	t.Parallel()

	integer := func(value uint64) traceDBValue {
		return traceDBValue{Valid: true, Text: strconv.FormatUint(value, 10), Datatype: 0}
	}
	validMax := map[string]traceDBValue{
		"transaction": integer(math.MaxInt32),
		"dest_node":   integer(math.MaxInt32),
		"dest_proc":   integer(math.MaxInt32),
		"dest_thread": integer(math.MaxInt32),
		"reply":       integer(0),
		"flags":       integer(math.MaxUint32),
		"code":        integer(math.MaxUint32),
	}
	if !traceDBRawRequiredArgs("binder_transaction", validMax, nil) {
		t.Fatal("canonical Binder int32/uint32 maxima were rejected")
	}

	validMin := map[string]traceDBValue{
		"transaction": integer(1),
		"dest_node":   integer(0),
		"dest_proc":   integer(1),
		"dest_thread": integer(0),
		"reply":       integer(1),
		"flags":       integer(0),
		"code":        integer(0),
	}
	if !traceDBRawRequiredArgs("binder_transaction", validMin, nil) {
		t.Fatal("canonical Binder minima, including dest_thread=0, were rejected")
	}
	receivedMax := map[string]traceDBValue{"transaction": integer(math.MaxInt32)}
	if !traceDBRawRequiredArgs("binder_transaction_received", receivedMax, nil) {
		t.Fatal("canonical received transaction int32 maximum was rejected")
	}

	clone := func(src map[string]traceDBValue) map[string]traceDBValue {
		out := make(map[string]traceDBValue, len(src))
		for key, value := range src {
			out[key] = value
		}
		return out
	}
	tests := []struct {
		name  string
		field string
		value uint64
	}{
		{name: "transaction_above_int32", field: "transaction", value: uint64(math.MaxInt32) + 1},
		{name: "dest_node_above_int32", field: "dest_node", value: uint64(math.MaxInt32) + 1},
		{name: "dest_proc_zero", field: "dest_proc", value: 0},
		{name: "dest_proc_above_int32", field: "dest_proc", value: uint64(math.MaxInt32) + 1},
		{name: "dest_thread_above_int32", field: "dest_thread", value: uint64(math.MaxInt32) + 1},
		{name: "flags_above_uint32", field: "flags", value: uint64(math.MaxUint32) + 1},
		{name: "code_above_uint32", field: "code", value: uint64(math.MaxUint32) + 1},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			args := clone(validMax)
			args[tt.field] = integer(tt.value)
			if traceDBRawRequiredArgs("binder_transaction", args, nil) {
				t.Fatalf("out-of-domain Binder %s=%d was accepted", tt.field, tt.value)
			}
		})
	}
	t.Run("received_transaction_above_int32", func(t *testing.T) {
		args := map[string]traceDBValue{"transaction": integer(uint64(math.MaxInt32) + 1)}
		if traceDBRawRequiredArgs("binder_transaction_received", args, nil) {
			t.Fatal("received transaction above int32 was accepted")
		}
	})
}

func TestTraceDBRawBinderRendererFeedsStrictTraceQueryReader(t *testing.T) {
	t.Parallel()

	integer := func(value uint64) traceDBValue {
		return traceDBValue{Valid: true, Text: strconv.FormatUint(value, 10), Datatype: 0}
	}
	args := map[string]traceDBValue{
		"transaction": integer(math.MaxInt32),
		"dest_node":   integer(math.MaxInt32),
		"dest_proc":   integer(math.MaxInt32),
		"dest_thread": integer(math.MaxInt32),
		"reply":       integer(0),
		"flags":       integer(math.MaxUint32),
		"code":        integer(math.MaxUint32),
	}
	if !traceDBRawRequiredArgs("binder_transaction", args, nil) {
		t.Fatal("valid Binder endpoint did not reach the renderer")
	}
	payload, ok := traceDBRenderRawBinder("binder_transaction", args)
	if !ok {
		t.Fatal("valid Binder endpoint was not rendered")
	}
	if !strings.Contains(payload, "flags=0xffffffff code=0xffffffff") || strings.Contains(payload, "0X") {
		t.Fatalf("Binder uint32 fields were not rendered as canonical lowercase hex: %q", payload)
	}

	tracePath := filepath.Join(t.TempDir(), "binder-domain.systrace")
	line := "binder-client-100 (100) [001] .... 1.000000: " + payload + "\n"
	if err := os.WriteFile(tracePath, []byte(line), 0o600); err != nil {
		t.Fatalf("write Binder reader fixture: %v", err)
	}
	idx, err := tracequery.BuildIndex(context.Background(), tracePath)
	if err != nil {
		t.Fatalf("strict trace reader rejected renderer output: %v", err)
	}
	for _, event := range idx.Events {
		if event.Type != tracequery.EventBinderTransaction {
			continue
		}
		fields := event.BinderFields
		if fields == nil || fields.TransactionID != math.MaxInt32 || fields.DestProc != math.MaxInt32 ||
			fields.DestThread != math.MaxInt32 || fields.Reply != 0 ||
			fields.Flags != "0xffffffff" || fields.Code != "0xffffffff" {
			t.Fatalf("strict reader changed canonical Binder renderer fields: %+v", fields)
		}
		return
	}
	t.Fatalf("strict reader did not retain the rendered Binder event: %+v", idx.Events)
}

func TestDirectBinderPayloadDomainClosure(t *testing.T) {
	t.Parallel()

	max := binderTransactionCoreCase(true)
	for index, value := range []uint32{
		math.MaxInt32, math.MaxInt32, math.MaxInt32, math.MaxInt32,
		1, math.MaxUint32, math.MaxUint32,
	} {
		binary.LittleEndian.PutUint32(max.content[index*4:index*4+4], value)
	}
	payload, admission, reason := decodeDirectCorePayload(max.ctx, decodeEvent(max.format, max.content), max.content)
	if admission != bodyAdmitted || reason != "" || payload.Binder == nil {
		t.Fatalf("direct Binder int32/uint32 maxima were rejected: admission=%d reason=%q payload=%+v", admission, reason, payload)
	}
	if payload.Binder.Transaction != math.MaxInt32 || payload.Binder.DestNode != math.MaxInt32 ||
		payload.Binder.DestProc != math.MaxInt32 || payload.Binder.DestThread != math.MaxInt32 ||
		payload.Binder.Reply != 1 || payload.Binder.Flags != math.MaxUint32 || payload.Binder.Code != math.MaxUint32 {
		t.Fatalf("direct Binder maxima changed during decode: %+v", payload.Binder)
	}

	zeroProc := binderTransactionCoreCase(true)
	binary.LittleEndian.PutUint32(zeroProc.content[8:12], 0)
	_, admission, reason = decodeDirectCorePayload(zeroProc.ctx, decodeEvent(zeroProc.format, zeroProc.content), zeroProc.content)
	if admission != bodyRejected || reason != "invalid_transaction_endpoint" {
		t.Fatalf("direct Binder dest_proc=0 escaped the endpoint domain: admission=%d reason=%q", admission, reason)
	}

	aboveSignedMax := binderTransactionCoreCase(true)
	binary.LittleEndian.PutUint32(aboveSignedMax.content[:4], uint32(math.MaxInt32)+1)
	_, admission, reason = decodeDirectCorePayload(aboveSignedMax.ctx, decodeEvent(aboveSignedMax.format, aboveSignedMax.content), aboveSignedMax.content)
	if admission != bodyRejected || reason != "invalid_transaction_id" {
		t.Fatalf("direct Binder transaction above int32 escaped the signed wire domain: admission=%d reason=%q", admission, reason)
	}
}
