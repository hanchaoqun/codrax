package hitraceconv

import (
	"context"
	"encoding/binary"
	"errors"
	"os"
	"path/filepath"
	"testing"
)

func TestInspectSimpleperfTextProfileStrictMetadata(t *testing.T) {
	goodEvents := "cpu-clock:u,1,0\nsched:sched_switch,2,91"
	tests := []struct {
		name string
		meta []byte
		mode simpleperfTextProfileMode
	}{
		{name: "meta_absent", mode: simpleperfTextProfileOrdinary},
		{name: "trace_offcpu_missing", meta: simpleperfMetaPairs("arch", "arm64"), mode: simpleperfTextProfileOrdinary},
		{name: "trace_offcpu_false", meta: simpleperfMetaPairs("trace_offcpu", "false", "event_type_info", "broken"), mode: simpleperfTextProfileOrdinary},
		{name: "trace_offcpu_true", meta: simpleperfMetaPairs("trace_offcpu", "true", "event_type_info", goodEvents), mode: simpleperfTextProfileTraceOffCPU},
		{name: "unknown_boolean", meta: simpleperfMetaPairs("trace_offcpu", "TRUE", "event_type_info", goodEvents), mode: simpleperfTextProfileUnknown},
		{name: "missing_event_table", meta: simpleperfMetaPairs("trace_offcpu", "true"), mode: simpleperfTextProfileUnknown},
		{name: "duplicate_meta", meta: simpleperfMetaPairs("trace_offcpu", "true", "trace_offcpu", "false"), mode: simpleperfTextProfileUnknown},
		{name: "truncated_meta", meta: []byte("trace_offcpu\x00true"), mode: simpleperfTextProfileUnknown},
		{name: "duplicate_event", meta: simpleperfMetaPairs("trace_offcpu", "true", "event_type_info", "cpu-clock,1,0\ncpu-clock,2,91"), mode: simpleperfTextProfileUnknown},
		{name: "noncanonical_scalar", meta: simpleperfMetaPairs("trace_offcpu", "true", "event_type_info", "cpu-clock,01,0\nsched:sched_switch,2,91"), mode: simpleperfTextProfileUnknown},
		{name: "trailing_event_newline", meta: simpleperfMetaPairs("trace_offcpu", "true", "event_type_info", goodEvents+"\n"), mode: simpleperfTextProfileUnknown},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			body := syntheticRawPerfData()
			if tc.meta != nil {
				body = syntheticRawPerfDataWithSimpleperfMeta(tc.meta)
			}
			path := filepath.Join(t.TempDir(), "perf.data")
			if err := os.WriteFile(path, body, 0o600); err != nil {
				t.Fatal(err)
			}
			authority, err := openConversionInputAuthority(path)
			if err != nil {
				t.Fatal(err)
			}
			defer authority.Close()
			binding, err := newDirectPerfInputBinding(authority, perfInputLinuxPerfData)
			if err != nil {
				t.Fatal(err)
			}
			profile, err := inspectSimpleperfTextProfile(context.Background(), binding)
			if err != nil {
				t.Fatalf("inspect profile: %v", err)
			}
			if profile.mode != tc.mode {
				t.Fatalf("profile=%+v want mode=%d", profile, tc.mode)
			}
			if tc.mode == simpleperfTextProfileTraceOffCPU {
				if profile.onCPUEvent != "cpu-clock:u" {
					t.Fatalf("on-CPU event=%q", profile.onCPUEvent)
				}
				if _, ok := profile.offCPUEvents["sched:sched_switch"]; !ok || len(profile.offCPUEvents) != 1 {
					t.Fatalf("off-CPU events=%v", profile.offCPUEvents)
				}
			}
		})
	}
}

func TestSimpleperfHelpExactTokenBoundary(t *testing.T) {
	good := "--trace-offcpu {on-cpu,off-cpu,on-off-cpu,mixed-on-off-cpu}"
	for _, token := range []string{"--trace-offcpu", "on-off-cpu", "mixed-on-off-cpu"} {
		if !simpleperfHelpHasExactToken(good, token) {
			t.Fatalf("exact token %q was not found", token)
		}
	}
	bad := "--trace-offcpu-extra mixed-on-off-cpu-only"
	if simpleperfHelpHasExactToken(bad, "--trace-offcpu") || simpleperfHelpHasExactToken(bad, "mixed-on-off-cpu") {
		t.Fatalf("deceptive help token was accepted: %q", bad)
	}
}

func TestInspectSimpleperfTextProfileGenerationAndCancellationAreFatal(t *testing.T) {
	meta := simpleperfMetaPairs(
		"trace_offcpu", "true",
		"event_type_info", "cpu-clock:u,1,0\nsched:sched_switch,2,91",
	)
	body := syntheticRawPerfDataWithSimpleperfMeta(meta)
	t.Run("generation", func(t *testing.T) {
		view := newScriptedStandaloneInputView("profile.perf.data", body)
		view.failStage = conversionInputStageDirectPerfRead
		view.failCall = 2
		binding, err := newDirectPerfInputBinding(view, perfInputLinuxPerfData)
		if err != nil {
			t.Fatal(err)
		}
		_, err = inspectSimpleperfTextProfile(context.Background(), binding)
		assertDirectPerfGenerationError(t, err)
		if view.reads == 0 {
			t.Fatal("generation fixture did not exercise immutable profile reads")
		}
	})
	t.Run("cancellation", func(t *testing.T) {
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()
		base := newScriptedStandaloneInputView("profile.perf.data", body)
		view := &cancelingDirectPerfInputView{scriptedStandaloneInputView: base, cancel: cancel}
		binding, err := newDirectPerfInputBinding(view, perfInputLinuxPerfData)
		if err != nil {
			t.Fatal(err)
		}
		_, err = inspectSimpleperfTextProfile(ctx, binding)
		if !errors.Is(err, context.Canceled) || view.reads != 1 {
			t.Fatalf("profile cancellation err=%T %v reads=%d", err, err, view.reads)
		}
	})
}

func simpleperfMetaPairs(values ...string) []byte {
	var out []byte
	for _, value := range values {
		out = append(out, value...)
		out = append(out, 0)
	}
	return out
}

func syntheticRawPerfDataWithSimpleperfMeta(meta []byte) []byte {
	out := append([]byte(nil), syntheticRawPerfData()...)
	featureByte := 72 + perfFeatureMetaInfo/8
	out[featureByte] |= byte(1 << (perfFeatureMetaInfo % 8))
	descriptorOffset := len(out)
	sectionOffset := descriptorOffset + 16
	descriptor := make([]byte, 16)
	binary.LittleEndian.PutUint64(descriptor[0:8], uint64(sectionOffset))
	binary.LittleEndian.PutUint64(descriptor[8:16], uint64(len(meta)))
	out = append(out, descriptor...)
	out = append(out, meta...)
	return out
}
