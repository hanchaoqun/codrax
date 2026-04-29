package render

import (
	"strings"
	"testing"
)

// TestStylePalette_DarkHeadingHierarchy locks the H1-H6 brightness
// gradient so a regression doesn't collapse the hierarchy back to
// "every level the same color". H1 must be the brightest, H6 the
// dimmest.
func TestStylePalette_DarkHeadingHierarchy(t *testing.T) {
	cfg := buildDarkPalette()
	want := map[string]string{
		"H1": "15",
		"H2": "117",
		"H3": "75",
		"H4": "67",
		"H5": "246",
		"H6": "240",
	}
	got := map[string]*string{
		"H1": cfg.H1.Color,
		"H2": cfg.H2.Color,
		"H3": cfg.H3.Color,
		"H4": cfg.H4.Color,
		"H5": cfg.H5.Color,
		"H6": cfg.H6.Color,
	}
	for level, expected := range want {
		if got[level] == nil || *got[level] != expected {
			t.Errorf("%s color = %v, want %s", level, deref(got[level]), expected)
		}
	}
	// H1-H5 bold; H6 explicitly NOT bold (visually retreats).
	if cfg.H6.Bold == nil || *cfg.H6.Bold {
		t.Errorf("H6 must be non-bold; got %v", cfg.H6.Bold)
	}
	if cfg.H1.Bold == nil || !*cfg.H1.Bold {
		t.Errorf("H1 must be bold; got %v", cfg.H1.Bold)
	}
}

// TestStylePalette_NoBackgroundColors locks the "terminal theme owns
// the background" invariant. Backgrounds on Code / H1 / chroma fence
// must all be nil.
func TestStylePalette_NoBackgroundColors(t *testing.T) {
	cfg := buildDarkPalette()
	if cfg.Code.BackgroundColor != nil {
		t.Errorf("inline code must not paint background; got %v", *cfg.Code.BackgroundColor)
	}
	if cfg.H1.BackgroundColor != nil {
		t.Errorf("H1 must not paint background; got %v", *cfg.H1.BackgroundColor)
	}
	if cfg.CodeBlock.Chroma == nil {
		t.Fatal("CodeBlock.Chroma must be non-nil")
	}
	if cfg.CodeBlock.Chroma.Background.BackgroundColor != nil {
		t.Errorf("chroma fence must not paint background; got %v",
			*cfg.CodeBlock.Chroma.Background.BackgroundColor)
	}
	if cfg.CodeBlock.Chroma.Error.BackgroundColor != nil {
		t.Errorf("chroma error must not paint background; got %v",
			*cfg.CodeBlock.Chroma.Error.BackgroundColor)
	}
}

// TestStylePalette_DarkHueDiscipline pins the hue-discipline rules:
// red is reserved for diagnostics + diff-deleted; operators are
// neutral grey; built-ins are yellow (not pink/red). Each rule
// closes a specific failure mode in the pre-2026-04-29 palette.
func TestStylePalette_DarkHueDiscipline(t *testing.T) {
	cfg := buildDarkPalette()
	chroma := cfg.CodeBlock.Chroma
	if chroma == nil {
		t.Fatal("Chroma must be non-nil")
	}
	// Operator must NOT be in the warm-red family (the pre-fix
	// #EF8080 made every `=` look like a diff removal). #909090 grey
	// is the canonical neutral.
	if chroma.Operator.Color == nil {
		t.Fatal("operator must have an explicit color")
	}
	op := *chroma.Operator.Color
	for _, banned := range []string{"#FF", "#EF", "#FD"} {
		if strings.HasPrefix(strings.ToUpper(op), banned) {
			t.Errorf("operator color %s is in the warm-red family — must be neutral grey", op)
		}
	}
	// NameBuiltin must NOT be pink/red; yellow is the rule.
	if chroma.NameBuiltin.Color == nil {
		t.Fatal("name.builtin must have an explicit color")
	}
	nb := strings.ToLower(*chroma.NameBuiltin.Color)
	if strings.HasPrefix(nb, "#ff5") || strings.HasPrefix(nb, "#ff8") || strings.HasPrefix(nb, "#ef") {
		t.Errorf("name.builtin %s is in the warm-red family", nb)
	}
	// generic.deleted must remain red — diff semantics depend on it.
	if chroma.GenericDeleted.Color == nil {
		t.Fatal("generic.deleted must have an explicit color")
	}
	gd := strings.ToLower(*chroma.GenericDeleted.Color)
	if !strings.HasPrefix(gd, "#ff") && !strings.HasPrefix(gd, "#fd") &&
		!strings.HasPrefix(gd, "#cf") {
		t.Errorf("generic.deleted %s — diff red is load-bearing, must be red family", gd)
	}
	// generic.inserted must remain green — diff semantics ditto.
	if chroma.GenericInserted.Color == nil {
		t.Fatal("generic.inserted must have an explicit color")
	}
	gi := strings.ToLower(*chroma.GenericInserted.Color)
	if !strings.HasPrefix(gi, "#5") && !strings.HasPrefix(gi, "#0") &&
		!strings.HasPrefix(gi, "#1") {
		t.Errorf("generic.inserted %s — diff green is load-bearing, must be green family", gi)
	}
}

// TestStylePalette_DarkInlineCodeAndLinksMatch confirms the cohort
// rule: link text uses the same hue family as inline code so a link
// reads as "interactive code reference" rather than a different kind
// of element.
func TestStylePalette_DarkInlineCodeAndLinksMatch(t *testing.T) {
	cfg := buildDarkPalette()
	if cfg.Code.Color == nil || cfg.LinkText.Color == nil {
		t.Fatal("inline code and link text must both have explicit colors")
	}
	if *cfg.Code.Color != *cfg.LinkText.Color {
		t.Errorf("link text (%s) and inline code (%s) should share a colour for cohort cohesion",
			*cfg.LinkText.Color, *cfg.Code.Color)
	}
}

// TestStylePalette_LightMirrorsDarkStructure verifies the light
// palette is a deliberate mirror — every override the dark palette
// performs has a light counterpart, so a user toggling background
// detection doesn't lose curation.
func TestStylePalette_LightMirrorsDarkStructure(t *testing.T) {
	cfg := buildLightPalette()
	for _, level := range []struct {
		name string
		c    *string
	}{
		{"H1", cfg.H1.Color},
		{"H2", cfg.H2.Color},
		{"H3", cfg.H3.Color},
		{"H4", cfg.H4.Color},
		{"H5", cfg.H5.Color},
		{"H6", cfg.H6.Color},
		{"Code", cfg.Code.Color},
		{"HR", cfg.HorizontalRule.Color},
		{"Link", cfg.Link.Color},
		{"LinkText", cfg.LinkText.Color},
		{"BlockQuote", cfg.BlockQuote.Color},
	} {
		if level.c == nil {
			t.Errorf("light palette: %s missing explicit colour (mirror with dark)", level.name)
		}
	}
	if cfg.CodeBlock.Chroma == nil ||
		cfg.CodeBlock.Chroma.Operator.Color == nil ||
		cfg.CodeBlock.Chroma.NameBuiltin.Color == nil {
		t.Errorf("light palette: chroma operator + name.builtin must have explicit colours")
	}
}

// deref pulls a *string into its value or "<nil>" for cleaner
// failure messages.
func deref(p *string) string {
	if p == nil {
		return "<nil>"
	}
	return *p
}
