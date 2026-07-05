package repomap

// Pins for qualified-name oracle resolution (QNO batch, 2026-07-05).
// Three pinned shapes per the batch ruling:
//  1. qualified HIT — deterministic decomposition + exact scope match;
//  2. precise MISS — unresolvable qualifier / absent bare name stays a miss;
//  3. no-fuzzy — similar-but-not-equal spellings never hit.
// Plus the baseline contrast: SymbolExistsFlat itself still misses
// qualified spellings (lane ownership unchanged — hard gates that read
// the flat index keep byte-identical behaviour).

import (
	"testing"

	rmtypes "github.com/hanchaoqun/codrax/internal/tool/repomap/types"
	"github.com/hanchaoqun/codrax/internal/types"
)

// qnoFixtureGraph builds the canonical s1a-shaped fixture: method
// Run with receiver Gate in package gate, plus a same-named bare
// function Run in package other, plus a Java-shaped class member.
func qnoFixtureGraph() *rmtypes.Graph {
	return &rmtypes.Graph{
		SymbolDefs: map[string][]*rmtypes.Symbol{
			"Run": {
				{Name: "Run", Kind: "method", File: "internal/analysis/gate/gate.go", Receiver: "Gate"},
				{Name: "Run", Kind: "function", File: "internal/other/run.go"},
			},
			"RunWith": {
				{Name: "RunWith", Kind: "function", File: "internal/analysis/gate/gate.go"},
			},
			"method": {
				{Name: "method", Kind: "method", File: "src/com/foo/Bar.java", Parent: "Bar"},
			},
			"carve": {
				{Name: "carve", Kind: "function", File: "src/tok/carve.c"},
			},
		},
		FileIndex: map[string]*rmtypes.FileInfo{
			"internal/analysis/gate/gate.go": {RelPath: "internal/analysis/gate/gate.go", Language: "go", Package: "gate", ParseTier: 1},
			"internal/other/run.go":          {RelPath: "internal/other/run.go", Language: "go", Package: "other", ParseTier: 2},
			"src/com/foo/Bar.java":           {RelPath: "src/com/foo/Bar.java", Language: "java", Package: "com.foo", ParseTier: 1},
			"src/tok/carve.c":                {RelPath: "src/tok/carve.c", Language: "c", ParseTier: 1}, // no Package — dir-basename lane
		},
	}
}

func qnoOracle(t *testing.T) types.QualifiedSymbolOracle {
	t.Helper()
	qo, ok := NewSymbolOracle(qnoFixtureGraph()).(types.QualifiedSymbolOracle)
	if !ok {
		t.Fatal("graph oracle must implement types.QualifiedSymbolOracle")
	}
	return qo
}

// Baseline contrast — the historical miss stays a miss on the FLAT
// lane. Qualified resolution is a separate opt-in extension; the
// many hard-gate consumers of SymbolExistsFlat are byte-unchanged.
func TestQualifiedOracle_FlatLaneStillMissesQualifiedForms(t *testing.T) {
	o := NewSymbolOracle(qnoFixtureGraph())
	for _, form := range []string{"gate.Run", "Gate.Run", "(*Gate).Run", "com.foo.Bar.method"} {
		if found, tier := o.SymbolExistsFlat(form); found || tier != 0 {
			t.Errorf("SymbolExistsFlat(%q) = (%v,%d); flat lane must keep missing qualified forms", form, found, tier)
		}
	}
	if found, _ := o.SymbolExistsFlat("Run"); !found {
		t.Error("bare Run must still hit the flat lane")
	}
}

// Pin 1 — qualified HIT across decomposition grammars and scope levels.
func TestQualifiedOracle_QualifiedHit(t *testing.T) {
	qo := qnoOracle(t)
	cases := []struct {
		form     string
		wantTier int
		why      string
	}{
		{"gate.Run", 1, "package qualifier (s1a canonical form)"},
		{"Gate.Run", 1, "receiver-type qualifier"},
		{"(*Gate).Run", 1, "Go pointer method-expression receiver"},
		{"(g *Gate).Run", 1, "Go receiver declaration form"},
		{"gate.run", 1, "case-form-aware trailing segment (flat equality rule)"},
		{"other.Run", 2, "same bare name disambiguated to the other package (tier follows the matched def)"},
		{"com.foo.Bar.method", 1, "multi-segment Java package + parent class, every segment verified"},
		{"tok.carve", 1, "dir-basename lane for languages without FileInfo.Package"},
		{"com::foo::Bar#method", 1, "mixed :: / # separators decompose to the same segments"},
	}
	for _, tc := range cases {
		found, tier := qo.QualifiedSymbolExists(tc.form)
		if !found || tier != tc.wantTier {
			t.Errorf("QualifiedSymbolExists(%q) = (%v,%d), want (true,%d) — %s", tc.form, found, tier, tc.wantTier, tc.why)
		}
	}
}

// Pin 2 — precise MISS: absent bare names and unverifiable qualifiers
// are honest misses, and single-segment spellings stay owned by the
// exact/flat lanes.
func TestQualifiedOracle_PreciseMiss(t *testing.T) {
	qo := qnoOracle(t)
	misses := []struct {
		form string
		why  string
	}{
		{"gate.Missing", "bare name not in symbol table"},
		{"nosuch.Run", "qualifier matches no scope level of any Run def"},
		{"wrong.foo.Bar.method", "one bad leading package segment fails the all-segments rule"},
		{"Run", "single-segment: flat/exact lanes own it, qualified lane answers false"},
		{"RunWith", "single-segment even though symbol exists"},
		{"()", "decomposes to nothing — honest decomposition failure"},
		{"gate.", "trailing separator leaves one segment — not a qualified form"},
	}
	for _, tc := range misses {
		if found, tier := qo.QualifiedSymbolExists(tc.form); found || tier != 0 {
			t.Errorf("QualifiedSymbolExists(%q) = (%v,%d), want (false,0) — %s", tc.form, found, tier, tc.why)
		}
	}
}

// Pin 3 — no-fuzzy red line: similar-but-not-equal names never hit.
// Deterministic decomposition + exact flat equality only; no
// similarity, prefix, substring, or edit-distance rescue.
func TestQualifiedOracle_NoFuzzySimilarNamesMiss(t *testing.T) {
	qo := qnoOracle(t)
	fuzz := []struct {
		form string
		why  string
	}{
		{"gat.Run", "qualifier one letter short of gate"},
		{"gates.Run", "qualifier one letter past gate"},
		{"gate.Ru", "trailing segment prefix of Run"},
		{"gate.Runn", "trailing segment edit-distance-1 from Run"},
		{"gate.Runner", "trailing segment superstring of Run"},
	}
	for _, tc := range fuzz {
		if found, _ := qo.QualifiedSymbolExists(tc.form); found {
			t.Errorf("QualifiedSymbolExists(%q) hit; must miss — %s (禁 fuzzy)", tc.form, tc.why)
		}
	}
	// Contrast guard: gate.RunWith is a REAL package-level function in
	// package gate, so it legitimately hits — proving the misses above
	// come from exactness, not from the lane being dead.
	if found, _ := qo.QualifiedSymbolExists("gate.RunWith"); !found {
		t.Error("gate.RunWith must hit (real symbol in package gate) — the no-fuzzy misses must not be a dead lane")
	}
}

// F3 pin (2026-07-05): a recorded package clause is the authoritative
// scope — the defining-directory basename must NOT widen it. Go
// `pkg/v2/` layouts are the canonical trap: package is `bar`, dir is
// `v2`; "v2.Bar" is an honest miss (accepted false negative), while
// the package spelling keeps hitting. The dir lane stays available
// ONLY for files with no Package (pinned by tok.carve in the hit set).
func TestQualifiedOracle_PackagePresenceSuppressesDirLane(t *testing.T) {
	g := &rmtypes.Graph{
		SymbolDefs: map[string][]*rmtypes.Symbol{
			"Bar": {{Name: "Bar", Kind: "type", File: "pkg/v2/bar.go"}},
		},
		FileIndex: map[string]*rmtypes.FileInfo{
			"pkg/v2/bar.go": {RelPath: "pkg/v2/bar.go", Language: "go", Package: "bar", ParseTier: 1},
		},
	}
	qo := NewSymbolOracle(g).(types.QualifiedSymbolOracle)
	if found, _ := qo.QualifiedSymbolExists("v2.Bar"); found {
		t.Error("dir-basename must not act as scope evidence when a package clause is recorded (v2.Bar must miss)")
	}
	if found, tier := qo.QualifiedSymbolExists("bar.Bar"); !found || tier != 1 {
		t.Errorf("package spelling must keep hitting; got (%v,%d)", found, tier)
	}
}

// Tier semantics — stale FileIndex rows degrade to tier 4 exactly as
// Graph.SymbolExists / the flat index do.
func TestQualifiedOracle_StaleIndexTier4(t *testing.T) {
	g := &rmtypes.Graph{
		SymbolDefs: map[string][]*rmtypes.Symbol{
			"Stale": {{Name: "Stale", File: "gone/pkg/stale.go", Receiver: "Ghost"}},
		},
		FileIndex: map[string]*rmtypes.FileInfo{},
	}
	qo := NewSymbolOracle(g).(types.QualifiedSymbolOracle)
	found, tier := qo.QualifiedSymbolExists("Ghost.Stale")
	if !found || tier != 4 {
		t.Errorf("stale-index qualified hit should be (true,4); got (%v,%d)", found, tier)
	}
}

func TestQualifiedOracle_NilSafety(t *testing.T) {
	if found, tier := (NewSymbolOracle(nil)).(types.QualifiedSymbolOracle).QualifiedSymbolExists("gate.Run"); found || tier != 0 {
		t.Errorf("nil graph must return (false,0); got (%v,%d)", found, tier)
	}
}

// SplitQualifiedSegments grammar pins — the single shared decomposition
// (oracle side AND agent-side expandEntityNameAliases both consume it).
func TestSplitQualifiedSegments_Grammar(t *testing.T) {
	cases := []struct {
		in   string
		want []string
	}{
		{"gate.Run", []string{"gate", "Run"}},
		{"mod::Type::method", []string{"mod", "Type", "method"}},
		{"Foo::Bar#baz", []string{"Foo", "Bar", "baz"}},
		{"(*Gate).Run", []string{"Gate", "Run"}},
		{"(g *Gate).Run", []string{"Gate", "Run"}},
		{"*Gate.Run", []string{"Gate", "Run"}},
		{"RunWith", []string{"RunWith"}},
		{"a..c", []string{"a", "c"}},
		{"", nil},
		{"   ", nil},
		{"()", nil},
	}
	for _, tc := range cases {
		got := SplitQualifiedSegments(tc.in)
		if len(got) != len(tc.want) {
			t.Errorf("SplitQualifiedSegments(%q) = %v, want %v", tc.in, got, tc.want)
			continue
		}
		for i := range got {
			if got[i] != tc.want[i] {
				t.Errorf("SplitQualifiedSegments(%q) = %v, want %v", tc.in, got, tc.want)
				break
			}
		}
	}
}
