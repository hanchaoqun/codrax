package hitraceconv

import (
	"bytes"
	"context"
	"crypto/sha256"
	"errors"
	"strings"
	"testing"
	"unicode/utf8"
)

type profilerByteCancelAfterPollContext struct {
	context.Context
	cancelAt int
	polls    int
	err      error
}

func (ctx *profilerByteCancelAfterPollContext) Err() error {
	ctx.polls++
	if ctx.cancelAt > 0 && ctx.polls >= ctx.cancelAt {
		return ctx.err
	}
	if ctx.Context == nil {
		return nil
	}
	return ctx.Context.Err()
}

func TestProfilerByteContextCheckpointBoundaries(t *testing.T) {
	if profilerContextByteCheckpointBytes != 64<<10 {
		t.Fatalf("checkpoint bytes=%d want=%d", profilerContextByteCheckpointBytes, 64<<10)
	}
	ctx := &profilerByteCancelAfterPollContext{
		Context: context.Background(), cancelAt: 2, err: context.Canceled,
	}
	processed := uint64(0)
	if err := profilerByteContextCheckpoint(ctx, &processed, 1); err != nil || processed != 1 || ctx.polls != 1 {
		t.Fatalf("first checkpoint err=%v processed=%d polls=%d", err, processed, ctx.polls)
	}
	if err := profilerByteContextCheckpoint(ctx, &processed, profilerContextByteCheckpointBytes-2); err != nil ||
		processed != profilerContextByteCheckpointBytes-1 || ctx.polls != 1 {
		t.Fatalf("pre-boundary checkpoint err=%v processed=%d polls=%d", err, processed, ctx.polls)
	}
	if err := profilerByteContextCheckpoint(ctx, &processed, 1); !errors.Is(err, context.Canceled) ||
		processed != profilerContextByteCheckpointBytes-1 || ctx.polls != 2 {
		t.Fatalf("boundary cancellation err=%v processed=%d polls=%d", err, processed, ctx.polls)
	}

	if err := profilerByteContextCheckpoint(context.Background(), nil, 0); err == nil {
		t.Fatal("nil processed checkpoint unexpectedly succeeded")
	} else if reason, ok := traceDBOutputInvariantReason(err); !ok || reason != "profiler_byte_checkpoint_nil" {
		t.Fatalf("nil processed reason=(%q,%v) want profiler_byte_checkpoint_nil", reason, ok)
	}
	processed = 7
	if err := profilerByteContextCheckpoint(context.Background(), &processed, profilerContextByteCheckpointBytes+1); err == nil {
		t.Fatal("oversized delta unexpectedly succeeded")
	} else if reason, ok := traceDBOutputInvariantReason(err); !ok || reason != "profiler_byte_checkpoint_delta" || processed != 7 {
		t.Fatalf("oversized delta reason=(%q,%v) processed=%d", reason, ok, processed)
	}
	processed = ^uint64(0)
	if err := profilerByteContextCheckpoint(context.Background(), &processed, 1); err == nil {
		t.Fatal("overflowing checkpoint unexpectedly succeeded")
	} else if reason, ok := traceDBOutputInvariantReason(err); !ok || reason != "profiler_byte_checkpoint_overflow" || processed != ^uint64(0) {
		t.Fatalf("overflow reason=(%q,%v) processed=%d", reason, ok, processed)
	}
}

func TestProfilerByteValidatorsMatchStringAuthorities(t *testing.T) {
	exactCap := bytes.Repeat([]byte("x"), maxTraceDBSystraceLineBytes)
	overCap := append(append([]byte(nil), exactCap...), 'x')
	tests := []struct {
		name string
		raw  []byte
	}{
		{name: "nil", raw: nil},
		{name: "empty", raw: []byte{}},
		{name: "ascii", raw: []byte("clock_name")},
		{name: "leading space", raw: []byte(" clock")},
		{name: "trailing space", raw: []byte("clock ")},
		{name: "ascii blank", raw: []byte("   ")},
		{name: "equals", raw: []byte("clock=name")},
		{name: "pipe", raw: []byte("clock|name")},
		{name: "tab", raw: []byte("clock\tname")},
		{name: "newline", raw: []byte("clock\nname")},
		{name: "nul", raw: []byte{'a', 0, 'b'}},
		{name: "delete control", raw: []byte{'a', 0x7f, 'b'}},
		{name: "unicode space", raw: []byte("clock\u00a0name")},
		{name: "unicode blank", raw: []byte("\u3000")},
		{name: "line separator", raw: []byte("clock\u2028name")},
		{name: "paragraph separator", raw: []byte("clock\u2029name")},
		{name: "replacement rune", raw: []byte("clock\ufffdname")},
		{name: "invalid utf8", raw: []byte{0xff, 0xfe}},
		{name: "truncated utf8", raw: []byte{0xe4, 0xb8}},
		{name: "exact cap", raw: exactCap},
		{name: "over cap", raw: overCap},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			for _, allowBlank := range []bool{false, true} {
				got, err := profilerSinglePhysicalLineBytesContext(context.Background(), test.raw, allowBlank)
				if err != nil {
					t.Fatalf("physical allowBlank=%v: %v", allowBlank, err)
				}
				want := traceDBSinglePhysicalLine(string(test.raw), allowBlank)
				if got != want {
					t.Fatalf("physical allowBlank=%v got=%v want=%v raw=%q", allowBlank, got, want, test.raw)
				}
			}
			got, err := profilerSingleTokenBytesContext(context.Background(), test.raw)
			if err != nil {
				t.Fatalf("token: %v", err)
			}
			want := traceDBSingleToken(string(test.raw))
			if got != want {
				t.Fatalf("token got=%v want=%v raw=%q", got, want, test.raw)
			}
		})
	}

	uncapped, err := profilerPhysicalRunesSafeBytesContext(context.Background(), overCap)
	if err != nil || !uncapped {
		t.Fatalf("uncapped physical over-cap=(%v,%v) want (true,nil)", uncapped, err)
	}
}

func TestProfilerStringValidatorsMatchByteAuthorities(t *testing.T) {
	inputs := []string{
		"", "value", " value", "value ", "   ", "a=b", "a|b", "a\tb", "a\nb",
		"时钟", "a\u00a0b", "\u3000", "a\u2028b", "a\u2029b",
		string([]byte{0xff, 0, 0xfe}),
		strings.Repeat("x", maxTraceDBSystraceLineBytes),
		strings.Repeat("x", maxTraceDBSystraceLineBytes+1),
	}
	for _, raw := range inputs {
		for _, allowBlank := range []bool{false, true} {
			fromBytes, bytesErr := profilerSinglePhysicalLineBytesContext(context.Background(), []byte(raw), allowBlank)
			fromString, stringErr := profilerSinglePhysicalLineStringContext(context.Background(), raw, allowBlank)
			if bytesErr != nil || stringErr != nil || fromString != fromBytes || fromString != traceDBSinglePhysicalLine(raw, allowBlank) {
				t.Fatalf("physical parity bytes=%d blank=%t got=(%t,%v) bytes=(%t,%v) legacy=%t",
					len(raw), allowBlank, fromString, stringErr, fromBytes, bytesErr, traceDBSinglePhysicalLine(raw, allowBlank))
			}
		}
		fromBytes, bytesErr := profilerSingleTokenBytesContext(context.Background(), []byte(raw))
		fromString, stringErr := profilerSingleTokenStringContext(context.Background(), raw)
		if bytesErr != nil || stringErr != nil || fromString != fromBytes || fromString != traceDBSingleToken(raw) {
			t.Fatalf("token parity bytes=%d got=(%t,%v) bytes=(%t,%v) legacy=%t",
				len(raw), fromString, stringErr, fromBytes, bytesErr, traceDBSingleToken(raw))
		}
	}
}

func TestProfilerStringValidatorsCancellationIdentity(t *testing.T) {
	raw := strings.Repeat("x", 4*profilerContextByteCheckpointBytes+31)
	for _, test := range []struct {
		name string
		run  func(context.Context) (bool, error)
	}{
		{name: "physical", run: func(ctx context.Context) (bool, error) {
			return profilerSinglePhysicalLineStringContext(ctx, raw, false)
		}},
		{name: "token", run: func(ctx context.Context) (bool, error) {
			return profilerSingleTokenStringContext(ctx, raw)
		}},
	} {
		t.Run(test.name, func(t *testing.T) {
			for _, want := range []error{context.Canceled, context.DeadlineExceeded} {
				ctx := &profilerByteCancelAfterPollContext{
					Context: context.Background(), cancelAt: 4, err: want,
				}
				valid, err := test.run(ctx)
				if valid || err != want || ctx.polls != ctx.cancelAt {
					t.Fatalf("string validator cancellation valid=%t polls=%d/%d err=%T %v want=%v",
						valid, ctx.polls, ctx.cancelAt, err, err, want)
				}
			}
		})
	}
}

func TestProfilerByteValidatorsCoverUnicodeRuneSemantics(t *testing.T) {
	for r := rune(0); r <= utf8.MaxRune; r += 997 {
		raw := []byte(string(r))
		for _, allowBlank := range []bool{false, true} {
			got, err := profilerSinglePhysicalLineBytesContext(context.Background(), raw, allowBlank)
			if err != nil {
				t.Fatalf("rune U+%04X physical: %v", r, err)
			}
			if want := traceDBSinglePhysicalLine(string(raw), allowBlank); got != want {
				t.Fatalf("rune U+%04X physical allowBlank=%v got=%v want=%v", r, allowBlank, got, want)
			}
		}
		got, err := profilerSingleTokenBytesContext(context.Background(), raw)
		if err != nil {
			t.Fatalf("rune U+%04X token: %v", r, err)
		}
		if want := traceDBSingleToken(string(raw)); got != want {
			t.Fatalf("rune U+%04X token got=%v want=%v", r, got, want)
		}
	}
}

func TestProfilerByteValidatorsAndCloneCancelWithoutOutput(t *testing.T) {
	raw := bytes.Repeat([]byte("x"), 2*profilerContextByteCheckpointBytes+17)
	for _, test := range []struct {
		name string
		run  func(context.Context) (bool, error)
	}{
		{name: "physical", run: func(ctx context.Context) (bool, error) {
			return profilerSinglePhysicalLineBytesContext(ctx, raw, false)
		}},
		{name: "token", run: func(ctx context.Context) (bool, error) {
			return profilerSingleTokenBytesContext(ctx, raw)
		}},
		{name: "uncapped", run: func(ctx context.Context) (bool, error) {
			return profilerPhysicalRunesSafeBytesContext(ctx, raw)
		}},
	} {
		t.Run(test.name, func(t *testing.T) {
			ctx := &profilerByteCancelAfterPollContext{
				Context: context.Background(), cancelAt: 3, err: context.DeadlineExceeded,
			}
			got, err := test.run(ctx)
			if got || !errors.Is(err, context.DeadlineExceeded) || ctx.polls < ctx.cancelAt {
				t.Fatalf("result=(%v,%v) polls=%d want false/deadline", got, err, ctx.polls)
			}
		})
	}

	shortFinalCtx := &profilerByteCancelAfterPollContext{
		Context: context.Background(), cancelAt: 3, err: context.Canceled,
	}
	cloned, err := profilerCloneBytesStringContext(shortFinalCtx, []byte("completed-copy"))
	if cloned != "" || !errors.Is(err, context.Canceled) || shortFinalCtx.polls != shortFinalCtx.cancelAt {
		t.Fatalf("final-check clone=(%q,%v) polls=%d", cloned, err, shortFinalCtx.polls)
	}
	midCtx := &profilerByteCancelAfterPollContext{
		Context: context.Background(), cancelAt: 3, err: context.DeadlineExceeded,
	}
	cloned, err = profilerCloneBytesStringContext(midCtx, raw)
	if cloned != "" || !errors.Is(err, context.DeadlineExceeded) || midCtx.polls != midCtx.cancelAt {
		t.Fatalf("mid-copy clone bytes=%d err=%v polls=%d", len(cloned), err, midCtx.polls)
	}

	original := []byte("clone-is-independent")
	cloned, err = profilerCloneBytesStringContext(context.Background(), original)
	if err != nil || cloned != string(original) {
		t.Fatalf("successful clone=(%q,%v)", cloned, err)
	}
	original[0] = 'X'
	if cloned != "clone-is-independent" {
		t.Fatalf("clone aliases input: %q", cloned)
	}
}

func TestProfilerASCIITrimAndCommentPrefixContextParity(t *testing.T) {
	for _, raw := range [][]byte{
		nil,
		{},
		[]byte("plain"),
		[]byte(" plain"),
		[]byte("plain "),
		[]byte("  plain  "),
		[]byte("   "),
		[]byte("\t# comment"),
		[]byte(" \t# comment "),
		[]byte("\u3000plain\u3000"),
	} {
		got, err := profilerTrimASCIISpacesBytesContext(context.Background(), raw)
		if err != nil || !bytes.Equal(got, bytes.Trim(raw, " ")) {
			t.Fatalf("trim raw=%q got=%q err=%v want=%q", raw, got, err, bytes.Trim(raw, " "))
		}
		prefix, err := profilerTextCommentPrefixContext(context.Background(), raw)
		if err != nil || prefix != profilerTextCommentPrefix(raw) {
			t.Fatalf("comment raw=%q got=%v err=%v want=%v", raw, prefix, err, profilerTextCommentPrefix(raw))
		}
	}

	longSpaces := append(bytes.Repeat([]byte{' '}, 2*profilerContextByteCheckpointBytes+1), 'x')
	trimCtx := &profilerByteCancelAfterPollContext{
		Context: context.Background(), cancelAt: 3, err: context.Canceled,
	}
	if got, err := profilerTrimASCIISpacesBytesContext(trimCtx, longSpaces); got != nil ||
		!errors.Is(err, context.Canceled) || trimCtx.polls != trimCtx.cancelAt {
		t.Fatalf("trim cancellation got=%q err=%v polls=%d", got, err, trimCtx.polls)
	}

	longTabs := append(bytes.Repeat([]byte{'\t'}, 2*profilerContextByteCheckpointBytes+1), '#')
	commentCtx := &profilerByteCancelAfterPollContext{
		Context: context.Background(), cancelAt: 3, err: context.DeadlineExceeded,
	}
	if got, err := profilerTextCommentPrefixContext(commentCtx, longTabs); got ||
		!errors.Is(err, context.DeadlineExceeded) || commentCtx.polls != commentCtx.cancelAt {
		t.Fatalf("comment cancellation got=%v err=%v polls=%d", got, err, commentCtx.polls)
	}
}

func TestProfilerStableSampleObserveContextMatchesLegacyAlgorithm(t *testing.T) {
	type input struct {
		domain string
		raw    []byte
	}
	inputs := make([]input, 0, 24)
	for index, size := range []int{0, 1, 95, 96, 97, profilerContextByteCheckpointBytes - 1,
		profilerContextByteCheckpointBytes, profilerContextByteCheckpointBytes + 1,
		2*profilerContextByteCheckpointBytes + 31} {
		raw := bytes.Repeat([]byte{byte('a' + index)}, size)
		inputs = append(inputs, input{domain: "domain-" + string(rune('a'+index)), raw: raw})
	}
	inputs = append(inputs,
		input{domain: strings.Repeat("d", profilerContextByteCheckpointBytes+9), raw: []byte("long-domain")},
		input{domain: "domain-a", raw: nil}, // exact duplicate
	)

	var got, want profilerStableSampleSet
	for _, item := range inputs {
		if err := got.observeContext(context.Background(), item.domain, item.raw); err != nil {
			t.Fatalf("observeContext domain=%q bytes=%d: %v", item.domain, len(item.raw), err)
		}
		profilerLegacyStableSampleObserve(&want, item.domain, item.raw)
		if got != want {
			t.Fatalf("sample parity drift after domain=%q bytes=%d\ngot=%+v\nwant=%+v", item.domain, len(item.raw), got, want)
		}
	}
	var wrapped profilerStableSampleSet
	for _, item := range inputs {
		wrapped.observe(item.domain, item.raw)
	}
	if wrapped != want || got.Used != profilerDiagnosticSampleLimit {
		t.Fatalf("background wrapper parity=%v used=%d want=%d", wrapped == want, got.Used, profilerDiagnosticSampleLimit)
	}
}

func TestProfilerStableSampleObserveStringContextMatchesBytes(t *testing.T) {
	inputs := []struct {
		domain string
		raw    string
	}{
		{domain: "", raw: ""},
		{domain: "short", raw: "value"},
		{domain: strings.Repeat("d", profilerContextByteCheckpointBytes+3), raw: "long-domain"},
		{domain: "long-raw", raw: strings.Repeat("r", 2*profilerContextByteCheckpointBytes+17)},
		{domain: "utf8", raw: "时钟-\ufffd-值"},
		{domain: "invalid", raw: string([]byte{0xff, 0, 0xfe})},
	}
	var fromBytes, fromStrings profilerStableSampleSet
	for _, input := range inputs {
		if err := fromBytes.observeContext(context.Background(), input.domain, []byte(input.raw)); err != nil {
			t.Fatalf("bytes domain=%q raw=%d: %v", input.domain, len(input.raw), err)
		}
		if err := fromStrings.observeStringContext(context.Background(), input.domain, input.raw); err != nil {
			t.Fatalf("string domain=%q raw=%d: %v", input.domain, len(input.raw), err)
		}
		if fromStrings != fromBytes {
			t.Fatalf("string/bytes parity drift domain=%q raw=%d\nstrings=%+v\nbytes=%+v",
				input.domain, len(input.raw), fromStrings, fromBytes)
		}
	}
}

func TestProfilerStableSampleObserveStringPartsContextMatchesLogicalBytes(t *testing.T) {
	large := strings.Repeat("n", maxTraceDBSystraceLineBytes)
	tests := []struct {
		name   string
		domain string
		parts  []string
	}{
		{name: "no parts", domain: "empty"},
		{name: "empty parts", domain: "empty-parts", parts: []string{"", "", ""}},
		{name: "symbol", domain: "profiler-ftrace-summary-symbol", parts: []string{"0x", "abc", "=", "VerifyClass"}},
		{name: "utf8 split", domain: "utf8", parts: []string{"时", "钟", "-", "值"}},
		{name: "large symbol", domain: "profiler-ftrace-summary-symbol", parts: []string{"0x", "ffffffff", "=", large}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			logical := strings.Join(test.parts, "")
			var got, want profilerStableSampleSet
			if err := got.observeStringPartsContext(context.Background(), test.domain, test.parts...); err != nil {
				t.Fatalf("parts sample: %v", err)
			}
			if err := want.observeContext(context.Background(), test.domain, []byte(logical)); err != nil {
				t.Fatalf("logical bytes sample: %v", err)
			}
			if got != want {
				t.Fatalf("parts/bytes parity drift parts=%d logical=%d\ngot=%+v\nwant=%+v",
					len(test.parts), len(logical), got, want)
			}
		})
	}
}

func TestProfilerStableSampleObserveContextCancellationIsAtomic(t *testing.T) {
	var samples profilerStableSampleSet
	samples.observe("seed", []byte("existing"))
	before := samples

	preCanceled, cancel := context.WithCancel(context.Background())
	cancel()
	if err := samples.observeContext(preCanceled, "new", []byte("value")); !errors.Is(err, context.Canceled) || samples != before {
		t.Fatalf("pre-cancel err=%v mutated=%v", err, samples != before)
	}

	shortFinalCtx := &profilerByteCancelAfterPollContext{
		Context: context.Background(), cancelAt: 3, err: context.DeadlineExceeded,
	}
	if err := samples.observeContext(shortFinalCtx, "new", []byte("fully-hashed")); !errors.Is(err, context.DeadlineExceeded) || samples != before || shortFinalCtx.polls != shortFinalCtx.cancelAt {
		t.Fatalf("final cancellation err=%v mutated=%v polls=%d", err, samples != before, shortFinalCtx.polls)
	}

	midCtx := &profilerByteCancelAfterPollContext{
		Context: context.Background(), cancelAt: 4, err: context.Canceled,
	}
	large := bytes.Repeat([]byte("z"), 2*profilerContextByteCheckpointBytes+1)
	if err := samples.observeContext(midCtx, "new", large); !errors.Is(err, context.Canceled) || samples != before || midCtx.polls != midCtx.cancelAt {
		t.Fatalf("mid-hash cancellation err=%v mutated=%v polls=%d", err, samples != before, midCtx.polls)
	}

	var nilSamples *profilerStableSampleSet
	if err := nilSamples.observeContext(preCanceled, "ignored", nil); !errors.Is(err, context.Canceled) {
		t.Fatalf("nil receiver lost pre-cancellation: %v", err)
	}

	stringFinalCtx := &profilerByteCancelAfterPollContext{
		Context: context.Background(), cancelAt: 3, err: context.DeadlineExceeded,
	}
	if err := samples.observeStringContext(stringFinalCtx, "new", "fully-hashed"); !errors.Is(err, context.DeadlineExceeded) || samples != before || stringFinalCtx.polls != stringFinalCtx.cancelAt {
		t.Fatalf("string final cancellation err=%v mutated=%v polls=%d", err, samples != before, stringFinalCtx.polls)
	}
	stringMidCtx := &profilerByteCancelAfterPollContext{
		Context: context.Background(), cancelAt: 4, err: context.Canceled,
	}
	if err := samples.observeStringContext(stringMidCtx, "new", string(large)); !errors.Is(err, context.Canceled) || samples != before || stringMidCtx.polls != stringMidCtx.cancelAt {
		t.Fatalf("string mid cancellation err=%v mutated=%v polls=%d", err, samples != before, stringMidCtx.polls)
	}

	partsMidCtx := &profilerByteCancelAfterPollContext{
		Context: context.Background(), cancelAt: 4, err: context.DeadlineExceeded,
	}
	if err := samples.observeStringPartsContext(partsMidCtx, "new", "0x", "abc", "=", string(large)); !errors.Is(err, context.DeadlineExceeded) || samples != before || partsMidCtx.polls != partsMidCtx.cancelAt {
		t.Fatalf("parts mid cancellation err=%v mutated=%v polls=%d", err, samples != before, partsMidCtx.polls)
	}

	emptyParts := make([]string, 257)
	partsOccurrenceCtx := &profilerByteCancelAfterPollContext{
		Context: context.Background(), cancelAt: 3, err: context.Canceled,
	}
	if err := samples.observeStringPartsContext(partsOccurrenceCtx, "new", emptyParts...); !errors.Is(err, context.Canceled) || samples != before || partsOccurrenceCtx.polls != partsOccurrenceCtx.cancelAt {
		t.Fatalf("parts occurrence cancellation err=%v mutated=%v polls=%d", err, samples != before, partsOccurrenceCtx.polls)
	}
}

func profilerLegacyStableSampleObserve(samples *profilerStableSampleSet, domain string, raw []byte) {
	if samples == nil {
		return
	}
	hash := sha256.New()
	_, _ = hash.Write([]byte(domain))
	_, _ = hash.Write([]byte{0})
	_, _ = hash.Write(raw)
	var digest [sha256.Size]byte
	copy(digest[:], hash.Sum(nil))
	used := int(samples.Used)
	insert := used
	for index := 0; index < used; index++ {
		cmp := bytes.Compare(digest[:], samples.Items[index].Digest[:])
		if cmp == 0 {
			return
		}
		if cmp < 0 {
			insert = index
			break
		}
	}
	if used == profilerDiagnosticSampleLimit && insert == used {
		return
	}
	if used < profilerDiagnosticSampleLimit {
		used++
		samples.Used = uint8(used)
	}
	for index := used - 1; index > insert; index-- {
		samples.Items[index] = samples.Items[index-1]
	}
	item := profilerDiagnosticSample{Digest: digest, InputLen: uint64(len(raw))}
	prefixLen := min(len(raw), profilerDiagnosticPrefixBytes)
	copy(item.Prefix[:], raw[:prefixLen])
	item.PrefixLen = uint8(prefixLen)
	samples.Items[insert] = item
}

var _ context.Context = (*profilerByteCancelAfterPollContext)(nil)
