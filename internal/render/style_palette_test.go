package render

import (
	"strings"
	"testing"
)

// TestStylePalette_DarkHeadingHierarchy locks the H1-H6 ladder
// after the 2026-04-30 redesign: blue at H1/H2, then pure gray-
// scale gradient. The "color → gray" shift is what gives the eye
// an immediate sense of hierarchy without four similar shades of
// blue/cyan competing.
func TestStylePalette_DarkHeadingHierarchy(t *testing.T) {
	cfg := buildDarkPalette()
	want := map[string]string{
		"H1": "15",
		"H2": "39",
		"H3": "252",
		"H4": "245",
		"H5": "240",
		"H6": "238",
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
	// Heading family fallback aligns with H4 (mid gray) so unexpected
	// H7+ degrades into the gray ladder rather than jumping back to a
	// saturated color.
	if cfg.Heading.Color == nil || *cfg.Heading.Color != "245" {
		t.Errorf("Heading.Color = %v, want 245 (mid gray fallback)", deref(cfg.Heading.Color))
	}
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

// TestStylePalette_DarkChromaHueDiscipline pins the chroma fenced-
// code rules: red is reserved for diagnostics + diff-deleted;
// operators are neutral grey; built-ins are yellow (not pink/red).
// Each rule closes a specific failure mode in the pre-2026-04-29
// palette.
func TestStylePalette_DarkChromaHueDiscipline(t *testing.T) {
	cfg := buildDarkPalette()
	chroma := cfg.CodeBlock.Chroma
	if chroma == nil {
		t.Fatal("Chroma must be non-nil")
	}
	op := *chroma.Operator.Color
	for _, banned := range []string{"#FF", "#EF", "#FD"} {
		if strings.HasPrefix(strings.ToUpper(op), banned) {
			t.Errorf("operator color %s is in the warm-red family — must be neutral grey", op)
		}
	}
	nb := strings.ToLower(*chroma.NameBuiltin.Color)
	if strings.HasPrefix(nb, "#ff5") || strings.HasPrefix(nb, "#ff8") || strings.HasPrefix(nb, "#ef") {
		t.Errorf("name.builtin %s is in the warm-red family", nb)
	}
	gd := strings.ToLower(*chroma.GenericDeleted.Color)
	if !strings.HasPrefix(gd, "#ff") && !strings.HasPrefix(gd, "#fd") &&
		!strings.HasPrefix(gd, "#cf") {
		t.Errorf("generic.deleted %s — diff red is load-bearing, must be red family", gd)
	}
	gi := strings.ToLower(*chroma.GenericInserted.Color)
	if !isGreenFamily(gi) {
		t.Errorf("generic.inserted %s — diff green is load-bearing, must be green family", gi)
	}
}

// TestStylePalette_DarkInlineFamilies locks the four family
// assignments after the 2026-04-30 redesign:
//   blue    39  — Link, LinkText, Item, Enumeration   (interactive)
//   amber  215  — Code (inline), BlockQuote           (literal/quoted)
//   gray   238/240/252  — HR, Strikethrough, Emph     (structure)
//   white   15  — Strong, H1                          (max contrast)
//
// Each element is asserted against its expected family color so a
// regression that flips one back into the old "everything is cyan"
// world is caught at test time.
func TestStylePalette_DarkInlineFamilies(t *testing.T) {
	cfg := buildDarkPalette()
	tests := []struct {
		name string
		got  *string
		want string
	}{
		{"Code (inline)", cfg.Code.Color, "215"},
		{"BlockQuote", cfg.BlockQuote.Color, "222"},
		{"Link", cfg.Link.Color, "39"},
		{"LinkText", cfg.LinkText.Color, "39"},
		{"Item", cfg.Item.Color, "39"},
		{"Enumeration", cfg.Enumeration.Color, "39"},
		{"Strong", cfg.Strong.Color, "15"},
		{"Emph", cfg.Emph.Color, "252"},
		{"Strikethrough", cfg.Strikethrough.Color, "240"},
		{"HorizontalRule", cfg.HorizontalRule.Color, "238"},
		// Table cell color is intentionally NOT pinned here — cells
		// MUST inherit terminal foreground (Text.Color contract). See
		// TestStylePalette_TableCellsInheritTerminalDefault for the
		// readability rationale.
	}
	for _, tt := range tests {
		if tt.got == nil {
			t.Errorf("%s: missing explicit color, want %s", tt.name, tt.want)
			continue
		}
		if *tt.got != tt.want {
			t.Errorf("%s color = %s, want %s", tt.name, *tt.got, tt.want)
		}
	}
	// Strong must be bold (the "carry the signal via attribute, not
	// color" contract); explicit color 15 + bold = pure white bold.
	if cfg.Strong.Bold == nil || !*cfg.Strong.Bold {
		t.Errorf("Strong must be bold; got %v", cfg.Strong.Bold)
	}
	// Emph must be italic (italic IS the signal).
	if cfg.Emph.Italic == nil || !*cfg.Emph.Italic {
		t.Errorf("Emph must be italic; got %v", cfg.Emph.Italic)
	}
	// Strikethrough must have CrossedOut attribute — color alone
	// would not communicate the deletion intent.
	if cfg.Strikethrough.CrossedOut == nil || !*cfg.Strikethrough.CrossedOut {
		t.Errorf("Strikethrough must have CrossedOut=true; got %v", cfg.Strikethrough.CrossedOut)
	}
	// Link / LinkText must underline (Web/IDE contract: blue +
	// underline = clickable). Without underline the blue alone
	// would visually merge with list bullets and lose the link
	// affordance.
	if cfg.Link.Underline == nil || !*cfg.Link.Underline {
		t.Errorf("Link must underline; got %v", cfg.Link.Underline)
	}
	if cfg.LinkText.Underline == nil || !*cfg.LinkText.Underline {
		t.Errorf("LinkText must underline; got %v", cfg.LinkText.Underline)
	}
}

// TestStylePalette_DarkTaskGlyphs verifies the task list checkbox
// glyphs render as styled UTF-8 brackets. The pre-redesign palette
// did not set Task at all so glamour's default `[X] / [ ]` literal
// leaked through; the redesign sets explicit `[✓] / [ ]` so checked
// vs unchecked reads at a glance.
func TestStylePalette_DarkTaskGlyphs(t *testing.T) {
	cfg := buildDarkPalette()
	if cfg.Task.Ticked != "[✓]" {
		t.Errorf("Task.Ticked = %q, want %q", cfg.Task.Ticked, "[✓]")
	}
	if cfg.Task.Unticked != "[ ]" {
		t.Errorf("Task.Unticked = %q, want %q", cfg.Task.Unticked, "[ ]")
	}
}

// TestStylePalette_DarkTextFallbackUnset locks the "terminal owns
// the foreground" red line: cfg.Text.Color MUST stay nil so the
// user's terminal theme dictates default prose color. Pinning
// Text.Color would override solarized / one-dark / custom themes.
func TestStylePalette_DarkTextFallbackUnset(t *testing.T) {
	cfg := buildDarkPalette()
	if cfg.Text.Color != nil {
		t.Errorf("Text.Color must remain nil so terminal theme owns the foreground; got %s", *cfg.Text.Color)
	}
}

// TestStylePalette_TableCellsInheritTerminalDefault locks the
// regression fix for the user-reported "表格的渲染字体颜色和背景太接近
// 了，看不清楚" readability bug. Pre-fix the dark palette set
// Table.StyleBlock.StylePrimitive.Color = "238" and the light palette
// set "250" with the comment "colors the grid runes without touching
// cell content". That comment was incorrect: glamour's
// TableCellElement.Render (vendor ansi/table.go:171) applies
// Table.StylePrimitive to ALL cell content, so cells were rendered
// in #444 (dark) / #d0d0d0 (light) — barely distinguishable from
// typical terminal backgrounds. Grid runes (┼ / │ / ─) are written
// via lipgloss.Border (table.go:104) which does NOT consume
// StylePrimitive.Color, so leaving the cell color UNSET fixes
// readability without affecting the grid appearance. This test pins
// `Color == nil` so a future re-paint accidentally re-introducing
// the dim cell color is caught at test time.
func TestStylePalette_TableCellsInheritTerminalDefault(t *testing.T) {
	dark := buildDarkPalette()
	if dark.Table.StyleBlock.StylePrimitive.Color != nil {
		t.Errorf("dark palette: Table cell color must be nil so cells inherit terminal foreground (readability bug fix); got %q", *dark.Table.StyleBlock.StylePrimitive.Color)
	}
	light := buildLightPalette()
	if light.Table.StyleBlock.StylePrimitive.Color != nil {
		t.Errorf("light palette: Table cell color must be nil so cells inherit terminal foreground (readability bug fix); got %q", *light.Table.StyleBlock.StylePrimitive.Color)
	}
	// Grid separators must still be set — they're the structural
	// indicators that turn a stack of strings into a visible table.
	for _, c := range []struct {
		name string
		cfg  *string
	}{
		{"dark CenterSeparator", dark.Table.CenterSeparator},
		{"dark ColumnSeparator", dark.Table.ColumnSeparator},
		{"dark RowSeparator", dark.Table.RowSeparator},
		{"light CenterSeparator", light.Table.CenterSeparator},
		{"light ColumnSeparator", light.Table.ColumnSeparator},
		{"light RowSeparator", light.Table.RowSeparator},
	} {
		if c.cfg == nil || *c.cfg == "" {
			t.Errorf("%s missing — grid runes must be configured for visible table structure", c.name)
		}
	}
}

// TestStylePalette_NoColorCollisions guards the load-bearing
// invariant after the redesign: Strong vs H3, inline Code vs any
// heading, BlockQuote vs Code (different family), Item vs Emph
// (different family) — none of these may share a color or the eye
// loses the ability to distinguish them.
func TestStylePalette_NoColorCollisions(t *testing.T) {
	cfg := buildDarkPalette()
	pairs := []struct {
		a, b   string
		ac, bc *string
	}{
		{"Strong", "H3", cfg.Strong.Color, cfg.H3.Color},
		{"Strong", "H4", cfg.Strong.Color, cfg.H4.Color},
		{"Code", "H2", cfg.Code.Color, cfg.H2.Color},
		{"Code", "H3", cfg.Code.Color, cfg.H3.Color},
		{"Code", "Item", cfg.Code.Color, cfg.Item.Color},
		{"BlockQuote", "Code", cfg.BlockQuote.Color, cfg.Code.Color},
		{"BlockQuote", "H2", cfg.BlockQuote.Color, cfg.H2.Color},
		{"Item", "Emph", cfg.Item.Color, cfg.Emph.Color},
		// HorizontalRule (238) intentionally MAY share its dim-gray
		// with H6 (238): HR is a line of dashes and H6 is text, so
		// the color collision does not produce visual ambiguity. Keep
		// them in the same "deepest structural" cohort.
	}
	for _, p := range pairs {
		if p.ac == nil || p.bc == nil {
			continue
		}
		if *p.ac == *p.bc {
			t.Errorf("color collision: %s and %s both = %s — must differ for visual hierarchy",
				p.a, p.b, *p.ac)
		}
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
		{"Heading fallback", cfg.Heading.Color},
		{"Code", cfg.Code.Color},
		{"HR", cfg.HorizontalRule.Color},
		{"Link", cfg.Link.Color},
		{"LinkText", cfg.LinkText.Color},
		{"BlockQuote", cfg.BlockQuote.Color},
		{"Strong", cfg.Strong.Color},
		{"Emph", cfg.Emph.Color},
		{"Strikethrough", cfg.Strikethrough.Color},
		{"Item", cfg.Item.Color},
		{"Enumeration", cfg.Enumeration.Color},
		// Table cell color: see dark palette test — must stay nil so
		// cells inherit terminal foreground regardless of background
		// detection.
	} {
		if level.c == nil {
			t.Errorf("light palette: %s missing explicit colour (mirror with dark)", level.name)
		}
	}
	if cfg.Strikethrough.CrossedOut == nil || !*cfg.Strikethrough.CrossedOut {
		t.Errorf("light palette: Strikethrough must have CrossedOut=true")
	}
	if cfg.Link.Underline == nil || !*cfg.Link.Underline {
		t.Errorf("light palette: Link must underline")
	}
	if cfg.Text.Color != nil {
		t.Errorf("light palette: Text.Color must remain nil; got %s", *cfg.Text.Color)
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

// deref pulls a *string into its value or "<nil>" for cleaner
// failure messages.
func deref(p *string) string {
	if p == nil {
		return "<nil>"
	}
	return *p
}
