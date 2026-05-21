package types

import (
	"fmt"
	"strconv"
	"strings"
)

// ObservationSourceKind names the physical namespace of an observation's source
// reference. It is intentionally finer than AnswerEvidenceOrigin: an MCP
// resource and a connector response may both be external-resource evidence, but
// their address shapes and safety rules differ.
type ObservationSourceKind string

const (
	ObservationSourceUnknown          ObservationSourceKind = ""
	ObservationSourceCurrentSource    ObservationSourceKind = "current_source"
	ObservationSourceVCSMetadata      ObservationSourceKind = "vcs_metadata"
	ObservationSourceVCSDiff          ObservationSourceKind = "vcs_diff"
	ObservationSourceRuntimeArtifact  ObservationSourceKind = "runtime_artifact"
	ObservationSourceCommand          ObservationSourceKind = "command"
	ObservationSourceCrossRepoIndex   ObservationSourceKind = "cross_repo_index"
	ObservationSourceExternalDocument ObservationSourceKind = "external_document"
	ObservationSourceWebPage          ObservationSourceKind = "web_page"
	ObservationSourceMCPResource      ObservationSourceKind = "mcp_resource"
	ObservationSourceConnector        ObservationSourceKind = "connector_resource"
)

// ObservationSourceRef is the origin-specific address of the thing that was
// observed. Only the fields that apply to SourceKind should be populated.
type ObservationSourceRef struct {
	Kind         ObservationSourceKind `json:"kind,omitempty"`
	Repo         string                `json:"repo,omitempty"`
	Path         string                `json:"path,omitempty"`
	Commit       string                `json:"commit,omitempty"`
	Range        string                `json:"range,omitempty"`
	Pathspec     string                `json:"pathspec,omitempty"`
	Command      string                `json:"command,omitempty"`
	ToolCallID   string                `json:"tool_call_id,omitempty"`
	RawRef       string                `json:"raw_ref,omitempty"`
	ArtifactID   string                `json:"artifact_id,omitempty"`
	ArtifactKind string                `json:"artifact_kind,omitempty"`
	URL          string                `json:"url,omitempty"`
	FetchedAt    string                `json:"fetched_at,omitempty"`
	Server       string                `json:"server,omitempty"`
	ResourceURI  string                `json:"resource_uri,omitempty"`
	MIMEType     string                `json:"mime_type,omitempty"`
	Connector    string                `json:"connector,omitempty"`
}

// ObservationSpan locates the observation inside SourceRef when that source has
// an addressable interior. It supports current source lines, artifact-local
// lines, VCS hunk coordinates, web/resource selectors, JSON pointers, table
// rows, and trace time ranges without overloading repo citations.
type ObservationSpan struct {
	LineStart   int     `json:"line_start,omitempty"`
	LineEnd     int     `json:"line_end,omitempty"`
	OldLine     int     `json:"old_line,omitempty"`
	NewLine     int     `json:"new_line,omitempty"`
	HunkHeader  string  `json:"hunk_header,omitempty"`
	Paragraph   int     `json:"paragraph,omitempty"`
	Selector    string  `json:"selector,omitempty"`
	JSONPointer string  `json:"json_pointer,omitempty"`
	Row         int     `json:"row,omitempty"`
	TextStart   int     `json:"text_start,omitempty"`
	TextEnd     int     `json:"text_end,omitempty"`
	StartTsMs   float64 `json:"start_ts_ms,omitempty"`
	EndTsMs     float64 `json:"end_ts_ms,omitempty"`
}

// ObservationRecord is the normalized, read-only fact surface compiled from
// accepted producer outputs. It is not a replacement for EvidenceItem; source
// evidence remains the only lane that may become repo citations. The ledger
// gives non-code facts an equally typed route to finalizer/reviewer consumers.
type ObservationRecord struct {
	ID              string               `json:"id"`
	Origin          AnswerEvidenceOrigin `json:"origin"`
	Producer        string               `json:"producer,omitempty"`
	Role            AnswerAggregateRole  `json:"role,omitempty"`
	GroundingPolicy ClaimGroundingPolicy `json:"grounding_policy,omitempty"`
	SourceRef       ObservationSourceRef `json:"source_ref,omitempty"`
	Span            ObservationSpan      `json:"span,omitempty"`
	ClaimKey        string               `json:"claim_key,omitempty"`
	Subject         string               `json:"subject,omitempty"`
	Predicate       string               `json:"predicate,omitempty"`
	Object          string               `json:"object,omitempty"`
	Value           string               `json:"value,omitempty"`
	Unit            string               `json:"unit,omitempty"`
	Negative        bool                 `json:"negative,omitempty"`
	ResultCount     *int                 `json:"result_count,omitempty"`
	Summary         string               `json:"summary,omitempty"`
	RawExcerpt      string               `json:"raw_excerpt,omitempty"`
	RichNotes       []string             `json:"rich_notes,omitempty"`
	SupportRefs     []string             `json:"support_refs,omitempty"`
	ObservedAt      string               `json:"observed_at,omitempty"`
	Scope           string               `json:"scope,omitempty"`
	Confidence      float64              `json:"confidence,omitempty"`
}

// ObservationLedger is the deterministic, compiled fact ledger for a run. Later
// batches will populate it from the existing carriers and make finalizer/reviewer
// consume it first. Keeping the type in internal/types lets all producers depend
// on the same contract without importing tool or agent packages.
type ObservationLedger struct {
	Records []ObservationRecord `json:"records,omitempty"`
}

func (l ObservationLedger) Empty() bool {
	return len(l.Records) == 0
}

// ObservationLedgerInput carries existing accepted producer outputs into the
// ledger compiler. The compiler is intentionally side-effect free and must not
// inspect raw user prose or model free text to classify facts.
type ObservationLedgerInput struct {
	EvidenceItems  []EvidenceItem
	AggregateFacts []AnswerAggregateFact
	ToolResults    []ToolResult
	LogBundle      *LogBundle
	PerfBundle     *PerfBundle
	MCPResponses   []MCPResponse
	RequestModel   *RequestModel
	AnswerContract *AnswerContract
}

// CompileObservationLedger projects accepted producer outputs into one read-only
// ledger. It deliberately consumes structured fields and system-authored tool
// origin banners only; raw user prose and model free text must not classify
// records.
func CompileObservationLedger(input ObservationLedgerInput) ObservationLedger {
	var out []ObservationRecord
	add := func(record ObservationRecord) {
		if record.Origin == AnswerEvidenceOriginUnknown || !record.Origin.IsValid() {
			return
		}
		if record.ID == "" {
			record.ID = fmt.Sprintf("obs:%03d", len(out)+1)
		}
		if record.SourceRef.Kind == ObservationSourceUnknown {
			record.SourceRef.Kind = ObservationSourceKindForOrigin(record.Origin)
		}
		if record.GroundingPolicy == ClaimGroundingUnknown {
			record.GroundingPolicy = AnswerClaimBindingGroundingPolicy(record.Origin, record.Role)
		}
		out = append(out, record)
	}
	compileEvidenceItemObservations(input.EvidenceItems, add)
	compileAggregateFactObservations(input.AggregateFacts, input.RequestModel, add)
	compileToolResultObservations(input.ToolResults, add)
	compileLogBundleObservations(input.LogBundle, add)
	compilePerfBundleObservations(input.PerfBundle, add)
	compileMCPResponseObservations(input.MCPResponses, add)
	return ObservationLedger{Records: out}
}

func ObservationSourceKindForOrigin(origin AnswerEvidenceOrigin) ObservationSourceKind {
	switch origin {
	case AnswerEvidenceOriginCurrentSource:
		return ObservationSourceCurrentSource
	case AnswerEvidenceOriginVCSMetadata:
		return ObservationSourceVCSMetadata
	case AnswerEvidenceOriginVCSDiff:
		return ObservationSourceVCSDiff
	case AnswerEvidenceOriginRuntimeArtifact:
		return ObservationSourceRuntimeArtifact
	case AnswerEvidenceOriginCommandMeasurement:
		return ObservationSourceCommand
	case AnswerEvidenceOriginRepoNegativeSearch:
		return ObservationSourceCurrentSource
	case AnswerEvidenceOriginCrossRepoIndex:
		return ObservationSourceCrossRepoIndex
	case AnswerEvidenceOriginExternalDocument:
		return ObservationSourceExternalDocument
	case AnswerEvidenceOriginWebPage:
		return ObservationSourceWebPage
	case AnswerEvidenceOriginMCPResource:
		return ObservationSourceMCPResource
	case AnswerEvidenceOriginConnectorResource:
		return ObservationSourceConnector
	default:
		return ObservationSourceUnknown
	}
}

func compileEvidenceItemObservations(items []EvidenceItem, add func(ObservationRecord)) {
	for i, ev := range items {
		if strings.TrimSpace(ev.Source) == "" && strings.TrimSpace(ev.Summary) == "" {
			continue
		}
		role := AnswerAggregateRoleSupportingCoverage
		if ev.Salience == SalienceLoadBearing || ev.Salience == SalienceExhaustListed {
			role = AnswerAggregateRolePrincipalAnswer
		}
		id := strings.TrimSpace(ev.ID)
		if id == "" {
			id = fmt.Sprintf("evidence:%d", i)
		} else {
			id = "evidence:" + id
		}
		add(ObservationRecord{
			ID:              id,
			Origin:          AnswerEvidenceOriginCurrentSource,
			Producer:        firstNonEmptyString(ev.Producer, "evidence_item"),
			Role:            role,
			GroundingPolicy: AnswerClaimBindingGroundingPolicy(AnswerEvidenceOriginCurrentSource, role),
			SourceRef: ObservationSourceRef{
				Kind: ObservationSourceCurrentSource,
				Path: strings.TrimSpace(ev.Source),
			},
			Span: ObservationSpan{
				LineStart: ev.LineStart,
				LineEnd:   ev.LineEnd,
			},
			ClaimKey:    firstNonEmptyString(ev.AnchorSymbol, ev.Subject, ev.ID),
			Subject:     strings.TrimSpace(ev.Subject),
			Predicate:   strings.TrimSpace(ev.Predicate),
			Object:      strings.TrimSpace(ev.Object),
			Summary:     strings.TrimSpace(ev.Summary),
			RawExcerpt:  strings.TrimSpace(ev.Snippet),
			SupportRefs: cloneStringSlice(ev.SurfaceTerms),
			Confidence:  ev.Confidence,
		})
	}
}

func compileAggregateFactObservations(facts []AnswerAggregateFact, rm *RequestModel, add func(ObservationRecord)) {
	for i, fact := range facts {
		role := AnswerAggregateFactRoleForRequest(fact, rm)
		origins := AnswerAggregateFactEvidenceOrigins(fact, rm)
		if len(origins) == 0 {
			origins = []AnswerEvidenceOrigin{AnswerEvidenceOriginCurrentSource}
		}
		dims := aggregateDimensionMap(fact.Dimensions)
		for _, origin := range origins {
			resultCount := aggregateFactResultCount(fact, dims)
			add(ObservationRecord{
				ID:              fmt.Sprintf("aggregate:%d#%s", i, origin),
				Origin:          origin,
				Producer:        firstNonEmptyString(fact.Provenance, "aggregate_facts"),
				Role:            role,
				GroundingPolicy: AnswerClaimBindingGroundingPolicy(origin, role),
				SourceRef:       sourceRefForAggregateFact(origin, dims),
				ClaimKey:        firstNonEmptyString(dims["target"], dims["query"], dims["pattern"], dims["predicate"], fact.Label),
				Subject:         firstNonEmptyString(dims["target"], dims["query"], dims["pattern"], fact.Label),
				Predicate:       dims["predicate"],
				Value:           strings.TrimSpace(fact.Value),
				Unit:            strings.TrimSpace(fact.Unit),
				Negative:        fact.Kind == AnswerAggregateNegativeSearch || fact.Kind == AnswerAggregateNegativeObservation,
				ResultCount:     resultCount,
				Summary:         strings.TrimSpace(fact.Label),
				RichNotes:       cloneStringSlice(fact.Members),
				SupportRefs:     cloneStringSlice(fact.SupportRefs),
				ObservedAt:      dims["searched_at"],
				Scope:           dims["scope"],
			})
		}
	}
}

func compileToolResultObservations(results []ToolResult, add func(ObservationRecord)) {
	for i, result := range results {
		if !result.Success {
			continue
		}
		origins := toolResultEvidenceOrigins(result)
		for _, origin := range origins {
			role := AnswerAggregateRoleSupportingCoverage
			add(ObservationRecord{
				ID:              fmt.Sprintf("tool:%d#%s", i, origin),
				Origin:          origin,
				Producer:        strings.TrimSpace(result.ToolName),
				Role:            role,
				GroundingPolicy: AnswerClaimBindingGroundingPolicy(origin, role),
				SourceRef: ObservationSourceRef{
					Kind:       ObservationSourceKindForOrigin(origin),
					ToolCallID: fmt.Sprintf("%s[%d]", strings.TrimSpace(result.ToolName), i),
					RawRef:     strings.TrimSpace(result.RawRef),
				},
				ClaimKey:   strings.TrimSpace(result.ToolName),
				Summary:    firstNonBannerLine(result.Summary),
				RawExcerpt: clippedObservationExcerpt(result.Summary),
				ObservedAt: result.Timestamp.Format("2006-01-02T15:04:05Z07:00"),
			})
		}
	}
}

func compileLogBundleObservations(bundle *LogBundle, add func(ObservationRecord)) {
	if bundle == nil {
		return
	}
	var errIndex int
	var walkErr func(LogError)
	walkErr = func(err LogError) {
		target := firstNonEmptyString(err.Type, err.Message)
		if target != "" {
			add(ObservationRecord{
				ID:              fmt.Sprintf("log:error:%d", errIndex),
				Origin:          AnswerEvidenceOriginRuntimeArtifact,
				Producer:        "log_triage",
				Role:            AnswerAggregateRolePrincipalAnswer,
				GroundingPolicy: AnswerClaimBindingGroundingPolicy(AnswerEvidenceOriginRuntimeArtifact, AnswerAggregateRolePrincipalAnswer),
				SourceRef: ObservationSourceRef{
					Kind:         ObservationSourceRuntimeArtifact,
					ArtifactKind: "log",
				},
				ClaimKey:    target,
				Subject:     target,
				Summary:     firstNonEmptyString(err.Message, err.Type),
				SupportRefs: logFrameRawRefs(err.Frames),
			})
			errIndex++
		}
		if err.Cause != nil {
			walkErr(*err.Cause)
		}
	}
	for _, err := range bundle.Errors {
		walkErr(err)
	}
	for i, obs := range bundle.Observations {
		add(ObservationRecord{
			ID:              fmt.Sprintf("log:observation:%d", i),
			Origin:          AnswerEvidenceOriginRuntimeArtifact,
			Producer:        "log_triage",
			Role:            AnswerAggregateRolePrincipalAnswer,
			GroundingPolicy: AnswerClaimBindingGroundingPolicy(AnswerEvidenceOriginRuntimeArtifact, AnswerAggregateRolePrincipalAnswer),
			SourceRef: ObservationSourceRef{
				Kind:         ObservationSourceRuntimeArtifact,
				ArtifactKind: "log",
			},
			Span: ObservationSpan{
				LineStart: obs.LineStart,
				LineEnd:   obs.LineEnd,
			},
			ClaimKey:   firstNonEmptyString(obs.Subject, string(obs.Kind)),
			Subject:    strings.TrimSpace(obs.Subject),
			Predicate:  string(obs.Kind),
			Summary:    strings.TrimSpace(obs.Summary),
			RawExcerpt: strings.TrimSpace(obs.Evidence),
			Confidence: obs.Confidence,
		})
	}
}

func compilePerfBundleObservations(bundle *PerfBundle, add func(ObservationRecord)) {
	if bundle == nil {
		return
	}
	for i, frame := range bundle.Frames {
		add(ObservationRecord{
			ID:              fmt.Sprintf("perf:frame:%d", i),
			Origin:          AnswerEvidenceOriginRuntimeArtifact,
			Producer:        "perf_trace",
			Role:            AnswerAggregateRolePrincipalAnswer,
			GroundingPolicy: AnswerClaimBindingGroundingPolicy(AnswerEvidenceOriginRuntimeArtifact, AnswerAggregateRolePrincipalAnswer),
			SourceRef:       ObservationSourceRef{Kind: ObservationSourceRuntimeArtifact, ArtifactKind: firstNonEmptyString(bundle.Meta.Source, "trace")},
			ClaimKey:        fmt.Sprintf("frame:%d", frame.FrameNo),
			Subject:         fmt.Sprintf("frame %d", frame.FrameNo),
			Value:           strconv.FormatFloat(frame.DurationMs, 'f', -1, 64),
			Unit:            "ms",
			Summary:         fmt.Sprintf("frame %d duration %gms", frame.FrameNo, frame.DurationMs),
			ObservedAt:      strconv.FormatFloat(frame.TsMs, 'f', -1, 64),
		})
	}
	for i, jank := range bundle.Janks {
		add(ObservationRecord{
			ID:              fmt.Sprintf("perf:jank:%d", i),
			Origin:          AnswerEvidenceOriginRuntimeArtifact,
			Producer:        "perf_trace",
			Role:            AnswerAggregateRolePrincipalAnswer,
			GroundingPolicy: AnswerClaimBindingGroundingPolicy(AnswerEvidenceOriginRuntimeArtifact, AnswerAggregateRolePrincipalAnswer),
			SourceRef:       ObservationSourceRef{Kind: ObservationSourceRuntimeArtifact, ArtifactKind: firstNonEmptyString(bundle.Meta.Source, "trace")},
			Span: ObservationSpan{
				StartTsMs: jank.StartTsMs,
				EndTsMs:   jank.StartTsMs + jank.DurationMs,
			},
			ClaimKey:  firstNonEmptyString(jank.TriggerSpan, "jank"),
			Subject:   firstNonEmptyString(jank.TriggerSpan, "jank"),
			Predicate: "duration",
			Value:     strconv.FormatFloat(jank.DurationMs, 'f', -1, 64),
			Unit:      "ms",
			Summary:   firstNonEmptyString(jank.Reason, "observed jank"),
			RichNotes: cloneStringSlice(jank.Tags),
		})
	}
	for i, stall := range bundle.Stalls {
		add(ObservationRecord{
			ID:              fmt.Sprintf("perf:stall:%d", i),
			Origin:          AnswerEvidenceOriginRuntimeArtifact,
			Producer:        "perf_trace",
			Role:            AnswerAggregateRolePrincipalAnswer,
			GroundingPolicy: AnswerClaimBindingGroundingPolicy(AnswerEvidenceOriginRuntimeArtifact, AnswerAggregateRolePrincipalAnswer),
			SourceRef: ObservationSourceRef{
				Kind:         ObservationSourceRuntimeArtifact,
				ArtifactKind: firstNonEmptyString(bundle.Meta.Source, "trace"),
				Path:         strings.TrimSpace(stall.File),
			},
			Span: ObservationSpan{
				LineStart: stall.Line,
				StartTsMs: stall.StartTsMs,
				EndTsMs:   stall.StartTsMs + stall.DurationMs,
			},
			ClaimKey:  firstNonEmptyString(stall.Symbol, stall.Kind, "stall"),
			Subject:   firstNonEmptyString(stall.Symbol, stall.Kind, "stall"),
			Predicate: "duration",
			Value:     strconv.FormatFloat(stall.DurationMs, 'f', -1, 64),
			Unit:      "ms",
			Summary:   firstNonEmptyString(stall.Kind, "observed stall"),
		})
	}
	if bundle.Startup != nil {
		add(ObservationRecord{
			ID:              "perf:startup",
			Origin:          AnswerEvidenceOriginRuntimeArtifact,
			Producer:        "perf_trace",
			Role:            AnswerAggregateRolePrincipalAnswer,
			GroundingPolicy: AnswerClaimBindingGroundingPolicy(AnswerEvidenceOriginRuntimeArtifact, AnswerAggregateRolePrincipalAnswer),
			SourceRef:       ObservationSourceRef{Kind: ObservationSourceRuntimeArtifact, ArtifactKind: firstNonEmptyString(bundle.Meta.Source, "trace")},
			ClaimKey:        firstNonEmptyString(bundle.Startup.Mode, "startup"),
			Subject:         firstNonEmptyString(bundle.Startup.Mode, "startup"),
			Predicate:       "launch_duration",
			Value:           strconv.FormatFloat(bundle.Startup.AppLaunchMs, 'f', -1, 64),
			Unit:            "ms",
			Summary:         firstNonEmptyString(bundle.Meta.Summary, "startup timing"),
		})
	}
	for i, obs := range bundle.Observations {
		add(ObservationRecord{
			ID:              fmt.Sprintf("perf:observation:%d", i),
			Origin:          AnswerEvidenceOriginRuntimeArtifact,
			Producer:        "perf_trace",
			Role:            AnswerAggregateRolePrincipalAnswer,
			GroundingPolicy: AnswerClaimBindingGroundingPolicy(AnswerEvidenceOriginRuntimeArtifact, AnswerAggregateRolePrincipalAnswer),
			SourceRef:       ObservationSourceRef{Kind: ObservationSourceRuntimeArtifact, ArtifactKind: firstNonEmptyString(bundle.Meta.Source, "trace")},
			Span: ObservationSpan{
				LineStart: obs.LineStart,
				LineEnd:   obs.LineEnd,
				StartTsMs: obs.StartTsMs,
				EndTsMs:   obs.EndTsMs,
			},
			ClaimKey:   firstNonEmptyString(obs.Subject, obs.Kind),
			Subject:    strings.TrimSpace(obs.Subject),
			Predicate:  strings.TrimSpace(obs.Kind),
			Value:      strconv.FormatFloat(obs.DurationMs, 'f', -1, 64),
			Unit:       "ms",
			Summary:    strings.TrimSpace(obs.Summary),
			RawExcerpt: strings.TrimSpace(obs.Evidence),
			RichNotes:  cloneStringSlice(obs.Tags),
			Confidence: obs.Confidence,
		})
	}
}

func compileMCPResponseObservations(responses []MCPResponse, add func(ObservationRecord)) {
	for i, response := range responses {
		if !response.Success {
			continue
		}
		add(ObservationRecord{
			ID:              fmt.Sprintf("mcp:%d", i),
			Origin:          AnswerEvidenceOriginMCPResource,
			Producer:        firstNonEmptyString(response.Method, "mcp"),
			Role:            AnswerAggregateRoleSupportingCoverage,
			GroundingPolicy: AnswerClaimBindingGroundingPolicy(AnswerEvidenceOriginMCPResource, AnswerAggregateRoleSupportingCoverage),
			SourceRef: ObservationSourceRef{
				Kind:   ObservationSourceMCPResource,
				Server: strings.TrimSpace(response.ServerName),
				RawRef: strings.TrimSpace(response.RawRef),
			},
			ClaimKey:   firstNonEmptyString(response.Method, response.ServerName),
			Summary:    firstLine(strings.TrimSpace(response.Summary)),
			RawExcerpt: clippedObservationExcerpt(response.Summary),
			ObservedAt: response.Timestamp.Format("2006-01-02T15:04:05Z07:00"),
		})
	}
}

func aggregateFactResultCount(fact AnswerAggregateFact, dims map[string]string) *int {
	for _, raw := range []string{dims["result_count"], fact.Value} {
		n, err := strconv.Atoi(strings.TrimSpace(raw))
		if err == nil && n >= 0 {
			return &n
		}
	}
	return nil
}

func sourceRefForAggregateFact(origin AnswerEvidenceOrigin, dims map[string]string) ObservationSourceRef {
	ref := ObservationSourceRef{Kind: ObservationSourceKindForOrigin(origin)}
	ref.RawRef = firstNonEmptyString(dims["raw_ref"], dims["blob_ref"], dims["source_blob"], dims["tool_raw_ref"])
	switch origin {
	case AnswerEvidenceOriginCurrentSource, AnswerEvidenceOriginRepoNegativeSearch:
		ref.Repo = dims["repo"]
		ref.Path = firstNonEmptyString(dims["path"], dims["scope"])
	case AnswerEvidenceOriginVCSMetadata, AnswerEvidenceOriginVCSDiff:
		ref.Repo = dims["repo"]
		ref.Commit = dims["commit"]
		ref.Range = firstNonEmptyString(dims["commit_range"], dims["range"])
		ref.Pathspec = firstNonEmptyString(dims["pathspec"], dims["diff_path"], dims["window_path"])
	case AnswerEvidenceOriginRuntimeArtifact:
		ref.ArtifactID = firstNonEmptyString(dims["artifact_id"], dims["trace_window"], dims["scope"])
		ref.ArtifactKind = firstNonEmptyString(dims["artifact_kind"], dims["origin"], "runtime_artifact")
	case AnswerEvidenceOriginCommandMeasurement:
		ref.Command = dims["command"]
		ref.ToolCallID = firstNonEmptyString(dims["tool_result"], dims["tool_call_id"])
	case AnswerEvidenceOriginCrossRepoIndex:
		ref.Repo = dims["repo"]
		ref.Path = dims["scope"]
	case AnswerEvidenceOriginExternalDocument:
		ref.ResourceURI = firstNonEmptyString(dims["source_ref"], dims["resource_uri"], dims["scope"])
	case AnswerEvidenceOriginWebPage:
		ref.URL = firstNonEmptyString(dims["url"], dims["source_ref"], dims["scope"])
		ref.FetchedAt = dims["fetched_at"]
	case AnswerEvidenceOriginMCPResource:
		ref.Server = dims["server"]
		ref.ResourceURI = firstNonEmptyString(dims["resource_uri"], dims["source_ref"], dims["scope"])
	case AnswerEvidenceOriginConnectorResource:
		ref.Connector = dims["connector"]
		ref.ResourceURI = firstNonEmptyString(dims["resource_uri"], dims["source_ref"], dims["scope"])
	}
	return ref
}

func toolResultEvidenceOrigins(result ToolResult) []AnswerEvidenceOrigin {
	seen := map[AnswerEvidenceOrigin]bool{}
	var out []AnswerEvidenceOrigin
	add := func(origin AnswerEvidenceOrigin) {
		if origin == AnswerEvidenceOriginUnknown || !origin.IsValid() || seen[origin] {
			return
		}
		seen[origin] = true
		out = append(out, origin)
	}
	for _, kv := range toolResultBanners(result.Summary) {
		for _, key := range []string{"origin", "evidence_origin", "secondary_origin", "diff_origin", "proof_source", "tool", "source", "measurement_kind", "measurement_origin"} {
			answerEvidenceOriginFromStructuredToken(kv[key], add)
		}
	}
	return out
}

func toolResultBanners(summary string) []map[string]string {
	var out []map[string]string
	for _, line := range strings.Split(summary, "\n") {
		line = strings.TrimSpace(line)
		if !strings.HasPrefix(line, "[") || !strings.HasSuffix(line, "]") {
			continue
		}
		body := strings.TrimSuffix(strings.TrimPrefix(line, "["), "]")
		colon := strings.Index(body, ":")
		if colon < 0 {
			continue
		}
		fields := strings.Fields(strings.TrimSpace(body[colon+1:]))
		kv := map[string]string{}
		for _, field := range fields {
			k, v, ok := strings.Cut(field, "=")
			if !ok {
				continue
			}
			kv[strings.ToLower(strings.TrimSpace(k))] = strings.TrimSpace(v)
		}
		if len(kv) > 0 {
			out = append(out, kv)
		}
	}
	return out
}

func logFrameRawRefs(frames []LogFrame) []string {
	out := make([]string, 0, len(frames))
	for _, frame := range frames {
		if raw := strings.TrimSpace(frame.Raw); raw != "" {
			out = append(out, raw)
			continue
		}
		if frame.File != "" && frame.Line > 0 {
			out = append(out, fmt.Sprintf("%s:%d", frame.File, frame.Line))
		}
	}
	return out
}

func firstNonEmptyString(values ...string) string {
	for _, value := range values {
		if trimmed := strings.TrimSpace(value); trimmed != "" {
			return trimmed
		}
	}
	return ""
}

func firstLine(s string) string {
	if idx := strings.IndexByte(s, '\n'); idx >= 0 {
		return strings.TrimSpace(s[:idx])
	}
	return strings.TrimSpace(s)
}

func firstNonBannerLine(s string) string {
	for _, line := range strings.Split(s, "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		if strings.HasPrefix(line, "[") && strings.HasSuffix(line, "]") {
			continue
		}
		return line
	}
	return firstLine(s)
}

func clippedObservationExcerpt(s string) string {
	const max = 600
	s = strings.TrimSpace(s)
	if len([]rune(s)) <= max {
		return s
	}
	runes := []rune(s)
	return string(runes[:max]) + "...[truncated]"
}

func cloneStringSlice(in []string) []string {
	if len(in) == 0 {
		return nil
	}
	out := make([]string, len(in))
	copy(out, in)
	return out
}
