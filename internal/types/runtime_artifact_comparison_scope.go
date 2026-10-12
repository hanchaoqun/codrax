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

// Independent source ownership does not require the user to supply numeric
// bounds. A partial bound is invalid; absent bounds never mean zero seconds.
func (s RuntimeArtifactComparisonSide) Valid() bool {
	return strings.TrimSpace(s.Path) != "" && strings.TrimSpace(s.SourceQuote) != "" &&
		(s.TimeStart == nil && s.TimeEnd == nil || s.RuntimeArtifactTimeWindow.Valid())
}

func (s RuntimeArtifactComparisonSide) HasExplicitWindow() bool {
	return s.RuntimeArtifactTimeWindow.Valid()
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

// EffectiveComparisonWindow resolves the request ruler without changing the
// original declarations. A shared outer interval and source-owned intervals
// are mutually exclusive. An unbounded side never inherits its peer's window.
// ok means the binding shape is valid; window.Valid reports whether it is
// bounded. This is not a source receipt or proof of aligned capture clocks.
func (p *RuntimeArtifactScopeProfile) EffectiveComparisonWindow(sideIndex int) (window RuntimeArtifactTimeWindow, ok bool) {
	if p == nil || !p.Comparison.Valid() || sideIndex < 0 || sideIndex > 1 {
		return window, false
	}
	sides := p.Comparison.Sides()
	if p.TimeStart != nil || p.TimeEnd != nil {
		window = RuntimeArtifactTimeWindow{TimeStart: p.TimeStart, TimeEnd: p.TimeEnd, SourceQuote: p.SourceQuote}
		if p.RequestedScope != RuntimeArtifactScopeExplicitWindow || p.TimeWindows != nil || !window.Valid() || sides[0].HasExplicitWindow() || sides[1].HasExplicitWindow() {
			return RuntimeArtifactTimeWindow{}, false
		}
	} else {
		window = sides[sideIndex].RuntimeArtifactTimeWindow
	}
	return cloneRuntimeArtifactTimeWindows([]RuntimeArtifactTimeWindow{window})[0], true
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
	for i, side := range p.Comparison.Sides() {
		if RuntimeArtifactComparisonPathEqual(side.Path, originalPath, repoRoot) {
			window, ok := p.EffectiveComparisonWindow(i)
			if !ok {
				return false
			}
			if !window.Valid() {
				return true
			}
			return TraceCausalProjectionPrincipalValueWindowContains(*window.TimeStart, *window.TimeEnd, start, end)
		}
	}
	return false
}
