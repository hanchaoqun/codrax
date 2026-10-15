package tool

import (
	"bytes"
	"encoding/json"
	"fmt"

	"github.com/hanchaoqun/codrax/internal/types"
)

func traceQueryCatalogReferenceSchema(raw json.RawMessage) json.RawMessage {
	var s map[string]any
	if json.Unmarshal(raw, &s) != nil {
		return raw
	}
	p, _ := s["properties"].(map[string]any)
	p["catalog_query_ref"] = map[string]any{"type": "object", "additionalProperties": false, "required": []string{"catalog_id", "query_id"}, "properties": map[string]any{"catalog_id": map[string]any{"type": "string", "minLength": 1}, "query_id": map[string]any{"type": "string", "minLength": 1}}, "description": "Exact current trace_catalog plan. Supply this property alone: current host authorization, input generation and argument schema are checked again, then normal trace_query executes. Historical snapshots/payloads never substitute for execution."}
	// The validator executes if/then (not arbitrary oneOf). Keep ordinary
	// queries byte-for-byte in their existing branch; references allow one key.
	s["if"] = map[string]any{"required": []string{"catalog_query_ref"}}
	s["then"] = map[string]any{"properties": map[string]any{"catalog_query_ref": p["catalog_query_ref"]}, "additionalProperties": false}
	var b bytes.Buffer
	encoder := json.NewEncoder(&b)
	encoder.SetEscapeHTML(false)
	if err := encoder.Encode(s); err != nil {
		return raw
	}
	return b.Bytes()
}

// Expand only a live exact plan. Once identified, failures are attributed to
// that item; malformed/unknown references cannot create attempts on others.
func traceQueryResolveCatalogReference(ctx *types.BusContext, raw json.RawMessage) (json.RawMessage, []traceCatalogQueryTicket, error) {
	var obj map[string]json.RawMessage
	if json.Unmarshal(raw, &obj) != nil {
		return raw, nil, nil
	}
	ref, present := obj["catalog_query_ref"]
	if !present {
		return raw, nil, nil
	}
	if err := ToolArgumentEnvelopeIntegrityError("trace_query", raw, (&TraceQuery{}).Parameters()); err != nil {
		return nil, nil, err
	}
	if len(obj) != 1 {
		return nil, nil, fmt.Errorf("catalog_query_ref must be the only query property; edit or issue a separate ordinary query instead")
	}
	var r struct {
		CatalogID string `json:"catalog_id"`
		QueryID   string `json:"query_id"`
	}
	d := json.NewDecoder(bytes.NewReader(ref))
	d.DisallowUnknownFields()
	if err := d.Decode(&r); err != nil {
		return nil, nil, err
	}
	if ctx == nil || ctx.Mutable == nil || r.CatalogID == "" || r.QueryID == "" {
		return nil, nil, fmt.Errorf("catalog_query_ref requires exact current catalog_id and query_id")
	}
	for _, c := range ctx.Mutable.TraceCatalogs() {
		if c.ID() != r.CatalogID {
			continue
		}
		q, found := c.Query(r.QueryID)
		if !found {
			break
		}
		for _, a := range c.Snapshot().Artifacts {
			if a.ID != q.ArtifactID {
				continue
			}
			tickets := []traceCatalogQueryTicket{{catalog: c, artifact: a, queryID: q.ID, fixedPlan: true, admissionFailed: true, raw: append(json.RawMessage(nil), q.Parameters...)}}
			if _, err := traceCatalogAuthorizedRoot(ctx, a.Path); err != nil {
				return nil, tickets, err
			}
			if _, err := traceCatalogRevalidatePlan(q.Parameters, a.ID); err != nil {
				return nil, tickets, err
			}
			if _, err := c.Resolve(contextFromBus(ctx), a.ID); err != nil {
				return nil, tickets, err
			}
			args := map[string]json.RawMessage{}
			if err := json.Unmarshal(q.Parameters, &args); err != nil {
				return nil, tickets, err
			}
			args["source"] = json.RawMessage(`"path"`)
			args["path"], _ = json.Marshal(a.Path)
			expanded, err := json.Marshal(args)
			tickets[0].raw = expanded
			tickets[0].admissionFailed = false
			tickets[0].predeclared = c.IsDeclaredQuery(q.ID)
			tickets[0].resumedPlan = c.IsResumedQuery(q.ID)
			return expanded, tickets, err
		}
	}
	return nil, nil, fmt.Errorf("catalog_query_ref is not a planned query in this current run; use trace_catalog resume with an explicitly authorized root for history")
}
