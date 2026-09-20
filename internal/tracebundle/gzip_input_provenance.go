package tracebundle

import (
	"fmt"
	"unicode"
	"unicode/utf8"
)

const (
	// GzipInputProfileV1 identifies the non-causal, top-level gzip transport
	// receipt. It is distinct from a standalone perf artifact transform.
	GzipInputProfileV1 = "gzip_input_v1"

	gzipInputMaxBytes           int64 = 64 << 30
	gzipInputMaxExpansionRatio  int64 = 1000
	gzipInputGenerationMaxBytes       = 1024
)

// GzipInputProvenance records the outer source and the exact decoded bytes
// admitted by a converter. It does not add capture members, clock authority,
// query capabilities, or paths to a consumer's physical source universe.
// Generation strings are opaque producer observations, not identity proofs
// that a consumer can authenticate or use to reopen either file.
type GzipInputProvenance struct {
	Profile           string `json:"profile"`
	SourceBytes       int64  `json:"source_bytes"`
	SourceSHA256      string `json:"source_sha256"`
	SourceGeneration  string `json:"source_generation"`
	DecodedFormat     string `json:"decoded_format"`
	DecodedBytes      int64  `json:"decoded_bytes"`
	DecodedSHA256     string `json:"decoded_sha256"`
	DecodedGeneration string `json:"decoded_generation"`
}

// ValidateGzipInputProvenance checks the closed transport tuple. Absence is
// allowed. Passing this syntax check does not prove either file's identity;
// producers must bind each observation to its held strong file generation.
func ValidateGzipInputProvenance(value *GzipInputProvenance) error {
	if value == nil {
		return nil
	}
	if value.Profile != GzipInputProfileV1 {
		return fmt.Errorf("profile must be exact %s", GzipInputProfileV1)
	}
	if value.SourceBytes <= 0 || value.SourceBytes > gzipInputMaxBytes {
		return fmt.Errorf("source_bytes must be positive and at most %d", gzipInputMaxBytes)
	}
	if value.DecodedBytes <= 0 || value.DecodedBytes > gzipInputMaxBytes {
		return fmt.Errorf("decoded_bytes must be positive and at most %d", gzipInputMaxBytes)
	}
	// SourceBytes has already been bounded, so multiplication cannot overflow.
	if value.DecodedBytes > value.SourceBytes*gzipInputMaxExpansionRatio {
		return fmt.Errorf("decoded_bytes exceeds the %d:1 gzip expansion limit", gzipInputMaxExpansionRatio)
	}
	if err := ValidateSHA256(value.SourceSHA256); err != nil {
		return fmt.Errorf("source_sha256: %w", err)
	}
	if err := ValidateSHA256(value.DecodedSHA256); err != nil {
		return fmt.Errorf("decoded_sha256: %w", err)
	}
	if err := validateGzipInputGeneration(value.SourceGeneration); err != nil {
		return fmt.Errorf("source_generation: %w", err)
	}
	if err := validateGzipInputGeneration(value.DecodedGeneration); err != nil {
		return fmt.Errorf("decoded_generation: %w", err)
	}
	// Keep the wire tokens here rather than depending on attachment, whose
	// higher-level consumers may already depend on tracebundle.
	switch value.DecodedFormat {
	case "harmony_rmq", "openharmony_profiler", "linux_perf_data", "simpleperf_report_sample_proto", "openharmony_raw":
	default:
		return fmt.Errorf("decoded_format is outside the supported binary format set")
	}
	return nil
}

func validateGzipInputGeneration(value string) error {
	if len(value) == 0 || len(value) > gzipInputGenerationMaxBytes || !utf8.ValidString(value) {
		return fmt.Errorf("must be a non-empty UTF-8 token of at most %d bytes", gzipInputGenerationMaxBytes)
	}
	for _, character := range value {
		if unicode.IsControl(character) || unicode.IsSpace(character) {
			return fmt.Errorf("must not contain control or whitespace characters")
		}
	}
	return nil
}

// CloneGzipInputProvenance detaches a receipt from the caller's mutable value.
func CloneGzipInputProvenance(value *GzipInputProvenance) *GzipInputProvenance {
	if value == nil {
		return nil
	}
	clone := *value
	return &clone
}
