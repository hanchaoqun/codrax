// Package tracecatalog keeps a run-local navigation directory for trace inputs
// and per-object query attempts. A catalog is not a read receipt, a query
// executor, or evidence authority. Its persisted snapshots are navigation only.
package tracecatalog

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"path/filepath"
	"sort"
	"sync"
	"sync/atomic"

	"github.com/hanchaoqun/codrax/internal/filegeneration"
)

const SchemaVersion = 1

var (
	ErrStale           = errors.New("trace catalog source changed")
	ErrUnknownArtifact = errors.New("trace catalog artifact not found")
	ErrUnknownQuery    = errors.New("trace catalog query not found")
	ErrForeignRun      = errors.New("trace catalog is not from the same live run")
	ErrUnavailable     = errors.New("trace catalog candidate unavailable")
)

type Outcome string

const (
	OutcomeNotExecuted Outcome = "not_executed"
	OutcomeSuccess     Outcome = "success"
	OutcomeEmpty       Outcome = "empty"
	OutcomeFailure     Outcome = "failure"
	OutcomeCanceled    Outcome = "canceled"
	OutcomeStale       Outcome = "stale"
)

// Issue is a navigation/coverage explanation, not a model-derived finding.
type Issue struct {
	Code    string `json:"code"`
	Path    string `json:"path,omitempty"`
	Message string `json:"message,omitempty"`
}

// Artifact is a discovered candidate. Discovery never asserts format support.
// RevisionID is a display fingerprint, not a reconstructible file permission.
type Artifact struct {
	ID             string  `json:"id"`
	Path           string  `json:"path"`
	RelativePath   string  `json:"relative_path"`
	RevisionID     string  `json:"revision_id,omitempty"`
	Bytes          int64   `json:"bytes"`
	Status         string  `json:"status"`
	SourceRevision string  `json:"source_revision,omitempty"`
	Issues         []Issue `json:"issues,omitempty"`
}

// Discovery describes the frozen roster, not the population of query results.
type Discovery struct {
	Recursive             bool    `json:"recursive"`
	FilterLabel           string  `json:"filter_label,omitempty"`
	Complete              bool    `json:"complete"`
	Canceled              bool    `json:"canceled,omitempty"`
	Truncated             bool    `json:"truncated,omitempty"`
	EntriesVisited        int     `json:"entries_visited"`
	RegularFilesSeen      int     `json:"regular_files_seen"`
	FilteredFiles         int     `json:"filtered_files"`
	SymlinksSkipped       int     `json:"symlinks_skipped"`
	SpecialFilesSkipped   int     `json:"special_files_skipped"`
	SubdirectoriesSkipped int     `json:"subdirectories_skipped,omitempty"`
	IssueCount            int     `json:"issue_count"`
	Issues                []Issue `json:"issues,omitempty"`
}

// Object identity is local to an artifact revision. Scope can distinguish an
// owner process or another native namespace; labels/tags are not global IDs.
type Object struct {
	Kind  string `json:"kind"`
	ID    string `json:"id"`
	Scope string `json:"scope,omitempty"`
}

// Window retains exact argument bounds. A nil EndInclusive means the
// navigation caller has no producer-owned endpoint-inclusion contract; it
// must not infer a half-open interval. Zero is valid, with no clock conversion.
type Window struct {
	StartNS      int64 `json:"start_ns"`
	EndNS        int64 `json:"end_ns"`
	EndInclusive *bool `json:"end_inclusive,omitempty"`
}

// Plan records the actual tool parameters as canonical JSON solely for
// navigation. The tool remains responsible for its own parameter schema.
type Plan struct {
	ArtifactID string          `json:"artifact_id"`
	Object     Object          `json:"object"`
	View       string          `json:"view"`
	Parameters json.RawMessage `json:"parameters"`
	Window     *Window         `json:"window,omitempty"`
}

type Completion struct {
	Outcome        Outcome `json:"outcome"`
	SourceRevision string  `json:"source_revision,omitempty"`
	PayloadRef     string  `json:"payload_ref,omitempty"`
	RawRef         string  `json:"raw_ref,omitempty"`
	Truncated      bool    `json:"truncated,omitempty"`
	Error          *Issue  `json:"error,omitempty"`
}

type Attempt struct {
	ID       string `json:"id"`
	Sequence uint64 `json:"sequence"`
	Completion
}

type QueryRecord struct {
	ID               string `json:"id"`
	ArtifactRevision string `json:"artifact_revision"`
	Plan
	Outcome  Outcome   `json:"outcome"`
	Attempts []Attempt `json:"attempts,omitempty"`
}

// Snapshot never carries the run identity or source validators. Load returns
// this type only; it cannot be converted back into a live Catalog.
type Snapshot struct {
	SchemaVersion  int           `json:"schema_version"`
	ID             string        `json:"id"`
	Root           string        `json:"root"`
	NavigationOnly bool          `json:"navigation_only"`
	Discovery      Discovery     `json:"discovery"`
	Artifacts      []Artifact    `json:"artifacts"`
	Queries        []QueryRecord `json:"queries"`
}

type sourceBinding struct {
	fingerprint string
	validate    func(context.Context) error
}

type runIdentity struct {
	sequence    atomic.Uint64
	publication sync.Mutex
	published   map[string]*Catalog
}

type Catalog struct {
	mu          sync.Mutex
	run         *runIdentity
	id          string
	root        string
	discovery   Discovery
	artifacts   map[string]Artifact
	generations map[string]filegeneration.Identity
	bindings    map[string]sourceBinding
	queries     map[string]QueryRecord
	declared    map[string]bool
}

func digest(prefix string, value any) string {
	b, _ := json.Marshal(value)
	sum := sha256.Sum256(b)
	return prefix + hex.EncodeToString(sum[:])
}

func (c *Catalog) ID() string {
	if c == nil {
		return ""
	}
	return c.id
}

func (c *Catalog) Snapshot() Snapshot {
	if c == nil {
		return Snapshot{SchemaVersion: SchemaVersion, NavigationOnly: true}
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.snapshotLocked()
}

func (c *Catalog) snapshotLocked() Snapshot {
	s := Snapshot{SchemaVersion: SchemaVersion, ID: c.id, Root: c.root, NavigationOnly: true, Discovery: c.discovery,
		Artifacts: make([]Artifact, 0, len(c.artifacts)), Queries: make([]QueryRecord, 0, len(c.queries))}
	s.Discovery.Issues = append([]Issue(nil), c.discovery.Issues...)
	for _, a := range c.artifacts {
		a.Issues = append([]Issue(nil), a.Issues...)
		s.Artifacts = append(s.Artifacts, a)
	}
	for _, q := range c.queries {
		s.Queries = append(s.Queries, cloneQuery(q))
	}
	sort.Slice(s.Artifacts, func(i, j int) bool { return s.Artifacts[i].RelativePath < s.Artifacts[j].RelativePath })
	sort.Slice(s.Queries, func(i, j int) bool { return s.Queries[i].ID < s.Queries[j].ID })
	return s
}

func cloneQuery(q QueryRecord) QueryRecord {
	q.Parameters = append(json.RawMessage(nil), q.Parameters...)
	if q.Window != nil {
		w := *q.Window
		if w.EndInclusive != nil {
			value := *w.EndInclusive
			w.EndInclusive = &value
		}
		q.Window = &w
	}
	q.Attempts = append([]Attempt(nil), q.Attempts...)
	for i := range q.Attempts {
		if q.Attempts[i].Error != nil {
			v := *q.Attempts[i].Error
			q.Attempts[i].Error = &v
		}
	}
	return q
}

// ArtifactForPath matches only an exact canonical candidate path. It is a
// navigation lookup; Resolve and the normal tool admission are still required.
func (c *Catalog) ArtifactForPath(path string) (Artifact, bool) {
	if c == nil {
		return Artifact{}, false
	}
	abs, err := filepath.Abs(path)
	if err != nil {
		return Artifact{}, false
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	for _, a := range c.artifacts {
		if a.Path == filepath.Clean(abs) {
			a.Issues = append([]Issue(nil), a.Issues...)
			return a, true
		}
	}
	return Artifact{}, false
}

// Resolve checks only the current physical generation and optional injected
// source-universe validator. It does not authorize reading or querying.
func (c *Catalog) Resolve(ctx context.Context, artifactID string) (Artifact, error) {
	if c == nil {
		return Artifact{}, ErrUnknownArtifact
	}
	if err := ctx.Err(); err != nil {
		return Artifact{}, err
	}
	c.mu.Lock()
	a, ok := c.artifacts[artifactID]
	g := c.generations[artifactID]
	b := c.bindings[artifactID]
	c.mu.Unlock()
	if !ok {
		return Artifact{}, ErrUnknownArtifact
	}
	if a.Status == "stale" {
		return a, ErrStale
	}
	if a.Status == "unavailable" || !g.Initialized() {
		return a, ErrUnavailable
	}
	canonical, err := filepath.EvalSymlinks(a.Path)
	if err == nil && canonical != a.Path {
		err = fmt.Errorf("candidate path changed through a symbolic link")
	}
	if err == nil {
		var actual filegeneration.Identity
		actual, err = filegeneration.FromPath(a.Path)
		if err == nil && !g.SameVersion(actual) {
			err = ErrStale
		}
	}
	if err == nil && b.validate != nil {
		err = b.validate(ctx)
	}
	if ctx.Err() != nil {
		return a, ctx.Err()
	}
	if err != nil {
		c.markStale(artifactID, err)
		a.Status = "stale"
		return a, fmt.Errorf("%w: %s: %v", ErrStale, a.Path, err)
	}
	a.Issues = append([]Issue(nil), a.Issues...)
	return a, nil
}

func (c *Catalog) markStale(artifactID string, cause error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	a := c.artifacts[artifactID]
	if a.Status == "stale" {
		return
	}
	a.Status = "stale"
	issue := Issue{Code: "source_changed", Path: a.Path, Message: boundedMessage(cause.Error())}
	a.Issues = append(a.Issues, issue)
	c.artifacts[artifactID] = a
	remaining := MaxAttempts - c.attemptCountLocked()
	for id, q := range c.queries {
		if q.ArtifactID != artifactID {
			continue
		}
		if remaining > 0 {
			c.appendAttemptLocked(&q, Completion{Outcome: OutcomeStale, SourceRevision: a.SourceRevision, Error: &issue})
			remaining--
		} else {
			// The artifact-level stale issue remains authoritative navigation;
			// do not drop old attempts to create room or retain a success state.
			q.Outcome = OutcomeStale
		}
		c.queries[id] = q
	}
}

// BindSource retains a producer's complete source-universe check privately.
// Matching display fingerprints alone can never replace this check.
func (c *Catalog) BindSource(ctx context.Context, artifactID, fingerprint string, validate func(context.Context) error) error {
	if fingerprint == "" || validate == nil {
		return fmt.Errorf("source fingerprint and live validator are required")
	}
	if _, err := c.Resolve(ctx, artifactID); err != nil {
		return err
	}
	if err := validate(ctx); err != nil {
		if ctx.Err() == nil {
			c.markStale(artifactID, err)
		}
		return err
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	if old, ok := c.bindings[artifactID]; ok && old.fingerprint != fingerprint {
		a := c.artifacts[artifactID]
		a.Status = "stale"
		a.Issues = append(a.Issues, Issue{Code: "source_universe_changed", Path: a.Path, Message: "bound source revision differs"})
		c.artifacts[artifactID] = a
		for id, q := range c.queries {
			if q.ArtifactID == artifactID {
				q.Outcome = OutcomeStale
				c.queries[id] = q
			}
		}
		return fmt.Errorf("%w: source universe differs", ErrStale)
	}
	if c.artifacts[artifactID].Status == "stale" {
		return ErrStale
	}
	if _, exists := c.bindings[artifactID]; !exists {
		c.bindings[artifactID] = sourceBinding{fingerprint: fingerprint, validate: validate}
	}
	a := c.artifacts[artifactID]
	a.SourceRevision = fingerprint
	c.artifacts[artifactID] = a
	return nil
}
