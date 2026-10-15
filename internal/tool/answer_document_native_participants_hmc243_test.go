package tool

import (
	"encoding/json"
	"fmt"
	"reflect"
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/mermaidcompat"
	"github.com/hanchaoqun/codrax/internal/render"
	"github.com/hanchaoqun/codrax/internal/types"
)

func nativeParticipantsPublicInputHMC243(t *testing.T) (*types.BusContext, []RuntimeDiagramRelation, json.RawMessage) {
	t.Helper()
	ctx, rows := requestedThreadFixtureCustomHMC242(t, "", func(p map[string]any) {
		p["diagram_hint"].(map[string]any)["participants"].([]any)[0].(map[string]any)["identity"] = "target"
	})
	doc := requestedThreadDocHMC242(rows, true)
	b := &doc.Blocks[1]
	b.Diagram.Body += "participant target as target-42\nparticipant L as loader-11\nparticipant T as target-42\nparticipant W as worker-13\nNote over T: original model-owned state; target and T text stay unchanged\n"
	var payload map[string]any
	raw, _ := json.Marshal(doc)
	_ = json.Unmarshal(raw, &payload)
	diagram := payload["blocks"].([]any)[1].(map[string]any)["diagram"].(map[string]any)
	var selected []any
	for _, row := range rows {
		alias := "L"
		if row.threadEndpoints.from.PID == 13 {
			alias = "W"
		}
		found := false
		for _, prior := range selected {
			found = found || prior.(map[string]any)["node_id"] == alias
		}
		if !found {
			selected = append(selected, map[string]any{"node_id": alias, "native_participant": row.FromNode})
		}
	}
	selected = append(selected, map[string]any{"node_id": "T", "native_participant": rows[0].ToNode}, map[string]any{"node_id": "target", "native_participant": rows[0].ToNode})
	diagram["runtime_participants_v1"] = selected
	raw, _ = json.Marshal(payload)
	return ctx, rows, raw
}

func mutateNativeParticipantsInputHMC243(t *testing.T, raw json.RawMessage, change func(map[string]any, map[string]any)) json.RawMessage {
	t.Helper()
	var root map[string]any
	if err := json.Unmarshal(raw, &root); err != nil {
		t.Fatal(err)
	}
	block := root["blocks"].([]any)[1].(map[string]any)
	change(block, block["diagram"].(map[string]any))
	out, err := json.Marshal(root)
	if err != nil {
		t.Fatal(err)
	}
	return out
}

func TestHMC243NativeParticipantsPublicBoundaries(t *testing.T) {
	for _, mode := range []string{"stale", "undeclared", "duplicate_selection", "conflicting_selection", "anchor_conflict", "requested_conflict", "nonsequence", "activation", "create", "destroy", "ref_region", "duplicate_declaration"} {
		t.Run(mode, func(t *testing.T) {
			ctx, rows, raw := nativeParticipantsPublicInputHMC243(t)
			raw = mutateNativeParticipantsInputHMC243(t, raw, func(block, diagram map[string]any) {
				ss := diagram["runtime_participants_v1"].([]any)
				body := diagram["body"].(string)
				switch mode {
				case "stale":
					ss[0].(map[string]any)["native_participant"] = "rt_thread_stale"
				case "undeclared":
					ss[0].(map[string]any)["node_id"] = "Absent"
				case "duplicate_selection":
					ss = append(ss, ss[0])
				case "conflicting_selection":
					ss = append(ss, map[string]any{"node_id": "L", "native_participant": rows[0].ToNode})
				case "anchor_conflict":
					ss = append(ss, map[string]any{"node_id": rows[0].FromNode, "native_participant": rows[0].ToNode})
				case "requested_conflict":
					ss[len(ss)-1].(map[string]any)["native_participant"] = rows[0].FromNode
				case "nonsequence":
					diagram["kind"] = "flow"
					body = "flowchart LR\n A-->B\n"
				case "activation":
					body += "activate " + rows[0].ToNode + "\n"
				case "create":
					body += "create participant " + rows[0].ToNode + "\n"
				case "destroy":
					body += "destroy " + rows[0].ToNode + "\n"
				case "ref_region":
					body += "ref over T: arbitrary region\n"
				case "duplicate_declaration":
					body += "participant T as other\n"
				}
				diagram["runtime_participants_v1"], diagram["body"] = ss, body
			})
			res, err := (&EmitAnswerDocument{}).Execute(ctx, raw)
			if err != nil || res.Success || ctx.Mutable.AnswerDocumentV2() != nil {
				t.Fatalf("%s acquired display/identity: %v %s", mode, err, res.Summary)
			}
		})
	}
}

func TestHMC243NativeParticipantsPublicPatchCloneAndStale(t *testing.T) {
	ctx, _, raw := nativeParticipantsPublicInputHMC243(t)
	res, err := (&EmitAnswerDocument{}).Execute(ctx, raw)
	if err != nil || !res.Success {
		t.Fatalf("emit: %v %s", err, res.Summary)
	}
	before := ctx.Mutable.AnswerDocumentV2()
	copy := ctx.Mutable.AnswerDocumentV2()
	copy.Blocks[1].Diagram.RuntimeParticipantsV1[0].NativeParticipant = "rt_thread_forged"
	if !reflect.DeepEqual(before, ctx.Mutable.AnswerDocumentV2()) {
		t.Fatal("selector slice aliases accepted document")
	}
	patch, _ := json.Marshal(map[string]any{"replace_blocks": []types.AnswerBlock{before.Blocks[1]}, "unchanged_block_ids": []string{before.Blocks[0].ID}})
	res, err = (&EmitAnswerDocumentPatch{}).Execute(ctx, patch)
	if err != nil || !res.Success {
		t.Fatalf("same current selections must rebind: %v %s", err, res.Summary)
	}
	if got := ctx.Mutable.AnswerDocumentV2().Blocks[1]; !reflect.DeepEqual(before.Blocks[1], got) {
		t.Fatalf("repeat patch is not idempotent: before=%+v after=%+v", before.Blocks[1], got)
	}
	bad, _ := json.Marshal(map[string]any{"replace_blocks": []types.AnswerBlock{copy.Blocks[1]}, "unchanged_block_ids": []string{before.Blocks[0].ID}})
	res, err = (&EmitAnswerDocumentPatch{}).Execute(ctx, bad)
	if err != nil || res.Success || !reflect.DeepEqual(before, ctx.Mutable.AnswerDocumentV2()) {
		t.Fatalf("stale JSON selector mutated accepted doc: %v %s", err, res.Summary)
	}
}

func TestHMC243NativeParticipantsPublicLifecycleMerge(t *testing.T) {
	for _, mode := range []string{"representative_activation", "native_inline_activation", "native_inline_deactivation"} {
		t.Run(mode, func(t *testing.T) {
			ctx, rows, raw := nativeParticipantsPublicInputHMC243(t)
			raw = mutateNativeParticipantsInputHMC243(t, raw, func(_, diagram map[string]any) {
				body := diagram["body"].(string)
				if mode == "representative_activation" {
					body += "activate T\n"
				} else {
					suffix := "+"
					if mode == "native_inline_deactivation" {
						suffix = "-"
					}
					body = strings.Replace(body, "->>"+rows[0].ToNode+":", "->>"+suffix+rows[0].ToNode+":", 1)
				}
				diagram["body"] = body
			})
			res, err := (&EmitAnswerDocument{}).Execute(ctx, raw)
			if err != nil || res.Success || ctx.Mutable.AnswerDocumentV2() != nil {
				t.Fatalf("%s merged a lifecycle as display-only: %v %s", mode, err, res.Summary)
			}
		})
	}
}

func TestHMC243NativeParticipantsUnselectedNoteAndProsePreserved(t *testing.T) {
	ctx, _, raw := nativeParticipantsPublicInputHMC243(t)
	raw = mutateNativeParticipantsInputHMC243(t, raw, func(_, diagram map[string]any) {
		diagram["body"] = diagram["body"].(string) + "participant U as target-42\nNote over U: same visible label does not prove identity\n"
	})
	res, err := (&EmitAnswerDocument{}).Execute(ctx, raw)
	if err != nil || !res.Success {
		t.Fatalf("honest unbound model note: %v %s", err, res.Summary)
	}
	doc := ctx.Mutable.AnswerDocumentV2()
	got := doc.Blocks[1].Diagram.Body
	if !strings.Contains(got, "participant U as target-42") || !strings.Contains(got, "Note over U: same visible label does not prove identity") {
		t.Fatalf("label was used as identity: %s", got)
	}
	visible := render.RenderAnswerDocument(doc, "zh")
	if !strings.Contains(visible, "original model-owned state; target and T text stay unchanged") || !strings.Contains(visible, "Note over U:") {
		t.Fatalf("render lost untrusted text: %s", visible)
	}
}

func TestHMC243NativeParticipantsExactSyntaxOnly(t *testing.T) {
	body := "sequenceDiagram\n participant A as old\n participant Note as chosen\n participant B as other\n Note over A, B: A and Note in text; A->>B is not another edge\n A-->>+B: untouched A token\n"
	got, lines, err := rewriteNativeSequenceParticipants(body, map[string]string{"A": "Note"})
	if err != nil || !strings.Contains(got, "Note over Note, B: A and Note in text; A->>B is not another edge") || !strings.Contains(got, "Note-->>+B: untouched A token") || lines[5] != 4 {
		t.Fatalf("syntax edit: %v %v %s", err, lines, got)
	}
	again, _, err := rewriteNativeSequenceParticipants(got, map[string]string{"A": "Note"})
	if err != nil || got != again {
		t.Fatalf("syntax not idempotent: %v %s", err, again)
	}
	// An untouched participant's lifetime is independent of lane sharing.
	if _, _, err := rewriteNativeSequenceParticipants(body+"activate B\n", map[string]string{"A": "Note"}); err != nil {
		t.Fatalf("unrelated lifetime was rejected: %v", err)
	}
	// A selection without an actual merge must not alter existing lifecycle
	// semantics or create a new capability restriction.
	plain := "sequenceDiagram\n participant T\n activate T\n T->>+T: existing self activation\n"
	if got, _, err := rewriteNativeSequenceParticipants(plain, map[string]string{"UnusedNative": "T"}); err != nil || got != plain {
		t.Fatalf("no-merge lifecycle changed: %v %s", err, got)
	}
}

func TestHMC243NativeParticipantsNotePairRenders(t *testing.T) {
	body := "sequenceDiagram\n participant N as native\n participant T as selected\n Note over N,T: same observed thread\n"
	got, _, err := rewriteNativeSequenceParticipants(body, map[string]string{"N": "T"})
	if err != nil || !strings.Contains(got, "Note over T: same observed thread") || strings.Contains(got, "T,T") {
		t.Fatalf("duplicate Note endpoint: %v %s", err, got)
	}
	terminal := render.RenderMermaidBlocks("```mermaid\n" + got + "```\n")
	if strings.Contains(terminal, "# ⚠") || !strings.Contains(terminal, "same observed thread") {
		t.Fatalf("shared Note does not render: %s", terminal)
	}
}

func TestHMC243NativeParticipantsNormalizationIsAtomicAndScopeExact(t *testing.T) {
	ctx, rows, raw := nativeParticipantsPublicInputHMC243(t)
	var doc types.AnswerDocumentV2
	if err := json.Unmarshal(raw, &doc); err != nil {
		t.Fatal(err)
	}
	original, _ := json.Marshal(doc)
	bad := cloneAtomicDiagramPatchBlock(doc.Blocks[1])
	bad.ID = "late-bad"
	bad.Diagram.RuntimeParticipantsV1[0].NativeParticipant = "rt_thread_other_scope"
	doc.Blocks = append(doc.Blocks, bad)
	before, _ := json.Marshal(doc)
	if _, err := normalizeNativeDiagramParticipants(&doc, rows, &ctx.AnalysisIR.RequestModel); err == nil {
		t.Fatal("foreign current lane accepted")
	}
	after, _ := json.Marshal(doc)
	if string(before) != string(after) {
		t.Fatal("late failure partially changed earlier graph")
	}
	_ = json.Unmarshal(original, &doc)
	if _, err := normalizeNativeDiagramParticipants(&doc, rows, &ctx.AnalysisIR.RequestModel); err != nil {
		t.Fatal(err)
	}
	if fixed, err := normalizeNativeDiagramParticipants(&doc, rows, &ctx.AnalysisIR.RequestModel); err != nil || fixed != 0 {
		t.Fatalf("normalizer not idempotent: %d %v", fixed, err)
	}
	for _, change := range []string{"source", "window", "clock", "generation", "tgid-known", "tgid-unknown"} {
		other := append([]RuntimeDiagramRelation(nil), rows...)
		for i := range other {
			other[i].FromNode += "_" + change
			other[i].ToNode += "_" + change
		}
		if _, err := normalizeNativeDiagramParticipants(&doc, other, &ctx.AnalysisIR.RequestModel); err == nil {
			t.Fatalf("same labels across %s borrowed prior lane", change)
		}
	}
}

func TestHMC243NativeParticipantsPublicSevenLanes(t *testing.T) {
	ctx, rows, raw := nativeParticipantsPublicInputHMC243(t)
	res, err := (&EmitAnswerDocument{}).Execute(ctx, raw)
	if err != nil || !res.Success {
		t.Fatalf("public emit: %v %s", err, res.Summary)
	}
	got := ctx.Mutable.AnswerDocumentV2().Blocks[1]
	decls := 0
	for _, line := range strings.Split(got.Diagram.Body, "\n") {
		decls += len(mermaidcompat.SequenceParticipantDeclarations(line))
	}
	if decls != 3 {
		t.Fatalf("exact selected native subjects still split into %d lanes: %s", decls, got.Diagram.Body)
	}
	if !strings.Contains(got.Diagram.Body, "Note over T: original model-owned state; target and T text stay unchanged") {
		t.Fatalf("note moved away from chosen display lane or text changed: %s", got.Diagram.Body)
	}
	if len(got.EdgeAnchors) != 3 || len(mermaidcompat.ParseEdges(got.Diagram.Body)) != 3 {
		t.Fatalf("event population changed: %+v", got)
	}
	for i, anchor := range got.EdgeAnchors {
		if anchor.FromIdentity != rows[i].FromIdentity || anchor.ToIdentity != rows[i].ToIdentity || anchor.ToNode != "T" {
			t.Fatalf("event %d identity/lane drift: %+v", i, anchor)
		}
	}
	if strings.Contains(fmt.Sprint(got.ParticipantBoundaries), "unproven") {
		t.Fatalf("proved request acquired missing boundary: %+v", got.ParticipantBoundaries)
	}
}
