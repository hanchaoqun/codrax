package glossarylint

import (
	"path/filepath"
	"strings"
	"testing"
)

const scratchPromptPackage = `package scratch

import (
	"encoding/json"

	"github.com/hanchaoqun/codrax/internal/llm"
)

const sharedTail = " and the TaskGraph tail"

const reviewerSystemPrompt = "You review answers." + sharedTail

var operatorBanner = "finalizer text that is NOT a prompt surface"

var reviewTool = llm.ToolSchema{
	Name:        "emit_review",
	Description: "Explain what the EvidencePlan needs.",
	Parameters:  json.RawMessage(` + "`" + `{"type":"object","description":"BusContext leaks here"}` + "`" + `),
}

func localSchema() llm.ToolSchema {
	return llm.ToolSchema{
		Name:        "emit_local",
		Description: "clean " + "concatenation",
		Parameters:  json.RawMessage(reviewerSystemPrompt),
	}
}

func zero() llm.ToolSchema { return llm.ToolSchema{} }

func tail(s string) string { return s }

func messages(userLine string) []llm.Message {
	systemPrompt := reviewerSystemPrompt + " plus HypothesisSet"
	return []llm.Message{
		{Role: "system", Content: systemPrompt + "\n\n" + tail(userLine)},
		{Role: "user", Content: userLine},
	}
}
`

func TestScanPromptSurfaces_RecognizedShapes(t *testing.T) {
	dir := writeScratchPackage(t, map[string]string{"p.go": scratchPromptPackage})
	hits, surfaces, err := ScanPromptSurfaces(dir)
	if err != nil {
		t.Fatalf("scan: %v", err)
	}
	var owners []string
	for _, s := range surfaces {
		owners = append(owners, s.Label[strings.LastIndex(s.Label, " ")+1:])
	}
	wantOwners := "reviewerSystemPrompt ToolSchema.Name ToolSchema.Description ToolSchema.Parameters ToolSchema.Name ToolSchema.Description ToolSchema.Parameters SystemMessage.Content"
	if got := strings.Join(owners, " "); got != wantOwners {
		t.Fatalf("surfaces = %v\nwant %s", owners, wantOwners)
	}
	// operatorBanner is a var without the Prompt suffix and outside any
	// ToolSchema literal: it is operator text and must NOT be scanned.
	wantHits := []string{
		"p.go:11/TaskGraph",    // prompt const through a concatenated const
		"p.go:15/EvidencePlan", // package-level ToolSchema description
		"p.go:15/BusContext",   // json.RawMessage(literal)
		"p.go:22/TaskGraph",    // function-local literal, Parameters bound to a prompt const
		"p.go:36/TaskGraph",    // Role:"system" Content through a single-assignment local + call pass-through
		"p.go:36/HypothesisSet",
	}
	var got []string
	for _, h := range hits {
		pos := h.Label[:strings.Index(h.Label, " ")]
		got = append(got, filepath.Base(pos)+"/"+h.Term)
	}
	if strings.Join(got, " ") != strings.Join(wantHits, " ") {
		t.Fatalf("hits = %v\nwant %v", got, wantHits)
	}
}

func TestScanPromptSurfaces_UnrecognizedShapesFailLoud(t *testing.T) {
	cases := map[string]string{
		"local variable": `package scratch

import "github.com/hanchaoqun/codrax/internal/llm"

func build(desc string) llm.ToolSchema {
	return llm.ToolSchema{Name: "x", Description: desc}
}
`,
		"function result": `package scratch

import "github.com/hanchaoqun/codrax/internal/llm"

func text() string { return "" }

var tool = llm.ToolSchema{Name: "x", Description: text()}
`,
		"positional element": `package scratch

import "github.com/hanchaoqun/codrax/internal/llm"

var tool = llm.ToolSchema{"x", "desc", nil}
`,
		"prompt const bound to a call": `package scratch

func text() string { return "" }

var dynamicSystemPrompt = text()
`,
		"reassigned local in a system message": `package scratch

import "github.com/hanchaoqun/codrax/internal/llm"

func build(flag bool) []llm.Message {
	prompt := "a"
	if flag {
		prompt = "b"
	}
	return []llm.Message{{Role: "system", Content: prompt}}
}
`,
	}
	for name, body := range cases {
		dir := writeScratchPackage(t, map[string]string{"p.go": body})
		_, _, err := ScanPromptSurfaces(dir)
		if err == nil || !strings.Contains(err.Error(), "unrecognized") {
			t.Errorf("%s: expected an unrecognized-shape error, got %v", name, err)
		}
	}
}

func TestRunPromptSurfaceScan_RequiresASurface(t *testing.T) {
	dir := writeScratchPackage(t, map[string]string{"p.go": "package scratch\n\nvar x = \"nothing model-facing\"\n"})
	rec := &recordingTB{TB: t}
	func() {
		defer func() { _ = recover() }()
		RunPromptSurfaceScan(rec, dir)
	}()
	if !rec.fatal {
		t.Fatalf("a marker on a package with no prompt surface must fail loud")
	}
}
