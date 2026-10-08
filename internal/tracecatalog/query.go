package tracecatalog

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"sort"
	"strings"
	"unicode/utf8"

	"github.com/hanchaoqun/codrax/internal/filegeneration"
)

const (
	MaxParameterBytes = 32768
	MaxQueries        = 4096
	MaxAttempts       = 16384
	MaxTextBytes      = 4096
)

// RegisterPlannedQuery freezes expected work before any execution. Repeating
// the same plan is idempotent. It neither reads payloads nor executes a query.
func (c *Catalog) RegisterPlannedQuery(ctx context.Context, plan Plan) (QueryRecord, error) {
	if err := ctx.Err(); err != nil {
		return QueryRecord{}, err
	}
	if c == nil {
		return QueryRecord{}, ErrUnknownArtifact
	}
	normalized, err := normalizePlan(plan)
	if err != nil {
		return QueryRecord{}, err
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	a, ok := c.artifacts[plan.ArtifactID]
	if !ok {
		return QueryRecord{}, ErrUnknownArtifact
	}
	q := QueryRecord{ArtifactRevision: a.RevisionID, Plan: normalized, Outcome: OutcomeNotExecuted}
	q.ID = queryID(q)
	if old, ok := c.queries[q.ID]; ok {
		return cloneQuery(old), nil
	}
	if len(c.queries) >= MaxQueries {
		return QueryRecord{}, fmt.Errorf("trace catalog query limit reached")
	}
	c.queries[q.ID] = q
	return cloneQuery(q), nil
}

// RegisterDeclaredQuery is for host-validated discovery-time expectations.
// An ordinary actual-query registration cannot acquire this private marker by
// repeating a prior call; serialization contains no field that can restore it.
func (c *Catalog) RegisterDeclaredQuery(ctx context.Context, plan Plan) (QueryRecord, error) {
	q, err := c.RegisterPlannedQuery(ctx, plan)
	if err != nil {
		return QueryRecord{}, err
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.declared == nil {
		c.declared = map[string]bool{}
	}
	c.declared[q.ID] = true
	return q, nil
}

func (c *Catalog) IsDeclaredQuery(id string) bool {
	if c == nil {
		return false
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.declared[id]
}

func queryID(q QueryRecord) string {
	return digest("query_", struct {
		Revision string
		Plan     Plan
	}{q.ArtifactRevision, q.Plan})
}

func normalizePlan(p Plan) (Plan, error) {
	for _, value := range []string{p.ArtifactID, p.Object.Kind, p.Object.ID, p.View} {
		if value == "" || len(value) > MaxTextBytes || !utf8.ValidString(value) {
			return Plan{}, fmt.Errorf("artifact, object identity and view must be nonempty bounded UTF-8")
		}
	}
	if len(p.Object.Scope) > MaxTextBytes || !utf8.ValidString(p.Object.Scope) {
		return Plan{}, fmt.Errorf("object scope is invalid")
	}
	if p.Window != nil {
		w := *p.Window
		if w.EndNS < w.StartNS || (w.EndNS == w.StartNS && (w.EndInclusive == nil || !*w.EndInclusive)) {
			return Plan{}, fmt.Errorf("query window is reversed or empty")
		}
		if w.EndInclusive != nil {
			value := *w.EndInclusive
			w.EndInclusive = &value
		}
		p.Window = &w
	}
	if len(p.Parameters) == 0 {
		p.Parameters = json.RawMessage(`{}`)
	}
	if len(p.Parameters) > MaxParameterBytes || !utf8.Valid(p.Parameters) {
		return Plan{}, fmt.Errorf("query parameters exceed bounded UTF-8 JSON")
	}
	decoder := json.NewDecoder(bytes.NewReader(p.Parameters))
	decoder.UseNumber()
	value, err := canonicalValue(decoder, 0)
	if err != nil {
		return Plan{}, fmt.Errorf("invalid query parameters: %w", err)
	}
	if _, ok := value.(map[string]any); !ok {
		return Plan{}, fmt.Errorf("query parameters must be an object")
	}
	if _, err := decoder.Token(); err != io.EOF {
		return Plan{}, fmt.Errorf("query parameters have trailing data")
	}
	p.Parameters, err = json.Marshal(value)
	if err != nil {
		return Plan{}, err
	}
	return p, nil
}

// Preserve JSON number spelling/precision, but normalize member ordering and
// whitespace. Reject duplicate keys rather than selecting an arbitrary value.
func canonicalValue(d *json.Decoder, depth int) (any, error) {
	if depth > 64 {
		return nil, fmt.Errorf("JSON nesting exceeds 64")
	}
	token, err := d.Token()
	if err != nil {
		return nil, err
	}
	delim, structured := token.(json.Delim)
	if !structured {
		return token, nil
	}
	switch delim {
	case '{':
		out := map[string]any{}
		for d.More() {
			keyToken, err := d.Token()
			if err != nil {
				return nil, err
			}
			key, ok := keyToken.(string)
			if !ok {
				return nil, fmt.Errorf("invalid object key")
			}
			if _, exists := out[key]; exists {
				return nil, fmt.Errorf("duplicate object key %q", key)
			}
			value, err := canonicalValue(d, depth+1)
			if err != nil {
				return nil, err
			}
			out[key] = value
		}
		close, err := d.Token()
		if err != nil || close != json.Delim('}') {
			return nil, fmt.Errorf("object is not closed")
		}
		return out, nil
	case '[':
		out := []any{}
		for d.More() {
			value, err := canonicalValue(d, depth+1)
			if err != nil {
				return nil, err
			}
			out = append(out, value)
		}
		close, err := d.Token()
		if err != nil || close != json.Delim(']') {
			return nil, fmt.Errorf("array is not closed")
		}
		return out, nil
	default:
		return nil, fmt.Errorf("unexpected JSON delimiter")
	}
}

// Complete appends an independent execution attempt. Failure, cancellation and
// stale attempts never disappear when a later attempt succeeds. For success or
// empty, the caller must first BindSource using the actual query producer's
// complete source-universe validator, and retain the original payload ref.
func (c *Catalog) Complete(ctx context.Context, queryID string, completion Completion) (QueryRecord, error) {
	if c == nil {
		return QueryRecord{}, ErrUnknownQuery
	}
	if err := validateCompletion(completion); err != nil {
		return QueryRecord{}, err
	}
	c.mu.Lock()
	q, ok := c.queries[queryID]
	b := c.bindings[q.ArtifactID]
	c.mu.Unlock()
	if !ok {
		return QueryRecord{}, ErrUnknownQuery
	}
	var validationErr error
	if err := ctx.Err(); err != nil {
		validationErr = err
		completion.Outcome = OutcomeCanceled
		completion.Error = &Issue{Code: "query_canceled", Message: boundedMessage(err.Error())}
	} else if completion.Outcome == OutcomeSuccess || completion.Outcome == OutcomeEmpty {
		if b.validate == nil || b.fingerprint == "" || b.fingerprint != completion.SourceRevision {
			return QueryRecord{}, fmt.Errorf("successful query requires its current bound source revision")
		}
		if _, err := c.Resolve(ctx, q.ArtifactID); err != nil {
			validationErr = err
			completion.Outcome = OutcomeStale
			if ctx.Err() != nil {
				completion.Outcome = OutcomeCanceled
			}
			completion.Error = &Issue{Code: "query_source_validation_failed", Message: boundedMessage(err.Error())}
		}
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.attemptCountLocked() >= MaxAttempts {
		return QueryRecord{}, fmt.Errorf("trace catalog attempt limit reached")
	}
	q = c.queries[queryID]
	if c.artifacts[q.ArtifactID].Status == "stale" && completion.Outcome != OutcomeCanceled {
		completion.Outcome = OutcomeStale
		if completion.Error == nil {
			completion.Error = &Issue{Code: "source_changed", Message: ErrStale.Error()}
		}
		if validationErr == nil {
			validationErr = ErrStale
		}
	}
	c.appendAttemptLocked(&q, completion)
	c.queries[queryID] = q
	return cloneQuery(q), validationErr
}

func validateCompletion(c Completion) error {
	switch c.Outcome {
	case OutcomeSuccess, OutcomeEmpty:
		if (c.PayloadRef == "" && c.RawRef == "") || c.SourceRevision == "" || c.Error != nil {
			return fmt.Errorf("successful/empty attempt requires an original payload or raw result reference and source revision, without an error")
		}
	case OutcomeFailure, OutcomeCanceled, OutcomeStale:
		if c.Error == nil || c.Error.Code == "" {
			return fmt.Errorf("failed/canceled/stale attempt requires an explicit reason")
		}
	default:
		return fmt.Errorf("completion outcome is invalid")
	}
	for _, s := range []string{c.SourceRevision, c.PayloadRef, c.RawRef} {
		if len(s) > MaxTextBytes || !utf8.ValidString(s) {
			return fmt.Errorf("completion reference is invalid or oversized")
		}
	}
	if c.Error != nil {
		for _, s := range []string{c.Error.Code, c.Error.Path, c.Error.Message} {
			if len(s) > MaxTextBytes || !utf8.ValidString(s) {
				return fmt.Errorf("completion error is invalid or oversized")
			}
		}
	}
	return nil
}

func (c *Catalog) attemptCountLocked() int {
	n := 0
	for _, q := range c.queries {
		n += len(q.Attempts)
	}
	return n
}

func (c *Catalog) appendAttemptLocked(q *QueryRecord, completion Completion) {
	seq := c.run.sequence.Add(1)
	a := Attempt{ID: digest("attempt_", struct {
		Query    string
		Sequence uint64
	}{q.ID, seq}), Sequence: seq, Completion: completion}
	if a.Error != nil {
		e := *a.Error
		a.Error = &e
	}
	q.Attempts = append(q.Attempts, a)
	q.Outcome = completion.Outcome
}

func boundedMessage(s string) string {
	if len(s) <= MaxTextBytes {
		return s
	}
	s = s[:MaxTextBytes-3]
	for !utf8.ValidString(s) {
		s = s[:len(s)-1]
	}
	return s + "..."
}

// Clone retains private current-run validators, but copies all mutable data.
// It is for a live agent fork, never restoration from a serialized snapshot.
func (c *Catalog) Clone() *Catalog {
	if c == nil {
		return nil
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	copy := &Catalog{run: c.run, id: c.id, root: c.root, discovery: c.discovery,
		artifacts: map[string]Artifact{}, generations: make(map[string]filegeneration.Identity),
		bindings: map[string]sourceBinding{}, queries: map[string]QueryRecord{}}
	copy.declared = make(map[string]bool, len(c.declared))
	copy.discovery.Issues = append([]Issue(nil), c.discovery.Issues...)
	for id, a := range c.artifacts {
		a.Issues = append([]Issue(nil), a.Issues...)
		copy.artifacts[id] = a
	}
	for id, g := range c.generations {
		copy.generations[id] = g
	}
	for id, b := range c.bindings {
		copy.bindings[id] = b
	}
	for id, q := range c.queries {
		copy.queries[id] = cloneQuery(q)
	}
	for id, declared := range c.declared {
		copy.declared[id] = declared
	}
	return copy
}

// Merge accepts live clones only. Full stable query/attempt keys are joined,
// never positions, display tags, object IDs without artifact scope, or paths
// found in a historical JSON file. A stale source cannot be rescued by a fork.
func (c *Catalog) Merge(other *Catalog) error {
	if c == nil || other == nil || c.run == nil || c.run != other.run || c.id != other.id {
		return ErrForeignRun
	}
	if c == other {
		return nil
	}
	copy := other.Clone()
	c.mu.Lock()
	defer c.mu.Unlock()
	if len(c.queries)+len(copy.queries) > 2*MaxQueries {
		return fmt.Errorf("catalog merge query limit reached")
	}
	merged := make(map[string]QueryRecord, len(c.queries)+len(copy.queries))
	for id, q := range c.queries {
		merged[id] = cloneQuery(q)
	}
	for id, q := range copy.queries {
		if old, ok := merged[id]; ok {
			seen := map[string]Attempt{}
			for _, a := range old.Attempts {
				seen[a.ID] = a
			}
			for _, a := range q.Attempts {
				if previous, exists := seen[a.ID]; exists && digest("", previous) != digest("", a) {
					return fmt.Errorf("conflicting query attempt")
				}
				seen[a.ID] = a
			}
			q.Attempts = q.Attempts[:0]
			for _, a := range seen {
				q.Attempts = append(q.Attempts, a)
			}
			sort.Slice(q.Attempts, func(i, j int) bool { return q.Attempts[i].Sequence < q.Attempts[j].Sequence })
		}
		q.Outcome = OutcomeNotExecuted
		if len(q.Attempts) != 0 {
			q.Outcome = q.Attempts[len(q.Attempts)-1].Outcome
		}
		merged[id] = q
	}
	if len(merged) > MaxQueries {
		return fmt.Errorf("catalog merge query limit reached")
	}
	attempts := 0
	for _, q := range merged {
		attempts += len(q.Attempts)
	}
	if attempts > MaxAttempts {
		return fmt.Errorf("catalog merge attempt limit reached")
	}
	conflicts := map[string]bool{}
	for id, b := range copy.bindings {
		if own, ok := c.bindings[id]; ok && own.fingerprint != b.fingerprint {
			conflicts[id] = true
		}
	}
	for id, b := range copy.bindings {
		if conflicts[id] {
			continue
		}
		c.bindings[id] = b
		a := c.artifacts[id]
		a.SourceRevision = b.fingerprint
		c.artifacts[id] = a
	}
	for id, a := range copy.artifacts {
		if a.Status == "stale" {
			c.artifacts[id] = a
		}
	}
	// Different full source universes cannot select a winner by merge order.
	// Keep every attempt for navigation, but invalidate the current artifact
	// and all of its query statuses. This is a recorded conflict, not a failed
	// merge that a caller could ignore while displaying an earlier success.
	for id := range conflicts {
		a := c.artifacts[id]
		a.Status = "stale"
		a.SourceRevision = ""
		found := false
		for _, issue := range a.Issues {
			found = found || issue.Code == "conflicting_source_universes"
		}
		if !found {
			a.Issues = append(a.Issues, Issue{Code: "conflicting_source_universes", Path: a.Path, Message: "Live query branches observed different complete source revisions; individual attempts remain in history."})
		}
		c.artifacts[id] = a
	}
	for id, q := range merged {
		if c.artifacts[q.ArtifactID].Status == "stale" {
			q.Outcome = OutcomeStale
			merged[id] = q
		}
	}
	c.queries = merged
	if c.declared == nil {
		c.declared = map[string]bool{}
	}
	for id, declared := range copy.declared {
		if declared {
			c.declared[id] = true
		}
	}
	return nil
}

// Query returns a detached navigation row; no payload is read.
func (c *Catalog) Query(id string) (QueryRecord, bool) {
	if c == nil {
		return QueryRecord{}, false
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	q, ok := c.queries[strings.TrimSpace(id)]
	return cloneQuery(q), ok
}
