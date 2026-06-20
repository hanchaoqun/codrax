package tool

import (
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/hanchaoqun/codrax/internal/types"
)

func TestToolJSONSurfaceRegistryCoversEmitTools(t *testing.T) {
	for _, tool := range []jsonSurfaceTool{
		&EmitAnalysis{},
		&EmitAnswerDocument{},
		&EmitAnswerDocumentPatch{},
		&EmitAnswerSymbol{},
		&EmitChangePlan{},
		&EmitEvidence{},
		&EmitHypothesisVerdict{},
		&EmitInvestigationComplete{},
		&EmitLogSegmentation{},
		&EmitLogTriage{},
		&EmitMultiRepoFocus{},
		&EmitPerfSegmentation{},
		&EmitPerfTrace{},
		&EmitPlanChange{},
		&EmitPlanSkeleton{},
		&EmitTestResults{},
		&EmitWriteAnalysis{},
		&EmitWriteWorkflowDecision{},
	} {
		surface, ok := toolJSONSurfaceDescriptorForTool(tool.Name())
		if !ok {
			t.Fatalf("missing JSON surface descriptor for %s", tool.Name())
		}
		if surface.ToolName != tool.Name() || len(surface.AcceptedFieldPaths) == 0 {
			t.Fatalf("surface for %s is not useful: %+v", tool.Name(), surface)
		}
	}
}

func TestStrictDecodeFailureAttachesToolJSONSurfaceMetadata(t *testing.T) {
	res, _ := failStrictDecode("emit_change_plan", time.Now(), errors.New(`json: unknown field "kind"`), nil, nil)
	if res.Repair == nil {
		t.Fatalf("expected repair: %+v", res)
	}
	surface, ok := types.ToolJSONSurfaceDescriptorFromToolRepair(res.ToolName, res.Repair)
	if !ok {
		t.Fatalf("missing surface metadata: %+v", res.Repair.Metadata)
	}
	if surface.ToolName != "emit_change_plan" ||
		len(surface.AcceptedFieldPaths) == 0 ||
		len(surface.FailingFieldPaths) == 0 {
		t.Fatalf("surface mismatch: %+v", surface)
	}
	if !strings.HasPrefix(surface.FailingFieldPaths[0], "$.") || !strings.HasSuffix(surface.FailingFieldPaths[0], ".kind") {
		t.Fatalf("unexpected failing path: %+v", surface.FailingFieldPaths)
	}
}
