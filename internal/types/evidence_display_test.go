package types

import "testing"

// TestEvidenceItem_IsCitable locks the session-8 + scope-axis 2026-05+
// citation-visibility policy: Grounded items (any scope) are citable;
// legacy empty-status line-shaped items (concrete_value emitter) are
// citable; Recovered / Ungrounded are NOT citable; legacy empty-status
// schema-level scopes (File / Crossfile / Negative) are NOT citable —
// those scopes are LLM-emit-only and require grounder validation.
func TestEvidenceItem_IsCitable(t *testing.T) {
	cases := []struct {
		name   string
		status GroundingStatus
		scope  EvidenceScope
		want   bool
	}{
		{"grounded line is citable", GroundingGrounded, ScopeLine, true},
		{"grounded file is citable", GroundingGrounded, ScopeFile, true},
		{"grounded negative is citable", GroundingGrounded, ScopeNegative, true},
		{"recovered is NOT citable", GroundingRecovered, ScopeLine, false},
		{"ungrounded is NOT citable", GroundingUngrounded, ScopeLine, false},
		{"legacy empty-status line counts as citable", "", ScopeLine, true},
		{"legacy empty-status range counts as citable", "", ScopeLineRange, true},
		{"legacy empty-status section counts as citable", "", ScopeSection, true},
		{"legacy empty-status file is NOT citable", "", ScopeFile, false},
		{"legacy empty-status crossfile is NOT citable", "", ScopeCrossfile, false},
		{"legacy empty-status negative is NOT citable", "", ScopeNegative, false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			e := EvidenceItem{GroundingStatus: c.status, Scope: c.scope}
			if got := e.IsCitable(); got != c.want {
				t.Errorf("IsCitable() = %v, want %v", got, c.want)
			}
		})
	}
}

// TestEvidenceItem_DisplayLocation covers the strict/relaxed dispatch:
// strict=true drops LineStart for non-Tier-1 items; strict=false is
// backward-compatible with historical `file:line` rendering. Both
// modes preserve the line range suffix when LineEnd > LineStart.
func TestEvidenceItem_DisplayLocation(t *testing.T) {
	grounded := EvidenceItem{Source: "a.go", LineStart: 10, GroundingStatus: GroundingGrounded}
	recovered := EvidenceItem{Source: "b.go", LineStart: 20, GroundingStatus: GroundingRecovered}
	ungrounded := EvidenceItem{Source: "c.go", LineStart: 30, GroundingStatus: GroundingUngrounded}
	ranged := EvidenceItem{Source: "d.go", LineStart: 40, LineEnd: 50, GroundingStatus: GroundingGrounded}
	noSource := EvidenceItem{}

	// strict=false: always show LineStart when present.
	if got := grounded.DisplayLocation(false); got != "a.go:10" {
		t.Errorf("grounded non-strict: got %q, want a.go:10", got)
	}
	if got := recovered.DisplayLocation(false); got != "b.go:20" {
		t.Errorf("recovered non-strict must show line: got %q, want b.go:20", got)
	}
	if got := ungrounded.DisplayLocation(false); got != "c.go:30" {
		t.Errorf("ungrounded non-strict must show line: got %q, want c.go:30", got)
	}
	if got := ranged.DisplayLocation(false); got != "d.go:40-50" {
		t.Errorf("range-span must render: got %q, want d.go:40-50", got)
	}
	if got := noSource.DisplayLocation(false); got != "" {
		t.Errorf("missing source must return empty: got %q", got)
	}

	// strict=true: strip LineStart for recovered + ungrounded.
	if got := grounded.DisplayLocation(true); got != "a.go:10" {
		t.Errorf("grounded strict must keep line: got %q", got)
	}
	if got := recovered.DisplayLocation(true); got != "b.go" {
		t.Errorf("recovered strict must strip line: got %q, want b.go", got)
	}
	if got := ungrounded.DisplayLocation(true); got != "c.go" {
		t.Errorf("ungrounded strict must strip line: got %q, want c.go", got)
	}
}
