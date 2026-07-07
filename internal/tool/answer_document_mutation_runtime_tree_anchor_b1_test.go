package tool

// B1 anchor-election display pins (§10-B1 + §12.3 裁定3): the 🎯 root and its
// label lane follow the compile-side anchor election.
//   - elected anchor → root = user thread, header keeps ‹用户关注线程›, no
//     免责横幅 — even when the renderer-side R2 entity list diverges from the
//     compile-side election source (Q4-E entity-starvation family);
//   - no matching path → legacy anchor + 免责横幅 byte-stable (pin ③ display
//     half).

import (
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/types"
)

func anchorB1ToolPathRecord(id, target, path string) types.ObservationRecord {
	return types.ObservationRecord{
		ID:              id,
		Origin:          types.AnswerEvidenceOriginRuntimeArtifact,
		Producer:        "trace_query",
		Role:            types.AnswerAggregateRoleSupportingCoverage,
		GroundingPolicy: types.ClaimGroundingHard,
		ClaimKey:        "wakeup_chain:path",
		Subject:         target,
		Predicate:       "wakeup_chain",
		Object:          path,
		Confidence:      0.82,
	}
}

func anchorB1ToolProjection(t *testing.T, entities []string, records ...types.ObservationRecord) types.TraceCausalProjection {
	t.Helper()
	typed := make([]types.AnchorUserEntity, 0, len(entities))
	for _, e := range entities {
		typed = append(typed, types.AnchorUserEntity{Value: e, TypedLane: true})
	}
	set := types.CompileTraceCausalProjectionSet(types.ObservationLedger{
		Records:            records,
		AnchorUserEntities: typed,
	})
	if len(set.Projections) != 1 {
		t.Fatalf("expected one projection, got %d", len(set.Projections))
	}
	return set.Projections[0]
}

// Pin ① display half (q1 shape): with the typed user entity "42591" in the
// ledger, the tree roots on the user thread and the header label stays
// ‹用户关注线程› with no 免责横幅.
func TestRuntimeTraceProjTreeAnchorElectionRootsUserThread(t *testing.T) {
	projection := anchorB1ToolProjection(t, []string{"42591"},
		anchorB1ToolPathRecord("path-vsync", "VSyncGenerator-2270",
			"tppmgr-idle-7-192 -> VSyncGenerator-2270"),
		anchorB1ToolPathRecord("path-user", "oney.hmn.berlin-42591",
			"VSyncGenerator-2270 -> oney.hmn.berlin-42591"),
	)
	model := buildRuntimeTraceProjTreeModel(projection, nil, true)
	if model.Target != "oney.hmn.berlin-42591" {
		t.Fatalf("🎯 root must be the elected user thread, got %q", model.Target)
	}
	if !model.TargetUserElected {
		t.Fatalf("model must carry the typed election marker")
	}
	runtimeTraceProjApplyUserFocus(&model, runtimeTraceProjUserFocus{Entities: []string{"42591"}})
	if model.RootFocusAnchorOnly {
		t.Fatalf("elected anchor must keep ‹用户关注线程›, not the 免责 lane")
	}
	if header := runtimeTraceProjTreeHeaderLabel(model, true); !strings.Contains(header, "‹用户关注线程›") {
		t.Fatalf("header must carry ‹用户关注线程›, got %q", header)
	}
}

// Banner follows the ELECTION even when the renderer-side R2 entity list
// diverges (Q4-E shape: analyzer hands the renderer a frame number while the
// compile-side ledger entities elected the pid path) — the compile-side typed
// marker short-circuits the mismatch disclaimer.
func TestRuntimeTraceProjTreeAnchorElectionOverridesDivergentFocus(t *testing.T) {
	projection := anchorB1ToolProjection(t, []string{"42591"},
		anchorB1ToolPathRecord("path-vsync", "VSyncGenerator-2270",
			"tppmgr-idle-7-192 -> VSyncGenerator-2270"),
		anchorB1ToolPathRecord("path-user", "oney.hmn.berlin-42591",
			"VSyncGenerator-2270 -> oney.hmn.berlin-42591"),
	)
	model := buildRuntimeTraceProjTreeModel(projection, nil, true)
	runtimeTraceProjApplyUserFocus(&model, runtimeTraceProjUserFocus{Entities: []string{"3703298"}})
	if model.RootFocusAnchorOnly {
		t.Fatalf("compile-side election must short-circuit the R2 mismatch disclaimer")
	}
	if header := runtimeTraceProjTreeHeaderLabel(model, true); !strings.Contains(header, "‹用户关注线程›") {
		t.Fatalf("header must keep ‹用户关注线程› on an elected anchor, got %q", header)
	}
}

// Pin ③ display half: an entity with NO matching path keeps the legacy anchor
// AND the 免责横幅 lane byte-stable (‹分析锚点线程› + roster line).
func TestRuntimeTraceProjTreeAnchorNoMatchKeepsDisclaimer(t *testing.T) {
	projection := anchorB1ToolProjection(t, []string{"42591"},
		anchorB1ToolPathRecord("path-vsync", "VSyncGenerator-2270",
			"tppmgr-idle-7-192 -> VSyncGenerator-2270"),
	)
	model := buildRuntimeTraceProjTreeModel(projection, nil, true)
	if model.Target != "VSyncGenerator-2270" || model.TargetUserElected {
		t.Fatalf("no-match lane must keep the legacy anchor unelected, got %q elected=%v",
			model.Target, model.TargetUserElected)
	}
	runtimeTraceProjApplyUserFocus(&model, runtimeTraceProjUserFocus{Entities: []string{"42591"}})
	if !model.RootFocusAnchorOnly {
		t.Fatalf("no-match lane must keep the R2 免责 lane")
	}
	if header := runtimeTraceProjTreeHeaderLabel(model, true); !strings.Contains(header, "‹分析锚点线程›") {
		t.Fatalf("header must carry ‹分析锚点线程› on the unelected mismatch lane, got %q", header)
	}
	if len(model.RootFocusUserEntities) == 0 || model.RootFocusUserEntities[0] != "42591" {
		t.Fatalf("免责 roster must keep the user entity, got %v", model.RootFocusUserEntities)
	}
}
