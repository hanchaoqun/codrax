package mcp

import (
	"bytes"
	"encoding/json"
	"fmt"
	"strconv"
	"strings"

	"github.com/hanchaoqun/codrax/internal/types"
)

const (
	typedObservationVersion = "codrax.mcp.observation.v1"
	typedObservationMIME    = "application/vnd.codrax.observation+json"
	maxTypedObservations    = 256
)

type decodedMCPResult struct {
	Summary      string
	MIMEType     string
	IsError      bool
	Observations []types.MCPTypedObservation
}

type rawObservationEnvelope struct {
	Version      string                   `json:"version"`
	Summary      string                   `json:"summary"`
	RawRef       string                   `json:"raw_ref"`
	PayloadRef   string                   `json:"payload_ref"`
	RowSetRef    string                   `json:"row_set_ref"`
	PageRef      string                   `json:"page_ref"`
	ResourceURI  string                   `json:"resource_uri"`
	MIMEType     string                   `json:"mime_type"`
	MIMETypeAlt  string                   `json:"mimeType"`
	JSONPointer  string                   `json:"json_pointer"`
	Selector     string                   `json:"selector"`
	Row          flexibleInt              `json:"row"`
	LineStart    flexibleInt              `json:"line_start"`
	LineEnd      flexibleInt              `json:"line_end"`
	Observations []rawObservationEnvelope `json:"observations"`
}

type flexibleInt int

func (n *flexibleInt) UnmarshalJSON(raw []byte) error {
	raw = bytes.TrimSpace(raw)
	if len(raw) == 0 || bytes.Equal(raw, []byte("null")) {
		*n = 0
		return nil
	}
	var number json.Number
	if err := json.Unmarshal(raw, &number); err == nil {
		if i, err := strconv.Atoi(number.String()); err == nil {
			*n = flexibleInt(i)
			return nil
		}
	}
	var s string
	if err := json.Unmarshal(raw, &s); err == nil {
		s = strings.TrimSpace(s)
		if s == "" {
			*n = 0
			return nil
		}
		i, err := strconv.Atoi(s)
		if err != nil {
			return fmt.Errorf("invalid integer string %q", s)
		}
		*n = flexibleInt(i)
		return nil
	}
	return fmt.Errorf("invalid integer value %s", string(raw))
}

func parseTypedObservationEnvelope(payload, mimeType, fallbackResourceURI string) ([]types.MCPTypedObservation, string, bool) {
	payload = strings.TrimSpace(payload)
	if payload == "" || !strings.HasPrefix(payload, "{") {
		return nil, "", false
	}
	var env rawObservationEnvelope
	dec := json.NewDecoder(strings.NewReader(payload))
	dec.UseNumber()
	if err := dec.Decode(&env); err != nil {
		return nil, "", false
	}
	activated := isTypedObservationMIME(mimeType) || strings.TrimSpace(env.Version) == typedObservationVersion
	if !activated {
		return nil, "", false
	}
	observations := typedObservationsFromEnvelope(env, fallbackResourceURI)
	if len(observations) > maxTypedObservations {
		observations = observations[:maxTypedObservations]
	}
	return observations, typedObservationEnvelopeSummary(env, observations), true
}

func isTypedObservationMIME(mimeType string) bool {
	mimeType = strings.TrimSpace(strings.ToLower(mimeType))
	if i := strings.IndexByte(mimeType, ';'); i >= 0 {
		mimeType = strings.TrimSpace(mimeType[:i])
	}
	return mimeType == typedObservationMIME
}

func typedObservationsFromEnvelope(env rawObservationEnvelope, fallbackResourceURI string) []types.MCPTypedObservation {
	if len(env.Observations) == 0 {
		if obs, ok := normalizeTypedObservation(env, rawObservationEnvelope{}, fallbackResourceURI); ok {
			return []types.MCPTypedObservation{obs}
		}
		return nil
	}
	out := make([]types.MCPTypedObservation, 0, len(env.Observations))
	for _, child := range env.Observations {
		if obs, ok := normalizeTypedObservation(child, env, fallbackResourceURI); ok {
			out = append(out, obs)
			if len(out) >= maxTypedObservations {
				break
			}
		}
	}
	return out
}

func normalizeTypedObservation(raw, parent rawObservationEnvelope, fallbackResourceURI string) (types.MCPTypedObservation, bool) {
	lineStart := firstPositiveFlexibleInt(raw.LineStart, parent.LineStart)
	lineEnd := firstPositiveFlexibleInt(raw.LineEnd, parent.LineEnd)
	if lineStart <= 0 {
		lineEnd = 0
	} else if lineEnd > 0 && lineEnd < lineStart {
		lineEnd = 0
	}
	obs := types.MCPTypedObservation{
		Summary:     firstNonEmpty(raw.Summary, parent.Summary),
		RawRef:      firstNonEmpty(raw.RawRef, parent.RawRef),
		PayloadRef:  firstNonEmpty(raw.PayloadRef, parent.PayloadRef),
		RowSetRef:   firstNonEmpty(raw.RowSetRef, parent.RowSetRef),
		PageRef:     firstNonEmpty(raw.PageRef, parent.PageRef),
		ResourceURI: firstNonEmpty(raw.ResourceURI, parent.ResourceURI, fallbackResourceURI),
		MIMEType:    firstNonEmpty(raw.MIMEType, raw.MIMETypeAlt, parent.MIMEType, parent.MIMETypeAlt),
		JSONPointer: firstNonEmpty(raw.JSONPointer, parent.JSONPointer),
		Selector:    firstNonEmpty(raw.Selector, parent.Selector),
		Row:         firstPositiveFlexibleInt(raw.Row, parent.Row),
		LineStart:   lineStart,
		LineEnd:     lineEnd,
	}
	if obs.Summary == "" && obs.RawRef == "" && obs.PayloadRef == "" &&
		obs.RowSetRef == "" && obs.PageRef == "" && obs.ResourceURI == "" &&
		obs.JSONPointer == "" && obs.Selector == "" && obs.Row == 0 &&
		obs.LineStart == 0 {
		return types.MCPTypedObservation{}, false
	}
	return obs, true
}

func typedObservationEnvelopeSummary(env rawObservationEnvelope, observations []types.MCPTypedObservation) string {
	if summary := strings.TrimSpace(env.Summary); summary != "" {
		return summary
	}
	if len(observations) == 0 {
		return ""
	}
	parts := make([]string, 0, minInt(len(observations), 4))
	for i, obs := range observations {
		if i >= 4 {
			break
		}
		label := strings.TrimSpace(obs.Summary)
		if label == "" {
			label = "typed MCP observation"
		}
		if obs.LineStart > 0 {
			if obs.LineEnd > obs.LineStart {
				label = fmt.Sprintf("%s @ lines %d-%d", label, obs.LineStart, obs.LineEnd)
			} else {
				label = fmt.Sprintf("%s @ line %d", label, obs.LineStart)
			}
		}
		parts = append(parts, label)
	}
	if extra := len(observations) - len(parts); extra > 0 {
		parts = append(parts, fmt.Sprintf("... %d more", extra))
	}
	return strings.Join(parts, "; ")
}

func firstPositiveFlexibleInt(values ...flexibleInt) int {
	for _, value := range values {
		if int(value) > 0 {
			return int(value)
		}
	}
	return 0
}

func minInt(a, b int) int {
	if a < b {
		return a
	}
	return b
}
