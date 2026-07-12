package tracequery

import (
	"go/ast"
	"go/parser"
	"go/token"
	"strings"
	"testing"
)

func TestDecodePairingEndpointCanonicalIdentityMatrix(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name     string
		eventA   string
		fieldsA  string
		tidA     int64
		eventB   string
		fieldsB  string
		tidB     int64
		family   PairingEndpointFamily
		idle     bool
		admitted bool
	}{
		{
			name: "binder decimal canonical", eventA: "binder_transaction", fieldsA: "transaction=0007 dest_thread=20", tidA: 10,
			eventB: "binder_transaction_received", fieldsB: "transaction=7", tidB: 20,
			family: PairingEndpointBinder, admitted: true,
		},
		{
			name: "work pointer canonical and function metadata", eventA: "workqueue_execute_start", fieldsA: "work=0x0A function=0x111", tidA: 10,
			eventB: "workqueue_execute_end", fieldsB: "work=0000000a function=0x222", tidB: 10,
			family: PairingEndpointWorkqueue, admitted: true,
		},
		{
			name: "dma unsigned canonical", eventA: "dma_fence_wait_start", fieldsA: "driver=gpu timeline=render context=0x7 seqno=007", tidA: 10,
			eventB: "dma_fence_wait_end", fieldsB: "driver=gpu timeline=render context=7 seqno=0x7", tidB: 10,
			family: PairingEndpointDMAFence, admitted: true,
		},
		{
			name: "block device canonical and cross thread", eventA: "block_rq_issue", fieldsA: "8:0 R 4096 (cmd) 32 + 8 [worker]", tidA: 0,
			eventB: "block_rq_complete", fieldsB: "8,0 R (cmd) 32 + 8 [0]", tidB: 99,
			family: PairingEndpointBlock, idle: true, admitted: true,
		},
		{
			name: "generic storage coarse identity", eventA: "android_fs_dataread_start", fieldsA: "dev=8:0 ino=0007 rw=read tag=1", tidA: 0,
			eventB: "android_fs_dataread_done", fieldsB: "dev=8,0 ino=7 rw=read tag=999", tidB: 0,
			family: PairingEndpointStorage, idle: true, admitted: true,
		},
	}
	for _, tc := range tests {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			a := DecodePairingEndpoint(tc.eventA, tc.fieldsA, tc.tidA)
			b := DecodePairingEndpoint(tc.eventB, tc.fieldsB, tc.tidB)
			if !a.Recognized || !b.Recognized || !a.KeyKnown || !b.KeyKnown {
				t.Fatalf("fingerprint not admitted: a=%+v b=%+v", a, b)
			}
			if a.Family != tc.family || b.Family != tc.family || a.SemanticKey != b.SemanticKey {
				t.Fatalf("canonical key mismatch: a=%+v b=%+v", a, b)
			}
			if a.Phase != PairingEndpointStart || b.Phase != PairingEndpointDone {
				t.Fatalf("phase mismatch: a=%+v b=%+v", a, b)
			}
			if a.IdleAllowed != tc.idle || b.IdleAllowed != tc.idle || a.EmitterAdmitted != tc.admitted || b.EmitterAdmitted != tc.admitted {
				t.Fatalf("emitter policy mismatch: a=%+v b=%+v", a, b)
			}
		})
	}
}

func TestDecodePairingEndpointIdleAndUnknownEmitterPolicy(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name     string
		event    string
		fields   string
		tid      int64
		keyKnown bool
		admitted bool
	}{
		{name: "binder idle exact key", event: "binder_transaction", fields: "transaction=7", tid: 0, keyKnown: true},
		{name: "workqueue idle exact key", event: "workqueue_execute_start", fields: "work=0x1", tid: 0, keyKnown: true},
		{name: "dma idle exact key", event: "dma_fence_wait_start", fields: "driver=gpu timeline=t context=1 seqno=2", tid: 0, keyKnown: true},
		{name: "workqueue unknown owner", event: "workqueue_execute_start", fields: "work=0x1", tid: -1},
		{name: "binder malformed transaction", event: "binder_transaction", fields: "transaction=0", tid: 10, admitted: true},
		{name: "block idle admitted", event: "block_bio_queue", fields: "8,0 R 1 + 1 [kworker]", tid: 0, keyKnown: true, admitted: true},
	}
	for _, tc := range tests {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			got := DecodePairingEndpoint(tc.event, tc.fields, tc.tid)
			if !got.Recognized || got.KeyKnown != tc.keyKnown || got.EmitterAdmitted != tc.admitted {
				t.Fatalf("verdict=%+v want keyKnown=%t admitted=%t", got, tc.keyKnown, tc.admitted)
			}
		})
	}
}

func TestDecodePairingEndpointKeepsExactCaseClosedSet(t *testing.T) {
	t.Parallel()
	for _, name := range []string{
		"BINDER_TRANSACTION",
		"WORKQUEUE_EXECUTE_START",
		"DMA_FENCE_WAIT_END",
		"BLOCK_RQ_ISSUE",
	} {
		if got := DecodePairingEndpoint(name, "transaction=7 work=0x1 driver=gpu timeline=t context=1 seqno=2", 10); got.Recognized {
			t.Errorf("case-drift event %q entered exact pairing registry: %+v", name, got)
		}
	}
}

func TestDecodePairingEndpointDistinguishesIdleFromUnknownEmitter(t *testing.T) {
	t.Parallel()
	idle := DecodePairingEndpoint("binder_transaction", "transaction=7", 0)
	unknown := DecodePairingEndpoint("binder_transaction", "transaction=7", -1)
	if !idle.KeyKnown || !idle.EmitterKnown || idle.EmitterAdmitted {
		t.Fatalf("idle emitter status=%+v", idle)
	}
	if !unknown.KeyKnown || unknown.EmitterKnown || unknown.EmitterAdmitted {
		t.Fatalf("unknown emitter status=%+v", unknown)
	}
}

func TestPairingEndpointLaneKeyNamespacesSourceAndIsInjective(t *testing.T) {
	t.Parallel()
	verdict := DecodePairingEndpoint("binder_transaction", "transaction=7", 10)
	left, leftOK := verdict.LaneKey("/trace/a")
	right, rightOK := verdict.LaneKey("/trace/b")
	if !leftOK || !rightOK || left == right {
		t.Fatalf("source namespace mismatch: left=%q/%t right=%q/%t", left, leftOK, right, rightOK)
	}
	if encodePairingKey("a", "b\x00c") == encodePairingKey("a\x00b", "c") {
		t.Fatal("length-prefixed pairing key is not injective")
	}
}

func TestTypedPairingFingerprintMatchesWireAuthority(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name  string
		wire  PairingEndpointVerdict
		typed PairingEndpointTypedInput
	}{
		{
			name:  "binder",
			wire:  DecodePairingEndpoint("binder_transaction", "transaction=7 dest_proc=20", 10),
			typed: PairingEndpointTypedInput{Name: "binder_transaction", HeaderTID: 10, Transaction: "007"},
		},
		{
			name:  "workqueue",
			wire:  DecodePairingEndpoint("workqueue_execute_start", "work=0x0a function=0x1", 10),
			typed: PairingEndpointTypedInput{Name: "workqueue_execute_start", HeaderTID: 10, Work: "0000000a"},
		},
		{
			name:  "dma",
			wire:  DecodePairingEndpoint("dma_fence_wait_start", "driver=gpu timeline=t context=0x7 seqno=007", 10),
			typed: PairingEndpointTypedInput{Name: "dma_fence_wait_start", HeaderTID: 10, Driver: "gpu", Timeline: "t", Context: "7", Seqno: "0x7"},
		},
		{
			name:  "block",
			wire:  DecodePairingEndpoint("block_rq_issue", "8,0 R 4096 (cmd) 32 + 8 [worker]", 10),
			typed: PairingEndpointTypedInput{Name: "block_rq_issue", HeaderTID: 10, BlockIdentityKnown: true, BlockDevice: "8:0", BlockOperation: "R", BlockSector: 32, BlockLength: 8},
		},
		{
			name:  "storage",
			wire:  DecodePairingEndpoint("scsi_dispatch_cmd_start", "tag=1 dev=12,80 lba=100 len=4096 opcode=READ_10", 10),
			typed: PairingEndpointTypedInput{Name: "scsi_dispatch_cmd_start", HeaderTID: 10, StorageIdentityKnown: true, StorageDevice: "12:80", StorageInode: "999", StorageOperation: "READ_10"},
		},
		{
			name:  "mmc storage",
			wire:  DecodePairingEndpoint("mmc_request_start", "mmc0 tag=1 opcode=17 blocks=8 block_size=512 blk_addr=10", 10),
			typed: PairingEndpointTypedInput{Name: "mmc_request_start", HeaderTID: 10, StorageIdentityKnown: true, StorageDevice: "mmc0", StorageInode: "10", StorageOperation: "17"},
		},
	}
	for _, tc := range tests {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			got := FingerprintPairingEndpoint(tc.typed)
			if !tc.wire.Recognized || !tc.wire.KeyKnown || got.Family != tc.wire.Family || got.Phase != tc.wire.Phase || got.SemanticKey != tc.wire.SemanticKey || got.EmitterAdmitted != tc.wire.EmitterAdmitted {
				t.Fatalf("wire/typed parity mismatch: wire=%+v typed=%+v", tc.wire, got)
			}
		})
	}
}

func TestTypedPairingNumericScalarsMatchCanonicalWire(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name  string
		wire  PairingEndpointVerdict
		typed PairingEndpointTypedInput
	}{
		{
			name: "binder integer", wire: DecodePairingEndpoint("binder_transaction", "transaction=7", 10),
			typed: PairingEndpointTypedInput{Name: "binder_transaction", HeaderTID: 10, TransactionNumber: 7, TransactionNumberKnown: true},
		},
		{
			name: "work address integer", wire: DecodePairingEndpoint("workqueue_execute_start", "work=0xa", 10),
			typed: PairingEndpointTypedInput{Name: "workqueue_execute_start", HeaderTID: 10, WorkAddress: 10, WorkAddressKnown: true},
		},
		{
			name: "dma integer and quoted text", wire: DecodePairingEndpoint("dma_fence_wait_start", `driver="gpu" timeline='render' context=7 seqno=9`, 10),
			typed: PairingEndpointTypedInput{Name: "dma_fence_wait_start", HeaderTID: 10, Driver: `"gpu"`, Timeline: `'render'`, ContextNumber: 7, ContextNumberKnown: true, SeqnoNumber: 9, SeqnoNumberKnown: true},
		},
		{
			name: "block numeric dev_t", wire: DecodePairingEndpoint("block_rq_issue", "8,0 R 4096 (cmd) 32 + 8 [worker]", 10),
			typed: PairingEndpointTypedInput{Name: "block_rq_issue", HeaderTID: 10, BlockIdentityKnown: true, BlockDeviceNumber: 8 << 20, BlockDeviceNumeric: true, BlockOperation: "R", BlockSector: 32, BlockLength: 8},
		},
		{
			name: "storage packed decimal dev_t inode and op", wire: DecodePairingEndpoint("android_fs_dataread_start", "dev=08388608 ino=7 rw=R bytes=4096", 10),
			typed: PairingEndpointTypedInput{Name: "android_fs_dataread_start", HeaderTID: 10, StorageIdentityKnown: true, StorageDeviceNumber: 8 << 20, StorageDeviceNumeric: true, StorageInodeNumber: 7, StorageInodeNumeric: true, StorageOperation: "R"},
		},
		{
			name: "storage hexadecimal inode with equal numeric evidence", wire: DecodePairingEndpoint("android_fs_dataread_start", "dev=8,0 ino=7 rw=R bytes=4096", 10),
			typed: PairingEndpointTypedInput{Name: "android_fs_dataread_start", HeaderTID: 10, StorageIdentityKnown: true, StorageDevice: "8,0", StorageInode: "0x7", StorageInodeNumber: 7, StorageInodeNumeric: true, StorageOperation: "R"},
		},
	}
	for _, tc := range tests {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			got := FingerprintPairingEndpoint(tc.typed)
			if !tc.wire.KeyKnown || !got.KeyKnown || got.Family != tc.wire.Family || got.Phase != tc.wire.Phase || got.SemanticKey != tc.wire.SemanticKey || got.EmitterAdmitted != tc.wire.EmitterAdmitted {
				t.Fatalf("numeric typed/wire mismatch: wire=%+v typed=%+v", tc.wire, got)
			}
		})
	}
}

func TestTypedPairingDualEvidenceConflictsFailClosed(t *testing.T) {
	t.Parallel()
	tests := []PairingEndpointTypedInput{
		{Name: "binder_transaction", HeaderTID: 10, Transaction: "7", TransactionNumber: 8, TransactionNumberKnown: true},
		{Name: "workqueue_execute_start", HeaderTID: 10, Work: "0x0a", WorkAddress: 11, WorkAddressKnown: true},
		{Name: "dma_fence_wait_start", HeaderTID: 10, Driver: "gpu", Timeline: "t", Context: "7", ContextNumber: 8, ContextNumberKnown: true, Seqno: "9"},
		{Name: "block_rq_issue", HeaderTID: 10, BlockIdentityKnown: true, BlockDevice: "8:0", BlockDeviceNumber: 9 << 20, BlockDeviceNumeric: true, BlockOperation: "R", BlockSector: 1, BlockLength: 1},
		{Name: "android_fs_dataread_start", HeaderTID: 10, StorageIdentityKnown: true, StorageDevice: "8:0", StorageDeviceNumber: 9 << 20, StorageDeviceNumeric: true, StorageInode: "7", StorageOperation: "R"},
		{Name: "android_fs_dataread_start", HeaderTID: 10, StorageIdentityKnown: true, StorageDevice: "8:0", StorageInode: "7", StorageInodeNumber: 8, StorageInodeNumeric: true, StorageOperation: "R"},
		{Name: "android_fs_dataread_start", HeaderTID: 10, StorageIdentityKnown: true, StorageDevice: "8:0", StorageInode: "0x7", StorageInodeNumber: 8, StorageInodeNumeric: true, StorageOperation: "R"},
	}
	for _, input := range tests {
		got := FingerprintPairingEndpoint(input)
		if !got.Recognized || got.KeyKnown {
			t.Errorf("conflicting typed representations minted key: input=%+v verdict=%+v", input, got)
		}
	}
	consistent := FingerprintPairingEndpoint(PairingEndpointTypedInput{
		Name: "android_fs_dataread_start", HeaderTID: 10, StorageIdentityKnown: true,
		StorageDevice: "08388608", StorageDeviceNumber: 8 << 20, StorageDeviceNumeric: true,
		StorageInode: "007", StorageInodeNumber: 7, StorageInodeNumeric: true, StorageOperation: "R",
	})
	if !consistent.KeyKnown {
		t.Fatalf("consistent packed/text typed evidence was rejected: %+v", consistent)
	}
	for _, input := range []PairingEndpointTypedInput{
		{Name: "binder_transaction", HeaderTID: 10, TransactionNumberKnown: true},
		{Name: "workqueue_execute_start", HeaderTID: 10, WorkAddressKnown: true},
		{Name: "block_rq_issue", HeaderTID: 10, BlockIdentityKnown: true, BlockDeviceNumber: 1 << 40, BlockDeviceNumeric: true, BlockOperation: "R", BlockSector: 1, BlockLength: 1},
		{Name: "android_fs_dataread_start", HeaderTID: 10, StorageIdentityKnown: true, StorageDevice: "4294967296", StorageInode: "7", StorageOperation: "R"},
	} {
		if got := FingerprintPairingEndpoint(input); !got.Recognized || got.KeyKnown {
			t.Errorf("zero/overflow typed scalar minted key: input=%+v verdict=%+v", input, got)
		}
	}
}

func TestTypedGenericStorageRecognitionAndIgnoredMetadata(t *testing.T) {
	t.Parallel()
	missing := FingerprintPairingEndpoint(PairingEndpointTypedInput{Name: "scsi_dispatch_cmd_start", HeaderTID: 10})
	unknownOwner := FingerprintPairingEndpoint(PairingEndpointTypedInput{Name: "scsi_dispatch_cmd_start", HeaderTID: -1, StorageIdentityKnown: true, StorageDevice: "ufs0"})
	if !missing.Recognized || missing.Family != PairingEndpointStorage || missing.KeyKnown || !unknownOwner.Recognized || unknownOwner.KeyKnown || unknownOwner.EmitterKnown {
		t.Fatalf("generic storage recognized/unkeyable contract drifted: missing=%+v owner=%+v", missing, unknownOwner)
	}
	wireNamed := DecodePairingEndpoint("ufshcd_command_start", "dev=ufs0 tag=1 lba=2 len=3 opcode=READ_10", 10)
	typedNamed := FingerprintPairingEndpoint(PairingEndpointTypedInput{Name: "ufshcd_command_start", HeaderTID: 10, StorageIdentityKnown: true, StorageDevice: "ufs0", StorageInode: "bad", StorageInodeNumber: 7, StorageInodeNumeric: true, StorageOperation: "WRITE"})
	if !wireNamed.KeyKnown || !typedNamed.KeyKnown || wireNamed.SemanticKey != typedNamed.SemanticKey {
		t.Fatalf("text device or ignored SCSI/UFS metadata changed hard key: wire=%+v typed=%+v", wireNamed, typedNamed)
	}
	wireUnknown := DecodePairingEndpoint("ufshcd_command_start", "tag=1 lba=2 len=3 opcode=READ_10", 10)
	typedUnknown := FingerprintPairingEndpoint(PairingEndpointTypedInput{Name: "ufshcd_command_start", HeaderTID: 10, StorageIdentityKnown: true})
	if !wireUnknown.KeyKnown || !typedUnknown.KeyKnown || wireUnknown.SemanticKey != typedUnknown.SemanticKey {
		t.Fatalf("generic unknown-device parity drifted: wire=%+v typed=%+v", wireUnknown, typedUnknown)
	}
}

func TestBinderPairingAliasAuthorityRejectsDuplicatesAndConflicts(t *testing.T) {
	t.Parallel()
	start := DecodePairingEndpoint("binder_transaction", "debug_id=42", 10)
	done := DecodePairingEndpoint("binder_transaction_received", "transaction_id=42", 20)
	if !start.KeyKnown || !done.KeyKnown || start.SemanticKey != done.SemanticKey {
		t.Fatalf("legal Binder aliases did not canonicalize: start=%+v done=%+v", start, done)
	}
	for _, fields := range []string{
		"transaction=42 transaction=42",
		"transaction=42 debug_id=42",
		"transaction=42 debug_id=43",
	} {
		got := DecodePairingEndpoint("binder_transaction_received", fields, 20)
		if !got.Recognized || got.KeyKnown {
			t.Errorf("duplicate/conflicting Binder alias minted key: fields=%q verdict=%+v", fields, got)
		}
	}
}

func TestPairingHardScalarsRejectDuplicateAndUnbalancedWire(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name   string
		event  string
		fields string
	}{
		{name: "work duplicate", event: "workqueue_execute_start", fields: "work=0x1 work=0x2"},
		{name: "work unbalanced", event: "workqueue_execute_start", fields: `work='0x1`},
		{name: "work positional unbalanced", event: "workqueue_execute_start", fields: `work struct '0x1`},
		{name: "dma duplicate", event: "dma_fence_wait_start", fields: "driver=gpu timeline=t context=1 context=2 seqno=3"},
		{name: "dma unbalanced", event: "dma_fence_wait_start", fields: `driver="gpu timeline=t context=1 seqno=3`},
		{name: "storage duplicate dev", event: "scsi_dispatch_cmd_start", fields: "dev=12,80 dev=13,0 op=read bytes=4096"},
		{name: "storage conflicting inode aliases", event: "android_fs_dataread_start", fields: "dev=8,0 ino=7 inode=8 bytes=4096 rw=R"},
		{name: "storage unbalanced dev", event: "scsi_dispatch_cmd_start", fields: `dev="12,80 op=read bytes=4096`},
		{name: "binder quoted metadata injection", event: "binder_transaction", fields: `note="x transaction=42 y"`},
		{name: "work quoted metadata injection", event: "workqueue_execute_start", fields: `function="x work=0x0000000a y"`},
		{name: "dma quoted metadata injection", event: "dma_fence_wait_start", fields: `driver=gpu timeline=t seqno=3 note="x context=7 y"`},
		{name: "storage quoted metadata injection", event: "scsi_dispatch_cmd_start", fields: `tag=1 lba=2 len=3 opcode=READ_10 note="x dev=12,80 y"`},
	}
	for _, tc := range tests {
		got := DecodePairingEndpoint(tc.event, tc.fields, 10)
		if !got.Recognized || got.KeyKnown {
			t.Errorf("malformed hard scalar minted key: %s verdict=%+v", tc.name, got)
		}
	}
}

func TestPairingHardScalarsIgnoreQuotedMetadataWhenExactFieldsExist(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name   string
		event  string
		fields string
	}{
		{name: "binder", event: "binder_transaction", fields: `transaction=42 note="x transaction=99 y"`},
		{name: "workqueue", event: "workqueue_execute_start", fields: `work=0x0000000a function="x work=0x0000000b y"`},
		{name: "dma", event: "dma_fence_wait_start", fields: `driver=gpu timeline=t context=7 seqno=3 note="x context=8 y"`},
		{name: "storage", event: "scsi_dispatch_cmd_start", fields: `tag=1 dev=12,80 lba=2 len=3 opcode=READ_10 note="x dev=13,0 y"`},
	} {
		got := DecodePairingEndpoint(tc.event, tc.fields, 10)
		if !got.KeyKnown || !got.PayloadAdmitted {
			t.Errorf("quoted non-authority metadata suppressed exact endpoint %s: %+v", tc.name, got)
		}
	}
}

func TestWorkqueueDMAExactHelpersShareStrictFingerprintFields(t *testing.T) {
	t.Parallel()
	work := Event{Name: "workqueue_execute_start", PID: 10, Type: EventWorkqueue, FieldText: `work=0x0000000a function="x work=0x0000000b y"`}
	workValue, function := workqueueExactEndpointFields(work)
	workVerdict := fingerprintPairingEvent(work)
	if workValue != "0x0000000a" || function != "" || !workVerdict.KeyKnown || !workVerdict.PayloadAdmitted {
		t.Fatalf("workqueue helper/fingerprint strict fields diverged: work=%q function=%q verdict=%+v", workValue, function, workVerdict)
	}
	work.FieldText = `function="x work=0x0000000a y"`
	workValue, _ = workqueueExactEndpointFields(work)
	if verdict := fingerprintPairingEvent(work); workValue != "" || verdict.KeyKnown {
		t.Fatalf("workqueue helper or fingerprint stole quoted work: work=%q verdict=%+v", workValue, verdict)
	}

	dma := Event{Name: "dma_fence_wait_start", PID: 20, Type: EventDMAFence, FieldText: `driver=gpu timeline=t context=7 seqno=3 note="x context=8 y"`}
	driver, timeline, context, seqno := dmaFenceExactEndpointFields(dma)
	dmaVerdict := fingerprintPairingEvent(dma)
	if driver != "gpu" || timeline != "t" || context != "7" || seqno != "3" || !dmaVerdict.KeyKnown || !dmaVerdict.PayloadAdmitted {
		t.Fatalf("DMA helper/fingerprint strict fields diverged: fields=%q/%q/%q/%q verdict=%+v", driver, timeline, context, seqno, dmaVerdict)
	}
	dma.FieldText = `driver=gpu timeline=t seqno=3 note="x context=7 y"`
	_, _, context, _ = dmaFenceExactEndpointFields(dma)
	if verdict := fingerprintPairingEvent(dma); context != "" || verdict.KeyKnown {
		t.Fatalf("DMA helper or fingerprint stole quoted context: context=%q verdict=%+v", context, verdict)
	}
}

func TestGenericStorageWireProfilesSeparateKeyFromAdmission(t *testing.T) {
	t.Parallel()
	for _, event := range []string{"scsi_dispatch_cmd_start", "mmc_request_start", "android_fs_dataread_start", "f2fs_direct_IO_enter"} {
		got := DecodePairingEndpoint(event, "malformed", 10)
		if !got.Recognized || got.KeyKnown || got.PayloadAdmitted {
			t.Errorf("empty malformed profile was admitted: event=%s verdict=%+v", event, got)
		}
	}
	ufsUnknown := DecodePairingEndpoint("ufshcd_command_start", "malformed", 10)
	if !ufsUnknown.Recognized || !ufsUnknown.KeyKnown || ufsUnknown.PayloadAdmitted {
		t.Fatalf("UFS unknown-dev coarse lane did not separate key from malformed payload: %+v", ufsUnknown)
	}
	tests := []struct {
		name        string
		event       string
		fields      string
		wantKey     bool
		wantPayload bool
	}{
		{name: "scsi legacy complete", event: "scsi_dispatch_cmd_start", fields: "dev=12,80 op=read bytes=4096", wantKey: true, wantPayload: true},
		{name: "scsi signed tag", event: "scsi_dispatch_cmd_start", fields: "tag=-1 dev=12,80 lba=1 len=4096 opcode=READ_10", wantKey: true, wantPayload: true},
		{name: "scsi tag overflow", event: "scsi_dispatch_cmd_start", fields: "tag=-9223372036854775809 dev=12,80 lba=1 len=4096 opcode=READ_10", wantKey: true},
		{name: "scsi missing bytes", event: "scsi_dispatch_cmd_start", fields: "dev=12,80 op=read", wantKey: true},
		{name: "scsi malformed nonkey scalar", event: "scsi_dispatch_cmd_start", fields: "tag=bad dev=12,80 lba=1 len=4096 opcode=READ_10", wantKey: true},
		{name: "scsi bad hard dev", event: "scsi_dispatch_cmd_start", fields: "tag=1 dev=bad lba=1 len=4096 opcode=READ_10"},
		{name: "mmc positional full", event: "mmc_request_start", fields: "mmc0 tag=1 opcode=17 blocks=8 block_size=512 blk_addr=10", wantKey: true, wantPayload: true},
		{name: "mmc positional missing mandatory", event: "mmc_request_start", fields: "mmc0 opcode=17", wantKey: true},
		{name: "mmc unregistered kv compatibility is inventory only", event: "mmc_request_start", fields: "dev=mmcblk0 op=read"},
		{name: "ufs named device", event: "ufshcd_command_start", fields: "dev=ufs0 tag=1 opcode=READ_10", wantKey: true, wantPayload: true},
		{name: "ufs absent device", event: "ufshcd_command_start", fields: "tag=1 opcode=READ_10", wantKey: true, wantPayload: true},
		{name: "ufs malformed nonkey tag", event: "ufshcd_command_start", fields: "tag=bad opcode=READ_10", wantKey: true},
		{name: "ufs empty nonkey opcode", event: "ufshcd_command_start", fields: "tag=1 opcode=", wantKey: true},
		{name: "ufs rejects signed tag", event: "ufshcd_command_start", fields: "dev=ufs0 tag=-1 opcode=READ_10", wantKey: true},
		{name: "ufs missing opcode", event: "ufshcd_command_start", fields: "dev=ufs0 tag=1", wantKey: true},
		{name: "ufs explicit empty device", event: "ufshcd_command_start", fields: "tag=1 opcode=READ_10 dev="},
		{name: "ufs explicit spaced empty device", event: "ufshcd_command_start", fields: "tag=1 opcode=READ_10 dev ="},
		{name: "ufs unbalanced device", event: "ufshcd_command_start", fields: `dev="ufs0 tag=1 opcode=READ_10`},
		{name: "android full", event: "android_fs_dataread_start", fields: "dev=8,0 ino=7 bytes=4096 rw=R", wantKey: true, wantPayload: true},
		{name: "android missing bytes", event: "android_fs_dataread_start", fields: "dev=8,0 ino=7 rw=R", wantKey: true},
		{name: "android bad inode", event: "android_fs_dataread_start", fields: "dev=8,0 ino=bad bytes=4096 rw=R"},
		{name: "f2fs compatibility alias rejected", event: "f2fs_direct_IO_enter", fields: "fs_dev=8,0 ino=7 len=4096 rw=R"},
		{name: "f2fs missing inode", event: "f2fs_direct_IO_enter", fields: "dev=8,0 len=4096 rw=R"},
	}
	for _, tc := range tests {
		got := DecodePairingEndpoint(tc.event, tc.fields, 10)
		if !got.Recognized || got.KeyKnown != tc.wantKey || got.PayloadAdmitted != tc.wantPayload {
			t.Errorf("profile verdict mismatch %s: got=%+v want key=%t payload=%t", tc.name, got, tc.wantKey, tc.wantPayload)
		}
	}
}

func TestUFSInvalidDeviceCannotBridgeAbsentDeviceLane(t *testing.T) {
	t.Parallel()
	validDone := DecodePairingEndpoint("ufshcd_command_done", "tag=1 opcode=READ_10", 40)
	if !validDone.KeyKnown || !validDone.PayloadAdmitted {
		t.Fatalf("legal absent-device UFS endpoint was not admitted: %+v", validDone)
	}
	for _, tc := range []struct {
		name   string
		fields string
	}{
		{name: "empty", fields: "tag=1 opcode=READ_10 dev="},
		{name: "spaced empty", fields: "tag=1 opcode=READ_10 dev ="},
		{name: "unbalanced", fields: `dev="ufs0 tag=1 opcode=READ_10`},
	} {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			invalidStart := DecodePairingEndpoint("ufshcd_command_start", tc.fields, 40)
			if !invalidStart.Recognized || invalidStart.KeyKnown || invalidStart.PayloadAdmitted || invalidStart.SemanticKey != "" {
				t.Fatalf("invalid UFS dev acquired the absent-device lane: %+v", invalidStart)
			}

			idx := &Index{
				Path: "/trace/ufs-invalid-device.systrace", TimestampOrder: TraceTimestampOrderMonotonic, LineCount: 2,
				Events: []Event{
					{Line: 1, Ts: 1, PID: 40, Type: EventStorage, Name: "ufshcd_command_start", FieldText: tc.fields},
					{Line: 2, Ts: 1.002, PID: 40, Type: EventStorage, Name: "ufshcd_command_done", FieldText: "tag=1 opcode=READ_10"},
				},
			}
			rows, caveats := computeStorageLatencyByLayer(idx, Query{TimeStart: .9, TimeEnd: 1.1}, nil, 8)
			if row := storageLatencyRow(rows, "ufs", "ufshcd_command"); row != nil && row.PairedCount != 0 {
				t.Fatalf("invalid UFS start bridged a legal absent-device done: row=%+v caveats=%v", row, caveats)
			}
			if !containsSubstring(caveats, "duration_pairing") {
				t.Fatalf("invalid UFS key was suppressed without integrity disclosure: rows=%+v caveats=%v", rows, caveats)
			}
		})
	}
}

func TestUFSEmptyNonKeyKeepsNamedLaneAndBlocksBridge(t *testing.T) {
	t.Parallel()
	const validFields = "dev=ufs0 tag=1 opcode=READ_10"
	for _, tc := range []struct {
		name   string
		fields string
	}{
		{name: "attached equals", fields: "opcode= dev=ufs0 tag=1"},
		{name: "spaced equals", fields: "opcode = dev=ufs0 tag=1"},
	} {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			start := DecodePairingEndpoint("ufshcd_command_start", validFields, 40)
			barrier := DecodePairingEndpoint("ufshcd_command_start", tc.fields, 40)
			done := DecodePairingEndpoint("ufshcd_command_done", validFields, 40)
			if !start.KeyKnown || !start.PayloadAdmitted || !done.KeyKnown || !done.PayloadAdmitted || start.SemanticKey != done.SemanticKey {
				t.Fatalf("valid named UFS endpoints did not establish one lane: start=%+v done=%+v", start, done)
			}
			if !barrier.KeyKnown || barrier.PayloadAdmitted || barrier.SemanticKey != start.SemanticKey {
				t.Fatalf("empty opcode hid the following named device or escaped exact-lane quarantine: start=%+v barrier=%+v", start, barrier)
			}

			idx := &Index{
				Path: "/trace/ufs-empty-opcode.systrace", TimestampOrder: TraceTimestampOrderMonotonic, LineCount: 3,
				Events: []Event{
					{Line: 1, Ts: 1, PID: 40, Type: EventStorage, Name: "ufshcd_command_start", FieldText: validFields},
					{Line: 2, Ts: 1.001, PID: 40, Type: EventStorage, Name: "ufshcd_command_start", FieldText: tc.fields},
					{Line: 3, Ts: 1.002, PID: 40, Type: EventStorage, Name: "ufshcd_command_done", FieldText: validFields},
				},
			}
			rows, caveats := computeStorageLatencyByLayer(idx, Query{TimeStart: .9, TimeEnd: 1.1}, nil, 8)
			if row := storageLatencyRow(rows, "ufs", "ufshcd_command"); row != nil && row.PairedCount != 0 {
				t.Fatalf("malformed named UFS endpoint allowed surrounding rows to bridge: row=%+v caveats=%v", row, caveats)
			}
			if !containsSubstring(caveats, "duration_pairing_exact_lane_quarantined=true") {
				t.Fatalf("malformed named UFS endpoint did not disclose exact-lane quarantine: rows=%+v caveats=%v", rows, caveats)
			}
		})
	}
}

func TestGenericStorageDeviceAliasesShareCanonicalLane(t *testing.T) {
	t.Parallel()
	aliases := []string{
		"dev=8,0",
		"s_dev=8:0",
		"fs_dev=08388608",
		"dev_t=8,0",
		"sdev=8:0",
		"dev_name=08388608",
		"devname=8,0",
	}
	var canonical string
	for _, alias := range aliases {
		got := DecodePairingEndpoint("android_fs_dataread_start", alias+" ino=7 bytes=4096 rw=R", 10)
		if !got.KeyKnown || !got.PayloadAdmitted {
			t.Fatalf("standard device alias was not admitted: alias=%q verdict=%+v", alias, got)
		}
		if canonical == "" {
			canonical = got.SemanticKey
		} else if got.SemanticKey != canonical {
			t.Fatalf("equivalent device aliases split one physical lane: alias=%q key=%q canonical=%q", alias, got.SemanticKey, canonical)
		}
	}
}

func TestOfficialExt4SpaceLabeledProfilesPairEndToEnd(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name, startEvent, startBody, doneEvent, doneBody string
	}{
		{
			name: "da_write", startEvent: "ext4_da_write_begin", startBody: "dev 8,0 ino 7 pos 0 len 4096 flags 0",
			doneEvent: "ext4_da_write_end", doneBody: "dev 8,0 ino 0x7 pos 0 len 4096 copied 4096",
		},
		{
			name: "sync_file", startEvent: "ext4_sync_file_enter", startBody: "dev 8,0 ino 7 parent 2 datasync 1",
			doneEvent: "ext4_sync_file_exit", doneBody: "dev 8,0 ino 0x7 ret 0",
		},
	}
	for _, tc := range tests {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			start := DecodePairingEndpoint(tc.startEvent, tc.startBody, 40)
			done := DecodePairingEndpoint(tc.doneEvent, tc.doneBody, 40)
			if !start.KeyKnown || !start.PayloadAdmitted || !done.KeyKnown || !done.PayloadAdmitted || start.SemanticKey != done.SemanticKey {
				t.Fatalf("official ext4 space-labeled endpoints diverged: start=%+v done=%+v", start, done)
			}
			badTail := DecodePairingEndpoint(tc.startEvent, "dev 8,0 ino 7", 40)
			if !badTail.KeyKnown || badTail.PayloadAdmitted {
				t.Fatalf("truncated ext4 non-key payload did not quarantine the exact lane: %+v", badTail)
			}
			idx := buildTraceIndex(t, "ext4-space.systrace", "io-40 (40) [003] .... 1.000000: "+tc.startEvent+": "+tc.startBody+"\n"+
				"io-40 (40) [003] .... 1.002000: "+tc.doneEvent+": "+tc.doneBody+"\n")
			rows, caveats := computeStorageLatencyByLayer(idx, Query{TimeStart: .9, TimeEnd: 1.1}, nil, 8)
			row := storageLatencyRow(rows, "ext4", strings.TrimSuffix(strings.TrimSuffix(tc.startEvent, "_begin"), "_enter"))
			if row == nil || row.PairedCount != 1 || !near(row.MaxLatencyMs, 2, .001) {
				t.Fatalf("official ext4 space-labeled pair was not published once: rows=%+v caveats=%v", rows, caveats)
			}
		})
	}
}

func TestOfficialMMCSignedTagAndIndependentErrorsPairEndToEnd(t *testing.T) {
	t.Parallel()
	// Exact official_render.go envelope: positional device, phase prose and
	// request pointer precede the typed KV body; response words remain
	// non-authoritative inventory between independent error fields.
	startFields := "mmc0: start struct mmc_request[0x1]: cmd_opcode=17 cmd_arg=0x0 cmd_flags=0x0 cmd_retries=0 stop_opcode=0 stop_arg=0x0 stop_flags=0x0 stop_retries=0 sbc_opcode=0 sbc_arg=0x0 sbc_flags=0x0 sbc_retires=0 blocks=8 block_size=512 blk_addr=10 data_flags=0x0 tag=-1 can_retune=0 doing_retune=0 retune_now=0 need_retune=0 hold_retune=0 retune_period=0"
	doneFields := "mmc0: end struct mmc_request[0x1]: cmd_opcode=17 cmd_err=0 cmd_resp=0x0 0x0 0x0 0x0 cmd_retries=0 stop_opcode=0 stop_err=0 stop_resp=0x0 0x0 0x0 0x0 stop_retries=0 sbc_opcode=0 sbc_err=0 sbc_resp=0x0 0x0 0x0 0x0 sbc_retries=0 bytes_xfered=4096 data_err=0 tag=-1 can_retune=0 doing_retune=0 retune_now=0 need_retune=0 hold_retune=0 retune_period=0"
	start := DecodePairingEndpoint("mmc_request_start", startFields, 40)
	done := DecodePairingEndpoint("mmc_request_done", doneFields, 40)
	if !start.KeyKnown || !start.PayloadAdmitted || !done.KeyKnown || !done.PayloadAdmitted || start.SemanticKey != done.SemanticKey {
		t.Fatalf("official MMC signed-tag endpoints did not share one admitted lane: start=%+v done=%+v", start, done)
	}
	overflow := DecodePairingEndpoint("mmc_request_start", "mmc0: start struct mmc_request[0x1]: cmd_opcode=17 blocks=8 block_size=512 blk_addr=10 tag=-9223372036854775809", 40)
	if !overflow.KeyKnown || overflow.PayloadAdmitted {
		t.Fatalf("overflowed non-key MMC tag did not quarantine its exact coarse lane: %+v", overflow)
	}
	idx := buildTraceIndex(t, "mmc-signed-tag.systrace", "io-40 (40) [003] .... 1.000000: mmc_request_start: "+startFields+"\n"+
		"io-40 (40) [003] .... 1.002000: mmc_request_done: "+doneFields+"\n")
	stats := ComputeWindowStats(idx, Query{TimeStart: .9, TimeEnd: 1.1})
	var row *StorageLatencySummary
	for i := range stats.StorageLatencyByLayer {
		if stats.StorageLatencyByLayer[i].Layer == "mmc" {
			row = &stats.StorageLatencyByLayer[i]
			break
		}
	}
	if row == nil || row.PairedCount != 1 || !near(row.MaxLatencyMs, 2, .001) {
		t.Fatalf("official MMC signed-tag pair was not published once: rows=%+v caveats=%v", stats.StorageLatencyByLayer, stats.Caveats)
	}
}

func TestBlockLocatedButRejectedPayloadDoesNotBecomeEndpoint(t *testing.T) {
	t.Parallel()
	zeroRead := DecodePairingEndpoint("block_rq_issue", "8,0 R 0 () 1 + 0 [io]", 10)
	badBytes := DecodePairingEndpoint("block_rq_issue", "8,0 R 4294967296 () 1 + 1 [io]", 10)
	badError := DecodePairingEndpoint("block_rq_complete", "8,0 R () 1 + 1 [2147483648]", 10)
	for name, got := range map[string]PairingEndpointVerdict{"zero_read": zeroRead, "bad_bytes": badBytes, "bad_error": badError} {
		if !got.Recognized || !got.KeyKnown || got.PayloadAdmitted {
			t.Errorf("located rejected Block payload contract drifted %s: %+v", name, got)
		}
	}
	emptyTyped := integrityBlockEvent(1, 1, 10, "block_rq_issue", 10)
	emptyTyped.FieldText = ""
	emptyTyped.BlockIOFields.IdentityParsed = true
	emptyTyped.BlockIOFields.IdentityValid = false
	verdict := fingerprintPairingEvent(emptyTyped)
	if !verdict.KeyKnown || verdict.PayloadAdmitted {
		t.Fatalf("empty-payload invalid typed flag was rescued: %+v", verdict)
	}
}

func TestTypedPayloadAdmissionFlagsStayInTheirOwnFamily(t *testing.T) {
	t.Parallel()
	dma := FingerprintPairingEndpoint(PairingEndpointTypedInput{
		Name: "dma_fence_wait_start", HeaderTID: 10, Driver: "gpu", Timeline: "t", Context: "1", Seqno: "2",
		StoragePayloadAdmissionKnown: true, StoragePayloadAdmitted: false,
	})
	storage := FingerprintPairingEndpoint(PairingEndpointTypedInput{
		Name: "scsi_dispatch_cmd_start", HeaderTID: 10, StorageIdentityKnown: true, StorageDevice: "12,80",
		StoragePayloadAdmissionKnown: true, StoragePayloadAdmitted: false,
	})
	if !dma.KeyKnown || !dma.PayloadAdmitted || !storage.KeyKnown || storage.PayloadAdmitted {
		t.Fatalf("typed payload-admission family isolation drifted: dma=%+v storage=%+v", dma, storage)
	}
}

func TestTypedPairingFingerprintCanQuarantineMalformedNonKeyMetadata(t *testing.T) {
	t.Parallel()
	wire := DecodePairingEndpoint("block_rq_complete", "8,0 R (cmd) 32 + 8 [bad-error]", 10)
	typed := FingerprintPairingEndpoint(PairingEndpointTypedInput{
		Name: "block_rq_complete", HeaderTID: 10,
		BlockIdentityKnown: true, BlockDevice: "8,0", BlockOperation: "R", BlockSector: 32, BlockLength: 8,
	})
	if !wire.Recognized || wire.KeyKnown || !typed.KeyKnown || !typed.EmitterAdmitted {
		t.Fatalf("typed key could not isolate malformed non-key metadata: wire=%+v typed=%+v", wire, typed)
	}
}

func TestRetainedEventAdapterNeverRescuesMalformedWireFromTypedFields(t *testing.T) {
	t.Parallel()
	idx := &Index{Path: "/trace/no-rescue.systrace"}
	block := integrityBlockEvent(1, 1, 10, "block_rq_issue", 10)
	block.FieldText = "malformed"
	if verdict := fingerprintPairingEvent(block); !verdict.Recognized || verdict.KeyKnown {
		t.Fatalf("malformed named Block wire was rescued: %+v", verdict)
	}
	if key, _, ok := blockPairingKey(idx, block); ok || key != "" || len(durationOrderObservations(block)) != 0 {
		t.Fatalf("Block consumer bypassed retained-event authority: key=%q ok=%t observations=%+v", key, ok, durationOrderObservations(block))
	}
	storage := integrityStorageEvent(2, 1, 20, "scsi_dispatch_cmd_start", "12,80")
	storage.FieldText = "malformed"
	if verdict := fingerprintPairingEvent(storage); !verdict.Recognized || verdict.KeyKnown {
		t.Fatalf("malformed named Storage wire was rescued: %+v", verdict)
	}
	if key, _, _, _, ok := genericStoragePairingKey(idx, storage); ok || key != "" || len(durationOrderObservations(storage)) != 0 {
		t.Fatalf("Storage consumer bypassed retained-event authority: key=%q ok=%t observations=%+v", key, ok, durationOrderObservations(storage))
	}
}

func TestRetainedEventAdapterKeepsEmptyPayloadTypedCompatibilityUnified(t *testing.T) {
	t.Parallel()
	idx := &Index{Path: "/trace/compat.systrace"}
	block := integrityBlockEvent(1, 1, 10, "block_rq_issue", 10)
	block.FieldText = ""
	blockVerdict := fingerprintPairingEvent(block)
	blockKey, _, blockOK := blockPairingKey(idx, block)
	wantBlockKey, wantBlockOK := blockVerdict.LaneKey(idx.Path)
	if !blockVerdict.KeyKnown || !blockOK || !wantBlockOK || blockKey != wantBlockKey || len(durationOrderObservations(block)) != 1 {
		t.Fatalf("empty-payload Block compatibility diverged: verdict=%+v key=%q want=%q observations=%+v", blockVerdict, blockKey, wantBlockKey, durationOrderObservations(block))
	}
	storage := integrityStorageEvent(2, 1, 20, "scsi_dispatch_cmd_start", "12,80")
	storage.FieldText = ""
	storageVerdict := fingerprintPairingEvent(storage)
	storageKey, _, _, _, storageOK := genericStoragePairingKey(idx, storage)
	wantStorageKey, wantStorageOK := storageVerdict.LaneKey(idx.Path)
	if !storageVerdict.KeyKnown || !storageOK || !wantStorageOK || storageKey != wantStorageKey || len(durationOrderObservations(storage)) != 1 {
		t.Fatalf("empty-payload Storage compatibility diverged: verdict=%+v key=%q want=%q observations=%+v", storageVerdict, storageKey, wantStorageKey, durationOrderObservations(storage))
	}
}

func TestGenericStorageFingerprintDoesNotStealUnprovenRequestTokens(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name   string
		eventA string
		bodyA  string
		eventB string
		bodyB  string
	}{
		{
			name:   "scsi tag lba len remain metadata",
			eventA: "scsi_dispatch_cmd_start", bodyA: "tag=1 dev=12,80 lba=100 len=4096 opcode=read",
			eventB: "scsi_dispatch_cmd_done", bodyB: "tag=999 dev=12:80 lba=900 len=8192 opcode=write",
		},
		{
			name:   "mmc tag opcode remain metadata",
			eventA: "mmc_request_start", bodyA: "mmc0 tag=1 opcode=17 blocks=8 block_size=512 blk_addr=10",
			eventB: "mmc_request_done", bodyB: "mmc0 tag=999 opcode=24 bytes_xfered=4096 ret=0",
		},
	}
	for _, tc := range tests {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			a := DecodePairingEndpoint(tc.eventA, tc.bodyA, 40)
			b := DecodePairingEndpoint(tc.eventB, tc.bodyB, 40)
			if !a.KeyKnown || !b.KeyKnown || a.Family != PairingEndpointStorage || b.Family != PairingEndpointStorage || a.SemanticKey != b.SemanticKey {
				t.Fatalf("unproven request token changed coarse key: a=%+v b=%+v", a, b)
			}
		})
	}
}

func TestPairingConsumersUseCanonicalWorkqueueAndDMAKeys(t *testing.T) {
	t.Parallel()
	idx := &Index{
		Path: "/trace/canonical-duration.systrace", TimestampOrder: TraceTimestampOrderMonotonic,
		Events: []Event{
			{Line: 1, Ts: 1.000, Type: EventWorkqueue, Name: "workqueue_execute_start", PID: 10, FieldText: "work=0x0A function=0x111"},
			{Line: 2, Ts: 1.002, Type: EventWorkqueue, Name: "workqueue_execute_end", PID: 10, FieldText: "work=0000000a function=0x222"},
			{Line: 3, Ts: 1.003, Type: EventDMAFence, Name: "dma_fence_wait_start", PID: 20, FieldText: "driver=gpu timeline=render context=0x7 seqno=007"},
			{Line: 4, Ts: 1.006, Type: EventDMAFence, Name: "dma_fence_wait_end", PID: 20, FieldText: "driver=gpu timeline=render context=7 seqno=0x7"},
		},
	}
	stats := ComputeWindowStats(idx, Query{TimeStart: .9, TimeEnd: 1.1})
	if len(stats.WorkqueueActivity) != 1 || stats.WorkqueueActivity[0].PairedCount != 1 || !near(stats.WorkqueueActivity[0].DurationMs, 2, .001) {
		t.Fatalf("workqueue canonical fingerprint not consumed: %+v", stats.WorkqueueActivity)
	}
	if len(stats.DMAFenceActivity) != 1 || stats.DMAFenceActivity[0].PairedCount != 1 || !near(stats.DMAFenceActivity[0].WaitMs, 3, .001) {
		t.Fatalf("DMA canonical fingerprint not consumed: %+v", stats.DMAFenceActivity)
	}
}

func TestPairingConsumersRejectIdleWorkqueueAndDMAEndpoints(t *testing.T) {
	t.Parallel()
	idx := &Index{
		Path: "/trace/idle-duration.systrace", TimestampOrder: TraceTimestampOrderMonotonic,
		Events: []Event{
			{Line: 1, Ts: 1.000, Type: EventWorkqueue, Name: "workqueue_execute_start", PID: 0, FieldText: "work=0x1"},
			{Line: 2, Ts: 1.002, Type: EventWorkqueue, Name: "workqueue_execute_end", PID: 0, FieldText: "work=0x1"},
			{Line: 3, Ts: 1.003, Type: EventDMAFence, Name: "dma_fence_wait_start", PID: 0, FieldText: "driver=gpu timeline=render context=1 seqno=2"},
			{Line: 4, Ts: 1.006, Type: EventDMAFence, Name: "dma_fence_wait_end", PID: 0, FieldText: "driver=gpu timeline=render context=1 seqno=2"},
			{Line: 5, Ts: 1.007, Type: EventWorkqueue, Name: "workqueue_execute_start", PID: 10, FieldText: "work=0x2"},
			{Line: 6, Ts: 1.009, Type: EventWorkqueue, Name: "workqueue_execute_end", PID: 10, FieldText: "work=0x2"},
			{Line: 7, Ts: 1.010, Type: EventDMAFence, Name: "dma_fence_wait_start", PID: 20, FieldText: "driver=gpu timeline=render context=3 seqno=4"},
			{Line: 8, Ts: 1.013, Type: EventDMAFence, Name: "dma_fence_wait_end", PID: 20, FieldText: "driver=gpu timeline=render context=3 seqno=4"},
		},
	}
	stats := ComputeWindowStats(idx, Query{TimeStart: .9, TimeEnd: 1.1})
	if len(stats.WorkqueueActivity) != 1 || stats.WorkqueueActivity[0].Thread.PID != 10 || stats.WorkqueueActivity[0].PairedCount != 1 {
		t.Fatalf("idle workqueue lane erased or minted sibling duration: %+v", stats.WorkqueueActivity)
	}
	if len(stats.DMAFenceActivity) != 1 || stats.DMAFenceActivity[0].Thread.PID != 20 || stats.DMAFenceActivity[0].PairedCount != 1 {
		t.Fatalf("idle DMA lane erased or minted sibling duration: %+v", stats.DMAFenceActivity)
	}
	if !containsSubstring(stats.Caveats, "duration_pairing_exact_lane_quarantined=true") {
		t.Fatalf("idle consumer rejection was not disclosed: %+v", stats.Caveats)
	}
}

func TestGenericStorageLifecycleResetIsPhysicalSourceScoped(t *testing.T) {
	t.Parallel()
	storage := func(line int, ts float64, name string) Event {
		return Event{
			Line: line, Ts: ts, Type: EventStorage, Name: name, PID: 40,
			FieldText:      "dev=12,80 op=read bytes=4096",
			ResourceFields: &ResourceFields{Op: "read", Bytes: 4096},
			FileFields:     &FileFields{Dev: "12,80", RW: "read", Len: 4096},
		}
	}
	idx := &Index{
		Path: "/trace/bundle.tracebundle.json", TimestampOrder: TraceTimestampOrderMonotonic,
		TraceArtifacts: []TraceArtifactSource{
			{SourcePath: "/trace/a.systrace", LocalLineCount: 50, VirtualLineBase: 0, CausalCompatible: true},
			{SourcePath: "/trace/b.systrace", LocalLineCount: 50, VirtualLineBase: 100, CausalCompatible: true},
		},
		Events: []Event{
			storage(101, 1.000, "scsi_dispatch_cmd_start"),
			{Line: 1, Ts: 1.001, Type: EventSchedSwitch, PrevPID: 40, PrevState: "X"},
			storage(102, 1.003, "scsi_dispatch_cmd_done"),
		},
	}
	rows, caveats := computeStorageLatencyByLayer(idx, Query{TimeStart: .9, TimeEnd: 1.1}, nil, 8)
	row := storageLatencyRow(rows, "scsi", "scsi_dispatch_cmd")
	if row == nil || row.PairedCount != 1 || !near(row.MaxLatencyMs, 3, .001) {
		t.Fatalf("source-A lifecycle event cleared source-B storage lane: rows=%+v caveats=%+v", rows, caveats)
	}
}

func TestGenericStorageLifecycleResetUsesOwnerInverseIndex(t *testing.T) {
	t.Parallel()
	fset := token.NewFileSet()
	file, err := parser.ParseFile(fset, "query.go", nil, 0)
	if err != nil {
		t.Fatal(err)
	}
	var target *ast.FuncDecl
	for _, decl := range file.Decls {
		fn, ok := decl.(*ast.FuncDecl)
		if ok && fn.Name.Name == "computeStorageLatencyByLayer" {
			target = fn
			break
		}
	}
	if target == nil {
		t.Fatal("computeStorageLatencyByLayer not found")
	}
	callName := func(call *ast.CallExpr) string {
		if ident, ok := call.Fun.(*ast.Ident); ok {
			return ident.Name
		}
		return ""
	}
	hasAdd, hasDrop, resetUsesOwnerIndex := false, false, false
	ast.Inspect(target.Body, func(node ast.Node) bool {
		if call, ok := node.(*ast.CallExpr); ok {
			switch callName(call) {
			case "addDurationPairingReplayLane":
				hasAdd = true
			case "dropDurationPairingReplayLane":
				hasDrop = true
			}
		}
		ifStmt, ok := node.(*ast.IfStmt)
		if !ok || ifStmt.Init == nil {
			return true
		}
		isLifecycleReset := false
		ast.Inspect(ifStmt.Init, func(initNode ast.Node) bool {
			if call, ok := initNode.(*ast.CallExpr); ok && callName(call) == "schedulerLifecycleResetPID" {
				isLifecycleReset = true
			}
			return true
		})
		if isLifecycleReset {
			ast.Inspect(ifStmt.Body, func(bodyNode ast.Node) bool {
				if call, ok := bodyNode.(*ast.CallExpr); ok && callName(call) == "durationPairingReplayOwnerLaneKeys" {
					resetUsesOwnerIndex = true
				}
				return true
			})
		}
		return true
	})
	if !hasAdd || !hasDrop || !resetUsesOwnerIndex {
		t.Fatalf("storage lifecycle replay lost bounded owner index: add=%t drop=%t reset_lookup=%t", hasAdd, hasDrop, resetUsesOwnerIndex)
	}
}

func TestPairingConsumersUseSingleFingerprintAuthority(t *testing.T) {
	t.Parallel()
	targets := map[string][]string{
		"pairing_endpoint.go":        {"DecodePairingEndpoint", "decodePairingEndpointWire", "FingerprintPairingEndpoint", "fingerprintPairingEvent"},
		"binder_pairing.go":          {"binderEndpointVerdictForEvent", "auditBinderPairingWithBudget"},
		"block_pairing.go":           {"blockPairingKey", "blockLatencyEndpoint"},
		"storage_pairing.go":         {"genericStoragePairingKey", "decodeGenericStoragePairingEvent"},
		"duration_order.go":          {"durationOrderObservations"},
		"query.go":                   {"computeWorkqueueActivity", "computeDMAFenceActivity", "workqueueBaseAndPhase", "dmaFenceBaseAndPhase"},
		"window_discovery.go":        {"decode"},
		"workqueue_dma_integrity.go": {"durationExactEndpointFamily"},
	}
	profileOnly := map[string]bool{
		"blockLatencyEndpoint":        true,
		"workqueueBaseAndPhase":       true,
		"dmaFenceBaseAndPhase":        true,
		"durationExactEndpointFamily": true,
	}
	coreAuthority := map[string]bool{
		"DecodePairingEndpoint":      true,
		"decodePairingEndpointWire":  true,
		"FingerprintPairingEndpoint": true,
		"fingerprintPairingEvent":    true,
	}
	delegatedAuthority := map[string]string{
		"DecodePairingEndpoint":     "decodePairingEndpointWire",
		"decodePairingEndpointWire": "FingerprintPairingEndpoint",
		"genericStoragePairingKey":  "decodeGenericStoragePairingEvent",
	}
	payloadRequired := map[string]bool{
		"auditBinderPairingWithBudget":     true,
		"blockPairingKey":                  true,
		"decodeGenericStoragePairingEvent": true,
		"durationOrderObservations":        true,
		"computeWorkqueueActivity":         true,
		"computeDMAFenceActivity":          true,
		"decode":                           true,
	}
	for filename, functions := range targets {
		file, err := parser.ParseFile(token.NewFileSet(), filename, nil, 0)
		if err != nil {
			t.Fatalf("parse %s: %v", filename, err)
		}
		for _, function := range functions {
			foundFunction := false
			requiresProfile := (coreAuthority[function] || profileOnly[function]) && delegatedAuthority[function] == ""
			foundAuthority := coreAuthority[function] || profileOnly[function]
			foundProfile := !requiresProfile
			foundDelegate := delegatedAuthority[function] == ""
			foundEmitterGate := function != "computeWorkqueueActivity" && function != "computeDMAFenceActivity"
			foundPayloadGate := !payloadRequired[function]
			manualKeyFallback := false
			for _, declaration := range file.Decls {
				fn, ok := declaration.(*ast.FuncDecl)
				if !ok || fn.Name.Name != function {
					continue
				}
				foundFunction = true
				ast.Inspect(fn.Body, func(node ast.Node) bool {
					call, ok := node.(*ast.CallExpr)
					if !ok {
						return true
					}
					if ident, ok := call.Fun.(*ast.Ident); ok && delegatedAuthority[function] == ident.Name {
						foundDelegate = true
						foundAuthority = true
					}
					if ident, ok := call.Fun.(*ast.Ident); ok && ident.Name == "DecodePairingEndpoint" {
						foundAuthority = true
					}
					if ident, ok := call.Fun.(*ast.Ident); ok && ident.Name == "fingerprintPairingEvent" {
						foundAuthority = true
					}
					if ident, ok := call.Fun.(*ast.Ident); ok && ident.Name == "binderEndpointVerdictForEvent" {
						foundAuthority = true
					}
					if ident, ok := call.Fun.(*ast.Ident); ok && ident.Name == "decodePairingEndpointWire" {
						foundAuthority = true
					}
					if ident, ok := call.Fun.(*ast.Ident); ok && ident.Name == "pairingEndpointProfileForName" {
						foundProfile = true
					}
					if ident, ok := call.Fun.(*ast.Ident); ok && ident.Name == "encodePairingKey" && (function == "blockPairingKey" || function == "genericStoragePairingKey") {
						manualKeyFallback = true
					}
					return true
				})
				ast.Inspect(fn.Body, func(node ast.Node) bool {
					selector, ok := node.(*ast.SelectorExpr)
					if ok && selector.Sel.Name == "EmitterAdmitted" {
						foundEmitterGate = true
					}
					if ok && selector.Sel.Name == "PayloadAdmitted" {
						foundPayloadGate = true
					}
					return true
				})
			}
			if !foundFunction || !foundAuthority || !foundProfile || !foundDelegate || !foundEmitterGate || !foundPayloadGate || manualKeyFallback {
				t.Errorf("%s:%s authority=%t profile=%t delegate=%t emitter_gate=%t payload_gate=%t manual_key_fallback=%t", filename, function, foundAuthority, foundProfile, foundDelegate, foundEmitterGate, foundPayloadGate, manualKeyFallback)
			}
		}
	}
}

func TestPairingWindowDiscoveryRejectsUnknownEndpointOwner(t *testing.T) {
	t.Parallel()
	discovery := newPairingWindowDiscovery(WindowDiscoveryRequest{
		Families: []WindowDiscoveryFamily{WindowDiscoveryFamilyBlock, WindowDiscoveryFamilyStorage},
	}, "/trace/source.systrace")
	block := integrityBlockEvent(1, 1, -1, "block_rq_issue", 10)
	storage := integrityStorageEvent(2, 1, -1, "scsi_dispatch_cmd_start", "12,80")
	for _, ev := range []Event{block, storage} {
		endpoint, recognized := discovery.decode(ev)
		if !recognized || endpoint.valid {
			t.Errorf("unknown emitter entered window discovery hard endpoint: event=%+v endpoint=%+v", ev, endpoint)
		}
	}
}
