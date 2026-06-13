package tool

import (
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/hanchaoqun/codrax/internal/types"
)

// EmitMultiRepoFocus records the lightweight selector's typed active-set
// recommendation. It is scope metadata only; it does not satisfy evidence
// gates and never touches source files.
type EmitMultiRepoFocus struct {
	ReadOnly
	NonEvidenceTool
}

type multiRepoFocusDecodeEnvelope struct {
	RecommendedFocusSubrepos json.RawMessage `json:"recommended_focus_subrepos,omitempty"`
	Candidates               json.RawMessage `json:"candidates,omitempty"`
	Confidence               float64         `json:"confidence"`
	Source                   string          `json:"source,omitempty"`
	Rationale                string          `json:"rationale,omitempty"`
}

func (t *EmitMultiRepoFocus) Name() string { return "emit_multi_repo_focus" }

func (t *EmitMultiRepoFocus) Description() string {
	return "Select which sub-repos should be active for this multi-repo question. Use only with compact topology input; do not read files. recommended_focus_subrepos may be objects or exact root_rel strings."
}

func (t *EmitMultiRepoFocus) Parameters() json.RawMessage {
	return json.RawMessage(`{
  "type": "object",
  "required": ["recommended_focus_subrepos", "confidence"],
  "properties": {
    "recommended_focus_subrepos": {
      "description": "One to five exact sub-repo root paths from the topology, or objects {root_rel, confidence, reason, source}. Do not invent paths.",
      "type": "array",
      "items": {
        "oneOf": [
          {"type": "string"},
          {
            "type": "object",
            "properties": {
              "root_rel": {"type": "string"},
              "slug": {"type": "string"},
              "confidence": {"type": "number"},
              "reason": {"type": "string"},
              "source": {"type": "string", "enum": ["model_recommended", "user_explicit_in_request"]}
            }
          }
        ]
      }
    },
    "confidence": {"type": "number", "description": "Overall confidence from 0.0 to 1.0."},
    "source": {"type": "string", "enum": ["model_recommended", "user_explicit_in_request"]},
    "rationale": {"type": "string", "description": "One short reason tied to the current request."}
  }
}`)
}

func emitMultiRepoFocusDecodeParameters() json.RawMessage {
	return json.RawMessage(`{
  "type": "object",
  "required": ["recommended_focus_subrepos", "confidence"],
  "properties": {
    "recommended_focus_subrepos": {
      "description": "One to five exact sub-repo root paths from the topology, or objects {root_rel, confidence, reason, source}. Do not invent paths.",
      "type": "array",
      "items": {
        "oneOf": [
          {"type": "string"},
          {
            "type": "object",
            "properties": {
              "root_rel": {"type": "string"},
              "slug": {"type": "string"},
              "confidence": {"type": "number"},
              "reason": {"type": "string"},
              "source": {"type": "string", "enum": ["model_recommended", "user_explicit_in_request"]}
            }
          }
        ]
      }
    },
    "candidates": {"description": "Deprecated backend-only alias for recommended_focus_subrepos.", "type": "array"},
    "confidence": {"type": "number", "description": "Overall confidence from 0.0 to 1.0."},
    "source": {"type": "string", "enum": ["model_recommended", "user_explicit_in_request"]},
    "rationale": {"type": "string", "description": "One short reason tied to the current request."}
  }
}`)
}

func (t *EmitMultiRepoFocus) Execute(ctx *types.BusContext, params json.RawMessage) (types.ToolResult, error) {
	start := time.Now()
	result := func(success bool, summary string) types.ToolResult {
		return types.ToolResult{ToolName: t.Name(), Success: success, Summary: summary, Timestamp: start}
	}
	if ctx == nil || ctx.Mutable == nil {
		return result(false, "emit_multi_repo_focus requires a writable run context"), nil
	}
	resolver, _ := ctx.MultiGraph.(types.MultiRepoFocusResolver)
	if resolver == nil || resolver.IsSingle() || resolver.MultiRepoSubRepoCount() < 2 {
		return result(false, "emit_multi_repo_focus rejected: no multi-repo topology is active"), nil
	}

	var envelope multiRepoFocusDecodeEnvelope
	normalized, decodeFailure, err := decodeStrictToolParams(t.Name(), params, emitMultiRepoFocusDecodeParameters(), &envelope, nil)
	if err != nil {
		return *decodeFailure, err
	}
	raw, err := parseMultiRepoFocusRaw(normalized)
	if err != nil {
		return result(false, "emit_multi_repo_focus rejected: "+err.Error()), nil
	}
	overall := raw.Confidence
	if overall <= 0 {
		overall = 0.5
	}
	if overall > 1 {
		overall = 1
	}
	source := raw.Source
	sourceWasImplicit := source == types.MultiRepoFocusSourceUnknown
	if source == types.MultiRepoFocusSourceUnknown {
		source = types.MultiRepoFocusSourceModelRecommended
	}
	if source != types.MultiRepoFocusSourceModelRecommended && source != types.MultiRepoFocusSourceUserExplicitInRequest {
		return result(false, "emit_multi_repo_focus rejected: source must be model_recommended or user_explicit_in_request"), nil
	}

	objective := ""
	if ctx.Mutable != nil {
		objective = ctx.Mutable.Objective()
	}
	seen := make(map[string]bool)
	candidates := make([]types.MultiRepoFocusCandidate, 0, len(raw.Candidates))
	var rejected []string
	for _, cand := range raw.Candidates {
		token := strings.TrimSpace(cand.RootRel)
		if token == "" {
			token = strings.TrimSpace(cand.Slug)
		}
		if token == "" {
			rejected = append(rejected, "empty root_rel")
			continue
		}
		rootRel, slug, ok := resolver.ResolveMultiRepoSubRepo(token)
		if !ok {
			rejected = append(rejected, fmt.Sprintf("%q not in topology", token))
			continue
		}
		candSource := cand.Source
		if candSource == types.MultiRepoFocusSourceUnknown {
			candSource = source
		}
		if candSource == types.MultiRepoFocusSourceUserExplicitInRequest && !objectiveMentionsRootRel(objective, rootRel) {
			rejected = append(rejected, fmt.Sprintf("%q source=user_explicit_in_request but current request does not exactly mention that root path", rootRel))
			continue
		}
		if seen[slug] {
			continue
		}
		seen[slug] = true
		conf := cand.Confidence
		if conf <= 0 {
			conf = overall
		}
		if conf > 1 {
			conf = 1
		}
		candidates = append(candidates, types.MultiRepoFocusCandidate{
			RootRel:    rootRel,
			Slug:       slug,
			Confidence: conf,
			Reason:     strings.TrimSpace(cand.Reason),
			Source:     candSource,
		})
	}
	if len(candidates) == 0 {
		advice := "use exact root_rel values from the topology, or omit user_explicit_in_request unless the path is literally present in the current request"
		if len(rejected) > 0 {
			advice += "; rejected: " + strings.Join(rejected, "; ")
		}
		return result(false, "emit_multi_repo_focus rejected: no valid candidates; "+advice), nil
	}
	if len(candidates) > resolver.Cap() {
		return result(false, fmt.Sprintf("emit_multi_repo_focus rejected: %d candidates exceed active cap %d; return the most relevant sub-repos only", len(candidates), resolver.Cap())), nil
	}
	if sourceWasImplicit {
		allExplicit := len(candidates) > 0
		for _, cand := range candidates {
			if cand.Source != types.MultiRepoFocusSourceUserExplicitInRequest {
				allExplicit = false
				break
			}
		}
		if allExplicit {
			source = types.MultiRepoFocusSourceUserExplicitInRequest
		}
	}
	decision := &types.MultiRepoFocusDecision{
		Source:     source,
		Confidence: overall,
		Candidates: candidates,
		Rationale:  strings.TrimSpace(raw.Rationale),
	}
	ctx.Mutable.SetMultiRepoFocusDecision(decision)
	rootRels := make([]string, 0, len(candidates))
	for _, c := range candidates {
		rootRels = append(rootRels, c.RootRel)
	}
	return result(true, fmt.Sprintf("multi-repo focus selected: %s (source=%s confidence=%.2f)", strings.Join(rootRels, ", "), source, overall)), nil
}

type multiRepoFocusRaw struct {
	Candidates []types.MultiRepoFocusCandidate
	Confidence float64
	Source     types.MultiRepoFocusSource
	Rationale  string
}

func parseMultiRepoFocusRaw(params json.RawMessage) (multiRepoFocusRaw, error) {
	var obj map[string]json.RawMessage
	if err := json.Unmarshal(params, &obj); err != nil {
		return multiRepoFocusRaw{}, err
	}
	var out multiRepoFocusRaw
	out.Confidence = parseJSONFloat(obj["confidence"])
	out.Source = types.MultiRepoFocusSource(parseJSONString(obj["source"]))
	out.Rationale = parseJSONString(obj["rationale"])
	rawCandidates := obj["recommended_focus_subrepos"]
	if len(rawCandidates) == 0 {
		rawCandidates = obj["candidates"]
	}
	cands, err := parseFocusCandidates(rawCandidates, out.Source, out.Confidence)
	if err != nil {
		return multiRepoFocusRaw{}, err
	}
	out.Candidates = cands
	return out, nil
}

func parseFocusCandidates(raw json.RawMessage, source types.MultiRepoFocusSource, confidence float64) ([]types.MultiRepoFocusCandidate, error) {
	if len(raw) == 0 {
		return nil, fmt.Errorf("recommended_focus_subrepos is required")
	}
	var arr []json.RawMessage
	if err := json.Unmarshal(raw, &arr); err != nil {
		if s := parseJSONString(raw); s != "" {
			parts := strings.Split(s, ",")
			out := make([]types.MultiRepoFocusCandidate, 0, len(parts))
			for _, part := range parts {
				if trimmed := strings.TrimSpace(part); trimmed != "" {
					out = append(out, types.MultiRepoFocusCandidate{RootRel: trimmed, Source: source, Confidence: confidence})
				}
			}
			return out, nil
		}
		return nil, err
	}
	out := make([]types.MultiRepoFocusCandidate, 0, len(arr))
	for _, item := range arr {
		if s := parseJSONString(item); s != "" {
			out = append(out, types.MultiRepoFocusCandidate{RootRel: s, Source: source, Confidence: confidence})
			continue
		}
		var cand types.MultiRepoFocusCandidate
		if err := json.Unmarshal(item, &cand); err != nil {
			return nil, err
		}
		if cand.Source == types.MultiRepoFocusSourceUnknown {
			cand.Source = source
		}
		if cand.Confidence <= 0 {
			cand.Confidence = confidence
		}
		out = append(out, cand)
	}
	return out, nil
}

func parseJSONString(raw json.RawMessage) string {
	if len(raw) == 0 {
		return ""
	}
	var s string
	if err := json.Unmarshal(raw, &s); err == nil {
		return strings.TrimSpace(s)
	}
	return ""
}

func parseJSONFloat(raw json.RawMessage) float64 {
	if len(raw) == 0 {
		return 0
	}
	var f float64
	if err := json.Unmarshal(raw, &f); err == nil {
		return f
	}
	var s string
	if err := json.Unmarshal(raw, &s); err == nil {
		if parsed, err := strconv.ParseFloat(strings.TrimSpace(s), 64); err == nil {
			return parsed
		}
	}
	return 0
}

func objectiveMentionsRootRel(objective, rootRel string) bool {
	rootRel = strings.Trim(strings.ReplaceAll(rootRel, "\\", "/"), "/")
	if rootRel == "" || rootRel == "." {
		return false
	}
	normalised := strings.ReplaceAll(objective, "\\", "/")
	return strings.Contains(normalised, rootRel)
}
