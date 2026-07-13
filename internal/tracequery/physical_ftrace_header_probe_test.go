package tracequery

import (
	"math"
	"testing"
)

func TestProbePhysicalFtraceHeaderOwnerProvenance(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name       string
		line       string
		headerTID  int64
		ownerKnown bool
	}{
		{
			name:       "idle is a known physical owner",
			line:       "<idle>-0 [002] d..2 5.000000: block_rq_issue: 8,0 R 4096 () 32 + 8 []",
			headerTID:  0,
			ownerKnown: true,
		},
		{
			name:       "maximum signed pid",
			line:       "worker-2147483647 [002] d..2 5.000000: block_rq_issue: 8,0 R 4096 () 32 + 8 []",
			headerTID:  math.MaxInt32,
			ownerKnown: true,
		},
		{
			name:       "overflow is not idle",
			line:       "worker-2147483648 [002] d..2 5.000000: block_rq_issue: 8,0 R 4096 () 32 + 8 []",
			ownerKnown: false,
		},
		{
			name:       "malformed scalar is not idle",
			line:       "worker-bad [002] d..2 5.000000: block_rq_issue: 8,0 R 4096 () 32 + 8 []",
			ownerKnown: false,
		},
		{
			name:       "missing scalar is not idle",
			line:       "worker [002] d..2 5.000000: block_rq_issue: 8,0 R 4096 () 32 + 8 []",
			ownerKnown: false,
		},
	}
	for _, test := range tests {
		test := test
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			probe, ok := ProbePhysicalFtraceHeader(test.line)
			if !ok {
				t.Fatal("expected physical header provenance")
			}
			if probe.EventName != "block_rq_issue" {
				t.Fatalf("event name = %q", probe.EventName)
			}
			if probe.OwnerKnown != test.ownerKnown {
				t.Fatalf("owner known = %t, want %t: %+v", probe.OwnerKnown, test.ownerKnown, probe)
			}
			if test.ownerKnown && probe.HeaderTID != test.headerTID {
				t.Fatalf("header tid = %d, want %d", probe.HeaderTID, test.headerTID)
			}
			if !test.ownerKnown && probe.HeaderTID != 0 {
				t.Fatalf("unknown owner retained tid %d", probe.HeaderTID)
			}
		})
	}
}

func TestProbeExactEventNamePrefixRequiresPhysicalDelimiter(t *testing.T) {
	t.Parallel()
	valid := "worker-40 [002] d..2 5.000000: block_rq_issue: 8,0 R 4096 () 32 + 8 []"
	if name, ok := ProbeExactEventNamePrefix(valid); !ok || name != "block_rq_issue" {
		t.Fatalf("canonical endpoint not recognized: name=%q ok=%t", name, ok)
	}
	for _, near := range []string{
		"worker-40 [002] d..2 5.000000: block_rq_issue 8,0 R 4096 () 32 + 8 []",
		"worker-40 [002] d..2 5.000000: block_rq_issue : 8,0 R 4096 () 32 + 8 []",
		"worker-40 [002] d..2 5.000000: BLOCK_RQ_ISSUE: 8,0 R 4096 () 32 + 8 []",
		"worker-40 [002] d..2 5.000000: block_rq_issue_extra: 8,0 R 4096 () 32 + 8 []",
	} {
		name, ok := ProbeExactEventNamePrefix(near)
		if ok && name == "block_rq_issue" {
			t.Fatalf("near endpoint gained exact authority: name=%q line=%q", name, near)
		}
	}
}

func TestProbeLeadingExactEventNamePrefixRejectsEmbeddedHeaderPrefix(t *testing.T) {
	t.Parallel()
	valid := "   worker-40 [002] d..2 5.000000: block_rq_issue: 8,0 R 4096 () 32 + 8 []"
	for _, line := range []string{
		valid,
		valid + "\x01",
		valid + string([]byte{0xff}),
	} {
		name, ok := ProbeLeadingExactEventNamePrefix(line)
		if !ok || name != "block_rq_issue" {
			t.Fatalf("physical leading endpoint with unsafe body tail was lost: name=%q ok=%t line=%q", name, ok, line)
		}
	}

	for _, embedded := range []string{
		"\x00" + valid,
		"\n" + valid,
		"\t" + valid,
		string([]byte{0xff}) + valid,
		"metadata\x00" + valid,
	} {
		if name, ok := ProbeLeadingExactEventNamePrefix(embedded); ok {
			t.Fatalf("embedded header gained physical-origin authority: name=%q line=%q", name, embedded)
		}
	}
}
