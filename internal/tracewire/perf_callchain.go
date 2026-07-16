package tracewire

import (
	"context"
	"strings"
	"unicode/utf8"
)

// PerfCallchainBuilder constructs the canonical root-to-leaf semicolon list
// without first allocating a complete []string. AppendFrame is transactional:
// cancellation or validation failure leaves the previous wire value intact.
type PerfCallchainBuilder struct {
	body   strings.Builder
	frames int
}

func (b *PerfCallchainBuilder) AppendFrame(ctx context.Context, frame string) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if !utf8.ValidString(frame) {
		return &PerfWireBuildError{Field: "callchain", Reason: "invalid_utf8"}
	}
	if frame == "" {
		return &PerfWireBuildError{Field: "callchain", Reason: "empty_frame"}
	}
	extra := len(frame)
	if b.frames > 0 {
		extra++
	}
	actual := b.body.Len() + extra
	if actual > MaxPerfCallchainBytes {
		return &PerfWireBuildError{
			Field:  "callchain",
			Reason: "decoded_value_too_long",
			Limit:  MaxPerfCallchainBytes,
			Actual: uint64(actual),
		}
	}
	if b.frames > 0 {
		b.body.WriteByte(';')
	}
	b.body.WriteString(frame)
	b.frames++
	return nil
}

func (b *PerfCallchainBuilder) String() string {
	return b.body.String()
}

func (b *PerfCallchainBuilder) Len() int {
	return b.body.Len()
}

func (b *PerfCallchainBuilder) Frames() int {
	return b.frames
}
