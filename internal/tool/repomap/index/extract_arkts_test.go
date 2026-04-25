package index

import (
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/tool/repomap/types"
)

// TestExtractArkTS_EntryComponent verifies the post-pass correctly
// identifies a struct declaration preceded by @Entry + @Component
// decorators as a component symbol, attaches the build() ui-entry,
// and captures @State fields — the fundamentals of ArkTS UI code.
func TestExtractArkTS_EntryComponent(t *testing.T) {
	src := []byte(`
@Entry
@Component
struct Index {
  @State message: string = 'Hello World'

  build() {
    Row() {
      Column() {
        Text(this.message)
      }
    }
  }
}
`)
	pkg, syms, _, _, tier := extractArkTS(src, "pages/Index.ets")
	if pkg != "" {
		t.Errorf("ArkTS has no package concept; got pkg=%q", pkg)
	}
	if tier != 1 {
		t.Errorf("minimal ArkTS should be Tier 1; got tier=%d", tier)
	}

	foundComponent := false
	foundBuild := false
	foundState := false
	for _, s := range syms {
		switch {
		case s.Kind == "component" && s.Name == "Index":
			foundComponent = true
			if !strings.Contains(s.Doc, "@Entry") || !strings.Contains(s.Doc, "@Component") {
				t.Errorf("component Doc should list both decorators; got %q", s.Doc)
			}
			if !s.Exported {
				t.Errorf("@Entry component must be Exported")
			}
			if s.EndLine <= s.Line {
				t.Errorf("component should carry block EndLine; got %+v", s)
			}
		case s.Kind == "ui-entry" && s.Name == "build":
			foundBuild = true
			if s.Parent != "Index" {
				t.Errorf("build() should be parented to Index; got parent=%q", s.Parent)
			}
			if s.EndLine <= s.Line {
				t.Errorf("build() should carry block EndLine; got %+v", s)
			}
		case s.Kind == "state-field" && s.Name == "message":
			foundState = true
		}
	}
	if !foundComponent {
		t.Errorf("missing @Component struct symbol; syms=%+v", symKeys(syms))
	}
	if !foundBuild {
		t.Errorf("missing build() ui-entry; syms=%+v", symKeys(syms))
	}
	if !foundState {
		t.Errorf("missing @State state-field; syms=%+v", symKeys(syms))
	}
}

// TestExtractArkTS_BuilderDecorator pins the @Builder / @Styles /
// @Extend kinds so downstream consumers (rank, skill prompt) can
// rely on a stable symbol-kind contract.
func TestExtractArkTS_BuilderDecorator(t *testing.T) {
	src := []byte(`
@Builder function GlobalCard(title: string) {
  Column() { Text(title) }
}

@Styles function commonStyle() {
  .width('100%')
}

@Extend(Text) function highlight(color: string) {
  .fontColor(color)
}
`)
	_, syms, _, _, _ := extractArkTS(src, "ui/util.ets")
	kindByName := map[string]string{}
	endLineByName := map[string]int{}
	lineByName := map[string]int{}
	for _, s := range syms {
		kindByName[s.Name] = s.Kind
		endLineByName[s.Name] = s.EndLine
		lineByName[s.Name] = s.Line
	}
	for _, tc := range []struct{ name, wantKind string }{
		{"GlobalCard", "builder"},
		{"commonStyle", "styles"},
		{"highlight", "extend"},
	} {
		if got := kindByName[tc.name]; got != tc.wantKind {
			t.Errorf("symbol %q: want kind=%q, got %q", tc.name, tc.wantKind, got)
		}
		if endLineByName[tc.name] <= lineByName[tc.name] {
			t.Errorf("symbol %q should carry block EndLine; got line=%d end=%d", tc.name, lineByName[tc.name], endLineByName[tc.name])
		}
	}
}

// TestExtractArkTS_Imports verifies the ArkTS post-pass finds
// imports even when they coexist with a struct declaration that
// would break strict TS parsing.
func TestExtractArkTS_Imports(t *testing.T) {
	src := []byte(`
import UIAbility from '@ohos.app.ability.UIAbility'
import { window } from '@kit.ArkUI'
import UserCard from '../components/UserCard'

@Entry
@Component
struct Index {
  build() {}
}
`)
	_, _, imps, _, _ := extractArkTS(src, "pages/Index.ets")
	got := map[string]bool{}
	for _, i := range imps {
		got[i.Path] = true
	}
	for _, want := range []string{
		"@ohos.app.ability.UIAbility",
		"@kit.ArkUI",
		"../components/UserCard",
	} {
		if !got[want] {
			t.Errorf("missing import path %q; got %+v", want, got)
		}
	}
}

// TestExtractArkTS_NonArkTSFallback verifies that a plain
// TypeScript file (no ArkTS-specific surface) still parses at
// Tier 1 — an ArkTS project's regular .ts helpers must round-trip.
func TestExtractArkTS_NonArkTSFallback(t *testing.T) {
	src := []byte(`
export function add(a: number, b: number): number {
  return a + b
}
export class Counter {
  count = 0
  inc() { this.count++ }
}
`)
	_, syms, _, _, tier := extractArkTS(src, "util/math.ts")
	if tier != 1 {
		t.Errorf("pure TS in ArkTS project must be Tier 1; got %d", tier)
	}
	hasAdd := false
	hasCounter := false
	for _, s := range syms {
		if s.Name == "add" {
			hasAdd = true
		}
		if s.Name == "Counter" {
			hasCounter = true
		}
	}
	if !hasAdd || !hasCounter {
		t.Errorf("plain TS extraction missed symbols: add=%v Counter=%v", hasAdd, hasCounter)
	}
}

func symKeys(syms []types.Symbol) []string {
	out := make([]string, 0, len(syms))
	for _, s := range syms {
		out = append(out, s.Kind+":"+s.Name)
	}
	return out
}
