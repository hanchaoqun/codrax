package tracebundle

import (
	"context"
	"errors"
	"strings"
	"testing"
)

func TestJSONEnvelopeRejectsMalformedAmbiguousAndNonObjectInputs(t *testing.T) {
	tests := []struct {
		name string
		body []byte
	}{
		{name: "empty", body: nil},
		{name: "root array", body: []byte(`[]`)},
		{name: "root scalar", body: []byte(`true`)},
		{name: "trailing value", body: []byte(`{} {}`)},
		{name: "trailing garbage", body: []byte(`{} x`)},
		{name: "invalid utf8", body: []byte{'{', '"', 'x', '"', ':', '"', 0xff, '"', '}'}},
		{name: "exact duplicate", body: []byte(`{"a":1,"a":2}`)},
		{name: "escaped duplicate", body: []byte(`{"a":1,"\u0061":2}`)},
		{name: "ASCII case duplicate", body: []byte(`{"Foo":1,"fOO":2}`)},
		{name: "SimpleFold duplicate", body: []byte(`{"K":1,"\u212a":2}`)},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if err := validateJSONEnvelope(context.Background(), test.body); !errors.Is(err, ErrInvalidManifest) {
				t.Fatalf("error = %v", err)
			}
		})
	}
	if err := validateJSONEnvelope(context.Background(), []byte(`{"x":{"A":1},"y":{"a":2}}`)); err != nil {
		t.Fatalf("same folded key in distinct objects rejected: %v", err)
	}
}

func TestJSONEnvelopeDepthBoundaries(t *testing.T) {
	build := func(containerDepth int) []byte {
		// The root object is depth one; each '[' adds one container level.
		return []byte(`{"x":` + strings.Repeat("[", containerDepth-1) + "0" + strings.Repeat("]", containerDepth-1) + "}")
	}
	if err := validateJSONEnvelope(context.Background(), build(maxJSONDepth)); err != nil {
		t.Fatalf("depth %d rejected: %v", maxJSONDepth, err)
	}
	if err := validateJSONEnvelope(context.Background(), build(maxJSONDepth+1)); !errors.Is(err, ErrInvalidManifest) || !strings.Contains(err.Error(), "nesting depth") {
		t.Fatalf("depth %d error = %v", maxJSONDepth+1, err)
	}
}

func TestJSONEnvelopeStringBoundaries(t *testing.T) {
	exact := `{"value":"` + strings.Repeat("x", maxJSONStringBytes) + `"}`
	if err := validateJSONEnvelope(context.Background(), []byte(exact)); err != nil {
		t.Fatalf("exact string limit rejected: %v", err)
	}
	over := `{"value":"` + strings.Repeat("x", maxJSONStringBytes+1) + `"}`
	if err := validateJSONEnvelope(context.Background(), []byte(over)); !errors.Is(err, ErrInvalidManifest) || !strings.Contains(err.Error(), "string limit") {
		t.Fatalf("string limit error = %v", err)
	}
}

func TestJSONEnvelopeRejectsUnpairedSurrogateEscapes(t *testing.T) {
	for name, body := range map[string]string{
		"high_value":       `{"path":"\uD800"}`,
		"low_value":        `{"path":"\uDC00"}`,
		"high_key":         `{"\uD800":"value"}`,
		"high_then_scalar": `{"path":"\uD800\u0041"}`,
		"high_then_high":   `{"path":"\uD800\uD801"}`,
	} {
		t.Run(name, func(t *testing.T) {
			if err := validateJSONEnvelope(context.Background(), []byte(body)); !errors.Is(err, ErrInvalidManifest) {
				t.Fatalf("unpaired surrogate error = %v", err)
			}
		})
	}
}

func TestJSONEnvelopeAcceptsPairedAndNonEscapedReplacementText(t *testing.T) {
	body := `{"emoji":"\uD83D\uDE00","replacement":"�","literal":"\\uD800"}`
	if err := validateJSONEnvelope(context.Background(), []byte(body)); err != nil {
		t.Fatalf("valid Unicode string forms rejected: %v", err)
	}
}

func TestJSONEnvelopeObjectMemberBoundaries(t *testing.T) {
	if err := validateJSONEnvelope(context.Background(), []byte(objectWithMembers(maxJSONObjectMembers))); err != nil {
		t.Fatalf("exact member limit rejected: %v", err)
	}
	if err := validateJSONEnvelope(context.Background(), []byte(objectWithMembers(maxJSONObjectMembers+1))); !errors.Is(err, ErrInvalidManifest) || !strings.Contains(err.Error(), "object member limit") {
		t.Fatalf("member limit error = %v", err)
	}
}

func TestJSONEnvelopeKnownAndUnknownArrayBoundaries(t *testing.T) {
	tests := []struct {
		field string
		limit int
	}{
		{field: "artifacts", limit: maxArtifactElements},
		{field: "perf_clock_alignments", limit: maxAlignmentOrGateElements},
		{field: "trace_tool_gates", limit: maxAlignmentOrGateElements},
		{field: "provider_decisions", limit: maxDecisionOrCoverageElements},
		{field: "trace_decisions", limit: maxDecisionOrCoverageElements},
		{field: "trace_provider_decisions", limit: maxDecisionOrCoverageElements},
		{field: "trace_db_coverage", limit: maxDecisionOrCoverageElements},
		{field: "trace_coverage", limit: maxDecisionOrCoverageElements},
		{field: "future_array", limit: maxUnknownArrayElements},
	}
	for _, test := range tests {
		t.Run(test.field, func(t *testing.T) {
			exact := `{"` + test.field + `":` + repeatedArray(test.limit) + `}`
			if err := validateJSONEnvelope(context.Background(), []byte(exact)); err != nil {
				t.Fatalf("exact limit %d rejected: %v", test.limit, err)
			}
			over := `{"` + strings.ToUpper(test.field) + `":` + repeatedArray(test.limit+1) + `}`
			if err := validateJSONEnvelope(context.Background(), []byte(over)); !errors.Is(err, ErrInvalidManifest) || !strings.Contains(err.Error(), "array element limit") {
				t.Fatalf("limit+1 error = %v", err)
			}
		})
	}
	nestedKnownName := `{"future":{"artifacts":` + repeatedArray(maxArtifactElements+1) + `}}`
	if err := validateJSONEnvelope(context.Background(), []byte(nestedKnownName)); err != nil {
		t.Fatalf("nested future field inherited top-level artifacts gate: %v", err)
	}
}

func TestJSONEnvelopeCaveatEvidenceBudgets(t *testing.T) {
	topExact := `{"caveats":` + repeatedArray(maxTopLevelCaveatElements) + `}`
	if err := validateJSONEnvelope(context.Background(), []byte(topExact)); err != nil {
		t.Fatalf("top caveat exact limit rejected: %v", err)
	}
	topOver := `{"caveats":` + repeatedArray(maxTopLevelCaveatElements+1) + `}`
	if err := validateJSONEnvelope(context.Background(), []byte(topOver)); !errors.Is(err, ErrInvalidManifest) || !strings.Contains(err.Error(), "array element limit") {
		t.Fatalf("top caveat limit error = %v", err)
	}

	globalExact := `{"nested":{"evidence":` + repeatedArray(maxCaveatEvidenceElements) + `}}`
	if err := validateJSONEnvelope(context.Background(), []byte(globalExact)); err != nil {
		t.Fatalf("global caveat/evidence exact limit rejected: %v", err)
	}
	globalOver := `{"nested":{"evidence":` + repeatedArray(maxCaveatEvidenceElements+1) + `}}`
	if err := validateJSONEnvelope(context.Background(), []byte(globalOver)); !errors.Is(err, ErrInvalidManifest) || !strings.Contains(err.Error(), "caveat/evidence") {
		t.Fatalf("global caveat/evidence limit error = %v", err)
	}

	splitOver := `{"a":{"caveats":` + repeatedArray(2049) + `},"b":{"evidence":` + repeatedArray(2048) + `}}`
	if err := validateJSONEnvelope(context.Background(), []byte(splitOver)); !errors.Is(err, ErrInvalidManifest) || !strings.Contains(err.Error(), "caveat/evidence") {
		t.Fatalf("cross-array caveat/evidence limit error = %v", err)
	}
}

func TestJSONEnvelopeTokenBoundaries(t *testing.T) {
	// root delimiters contribute 2 tokens; each field contributes its key,
	// array delimiters, and elements. These four arrays total exactly 262144.
	counts := []int{65536, 65536, 65536, 65522}
	build := func(extra int) []byte {
		var builder strings.Builder
		builder.WriteByte('{')
		for i, count := range counts {
			if i > 0 {
				builder.WriteByte(',')
			}
			builder.WriteString(`"k`)
			builder.WriteByte(byte('0' + i))
			builder.WriteString(`":`)
			if i == len(counts)-1 {
				count += extra
			}
			builder.WriteString(repeatedArray(count))
		}
		builder.WriteByte('}')
		return []byte(builder.String())
	}
	if err := validateJSONEnvelope(context.Background(), build(0)); err != nil {
		t.Fatalf("exact token limit rejected: %v", err)
	}
	if err := validateJSONEnvelope(context.Background(), build(1)); !errors.Is(err, ErrInvalidManifest) || !strings.Contains(err.Error(), "token limit") {
		t.Fatalf("token limit+1 error = %v", err)
	}
}

func TestJSONEnvelopePreservesCancellationDuringTokenScan(t *testing.T) {
	ctx := &cancelAfterChecksContext{cancelAt: 20}
	err := validateJSONEnvelope(ctx, []byte(`{"future":`+repeatedArray(1000)+`}`))
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("token-scan cancellation error = %v (checks=%d)", err, ctx.checks.Load())
	}
}

func TestCanonicalJSONKeyMatchesSimpleFoldClasses(t *testing.T) {
	for _, pair := range [][2]string{
		{"foo", "FOO"},
		{"K", "\u212a"},
		{"S", "\u017f"},
	} {
		if canonicalJSONKey(pair[0]) != canonicalJSONKey(pair[1]) {
			t.Fatalf("canonical keys differ: %q=%q %q=%q", pair[0], canonicalJSONKey(pair[0]), pair[1], canonicalJSONKey(pair[1]))
		}
	}
}
