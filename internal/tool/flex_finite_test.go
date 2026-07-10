package tool

import (
	"encoding/json"
	"math"
	"os"
	"strings"
	"testing"
)

func TestFlexFloatAcceptsFiniteNumbersAndUnitConversions(t *testing.T) {
	tests := map[string]float64{
		`1.25`:    1.25,
		`"-2.5"`:  -2.5,
		`"0.5s"`:  500,
		`"250us"`: 0.25,
	}
	for raw, want := range tests {
		t.Run(raw, func(t *testing.T) {
			var got FlexFloat
			if err := json.Unmarshal([]byte(raw), &got); err != nil {
				t.Fatalf("unmarshal finite value %s: %v", raw, err)
			}
			if math.Abs(got.Float64()-want) > 1e-12 {
				t.Fatalf("%s normalized to %g, want %g", raw, got.Float64(), want)
			}
		})
	}
}

func TestFlexFloatRejectsNonFiniteAndOverflowValues(t *testing.T) {
	tests := map[string]string{
		`"NaN"`:                     "non-finite",
		`"+Inf"`:                    "non-finite",
		`"-Inf"`:                    "non-finite",
		`1e309`:                     "finite float64 range",
		`"1e309"`:                   "finite float64 range",
		`"1.7976931348623157e308s"`: "unit conversion",
	}
	for raw, wantError := range tests {
		t.Run(raw, func(t *testing.T) {
			got := FlexFloat(17)
			err := json.Unmarshal([]byte(raw), &got)
			if err == nil {
				t.Fatalf("expected %s to be rejected, got %g", raw, got.Float64())
			}
			if !strings.Contains(err.Error(), wantError) {
				t.Fatalf("%s error = %q, want substring %q", raw, err, wantError)
			}
			if got.Float64() != 17 {
				t.Fatalf("failed decode mutated receiver to %g", got.Float64())
			}
		})
	}
}

func TestTraceSecondAcceptsFiniteNumberStringAndCompoundValues(t *testing.T) {
	tests := map[string]float64{
		`1.25`:           1.25,
		`"1250ms"`:       1.25,
		`"1s 250ms 5us"`: 1.250005,
	}
	for raw, want := range tests {
		t.Run(raw, func(t *testing.T) {
			var got TraceSecond
			if err := json.Unmarshal([]byte(raw), &got); err != nil {
				t.Fatalf("unmarshal finite timestamp %s: %v", raw, err)
			}
			if math.Abs(got.Seconds()-want) > 1e-12 {
				t.Fatalf("%s normalized to %.12g, want %.12g", raw, got.Seconds(), want)
			}
		})
	}
}

func TestTraceSecondRejectsNonFiniteOverflowAndCompoundOverflow(t *testing.T) {
	hugeFiniteItem := strings.Repeat("9", 308)
	hugeOutOfRangeItem := strings.Repeat("9", 400)
	tests := map[string]string{
		`"NaN"`:                             "non-finite",
		`"+Inf"`:                            "non-finite",
		`"-Inf"`:                            "non-finite",
		`1e309`:                             "finite float64 range",
		`"1e309s"`:                          "finite float64 range",
		`"` + hugeOutOfRangeItem + `s 1ms"`: "compound item",
		`"` + hugeFiniteItem + `s ` + hugeFiniteItem + `s"`: "compound timestamp",
	}
	for raw, wantError := range tests {
		t.Run(raw[:min(len(raw), 48)], func(t *testing.T) {
			var got TraceSecond
			err := json.Unmarshal([]byte(raw), &got)
			if err == nil {
				t.Fatalf("expected timestamp to be rejected, got %g", got.Seconds())
			}
			if !strings.Contains(err.Error(), wantError) {
				t.Fatalf("error = %q, want substring %q", err, wantError)
			}
			if got.Set() {
				t.Fatalf("failed decode must not publish a timestamp: %+v", got)
			}
		})
	}
}

func TestTraceQueryMarshalPayloadFailsClosedForNonFiniteResults(t *testing.T) {
	for name, value := range map[string]float64{
		"nan":     math.NaN(),
		"pos_inf": math.Inf(1),
		"neg_inf": math.Inf(-1),
	} {
		t.Run(name, func(t *testing.T) {
			payload, failure := traceQueryMarshalPayload("trace_query", struct {
				Value float64 `json:"value"`
			}{Value: value})
			if payload != nil {
				t.Fatalf("marshal failure published payload %q", payload)
			}
			if failure == nil || failure.Success {
				t.Fatalf("expected fail-closed ToolResult, got %+v", failure)
			}
			if failure.ToolName != "trace_query" || failure.RawRef != "" || len(failure.Observations) != 0 {
				t.Fatalf("marshal failure leaked publication metadata: %+v", failure)
			}
			if !strings.Contains(failure.Summary, "failed to serialize result") {
				t.Fatalf("unexpected failure summary: %q", failure.Summary)
			}
		})
	}

	payload, failure := traceQueryMarshalPayload("trace_query", struct {
		Value float64 `json:"value"`
	}{Value: 1.25})
	if failure != nil || !strings.Contains(string(payload), `"value": 1.25`) {
		t.Fatalf("finite result failed to serialize: payload=%q failure=%+v", payload, failure)
	}
}

func TestTraceQueryAllIndentedJSONPublicationUsesFailClosedGateway(t *testing.T) {
	source, err := os.ReadFile("trace_query.go")
	if err != nil {
		t.Fatal(err)
	}
	if count := strings.Count(string(source), "json.MarshalIndent("); count != 1 {
		t.Fatalf("trace_query.go has %d direct MarshalIndent calls; want only the fail-closed gateway", count)
	}
	if strings.Contains(string(source), "_, _ := json.MarshalIndent") || strings.Contains(string(source), "payload, _ := json.MarshalIndent") {
		t.Fatal("trace_query.go must not ignore a MarshalIndent error")
	}
}
