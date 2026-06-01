package hitraceconv

import "fmt"

const (
	defaultOutputSuffix = ".systrace"
	converterVersion    = "hitraceconv-v1"
)

// Options controls one explicit binary HiTrace conversion.
type Options struct {
	InputPath  string
	OutputPath string
	Flavor     string
}

// Result summarizes a completed conversion.
type Result struct {
	InputPath          string
	OutputPath         string
	InputBytes         int64
	OutputBytes        int64
	EventsWritten      int
	MissingFormatCount int
	UnknownEventCount  int
	FirstTimestampSec  float64
	LastTimestampSec   float64
	Caveats            []string
}

// DefaultOutputPath appends the fixed text-trace suffix to the source path.
func DefaultOutputPath(input string) string {
	return input + defaultOutputSuffix
}

func (r Result) Summary() string {
	return fmt.Sprintf("%s -> %s, %d events, %d missing format(s), %d unknown event(s)",
		r.InputPath, r.OutputPath, r.EventsWritten, r.MissingFormatCount, r.UnknownEventCount)
}
