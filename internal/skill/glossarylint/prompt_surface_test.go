package glossarylint

import (
	"path/filepath"
	"sort"
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
	wantOwners := "reviewerSystemPrompt ToolSchema.Name ToolSchema.Description ToolSchema.Parameters ToolSchema.Name ToolSchema.Description ToolSchema.Parameters SystemMessage.Content UserMessage.Content"
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
		"p.go:36/TaskGraph",    // Role:"system" Content through a single-assignment local + same-package call operand
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

// scratchInstructionPackage exercises every text-flow shape of the
// instruction walker on one synthetic package. Each glossary token
// names the shape that must (or must not) bind it; the expected roster
// below is the shape census.
const scratchInstructionPackage = `package scratch

import (
	"context"
	"fmt"
	"strings"

	"github.com/hanchaoqun/codrax/internal/llm"
)

const reviewerSystemPrompt = "You review answers."

type planner struct{ adapter llm.Adapter }

// repairPrompt is a builder function: strings.Builder writes, a
// fmt.Fprintf into the builder, and a helper the builder is passed to.
func repairPrompt(err error) string {
	var b strings.Builder
	b.WriteString("Your prior call was malformed; the TaskGraph is wrong.\n")
	fmt.Fprintf(&b, "error=%v (EvidencePlan)\n", err)
	writeFooter(&b, "closing AnswerContract note")
	return b.String()
}

func writeFooter(b *strings.Builder, tail string) {
	b.WriteString("Footer: MutableState " + tail)
}

func section(label string) string { return "## " + label + "\n" }

func (p *planner) plan(ctx context.Context, userLine string, records []string) {
	var b strings.Builder
	fmt.Fprintf(&b, "## user_request (RiskMatrix)\n%s\n", userLine)
	b.WriteString(section("BusContext rows"))
	for _, r := range records {
		b.WriteString(r)
	}
	messages := []llm.Message{
		{Role: "system", Content: reviewerSystemPrompt},
		{Role: "user", Content: b.String()},
	}
	p.adapter.Chat(ctx, messages, nil, llm.ChatOptions{})
	repair := append([]llm.Message(nil), messages...)
	repair = append(repair, llm.Message{Role: "user", Content: repairPrompt(nil)})
	_ = repair
}

// send binds its Content to a parameter: the hop reaches every caller.
func (p *planner) send(ctx context.Context, prompt string) {
	p.adapter.Chat(ctx, []llm.Message{{Role: "user", Content: prompt}}, nil, llm.ChatOptions{})
}

func (p *planner) planRepair(ctx context.Context, userLine string) {
	p.send(ctx, "Repair the HypothesisSet: "+userLine)
}

// forward is pure forwarding: the hop keeps going to planTwice.
func (p *planner) forward(ctx context.Context, prompt string) { p.send(ctx, prompt) }

func (p *planner) planTwice(ctx context.Context) { p.forward(ctx, "AnalysisIR through forwarding") }

// answer reassigns a local: both values are bound.
func (p *planner) answer(ctx context.Context, userLine, prior string) {
	content := userLine
	if prior != "" {
		content = "## Prior (RequestModel)\n" + prior + "\n## Current\n" + userLine
	}
	p.adapter.Chat(ctx, []llm.Message{{Role: "user", Content: content}}, nil, llm.ChatOptions{})
}

func operatorHelp() []string { return []string{"help: the finalizer knob"} }

// readLine's parameter is prompt decoration, not its result.
func readLine(prompt string) string { return stdin() }

func stdin() string { return "" }

func (p *planner) dispatch(ctx context.Context) {
	hint := "PendingReads carried as prior context"
	for _, hint := range operatorHelp() {
		fmt.Println(hint)
	}
	line := readLine("prompt> QualityGate decoration")
	p.answer(ctx, line, hint)
	p.answer(ctx, line, "")
}

func (p *planner) toolRound(ctx context.Context, resp llm.Response) {
	var toolReply string
	switch resp.Content {
	case "x":
		toolReply = lookup(resp.Content)
	default:
		toolReply = fmt.Sprintf("(tool %q not available — GroundingStatus)", resp.Content)
	}
	p.adapter.Chat(ctx, []llm.Message{
		{Role: "assistant", Content: resp.Content},
		{Role: "tool", Content: toolReply},
	}, nil, llm.ChatOptions{})
}

func lookup(key string) string { return "found " + key }

func buildSystem() string { return "system built in place: the ReadSet" }

func (p *planner) wholeCall(ctx context.Context) {
	p.adapter.Chat(ctx, []llm.Message{{Role: "system", Content: buildSystem()}}, nil, llm.ChatOptions{})
}
`

func TestScanPromptSurfaces_InstructionFlowShapes(t *testing.T) {
	dir := writeScratchPackage(t, map[string]string{"p.go": scratchInstructionPackage})
	hits, surfaces, err := ScanPromptSurfaces(dir)
	if err != nil {
		t.Fatalf("scan: %v", err)
	}
	owners := map[string]int{}
	for _, s := range surfaces {
		owners[s.Label[strings.LastIndex(s.Label, " ")+1:]]++
	}
	if owners["UserMessage.Content"] != 4 || owners["SystemMessage.Content"] != 2 || owners["AssistantMessage.Content"] != 1 || owners["ToolMessage.Content"] != 1 {
		t.Fatalf("message surfaces = %v, want 4 user / 2 system / 1 assistant / 1 tool", owners)
	}
	got := map[string]bool{}
	for _, h := range hits {
		got[h.Term] = true
	}
	var terms []string
	for term := range got {
		terms = append(terms, term)
	}
	sort.Strings(terms)
	want := []string{
		"AnalysisIR",      // pure forwarding keeps the hop open to the authoring caller
		"AnswerContract",  // argument passed alongside the builder into a same-package helper
		"BusContext",      // same-package call operand of a WriteString, bound per call site
		"EvidencePlan",    // fmt.Fprintf format into the builder
		"GroundingStatus", // Role:"tool" content through a reassigned local and fmt.Sprintf
		"HypothesisSet",   // the parameter hop to a caller's literal
		"MutableState",    // helper writes into the builder it received
		"PendingReads",    // the lexically resolved outer `hint` local, not the loop variable
		"ReadSet",         // same-package call as the whole system Content
		"RequestModel",    // reassigned local: both values bound
		"RiskMatrix",      // fmt.Fprintf into the enclosing function's builder
		"TaskGraph",       // builder function returned into a repair message
	}
	if strings.Join(terms, " ") != strings.Join(want, " ") {
		t.Fatalf("bound terms = %v\nwant %v", terms, want)
	}
	// Shapes that must stay OUTSIDE the flow: a callee's decoration
	// parameter that is not on its return path (QualityGate), a loop
	// variable shadowing the local that feeds the prompt (finalizer), and
	// the data-only assistant echo.
	for _, term := range []string{"QualityGate", "finalizer"} {
		if got[term] {
			t.Errorf("%s must not be bound: it is not on the prompt-text flow", term)
		}
	}
}

// TestScanPromptSurfaces_SiteBoundCalleeParameters pins the per-call-site
// binding: two callers of one helper each see only their own argument.
func TestScanPromptSurfaces_SiteBoundCalleeParameters(t *testing.T) {
	dir := writeScratchPackage(t, map[string]string{"p.go": `package scratch

import "github.com/hanchaoqun/codrax/internal/llm"

func wrap(s string) string { return "[" + s + "]" }

func prompt() llm.Message { return llm.Message{Role: "user", Content: wrap("TaskGraph in the prompt")} }

func banner() string { return wrap("EvidencePlan in operator output") }
`})
	hits, _, err := ScanPromptSurfaces(dir)
	if err != nil {
		t.Fatalf("scan: %v", err)
	}
	var terms []string
	for _, h := range hits {
		terms = append(terms, h.Term)
	}
	if strings.Join(terms, " ") != "TaskGraph" {
		t.Fatalf("hits = %v, want only the argument of the prompt's own call to wrap", terms)
	}
}

// TestScanPromptSurfaces_SystemExactTextAndBuilderPartsBothScanned pins
// that a system Content of the form `const + samePackageBuilder()` scans
// BOTH the exactly-resolved const text (under the message site) and the
// builder's flow-bound literals (under their own lines). EVOLUTION RECORD
// (batch six fold-in, F6-prompt-surface #5): the hit loop scanned the
// resolved text only when no parts were bound, so the const beside a
// builder was reported bound and never scanned — red on 381f36cc9 (only
// the builder's EvidencePlan hit, TaskGraph in the const silent).
func TestScanPromptSurfaces_SystemExactTextAndBuilderPartsBothScanned(t *testing.T) {
	dir := writeScratchPackage(t, map[string]string{"p.go": `package scratch

import "github.com/hanchaoqun/codrax/internal/llm"

const toolUseTail = "Use tools when the TaskGraph wants them."

func tail() string { return "Builder tail naming the EvidencePlan." }

func build() []llm.Message {
	return []llm.Message{{Role: "system", Content: toolUseTail + "\n" + tail()}}
}
`})
	hits, surfaces, err := ScanPromptSurfaces(dir)
	if err != nil {
		t.Fatalf("scan: %v", err)
	}
	if len(surfaces) != 1 || !strings.HasSuffix(surfaces[0].Label, "p.go:10 SystemMessage.Content") {
		t.Fatalf("surfaces = %+v, want the one system message at p.go:10", surfaces)
	}
	var got []string
	for _, h := range hits {
		pos := h.Label[:strings.Index(h.Label, " ")]
		got = append(got, filepath.Base(pos)+"/"+h.Term)
	}
	want := []string{
		"p.go:10/TaskGraph",   // the const, resolved exactly, reported at the message site
		"p.go:7/EvidencePlan", // the builder literal, bound by flow, reported at its own line
	}
	if strings.Join(got, " ") != strings.Join(want, " ") {
		t.Fatalf("hits = %v\nwant %v", got, want)
	}
	if !strings.Contains(surfaces[0].Text, "TaskGraph") || !strings.Contains(surfaces[0].Text, "EvidencePlan") {
		t.Fatalf("surface Text must be the union of the exact text and the builder parts, got %q", surfaces[0].Text)
	}
}

// TestScanPromptSurfaces_ContentPartsTextIsBound pins that the Text of a
// Type:"text" ContentParts element is a surface of its own, bound through
// the message's role lane (flow for user, exact for system), while image
// parts contribute nothing. EVOLUTION RECORD (batch six fold-in,
// F6-prompt-surface #6): the lane read only the Content key — red on
// 381f36cc9 (two Content surfaces, zero ContentParts surfaces, zero hits).
func TestScanPromptSurfaces_ContentPartsTextIsBound(t *testing.T) {
	dir := writeScratchPackage(t, map[string]string{"p.go": `package scratch

import "github.com/hanchaoqun/codrax/internal/llm"

const visionSystemPrompt = "Describe the image."

const partHeader = "Part header: the HypothesisSet."

func caption(kind string) string { return "Caption kind=" + kind + " (RiskMatrix)" }

func build(dataURL string) []llm.Message {
	return []llm.Message{
		{Role: "system", Content: visionSystemPrompt, ContentParts: []llm.ContentPart{{Type: "text", Text: partHeader}}},
		{
			Role:    "user",
			Content: "Extract text evidence.",
			ContentParts: []llm.ContentPart{
				{Type: "image_url", ImageURL: dataURL, Detail: "high"},
				{Type: "Text ", Text: caption("chart") + " and the TaskGraph"},
				{Type: "", Text: "untyped part is text: EvidencePlan"},
			},
		},
	}
}
`})
	hits, surfaces, err := ScanPromptSurfaces(dir)
	if err != nil {
		t.Fatalf("scan: %v", err)
	}
	var owners []string
	for _, s := range surfaces {
		owners = append(owners, filepath.Base(s.Label))
	}
	wantOwners := "p.go:5 visionSystemPrompt p.go:13 SystemMessage.Content p.go:13 SystemMessage.ContentParts.Text p.go:14 UserMessage.Content p.go:19 UserMessage.ContentParts.Text p.go:20 UserMessage.ContentParts.Text"
	if got := strings.Join(owners, " "); got != wantOwners {
		t.Fatalf("surfaces = %v\nwant %s", owners, wantOwners)
	}
	var got []string
	for _, h := range hits {
		got = append(got, filepath.Base(h.Label)+"/"+h.Term)
	}
	want := []string{
		"p.go:13 SystemMessage.ContentParts.Text/HypothesisSet", // system lane: the const resolved exactly at the part site
		"p.go:9 UserMessage.ContentParts.Text/RiskMatrix",       // user lane: the builder literal reached by flow
		"p.go:19 UserMessage.ContentParts.Text/TaskGraph",       // user lane: the inline literal (Type "Text " normalizes to text)
		"p.go:20 UserMessage.ContentParts.Text/EvidencePlan",    // user lane: Type "" serializes as text
	}
	if strings.Join(got, " ") != strings.Join(want, " ") {
		t.Fatalf("hits = %v\nwant %v", got, want)
	}
}

func TestScanPromptSurfaces_ContentPartsUnrecognizedShapesFailLoud(t *testing.T) {
	const header = `package scratch

import "github.com/hanchaoqun/codrax/internal/llm"

`
	cases := map[string]struct{ src, want string }{
		"parts bound to a variable": {header + `func build(parts []llm.ContentPart) llm.Message {
	return llm.Message{Role: "user", Content: "x", ContentParts: parts}
}
`, "bound to parts is an unrecognized shape"},
		"parts bound to a call": {header + `func parts() []llm.ContentPart { return nil }

var m = llm.Message{Role: "user", Content: "x", ContentParts: parts()}
`, "bound to parts() is an unrecognized shape"},
		"part element that is not a literal": {header + `func build(p llm.ContentPart) llm.Message {
	return llm.Message{Role: "user", Content: "x", ContentParts: []llm.ContentPart{p}}
}
`, "element 1 is not a ContentPart literal"},
		"positional part element": {header + `var m = llm.Message{Role: "user", Content: "x", ContentParts: []llm.ContentPart{{"text", "y", "", ""}}}
`, "positional ContentPart element"},
		"part without a Type": {header + `var m = llm.Message{Role: "user", Content: "x", ContentParts: []llm.ContentPart{{Text: "y"}}}
`, "without a Type field"},
		"part Type bound to a const": {header + `const partText = "text"

var m = llm.Message{Role: "user", Content: "x", ContentParts: []llm.ContentPart{{Type: partText, Text: "y"}}}
`, "Type partText is not a string literal"},
		"part Type outside the adapter set": {header + `var m = llm.Message{Role: "user", Content: "x", ContentParts: []llm.ContentPart{{Type: "audio", Text: "y"}}}
`, `Type "audio" is outside the adapter's part set`},
		"text part without Text": {header + `var m = llm.Message{Role: "user", Content: "x", ContentParts: []llm.ContentPart{{Type: "text"}}}
`, "text ContentPart without a Text field"},
		"image part carrying Text": {header + `var m = llm.Message{Role: "user", Content: "x", ContentParts: []llm.ContentPart{{Type: "image_url", ImageURL: "data:", Text: "y"}}}
`, "image ContentPart carrying a Text field"},
		"system text part bound to a parameter": {header + `func build(caption string) llm.Message {
	return llm.Message{Role: "system", Content: "x", ContentParts: []llm.ContentPart{{Type: "text", Text: caption}}}
}
`, "SystemMessage.ContentParts.Text: identifier \"caption\" is not a single-assignment"},
		"user text part with an unresolvable identifier": {header + `var m = llm.Message{Role: "user", Content: "x", ContentParts: []llm.ContentPart{{Type: "text", Text: mystery}}}
`, "UserMessage.ContentParts.Text: "},
	}
	for name, c := range cases {
		dir := writeScratchPackage(t, map[string]string{"p.go": c.src})
		_, _, err := ScanPromptSurfaces(dir)
		if err == nil || !strings.Contains(err.Error(), c.want) {
			t.Errorf("%s: expected an error containing %q, got %v", name, c.want, err)
		}
	}
}

func TestScanPromptSurfaces_UnrecognizedShapesFailLoud(t *testing.T) {
	cases := map[string]struct{ src, want string }{
		"local variable": {`package scratch

import "github.com/hanchaoqun/codrax/internal/llm"

func build(desc string) llm.ToolSchema {
	return llm.ToolSchema{Name: "x", Description: desc}
}
`, "unrecognized"},
		"function result": {`package scratch

import "github.com/hanchaoqun/codrax/internal/llm"

func text() string { return "" }

var tool = llm.ToolSchema{Name: "x", Description: text()}
`, "unrecognized"},
		"positional element": {`package scratch

import "github.com/hanchaoqun/codrax/internal/llm"

var tool = llm.ToolSchema{"x", "desc", nil}
`, "unrecognized"},
		"prompt const bound to a call": {`package scratch

func text() string { return "" }

var dynamicSystemPrompt = text()
`, "unrecognized"},
		"reassigned local in a system message": {`package scratch

import "github.com/hanchaoqun/codrax/internal/llm"

func build(flag bool) []llm.Message {
	prompt := "a"
	if flag {
		prompt = "b"
	}
	return []llm.Message{{Role: "system", Content: prompt}}
}
`, "unrecognized"},
		"other package's call as the whole system Content": {`package scratch

import (
	"github.com/hanchaoqun/codrax/internal/llm"
	"github.com/hanchaoqun/codrax/internal/other"
)

func build() []llm.Message {
	return []llm.Message{{Role: "system", Content: other.Build()}}
}
`, "as the whole value is an unrecognized shape"},
		"other package's call through a local as the whole system Content": {`package scratch

import (
	"github.com/hanchaoqun/codrax/internal/llm"
	"github.com/hanchaoqun/codrax/internal/other"
)

func build() []llm.Message {
	p := other.Build()
	return []llm.Message{{Role: "system", Content: p}}
}
`, "as the whole value is an unrecognized shape"},
		"Role bound to a const": {`package scratch

import "github.com/hanchaoqun/codrax/internal/llm"

const roleSystem = "system"

var m = llm.Message{Role: roleSystem, Content: "x"}
`, "not a string literal"},
		"Role outside the provider set": {`package scratch

import "github.com/hanchaoqun/codrax/internal/llm"

var m = llm.Message{Role: "function", Content: "x"}
`, "outside the provider role set"},
		"message without Content": {`package scratch

import "github.com/hanchaoqun/codrax/internal/llm"

var m = llm.Message{Role: "user"}
`, "without a Content field"},
		"message without Role": {`package scratch

import "github.com/hanchaoqun/codrax/internal/llm"

var m = llm.Message{Content: "x"}
`, "without a Role field"},
		"Role literal on a non-llm type": {`package scratch

type turn struct{ Role, Content string }

var m = turn{Role: "system", Content: "x"}
`, "type other than llm.Message"},
		"unresolvable identifier on the user flow": {`package scratch

import "github.com/hanchaoqun/codrax/internal/llm"

var m = llm.Message{Role: "user", Content: mystery}
`, "not resolvable"},
		"unresolvable call on the user flow": {`package scratch

import "github.com/hanchaoqun/codrax/internal/llm"

func build() llm.Message { return llm.Message{Role: "user", Content: mystery()} }
`, "not resolvable"},
	}
	for name, c := range cases {
		dir := writeScratchPackage(t, map[string]string{"p.go": c.src})
		_, _, err := ScanPromptSurfaces(dir)
		if err == nil || !strings.Contains(err.Error(), c.want) {
			t.Errorf("%s: expected an error containing %q, got %v", name, c.want, err)
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
