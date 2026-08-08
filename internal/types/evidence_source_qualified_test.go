package types

import "testing"

func TestDeterministicSourceQualifiedEvidenceSymbolRequiresExactProducerAndFileAxis(t *testing.T) {
	item := EvidenceItem{
		Producer: "dataflow.lowerer.go",
		Source:   "internal/types/stage_binding.go",
	}
	if got, ok := DeterministicSourceQualifiedEvidenceSymbol(item,
		"internal/types/stage_binding.go:ReadModeMainStageBindings"); !ok || got != "ReadModeMainStageBindings" {
		t.Fatalf("exact deterministic source symbol = %q ok=%v", got, ok)
	}
	if got, ok := DeterministicSourceQualifiedEvidenceSymbol(item,
		"internal/other/stage_binding.go:ReadModeMainStageBindings"); ok || got != "" {
		t.Fatalf("different source axis must fail closed: %q ok=%v", got, ok)
	}
	item.Producer = "model"
	if got, ok := DeterministicSourceQualifiedEvidenceSymbol(item,
		"internal/types/stage_binding.go:ReadModeMainStageBindings"); ok || got != "" {
		t.Fatalf("model-authored source prefix must not mint an identity: %q ok=%v", got, ok)
	}
}
