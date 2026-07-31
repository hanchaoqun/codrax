package tool

import (
	"fmt"
	"path"
	"strings"

	"github.com/hanchaoqun/codrax/internal/types"
)

const runtimeArtifactPairRelationDisplayLimit = 8

// materializeRuntimeArtifactPairRelationAuthorityBlock publishes the typed
// relationship boundary for independently identified runtime artifacts. It
// intentionally does not inspect the request or model-authored answer text:
// accepted deterministic observation identities are the only trigger and
// BuildRuntimeArtifactPairRelationAuthority is the only relation authority.
//
// This block is independent of the causal-report gate. A generic comparison
// gets this bounded relation table without gaining Trace causal projection;
// an explicit-window or typed-causal request can carry both surfaces.
func materializeRuntimeArtifactPairRelationAuthorityBlock(
	doc *types.AnswerDocumentV2,
	ctx *types.BusContext,
) bool {
	if doc == nil || ctx == nil || ctx.Mutable == nil ||
		len(doc.Blocks) >= maxBlocksPerDoc ||
		answerDocumentHasRuntimeTraceSystemBlockID(doc, runtimeArtifactPairRelationAuthorityBlockID) {
		return false
	}
	ledger := types.CompileObservationLedger(types.ObservationLedgerInputFromBusContext(
		ctx, types.ObservationExtractLedgerEvidenceLimit,
	))
	authority := types.BuildRuntimeArtifactPairRelationAuthority(ledger)
	if !authority.Active || len(authority.Pairs) == 0 {
		return false
	}
	block := runtimeArtifactPairRelationAuthorityBlock(
		authority, runtimeTraceCausalProjectionUseChinese(requestedAnswerDocumentLanguage(ctx)),
	)
	markRuntimeTraceSystemBlock(&block)
	insertAt := answerDocumentInsertionIndexBeforeCaveat(doc)
	doc.Blocks = append(doc.Blocks, types.AnswerBlock{})
	copy(doc.Blocks[insertAt+1:], doc.Blocks[insertAt:])
	doc.Blocks[insertAt] = block
	return true
}

func runtimeArtifactPairRelationAuthorityBlock(
	authority types.RuntimeArtifactPairRelationAuthority,
	zh bool,
) types.AnswerBlock {
	block := types.AnswerBlock{
		ID:          runtimeArtifactPairRelationAuthorityBlockID,
		Kind:        types.BlockTable,
		SurfaceRole: types.SurfacePrincipal,
		FacetIDs:    []string{"observed_artifact_fact"},
	}
	if zh {
		block.Title = "跨工件关系边界（系统确定性）"
		block.Text = "“未证明”不表示相同或不同；它表示当前确定性证据没有共同设备、采集会话或时钟校准锚点。两端各自的 `alignment=identity` 仅说明本地无需变换，相同的时间域标签和时间戳差值都不能证明共享时钟源，也不能单独构成直接时间对齐。"
		block.Columns = []string{"工件对", "共享时钟源", "直接时间对齐", "同设备", "同采集会话", "证据边界"}
	} else {
		block.Title = "Cross-artifact relation boundary (system-determined)"
		block.Text = "`Unproven` means neither same nor different has been established: deterministic evidence contains no shared device, capture-session, or clock-calibration anchor. Endpoint-local `alignment=identity`, matching time-domain labels, and a numeric timestamp difference do not prove a shared clock origin or direct time alignment."
		block.Columns = []string{"Artifact pair", "Shared clock origin", "Direct time alignment", "Same device", "Same capture session", "Evidence boundary"}
	}
	limit := minInt(len(authority.Pairs), runtimeArtifactPairRelationDisplayLimit)
	block.Items = make([]types.AnswerBlockItem, 0, limit)
	for i := 0; i < limit; i++ {
		pair := authority.Pairs[i]
		block.Items = append(block.Items, types.AnswerBlockItem{
			ID:          fmt.Sprintf("runtime-artifact-pair-%d", i+1),
			CitationRef: -1,
			Cells: []string{
				runtimeArtifactPairRelationDisplayLabel(pair),
				runtimeArtifactPairRelationStatusLabel(pair.SharedClockOrigin, zh),
				runtimeArtifactPairRelationStatusLabel(pair.DirectTimeAlignment, zh),
				runtimeArtifactPairRelationStatusLabel(pair.SharedDevice, zh),
				runtimeArtifactPairRelationStatusLabel(pair.SharedCaptureSession, zh),
				runtimeArtifactPairRelationEvidenceBoundary(pair, zh),
			},
		})
	}
	if omitted := len(authority.Pairs) - limit; omitted > 0 {
		if zh {
			block.Text += fmt.Sprintf(" 表格按稳定顺序显示前 %d 对，另有 %d 对遵守同一 typed 权限边界。", limit, omitted)
		} else {
			block.Text += fmt.Sprintf(" The table shows the first %d pairs in stable order; %d additional pairs follow the same typed authority boundary.", limit, omitted)
		}
	}
	return block
}

func runtimeArtifactPairRelationStatusLabel(
	status types.RuntimeArtifactPairRelationStatus,
	zh bool,
) string {
	if status == types.RuntimeArtifactPairRelationUnproven {
		if zh {
			return "未证明"
		}
		return "unproven"
	}
	return strings.TrimSpace(string(status))
}

func runtimeArtifactPairRelationDisplayLabel(pair types.RuntimeArtifactPairRelation) string {
	left := runtimeArtifactRelationEndpointDisplayLabel(pair.Left)
	right := runtimeArtifactRelationEndpointDisplayLabel(pair.Right)
	if left == right {
		left = runtimeArtifactRelationEndpointFullLabel(pair.Left)
		right = runtimeArtifactRelationEndpointFullLabel(pair.Right)
	}
	return left + " ↔ " + right
}

func runtimeArtifactRelationEndpointDisplayLabel(endpoint types.RuntimeArtifactRelationEndpoint) string {
	raw := strings.TrimSpace(strings.ReplaceAll(endpoint.Path, "\\", "/"))
	if raw != "" && raw != "multiple" {
		return path.Base(raw)
	}
	return strings.TrimSpace(endpoint.ArtifactID)
}

func runtimeArtifactRelationEndpointFullLabel(endpoint types.RuntimeArtifactRelationEndpoint) string {
	if raw := strings.TrimSpace(endpoint.Path); raw != "" && raw != "multiple" {
		return raw
	}
	return strings.TrimSpace(endpoint.ArtifactID)
}

func runtimeArtifactPairRelationEvidenceBoundary(
	pair types.RuntimeArtifactPairRelation,
	zh bool,
) string {
	parts := make([]string, 0, 2)
	if pair.SameTimeDomainLabel || pair.SameCanonicalDomain {
		if zh {
			parts = append(parts, "仅本地时间域标签相同")
		} else {
			parts = append(parts, "local time-domain labels match only")
		}
	}
	if pair.LocalIdentityOnly {
		if zh {
			parts = append(parts, "identity 仅端点本地有效")
		} else {
			parts = append(parts, "identity is endpoint-local only")
		}
	}
	if len(parts) == 0 {
		if zh {
			return "缺少共同关系锚点"
		}
		return "shared relation anchors are absent"
	}
	return strings.Join(parts, "；")
}
