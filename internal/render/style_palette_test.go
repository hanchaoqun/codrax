package render

import (
	"strings"
	"testing"
)

// TestStylePalette_DarkHeadingHierarchy locks the H1-H6 brightness
// gradient so a regression doesn't collapse the hierarchy back to
// "every level the same color". H1 = pure white, H2-H4 = bright/
// light/mid cyan-blue, H5/H6 = readable greys. The 2026-04-30
// brightening fixed user feedback that H3/H4 was too dim.
func TestStylePalette_DarkHeadingHierarchy(t *testing.T) {
	cfg := buildDarkPalette()
	want := map[string]string{
		"H1": "15",
		"H2": "51",
		"H3": "117",
		"H4": "75",
		"H5": "250",
		"H6": "245",
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
	// H1-H4 bold; H5 + H6 italic, not bold (visually retreated).
	for _, level := range []struct {
		name string
		bold *bool
	}{
		{"H1", cfg.H1.Bold},
		{"H2", cfg.H2.Bold},
		{"H3", cfg.H3.Bold},
		{"H4", cfg.H4.Bold},
	} {
		if level.bold == nil || !*level.bold {
			t.Errorf("%s must be bold; got %v", level.name, level.bold)
		}
	}
	if cfg.H5.Bold == nil || *cfg.H5.Bold {
		t.Errorf("H5 must be non-bold; got %v", cfg.H5.Bold)
	}
	if cfg.H6.Bold == nil || *cfg.H6.Bold {
		t.Errorf("H6 must be non-bold; got %v", cfg.H6.Bold)
	}
	// Italics intentionally disabled per user feedback — heading
	// hierarchy expressed via colour alone, not weight or slant.
	if cfg.H5.Italic == nil || *cfg.H5.Italic {
		t.Errorf("H5 must NOT be italic (color-only hierarchy); got %v", cfg.H5.Italic)
	}
	if cfg.H6.Italic == nil || *cfg.H6.Italic {
		t.Errorf("H6 must NOT be italic (color-only hierarchy); got %v", cfg.H6.Italic)
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
	// Accept any hex colour where the green channel dominates (or at
	// least matches) red and blue, so a soft mint like #87d7af passes
	// the same family check as a pure green like #50fa7b.
	if chroma.GenericInserted.Color == nil {
		t.Fatal("generic.inserted must have an explicit color")
	}
	gi := strings.ToLower(*chroma.GenericInserted.Color)
	if !isGreenFamily(gi) {
		t.Errorf("generic.inserted %s — diff green is load-bearing, must be green family", gi)
	}
}

// isGreenFamily reports whether a hex colour reads as visually
// green (G channel ≥ R AND G channel ≥ B). Accepts both #rrggbb
// and shorter forms; returns false on parse failure.
func isGreenFamily(hex string) bool {
	if !strings.HasPrefix(hex, "#") || len(hex) != 7 {
		return false
	}
	r, ok1 := parseHexByte(hex[1:3])
	g, ok2 := parseHexByte(hex[3:5])
	b, ok3 := parseHexByte(hex[5:7])
	if !ok1 || !ok2 || !ok3 {
		return false
	}
	return g >= r && g >= b && g > 0x40
}

func parseHexByte(s string) (int, bool) {
	if len(s) != 2 {
		return 0, false
	}
	var v int
	for i := 0; i < 2; i++ {
		c := s[i]
		switch {
		case c >= '0' && c <= '9':
			v = v*16 + int(c-'0')
		case c >= 'a' && c <= 'f':
			v = v*16 + int(c-'a'+10)
		case c >= 'A' && c <= 'F':
			v = v*16 + int(c-'A'+10)
		default:
			return 0, false
		}
	}
	return v, true
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
