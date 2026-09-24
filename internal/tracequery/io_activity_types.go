package tracequery

const IOActivityPopulationEndpointEvents = "observed_endpoint_events"

// IOActivityStats counts retained, structurally admitted physical endpoint
// events, independently of request pairing and issuer identity lifetimes.
// Different layers, phases and byte calibers are never one additive IO total.
// Neither counts nor byte rates establish waiting or causal eligibility.
type IOActivityStats struct {
	Window                  *IOActivityWindow  `json:"window,omitempty"`
	WindowUnavailableReason string             `json:"window_unavailable_reason,omitempty"`
	Population              string             `json:"population"`
	IssuerScope             string             `json:"issuer_scope"`
	QueryPID                int                `json:"query_pid,omitempty"`
	LineStart               int                `json:"line_start,omitempty"`
	LineEnd                 int                `json:"line_end,omitempty"`
	BucketMs                float64            `json:"bucket_ms"`
	GroupCount              int                `json:"group_count"`
	Groups                  []IOActivityGroup  `json:"groups,omitempty"`
	OmittedGroups           int                `json:"omitted_groups"`
	Coverage                IOActivityCoverage `json:"coverage"`
}

// Bounds are seconds on the native trace axis, with a half-open selection.
type IOActivityWindow struct {
	StartTs float64 `json:"start_ts"`
	EndTs   float64 `json:"end_ts"`
	// Only the default captured-extent selection includes its last observed
	// endpoint. Explicit positive-width user time windows remain half-open.
	EndInclusive bool `json:"end_inclusive,omitempty"`
}

type IOActivityCoverage struct {
	SupportedEndpointCount int      `json:"supported_endpoint_count"`
	RejectedEndpointCount  int      `json:"rejected_endpoint_count"`
	UnresolvedSourceCount  int      `json:"unresolved_source_count"`
	Reasons                []string `json:"reasons,omitempty"`
}

type IOActivityGroup struct {
	SourcePath               string                    `json:"source_path"`
	Layer                    string                    `json:"layer"`
	EndpointFamily           string                    `json:"endpoint_family"`
	Phase                    string                    `json:"phase"`
	Dev                      string                    `json:"dev"`
	ByteCaliber              string                    `json:"byte_caliber"`
	Values                   IOActivityValues          `json:"values"`
	Rates                    *IOActivityRates          `json:"rates,omitempty"`
	Directions               []IOActivityDirection     `json:"directions"`
	ReadWrite                *IOActivityReadWriteRatio `json:"read_write,omitempty"`
	Buckets                  []IOActivityBucket        `json:"buckets,omitempty"`
	BucketCount              uint64                    `json:"bucket_count"`
	OmittedBuckets           uint64                    `json:"omitted_buckets"`
	BucketsUnavailableReason string                    `json:"buckets_unavailable_reason,omitempty"`
}

type IOActivityDirection struct {
	Direction string           `json:"direction"` // read, write, other
	Values    IOActivityValues `json:"values"`
	Rates     *IOActivityRates `json:"rates,omitempty"`
}

// The four byte-status counts partition EventCount. KnownBytes is the sum
// over known-size events only, not an estimate for unknown/invalid events.
// Nil means no known sizes or sum overflow; known zero is a non-nil zero.
type IOActivityValues struct {
	EventCount             int                    `json:"event_count"`
	KnownByteEventCount    int                    `json:"known_byte_event_count"`
	UnknownByteEventCount  int                    `json:"unknown_byte_event_count"`
	InvalidByteEventCount  int                    `json:"invalid_byte_event_count"`
	OverflowByteEventCount int                    `json:"overflow_byte_event_count"`
	KnownBytes             *uint64                `json:"known_bytes,omitempty"`
	BytesOverflow          bool                   `json:"bytes_overflow"`
	KnownSizeMeanBytes     *float64               `json:"known_size_mean_bytes,omitempty"`
	SizeBuckets            []IOActivitySizeBucket `json:"size_buckets"`
}

// Half-open size bands in bytes. An absent upper bound is unbounded.
// They describe request/transfer sizes, never random/sequential access.
type IOActivitySizeBucket struct {
	MinBytes uint64  `json:"min_bytes"`
	MaxBytes *uint64 `json:"max_bytes,omitempty"`
	Count    int     `json:"count"`
}

// Rates use the selected wall-clock width (including idle time), or the
// bucket's actual width, including a short final bucket. No duration sums.
type IOActivityRates struct {
	EventsPerSecond     float64  `json:"events_per_second"`
	KnownBytesPerSecond *float64 `json:"known_bytes_per_second,omitempty"`
}

// Read/write excludes other operations. Event share uses R+W event counts;
// byte share uses only known R+W bytes of this exact group/caliber. Nil is a
// zero/unavailable denominator, never an invented zero-percent measurement.
type IOActivityReadWriteRatio struct {
	EventDenominator     int      `json:"event_denominator"`
	ReadEventShare       *float64 `json:"read_event_share,omitempty"`
	WriteEventShare      *float64 `json:"write_event_share,omitempty"`
	KnownByteDenominator *uint64  `json:"known_byte_denominator,omitempty"`
	ReadKnownByteShare   *float64 `json:"read_known_byte_share,omitempty"`
	WriteKnownByteShare  *float64 `json:"write_known_byte_share,omitempty"`
}

// Empty wall-clock buckets are retained for each observed group. Buckets
// are a bounded chronological prefix; summaries use the complete population.
type IOActivityBucket struct {
	Window     IOActivityWindow      `json:"window"`
	Values     IOActivityValues      `json:"values"`
	Rates      *IOActivityRates      `json:"rates,omitempty"`
	Directions []IOActivityDirection `json:"directions"`
}
