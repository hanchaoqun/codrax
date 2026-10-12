package types

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"strings"
)

// RuntimeArtifactComparisonScope is request-owned independent acquisition,
// not a proof of comparability, clock alignment, successful reads or causality.
// It is optional; ordinary scope/window declarations never imply independence.
type RuntimeArtifactComparisonScope struct {
	Baseline RuntimeArtifactComparisonSide `json:"baseline"`
	Current  RuntimeArtifactComparisonSide `json:"current"`
}

type RuntimeArtifactComparisonSide struct {
	Path string `json:"path"`
	RuntimeArtifactTimeWindow
}

func (c *RuntimeArtifactComparisonScope) UnmarshalJSON(raw []byte) error {
	if !runtimeMeasurementUniqueKeys(string(raw)) {
		return fmt.Errorf("comparison contains duplicate or ambiguous JSON keys")
	}
	type plain RuntimeArtifactComparisonScope
	var value plain
	d := json.NewDecoder(bytes.NewReader(raw))
	d.DisallowUnknownFields()
	if err := d.Decode(&value); err != nil {
		return err
	}
	*c = RuntimeArtifactComparisonScope(value)
	return nil
}

func (c *RuntimeArtifactComparisonScope) Sides() []RuntimeArtifactComparisonSide {
	if c == nil {
		return nil
	}
	return []RuntimeArtifactComparisonSide{c.Baseline, c.Current}
}

func (c *RuntimeArtifactComparisonScope) Valid() bool {
	return c != nil && strings.TrimSpace(c.Baseline.Path) != "" && strings.TrimSpace(c.Current.Path) != "" &&
		c.Baseline.Path != c.Current.Path && c.Baseline.Valid() && c.Current.Valid()
}

func (c *RuntimeArtifactComparisonScope) Clone() *RuntimeArtifactComparisonScope {
	if c == nil {
		return nil
	}
	out := *c
	windows := cloneRuntimeArtifactTimeWindows([]RuntimeArtifactTimeWindow{c.Baseline.RuntimeArtifactTimeWindow, c.Current.RuntimeArtifactTimeWindow})
	out.Baseline.RuntimeArtifactTimeWindow, out.Current.RuntimeArtifactTimeWindow = windows[0], windows[1]
	return &out
}

func RuntimeArtifactComparisonPathEqual(left, right, repoRoot string) bool {
	left, right = RuntimeArtifactComparisonLocalPath(left, repoRoot), RuntimeArtifactComparisonLocalPath(right, repoRoot)
	return left != "" && right != "" && runtimeArtifactIdentityPathKey(left) == runtimeArtifactIdentityPathKey(right)
}

// RuntimeArtifactComparisonLocalPath normalizes a local spelling without
// resolving filesystem aliases or granting source authority.
func RuntimeArtifactComparisonLocalPath(path, repoRoot string) string {
	path = strings.TrimSpace(path)
	if strings.HasPrefix(path, "file://") {
		u, err := url.Parse(path)
		if err != nil || u.Host != "" || u.Scheme != "file" {
			return ""
		}
		path = u.Path
	}
	if strings.HasPrefix(path, "~/") {
		home, err := os.UserHomeDir()
		if err != nil {
			return ""
		}
		path = filepath.Join(home, strings.TrimPrefix(path, "~/"))
	}
	if path == "" {
		return ""
	}
	if !filepath.IsAbs(path) {
		path = filepath.Join(repoRoot, path)
	}
	return filepath.Clean(path)
}

// ComparisonWindowAllows consumes the exact original source, never a pooled
// list of windows or the order of observed captures. Callers must obtain that
// original path from their existing current source receipt.
func (p *RuntimeArtifactScopeProfile) ComparisonWindowAllows(originalPath, repoRoot string, start, end float64) bool {
	if p == nil || p.Comparison == nil {
		return true
	}
	if !p.Comparison.Valid() {
		return false
	}
	for _, side := range p.Comparison.Sides() {
		if RuntimeArtifactComparisonPathEqual(side.Path, originalPath, repoRoot) {
			return TraceCausalProjectionPrincipalValueWindowContains(*side.TimeStart, *side.TimeEnd, start, end)
		}
	}
	return false
}
