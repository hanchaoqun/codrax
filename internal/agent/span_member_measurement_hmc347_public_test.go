package agent

import (
	"bytes"
	"encoding/json"
	"reflect"
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/tool"
	"github.com/hanchaoqun/codrax/internal/types"
)

func hmc347MemberTables(ctx *types.AgentContext) []types.RuntimeMeasurementTable {
	var tables []types.RuntimeMeasurementTable
	for _, table := range types.BuildAnswerSemanticViewForAgentContext(ctx).RuntimeMeasurementContract.Choices() {
		if strings.Contains(table.ObservationID, "#span_members:") {
			tables = append(tables, table)
		}
	}
	return tables
}

// Exercise the public native query, current-source qualification, actual
// finalizer adapter and public answer emitter. An envelope is not a member.
func TestHMC347PublicDiscreteSpanMembersKeepNativeEndpoints(t *testing.T) {
	ctx, result := b1706SemanticFactContext(t)
	original, _ := json.Marshal(result)
	projection := hmc218CausalProjection(t, ctx)
	tables := hmc347MemberTables(ctx)
	if len(tables) != 2 {
		t.Fatalf("native JIT family and separate class-work family lack selectable member intervals: %d", len(tables))
	}
	_, messages := finalizerReaderFieldMessages(t, ctx)
	blocks := []any{map[string]any{"id": "summary", "kind": "summary", "text": "分别展示已观测的业务区间；区间之间的关系另需证据。"}}
	seen := map[string]bool{}
	for i, table := range tables {
		if table.View != types.RuntimeMeasurementMembers || !table.HasNativeMeasurementFacts() || table.MemberSet != nil || table.DisplayPopulation != nil {
			t.Fatal("member display lost current native facts or gained population authority")
		}
		if !strings.Contains(messages, table.ObservationID) || !strings.Contains(messages, "开始（秒，含）") {
			t.Fatal("actual finalizer context lost member selector or exact interval ruler")
		}
		for _, row := range table.Rows {
			seen[row[2]+".."+row[3]] = true
		}
		kind := "table"
		if i == 1 {
			kind = "diagram"
		}
		blocks = append(blocks, map[string]any{"id": kind, "kind": kind, "runtime_measurement": map[string]any{"observation_id": table.ObservationID, "view": table.View}})
	}
	for _, want := range []string{"10.001000..10.002781", "10.004000..10.004607", "10.006000..10.006250"} {
		if !seen[want] {
			t.Fatalf("native member endpoint missing: %s / %v", want, seen)
		}
	}
	if seen["10.001000..10.004607"] {
		t.Fatal("family envelope was substituted for two disjoint native intervals")
	}
	args, _ := json.Marshal(map[string]any{"blocks": blocks})
	out, err := (&tool.EmitAnswerDocument{}).Execute(types.ToolBusContext(ctx, types.AgentFinalizer), args)
	if err != nil || !out.Success {
		t.Fatalf("same native facts could not bind table/fact diagram: %v / %s", err, out.Summary)
	}
	for i, table := range tables {
		bound := ctx.Mutable.AnswerDocumentV2().Blocks[i+1].RuntimeMeasurement
		if bound == nil || !bound.IsBound() || !reflect.DeepEqual(bound.BoundTable.Rows, table.Rows) || !reflect.DeepEqual(bound.BoundTable.Notes, table.Notes) {
			t.Fatal("member facts changed between context and answer carrier")
		}
	}
	after, _ := json.Marshal(result)
	if !bytes.Equal(original, after) || !bytes.Equal(projection, hmc218CausalProjection(t, ctx)) {
		t.Fatal("display changed source facts or causal projection")
	}
	// Removing only this display publication must leave the original causal
	// identities, credentials, interval projection and numeric values intact.
	var old []types.ObservationRecord
	for _, record := range result.Observations {
		if !strings.Contains(record.ID, "#span_members:") {
			old = append(old, record)
		}
	}
	result.Observations = old
	b1706ReplaceResult(ctx, result)
	if !bytes.Equal(projection, hmc218CausalProjection(t, ctx)) {
		t.Fatal("new measured member display minted a causal seat")
	}
}
